package vision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// PerCallTimeout bounds one backend completion. qwen3-vl:4b averaged
	// 149 s/image with a 259 s worst case on the prod GPU (run 5); the
	// timeout leaves headroom without letting a hung call eat the job.
	PerCallTimeout = 5 * time.Minute
	// LadderRungs is the escalation ladder length (askJSON below).
	LadderRungs = 3
	// MaxCallsPerImage derives the per-image call bound FROM the ladder
	// so the job deadline cannot drift from it (plan phase 1 step 5):
	// each of the two passes runs at most 2×rungs calls (the ladder plus
	// one full restart after the OOM shrink), but the shrink is shared
	// across the whole image, so only one pass can ever restart:
	//   worst = rungs (clean pass) + 2×rungs (shrunk pass) = 3×rungs.
	// TestRecognizerWorstCaseCallCount drives exactly this sequence.
	MaxCallsPerImage = 3 * LadderRungs
	// defaultNumCtx mirrors the harness NUM_CTX.
	defaultNumCtx = 8192
)

// MaxImageBudget is the wall-clock budget for one image, derived — never
// hand-picked. Job deadlines are images × MaxImageBudget.
func MaxImageBudget() time.Duration {
	return time.Duration(MaxCallsPerImage) * PerCallTimeout
}

// backendSem serializes every backend call across the whole process. The
// hardware constraint is one 6 GB card — a per-user limit would let N
// users OOM it simultaneously.
var backendSem = make(chan struct{}, 1)

// Recognizer runs the two-pass read (rows, then slot count) over one
// prepared image, with the harness's escalation ladder per pass.
type Recognizer struct {
	backend Backend
}

func NewRecognizer(b Backend) *Recognizer {
	return &Recognizer{backend: b}
}

// Reading is one image's extraction. Slots is nil for wheel_result
// screens (asking invites a fabricated number — run 3), when the screen
// states no count, or when the slot pass failed (rows still prefill).
type Reading struct {
	Screen   Screen
	Slots    *Slots
	Attempts int // backend calls spent on this image
}

// Read extracts one screenshot. rawImage is the uploaded bytes (any
// supported format); catalog is the bank's known titles for the
// constrained vocabulary. ErrBadImage means skip this image with a note;
// ErrUnavailable / ErrFailed fail the job.
func (r *Recognizer) Read(ctx context.Context, rawImage []byte, catalog []string) (Reading, error) {
	img, err := Prepare(rawImage, DefaultLongEdge)
	if err != nil {
		return Reading{}, err
	}
	st := &readState{raw: rawImage, img: img}

	var screen Screen
	if err := r.askJSON(ctx, st, RowPrompt(catalog), RowSchema, &screen); err != nil {
		return Reading{Attempts: st.calls}, err
	}
	reading := Reading{Screen: screen, Attempts: st.calls}

	// Second pass: a narrow header/footer read got 5/5 slot counts where
	// the combined pass got 3/8 (run 2). Soft-fail: losing the count must
	// not lose the rows.
	if screen.ScreenType != "wheel_result" {
		var slots Slots
		err := r.askJSON(ctx, st, SlotPrompt, SlotSchema, &slots)
		reading.Attempts = st.calls
		switch {
		case err == nil && slots.SlotCount > 0:
			reading.Slots = &slots
		case err != nil && ctx.Err() != nil:
			return reading, ctx.Err()
		}
	}
	return reading, nil
}

// readState carries the per-image image bytes and the single shared
// shrink across both passes.
type readState struct {
	raw    []byte
	img    []byte
	shrunk bool
	calls  int
}

// askJSON is the ported ask_json ladder: (1) as configured, (2) schema +
// think with room to finish, (3) grammar off + prose forbidden. On the
// OOM signature the image is re-prepared at RetryLongEdge and the ladder
// restarts once; any other backend error aborts the pass (harness
// parity — HTTP errors never walked the ladder).
func (r *Recognizer) askJSON(ctx context.Context, st *readState, prompt string, schema json.RawMessage, target any) error {
	rungs := [LadderRungs]Request{
		{Prompt: prompt, Schema: schema, NumCtx: defaultNumCtx},
		{Prompt: prompt, Schema: schema, Think: true, NumPredict: 8192, NumCtx: 16384},
		{Prompt: prompt + noReason, NumPredict: 2048, NumCtx: defaultNumCtx},
	}
	var last Response
	for rung := 0; rung < len(rungs); rung++ {
		req := rungs[rung]
		req.ImageJPEG = st.img
		resp, err := r.complete(ctx, req)
		st.calls++
		if err != nil {
			if oomSignature(err) && !st.shrunk {
				smaller, perr := Prepare(st.raw, RetryLongEdge)
				if perr != nil {
					return perr
				}
				st.img, st.shrunk = smaller, true
				rung = -1 // restart the ladder at the reduced resolution
				continue
			}
			if errors.Is(err, ErrUnavailable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return fmt.Errorf("%w: %v", ErrFailed, err)
		}
		last = resp
		if raw := ExtractJSON(resp.Content, resp.Thinking); raw != nil {
			if uerr := json.Unmarshal(raw, target); uerr == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: no JSON after %d attempts (last: content=%d chars, thinking=%d chars, done_reason=%s)",
		ErrFailed, len(rungs), len(last.Content), len(last.Thinking), last.DoneReason)
}

// complete acquires the process-wide backend slot, then runs one call
// under PerCallTimeout.
func (r *Recognizer) complete(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err // don't race the free semaphore in the select below
	}
	select {
	case backendSem <- struct{}{}:
		defer func() { <-backendSem }()
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
	callCtx, cancel := context.WithTimeout(ctx, PerCallTimeout)
	defer cancel()
	return r.backend.Complete(callCtx, req)
}

// oomSignature spots the vision-encoder out-of-memory shape: HTTP 500
// whose detail mentions memory/CUDA, or the bare «unexpected EOF» run 5
// hit on the card-grid shot.
func oomSignature(err error) bool {
	var be *BackendError
	if errors.As(err, &be) && be.Status == 500 {
		d := strings.ToLower(be.Detail)
		return strings.Contains(d, "memory") || strings.Contains(d, "cuda") || strings.Contains(d, "unexpected eof")
	}
	return strings.Contains(strings.ToLower(err.Error()), "unexpected eof")
}

package vision

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"strings"
	"testing"
	"time"
)

const (
	menuJSON  = `{"screen_type":"menu","bank":"Альфа-Банк","rows":[{"percent":"5","title":"Супермаркеты","state":"unchecked"}]}`
	wheelJSON = `{"screen_type":"wheel_result","bank":"Альфа-Банк","rows":[{"percent":"7","title":"Такси","state":"checked"}]}`
	slotJSON  = `{"source_text":"Выберите 4 категории","slot_count":4}`
)

// scriptedBackend plays a fixed sequence of responses and records every
// request. This is the plan's contract C1 in miniature: the fake sits at
// the Backend seam, so the real ladder, prompts and extractor all run.
type scriptedBackend struct {
	t      *testing.T
	script []func(req Request) (Response, error)
	calls  []Request
}

func (b *scriptedBackend) Name() string { return "scripted" }

func (b *scriptedBackend) Complete(_ context.Context, req Request) (Response, error) {
	i := len(b.calls)
	b.calls = append(b.calls, req)
	if i >= len(b.script) {
		b.t.Fatalf("unexpected backend call #%d", i+1)
	}
	return b.script[i](req)
}

func ok(body string) func(Request) (Response, error) {
	return func(Request) (Response, error) { return Response{Content: body}, nil }
}

func junk() func(Request) (Response, error) {
	return func(Request) (Response, error) { return Response{Content: "не могу", DoneReason: "stop"}, nil }
}

func oom() func(Request) (Response, error) {
	return func(Request) (Response, error) {
		return Response{}, &BackendError{Backend: "scripted", Status: 500, Detail: "cudaMalloc failed: out of memory"}
	}
}

func longEdgeOf(t *testing.T, jpg []byte) int {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	return max(cfg.Width, cfg.Height)
}

func TestReadHappyPathTwoPasses(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){ok(menuJSON), ok(slotJSON)}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 1290, 2796), []string{"Супермаркеты", "Такси"})
	if err != nil {
		t.Fatal(err)
	}
	if reading.Screen.ScreenType != "menu" || len(reading.Screen.Rows) != 1 || reading.Screen.Rows[0].Title != "Супермаркеты" {
		t.Fatalf("screen = %+v", reading.Screen)
	}
	if reading.Slots == nil || reading.Slots.SlotCount != 4 {
		t.Fatalf("slots = %+v, want 4", reading.Slots)
	}
	if reading.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", reading.Attempts)
	}
	// Rung 1 requests carry the schema and the prepared 1664px JPEG.
	if b.calls[0].Schema == nil || b.calls[0].Think {
		t.Fatalf("rung 1 request misconfigured: %+v", b.calls[0])
	}
	if got := longEdgeOf(t, b.calls[0].ImageJPEG); got != DefaultLongEdge {
		t.Fatalf("image long edge = %d, want %d", got, DefaultLongEdge)
	}
	if !strings.Contains(b.calls[0].Prompt, "Супермаркеты") {
		t.Fatal("catalog vocabulary not injected into the row prompt")
	}
	if strings.Contains(b.calls[1].Prompt, "Супермаркеты") {
		t.Fatal("slot prompt must not carry the vocabulary")
	}
}

func TestReadJSONInThinking(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		func(Request) (Response, error) { return Response{Thinking: "reading rows... " + menuJSON}, nil },
		ok(slotJSON),
	}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Screen.ScreenType != "menu" {
		t.Fatalf("screen = %+v", reading.Screen)
	}
}

func TestReadLadderEscalation(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		junk(), junk(), ok(menuJSON), // row pass climbs to rung 3
		ok(slotJSON),
	}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Attempts != 4 {
		t.Fatalf("attempts = %d, want 4", reading.Attempts)
	}
	r2, r3 := b.calls[1], b.calls[2]
	if r2.Schema == nil || !r2.Think || r2.NumPredict != 8192 || r2.NumCtx != 16384 {
		t.Fatalf("rung 2 = %+v, want schema+think+8192/16384", r2)
	}
	if r3.Schema != nil || r3.Think || !strings.Contains(r3.Prompt, "Respond with ONLY the JSON object") {
		t.Fatalf("rung 3 = %+v, want no schema + noReason suffix", r3)
	}
}

func TestReadFailsAfterFullLadder(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){junk(), junk(), junk()}}
	_, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("want ErrFailed, got %v", err)
	}
	if len(b.calls) != LadderRungs {
		t.Fatalf("calls = %d, want %d", len(b.calls), LadderRungs)
	}
}

func TestReadShrinksOnOOM(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		oom(), ok(menuJSON), // rung 1 OOMs → ladder restarts at 1024px
		ok(slotJSON),
	}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 1290, 2796), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := longEdgeOf(t, b.calls[0].ImageJPEG); got != DefaultLongEdge {
		t.Fatalf("first call long edge = %d, want %d", got, DefaultLongEdge)
	}
	if got := longEdgeOf(t, b.calls[1].ImageJPEG); got != RetryLongEdge {
		t.Fatalf("retry long edge = %d, want %d", got, RetryLongEdge)
	}
	// The slot pass keeps the shrunk image — the card already OOM'd once.
	if got := longEdgeOf(t, b.calls[2].ImageJPEG); got != RetryLongEdge {
		t.Fatalf("slot pass long edge = %d, want %d", got, RetryLongEdge)
	}
	if reading.Screen.ScreenType != "menu" {
		t.Fatalf("screen = %+v", reading.Screen)
	}
}

func TestReadSecondOOMAborts(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){oom(), oom()}}
	_, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 1290, 2796), nil)
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("want ErrFailed, got %v", err)
	}
	if len(b.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (no second shrink)", len(b.calls))
	}
}

func TestReadWheelResultSkipsSlotPass(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){ok(wheelJSON)}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Slots != nil || len(b.calls) != 1 {
		t.Fatalf("slots = %+v calls = %d, want nil/1 — asking a prize card invites a fabricated count", reading.Slots, len(b.calls))
	}
}

func TestReadSlotPassSoftFails(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){ok(menuJSON), junk(), junk(), junk()}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Slots != nil || len(reading.Screen.Rows) != 1 {
		t.Fatal("losing the slot count must not lose the rows")
	}
}

func TestReadSlotCountZeroMeansAbsent(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		ok(menuJSON), ok(`{"source_text":"","slot_count":0}`),
	}}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Slots != nil {
		t.Fatalf("slots = %+v, want nil for a stated 0", reading.Slots)
	}
}

func TestReadUnavailablePropagates(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		func(Request) (Response, error) { return Response{}, ErrUnavailable },
	}}
	_, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil)
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrFailed) {
		t.Fatalf("want bare ErrUnavailable, got %v", err)
	}
}

func TestReadBadImageBeforeAnyCall(t *testing.T) {
	b := &scriptedBackend{t: t}
	_, err := NewRecognizer(b).Read(context.Background(), []byte("%PDF-1.4"), nil)
	if !errors.Is(err, ErrBadImage) {
		t.Fatalf("want ErrBadImage, got %v", err)
	}
	if len(b.calls) != 0 {
		t.Fatal("undecodable image must not reach the backend")
	}
}

// The deadline derives from the ladder — this test is the tripwire the
// plan requires so the two cannot drift. The worst case is driven for
// real: (rungs−1) junk + OOM, restart with rungs−1 junk + success on the
// row pass, then a full junk ladder on the slot pass.
func TestRecognizerWorstCaseCallCount(t *testing.T) {
	var script []func(Request) (Response, error)
	for i := 0; i < LadderRungs-1; i++ {
		script = append(script, junk())
	}
	script = append(script, oom())
	for i := 0; i < LadderRungs-1; i++ {
		script = append(script, junk())
	}
	script = append(script, ok(menuJSON))
	for i := 0; i < LadderRungs; i++ {
		script = append(script, junk())
	}
	b := &scriptedBackend{t: t, script: script}
	reading, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 1290, 2796), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.calls) != MaxCallsPerImage {
		t.Fatalf("worst case made %d calls, MaxCallsPerImage says %d — the deadline derivation drifted", len(b.calls), MaxCallsPerImage)
	}
	if reading.Attempts != MaxCallsPerImage {
		t.Fatalf("attempts = %d, want %d", reading.Attempts, MaxCallsPerImage)
	}
	if MaxImageBudget() != time.Duration(MaxCallsPerImage)*PerCallTimeout {
		t.Fatal("MaxImageBudget must be MaxCallsPerImage × PerCallTimeout")
	}
}

// The progress report is what keeps a multi-minute image from looking
// hung, so its sequence is part of the contract: one report per backend
// call, naming the pass, the ladder rung, and whether the image was
// re-prepared smaller after an OOM.
func TestReadReportsProgress(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){
		junk(),       // rows, rung 1
		oom(),        // rows, rung 2 → shrink, ladder restarts
		ok(menuJSON), // rows, rung 1 at reduced resolution
		junk(),       // slots, rung 1 (still reduced)
		ok(slotJSON), // slots, rung 2
	}}
	var got []Progress
	rec := NewRecognizer(b)
	rec.OnProgress = func(p Progress) { got = append(got, p) }

	if _, err := rec.Read(context.Background(), encodePNG(t, 1290, 2796), nil); err != nil {
		t.Fatal(err)
	}
	want := []Progress{
		{Pass: PassRows, Attempt: 1, Reduced: false},
		{Pass: PassRows, Attempt: 2, Reduced: false},
		{Pass: PassRows, Attempt: 1, Reduced: true},
		{Pass: PassSlots, Attempt: 1, Reduced: true},
		{Pass: PassSlots, Attempt: 2, Reduced: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d reports, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("report %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(got) != len(b.calls) {
		t.Fatalf("%d reports for %d backend calls — must be one per call", len(got), len(b.calls))
	}
}

func TestReadWithoutProgressCallback(t *testing.T) {
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){ok(menuJSON), ok(slotJSON)}}
	if _, err := NewRecognizer(b).Read(context.Background(), encodePNG(t, 800, 600), nil); err != nil {
		t.Fatalf("a nil OnProgress must be fine: %v", err)
	}
}

func TestReadContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &scriptedBackend{t: t, script: []func(Request) (Response, error){ok(menuJSON)}}
	_, err := NewRecognizer(b).Read(ctx, encodePNG(t, 800, 600), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if len(b.calls) != 0 {
		t.Fatal("cancelled context must not reach the backend")
	}
}

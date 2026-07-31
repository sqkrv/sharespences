// Package vision is the VLM adapter for the screenshot recognizer
// (docs/specs/cashback-recognizer.md, private meta-repo). It is
// infrastructure only, and holds three invariants:
//
//   - It never dials at construction — the `openapi` CI gate runs the full
//     server build with a dead DSN and no network; backends connect lazily
//     on the first Complete call.
//   - It never decides anything: prompts, the retry ladder and image
//     preparation live here, but every extracted string is returned to the
//     caller verbatim (the model's «1%» stays «1%» — number normalisation
//     is the cashback domain's job, so the stored reading stays auditable).
//   - It imports nothing from the project. A leaf package by construction.
//
// The prompts, JSON schemas, the tolerant extractor and the 3-rung retry
// ladder are ports of the benchmarked eval harness (meta-repo
// docs/research/recognizer-eval/bench.py) — the ladder is the measured
// difference between 5/13 and 13/13 images on the same model weights.
package vision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Row is one extracted category row, all fields verbatim model output.
type Row struct {
	Percent string `json:"percent"`
	Title   string `json:"title"`
	// Cap is the «Кешбэк до N ₽» chip figure, empty when absent.
	Cap   string `json:"cap,omitempty"`
	State string `json:"state"`
	// Subtitle is the grey line under the title, verbatim. It carries the
	// row's condition as often as a description — «За хранение остатков»
	// against «Оплата топлива и сопутствующих товаров».
	Subtitle string `json:"subtitle,omitempty"`
	// Section is the heading this row sits under, verbatim. Empty when
	// the screen has no sections or the heading scrolled off. It is the
	// only thing separating a granted row from an offered one when both
	// carry the same title: ВТБ lists «3% АЗС» in the picker and «5% АЗС»
	// under «Уже действующая выгода».
	Section string `json:"section,omitempty"`
	// CatalogMatch is the known catalog title the model matched this row
	// to (the constrained-vocabulary lever), empty for new rows.
	CatalogMatch string `json:"catalog_match,omitempty"`
	// RowKind flags non-category rows the prompt asks to exclude but a
	// model may still emit: slot_modifier / mechanic / section_header.
	// Empty or "category" means an ordinary row (spec invariant 2).
	RowKind string `json:"row_kind,omitempty"`
}

// Screen is the row-pass result for one image.
type Screen struct {
	ScreenType string `json:"screen_type"` // menu | summary | wheel_result | not_relevant
	Bank       string `json:"bank"`        // never trusted — cross-check hint only
	PeriodText string `json:"period_text,omitempty"`
	Rows       []Row  `json:"rows"`
	// HasHeader / HasFooterButton feed the completeness warning (spec §7);
	// nil when the model omitted them.
	HasHeader       *bool `json:"has_header,omitempty"`
	HasFooterButton *bool `json:"has_footer_button,omitempty"`
}

// Slots is the focused second-pass slot-count reading. SourceText is not
// always a verbatim quote — use the number, never the quote, as evidence
// (benchmark run 2).
type Slots struct {
	SourceText string `json:"source_text"`
	SlotCount  int    `json:"slot_count"`
}

// Request is one backend completion call. Fields beyond Prompt/ImageJPEG
// are hints a backend may ignore where it has no equivalent.
type Request struct {
	Prompt    string
	ImageJPEG []byte          // prepared JPEG bytes; nil for text-only calls
	Schema    json.RawMessage // constrained-output JSON schema; nil = free-form
	Think     bool
	// NumPredict / NumCtx: 0 = backend default.
	NumPredict int
	NumCtx     int
}

// Response is the raw model output. Content and Thinking both feed the
// tolerant JSON extractor — thinking models sometimes emit the object in
// the thinking channel with empty content.
type Response struct {
	Content    string
	Thinking   string
	DoneReason string
}

// Backend is the pluggable seam (VISION_BACKEND=ollama|anthropic). The E2E
// test injects a fake here and still exercises the real ladder, prompts
// and extractor.
type Backend interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

var (
	// ErrUnavailable — the backend cannot be reached or is misconfigured
	// (down, no model, no key). Maps to 503; manual entry still works.
	ErrUnavailable = errors.New("vision: backend unavailable")
	// ErrFailed — the backend answered but never produced a usable
	// reading (no JSON after the full ladder, refusal, persistent OOM).
	ErrFailed = errors.New("vision: recognition failed")
	// ErrBadImage — this one image cannot be processed (undecodable
	// format like HEIC/PDF, or a decompression bomb). Callers skip the
	// image with a visible note rather than failing the job.
	ErrBadImage = errors.New("vision: image cannot be processed")
)

// BackendError is a non-transport failure from a backend HTTP call.
type BackendError struct {
	Backend string
	Status  int
	Detail  string
}

func (e *BackendError) Error() string {
	return fmt.Sprintf("vision: %s: HTTP %d: %s", e.Backend, e.Status, e.Detail)
}

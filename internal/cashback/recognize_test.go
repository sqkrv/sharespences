package cashback

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sqkrv/sharespences/internal/vision"
)

func TestRecognitionStoreBusyAndTTL(t *testing.T) {
	now := time.Now()
	s := recognitionStore{now: func() time.Time { return now }}
	user, other := uuid.New(), uuid.New()

	job, err := s.start(user, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.start(user, 1); !errors.Is(err, ErrRecognitionBusy) {
		t.Fatalf("second job for the same user: %v, want busy", err)
	}
	if _, err := s.start(other, 1); err != nil {
		t.Fatalf("another user's job must not be blocked: %v", err)
	}

	if _, ok := s.get(other, job.id); ok {
		t.Fatal("foreign job must read as absent")
	}
	dto, ok := s.get(user, job.id)
	if !ok || dto.Status != "running" || dto.Total != 3 {
		t.Fatalf("dto = %+v ok=%v", dto, ok)
	}

	// A finished job survives its TTL window, then evicts; a running one
	// is never evicted (its deadline bounds it).
	s.finish(job.id, &RecognitionDraftDTO{})
	now = now.Add(recognitionTTL - time.Minute)
	if _, ok := s.get(user, job.id); !ok {
		t.Fatal("finished job evicted before TTL")
	}
	if _, err := s.start(user, 1); err != nil {
		t.Fatalf("finished job must not count as busy: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, ok := s.get(user, job.id); ok {
		t.Fatal("finished job survived past TTL")
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixedBackend answers every completion with the same body — enough for
// runRecognition, whose ladder logic is covered in the vision package.
type fixedBackend struct{ body string }

func (b fixedBackend) Name() string { return "fixed" }
func (b fixedBackend) Complete(_ context.Context, req vision.Request) (vision.Response, error) {
	if strings.Contains(req.Prompt, "How many categories") {
		return vision.Response{Content: `{"source_text":"Выберите 4 категории","slot_count":4}`}, nil
	}
	return vision.Response{Content: b.body}, nil
}

func TestRunRecognitionDedupSkipAndDraft(t *testing.T) {
	imgA := pngBytes(t, 400, 300)
	imgB := pngBytes(t, 401, 300)
	idA, idB, idPDF := uuid.New(), uuid.New(), uuid.New()
	files := map[uuid.UUID][]byte{
		idA:   imgA,
		idB:   imgB,
		idPDF: []byte("%PDF-1.4 pretending"),
	}
	// Listed explicitly, never by ranging the map: Go randomises map
	// iteration, and the assertions below address images by position — the
	// undecodable one has to be third. Ranging made this test pass only when
	// the PDF happened to land there.
	ids := []uuid.UUID{idA, idB, idPDF}
	dupOf := ids[0]
	dup := uuid.New()
	files[dup] = files[dupOf] // same bytes → sha256 dedup
	ids = append(ids, dup)

	svc := &Service{
		Vision: fixedBackend{body: `{"screen_type":"menu","bank":"Альфа-Банк","rows":[{"percent":"5","title":"Такси","state":"unchecked"}]}`},
		ReadAttachmentFile: func(id uuid.UUID) (io.ReadCloser, error) {
			raw, ok := files[id]
			if !ok {
				return nil, errors.New("missing")
			}
			return io.NopCloser(bytes.NewReader(raw)), nil
		},
	}
	job, err := svc.recognitions.start(uuid.New(), len(ids))
	if err != nil {
		t.Fatal(err)
	}
	svc.runRecognition(job.id, ids, nil, nil, nil, "Альфа-Банк", nil)

	dto, ok := svc.recognitions.get(job.userID, job.id)
	if !ok || dto.Status != "done" || dto.Draft == nil {
		t.Fatalf("job = %+v ok=%v, want done with draft", dto, ok)
	}
	if dto.Done != dto.Total {
		t.Fatalf("done = %d, total = %d", dto.Done, dto.Total)
	}
	imgs := dto.Draft.Images
	if len(imgs) != 4 {
		t.Fatalf("images = %+v", imgs)
	}
	if imgs[2].Skipped == false || !strings.Contains(imgs[2].Note, "не удалось декодировать") {
		t.Fatalf("undecodable image: %+v", imgs[2])
	}
	if imgs[3].Skipped == false || !strings.Contains(imgs[3].Note, "дубликат скриншота 1") {
		t.Fatalf("duplicate image: %+v", imgs[3])
	}
	// Both decodable menus read the same rows — merged to one.
	if len(dto.Draft.Rows) != 1 || dto.Draft.Rows[0].RawTitle != "Такси" {
		t.Fatalf("rows = %+v", dto.Draft.Rows)
	}
	if dto.Draft.SlotCount == nil || *dto.Draft.SlotCount != 4 {
		t.Fatalf("slot = %v, want 4", dto.Draft.SlotCount)
	}
}

// phaseBackend records the job's own DTO at the moment the backend is
// called — that is the only way to prove the phase is visible to a poll
// WHILE an image is in flight, which is the whole point of the field.
type phaseBackend struct {
	svc   *Service
	jobID uuid.UUID
	user  uuid.UUID
	seen  []RecognitionJobDTO
}

func (b *phaseBackend) Name() string { return "phase" }
func (b *phaseBackend) Complete(_ context.Context, req vision.Request) (vision.Response, error) {
	dto, _ := b.svc.recognitions.get(b.user, b.jobID)
	b.seen = append(b.seen, dto)
	if strings.Contains(req.Prompt, "How many categories") {
		return vision.Response{Content: `{"source_text":"Выберите 4 категории","slot_count":4}`}, nil
	}
	return vision.Response{Content: `{"screen_type":"menu","bank":"Ozon Банк","rows":[{"percent":"5","title":"Такси","state":"unchecked"}]}`}, nil
}

func TestRunRecognitionReportsPhase(t *testing.T) {
	user := uuid.New()
	// Distinct bytes per attachment, or the sha256 dedup skips the second
	// screenshot and there is no second image to report on.
	first, second := uuid.New(), uuid.New()
	files := map[uuid.UUID][]byte{first: pngBytes(t, 300, 200), second: pngBytes(t, 301, 200)}
	svc := &Service{ReadAttachmentFile: func(id uuid.UUID) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(files[id])), nil
	}}
	job, err := svc.recognitions.start(user, 2)
	if err != nil {
		t.Fatal(err)
	}
	b := &phaseBackend{svc: svc, jobID: job.id, user: user}
	svc.Vision = b
	svc.runRecognition(job.id, []uuid.UUID{first, second}, nil, nil, nil, "Ozon Банк", nil)

	if len(b.seen) != 4 { // two images × (rows + slots)
		t.Fatalf("backend called %d times, want 4", len(b.seen))
	}
	want := []struct {
		image int
		pass  string
		done  int
	}{
		{1, vision.PassRows, 0},
		{1, vision.PassSlots, 0},
		{2, vision.PassRows, 1},
		{2, vision.PassSlots, 1},
	}
	for i, w := range want {
		got := b.seen[i]
		if got.Image != w.image || got.Pass != w.pass || got.Attempt != 1 || got.Done != w.done || got.Total != 2 {
			t.Errorf("during call %d: image=%d pass=%q attempt=%d done=%d/%d, want image=%d pass=%q attempt=1 done=%d/2",
				i+1, got.Image, got.Pass, got.Attempt, got.Done, got.Total, w.image, w.pass, w.done)
		}
		if got.Status != "running" {
			t.Errorf("during call %d: status = %q, want running", i+1, got.Status)
		}
	}
	// A finished job reports nothing in flight.
	final, _ := svc.recognitions.get(user, job.id)
	if final.Status != "done" || final.Image != 0 || final.Pass != "" || final.Done != final.Total {
		t.Fatalf("finished job = %+v, want done, 2/2, no phase", final)
	}
}

type failingBackend struct{}

func (failingBackend) Name() string { return "failing" }
func (failingBackend) Complete(context.Context, vision.Request) (vision.Response, error) {
	return vision.Response{}, vision.ErrUnavailable
}

func TestRunRecognitionFailsJobOnBackendError(t *testing.T) {
	id := uuid.New()
	svc := &Service{
		Vision: failingBackend{},
		ReadAttachmentFile: func(uuid.UUID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(pngBytes(t, 100, 100))), nil
		},
	}
	job, err := svc.recognitions.start(uuid.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	svc.runRecognition(job.id, []uuid.UUID{id}, nil, nil, nil, "", nil)
	dto, _ := svc.recognitions.get(job.userID, job.id)
	if dto.Status != "failed" || dto.Error == "" {
		t.Fatalf("job = %+v, want failed with message", dto)
	}
}

type panickingBackend struct{}

func (panickingBackend) Name() string { return "panicking" }
func (panickingBackend) Complete(context.Context, vision.Request) (vision.Response, error) {
	panic("decoder blew up on user-supplied bytes")
}

// runRecognition is the only goroutine the app starts per request, and chi's
// Recoverer does not reach it: without the deferred recover, a panic anywhere
// in the decode/model/merge path takes the process down for every user.
func TestRunRecognitionSurvivesPanic(t *testing.T) {
	svc := &Service{
		Vision: panickingBackend{},
		ReadAttachmentFile: func(uuid.UUID) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(pngBytes(t, 100, 100))), nil
		},
	}
	job, err := svc.recognitions.start(uuid.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	svc.runRecognition(job.id, []uuid.UUID{uuid.New()}, nil, nil, nil, "", nil)

	dto, _ := svc.recognitions.get(job.userID, job.id)
	if dto.Status != "failed" {
		t.Fatalf("job status = %q, want failed", dto.Status)
	}
	// The panic value must stay in the log — the user reads a Russian sentence.
	if dto.Error == "" || strings.Contains(dto.Error, "decoder blew up") {
		t.Fatalf("job error = %q, want a Russian message that does not leak the panic", dto.Error)
	}
}

func TestStartRecognitionRequiresBackend(t *testing.T) {
	svc := &Service{} // Vision nil = feature off
	_, err := svc.StartRecognition(context.Background(), uuid.New(), 1, []uuid.UUID{uuid.New()})
	if !errors.Is(err, vision.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

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
	files := map[uuid.UUID][]byte{
		uuid.New(): imgA,
		uuid.New(): imgB,
		uuid.New(): []byte("%PDF-1.4 pretending"),
	}
	var ids []uuid.UUID
	for id := range files {
		ids = append(ids, id)
	}
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
	svc.runRecognition(job.id, ids, nil, nil, nil, "Альфа-Банк")

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
	svc.runRecognition(job.id, []uuid.UUID{id}, nil, nil, nil, "")
	dto, _ := svc.recognitions.get(job.userID, job.id)
	if dto.Status != "failed" || dto.Error == "" {
		t.Fatalf("job = %+v, want failed with message", dto)
	}
}

func TestStartRecognitionRequiresBackend(t *testing.T) {
	svc := &Service{} // Vision nil = feature off
	_, err := svc.StartRecognition(context.Background(), uuid.New(), 1, []uuid.UUID{uuid.New()})
	if !errors.Is(err, vision.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

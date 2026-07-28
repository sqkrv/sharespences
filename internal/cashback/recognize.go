// Screenshot-recognizer orchestration: the in-memory job store and the
// background run that turns uploaded attachments into a prefill draft.
// v1 adds NO write path — the job only produces the payloads the four
// existing endpoints already accept, and the SPA replays them on commit.
package cashback

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/vision"
)

const (
	// maxRecognitionImages bounds one job (owner decision 2026-07-28).
	// Альфа routinely needs 3–4 screenshots per period; 10 is headroom.
	maxRecognitionImages = 10
	// recognitionTTL keeps a FINISHED job around for the review screen.
	// Running jobs are never evicted — their deadline bounds them.
	recognitionTTL = 30 * time.Minute
)

var (
	// ErrRecognitionBusy — one running job per user (owner decision):
	// the backend is a single GPU and jobs run minutes.
	ErrRecognitionBusy = errors.New("cashback: распознавание уже идёт — дождись окончания")
	// ErrRecognitionImages — the 1–10 screenshot bound.
	ErrRecognitionImages = errors.New("cashback: recognition takes 1–10 screenshots")
)

// RecognitionJobDTO is the poll response. Draft appears only on done.
type RecognitionJobDTO struct {
	ID     uuid.UUID            `json:"id"`
	Status string               `json:"status" enum:"running,done,failed"`
	Done   int                  `json:"done" doc:"screenshots processed so far"`
	Total  int                  `json:"total"`
	Error  string               `json:"error,omitempty"`
	Draft  *RecognitionDraftDTO `json:"draft,omitempty"`
}

type recognitionJob struct {
	id        uuid.UUID
	userID    uuid.UUID
	status    string
	done      int
	total     int
	err       string
	draft     *RecognitionDraftDTO
	updatedAt time.Time
}

// recognitionStore is the first mutable server state outside Postgres:
// in-memory, mutex-guarded, single-replica only. State is LOST on
// restart by design — a job is an ephemeral draft the user re-runs, and
// srv.Shutdown's 10 s grace could never outwait ~149 s/image anyway
// (documented behaviour, not an accident).
type recognitionStore struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]*recognitionJob
	now  func() time.Time // test seam; nil = time.Now
}

func (s *recognitionStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// evictLocked drops terminal jobs past their TTL. Callers hold mu.
func (s *recognitionStore) evictLocked() {
	cutoff := s.clock().Add(-recognitionTTL)
	for id, j := range s.jobs {
		if j.status != "running" && j.updatedAt.Before(cutoff) {
			delete(s.jobs, id)
		}
	}
}

func (s *recognitionStore) start(userID uuid.UUID, total int) (*recognitionJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[uuid.UUID]*recognitionJob{}
	}
	s.evictLocked()
	for _, j := range s.jobs {
		if j.userID == userID && j.status == "running" {
			return nil, ErrRecognitionBusy
		}
	}
	j := &recognitionJob{id: uuid.New(), userID: userID, status: "running", total: total, updatedAt: s.clock()}
	s.jobs[j.id] = j
	return j, nil
}

func (s *recognitionStore) get(userID, id uuid.UUID) (RecognitionJobDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
	j, ok := s.jobs[id]
	if !ok || j.userID != userID { // scoping never reveals which
		return RecognitionJobDTO{}, false
	}
	return RecognitionJobDTO{ID: j.id, Status: j.status, Done: j.done, Total: j.total, Error: j.err, Draft: j.draft}, true
}

func (s *recognitionStore) progress(id uuid.UUID, done int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.done, j.updatedAt = done, s.clock()
	}
}

func (s *recognitionStore) finish(id uuid.UUID, draft *RecognitionDraftDTO) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.status, j.draft, j.done, j.updatedAt = "done", draft, j.total, s.clock()
	}
}

func (s *recognitionStore) fail(id uuid.UUID, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.status, j.err, j.updatedAt = "failed", msg, s.clock()
	}
}

// StartRecognition validates ownership, snapshots the bank's catalog and
// aliases, and spawns the background run. All DB reads happen here in
// the request context — the goroutine touches only the disk store and
// the vision backend.
func (s *Service) StartRecognition(ctx context.Context, userID uuid.UUID, bankClientID int64, attachmentIDs []uuid.UUID) (RecognitionJobDTO, error) {
	if s.Vision == nil || s.ReadAttachmentFile == nil {
		return RecognitionJobDTO{}, vision.ErrUnavailable
	}
	if len(attachmentIDs) < 1 || len(attachmentIDs) > maxRecognitionImages {
		return RecognitionJobDTO{}, ErrRecognitionImages
	}
	client, err := s.Q.GetBankClientForUser(ctx, db.GetBankClientForUserParams{ID: bankClientID, UserID: userID})
	if err != nil {
		return RecognitionJobDTO{}, notFound(err)
	}
	for _, id := range attachmentIDs {
		if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: id, UserID: &userID}); err != nil {
			return RecognitionJobDTO{}, notFound(err)
		}
	}
	catalogRows, err := s.Q.ListBankCategories(ctx, client.BankID)
	if err != nil {
		return RecognitionJobDTO{}, err
	}
	catalog := make([]CatalogRow, 0, len(catalogRows))
	titles := make([]string, 0, len(catalogRows))
	for _, r := range catalogRows {
		catalog = append(catalog, CatalogRow{ID: r.ID, Title: r.Title, CanonicalCategoryID: r.CanonicalCategoryID, Kind: OfferKind(r.Kind)})
		titles = append(titles, r.Title)
	}
	aliasRows, err := s.Q.ListAliasesForBank(ctx, client.BankID)
	if err != nil {
		return RecognitionJobDTO{}, err
	}
	aliases := make([]Alias, 0, len(aliasRows))
	for _, a := range aliasRows {
		aliases = append(aliases, Alias{CanonicalCategoryID: a.CanonicalCategoryID, RawTitle: a.RawTitle})
	}

	job, err := s.recognitions.start(userID, len(attachmentIDs))
	if err != nil {
		return RecognitionJobDTO{}, err
	}
	go s.runRecognition(job.id, attachmentIDs, titles, catalog, aliases, client.BankName)
	return RecognitionJobDTO{ID: job.id, Status: job.status, Total: job.total}, nil
}

// GetRecognition returns the job for polling; a foreign or expired job
// is a plain 404.
func (s *Service) GetRecognition(userID, id uuid.UUID) (RecognitionJobDTO, error) {
	dto, ok := s.recognitions.get(userID, id)
	if !ok {
		return RecognitionJobDTO{}, ErrNotFound
	}
	return dto, nil
}

// runRecognition is the background job body. Its deadline derives from
// the ladder (images × vision.MaxImageBudget) — never a hand-picked
// number. Any vision failure past per-image skips fails the whole job:
// a half-read batch would prefill a misleading menu.
func (s *Service) runRecognition(jobID uuid.UUID, attachmentIDs []uuid.UUID, titles []string, catalog []CatalogRow, aliases []Alias, bankName string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(attachmentIDs))*vision.MaxImageBudget())
	defer cancel()
	rec := vision.NewRecognizer(s.Vision)

	// Dedup is sha256 of the raw upload bytes: it catches the real
	// in-job case (the same file picked twice). The corpus «twins» are
	// re-exports with different bytes — those cost one redundant
	// inference and then collapse in the title merge, which is fine.
	seen := map[[sha256.Size]byte]int{}
	images := make([]RecognizedImage, 0, len(attachmentIDs))
	for i, attID := range attachmentIDs {
		ri := RecognizedImage{AttachmentID: attID}
		raw, err := s.readAttachment(attID)
		switch {
		case err != nil:
			ri.SkipNote = "файл вложения не найден"
		default:
			sum := sha256.Sum256(raw)
			if first, dup := seen[sum]; dup {
				ri.SkipNote = fmt.Sprintf("дубликат скриншота %d", first)
			} else {
				seen[sum] = i + 1
				reading, err := rec.Read(ctx, raw, titles)
				switch {
				case err == nil:
					ri.Reading = &reading
				case errors.Is(err, vision.ErrBadImage):
					ri.SkipNote = "не удалось декодировать изображение (HEIC и PDF не распознаются)"
				default:
					s.recognitions.fail(jobID, err.Error())
					return
				}
			}
		}
		images = append(images, ri)
		s.recognitions.progress(jobID, i+1)
	}
	draft := BuildDraft(images, catalog, aliases, bankName)
	s.recognitions.finish(jobID, &draft)
}

func (s *Service) readAttachment(id uuid.UUID) ([]byte, error) {
	f, err := s.ReadAttachmentFile(id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

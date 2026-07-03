// Package attach is the local-disk attachment store (screenshot evidence
// for offer periods and partner offers). Files land in Dir named by the
// attachment UUID; metadata lives in the attachment table, scoped by user.
package attach

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/sqkrv/sharespences/internal/db"
)

type Store struct {
	Q   *db.Queries
	Dir string
}

// Save streams an uploaded file to disk and records it for the user.
func (s *Store) Save(ctx context.Context, userID uuid.UUID, filename, mediaType string, r io.Reader) (db.Attachment, error) {
	id := uuid.New()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return db.Attachment{}, err
	}
	f, err := os.Create(s.Path(id))
	if err != nil {
		return db.Attachment{}, err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(s.Path(id))
		return db.Attachment{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(s.Path(id))
		return db.Attachment{}, err
	}
	var mt *string
	if mediaType != "" {
		mt = &mediaType
	}
	a, err := s.Q.CreateAttachment(ctx, db.CreateAttachmentParams{
		ID:        id,
		Filename:  filename,
		MediaType: mt,
		UserID:    &userID,
	})
	if err != nil {
		_ = os.Remove(s.Path(id))
		return db.Attachment{}, err
	}
	return a, nil
}

// Get returns the metadata row if it belongs to the user.
func (s *Store) Get(ctx context.Context, userID, id uuid.UUID) (db.Attachment, error) {
	return s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: id, UserID: &userID})
}

// Path is where the attachment's bytes live on disk.
func (s *Store) Path(id uuid.UUID) string {
	return filepath.Join(s.Dir, fmt.Sprintf("%s.bin", id))
}

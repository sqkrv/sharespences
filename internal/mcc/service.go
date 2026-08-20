package mcc

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
)

// Service is the MCC module's Go API. Reference reads of bank /
// bank_category / canonical_category are the documented seam (queries/
// mcc.sql header); no other module touches the MCC tables.
type Service struct {
	Q *db.Queries
	// RoleOf is injected at the composition root (the auth module owns the
	// "user" table — same seam practice as friends→cashback): the caller's
	// CURRENT role, read per request, gates moderation.
	RoleOf func(ctx context.Context, userID uuid.UUID) (auth.Role, error)
}

var numericRe = regexp.MustCompile(`^[0-9]+$`)

// Search finds dictionary codes by code prefix (numeric query, zero-padded
// comparison) or name substring (anything else).
func (s *Service) Search(ctx context.Context, query string, limit int32) ([]db.Mcc, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	return s.Q.SearchMCC(ctx, db.SearchMCCParams{
		IsNumeric: numericRe.MatchString(query),
		Query:     query,
		MaxRows:   limit,
	})
}

// Resolve returns the dictionary entry and every bank's active catalog
// category containing the code. A known code with no memberships is a valid
// answer (empty banks) — the base simply doesn't cover it yet.
func (s *Service) Resolve(ctx context.Context, code int16) (db.Mcc, []db.ResolveMCCRow, error) {
	entry, err := s.Q.GetMCC(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Mcc{}, nil, ErrNotFound
		}
		return db.Mcc{}, nil, err
	}
	rows, err := s.Q.ResolveMCC(ctx, code)
	if err != nil {
		return db.Mcc{}, nil, err
	}
	return entry, rows, nil
}

// SearchMerchants finds points of sale (the imported base + approved user
// submissions; the caller's own pending ones too) by name or merchant-title
// substring, most-confirmed first.
func (s *Service) SearchMerchants(ctx context.Context, userID uuid.UUID, query string, limit int32) ([]db.SearchMerchantsRow, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	return s.Q.SearchMerchants(ctx, db.SearchMerchantsParams{UserID: userID, Query: query, MaxRows: limit})
}

// SimilarPoints is the 5e duplicate net: same MCC, either name contains the
// other — shown on the form so an existing точка is opened, not copied.
func (s *Service) SimilarPoints(ctx context.Context, userID uuid.UUID, mccCode int16, name string) ([]db.FindSimilarPointsOfSaleRow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	return s.Q.FindSimilarPointsOfSale(ctx, db.FindSimilarPointsOfSaleParams{
		MccCode: &mccCode, UserID: userID, Name: name,
	})
}

// CreatePoint files a user-submitted точка продаж: pending until moderated,
// visible to its author immediately. The MCC must exist in the dictionary —
// the FK would refuse anyway, this check just answers in Russian.
func (s *Service) CreatePoint(ctx context.Context, userID uuid.UUID, p db.CreateUserPointOfSaleParams) (db.PointOfSale, error) {
	if p.MccCode == nil {
		return db.PointOfSale{}, ErrNotFound
	}
	if _, err := s.Q.GetMCC(ctx, *p.MccCode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.PointOfSale{}, ErrNotFound
		}
		return db.PointOfSale{}, err
	}
	p.AuthorUserID = &userID
	return s.Q.CreateUserPointOfSale(ctx, p)
}

// Changes returns the newest journal rows (news-digest precursor).
func (s *Service) Changes(ctx context.Context, limit int32) ([]db.ListMCCChangesRow, error) {
	return s.Q.ListMCCChanges(ctx, limit)
}

// requireModerator gates the moderation surface. Every caller gets the
// same answer on refusal — no existence leaks (roles-moderation inv. 6).
func (s *Service) requireModerator(ctx context.Context, userID uuid.UUID) error {
	role, err := s.RoleOf(ctx, userID)
	if err != nil {
		return err
	}
	if !role.CanModerate() {
		return ErrNotModerator
	}
	return nil
}

// ModerationPending lists the queue oldest-first (fairness: the longest
// waiting submission is reviewed first).
func (s *Service) ModerationPending(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ModerationListPendingPOSRow, error) {
	if err := s.requireModerator(ctx, userID); err != nil {
		return nil, err
	}
	return s.Q.ModerationListPendingPOS(ctx, db.ModerationListPendingPOSParams{MaxRows: limit, Skip: offset})
}

// ModerationPublished is the review stream: recently published non-scrape
// rows — what keeps the instant-publish path supervised after the fact.
func (s *Service) ModerationPublished(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]db.ModerationListPublishedPOSRow, error) {
	if err := s.requireModerator(ctx, userID); err != nil {
		return nil, err
	}
	return s.Q.ModerationListPublishedPOS(ctx, db.ModerationListPublishedPOSParams{MaxRows: limit, Skip: offset})
}

// ModerationApprove publishes a pending submission into the общий каталог.
func (s *Service) ModerationApprove(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	if err := s.requireModerator(ctx, userID); err != nil {
		return err
	}
	n, err := s.Q.ModerationApprovePOS(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ModerationReject pulls a pending or published non-scrape row out of
// circulation; the row is kept for audit and stays invisible to everyone,
// its author included.
func (s *Service) ModerationReject(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	if err := s.requireModerator(ctx, userID); err != nil {
		return err
	}
	n, err := s.Q.ModerationRejectPOS(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

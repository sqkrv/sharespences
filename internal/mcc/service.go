package mcc

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sqkrv/sharespences/internal/db"
)

// Service is the MCC module's Go API. Reference reads of bank /
// bank_category / canonical_category are the documented seam (queries/
// mcc.sql header); no other module touches the MCC tables.
type Service struct {
	Q *db.Queries
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
func (s *Service) SearchMerchants(ctx context.Context, userID uuid.UUID, query string, limit, offset int32) ([]db.SearchMerchantsRow, int64, error) {
	patterns := SearchPatterns(query)
	if len(patterns) == 0 {
		return nil, 0, nil
	}
	rows, err := s.Q.SearchMerchants(ctx, db.SearchMerchantsParams{
		UserID: userID, Head: patterns[0], Patterns: patterns, MaxRows: limit, SkipRows: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	// The window count is per row; an empty page carries no count, and past
	// the last page that is the honest answer for «how many are left».
	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalRows
	}
	return rows, total, nil
}

// SearchPatterns turns a raw query into the ILIKE patterns the search matches
// with: one per word, so word order stops mattering. LIKE wildcards typed by
// the user are escaped — «100%» is a merchant name, not a pattern.
func SearchPatterns(query string) []string {
	fields := strings.Fields(query)
	patterns := make([]string, 0, len(fields))
	for _, w := range fields {
		w = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(w)
		patterns = append(patterns, "%"+w+"%")
	}
	return patterns
}

// Point serves the «О точке» card (8b): one row by id, under the search's
// visibility rule — approved, or the caller's own pending submission.
func (s *Service) Point(ctx context.Context, userID uuid.UUID, id uuid.UUID) (db.GetPointOfSaleRow, error) {
	row, err := s.Q.GetPointOfSale(ctx, db.GetPointOfSaleParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetPointOfSaleRow{}, ErrNotFound
	}
	return row, err
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

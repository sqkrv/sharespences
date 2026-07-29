package mcc

import (
	"context"
	"errors"
	"regexp"
	"strings"

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

// SearchMerchants finds points of sale (the imported mcc-codes.ru base) by
// name or merchant-title substring, most-confirmed first.
func (s *Service) SearchMerchants(ctx context.Context, query string, limit int32) ([]db.SearchMerchantsRow, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	return s.Q.SearchMerchants(ctx, db.SearchMerchantsParams{Query: query, MaxRows: limit})
}

// Changes returns the newest journal rows (news-digest precursor).
func (s *Service) Changes(ctx context.Context, limit int32) ([]db.ListMCCChangesRow, error) {
	return s.Q.ListMCCChanges(ctx, limit)
}

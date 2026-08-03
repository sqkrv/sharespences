// Package admin is the operator sidecar's API (ADR-0008): catalog browsing
// for everything, writes only for what seed does not own, and a system
// dashboard. It spans module tables the way seed and import-pos do —
// operator tooling, not an app module; nothing here is part of the public
// /api/v1 contract.
package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
)

// ErrSeedManaged rejects a write to a row the seed refreshes on every
// deploy — the edit would be silently reverted (and, for category↔MCC
// links, journalled as a false ± event). Durable edits to seed-owned data
// go through the knowledge-base → seed pipeline.
var ErrSeedManaged = errors.New("строка управляется сидом — правки перезапишутся при деплое")

// Service owns the sidecar's DB access. The coverage sets come from
// seed.SeededMCCCodes / seed.SeededMembershipKeys at the composition root.
type Service struct {
	Q    *db.Queries
	Pool *pgxpool.Pool
	// SeededMCC: dictionary codes the seed unconditionally upserts.
	SeededMCC map[int16]bool
	// SeededMembership: (bank name, category title) pairs whose link sets
	// the seed refreshes by diff.
	SeededMembership map[[2]string]bool
}

// TableCount is one pg_stat_user_tables estimate — good enough for a
// dashboard, and it covers the legacy schema-only tables no query touches.
type TableCount struct {
	Table string
	Rows  int64
}

type DashboardData struct {
	Counts      db.AdminCountsRow
	DBSizeBytes int64
	Tables      []TableCount
	Migrations  []migrations.MigrationStatus
}

func (s *Service) Dashboard(ctx context.Context) (DashboardData, error) {
	var d DashboardData
	var err error
	if d.Counts, err = s.Q.AdminCounts(ctx); err != nil {
		return d, fmt.Errorf("dashboard counts: %w", err)
	}
	if err = s.Pool.QueryRow(ctx,
		`select pg_database_size(current_database())`).Scan(&d.DBSizeBytes); err != nil {
		return d, fmt.Errorf("dashboard db size: %w", err)
	}
	rows, err := s.Pool.Query(ctx,
		`select relname, n_live_tup from pg_stat_user_tables order by relname`)
	if err != nil {
		return d, fmt.Errorf("dashboard tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t TableCount
		if err := rows.Scan(&t.Table, &t.Rows); err != nil {
			return d, fmt.Errorf("dashboard tables scan: %w", err)
		}
		d.Tables = append(d.Tables, t)
	}
	if err := rows.Err(); err != nil {
		return d, fmt.Errorf("dashboard tables rows: %w", err)
	}
	if d.Migrations, err = migrations.Status(ctx, s.Pool); err != nil {
		return d, fmt.Errorf("dashboard migrations: %w", err)
	}
	return d, nil
}

// guardMCC rejects writes to dictionary codes the seed owns.
func (s *Service) guardMCC(code int16) error {
	if s.SeededMCC[code] {
		return ErrSeedManaged
	}
	return nil
}

// guardMembership rejects link writes under a category whose link set the
// seed refreshes. The category is identified by (bank name, title) — the
// same key the seed CSV uses.
func (s *Service) guardMembership(ctx context.Context, bankCategoryID int64) (db.AdminGetBankCategoryWithBankRow, error) {
	bc, err := s.Q.AdminGetBankCategoryWithBank(ctx, bankCategoryID)
	if err != nil {
		return bc, err // pgx.ErrNoRows → 404 at the HTTP layer
	}
	if s.SeededMembership[[2]string{bc.BankName, bc.Title}] {
		return bc, ErrSeedManaged
	}
	return bc, nil
}

// guardBankCategory distinguishes «not found» from «seed-managed» for a
// bank_category write (the SQL itself also carries `and is_custom`).
func (s *Service) guardBankCategory(ctx context.Context, id int64) error {
	bc, err := s.Q.AdminGetBankCategoryWithBank(ctx, id)
	if err != nil {
		return err
	}
	if !bc.IsCustom {
		return ErrSeedManaged
	}
	return nil
}

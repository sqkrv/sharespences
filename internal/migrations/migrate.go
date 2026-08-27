package migrations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Up applies all pending migrations using the given pool, including ones
// numbered below the database's current version.
//
// Out-of-order is not an accident here, it is what ADR-0007 asks for: work
// happens on short branches off main, so two branches routinely take the same
// next number, and whichever merges second carries a version the deployed
// database has already passed. That is exactly what happened to 00023
// (foreign-key indexes, branched before 00024 existed, merged after 00028 was
// live): goose's default refused the whole run, and the deploy stopped with
// «detected 1 missing (out-of-order) migration lower than database version».
//
// The rule this trades for is on the author, not the runner: a migration must
// not depend on having run before a higher-numbered one. That is already true
// of every migration this project writes — they add tables, columns and
// indexes rather than transform each other's output — and it is the same
// property that lets two branches migrate independently in the first place.
// A migration that genuinely needs an ordering has to say so by depending on
// state it can check, because the number no longer guarantees it.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS,
		goose.WithAllowOutofOrder(true))
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

// MigrationStatus is one embedded migration's state against the database.
type MigrationStatus struct {
	Version   int64
	Source    string
	AppliedAt *time.Time // nil = pending
}

// Status reports every embedded migration and whether it is applied —
// the admin dashboard's «is this DB current?» answer.
func Status(ctx context.Context, pool *pgxpool.Pool) ([]MigrationStatus, error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		return nil, err
	}
	rows, err := provider.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, len(rows))
	for i, r := range rows {
		out[i] = MigrationStatus{Version: r.Source.Version, Source: r.Source.Path}
		if r.State == goose.StateApplied {
			t := r.AppliedAt
			out[i].AppliedAt = &t
		}
	}
	return out, nil
}

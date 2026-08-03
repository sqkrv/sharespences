package migrations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Up applies all pending migrations using the given pool.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
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

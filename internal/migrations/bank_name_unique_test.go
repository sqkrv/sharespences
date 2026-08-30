// Migration test for 00031 (unique bank.name). Two paths matter and neither is
// reachable on a fresh database: that the constraint actually rejects a second
// row with the same name, and that a database which already holds duplicates is
// refused with a message an operator can act on rather than being half-migrated.
package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newPG(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	pg, err := postgres.Run(ctx, "postgis/postgis:18-3.6",
		postgres.WithDatabase("sharespences"),
		postgres.WithUsername("sharespences"),
		postgres.WithPassword("sharespences"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("no Docker: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pg) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestUp00031RejectsDuplicateBankName(t *testing.T) {
	ctx := context.Background()
	pool := newPG(ctx, t)

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into bank (name) values ('УникБанк')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into bank (name) values ('УникБанк')`); err == nil {
		t.Fatal("a second bank with the same name was accepted; the constraint is not doing its job")
	}
}

func TestUp00031RefusesPreExistingDuplicates(t *testing.T) {
	ctx := context.Background()
	pool := newPG(ctx, t)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		t.Fatal(err)
	}
	// Stop before the constraint: a database that already holds duplicates.
	// (00024–00029 live on another branch; the version number still reads as
	// «everything before this migration» whether or not they are present.)
	if _, err := provider.UpTo(ctx, 30); err != nil {
		t.Fatalf("up to 29: %v", err)
	}
	for range 2 {
		if _, err := pool.Exec(ctx, `insert into bank (name) values ('ДублиБанк')`); err != nil {
			t.Fatal(err)
		}
	}

	_, err = provider.UpTo(ctx, 31)
	if err == nil {
		t.Fatal("00031 applied over duplicate names; it must refuse instead")
	}
	// Specifically the deliberate guard (P0001 raise_exception), not Postgres's
	// own 23505 from the ALTER — that one also happens to quote the name, so
	// asserting on the name alone would pass either way and prove nothing about
	// the pre-check running before anything is touched.
	if !strings.Contains(err.Error(), "has duplicates, cannot make it unique") {
		t.Errorf("failed on something other than the pre-check guard: %v", err)
	}
	if !strings.Contains(err.Error(), "ДублиБанк") {
		t.Errorf("error does not name the duplicate: %v", err)
	}

	// And it must have changed nothing — the constraint is not half-applied.
	var n int
	if err := pool.QueryRow(ctx, `
		select count(*) from pg_constraint where conname = 'bank_name_key'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("bank_name_key exists despite the migration failing")
	}
	if err := pool.QueryRow(ctx,
		`select count(*) from bank where name = 'ДублиБанк'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("duplicate rows were altered (%d left); the migration must not modify data", n)
	}
}

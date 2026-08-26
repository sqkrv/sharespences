// Migration test for 00029 (Альфа-Смарт S/M split), covering the path a
// fresh-database run never reaches: an existing database that already holds the
// pre-split «Альфа-Смарт» tier with clients attached to it. On a fresh database
// 00029 renames nothing — the seed simply inserts S and M — so only this test
// exercises what the migration is actually for: without it those clients keep
// pointing at a tier whose name no longer appears in the seed, so nothing ever
// refreshes it again.
package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestUp00029RenamesInPlace(t *testing.T) {
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgis/postgis:18-3.6",
		postgres.WithDatabase("sharespences"),
		postgres.WithUsername("sharespences"),
		postgres.WithPassword("sharespences"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("no Docker: %v", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(pg) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		t.Fatal(err)
	}
	// Stop one short of the split: this is the world as it exists in production.
	if _, err := provider.UpTo(ctx, 28); err != nil {
		t.Fatalf("up to 28: %v", err)
	}

	var bankID int32
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('Альфа-Банк') returning id`).Scan(&bankID); err != nil {
		t.Fatal(err)
	}
	var progID int64
	if err := pool.QueryRow(ctx, `
		insert into cashback_program (bank_id, name, period_type, selection_mode, currency_kind)
		values ($1, 'Кэшбэк', 'calendar_month', 'atomic', 'rub')
		returning id`, bankID).Scan(&progID); err != nil {
		t.Fatal(err)
	}
	var tierID int64
	if err := pool.QueryRow(ctx, `
		insert into program_tier (program_id, name, is_paid_subscription, cap_value, cap_scope, max_categories)
		values ($1, 'Альфа-Смарт', true, 7000, 'total', 4)
		returning id`, progID).Scan(&tierID); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		insert into "user" (username, email, display_name, password_hash)
		values ('smarttester', 'smart@example.com', 'Smart Tester', 'x')
		returning id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var clientID int64
	if err := pool.QueryRow(ctx, `
		insert into bank_client (user_id, bank_id, label, program_tier_id)
		values ($1, $2, 'основной', $3)
		returning id`, userID, bankID, tierID).Scan(&clientID); err != nil {
		t.Fatal(err)
	}

	// A tier that another bank happens to name the same way must not be touched.
	var otherBank int32
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('ДругойБанк') returning id`).Scan(&otherBank); err != nil {
		t.Fatal(err)
	}
	var otherProg int64
	if err := pool.QueryRow(ctx, `
		insert into cashback_program (bank_id, name, period_type, selection_mode, currency_kind)
		values ($1, 'Кэшбэк', 'calendar_month', 'atomic', 'rub')
		returning id`, otherBank).Scan(&otherProg); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into program_tier (program_id, name, cap_scope)
		values ($1, 'Альфа-Смарт', 'total')`, otherProg); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 29); err != nil {
		t.Fatalf("up to 29: %v", err)
	}

	// The client must still point at the SAME row, now named M — not at a new
	// tier, and not at a dangling id.
	var gotID int64
	var gotName string
	var gotCap string
	if err := pool.QueryRow(ctx, `
		select pt.id, pt.name, pt.cap_value::text
		from bank_client bc
		         join program_tier pt on pt.id = bc.program_tier_id
		where bc.id = $1`, clientID).Scan(&gotID, &gotName, &gotCap); err != nil {
		t.Fatalf("client lost its tier: %v", err)
	}
	if gotID != tierID {
		t.Errorf("client moved to tier %d, want the original %d", gotID, tierID)
	}
	if gotName != "Альфа-Смарт M" {
		t.Errorf("tier name = %q, want «Альфа-Смарт M»", gotName)
	}
	if gotCap != "7000" {
		t.Errorf("cap = %s, want 7000 (existing subscribers must not be downgraded)", gotCap)
	}

	var otherName string
	if err := pool.QueryRow(ctx, `
		select pt.name from program_tier pt where pt.program_id = $1`, otherProg).Scan(&otherName); err != nil {
		t.Fatal(err)
	}
	if otherName != "Альфа-Смарт" {
		t.Errorf("another bank's tier was renamed to %q; the update is not scoped", otherName)
	}
}

// Migration test for 00038. The rename exists to keep existing rows attached,
// which a fresh-database run never exercises: there the seed inserts the new
// name and there is nothing to re-point.
package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestUp00038KeepsProgramReferencesAttached(t *testing.T) {
	ctx := context.Background()
	pool := newPG(ctx, t)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 37); err != nil {
		t.Fatalf("up to 37: %v", err)
	}

	var bankID int32
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('Тестбанк') returning id`).Scan(&bankID); err != nil {
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
		insert into program_tier (program_id, name, cap_value, cap_scope, max_categories)
		values ($1, 'Стандартный', 5000, 'total', 3)
		returning id`, progID).Scan(&tierID); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		insert into "user" (username, email, display_name, password_hash)
		values ('spellingtester', 'spelling@example.com', 'Spelling Tester', 'x')
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

	if _, err := provider.UpTo(ctx, 38); err != nil {
		t.Fatalf("up to 38: %v", err)
	}

	// The program keeps its identity — a second row would strand everything
	// below it, which is the whole reason this is a migration.
	var gotProgID int64
	var gotName string
	if err := pool.QueryRow(ctx, `
		select cp.id, cp.name from program_tier pt join cashback_program cp on cp.id = pt.program_id
		where pt.id = $1`, tierID).Scan(&gotProgID, &gotName); err != nil {
		t.Fatalf("tier lost its program: %v", err)
	}
	if gotProgID != progID {
		t.Errorf("tier moved to program %d, want the original %d", gotProgID, progID)
	}
	if gotName != "Кешбэк" {
		t.Errorf("program name = %q, want «Кешбэк»", gotName)
	}

	var gotTierID int64
	var gotCap string
	if err := pool.QueryRow(ctx, `
		select pt.id, pt.cap_value::text
		from bank_client bc join program_tier pt on pt.id = bc.program_tier_id
		where bc.id = $1`, clientID).Scan(&gotTierID, &gotCap); err != nil {
		t.Fatalf("client lost its tier: %v", err)
	}
	if gotTierID != tierID {
		t.Errorf("client moved to tier %d, want the original %d", gotTierID, tierID)
	}
	if gotCap != "5000" {
		t.Errorf("cap = %s, want 5000 unchanged by a rename", gotCap)
	}

	var programs int
	if err := pool.QueryRow(ctx,
		`select count(*) from cashback_program where bank_id = $1`, bankID).Scan(&programs); err != nil {
		t.Fatal(err)
	}
	if programs != 1 {
		t.Errorf("found %d programs after the rename, want exactly 1", programs)
	}
}

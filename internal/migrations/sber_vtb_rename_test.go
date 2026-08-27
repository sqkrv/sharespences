// Migration test for 00031. Both renames exist to keep existing rows attached,
// which a fresh-database run never exercises: there the seed simply inserts the
// new names and there is nothing to re-point.
package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestUp00031KeepsReferencesAttached(t *testing.T) {
	ctx := context.Background()
	pool := newPG(ctx, t)

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 30); err != nil {
		t.Fatalf("up to 30: %v", err)
	}

	var sberID, vtbID int32
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('Сбербанк') returning id`).Scan(&sberID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('ВТБ') returning id`).Scan(&vtbID); err != nil {
		t.Fatal(err)
	}
	var vtbProg int64
	if err := pool.QueryRow(ctx, `
		insert into cashback_program (bank_id, name, period_type, selection_mode, currency_kind)
		values ($1, 'Кэшбэк', 'calendar_month', 'atomic', 'rub')
		returning id`, vtbID).Scan(&vtbProg); err != nil {
		t.Fatal(err)
	}
	var stdTier int64
	if err := pool.QueryRow(ctx, `
		insert into program_tier (program_id, name, cap_value, cap_scope, max_categories)
		values ($1, 'Стандартный', 3000, 'total', 3)
		returning id`, vtbProg).Scan(&stdTier); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		insert into "user" (username, email, display_name, password_hash)
		values ('renametester', 'rename@example.com', 'Rename Tester', 'x')
		returning id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	// One client per bank: the Сбер one proves the bank rename does not orphan
	// it, the ВТБ one proves the tier rename does not.
	var sberClient, vtbClient int64
	if err := pool.QueryRow(ctx, `
		insert into bank_client (user_id, bank_id, label) values ($1, $2, 'основной')
		returning id`, userID, sberID).Scan(&sberClient); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		insert into bank_client (user_id, bank_id, label, program_tier_id) values ($1, $2, 'основной', $3)
		returning id`, userID, vtbID, stdTier).Scan(&vtbClient); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 31); err != nil {
		t.Fatalf("up to 31: %v", err)
	}

	var gotBankID int32
	var gotBankName string
	if err := pool.QueryRow(ctx, `
		select b.id, b.name from bank_client bc join bank b on b.id = bc.bank_id
		where bc.id = $1`, sberClient).Scan(&gotBankID, &gotBankName); err != nil {
		t.Fatalf("Сбер client lost its bank: %v", err)
	}
	if gotBankID != sberID {
		t.Errorf("client moved to bank %d, want the original %d", gotBankID, sberID)
	}
	if gotBankName != "СберБанк" {
		t.Errorf("bank name = %q, want «СберБанк»", gotBankName)
	}

	var gotTierID int64
	var gotTierName, gotCap string
	if err := pool.QueryRow(ctx, `
		select pt.id, pt.name, pt.cap_value::text
		from bank_client bc join program_tier pt on pt.id = bc.program_tier_id
		where bc.id = $1`, vtbClient).Scan(&gotTierID, &gotTierName, &gotCap); err != nil {
		t.Fatalf("ВТБ client lost its tier: %v", err)
	}
	if gotTierID != stdTier {
		t.Errorf("client moved to tier %d, want the original %d", gotTierID, stdTier)
	}
	if gotTierName != "Мультикарта" {
		t.Errorf("tier name = %q, want «Мультикарта»", gotTierName)
	}
	if gotCap != "3000" {
		t.Errorf("cap = %s, want 3000 unchanged by a rename", gotCap)
	}

	// A rename must not create a duplicate under 00030's constraint.
	var banks int
	if err := pool.QueryRow(ctx,
		`select count(*) from bank where name in ('СберБанк', 'Сбербанк')`).Scan(&banks); err != nil {
		t.Fatal(err)
	}
	if banks != 1 {
		t.Errorf("found %d Сбер rows after the rename, want exactly 1", banks)
	}
}

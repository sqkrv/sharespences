// Covers the path a fresh-database run never reaches: a re-seed over a
// database whose program_tier rows already exist. The tier insert is guarded by
// NOT EXISTS, so before the refresh pass a corrected cap could never land —
// Alfa Only shipped 15 000 ₽ against a documented 30 000 ₽ for exactly that
// reason. Needs a real PostGIS: the seed writes enum-typed columns.
package seed

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/migrations"
)

func TestTierRefreshCorrectsExistingRows(t *testing.T) {
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

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	capOf := func(bank, tier string) string {
		t.Helper()
		var v string
		if err := pool.QueryRow(ctx, `
			select pt.cap_value::text
			from program_tier pt
			         join cashback_program cp on cp.id = pt.program_id
			         join bank b on b.id = cp.bank_id
			where b.name = $1 and pt.name = $2`, bank, tier).Scan(&v); err != nil {
			t.Fatalf("read cap %s/%s: %v", bank, tier, err)
		}
		return v
	}

	// The documented value (MCCD appendix, 2026-08) must be what a fresh seed
	// produces — this is the number the previous constant got wrong.
	if got := capOf("Альфа-Банк", "Alfa Only"); got != "30000" {
		t.Errorf("fresh seed: Alfa Only cap = %s, want 30000", got)
	}

	// Simulate a database seeded before the correction, then re-seed. Without
	// the refresh pass the NOT EXISTS guard leaves the stale value in place.
	if _, err := pool.Exec(ctx, `
		update program_tier pt
		set cap_value = 15000, max_categories = 4
		from cashback_program cp, bank b
		where pt.program_id = cp.id and cp.bank_id = b.id
		  and b.name = 'Альфа-Банк' and pt.name = 'Alfa Only'`); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if got := capOf("Альфа-Банк", "Alfa Only"); got != "30000" {
		t.Errorf("re-seed did not refresh stale cap: got %s, want 30000", got)
	}

	var slots int
	if err := pool.QueryRow(ctx, `
		select pt.max_categories
		from program_tier pt
		         join cashback_program cp on cp.id = pt.program_id
		         join bank b on b.id = cp.bank_id
		where b.name = 'Альфа-Банк' and pt.name = 'Alfa Only'`).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if slots != 5 {
		t.Errorf("re-seed did not refresh slot count: got %d, want 5", slots)
	}

	// Refreshing must not multiply rows — the natural key still guards inserts.
	var n int
	if err := pool.QueryRow(ctx, `
		select count(*)
		from program_tier pt
		         join cashback_program cp on cp.id = pt.program_id
		         join bank b on b.id = cp.bank_id
		where b.name = 'Альфа-Банк'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("Альфа-Банк tier count = %d, want 4", n)
	}
}

// The fresh-database shape of the split: both tiers present with their own
// caps and slot counts, and the pre-split name gone. The upgrade path — an
// existing «Альфа-Смарт» row with clients on it — is 00029's job and is covered
// in internal/migrations/alfa_smart_levels_test.go.
func TestAlfaSmartTiersSeedAsSAndM(t *testing.T) {
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

	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var sCap, mCap string
	var sSlots, mSlots int
	if err := pool.QueryRow(ctx, `
		select pt.cap_value::text, pt.max_categories
		from program_tier pt
		         join cashback_program cp on cp.id = pt.program_id
		         join bank b on b.id = cp.bank_id
		where b.name = 'Альфа-Банк' and pt.name = 'Альфа-Смарт S'`).Scan(&sCap, &sSlots); err != nil {
		t.Fatalf("Альфа-Смарт S missing: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		select pt.cap_value::text, pt.max_categories
		from program_tier pt
		         join cashback_program cp on cp.id = pt.program_id
		         join bank b on b.id = cp.bank_id
		where b.name = 'Альфа-Банк' and pt.name = 'Альфа-Смарт M'`).Scan(&mCap, &mSlots); err != nil {
		t.Fatalf("Альфа-Смарт M missing: %v", err)
	}
	if sCap != "5000" || sSlots != 3 {
		t.Errorf("S = %s ₽ / %d slots, want 5000 / 3", sCap, sSlots)
	}
	if mCap != "7000" || mSlots != 4 {
		t.Errorf("M = %s ₽ / %d slots, want 7000 / 4", mCap, mSlots)
	}

	// The un-split name must be gone, or a client could still be attached to a
	// tier the seed no longer maintains.
	var stale int
	if err := pool.QueryRow(ctx, `
		select count(*)
		from program_tier pt
		         join cashback_program cp on cp.id = pt.program_id
		         join bank b on b.id = cp.bank_id
		where b.name = 'Альфа-Банк' and pt.name = 'Альфа-Смарт'`).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Errorf("pre-split «Альфа-Смарт» tier still present (%d rows)", stale)
	}
}

package mcc

// End-to-end for the snapshot import against a real PostGIS: baseline
// import journals `imported`/`excluded_imported`, a re-import is a strict
// no-op, a later snapshot journals real ±movements, and an unresolved
// code-carrying title aborts without writing. The test builds its own tiny
// bank + catalog instead of running the full seed, so it cannot drift with
// seed content.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/migrations"
)

func TestImportSnapshotE2E(t *testing.T) {
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

	var bankID int32
	if err := pool.QueryRow(ctx,
		`insert into bank (name) values ('Тестбанк') returning id`).Scan(&bankID); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"АЗС", "Такси"} {
		if _, err := pool.Exec(ctx,
			`insert into bank_category (bank_id, title) values ($1, $2)`, bankID, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, code := range []int16{5541, 5542, 4121, 3990} {
		if _, err := pool.Exec(ctx,
			`insert into mcc (code, name) values ($1, 'e2e') on conflict do nothing`, code); err != nil {
			t.Fatal(err)
		}
	}

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}

	first := `{
		"schema_version": 2, "bank": "Тестбанк", "captured_at": "2026-09-01",
		"source": {"id": "test-pl", "file": "first.json", "sha256": "aaaa000000000000"},
		"categories": [
			{"title": "АЗС", "mcc": ["5541", "5542"]},
			{"title": "Такси", "mcc": ["4121"],
			 "qualified": [{"mcc": "3990", "resolves_to": ["Яндекс Go"], "when": "только МИР"}]},
			{"title": "Дикси", "mcc": []}
		],
		"exclusions": {"mcc": ["4829"], "classes": ["оплата по СБП"]},
		"dictionary": [{"mcc": "4829", "name": "Денежные переводы"}]
	}`

	if err := ImportSnapshot(ctx, pool, []byte(first), false, t.Logf); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if n := count(`select count(*) from bank_category_mcc`); n != 4 {
		t.Fatalf("membership rows = %d, want 4", n)
	}
	var note string
	if err := pool.QueryRow(ctx, `
		select bcm.note from bank_category_mcc bcm
		join bank_category bc on bc.id = bcm.bank_category_id
		where bc.title = 'Такси' and bcm.mcc_code = 3990`).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != "Яндекс Go — только МИР" {
		t.Fatalf("qualified note = %q", note)
	}
	if n := count(`select count(*) from mcc where code = 4829`); n != 1 {
		t.Fatal("dictionary gloss not inserted")
	}
	if n := count(`select count(*) from bank_exclusion where kind = 'class' and value = 'оплата по СБП'`); n != 1 {
		t.Fatal("class exclusion not inserted")
	}
	// baseline actions, never `added`
	if n := count(`select count(*) from mcc_change where action = 'imported'`); n != 4 {
		t.Fatalf("imported journal rows = %d, want 4", n)
	}
	if n := count(`select count(*) from mcc_change where action = 'excluded_imported'`); n != 2 {
		t.Fatalf("excluded_imported rows = %d, want 2", n)
	}
	if n := count(`select count(*) from mcc_change where action = 'category_added' and category_title = 'Дикси'`); n != 1 {
		t.Fatalf("category_added for bare unknown title = %d, want 1", n)
	}

	journalBefore := count(`select count(*) from mcc_change`)
	if err := ImportSnapshot(ctx, pool, []byte(first), false, t.Logf); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if n := count(`select count(*) from mcc_change`); n != journalBefore {
		t.Fatalf("re-import journaled %d new rows", n-journalBefore)
	}

	second := strings.NewReplacer(
		`"mcc": ["5541", "5542"]`, `"mcc": ["5541"]`,
		`"mcc": ["4829"], "classes": ["оплата по СБП"]`, `"mcc": ["4829", "5933"], "classes": ["оплата по СБП"]`,
		`"sha256": "aaaa000000000000"`, `"sha256": "bbbb000000000000"`,
		`"dictionary": [{"mcc": "4829", "name": "Денежные переводы"}]`,
		`"dictionary": [{"mcc": "4829", "name": "Денежные переводы"}, {"mcc": "5933", "name": "Ломбарды"}]`,
	).Replace(first)
	if err := ImportSnapshot(ctx, pool, []byte(second), false, t.Logf); err != nil {
		t.Fatalf("second import: %v", err)
	}
	if n := count(`select count(*) from mcc_change where action = 'removed' and mcc_code = 5542`); n != 1 {
		t.Fatal("5542 removal not journaled")
	}
	if n := count(`select count(*) from mcc_change where action = 'excluded_added' and mcc_code = 5933`); n != 1 {
		t.Fatal("5933 exclusion add not journaled as excluded_added (kind had rows)")
	}

	// a second document of the same bank syncs only its own exclusion rows:
	// its overlapping 4829 is a second attributable row, the first
	// document's 5933 survives, and re-importing the first is still a no-op
	other := `{
		"schema_version": 2, "bank": "Тестбанк", "captured_at": "2026-09-01",
		"source": {"id": "test-blocklist", "file": "other.json", "sha256": "cccc000000000000"},
		"exclusions": {"mcc": ["4829", "6010"]}
	}`
	if err := ImportSnapshot(ctx, pool, []byte(other), false, t.Logf); err != nil {
		t.Fatalf("other-source import: %v", err)
	}
	if n := count(`select count(*) from bank_exclusion where kind = 'mcc'`); n != 4 {
		t.Fatalf("mcc exclusion rows across two sources = %d, want 4 (4829×2, 5933, 6010)", n)
	}
	if n := count(`select count(*) from mcc_change where action = 'excluded_imported' and mcc_code = 6010`); n != 1 {
		t.Fatal("the other source's first load must journal excluded_imported (its own scope was empty)")
	}
	journalBefore = count(`select count(*) from mcc_change`)
	if err := ImportSnapshot(ctx, pool, []byte(second), false, t.Logf); err != nil {
		t.Fatalf("re-import after sibling source: %v", err)
	}
	if n := count(`select count(*) from bank_exclusion where kind = 'mcc'`); n != 4 {
		t.Fatalf("sibling source's rows were touched: mcc rows = %d, want 4", n)
	}
	if n := count(`select count(*) from mcc_change`); n != journalBefore {
		t.Fatalf("re-import after sibling source journaled %d rows", n-journalBefore)
	}

	// dry run over a changed state writes nothing
	if err := ImportSnapshot(ctx, pool, []byte(first), true, t.Logf); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if n := count(`select count(*) from bank_category_mcc where mcc_code = 5542`); n != 0 {
		t.Fatal("dry run wrote membership")
	}

	// unresolved title with codes aborts with nothing written
	bad := strings.Replace(second, `"title": "АЗС"`, `"title": "Несуществующая"`, 1)
	rowsBefore := count(`select count(*) from bank_category_mcc`)
	if err := ImportSnapshot(ctx, pool, []byte(bad), false, t.Logf); err == nil ||
		!strings.Contains(err.Error(), "Несуществующая") {
		t.Fatalf("unresolved title must fail, got %v", err)
	}
	if n := count(`select count(*) from bank_category_mcc`); n != rowsBefore {
		t.Fatal("failed import left writes behind")
	}
}

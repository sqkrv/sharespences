// Migration test for 00027 (point-of-sale origin). Covers the two things a
// fresh-database run never reaches: the backfill that labels the base that
// already exists in production (62 117 rows from the mcc-codes.ru scrape),
// and the deliberate ABSENCE of a default — the property that makes a future
// write path fail loudly instead of being silently recorded as a scrape.
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

func TestUp00027LabelsTheExistingBase(t *testing.T) {
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

	// Everything before 00027. goose applies every version ≤ the target, so
	// naming 26 is stable: 00023–00026 precede this branch's migration
	// lineage.
	if _, err := provider.UpTo(ctx, 26); err != nil {
		t.Fatalf("up to 00026: %v", err)
	}

	// A row shaped the way `import-pos` wrote them before this migration.
	if _, err := pool.Exec(ctx, `insert into mcc (code, name) values (5411, 'Супермаркеты')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`insert into point_of_sale (name, merchant_title, mcc_code, confirmations)
		 values ('Пятёрочка на Ленина', 'Пятёрочка', 5411, 12)`); err != nil {
		t.Fatalf("insert pre-00027 row: %v", err)
	}

	if _, err := provider.UpTo(ctx, 27); err != nil {
		t.Fatalf("up to 00027: %v", err)
	}

	// The backfill: everything that predates the migration is the scrape.
	var origin string
	var userConfirmations, confirmations int64
	if err := pool.QueryRow(ctx,
		`select origin::text, user_confirmations, confirmations from point_of_sale`).
		Scan(&origin, &userConfirmations, &confirmations); err != nil {
		t.Fatal(err)
	}
	if origin != "mcc_codes" {
		t.Errorf("origin = %q, want mcc_codes — the base predates every other write path", origin)
	}
	// The two counters stay apart: mcc-codes.ru's number is untouched and
	// ours starts at zero rather than inheriting it.
	if confirmations != 12 {
		t.Errorf("confirmations = %d, want the imported 12 left alone", confirmations)
	}
	if userConfirmations != 0 {
		t.Errorf("user_confirmations = %d, want 0 — ours must not inherit theirs", userConfirmations)
	}

	// No default, on purpose: a write path that forgets to say where its row
	// came from must fail rather than be recorded as a scrape.
	_, err = pool.Exec(ctx, `insert into point_of_sale (name, mcc_code) values ('Без происхождения', 5411)`)
	if err == nil {
		t.Fatal("an unlabelled row was accepted — origin must have no default")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "origin") {
		t.Errorf("rejection should name the origin column, got: %v", err)
	}

	// Every label the enum promises is usable — the three write paths that
	// do not exist yet must not need another migration.
	for _, label := range []string{"user_manual", "user_transaction", "admin"} {
		if _, err := pool.Exec(ctx,
			`insert into point_of_sale (name, mcc_code, origin) values ($1, 5411, $2::point_of_sale_origin)`,
			"Точка "+label, label); err != nil {
			t.Errorf("origin %q rejected: %v", label, err)
		}
	}
}

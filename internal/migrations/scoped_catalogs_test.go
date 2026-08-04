// Migration tests for 00019 (user-scoped catalogs), covering the two paths a
// fresh-database run never reaches: the one-shot attribution backfill, which
// only ever executes against existing production data, and the Down that has
// to collapse two namespaces back into one. Both need a real PostGIS — the
// constraints under test are NULLS NOT DISTINCT uniques.
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

func TestDown00019(t *testing.T) {
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgis/postgis:16-3.4",
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

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("up: %v", err)
	}

	// A bank, two accounts, and the case the split allows: one (bank, title)
	// held by a global row and by both accounts.
	var bankID int32
	if err := pool.QueryRow(ctx, `insert into bank (name) values ('ТестБанк') returning id`).Scan(&bankID); err != nil {
		t.Fatal(err)
	}
	var u1, u2 string
	if err := pool.QueryRow(ctx, `insert into "user" (username, display_name, email) values ('aaa','A','a@example.com') returning id`).Scan(&u1); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `insert into "user" (username, display_name, email) values ('bbb','B','b@example.com') returning id`).Scan(&u2); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []any{nil, u1, u2} {
		if _, err := pool.Exec(ctx,
			`insert into bank_category (bank_id, title, is_custom, created_by) values ($1, 'Кофейни', $2::uuid is not null, $2::uuid)`,
			bankID, owner); err != nil {
			t.Fatalf("insert row owned by %v: %v", owner, err)
		}
	}
	var canonID int64
	if err := pool.QueryRow(ctx,
		`insert into canonical_category (slug, title_ru) values ('coffee', 'Кофейни') returning id`).Scan(&canonID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`insert into bank_category_alias (canonical_category_id, bank_id, raw_title, user_id)
		 values ($1, $2, 'Кофейни', $3::uuid)`, canonID, bankID, u1); err != nil {
		t.Fatalf("insert private alias: %v", err)
	}
	// The global mapping of the same title coexists — the row the seed owns.
	if _, err := pool.Exec(ctx,
		`insert into bank_category_alias (canonical_category_id, bank_id, raw_title, user_id)
		 values ($1, $2, 'Кофейни', null)`, canonID, bankID); err != nil {
		t.Fatalf("insert global alias: %v", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatalf("down 00019: %v", err)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`select count(*) from bank_category where bank_id = $1 and title = 'Кофейни'`, bankID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("after down: %d rows for one (bank, title), want 1", rows)
	}
	var hasCol bool
	if err := pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.columns
		 where table_name = 'bank_category' and column_name = 'created_by')`).Scan(&hasCol); err != nil {
		t.Fatal(err)
	}
	if hasCol {
		t.Fatal("created_by survived the down migration")
	}
	// The old unique is back and enforcing.
	if _, err := pool.Exec(ctx,
		`insert into bank_category (bank_id, title) values ($1, 'Кофейни')`, bankID); err == nil {
		t.Fatal("duplicate (bank_id, title) accepted — the pre-00019 unique was not restored")
	}
	var aliasRows int
	if err := pool.QueryRow(ctx, `select count(*) from bank_category_alias where bank_id = $1`, bankID).Scan(&aliasRows); err != nil {
		t.Fatal(err)
	}
	if aliasRows != 1 {
		t.Fatalf("aliases after down = %d, want 1 (the global row kept, the private one dropped)", aliasRows)
	}

	// And it re-applies.
	if err := Up(ctx, pool); err != nil {
		t.Fatalf("re-up: %v", err)
	}
}

// The backfill runs once, on production data, and no fresh-database test
// reaches it: stop at 00018, write the pre-migration shapes, then apply.
func TestBackfill00019(t *testing.T) {
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgis/postgis:16-3.4",
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
	if _, err := provider.UpTo(ctx, 18); err != nil {
		t.Fatalf("up to 00018: %v", err)
	}

	var bankID int32
	if err := pool.QueryRow(ctx, `insert into bank (name) values ('ТестБанк') returning id`).Scan(&bankID); err != nil {
		t.Fatal(err)
	}
	users := map[string]string{}
	periods := map[string]int64{}
	for _, name := range []string{"aaa", "bbb"} {
		var uid string
		if err := pool.QueryRow(ctx,
			`insert into "user" (username, display_name, email) values ($1, $1, $1 || '@example.com') returning id`,
			name).Scan(&uid); err != nil {
			t.Fatal(err)
		}
		users[name] = uid
		var cid int64
		if err := pool.QueryRow(ctx,
			`insert into bank_client (user_id, bank_id) values ($1::uuid, $2) returning id`, uid, bankID).Scan(&cid); err != nil {
			t.Fatal(err)
		}
		var pid int64
		if err := pool.QueryRow(ctx,
			`insert into offer_period (bank_client_id, period_start, period_end)
			 values ($1, '2026-07-01', '2026-07-31') returning id`, cid).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		periods[name] = pid
	}

	catalog := map[string]int64{}
	for _, row := range []struct {
		title  string
		custom bool
	}{
		{"ОдинАвтор", true},    // cited by aaa alone → attributed
		{"ДваАвтора", true},    // cited by both → stays global
		{"НикемНеВзята", true}, // cited by nobody → stays global
		{"Сидовая", false},     // seeded row cited by aaa → stays global
	} {
		var id int64
		if err := pool.QueryRow(ctx,
			`insert into bank_category (bank_id, title, is_custom) values ($1, $2, $3) returning id`,
			bankID, row.title, row.custom).Scan(&id); err != nil {
			t.Fatal(err)
		}
		catalog[row.title] = id
	}
	cite := func(user, title string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`insert into category_offer (offer_period_id, raw_title, bank_category_id) values ($1, $2, $3)`,
			periods[user], title, catalog[title]); err != nil {
			t.Fatal(err)
		}
	}
	cite("aaa", "ОдинАвтор")
	cite("aaa", "ДваАвтора")
	cite("bbb", "ДваАвтора")
	cite("aaa", "Сидовая")

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("apply 00019: %v", err)
	}

	owner := func(title string) *string {
		t.Helper()
		var got *string
		if err := pool.QueryRow(ctx, `select created_by::text from bank_category where id = $1`, catalog[title]).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := owner("ОдинАвтор"); got == nil || *got != users["aaa"] {
		t.Fatalf("unambiguous row: created_by = %v, want %s", got, users["aaa"])
	}
	for _, title := range []string{"ДваАвтора", "НикемНеВзята", "Сидовая"} {
		if got := owner(title); got != nil {
			t.Fatalf("%s: created_by = %v, want null (global)", title, *got)
		}
	}
}

// Migration-runner test: a migration numbered below the database's current
// version must still be applied. ADR-0007 puts work on short branches off
// main, so two branches routinely take the same next number and whichever
// merges second lands «in the past» of an already-deployed database — which
// is what 00023 (foreign-key indexes) did: branched before 00036 (партнёрки v2, then numbered 00024) existed,
// merged after 00028 was live, and stopped a deploy with «detected 1 missing
// (out-of-order) migration lower than database version (28): version 23».
//
// A fresh-database run can never reach this state, which is why the failure
// only ever showed up on the server.
package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// fkIndexes are what 00023 creates; the test drops them alongside its version
// row to rebuild the deployed state exactly.
var fkIndexes = []string{
	"category_offer_period_idx",
	"partner_offer_user_idx",
	"partner_offer_client_idx",
	"bank_card_client_idx",
	"offer_period_attachment_file_idx",
	"partner_offer_attachment_file_idx",
}

func TestUpAppliesAnOutOfOrderMigration(t *testing.T) {
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

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Rebuild the server's state: every version applied except 00023, whose
	// branch had not merged yet when the database reached 00028.
	if _, err := pool.Exec(ctx, `delete from goose_db_version where version_id = 23`); err != nil {
		t.Fatal(err)
	}
	for _, idx := range fkIndexes {
		if _, err := pool.Exec(ctx, `drop index if exists `+idx); err != nil {
			t.Fatal(err)
		}
	}
	var version int64
	if err := pool.QueryRow(ctx, `select max(version_id) from goose_db_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 23 {
		t.Fatalf("database version %d — this test needs a version above the out-of-order one", version)
	}

	// The run that used to fail.
	if err := Up(ctx, pool); err != nil {
		t.Fatalf("out-of-order run: %v", err)
	}

	var applied bool
	if err := pool.QueryRow(ctx,
		`select exists (select 1 from goose_db_version where version_id = 23)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Error("00023 is not recorded as applied after the run")
	}
	for _, idx := range fkIndexes {
		var exists bool
		if err := pool.QueryRow(ctx,
			`select exists (select 1 from pg_indexes where indexname = $1)`, idx).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("index %s missing — the out-of-order migration did not really run", idx)
		}
	}

	// Idempotent: the next deploy has nothing left to do and must not fail.
	if err := Up(ctx, pool); err != nil {
		t.Fatalf("re-run after catching up: %v", err)
	}
}

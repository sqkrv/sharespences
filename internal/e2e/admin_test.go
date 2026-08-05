// TestAdminE2E runs the admin sidecar spec's acceptance script
// (docs/specs/admin.md, «Definition of done»): dashboard sanity, the
// seed-coverage guards from both sides, PoS pagination with a windowed
// total, journal hand-adds, and the load-bearing invariant — a seed re-run
// after admin edits leaves them intact and writes no journal noise.
package e2e_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/admin"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
)

func TestAdminE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e needs Docker")
	}
	ctx := context.Background()

	pg, err := postgres.Run(ctx, "postgis/postgis:18-3.6",
		postgres.WithDatabase("sharespences"),
		postgres.WithUsername("sharespences"),
		postgres.WithPassword("sharespences"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("cannot start PostGIS container (Docker unavailable?): %v", err)
	}
	defer func() { _ = testcontainers.TerminateContainer(pg) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatal(err)
	}

	handler, err := admin.New(admin.Config{Pool: pool, Version: "vtest"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	c := newClient(t, srv.URL)

	// --- dashboard + version ---
	var ver struct {
		Version string `json:"version"`
	}
	c.must("GET", "/api/version", nil, &ver, 200)
	if ver.Version != "vtest" {
		t.Fatalf("version: got %q", ver.Version)
	}
	var dash struct {
		DBSizeBytes int64            `json:"db_size_bytes"`
		Counts      map[string]int64 `json:"counts"`
		Migrations  []struct {
			Version   int64   `json:"version"`
			AppliedAt *string `json:"applied_at"`
		} `json:"migrations"`
		Tables []struct {
			Table string `json:"table"`
		} `json:"tables"`
	}
	c.must("GET", "/api/dashboard", nil, &dash, 200)
	if dash.DBSizeBytes <= 0 {
		t.Fatal("dashboard: zero db size")
	}
	for _, k := range []string{"banks", "canonical_categories", "bank_categories", "mcc_codes", "mcc_links", "programs"} {
		if dash.Counts[k] == 0 {
			t.Fatalf("dashboard: seeded count %s is 0", k)
		}
	}
	if len(dash.Migrations) == 0 {
		t.Fatal("dashboard: no migrations listed")
	}
	for _, m := range dash.Migrations {
		if m.AppliedAt == nil {
			t.Fatalf("dashboard: migration %d pending on a fresh migrate", m.Version)
		}
	}

	// --- MCC dictionary: seeded codes read-only, new codes fully editable ---
	mccBody := map[string]any{"name": "Тестовый код", "description": "e2e"}
	if got := c.do("PUT", "/api/mcc/5411", mccBody, nil); got != 409 {
		t.Fatalf("edit of seeded MCC 5411: status %d, want 409", got)
	}
	if got := c.do("DELETE", "/api/mcc/5411", nil, nil); got != 409 {
		t.Fatalf("delete of seeded MCC 5411: status %d, want 409", got)
	}
	c.must("POST", "/api/mcc", map[string]any{"code": 2999, "name": "Тестовый код"}, nil, 201)
	c.must("PUT", "/api/mcc/2999", map[string]any{"name": "Тестовый код 2"}, nil, 200)
	var mccPage struct {
		Total int64 `json:"total"`
		Items []struct {
			Code        int  `json:"code"`
			SeedManaged bool `json:"seed_managed"`
		} `json:"items"`
	}
	c.must("GET", "/api/mcc?query=2999&limit=10", nil, &mccPage, 200)
	if len(mccPage.Items) != 1 || mccPage.Items[0].Code != 2999 || mccPage.Items[0].SeedManaged {
		t.Fatalf("search for 2999: %+v", mccPage)
	}

	// --- bank_category guards: seeded row refused, custom row editable ---
	var banks []struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}
	c.must("GET", "/api/banks", nil, &banks, 200)
	var cats []struct {
		ID          int64  `json:"id"`
		BankName    string `json:"bank_name"`
		Title       string `json:"title"`
		SeedManaged bool   `json:"seed_managed"`
	}
	c.must("GET", "/api/bank-categories", nil, &cats, 200)
	catBody := map[string]any{"title": "Правка", "kind": "regular", "active": true}
	var seededCat int64
	for _, bc := range cats {
		if bc.SeedManaged {
			seededCat = bc.ID
			break
		}
	}
	if seededCat == 0 {
		t.Fatal("no seed-managed catalog row found")
	}
	if got := c.do("PUT", fmt.Sprintf("/api/bank-categories/%d", seededCat), catBody, nil); got != 409 {
		t.Fatalf("edit of seeded catalog row: status %d, want 409", got)
	}

	var customID int64
	if err := pool.QueryRow(ctx,
		`insert into bank_category (bank_id, title, is_custom) values ($1, 'Тестовая категория адм', true) returning id`,
		banks[0].ID).Scan(&customID); err != nil {
		t.Fatal(err)
	}
	c.must("PUT", fmt.Sprintf("/api/bank-categories/%d", customID),
		map[string]any{"title": "Тестовая категория адм", "kind": "special", "emoji": "🧪", "active": true}, nil, 200)
	// A custom row's (bank, title) is outside the seed CSV → links editable.
	c.must("PUT", fmt.Sprintf("/api/bank-categories/%d/mcc/2999", customID),
		map[string]any{"note": "e2e link"}, nil, 204)

	// --- links guard: a CSV-covered category refuses link writes ---
	keys, err := seed.SeededMembershipKeys()
	if err != nil {
		t.Fatal(err)
	}
	var coveredID int64
	for _, bc := range cats {
		if keys[[2]string{bc.BankName, bc.Title}] {
			coveredID = bc.ID
			break
		}
	}
	if coveredID == 0 {
		t.Fatal("no CSV-covered catalog row found")
	}
	var links struct {
		SeedManaged bool `json:"seed_managed"`
	}
	c.must("GET", fmt.Sprintf("/api/bank-categories/%d/mcc", coveredID), nil, &links, 200)
	if !links.SeedManaged {
		t.Fatalf("covered category %d not reported seed_managed", coveredID)
	}
	if got := c.do("PUT", fmt.Sprintf("/api/bank-categories/%d/mcc/2999", coveredID),
		map[string]any{}, nil); got != 409 {
		t.Fatalf("link write under covered category: status %d, want 409", got)
	}

	// --- journal hand-add ---
	var created struct {
		ID int64 `json:"id"`
	}
	c.must("POST", "/api/mcc-changes", map[string]any{
		"bank_id": banks[0].ID, "category_title": "Тестовая категория адм",
		"mcc_code": 2999, "action": "added", "note": "e2e",
	}, &created, 201)
	var journal struct {
		Total int64 `json:"total"`
		Items []struct {
			ID     int64  `json:"id"`
			Source string `json:"source"`
		} `json:"items"`
	}
	c.must("GET", "/api/mcc-changes?limit=5", nil, &journal, 200)
	if len(journal.Items) == 0 || journal.Items[0].ID != created.ID || journal.Items[0].Source != "manual (admin)" {
		t.Fatalf("journal head after hand-add: %+v", journal.Items)
	}

	// --- PoS: create, windowed pagination, edit, delete ---
	var posIDs []string
	for i := 1; i <= 3; i++ {
		var out struct {
			ID string `json:"id"`
		}
		c.must("POST", "/api/pos", map[string]any{
			"name": fmt.Sprintf("Тестовый ларёк %d", i), "mcc_code": 2999, "type": "offline",
		}, &out, 201)
		posIDs = append(posIDs, out.ID)
	}
	var pos struct {
		Total int64 `json:"total"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	c.must("GET", "/api/pos?query=Тестовый+ларёк&limit=2&offset=0", nil, &pos, 200)
	if pos.Total != 3 || len(pos.Items) != 2 {
		t.Fatalf("pos page 1: total %d, items %d (want 3, 2)", pos.Total, len(pos.Items))
	}
	c.must("GET", "/api/pos?query=Тестовый+ларёк&limit=2&offset=2", nil, &pos, 200)
	if pos.Total != 3 || len(pos.Items) != 1 {
		t.Fatalf("pos page 2: total %d, items %d (want 3, 1)", pos.Total, len(pos.Items))
	}
	c.must("PUT", "/api/pos/"+posIDs[0], map[string]any{"name": "Тестовый ларёк 1 (переименован)"}, nil, 200)
	c.must("DELETE", "/api/pos/"+posIDs[2], nil, nil, 204)

	// --- the invariant: a seed re-run leaves admin edits intact, no noise ---
	journalBefore := journal.Total
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatal(err)
	}
	c.must("GET", "/api/mcc?query=2999&limit=10", nil, &mccPage, 200)
	if len(mccPage.Items) != 1 {
		t.Fatal("seed re-run dropped the admin-created MCC 2999")
	}
	var linkCount int
	if err := pool.QueryRow(ctx,
		`select count(*) from bank_category_mcc where bank_category_id = $1`, customID).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Fatalf("seed re-run touched the custom category's links: %d", linkCount)
	}
	c.must("GET", "/api/mcc-changes?limit=5", nil, &journal, 200)
	if journal.Total != journalBefore {
		t.Fatalf("seed re-run wrote journal noise: %d → %d entries", journalBefore, journal.Total)
	}

	// --- cleanup path exercises the delete endpoints ---
	c.must("DELETE", fmt.Sprintf("/api/mcc-changes/%d", created.ID), nil, nil, 204)
	c.must("DELETE", fmt.Sprintf("/api/bank-categories/%d/mcc/2999", customID), nil, nil, 204)
	c.must("DELETE", fmt.Sprintf("/api/bank-categories/%d", customID), nil, nil, 204)
	// 2999 is still referenced by two PoS rows → FK refusal, then clean up.
	if got := c.do("DELETE", "/api/mcc/2999", nil, nil); got != 409 {
		t.Fatalf("delete of referenced MCC: status %d, want 409", got)
	}
	c.must("DELETE", "/api/pos/"+posIDs[0], nil, nil, 204)
	c.must("DELETE", "/api/pos/"+posIDs[1], nil, nil, 204)
	c.must("DELETE", "/api/mcc/2999", nil, nil, 204)
}

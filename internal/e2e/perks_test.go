// TestPerksE2E runs the Привилегии spec's acceptance script
// (docs/specs/perks.md, «Definition of done + E2E»): ownership scoping, the
// two-level quota's nesting rules, the ledger's arithmetic across both levels,
// discrepancy flagging against the bank's own counter, and the two deletes
// (409 while a perk has windows, cascade underneath a window).
package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
)

type perkJSON struct {
	ID       int64  `json:"id"`
	BankID   int32  `json:"bank_id"`
	BankName string `json:"bank_name"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
}

type discrepancyJSON struct {
	Delta      int    `json:"delta"`
	Computed   int    `json:"computed"`
	Bank       int    `json:"bank"`
	ObservedOn string `json:"observed_on"`
}

type quotaJSON struct {
	ID            int64            `json:"id"`
	ParentQuotaID *int64           `json:"parent_quota_id"`
	WindowStart   string           `json:"window_start"`
	WindowEnd     string           `json:"window_end"`
	InitialSize   int              `json:"initial_size"`
	Size          int              `json:"size"`
	Used          int              `json:"used"`
	Remaining     int              `json:"remaining"`
	Discrepancy   *discrepancyJSON `json:"discrepancy"`
	LastSeenOn    *string          `json:"last_seen_on"`
	Active        bool             `json:"active"`
	ClientLabel   *string          `json:"client_label"`
	Children      []quotaJSON      `json:"children"`
}

type overviewClientJSON struct {
	BankClientID int64   `json:"bank_client_id"`
	Label        *string `json:"label"`
	BankName     string  `json:"bank_name"`
	Perks        []struct {
		PerkID int64       `json:"perk_id"`
		Name   string      `json:"name"`
		Unit   string      `json:"unit"`
		Quotas []quotaJSON `json:"quotas"`
	} `json:"perks"`
}

type perkHistoryJSON struct {
	Perk   perkJSON    `json:"perk"`
	Quotas []quotaJSON `json:"quotas"`
	Events []struct {
		ID      int64  `json:"id"`
		QuotaID int64  `json:"quota_id"`
		Kind    string `json:"kind"`
		Qty     int    `json:"qty"`
		Date    string `json:"event_date"`
	} `json:"events"`
	Snapshots []struct {
		ID        int64 `json:"id"`
		QuotaID   int64 `json:"quota_id"`
		Remaining int   `json:"remaining"`
	} `json:"snapshots"`
}

func TestPerksE2E(t *testing.T) {
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
		t.Fatalf("migrations: %v", err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(server.New(server.Config{Pool: pool, AttachmentsDir: t.TempDir(), InsecureCookie: true}))
	defer srv.Close()

	// Windows are derived from the clock so «сейчас» is genuinely inside them
	// whenever the suite runs: an annual pool over this calendar year, a
	// monthly sub-allowance over this month.
	now := time.Now()
	iso := func(d time.Time) string { return d.Format("2006-01-02") }
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, -1)
	today := iso(now)

	anna := newClient(t, srv.URL)
	boris := newClient(t, srv.URL)
	anna.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "anna", "display_name": "Аня", "email": "anna@example.com", "password": "correct horse",
	}, nil, http.StatusCreated)
	boris.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "boris", "display_name": "Боря", "email": "boris@example.com", "password": "correct horse",
	}, nil, http.StatusCreated)

	var banks []struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}
	anna.must("GET", "/api/v1/banks", nil, &banks, http.StatusOK)
	bankID := func(name string) int32 {
		for _, b := range banks {
			if b.Name == name {
				return b.ID
			}
		}
		t.Fatalf("seeded bank %q not found", name)
		return 0
	}
	alfaID, vtbID := bankID("Альфа-Банк"), bankID("ВТБ")

	// --- Step 1: ownership is airtight and non-revealing -------------------

	var alfaClient, vtbClient clientJSON
	anna.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": alfaID, "label": "Мама",
	}, &alfaClient, http.StatusCreated)
	anna.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": vtbID,
	}, &vtbClient, http.StatusCreated)

	var taxi perkJSON
	anna.must("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": alfaClient.ID, "name": "Компенсация такси", "unit": "поездка",
		"note": "до 1 000 ₽ за поездку, заявлять до конца месяца",
	}, &taxi, http.StatusCreated)

	// Boris sees nothing and cannot reach anything of Anna's — a foreign id and
	// a missing one answer identically (spec invariant 1).
	var borisOverview []overviewClientJSON
	boris.must("GET", "/api/v1/perks/overview", nil, &borisOverview, http.StatusOK)
	if len(borisOverview) != 0 {
		t.Fatalf("Boris's overview = %+v, want empty", borisOverview)
	}
	for _, probe := range []struct {
		method, path string
		body         any
	}{
		{"GET", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), nil},
		{"PATCH", fmt.Sprintf("/api/v1/perks/%d", taxi.ID), map[string]any{"name": "Моё"}},
		{"DELETE", fmt.Sprintf("/api/v1/perks/%d", taxi.ID), nil},
		{"POST", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), map[string]any{
			"window_start": iso(yearStart), "window_end": iso(yearEnd), "size": 15,
		}},
		{"GET", "/api/v1/perks/999999/quotas", nil},
	} {
		if got := boris.do(probe.method, probe.path, probe.body, nil); got != http.StatusNotFound {
			t.Fatalf("%s %s as a stranger: status %d, want 404", probe.method, probe.path, got)
		}
	}
	// Boris cannot anchor a perk on Anna's держатель — same 404 as everything
	// else he cannot see.
	if got := boris.do("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": alfaClient.ID, "name": "Компенсация такси", "unit": "поездка",
	}, nil); got != http.StatusNotFound {
		t.Fatalf("perk on a stranger's держатель: status %d, want 404", got)
	}
	// One name per держатель — not per bank. Four accounts at one bank with the
	// same perk are four rows, which is the household this was built from.
	if got := anna.do("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": alfaClient.ID, "name": "Компенсация такси", "unit": "поездка",
	}, nil); got != http.StatusConflict {
		t.Fatalf("duplicate perk name on one держатель: status %d, want 409", got)
	}
	var sameNameElsewhere perkJSON
	anna.must("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": vtbClient.ID, "name": "Компенсация такси", "unit": "поездка",
	}, &sameNameElsewhere, http.StatusCreated)
	if sameNameElsewhere.ID == taxi.ID {
		t.Fatal("the same name on another держатель reused the row")
	}
	anna.must("DELETE", fmt.Sprintf("/api/v1/perks/%d", sameNameElsewhere.ID), nil, nil, http.StatusNoContent)

	// --- Step 2: the two-level quota ---------------------------------------

	var annual quotaJSON
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), map[string]any{
		"window_start": iso(yearStart), "window_end": iso(yearEnd),
		"size": 15, "note": "Alfa Only, сгорает 31.12",
	}, &annual, http.StatusCreated)

	var monthly quotaJSON
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), map[string]any{
		"parent_quota_id": annual.ID,
		"window_start":    iso(monthStart), "window_end": iso(monthEnd), "size": 3,
	}, &monthly, http.StatusCreated)
	if monthly.ParentQuotaID == nil || *monthly.ParentQuotaID != annual.ID {
		t.Fatalf("child quota = %+v, want parent %d", monthly, annual.ID)
	}

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"child reaching outside the pool", map[string]any{
			"parent_quota_id": annual.ID,
			"window_start":    iso(yearStart.AddDate(0, 0, -10)), "window_end": iso(monthEnd), "size": 3,
		}},
		{"child ending after the pool", map[string]any{
			"parent_quota_id": annual.ID,
			"window_start":    iso(monthStart), "window_end": iso(yearEnd.AddDate(0, 0, 1)), "size": 3,
		}},
		{"grandchild", map[string]any{
			"parent_quota_id": monthly.ID,
			"window_start":    iso(monthStart), "window_end": iso(monthEnd), "size": 1,
		}},
		{"reversed window", map[string]any{
			"bank_client_id": alfaClient.ID,
			"window_start":   iso(monthEnd), "window_end": iso(monthStart), "size": 3,
		}},
	} {
		if got := anna.do("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), c.body, nil); got != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status %d, want 422", c.name, got)
		}
	}

	// --- Step 3: the ledger burns both levels ------------------------------

	postEvent := func(quotaID int64, kind string, qty int, on string, note string) {
		t.Helper()
		body := map[string]any{"kind": kind, "qty": qty, "event_date": on}
		if note != "" {
			body["note"] = note
		}
		anna.must("POST", fmt.Sprintf("/api/v1/perks/quotas/%d/events", quotaID), body, nil, http.StatusCreated)
	}

	// PV-S2: two compensated rides this month.
	postEvent(monthly.ID, "use", 1, today, "")
	postEvent(monthly.ID, "use", 1, today, "")

	// The overview is the one read that has to agree with hand math.
	readOverview := func() []overviewClientJSON {
		t.Helper()
		var out []overviewClientJSON
		anna.must("GET", "/api/v1/perks/overview", nil, &out, http.StatusOK)
		return out
	}
	findQuota := func(ov []overviewClientJSON, clientID int64, perkID int64) quotaJSON {
		t.Helper()
		for _, c := range ov {
			if c.BankClientID != clientID {
				continue
			}
			for _, p := range c.Perks {
				if p.PerkID == perkID && len(p.Quotas) > 0 {
					return p.Quotas[0]
				}
			}
		}
		t.Fatalf("no quota for client %d perk %d in %+v", clientID, perkID, ov)
		return quotaJSON{}
	}

	root := findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Remaining != 13 || root.Used != 2 || root.Size != 15 {
		t.Fatalf("annual after 2 uses = %+v, want size 15 used 2 remaining 13 — a leaf use burns the pool", root)
	}
	if len(root.Children) != 1 {
		t.Fatalf("annual children = %d, want the monthly window inline", len(root.Children))
	}
	if ch := root.Children[0]; ch.Remaining != 1 || ch.Used != 2 || ch.Size != 3 {
		t.Fatalf("monthly after 2 uses = %+v, want size 3 used 2 remaining 1", ch)
	}

	// PV-S3's first half: Альфа gifted a ride, so the month renders as x/4 —
	// and the pool is left alone, because whether the bank charged it is not
	// something the app may guess.
	postEvent(monthly.ID, "grant", 1, today, "подарочная поездка, компенсируется первой")
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if ch := root.Children[0]; ch.Size != 4 || ch.Remaining != 2 {
		t.Fatalf("monthly after the gift = %+v, want size 4 remaining 2", ch)
	}
	if root.Size != 15 || root.Remaining != 13 {
		t.Fatalf("annual after the gift = %+v, want size 15 remaining 13 — a grant stays on its own window", root)
	}

	// PV-S5: the pool re-rated mid-year. resize carries the NEW absolute size.
	postEvent(annual.ID, "resize", 12, today, "не выполнил условия")
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Size != 12 || root.Remaining != 10 {
		t.Fatalf("annual after resize 12 = %+v, want size 12 remaining 10", root)
	}

	// Spec invariant 4: nothing is refused for «insufficient quota» — the app
	// mirrors the bank, and a negative remaining is the drift made visible.
	postEvent(monthly.ID, "use", 9, today, "")
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if ch := root.Children[0]; ch.Remaining != -7 {
		t.Fatalf("monthly after overspending = %+v, want remaining −7", ch)
	}
	if root.Remaining != 1 {
		t.Fatalf("annual after overspending = %+v, want remaining 1 (12 − 11)", root)
	}
	// Undo it — the rest of the script reads better from a sane ledger.
	var history perkHistoryJSON
	anna.must("GET", fmt.Sprintf("/api/v1/perks/%d/quotas", taxi.ID), nil, &history, http.StatusOK)
	var bigUse int64
	for _, e := range history.Events {
		if e.Kind == "use" && e.Qty == 9 {
			bigUse = e.ID
		}
	}
	if bigUse == 0 {
		t.Fatalf("history did not carry the qty-9 use: %+v", history.Events)
	}
	anna.must("DELETE", fmt.Sprintf("/api/v1/perks/events/%d", bigUse), nil, nil, http.StatusNoContent)
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Remaining != 10 {
		t.Fatalf("annual after undoing the mistake = %+v, want remaining 10", root)
	}

	// A ledger row's qty is bounded per kind.
	for _, c := range []struct {
		kind string
		qty  int
	}{{"use", 0}, {"use", -1}, {"grant", 0}, {"resize", -1}, {"adjust", 0}} {
		if got := anna.do("POST", fmt.Sprintf("/api/v1/perks/quotas/%d/events", monthly.ID), map[string]any{
			"kind": c.kind, "qty": c.qty, "event_date": today,
		}, nil); got != http.StatusUnprocessableEntity {
			t.Fatalf("%s qty %d: status %d, want 422", c.kind, c.qty, got)
		}
	}

	// --- Step 4: snapshots flag, adjusts close -----------------------------

	postSnapshot := func(quotaID int64, remaining int, on string) {
		t.Helper()
		anna.must("POST", fmt.Sprintf("/api/v1/perks/quotas/%d/snapshots", quotaID), map[string]any{
			"remaining": remaining, "observed_on": on,
		}, nil, http.StatusCreated)
	}

	// PV-S1: the bank agrees. Green, no badge.
	postSnapshot(annual.ID, 10, today)
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Discrepancy != nil {
		t.Fatalf("matching snapshot flagged: %+v", root.Discrepancy)
	}
	if root.LastSeenOn == nil || *root.LastSeenOn != today {
		t.Fatalf("last_seen_on = %v, want %s", root.LastSeenOn, today)
	}

	// PV-S3: the bank says 9 where the ledger says 10.
	postSnapshot(annual.ID, 9, today)
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Discrepancy == nil {
		t.Fatal("mismatching snapshot did not raise the badge")
	}
	if d := root.Discrepancy; d.Delta != -1 || d.Computed != 10 || d.Bank != 9 {
		t.Fatalf("discrepancy = %+v, want delta −1, computed 10, bank 9", d)
	}
	// A reading never writes back (invariant 3): remaining is still the
	// ledger's number, not the bank's.
	if root.Remaining != 10 {
		t.Fatalf("remaining = %d, want 10 — a snapshot does not move the counter", root.Remaining)
	}

	// An adjust closes it, and the note explains the history forever.
	postEvent(annual.ID, "adjust", -1, today, "подарочная поездка списала годовой")
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Discrepancy != nil {
		t.Fatalf("adjust did not close the discrepancy: %+v", root.Discrepancy)
	}
	if root.Remaining != 9 {
		t.Fatalf("remaining after the adjust = %d, want 9", root.Remaining)
	}

	// An older reading does not unflag a newer one: the badge follows the
	// latest snapshot, not the friendliest.
	postSnapshot(annual.ID, 3, today)
	postSnapshot(annual.ID, 9, iso(now.AddDate(0, 0, -5)))
	root = findQuota(readOverview(), alfaClient.ID, taxi.ID)
	if root.Discrepancy == nil || root.Discrepancy.Bank != 3 {
		t.Fatalf("discrepancy = %+v, want the latest reading (3) to hold the badge", root.Discrepancy)
	}
	postSnapshot(annual.ID, 9, today)
	if findQuota(readOverview(), alfaClient.ID, taxi.ID).Discrepancy != nil {
		t.Fatal("a newer matching reading did not clear the badge")
	}

	// The bank's own counter never displays a negative.
	if got := anna.do("POST", fmt.Sprintf("/api/v1/perks/quotas/%d/snapshots", annual.ID), map[string]any{
		"remaining": -1, "observed_on": today,
	}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("negative snapshot: status %d, want 422", got)
	}

	// --- Step 5: size is immutable once history exists, and the deletes ----

	if got := anna.do("PATCH", fmt.Sprintf("/api/v1/perks/quotas/%d", annual.ID), map[string]any{
		"size": 20,
	}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("size patch on a window with history: status %d, want 422", got)
	}
	// The note stays editable regardless.
	anna.must("PATCH", fmt.Sprintf("/api/v1/perks/quotas/%d", annual.ID), map[string]any{
		"set_note": true, "note": "Alfa Only M",
	}, nil, http.StatusOK)
	// A fresh window still takes a typo fix.
	var vtbPerk, vtbQuota quotaJSON
	var prefs perkJSON
	anna.must("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": vtbClient.ID, "name": "Преференции", "unit": "преференция",
	}, &prefs, http.StatusCreated)
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", prefs.ID), map[string]any{
		"window_start": iso(monthStart), "window_end": iso(monthEnd), "size": 20,
	}, &vtbQuota, http.StatusCreated)
	anna.must("PATCH", fmt.Sprintf("/api/v1/perks/quotas/%d", vtbQuota.ID), map[string]any{
		"size": 2,
	}, &vtbPerk, http.StatusOK)
	if vtbPerk.InitialSize != 2 {
		t.Fatalf("size patch on a clean window = %+v, want 2", vtbPerk)
	}

	// --- Editing the definition (the PATCH the detail screen drives) --------

	var edited perkJSON
	anna.must("PATCH", fmt.Sprintf("/api/v1/perks/%d", prefs.ID), map[string]any{
		"name": "  Преференции ВТБ  ", "unit": "балл", "set_note": true, "note": "2 в месяц, сгорают",
	}, &edited, http.StatusOK)
	if edited.Name != "Преференции ВТБ" || edited.Unit != "балл" {
		t.Fatalf("perk patch = %+v, want the trimmed name and the new unit", edited)
	}
	// A field left out is left alone; set_note is what separates «не трогай» from «сотри».
	anna.must("PATCH", fmt.Sprintf("/api/v1/perks/%d", prefs.ID), map[string]any{
		"unit": "преференция",
	}, &edited, http.StatusOK)
	if edited.Name != "Преференции ВТБ" {
		t.Fatalf("omitted name was overwritten: %+v", edited)
	}
	var withNote struct {
		Note *string `json:"note"`
	}
	anna.must("PATCH", fmt.Sprintf("/api/v1/perks/%d", prefs.ID), map[string]any{
		"set_note": true,
	}, &withNote, http.StatusOK)
	if withNote.Note != nil {
		t.Fatalf("note = %v, want cleared by set_note with no value", *withNote.Note)
	}
	// The identity rule survives an edit: one name per bank per user.
	var clash perkJSON
	anna.must("POST", "/api/v1/perks", map[string]any{
		"bank_client_id": vtbClient.ID, "name": "Проходы", "unit": "проход",
	}, &clash, http.StatusCreated)
	if got := anna.do("PATCH", fmt.Sprintf("/api/v1/perks/%d", clash.ID), map[string]any{
		"name": "Преференции ВТБ",
	}, nil); got != http.StatusConflict {
		t.Fatalf("renaming onto a taken name: status %d, want 409", got)
	}
	if got := anna.do("PATCH", fmt.Sprintf("/api/v1/perks/%d", clash.ID), map[string]any{
		"name": "   ",
	}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("blank name: status %d, want 422", got)
	}
	anna.must("DELETE", fmt.Sprintf("/api/v1/perks/%d", clash.ID), nil, nil, http.StatusNoContent)

	// Invariant 6: a perk with windows refuses to go.
	if got := anna.do("DELETE", fmt.Sprintf("/api/v1/perks/%d", taxi.ID), nil, nil); got != http.StatusConflict {
		t.Fatalf("perk delete with quotas: status %d, want 409", got)
	}

	// Deleting the pool takes its ledger, its readings and its month with it.
	var eventsBefore, snapsBefore int
	if err := pool.QueryRow(ctx, "select count(*) from perk_event").Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "select count(*) from perk_snapshot").Scan(&snapsBefore); err != nil {
		t.Fatal(err)
	}
	if eventsBefore == 0 || snapsBefore == 0 {
		t.Fatalf("nothing to cascade: %d events, %d snapshots", eventsBefore, snapsBefore)
	}
	anna.must("DELETE", fmt.Sprintf("/api/v1/perks/quotas/%d", annual.ID), nil, nil, http.StatusNoContent)

	var orphanQuotas, orphanEvents, orphanSnaps int
	if err := pool.QueryRow(ctx, "select count(*) from perk_quota where id = $1 or parent_quota_id = $1", annual.ID).Scan(&orphanQuotas); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "select count(*) from perk_event where quota_id = any($1::bigint[])", []int64{annual.ID, monthly.ID}).Scan(&orphanEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "select count(*) from perk_snapshot where quota_id = any($1::bigint[])", []int64{annual.ID, monthly.ID}).Scan(&orphanSnaps); err != nil {
		t.Fatal(err)
	}
	if orphanQuotas != 0 || orphanEvents != 0 || orphanSnaps != 0 {
		t.Fatalf("cascade left %d quotas, %d events, %d snapshots", orphanQuotas, orphanEvents, orphanSnaps)
	}
	// …and now the perk itself goes.
	anna.must("DELETE", fmt.Sprintf("/api/v1/perks/%d", taxi.ID), nil, nil, http.StatusNoContent)

	// --- Step 6: the overview's shape --------------------------------------

	// ВТБ's monthly window ran 20th→19th until 2026-07-31 — a window that
	// crosses a month boundary is ordinary data, not an edge case.
	var vtbAnnual, vtbMonthly quotaJSON
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", prefs.ID), map[string]any{
		"window_start": iso(yearStart), "window_end": iso(yearEnd), "size": 12,
	}, &vtbAnnual, http.StatusCreated)
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", prefs.ID), map[string]any{
		"parent_quota_id": vtbAnnual.ID,
		"window_start":    iso(monthStart), "window_end": iso(monthEnd), "size": 2,
	}, &vtbMonthly, http.StatusCreated)
	// A window that closed last year must not show up on PV-01.
	anna.must("POST", fmt.Sprintf("/api/v1/perks/%d/quotas", prefs.ID), map[string]any{
		"window_start": iso(yearStart.AddDate(-1, 0, 0)), "window_end": iso(yearEnd.AddDate(-1, 0, 0)), "size": 30,
	}, nil, http.StatusCreated)

	ov := readOverview()
	if len(ov) != 1 || ov[0].BankClientID != vtbClient.ID {
		t.Fatalf("overview = %+v, want only the ВТБ client (Альфа's perk is gone)", ov)
	}
	if ov[0].BankName != "ВТБ" {
		t.Fatalf("bank name = %q, want «ВТБ» — banks render as the bank writes itself", ov[0].BankName)
	}
	if len(ov[0].Perks) != 1 || len(ov[0].Perks[0].Quotas) != 2 {
		t.Fatalf("overview quotas = %+v, want the standalone month and the pool — last year's window is closed", ov[0].Perks)
	}
	var pool2 *quotaJSON
	for i := range ov[0].Perks[0].Quotas {
		if ov[0].Perks[0].Quotas[i].ID == vtbAnnual.ID {
			pool2 = &ov[0].Perks[0].Quotas[i]
		}
	}
	if pool2 == nil || len(pool2.Children) != 1 || pool2.Children[0].ID != vtbMonthly.ID {
		t.Fatalf("the pool did not carry its month inline: %+v", ov[0].Perks[0].Quotas)
	}
	if pool2.Size != 12 || pool2.Remaining != 12 {
		t.Fatalf("untouched pool = %+v, want 12/12", pool2)
	}

	// PV-02 lists every window of the perk — closed ones included — and says
	// which one is running.
	anna.must("GET", fmt.Sprintf("/api/v1/perks/%d/quotas", prefs.ID), nil, &history, http.StatusOK)
	if len(history.Quotas) != 4 {
		t.Fatalf("history windows = %d, want 4 (last year's, the pool, its month, the standalone month)", len(history.Quotas))
	}
	var active, closed int
	for _, q := range history.Quotas {
		if q.Active {
			active++
		} else {
			closed++
		}
	}
	if active != 3 || closed != 1 {
		t.Fatalf("history: %d running, %d closed; want 3 and 1", active, closed)
	}
	if history.Perk.Unit != "преференция" || history.Perk.BankName != "ВТБ" {
		t.Fatalf("history perk = %+v", history.Perk)
	}
}

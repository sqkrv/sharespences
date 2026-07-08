// Package e2e_test runs the cashback spec's acceptance script (its
// «E2E proof» section, steps 1–7) against a real PostGIS container through
// the HTTP API, plus the DoD checks folded in: migrations apply on a fresh
// PostGIS, seed loads, invariant 4 and unique-selection at the API level,
// incremental (S2-style) later additions, and user scoping.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// do sends JSON and decodes JSON, returning the status code.
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if out != nil && len(data) > 0 && resp.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, data, err)
		}
	}
	return resp.StatusCode
}

func (c *client) must(method, path string, body any, out any, wantStatus int) {
	c.t.Helper()
	if got := c.do(method, path, body, out); got != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d", method, path, got, wantStatus)
	}
}

type programJSON struct {
	ID       int64  `json:"id"`
	BankID   int32  `json:"bank_id"`
	BankName string `json:"bank_name"`
}

type tierJSON struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	CapValue      *string `json:"cap_value"`
	MaxCategories *int32  `json:"max_categories"`
}

type cardJSON struct {
	ID int32 `json:"id"`
}

type periodJSON struct {
	ID int64 `json:"id"`
}

type offerJSON struct {
	ID int64 `json:"id"`
}

type suggestionJSON struct {
	Suggestion *struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	} `json:"suggestion"`
}

type helperJSON struct {
	SlotsUsed     int    `json:"slots_used"`
	MaxCategories *int32 `json:"max_categories"`
	Rows          []struct {
		CategoryOfferID int64  `json:"category_offer_id"`
		RawTitle        string `json:"raw_title"`
		Selected        bool   `json:"selected"`
		Collisions      []struct {
			BankName string `json:"bank_name"`
			Message  string `json:"message"`
		} `json:"collisions"`
		Comparisons []struct {
			BankName string  `json:"bank_name"`
			Percent  *string `json:"percent"`
		} `json:"comparisons"`
	} `json:"rows"`
}

type lookupJSON struct {
	Ranked []struct {
		BankName     string  `json:"bank_name"`
		Percent      *string `json:"percent"`
		CurrencyKind string  `json:"currency_kind"`
		CapValue     *string `json:"cap_value"`
	} `json:"ranked"`
	Special []any  `json:"special"`
	Message string `json:"message"`
}

func TestCashbackE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e needs Docker")
	}
	ctx := context.Background()

	pg, err := postgres.Run(ctx, "postgis/postgis:16-3.4",
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

	// DoD: migrations apply on a fresh PostGIS; seed loads; both idempotent.
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatalf("seed rerun (idempotency): %v", err)
	}

	srv := httptest.NewServer(server.New(server.Config{Pool: pool, AttachmentsDir: t.TempDir()}))
	defer srv.Close()

	owner := newClient(t, srv.URL)
	other := newClient(t, srv.URL)

	// Two users; registration signs in (cookie jar per client).
	var me struct {
		ID string `json:"id"`
	}
	owner.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "owner", "display_name": "Owner", "email": "owner@example.com", "password": "correct horse",
	}, &me, http.StatusCreated)
	other.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "other", "display_name": "Other", "email": "other@example.com", "password": "correct horse",
	}, nil, http.StatusCreated)

	// --- E2E step 1: seeded programs/tiers (Альфа-Смарт: 4 slots 7000₽;
	// Озон Стандартный: 4 slots, 1500/cat + 3000 total) ---
	var programs []programJSON
	owner.must("GET", "/api/v1/cashback/programs", nil, &programs, http.StatusOK)
	if len(programs) != 6 {
		t.Fatalf("seeded programs = %d, want 6", len(programs))
	}
	findProgram := func(bank string) programJSON {
		for _, p := range programs {
			if p.BankName == bank {
				return p
			}
		}
		t.Fatalf("no seeded program for %s", bank)
		return programJSON{}
	}
	alfa := findProgram("Альфа-Банк")
	ozon := findProgram("Озон Банк")

	findTier := func(programID int64, name string) tierJSON {
		var tiers []tierJSON
		owner.must("GET", fmt.Sprintf("/api/v1/cashback/programs/%d/tiers", programID), nil, &tiers, http.StatusOK)
		for _, tr := range tiers {
			if tr.Name == name {
				return tr
			}
		}
		t.Fatalf("tier %q not found in program %d", name, programID)
		return tierJSON{}
	}
	alfaSmart := findTier(alfa.ID, "Альфа-Смарт")
	ozonStd := findTier(ozon.ID, "Стандартный")
	if alfaSmart.CapValue == nil || *alfaSmart.CapValue != "7000" || alfaSmart.MaxCategories == nil || *alfaSmart.MaxCategories != 4 {
		t.Fatalf("Альфа-Смарт seed wrong: %+v", alfaSmart)
	}

	var alfaCard, ozonCard cardJSON
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_id": alfa.BankID, "last_4_digits": 1234, "payment_system": "mir", "program_tier_id": alfaSmart.ID,
	}, &alfaCard, http.StatusCreated)
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_id": ozon.BankID, "last_4_digits": 5678, "payment_system": "mir", "program_tier_id": ozonStd.ID,
	}, &ozonCard, http.StatusCreated)

	// --- E2E step 2: July periods + menus, alias Продукты→supermarkets ---
	var alfaPeriod, ozonPeriod periodJSON
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"card_id": alfaCard.ID, "period_start": "2026-07-01", "period_end": "2026-07-31",
	}, &alfaPeriod, http.StatusCreated)
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"card_id": ozonCard.ID, "period_start": "2026-07-01", "period_end": "2026-07-31",
	}, &ozonPeriod, http.StatusCreated)

	// Invariant 4 at the API: overlapping period on the same card → 409.
	if got := owner.do("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"card_id": ozonCard.ID, "period_start": "2026-07-15", "period_end": "2026-08-15",
	}, nil); got != http.StatusConflict {
		t.Fatalf("overlapping period: status %d, want 409", got)
	}

	// The seeded alias table pre-suggests canonical categories (S1).
	suggest := func(periodID int64, raw string) int64 {
		var s suggestionJSON
		owner.must("GET", "/api/v1/cashback/alias-suggestion?offer_period_id="+
			fmt.Sprint(periodID)+"&raw_title="+url.QueryEscape(raw), nil, &s, http.StatusOK)
		if s.Suggestion == nil {
			t.Fatalf("no alias suggestion for %q", raw)
		}
		return s.Suggestion.ID
	}
	supermarketsID := suggest(ozonPeriod.ID, "Продукты") // Озон «Продукты» → supermarkets
	if altID := suggest(alfaPeriod.ID, "Супермаркеты"); altID != supermarketsID {
		t.Fatalf("Супермаркеты (Альфа-Банк) → %d, Продукты (Озон) → %d: want same canonical", altID, supermarketsID)
	}

	addOffer := func(periodID int64, raw, percent string, canonical *int64) offerJSON {
		body := map[string]any{"offer_period_id": periodID, "raw_title": raw, "percent": percent}
		if canonical != nil {
			body["canonical_category_id"] = *canonical
		}
		var o offerJSON
		owner.must("POST", "/api/v1/cashback/category-offers", body, &o, http.StatusCreated)
		return o
	}
	gasID := suggest(alfaPeriod.ID, "АЗС")
	restID := suggest(alfaPeriod.ID, "Рестораны")
	pharmID := suggest(ozonPeriod.ID, "Аптеки")

	alfaSuper := addOffer(alfaPeriod.ID, "Супермаркеты", "5", &supermarketsID)
	addOffer(alfaPeriod.ID, "Рестораны", "5", &restID)
	alfaGas := addOffer(alfaPeriod.ID, "АЗС", "5", &gasID)

	ozonProducts := addOffer(ozonPeriod.ID, "Продукты", "5", &supermarketsID)
	ozonPharm := addOffer(ozonPeriod.ID, "Аптеки", "3", &pharmID)
	ozonFast := addOffer(ozonPeriod.ID, "Фастфуд", "5", nil)
	ozonCafe := addOffer(ozonPeriod.ID, "Кафе и Рестораны", "5", nil)
	ozonClothes := addOffer(ozonPeriod.ID, "Одежда", "2", nil)

	sel := func(offerID int64, at string) int {
		return owner.do("POST", "/api/v1/cashback/selections",
			map[string]any{"category_offer_id": offerID, "selected_at": at}, nil)
	}
	const july10 = "2026-07-10T12:00:00Z"

	// --- E2E step 3: Супермаркеты on Альфа-Банк, then Продукты on Озон:
	// warning in helper-context, both selections persist ---
	if got := sel(alfaSuper.ID, july10); got != http.StatusCreated {
		t.Fatalf("select Супермаркеты (Альфа-Банк): %d", got)
	}
	if got := sel(alfaGas.ID, july10); got != http.StatusCreated {
		t.Fatalf("select АЗС (Альфа-Банк): %d", got)
	}
	if got := sel(ozonProducts.ID, july10); got != http.StatusCreated {
		t.Fatalf("select Продукты (Озон): warning must not block, got %d", got)
	}

	var helper helperJSON
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/helper-context?offer_period_id=%d", ozonPeriod.ID), nil, &helper, http.StatusOK)
	var productsRow *struct {
		CategoryOfferID int64  `json:"category_offer_id"`
		RawTitle        string `json:"raw_title"`
		Selected        bool   `json:"selected"`
		Collisions      []struct {
			BankName string `json:"bank_name"`
			Message  string `json:"message"`
		} `json:"collisions"`
		Comparisons []struct {
			BankName string  `json:"bank_name"`
			Percent  *string `json:"percent"`
		} `json:"comparisons"`
	}
	for i := range helper.Rows {
		if helper.Rows[i].CategoryOfferID == ozonProducts.ID {
			productsRow = &helper.Rows[i]
		}
	}
	if productsRow == nil {
		t.Fatal("helper-context misses the Продукты row")
	}
	if len(productsRow.Collisions) != 1 || productsRow.Collisions[0].BankName != "Альфа-Банк" {
		t.Fatalf("Продукты collisions = %+v, want exactly one naming Альфа-Банк", productsRow.Collisions)
	}
	t.Logf("collision warning: %s", productsRow.Collisions[0].Message)
	if !productsRow.Selected {
		t.Fatal("Продукты must stay selected — warning, never a block (invariant 3)")
	}
	if helper.MaxCategories == nil || *helper.MaxCategories != 4 || helper.SlotsUsed != 1 {
		t.Fatalf("slots = %d/%v, want 1/4", helper.SlotsUsed, helper.MaxCategories)
	}

	// Unique selection per offer (schema UNIQUE + domain check).
	if got := sel(alfaSuper.ID, july10); got != http.StatusConflict {
		t.Fatalf("re-selecting the same offer: %d, want 409", got)
	}

	// S2-style incremental addition: a row entered later into an existing
	// period, selected with its own (later) date.
	transport := addOffer(alfaPeriod.ID, "Транспорт", "7", nil)
	if got := sel(transport.ID, "2026-07-20T09:00:00Z"); got != http.StatusCreated {
		t.Fatalf("incremental late selection: %d", got)
	}

	// --- E2E step 4: 5th regular selection on Озон (4 slots) → hard reject ---
	if got := sel(ozonPharm.ID, july10); got != http.StatusCreated {
		t.Fatalf("Озон 2nd selection: %d", got)
	}
	if got := sel(ozonFast.ID, july10); got != http.StatusCreated {
		t.Fatalf("Озон 3rd selection: %d", got)
	}
	if got := sel(ozonCafe.ID, july10); got != http.StatusCreated {
		t.Fatalf("Озон 4th selection: %d", got)
	}
	if got := sel(ozonClothes.ID, july10); got != http.StatusConflict {
		t.Fatalf("Озон 5th selection: %d, want 409 (invariant 1 hard reject)", got)
	}

	// Owner feedback 2026-07-04: slot counts vary per period — the override
	// raises the effective limit and the 5th selection then goes through.
	owner.must("PUT", fmt.Sprintf("/api/v1/cashback/offer-periods/%d/max-categories", ozonPeriod.ID),
		map[string]any{"value": 5}, nil, http.StatusOK)
	if got := sel(ozonClothes.ID, july10); got != http.StatusCreated {
		t.Fatalf("Озон 5th selection with override=5: %d, want 201", got)
	}
	var ozonHelper helperJSON
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/helper-context?offer_period_id=%d", ozonPeriod.ID), nil, &ozonHelper, http.StatusOK)
	if ozonHelper.MaxCategories == nil || *ozonHelper.MaxCategories != 5 || ozonHelper.SlotsUsed != 5 {
		t.Fatalf("helper after override: %d/%v, want 5/5", ozonHelper.SlotsUsed, ozonHelper.MaxCategories)
	}

	// Owner feedback 2026-07-04: entered rows must be deletable (with their
	// selection); the slot frees up.
	owner.must("DELETE", fmt.Sprintf("/api/v1/cashback/category-offers/%d", ozonClothes.ID), nil, nil, http.StatusNoContent)
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/helper-context?offer_period_id=%d", ozonPeriod.ID), nil, &ozonHelper, http.StatusOK)
	if ozonHelper.SlotsUsed != 4 {
		t.Fatalf("slots after deleting a selected row = %d, want 4", ozonHelper.SlotsUsed)
	}

	// Invariant 2: selection dated outside the period → 422.
	augustOffer := addOffer(alfaPeriod.ID, "Цветы", "5", nil)
	if got := sel(augustOffer.ID, "2026-08-05T10:00:00Z"); got != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-period selection: %d, want 422", got)
	}

	// Regression (owner report 2026-07-04, «Какой картой?» came back empty):
	// a row entered WITHOUT a canonical mapping is invisible to lookup;
	// editing the row to map it (+ selecting) makes it appear.
	var lookup lookupJSON
	owner.must("GET", "/api/v1/cashback/lookup?category=flowers&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 0 {
		t.Fatalf("unmapped+unselected Цветы must not rank, got %+v", lookup.Ranked)
	}
	var cats []struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	owner.must("GET", "/api/v1/cashback/canonical-categories", nil, &cats, http.StatusOK)
	var flowersID int64
	for _, c := range cats {
		if c.Slug == "flowers" {
			flowersID = c.ID
		}
	}
	owner.must("PUT", fmt.Sprintf("/api/v1/cashback/category-offers/%d", augustOffer.ID),
		map[string]any{"raw_title": "Цветы", "percent": "5", "canonical_category_id": flowersID, "kind": "regular"},
		nil, http.StatusOK)
	if got := sel(augustOffer.ID, july10); got != http.StatusCreated {
		t.Fatalf("selecting the corrected Цветы row: %d, want 201", got)
	}
	owner.must("GET", "/api/v1/cashback/lookup?category=flowers&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 1 || lookup.Ranked[0].BankName != "Альфа-Банк" {
		t.Fatalf("flowers after mapping+selecting = %+v, want Альфа-Банк ranked", lookup.Ranked)
	}

	// Overview (design screens 01/02): both cuts in one response.
	var banks []struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}
	owner.must("GET", "/api/v1/banks", nil, &banks, http.StatusOK)
	var vtbID int32
	for _, b := range banks {
		if b.Name == "ВТБ" {
			vtbID = b.ID
		}
	}
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_id": vtbID, "last_4_digits": 9012, "payment_system": "mir",
	}, nil, http.StatusCreated)

	var overview struct {
		Categories []struct {
			Slug        string `json:"slug"`
			OthersCount int    `json:"others_count"`
			Best        struct {
				BankName string `json:"bank_name"`
			} `json:"best"`
		} `json:"categories"`
		Cards []struct {
			BankName      string  `json:"bank_name"`
			PeriodID      *int64  `json:"period_id"`
			SlotsUsed     int     `json:"slots_used"`
			MaxCategories *int32  `json:"max_categories"`
			TierName      *string `json:"tier_name"`
		} `json:"cards"`
		SelectionOpensDay *int32 `json:"selection_opens_day"`
	}
	owner.must("GET", "/api/v1/cashback/overview?date=2026-07-15", nil, &overview, http.StatusOK)
	// Транспорт is selected but unmapped → invisible here, like in lookup.
	if len(overview.Categories) != 4 {
		t.Fatalf("overview categories = %d, want 4 (supermarkets, gas-stations, pharmacies, flowers)", len(overview.Categories))
	}
	var superRow *struct {
		Slug        string `json:"slug"`
		OthersCount int    `json:"others_count"`
		Best        struct {
			BankName string `json:"bank_name"`
		} `json:"best"`
	}
	for i := range overview.Categories {
		if overview.Categories[i].Slug == "supermarkets" {
			superRow = &overview.Categories[i]
		}
	}
	if superRow == nil || superRow.Best.BankName != "Альфа-Банк" || superRow.OthersCount != 1 {
		t.Fatalf("overview supermarkets = %+v, want best Альфа-Банк with 1 other", superRow)
	}
	if len(overview.Cards) != 3 {
		t.Fatalf("overview cards = %d, want 3", len(overview.Cards))
	}
	for _, c := range overview.Cards {
		switch c.BankName {
		case "Альфа-Банк":
			if c.PeriodID == nil || c.SlotsUsed != 4 || c.MaxCategories == nil || *c.MaxCategories != 4 {
				t.Fatalf("overview Альфа-Банк = %+v, want active period 4/4", c)
			}
		case "Озон Банк":
			if c.SlotsUsed != 4 || c.MaxCategories == nil || *c.MaxCategories != 5 {
				t.Fatalf("overview Озон = %+v, want 4/5 (override)", c)
			}
		case "ВТБ":
			if c.PeriodID != nil || c.TierName != nil {
				t.Fatalf("overview ВТБ = %+v, want no period, no tier", c)
			}
		}
	}
	if overview.SelectionOpensDay == nil || *overview.SelectionOpensDay != 25 {
		t.Fatalf("selection_opens_day = %v, want 25", overview.SelectionOpensDay)
	}

	// A whole mistaken period can be deleted with everything under it.
	var scratch periodJSON
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"card_id": alfaCard.ID, "period_start": "2026-08-01", "period_end": "2026-08-31",
	}, &scratch, http.StatusCreated)
	scratchOffer := addOffer(scratch.ID, "Книги", "3", nil)
	if got := sel(scratchOffer.ID, "2026-08-10T10:00:00Z"); got != http.StatusCreated {
		t.Fatalf("scratch selection: %d", got)
	}
	owner.must("DELETE", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", scratch.ID), nil, nil, http.StatusNoContent)
	if got := owner.do("GET", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", scratch.ID), nil, nil); got != http.StatusNotFound {
		t.Fatalf("deleted period still readable: %d, want 404", got)
	}

	// --- E2E step 5: lookup supermarkets on July 15 → both cards ranked
	// (same currency), Альфа-Банк with its static 7000₽ cap ---
	owner.must("GET", "/api/v1/cashback/lookup?category=supermarkets&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 2 {
		t.Fatalf("supermarkets ranked = %d entries, want 2 (got %+v)", len(lookup.Ranked), lookup.Ranked)
	}
	names := map[string]bool{}
	for _, e := range lookup.Ranked {
		names[e.BankName] = true
		if e.CurrencyKind != "rub" {
			t.Fatalf("ranked entry %s currency %s, want rub", e.BankName, e.CurrencyKind)
		}
		if e.BankName == "Альфа-Банк" && (e.CapValue == nil || *e.CapValue != "7000") {
			t.Fatalf("Альфа-Банк cap = %v, want static 7000", e.CapValue)
		}
	}
	if !names["Альфа-Банк"] || !names["Озон Банк"] {
		t.Fatalf("ranked banks = %v, want Альфа-Банк + Озон Банк", names)
	}

	// --- E2E step 6: АЗС → only Альфа-Банк; Такси → «нет активных выборов» ---
	owner.must("GET", "/api/v1/cashback/lookup?category=gas-stations&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 1 || lookup.Ranked[0].BankName != "Альфа-Банк" {
		t.Fatalf("gas-stations ranked = %+v, want only Альфа-Банк", lookup.Ranked)
	}
	owner.must("GET", "/api/v1/cashback/lookup?category=taxi&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 0 || lookup.Message != "нет активных выборов" {
		t.Fatalf("taxi lookup = %+v %q, want empty + «нет активных выборов»", lookup.Ranked, lookup.Message)
	}

	// --- E2E step 7: the same lookup as another user → empty (scoping) ---
	var otherLookup lookupJSON
	other.must("GET", "/api/v1/cashback/lookup?category=supermarkets&date=2026-07-15", nil, &otherLookup, http.StatusOK)
	if len(otherLookup.Ranked) != 0 || len(otherLookup.Special) != 0 {
		t.Fatalf("user B sees %d ranked entries, want 0 (auth scoping)", len(otherLookup.Ranked))
	}
	// …and user B cannot read user A's period at all.
	if got := other.do("GET", fmt.Sprintf("/api/v1/cashback/helper-context?offer_period_id=%d", ozonPeriod.ID), nil, nil); got != http.StatusNotFound {
		t.Fatalf("user B reading user A's helper-context: %d, want 404", got)
	}
	// Unauthenticated requests are rejected outright.
	anon := newClient(t, srv.URL)
	if got := anon.do("GET", "/api/v1/cashback/lookup?category=supermarkets", nil, nil); got != http.StatusUnauthorized {
		t.Fatalf("anonymous lookup: %d, want 401", got)
	}
}

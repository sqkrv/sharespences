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
	"mime/multipart"
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

// upload posts a file through the multipart attachments endpoint.
func (c *client) upload(filename string, content []byte) string {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		c.t.Fatal(err)
	}
	_ = mw.Close()
	req, err := http.NewRequest("POST", c.base+"/api/v1/attachments", &buf)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		c.t.Fatalf("upload: status %d: %s", resp.StatusCode, data)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.t.Fatal(err)
	}
	return out.ID
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

type clientJSON struct {
	ID int64 `json:"id"`
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
		HolderLabel  string  `json:"holder_label"`
		Percent      *string `json:"percent"`
		CurrencyKind string  `json:"currency_kind"`
		CapValue     *string `json:"cap_value"`
	} `json:"ranked"`
	Special  []any `json:"special"`
	Fallback []struct {
		BankName string  `json:"bank_name"`
		Percent  *string `json:"percent"`
	} `json:"fallback"`
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

	// Bank clients (person × bank) own держатель + tier; cards hang off them.
	var alfaClient, ozonClient clientJSON
	owner.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": alfa.BankID, "label": "Мама", "program_tier_id": alfaSmart.ID,
	}, &alfaClient, http.StatusCreated)
	owner.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": ozon.BankID, "program_tier_id": ozonStd.ID,
	}, &ozonClient, http.StatusCreated)

	// One self-relationship per (user, bank): a second unlabeled Озон client
	// is rejected (unique nulls not distinct).
	if got := owner.do("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": ozon.BankID,
	}, nil); got != http.StatusConflict {
		t.Fatalf("duplicate unlabeled bank client: status %d, want 409", got)
	}

	// Мама's plastic at Альфа-Банк, and TWO plastics of the owner's own Озон
	// client — they will share one period/selection set (the re-keying's point).
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": alfaClient.ID, "last_4_digits": 1234, "payment_system": "mir",
	}, nil, http.StatusCreated)
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": ozonClient.ID, "last_4_digits": 5678, "payment_system": "mir",
	}, nil, http.StatusCreated)
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": ozonClient.ID, "last_4_digits": 9012, "payment_system": "mir",
	}, nil, http.StatusCreated)

	// A card cannot be attached to another user's client (scoping → 404).
	if got := other.do("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": alfaClient.ID, "last_4_digits": 1111, "payment_system": "mir",
	}, nil); got != http.StatusNotFound {
		t.Fatalf("card on a foreign bank client: status %d, want 404", got)
	}

	// --- E2E step 2: July periods + menus, alias Продукты→supermarkets ---
	var alfaPeriod, ozonPeriod periodJSON
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"bank_client_id": alfaClient.ID, "period_start": "2026-07-01", "period_end": "2026-07-31",
	}, &alfaPeriod, http.StatusCreated)
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"bank_client_id": ozonClient.ID, "period_start": "2026-07-01", "period_end": "2026-07-31",
	}, &ozonPeriod, http.StatusCreated)

	// Invariant 4 at the API: overlapping period on the same CLIENT → 409,
	// even though the client has a second card — two plastics of one client
	// share one period/selection set, never two.
	if got := owner.do("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"bank_client_id": ozonClient.ID, "period_start": "2026-07-15", "period_end": "2026-08-15",
	}, nil); got != http.StatusConflict {
		t.Fatalf("overlapping period: status %d, want 409", got)
	}

	// Regression (owner bug report 2026-07-22): a freshly created period has
	// no menu rows yet — the overview must still show it on the client card
	// (it used to derive periods from the offers join, so an empty period
	// was invisible while re-creation 409'd on overlap).
	var freshOverview struct {
		Clients []struct {
			BankName string `json:"bank_name"`
			PeriodID *int64 `json:"period_id"`
		} `json:"clients"`
	}
	owner.must("GET", "/api/v1/cashback/overview?date=2026-07-15", nil, &freshOverview, http.StatusOK)
	for _, c := range freshOverview.Clients {
		if (c.BankName == "Альфа-Банк" || c.BankName == "Озон Банк") && c.PeriodID == nil {
			t.Fatalf("empty (offer-less) period invisible on overview for %s", c.BankName)
		}
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
	var vtbClient clientJSON
	owner.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": vtbID,
	}, &vtbClient, http.StatusCreated)
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": vtbClient.ID, "last_4_digits": 9013, "payment_system": "mir",
	}, nil, http.StatusCreated)

	var overview struct {
		Categories []struct {
			Slug        string `json:"slug"`
			OthersCount int    `json:"others_count"`
			Best        struct {
				BankName string `json:"bank_name"`
			} `json:"best"`
		} `json:"categories"`
		Clients []struct {
			BankName      string  `json:"bank_name"`
			HolderLabel   *string `json:"holder_label"`
			PeriodID      *int64  `json:"period_id"`
			SlotsUsed     int     `json:"slots_used"`
			MaxCategories *int32  `json:"max_categories"`
			TierName      *string `json:"tier_name"`
			Cards         []struct {
				Last4Digits int32 `json:"last_4_digits"`
			} `json:"cards"`
		} `json:"clients"`
		Base *struct {
			Best struct {
				BankName string  `json:"bank_name"`
				Percent  *string `json:"percent"`
			} `json:"best"`
		} `json:"base"`
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
	if len(overview.Clients) != 3 {
		t.Fatalf("overview clients = %d, want 3", len(overview.Clients))
	}
	for _, c := range overview.Clients {
		switch c.BankName {
		case "Альфа-Банк":
			if c.PeriodID == nil || c.SlotsUsed != 4 || c.MaxCategories == nil || *c.MaxCategories != 4 {
				t.Fatalf("overview Альфа-Банк = %+v, want active period 4/4", c)
			}
			if c.HolderLabel == nil || *c.HolderLabel != "Мама" {
				t.Fatalf("overview Альфа-Банк holder = %v, want Мама", c.HolderLabel)
			}
		case "Озон Банк":
			if c.SlotsUsed != 4 || c.MaxCategories == nil || *c.MaxCategories != 5 {
				t.Fatalf("overview Озон = %+v, want 4/5 (override)", c)
			}
			// Both plastics of the client hang off ONE row sharing ONE
			// period/slot count — the re-keying's core claim.
			if len(c.Cards) != 2 {
				t.Fatalf("overview Озон cards = %d, want 2 plastics on one client", len(c.Cards))
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

	// Screenshots are editable after creation (owner 2026-07-09): upload →
	// attach to an existing period → visible → detach → gone (row and file).
	attID := owner.upload("menu.png", []byte("fake-png-bytes"))
	owner.must("POST", fmt.Sprintf("/api/v1/cashback/offer-periods/%d/attachments", alfaPeriod.ID),
		map[string]any{"attachment_id": attID}, nil, http.StatusNoContent)
	var periodDetail struct {
		AttachmentIDs []string `json:"attachment_ids"`
	}
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", alfaPeriod.ID), nil, &periodDetail, http.StatusOK)
	if len(periodDetail.AttachmentIDs) != 1 || periodDetail.AttachmentIDs[0] != attID {
		t.Fatalf("period attachments = %v, want [%s]", periodDetail.AttachmentIDs, attID)
	}
	owner.must("DELETE", fmt.Sprintf("/api/v1/cashback/offer-periods/%d/attachments/%s", alfaPeriod.ID, attID), nil, nil, http.StatusNoContent)
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", alfaPeriod.ID), nil, &periodDetail, http.StatusOK)
	if len(periodDetail.AttachmentIDs) != 0 {
		t.Fatalf("period attachments after detach = %v, want empty", periodDetail.AttachmentIDs)
	}
	// Orphaned attachment row is gone: raw content endpoint 404s.
	if got := owner.do("GET", "/api/v1/attachments/"+attID+"/content", nil, nil); got != http.StatusNotFound {
		t.Fatalf("orphaned attachment content: status %d, want 404", got)
	}

	// «За все покупки» (owner correction 2026-07-09): an ORDINARY selectable
	// category — it consumes a slot like any other; its only quirk is that it
	// pays when no other selected category matches, which the lookup shows as
	// the fallback section and the overview as «Остальное».
	var allPurchasesID int64
	for _, c := range cats {
		if c.Slug == "all-purchases" {
			allPurchasesID = c.ID
		}
	}
	baseOffer := struct {
		ID int64 `json:"id"`
	}{}
	owner.must("POST", "/api/v1/cashback/category-offers", map[string]any{
		"offer_period_id": alfaPeriod.ID, "raw_title": "За все покупки", "percent": "1",
		"canonical_category_id": allPurchasesID, "kind": "regular",
	}, &baseOffer, http.StatusCreated)
	if got := sel(baseOffer.ID, july10); got != http.StatusConflict {
		t.Fatalf("«За все покупки» at full slots: %d, want 409 — it takes a slot like any category", got)
	}
	// Free a slot (drop the unmapped Транспорт row with its selection)…
	owner.must("DELETE", fmt.Sprintf("/api/v1/cashback/category-offers/%d", transport.ID), nil, nil, http.StatusNoContent)
	// …now it selects fine.
	if got := sel(baseOffer.ID, july10); got != http.StatusCreated {
		t.Fatalf("«За все покупки» after freeing a slot: %d, want 201", got)
	}
	owner.must("GET", "/api/v1/cashback/lookup?category=taxi&date=2026-07-15", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 0 || lookup.Message != "нет активных выборов" {
		t.Fatalf("taxi after base row: ranked=%d msg=%q, want empty + message", len(lookup.Ranked), lookup.Message)
	}
	if len(lookup.Fallback) != 1 || lookup.Fallback[0].BankName != "Альфа-Банк" || *lookup.Fallback[0].Percent != "1" {
		t.Fatalf("taxi fallback = %+v, want Альфа-Банк 1%% base", lookup.Fallback)
	}
	owner.must("GET", "/api/v1/cashback/overview?date=2026-07-15", nil, &overview, http.StatusOK)
	if overview.Base == nil || overview.Base.Best.BankName != "Альфа-Банк" || *overview.Base.Best.Percent != "1" {
		t.Fatalf("overview base = %+v, want Альфа-Банк 1%%", overview.Base)
	}
	if len(overview.Categories) != 4 {
		t.Fatalf("base row must not appear among categories, got %d", len(overview.Categories))
	}

	// Держатель is editable on the client (PUT /bank-clients/{id}).
	owner.must("PUT", fmt.Sprintf("/api/v1/bank-clients/%d", ozonClient.ID),
		map[string]any{"label": "Стас", "program_tier_id": ozonStd.ID}, nil, http.StatusOK)
	owner.must("GET", "/api/v1/cashback/overview?date=2026-07-15", nil, &overview, http.StatusOK)
	for _, c := range overview.Clients {
		if c.BankName == "Озон Банк" && (c.HolderLabel == nil || *c.HolderLabel != "Стас") {
			t.Fatalf("Озон holder after PUT = %v, want Стас", c.HolderLabel)
		}
	}

	// A whole mistaken period can be deleted with everything under it.
	var scratch periodJSON
	owner.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
		"bank_client_id": alfaClient.ID, "period_start": "2026-08-01", "period_end": "2026-08-31",
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
		if e.BankName == "Альфа-Банк" && e.HolderLabel != "Мама" {
			t.Fatalf("Альфа-Банк holder = %q, want Мама (whose plastic to pull out)", e.HolderLabel)
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

	// --- Picker catalogs (redesign 2026-07-16): seeded bank_category rows
	// with resolved emoji, custom rows, the offer↔catalog FK, brand colors ---

	// Brand colors are seeded and exposed on bank-list.
	var banksWithColor []struct {
		ID       int32   `json:"id"`
		Name     string  `json:"name"`
		ColorHex *string `json:"color_hex"`
	}
	owner.must("GET", "/api/v1/banks", nil, &banksWithColor, http.StatusOK)
	for _, b := range banksWithColor {
		if b.Name == "Альфа-Банк" && (b.ColorHex == nil || *b.ColorHex != "#EF3124") {
			t.Fatalf("Альфа-Банк color_hex = %v, want #EF3124", b.ColorHex)
		}
	}

	// Canonical categories carry the seeded emoji.
	var catsWithEmoji []struct {
		Slug  string  `json:"slug"`
		Emoji *string `json:"emoji"`
	}
	owner.must("GET", "/api/v1/cashback/canonical-categories", nil, &catsWithEmoji, http.StatusOK)
	for _, c := range catsWithEmoji {
		if c.Slug == "supermarkets" && (c.Emoji == nil || *c.Emoji != "🛒") {
			t.Fatalf("supermarkets emoji = %v, want 🛒", c.Emoji)
		}
	}

	// The period detail exposes its bank (the SPA fetches that bank's catalog).
	var periodBank struct {
		BankID int32 `json:"bank_id"`
	}
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", alfaPeriod.ID), nil, &periodBank, http.StatusOK)
	if periodBank.BankID != alfa.BankID {
		t.Fatalf("period bank_id = %d, want %d", periodBank.BankID, alfa.BankID)
	}

	type bankCategoryJSON struct {
		ID                  int64   `json:"id"`
		Title               string  `json:"title"`
		CanonicalCategoryID *int64  `json:"canonical_category_id"`
		CanonicalTitleRu    *string `json:"canonical_title_ru"`
		Kind                string  `json:"kind"`
		Emoji               *string `json:"emoji"`
		IsCustom            bool    `json:"is_custom"`
	}
	var alfaCatalog []bankCategoryJSON
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/banks/%d/categories", alfa.BankID), nil, &alfaCatalog, http.StatusOK)
	findCatalogRow := func(title string) bankCategoryJSON {
		for _, r := range alfaCatalog {
			if r.Title == title {
				return r
			}
		}
		t.Fatalf("Альфа-Банк catalog misses %q (%d rows)", title, len(alfaCatalog))
		return bankCategoryJSON{}
	}
	// A regular row inherits the canonical's emoji and carries the mapping.
	cafe := findCatalogRow("Кафе и рестораны")
	if cafe.CanonicalCategoryID == nil || cafe.CanonicalTitleRu == nil || cafe.Emoji == nil || *cafe.Emoji != "🍽️" || cafe.Kind != "regular" {
		t.Fatalf("catalog «Кафе и рестораны» = %+v, want mapped regular with inherited 🍽️", cafe)
	}
	// A service row has no canonical and its own emoji override — but it is
	// an ORDINARY regular category (owner correction 2026-07-21: special is
	// reserved for granted bonus mechanics, never catalog rows). This is the
	// whole reason bank_category exists next to bank_category_alias.
	trevel := findCatalogRow("Альфа-Тревел")
	if trevel.CanonicalCategoryID != nil || trevel.Kind != "regular" || trevel.Emoji == nil || *trevel.Emoji != "🧳" {
		t.Fatalf("catalog «Альфа-Тревел» = %+v, want canonical-less REGULAR with 🧳", trevel)
	}

	// Custom escape hatch: a new bank category, unmapped; duplicate → 409.
	var custom bankCategoryJSON
	owner.must("POST", "/api/v1/cashback/bank-categories", map[string]any{
		"bank_id": alfa.BankID, "title": "Кофейни", "emoji": "☕",
	}, &custom, http.StatusCreated)
	if !custom.IsCustom || custom.Emoji == nil || *custom.Emoji != "☕" {
		t.Fatalf("custom row = %+v, want is_custom with ☕", custom)
	}
	if got := owner.do("POST", "/api/v1/cashback/bank-categories", map[string]any{
		"bank_id": alfa.BankID, "title": "Кофейни",
	}, nil); got != http.StatusConflict {
		t.Fatalf("duplicate custom bank category: %d, want 409", got)
	}

	// An offer picked from the catalog carries the traceability FK…
	var pickedOffer struct {
		ID             int64  `json:"id"`
		RawTitle       string `json:"raw_title"`
		BankCategoryID *int64 `json:"bank_category_id"`
	}
	owner.must("POST", "/api/v1/cashback/category-offers", map[string]any{
		"offer_period_id": alfaPeriod.ID, "raw_title": custom.Title, "bank_category_id": custom.ID, "percent": "7",
	}, &pickedOffer, http.StatusCreated)
	if pickedOffer.BankCategoryID == nil || *pickedOffer.BankCategoryID != custom.ID {
		t.Fatalf("offer bank_category_id = %v, want %d", pickedOffer.BankCategoryID, custom.ID)
	}
	// …but another bank's catalog row is rejected (integrity, 422).
	var ozonCatalog []bankCategoryJSON
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/banks/%d/categories", ozon.BankID), nil, &ozonCatalog, http.StatusOK)
	if len(ozonCatalog) == 0 {
		t.Fatal("Озон Банк catalog is empty — seed missing")
	}
	if got := owner.do("POST", "/api/v1/cashback/category-offers", map[string]any{
		"offer_period_id": alfaPeriod.ID, "raw_title": "чужая", "bank_category_id": ozonCatalog[0].ID,
	}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("offer with another bank's catalog row: %d, want 422", got)
	}

	// Deleting a catalog row never touches user history: the FK nulls out,
	// the raw_title snapshot survives (on delete set null).
	if _, err := pool.Exec(ctx, "delete from bank_category where id = $1", custom.ID); err != nil {
		t.Fatalf("delete catalog row: %v", err)
	}
	var periodAfter struct {
		Offers []struct {
			ID             int64  `json:"id"`
			RawTitle       string `json:"raw_title"`
			BankCategoryID *int64 `json:"bank_category_id"`
		} `json:"offers"`
	}
	owner.must("GET", fmt.Sprintf("/api/v1/cashback/offer-periods/%d", alfaPeriod.ID), nil, &periodAfter, http.StatusOK)
	found := false
	for _, o := range periodAfter.Offers {
		if o.ID == pickedOffer.ID {
			found = true
			if o.BankCategoryID != nil || o.RawTitle != "Кофейни" {
				t.Fatalf("offer after catalog delete = %+v, want null FK + intact raw_title", o)
			}
		}
	}
	if !found {
		t.Fatal("picked offer vanished after catalog-row delete")
	}

	// NFC + homoglyph normalization: the REAL Альфа spelling with a Latin
	// «p» inside «pестораны» still resolves through the alias table.
	var homoglyphSuggestion suggestionJSON
	owner.must("GET", "/api/v1/cashback/alias-suggestion?offer_period_id="+
		fmt.Sprint(alfaPeriod.ID)+"&raw_title="+url.QueryEscape("Кафе и pестораны"), nil, &homoglyphSuggestion, http.StatusOK)
	if homoglyphSuggestion.Suggestion == nil || homoglyphSuggestion.Suggestion.Slug != "restaurants" {
		t.Fatalf("homoglyph title suggestion = %+v, want restaurants", homoglyphSuggestion.Suggestion)
	}

	// --- MCC module (2026-07-21): embedded dictionary + membership seed,
	// search, per-bank resolve, change journal ---

	if got := anon.do("GET", "/api/v1/mcc/resolve?code=5411", nil, nil); got != http.StatusUnauthorized {
		t.Fatalf("anonymous mcc resolve: %d, want 401", got)
	}

	type mccCodeJSON struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var codes []mccCodeJSON
	owner.must("GET", "/api/v1/mcc/codes?query=5411", nil, &codes, http.StatusOK)
	found5411 := false
	for _, c := range codes {
		if c.Code == "5411" {
			found5411 = true
			if c.Name == "" {
				t.Fatal("5411 dictionary row has empty name")
			}
		}
	}
	if !found5411 {
		t.Fatalf("codes?query=5411 = %+v, want a 5411 row", codes)
	}
	// Leading-zero code path (742 stored as smallint, padded in the API).
	owner.must("GET", "/api/v1/mcc/codes?query=0742", nil, &codes, http.StatusOK)
	if len(codes) == 0 || codes[0].Code != "0742" {
		t.Fatalf("codes?query=0742 = %+v, want 0742 first", codes)
	}
	// Name-substring search in Cyrillic.
	owner.must("GET", "/api/v1/mcc/codes?query="+url.QueryEscape("аптек"), nil, &codes, http.StatusOK)
	if len(codes) == 0 {
		t.Fatal("codes?query=аптек returned nothing")
	}

	var resolved struct {
		Code  mccCodeJSON `json:"code"`
		Banks []struct {
			BankName      string  `json:"bank_name"`
			Title         string  `json:"title"`
			Kind          string  `json:"kind"`
			Emoji         *string `json:"emoji"`
			CanonicalSlug *string `json:"canonical_slug"`
		} `json:"banks"`
		Canonicals []struct {
			Slug string `json:"slug"`
		} `json:"canonicals"`
	}
	owner.must("GET", "/api/v1/mcc/resolve?code=5411", nil, &resolved, http.StatusOK)
	gotBanks := map[string]string{}
	for _, b := range resolved.Banks {
		gotBanks[b.BankName] = b.Title
		if b.BankName == "Альфа-Банк" && (b.Emoji == nil || *b.Emoji != "🛒") {
			t.Fatalf("5411 Альфа-Банк emoji = %v, want inherited 🛒", b.Emoji)
		}
	}
	// Альфа's live menu says «Продукты» (owner-verified 2026-07-22).
	for bank, want := range map[string]string{"Альфа-Банк": "Продукты", "ВТБ": "Супермаркеты", "Озон Банк": "Супермаркеты"} {
		if gotBanks[bank] != want {
			t.Fatalf("5411 at %s = %q, want %s (all: %v)", bank, gotBanks[bank], want, gotBanks)
		}
	}
	haveSupermarkets := false
	for _, c := range resolved.Canonicals {
		if c.Slug == "supermarkets" {
			haveSupermarkets = true
		}
	}
	if !haveSupermarkets {
		t.Fatalf("5411 canonicals = %+v, want supermarkets", resolved.Canonicals)
	}
	// Unknown code → 404 (dictionary is the gate).
	if got := owner.do("GET", "/api/v1/mcc/resolve?code=0001", nil, nil); got != http.StatusNotFound {
		t.Fatalf("resolve unknown code: %d, want 404", got)
	}

	// The seed was the first import: journal is non-empty, all `imported`.
	var changes []struct {
		Action string `json:"action"`
		Source string `json:"source"`
	}
	owner.must("GET", "/api/v1/mcc/changes?limit=500", nil, &changes, http.StatusOK)
	if len(changes) == 0 {
		t.Fatal("mcc change journal empty after seed")
	}
	for _, c := range changes {
		if c.Action != "imported" {
			t.Fatalf("baseline journal action = %q (%s), want imported", c.Action, c.Source)
		}
	}
	// Idempotency of the journal itself: a third seed run writes nothing new
	// (the double run at the top already proved membership idempotency).
	var journalCount int
	if err := pool.QueryRow(ctx, "select count(*) from mcc_change").Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatalf("third seed run: %v", err)
	}
	var journalAfter int
	if err := pool.QueryRow(ctx, "select count(*) from mcc_change").Scan(&journalAfter); err != nil {
		t.Fatal(err)
	}
	if journalAfter != journalCount {
		t.Fatalf("journal grew on a no-change seed re-run: %d → %d", journalCount, journalAfter)
	}

	// --- Bank-first flow (2026-07-23): DELETE for cards and bank clients.
	// Card delete is plain but scoped; client delete takes its cards with it
	// and is refused (409) while КБ history exists — offer_period /
	// partner_offer keep plain FKs to bank_client, so history physically
	// blocks the delete. ---

	// A scratch card on the owner's Озон client: invisible to the other
	// user (404), gone for the owner (204).
	var scratchCard cardJSON
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": ozonClient.ID, "last_4_digits": 4444, "payment_system": "mir",
	}, &scratchCard, http.StatusCreated)
	if got := other.do("DELETE", fmt.Sprintf("/api/v1/cards/%d", scratchCard.ID), nil, nil); got != http.StatusNotFound {
		t.Fatalf("foreign card delete: %d, want 404", got)
	}
	owner.must("DELETE", fmt.Sprintf("/api/v1/cards/%d", scratchCard.ID), nil, nil, http.StatusNoContent)
	var cardsLeft []cardJSON
	owner.must("GET", "/api/v1/cards", nil, &cardsLeft, http.StatusOK)
	for _, c := range cardsLeft {
		if c.ID == scratchCard.ID {
			t.Fatal("deleted card still listed")
		}
	}

	// The Альфа-Банк client owns July periods → its delete is blocked.
	if got := owner.do("DELETE", fmt.Sprintf("/api/v1/bank-clients/%d", alfaClient.ID), nil, nil); got != http.StatusConflict {
		t.Fatalf("delete client with periods: %d, want 409", got)
	}

	// A partner offer alone is history too.
	var partnerClient clientJSON
	owner.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": vtbID, "label": "Партнёрский",
	}, &partnerClient, http.StatusCreated)
	owner.must("POST", "/api/v1/cashback/partner-offers", map[string]any{
		"bank_id": vtbID, "bank_client_id": partnerClient.ID, "merchant_title": "Кофейня у дома",
	}, nil, http.StatusCreated)
	if got := owner.do("DELETE", fmt.Sprintf("/api/v1/bank-clients/%d", partnerClient.ID), nil, nil); got != http.StatusConflict {
		t.Fatalf("delete client with partner offer: %d, want 409", got)
	}
	// Foreign client delete is invisible, like every other scoped op.
	if got := other.do("DELETE", fmt.Sprintf("/api/v1/bank-clients/%d", partnerClient.ID), nil, nil); got != http.StatusNotFound {
		t.Fatalf("foreign client delete: %d, want 404", got)
	}

	// A clean client goes away in one shot, cards and all.
	var scratchClient clientJSON
	owner.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": vtbID, "label": "Скретч",
	}, &scratchClient, http.StatusCreated)
	var scratchClientCard cardJSON
	owner.must("POST", "/api/v1/cards", map[string]any{
		"bank_client_id": scratchClient.ID, "last_4_digits": 7777, "payment_system": "mir",
	}, &scratchClientCard, http.StatusCreated)
	owner.must("DELETE", fmt.Sprintf("/api/v1/bank-clients/%d", scratchClient.ID), nil, nil, http.StatusNoContent)
	var clientsLeft []clientJSON
	owner.must("GET", "/api/v1/bank-clients", nil, &clientsLeft, http.StatusOK)
	for _, c := range clientsLeft {
		if c.ID == scratchClient.ID {
			t.Fatal("deleted bank client still listed")
		}
	}
	owner.must("GET", "/api/v1/cards", nil, &cardsLeft, http.StatusOK)
	for _, c := range cardsLeft {
		if c.ID == scratchClientCard.ID {
			t.Fatal("deleted client's card still listed")
		}
	}
}

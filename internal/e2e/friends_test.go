// TestFriendsE2E runs the friends-sharing spec's acceptance script
// (docs/specs/friends-sharing.md, «Definition of done + E2E») — the graph
// half: search, заявка lifecycle with auto-accept, invites (hash-only
// storage, burn/expire/self/revoke), unfriend, the enumeration guard.
// Grant-gated visibility lands with the sharing endpoints.
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
)

type foundUserJSON struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type friendJSON struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type requestsJSON struct {
	Incoming []struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"incoming"`
	Outgoing []struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"outgoing"`
}

type inviteCreatedJSON struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

func TestFriendsE2E(t *testing.T) {
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
	if err := migrations.Up(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := seed.Run(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(server.New(server.Config{Pool: pool, AttachmentsDir: t.TempDir(), InsecureCookie: true}))
	defer srv.Close()

	anna := newClient(t, srv.URL)
	boris := newClient(t, srv.URL)
	carl := newClient(t, srv.URL)

	var annaMe, borisMe, carlMe struct {
		ID string `json:"id"`
	}
	anna.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "Anna", "display_name": "Аня", "email": "anna@example.com", "password": "correct horse",
	}, &annaMe, http.StatusCreated)
	boris.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "boris", "display_name": "Боря", "email": "boris@example.com", "password": "correct horse",
	}, &borisMe, http.StatusCreated)
	carl.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "carl", "display_name": "Карл", "email": "carl@example.com", "password": "correct horse",
	}, &carlMe, http.StatusCreated)
	// dima never befriends anyone — the clean probe for expired/revoked
	// claims (a friend of the inviter would 409 as already-friends first)
	// and for the rate-limit walk.
	dima := newClient(t, srv.URL)
	dima.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "dima", "display_name": "Дима", "email": "dima@example.com", "password": "correct horse",
	}, nil, http.StatusCreated)

	// --- Step 1: search is exact-only, case-insensitive, minimal fields ---
	var found foundUserJSON
	boris.must("GET", "/api/v1/friends/search?username=anna", nil, &found, http.StatusOK)
	if found.Username != "Anna" || found.UserID != annaMe.ID {
		t.Fatalf("case-insensitive search: got %+v", found)
	}
	if got := boris.do("GET", "/api/v1/friends/search?username=ann", nil, nil); got != http.StatusNotFound {
		t.Fatalf("substring search: status %d, want 404", got)
	}
	if got := boris.do("GET", "/api/v1/friends/search?username=nobody", nil, nil); got != http.StatusNotFound {
		t.Fatalf("miss search: status %d, want 404", got)
	}

	// --- Step 2: заявка lifecycle ---
	var reqStatus struct {
		Status string `json:"status"`
	}
	anna.must("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, &reqStatus, http.StatusCreated)
	if reqStatus.Status != "pending" {
		t.Fatalf("first request status = %q, want pending", reqStatus.Status)
	}
	if got := anna.do("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, nil); got != http.StatusConflict {
		t.Fatalf("duplicate request: status %d, want 409", got)
	}
	if got := anna.do("POST", "/api/v1/friends/requests", map[string]any{"username": "Anna"}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("self request: status %d, want 422", got)
	}

	// Decline quietly removes the pending row from both lists.
	var borisReqs requestsJSON
	boris.must("GET", "/api/v1/friends/requests", nil, &borisReqs, http.StatusOK)
	if len(borisReqs.Incoming) != 1 || borisReqs.Incoming[0].Username != "Anna" {
		t.Fatalf("boris incoming = %+v, want one from Anna", borisReqs)
	}
	boris.must("POST", "/api/v1/friends/requests/"+itoa(borisReqs.Incoming[0].ID)+"/decline", nil, nil, http.StatusNoContent)
	var annaReqs requestsJSON
	anna.must("GET", "/api/v1/friends/requests", nil, &annaReqs, http.StatusOK)
	if len(annaReqs.Outgoing) != 0 {
		t.Fatalf("after decline, anna outgoing = %+v, want empty", annaReqs.Outgoing)
	}

	// Re-request and accept.
	anna.must("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, &reqStatus, http.StatusCreated)
	boris.must("GET", "/api/v1/friends/requests", nil, &borisReqs, http.StatusOK)
	// Only the recipient may accept; the sender probing the id sees 404.
	if got := anna.do("POST", "/api/v1/friends/requests/"+itoa(borisReqs.Incoming[0].ID)+"/accept", nil, nil); got != http.StatusNotFound {
		t.Fatalf("sender accepting own request: status %d, want 404", got)
	}
	boris.must("POST", "/api/v1/friends/requests/"+itoa(borisReqs.Incoming[0].ID)+"/accept", nil, nil, http.StatusNoContent)

	var annaFriends, borisFriends []friendJSON
	anna.must("GET", "/api/v1/friends", nil, &annaFriends, http.StatusOK)
	boris.must("GET", "/api/v1/friends", nil, &borisFriends, http.StatusOK)
	if len(annaFriends) != 1 || annaFriends[0].Username != "boris" {
		t.Fatalf("anna friends = %+v, want [boris]", annaFriends)
	}
	if len(borisFriends) != 1 || borisFriends[0].Username != "Anna" {
		t.Fatalf("boris friends = %+v, want [Anna]", borisFriends)
	}
	// Request to an existing friend → 409.
	if got := anna.do("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, nil); got != http.StatusConflict {
		t.Fatalf("request to a friend: status %d, want 409", got)
	}

	// Mutual pending auto-accepts: boris→carl, then carl→boris.
	boris.must("POST", "/api/v1/friends/requests", map[string]any{"username": "carl"}, &reqStatus, http.StatusCreated)
	carl.must("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, &reqStatus, http.StatusCreated)
	if reqStatus.Status != "accepted" {
		t.Fatalf("mutual pending: status %q, want accepted", reqStatus.Status)
	}
	var carlReqs requestsJSON
	carl.must("GET", "/api/v1/friends/requests", nil, &carlReqs, http.StatusOK)
	if len(carlReqs.Incoming)+len(carlReqs.Outgoing) != 0 {
		t.Fatalf("after auto-accept, carl still has pending rows: %+v", carlReqs)
	}

	// Cancel: anna→carl, anna cancels, carl sees nothing.
	anna.must("POST", "/api/v1/friends/requests", map[string]any{"username": "carl"}, &reqStatus, http.StatusCreated)
	anna.must("GET", "/api/v1/friends/requests", nil, &annaReqs, http.StatusOK)
	anna.must("DELETE", "/api/v1/friends/requests/"+itoa(annaReqs.Outgoing[0].ID), nil, nil, http.StatusNoContent)
	carl.must("GET", "/api/v1/friends/requests", nil, &carlReqs, http.StatusOK)
	if len(carlReqs.Incoming) != 0 {
		t.Fatalf("after cancel, carl incoming = %+v, want empty", carlReqs.Incoming)
	}

	// The canonical pair is stored once, ordered.
	var badPairs int
	if err := pool.QueryRow(ctx, "select count(*) from friendship where user_lo >= user_hi").Scan(&badPairs); err != nil {
		t.Fatal(err)
	}
	if badPairs != 0 {
		t.Fatalf("%d friendship rows violate lo<hi", badPairs)
	}

	// --- Step 3: enumeration safety for a non-friend ---
	if got := carl.do("DELETE", "/api/v1/friends/"+annaMe.ID, nil, nil); got != http.StatusNotFound {
		t.Fatalf("unfriending a non-friend: status %d, want 404", got)
	}

	// --- Step 4: invites ---
	var inv inviteCreatedJSON
	anna.must("POST", "/api/v1/friends/invites", nil, &inv, http.StatusCreated)
	if inv.Token == "" || inv.URL != "/friends/join/"+inv.Token {
		t.Fatalf("invite create: %+v", inv)
	}
	// Hash-only storage: the plaintext token never lands in the DB.
	var stored int
	if err := pool.QueryRow(ctx, "select count(*) from friend_invite where token_hash = $1 or encode(token_hash, 'escape') = $2",
		[]byte(inv.Token), inv.Token).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatal("invite token stored in plaintext")
	}

	// Self-claim → 422; carl claims → friends; re-claim → 409.
	if got := anna.do("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv.Token}, nil); got != http.StatusUnprocessableEntity {
		t.Fatalf("self claim: status %d, want 422", got)
	}
	var inviter foundUserJSON
	carl.must("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv.Token}, &inviter, http.StatusOK)
	if inviter.Username != "Anna" {
		t.Fatalf("claim response names %q, want Anna", inviter.Username)
	}
	var carlFriends []friendJSON
	carl.must("GET", "/api/v1/friends", nil, &carlFriends, http.StatusOK)
	if len(carlFriends) != 2 {
		t.Fatalf("carl friends = %+v, want boris + Anna", carlFriends)
	}
	if got := boris.do("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv.Token}, nil); got != http.StatusConflict {
		t.Fatalf("re-claim: status %d, want 409", got)
	}

	// Already-friends claim leaves the token unburned (invariant 5): a fresh
	// invite claimed by an existing friend answers 409 and stays live.
	var inv2 inviteCreatedJSON
	anna.must("POST", "/api/v1/friends/invites", nil, &inv2, http.StatusCreated)
	if got := carl.do("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv2.Token}, nil); got != http.StatusConflict {
		t.Fatalf("already-friends claim: status %d, want 409", got)
	}
	var live []inviteCreatedJSON
	anna.must("GET", "/api/v1/friends/invites", nil, &live, http.StatusOK)
	if len(live) != 1 || live[0].ID != inv2.ID {
		t.Fatalf("live invites = %+v, want just the unburned one", live)
	}

	// Expired → 410 (SQL-nudge; the API has no time machine).
	if _, err := pool.Exec(ctx, "update friend_invite set expires_at = now() - interval '1 hour' where id = $1", inv2.ID); err != nil {
		t.Fatal(err)
	}
	if got := dima.do("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv2.Token}, nil); got != http.StatusGone {
		t.Fatalf("expired claim: status %d, want 410", got)
	}

	// Revoke: a deleted invite claims as 404.
	var inv3 inviteCreatedJSON
	anna.must("POST", "/api/v1/friends/invites", nil, &inv3, http.StatusCreated)
	anna.must("DELETE", "/api/v1/friends/invites/"+inv3.ID, nil, nil, http.StatusNoContent)
	if got := dima.do("POST", "/api/v1/friends/invites/claim", map[string]any{"token": inv3.Token}, nil); got != http.StatusNotFound {
		t.Fatalf("revoked claim: status %d, want 404", got)
	}

	// --- Step 5: unfriend ---
	anna.must("DELETE", "/api/v1/friends/"+borisMe.ID, nil, nil, http.StatusNoContent)
	boris.must("GET", "/api/v1/friends", nil, &borisFriends, http.StatusOK)
	for _, f := range borisFriends {
		if f.Username == "Anna" {
			t.Fatal("boris still lists Anna after unfriend")
		}
	}
	// The graph is symmetric again: a fresh заявка works.
	anna.must("POST", "/api/v1/friends/requests", map[string]any{"username": "boris"}, &reqStatus, http.StatusCreated)
	if reqStatus.Status != "pending" {
		t.Fatalf("post-unfriend request status = %q, want pending", reqStatus.Status)
	}

	// --- Step 6: the enumeration guard rate-limits a search walk ---
	limited := false
	for i := 0; i < 25; i++ {
		if got := dima.do("GET", "/api/v1/friends/search?username=nobody", nil, nil); got == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("25 rapid searches never hit the rate limit")
	}

	// ================= Sharing (grants + the cashback read path) =========

	// Re-friend anna–boris: boris accepts the pending заявка from step 5.
	boris.must("GET", "/api/v1/friends/requests", nil, &borisReqs, http.StatusOK)
	boris.must("POST", "/api/v1/friends/requests/"+itoa(borisReqs.Incoming[0].ID)+"/accept", nil, nil, http.StatusNoContent)

	// --- anna's cashback picture: Альфа-Банк own client (Смарт) with a
	// current-month period — Супермаркеты 7% selected, барабан-Такси 10%
	// super marked, Рестораны 3% unselected — plus a «Мама» client that
	// never gets granted. ---
	var programs []programJSON
	anna.must("GET", "/api/v1/cashback/programs", nil, &programs, http.StatusOK)
	var alfaProgram programJSON
	for _, p := range programs {
		if p.BankName == "Альфа-Банк" {
			alfaProgram = p
		}
	}
	if alfaProgram.ID == 0 {
		t.Fatal("no seeded Альфа-Банк program")
	}
	var tiers []tierJSON
	anna.must("GET", fmt.Sprintf("/api/v1/cashback/programs/%d/tiers", alfaProgram.ID), nil, &tiers, http.StatusOK)
	var smart tierJSON
	for _, tr := range tiers {
		if tr.Name == "Альфа-Смарт" {
			smart = tr
		}
	}
	if smart.ID == 0 {
		t.Fatal("no seeded Альфа-Смарт tier")
	}

	var annaOwn, annaMama clientJSON
	anna.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": alfaProgram.BankID, "program_tier_id": smart.ID,
	}, &annaOwn, http.StatusCreated)
	anna.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": alfaProgram.BankID, "label": "Мама", "program_tier_id": smart.ID,
	}, &annaMama, http.StatusCreated)

	var cats []struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	anna.must("GET", "/api/v1/cashback/canonical-categories", nil, &cats, http.StatusOK)
	catID := func(slug string) int64 {
		for _, c := range cats {
			if c.Slug == slug {
				return c.ID
			}
		}
		t.Fatalf("no canonical category %q", slug)
		return 0
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	monthEnd := monthStart.AddDate(0, 1, -1)
	period := func(c *client, clientID int64) int64 {
		var p periodJSON
		c.must("POST", "/api/v1/cashback/offer-periods", map[string]any{
			"bank_client_id": clientID,
			"period_start":   monthStart.Format("2006-01-02"),
			"period_end":     monthEnd.Format("2006-01-02"),
		}, &p, http.StatusCreated)
		return p.ID
	}
	addOffer := func(c *client, periodID int64, raw, percent, kind string, canonical int64) int64 {
		body := map[string]any{"offer_period_id": periodID, "raw_title": raw, "percent": percent, "kind": kind}
		if canonical != 0 {
			body["canonical_category_id"] = canonical
		}
		var o offerJSON
		c.must("POST", "/api/v1/cashback/category-offers", body, &o, http.StatusCreated)
		return o.ID
	}
	sel := func(c *client, offerID int64) {
		c.must("POST", "/api/v1/cashback/selections",
			map[string]any{"category_offer_id": offerID, "selected_at": now.Format(time.RFC3339)}, nil, http.StatusCreated)
	}

	annaPeriod := period(anna, annaOwn.ID)
	annaSuper := addOffer(anna, annaPeriod, "Супермаркеты", "7", "regular", catID("supermarkets"))
	sel(anna, annaSuper)
	annaDrum := addOffer(anna, annaPeriod, "Барабан: Такси", "10", "super", catID("taxi"))
	sel(anna, annaDrum)
	addOffer(anna, annaPeriod, "Рестораны", "3", "regular", catID("restaurants"))
	mamaPeriod := period(anna, annaMama.ID)
	mamaPharm := addOffer(anna, mamaPeriod, "Аптеки", "5", "regular", catID("pharmacies"))
	sel(anna, mamaPharm)

	// --- Default nothing shared: boris sees friend anna with zero clients ---
	var browse friendsBrowseJSON
	boris.must("GET", "/api/v1/cashback/friends", nil, &browse, http.StatusOK)
	annaEntry := browse.find(t, "Anna")
	if len(annaEntry.Clients) != 0 {
		t.Fatalf("before any grant, anna's clients = %+v, want none", annaEntry.Clients)
	}

	// --- Grant probes are enumeration-safe: not-my-client and not-my-friend
	// answer the same 404 ---
	if got := dima.do("PUT", "/api/v1/friends/sharing", map[string]any{
		"bank_client_id": annaOwn.ID, "friend_user_id": borisMe.ID, "shared": true,
	}, nil); got != http.StatusNotFound {
		t.Fatalf("granting someone else's client: status %d, want 404", got)
	}
	var dimaMe foundUserJSON
	boris.must("GET", "/api/v1/friends/search?username=dima", nil, &dimaMe, http.StatusOK)
	if got := anna.do("PUT", "/api/v1/friends/sharing", map[string]any{
		"bank_client_id": annaOwn.ID, "friend_user_id": dimaMe.UserID, "shared": true,
	}, nil); got != http.StatusNotFound {
		t.Fatalf("granting to a non-friend: status %d, want 404", got)
	}

	// --- anna grants boris the own client (idempotent), not мамин ---
	anna.must("PUT", "/api/v1/friends/sharing", map[string]any{
		"bank_client_id": annaOwn.ID, "friend_user_id": borisMe.ID, "shared": true,
	}, nil, http.StatusNoContent)
	anna.must("PUT", "/api/v1/friends/sharing", map[string]any{
		"bank_client_id": annaOwn.ID, "friend_user_id": borisMe.ID, "shared": true,
	}, nil, http.StatusNoContent)
	var sharing []struct {
		BankClientID int64  `json:"bank_client_id"`
		FriendUserID string `json:"friend_user_id"`
	}
	anna.must("GET", "/api/v1/friends/sharing", nil, &sharing, http.StatusOK)
	if len(sharing) != 1 || sharing[0].BankClientID != annaOwn.ID || sharing[0].FriendUserID != borisMe.ID {
		t.Fatalf("sharing list = %+v, want one grant of the own client to boris", sharing)
	}
	// Invariant 2: every share's client owner is a member of its friendship.
	var corrupt int
	if err := pool.QueryRow(ctx, `select count(*) from friend_cashback_share s
		join bank_client cl on cl.id = s.bank_client_id
		join friendship f on f.id = s.friendship_id
		where cl.user_id <> f.user_lo and cl.user_id <> f.user_hi`).Scan(&corrupt); err != nil {
		t.Fatal(err)
	}
	if corrupt != 0 {
		t.Fatalf("%d corrupt grants (client owner outside the friendship)", corrupt)
	}

	// --- boris sees the granted client's picture; мамин client stays
	// invisible; no cap field ever serializes (invariant 4) ---
	raw := rawGet(t, boris, "/api/v1/cashback/friends")
	for _, needle := range []string{"cap_value", "cap_per_category", "max_categories"} {
		if strings.Contains(raw, needle) {
			t.Fatalf("friend browse payload leaks %q:\n%s", needle, raw)
		}
	}
	if err := json.Unmarshal([]byte(raw), &browse); err != nil {
		t.Fatal(err)
	}
	annaEntry = browse.find(t, "Anna")
	if len(annaEntry.Clients) != 1 || annaEntry.Clients[0].BankClientID != annaOwn.ID {
		t.Fatalf("granted clients = %+v, want just the own Альфа-Банк client", annaEntry.Clients)
	}
	cl := annaEntry.Clients[0]
	if cl.BankName != "Альфа-Банк" || cl.PeriodStart == nil {
		t.Fatalf("client view = %+v, want Альфа-Банк with a current period", cl)
	}
	if len(cl.Selected) != 1 || cl.Selected[0].RawTitle != "Супермаркеты" || *cl.Selected[0].Percent != "7" {
		t.Fatalf("selected = %+v, want [Супермаркеты 7]", cl.Selected)
	}
	if len(cl.Granted) != 1 || cl.Granted[0].Kind != "super" {
		t.Fatalf("granted = %+v, want the super барабан row", cl.Granted)
	}
	if len(cl.Menu) != 1 || cl.Menu[0].RawTitle != "Рестораны" {
		t.Fatalf("menu = %+v, want the unselected Рестораны row", cl.Menu)
	}

	// --- Direction independence: anna's view of boris stays empty ---
	anna.must("GET", "/api/v1/cashback/friends", nil, &browse, http.StatusOK)
	if got := browse.find(t, "boris"); len(got.Clients) != 0 {
		t.Fatalf("boris shared nothing, yet anna sees %+v", got.Clients)
	}

	// --- Lookup ranks the friend's 7% over the own 5%; on the tie the own
	// card wins; fallback and available stay personal (invariant 7) ---
	var borisClient clientJSON
	boris.must("POST", "/api/v1/bank-clients", map[string]any{
		"bank_id": alfaProgram.BankID, "program_tier_id": smart.ID,
	}, &borisClient, http.StatusCreated)
	borisPeriod := period(boris, borisClient.ID)
	borisSuper := addOffer(boris, borisPeriod, "Продукты", "5", "regular", catID("supermarkets"))
	sel(boris, borisSuper)

	var lookup friendLookupJSON
	boris.must("GET", "/api/v1/cashback/lookup?category=supermarkets", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 2 {
		t.Fatalf("ranked = %+v, want the friend's card + the own one", lookup.Ranked)
	}
	if lookup.Ranked[0].FriendName != "Аня" || *lookup.Ranked[0].Percent != "7" {
		t.Fatalf("ranked[0] = %+v, want «картой Ани» at 7%%", lookup.Ranked[0])
	}
	if lookup.Ranked[1].FriendName != "" {
		t.Fatalf("ranked[1] = %+v, want the own card", lookup.Ranked[1])
	}
	if lookup.Ranked[0].CapValue != nil || lookup.Ranked[0].OfferCapValue != nil {
		t.Fatalf("friend entry carries caps: %+v", lookup.Ranked[0])
	}

	boris.must("PUT", fmt.Sprintf("/api/v1/cashback/category-offers/%d", borisSuper),
		map[string]any{"raw_title": "Продукты", "percent": "7", "canonical_category_id": catID("supermarkets"), "kind": "regular"},
		nil, http.StatusOK)
	// Fresh decode target: Unmarshal into a populated struct keeps stale
	// fields for keys absent in the new payload (friend_name is omitempty).
	lookup = friendLookupJSON{}
	boris.must("GET", "/api/v1/cashback/lookup?category=supermarkets", nil, &lookup, http.StatusOK)
	if lookup.Ranked[0].FriendName != "" || lookup.Ranked[1].FriendName != "Аня" {
		t.Fatalf("equal-percent tie: ranked = %+v, want the own card first", lookup.Ranked)
	}

	// anna's unselected Рестораны row must not surface as boris's
	// «Можно выбрать» — verdicts are owner actions.
	lookup = friendLookupJSON{}
	boris.must("GET", "/api/v1/cashback/lookup?category=restaurants", nil, &lookup, http.StatusOK)
	if len(lookup.Ranked) != 0 || len(lookup.Available) != 0 {
		t.Fatalf("restaurants lookup = %+v, want empty (friend menu rows never rank or offer)", lookup)
	}
	// anna's selected «За все покупки» must not reach boris's fallback.
	annaAll := addOffer(anna, annaPeriod, "За все покупки", "1", "regular", catID("all-purchases"))
	sel(anna, annaAll)
	lookup = friendLookupJSON{}
	boris.must("GET", "/api/v1/cashback/lookup?category=supermarkets", nil, &lookup, http.StatusOK)
	if len(lookup.Fallback) != 0 {
		t.Fatalf("fallback = %+v, want empty (friends' base rates stay personal)", lookup.Fallback)
	}

	// --- Unfriend revokes the grants by cascade; re-friending resurrects
	// nothing ---
	anna.must("DELETE", "/api/v1/friends/"+borisMe.ID, nil, nil, http.StatusNoContent)
	var shareCount int
	if err := pool.QueryRow(ctx, "select count(*) from friend_cashback_share").Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 0 {
		t.Fatalf("%d grants survive the unfriend cascade", shareCount)
	}
	boris.must("GET", "/api/v1/cashback/friends", nil, &browse, http.StatusOK)
	for _, f := range browse.Friends {
		if f.Username == "Anna" {
			t.Fatal("boris still browses anna after unfriend")
		}
	}
	boris.must("POST", "/api/v1/friends/requests", map[string]any{"username": "Anna"}, &reqStatus, http.StatusCreated)
	anna.must("GET", "/api/v1/friends/requests", nil, &annaReqs, http.StatusOK)
	anna.must("POST", "/api/v1/friends/requests/"+itoa(annaReqs.Incoming[0].ID)+"/accept", nil, nil, http.StatusNoContent)
	boris.must("GET", "/api/v1/cashback/friends", nil, &browse, http.StatusOK)
	if got := browse.find(t, "Anna"); len(got.Clients) != 0 {
		t.Fatalf("re-friending resurrected grants: %+v", got.Clients)
	}

	// --- Deleting a shared client succeeds (the grant cascades, no 409) ---
	var vtb programJSON
	for _, p := range programs {
		if p.BankName == "ВТБ" {
			vtb = p
		}
	}
	var annaVtb clientJSON
	anna.must("POST", "/api/v1/bank-clients", map[string]any{"bank_id": vtb.BankID}, &annaVtb, http.StatusCreated)
	anna.must("PUT", "/api/v1/friends/sharing", map[string]any{
		"bank_client_id": annaVtb.ID, "friend_user_id": borisMe.ID, "shared": true,
	}, nil, http.StatusNoContent)
	anna.must("DELETE", fmt.Sprintf("/api/v1/bank-clients/%d", annaVtb.ID), nil, nil, http.StatusNoContent)
	if err := pool.QueryRow(ctx, "select count(*) from friend_cashback_share").Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 0 {
		t.Fatalf("%d grants survive the client-delete cascade", shareCount)
	}
}

type friendBrowseOffer struct {
	RawTitle string  `json:"raw_title"`
	Percent  *string `json:"percent"`
	Kind     string  `json:"kind"`
}

type friendBrowseClient struct {
	BankClientID int64               `json:"bank_client_id"`
	BankName     string              `json:"bank_name"`
	HolderLabel  string              `json:"holder_label"`
	PeriodStart  *string             `json:"period_start"`
	Selected     []friendBrowseOffer `json:"selected"`
	Granted      []friendBrowseOffer `json:"granted"`
	Menu         []friendBrowseOffer `json:"menu"`
}

type friendBrowseEntry struct {
	Username    string               `json:"username"`
	DisplayName string               `json:"display_name"`
	Clients     []friendBrowseClient `json:"clients"`
}

type friendsBrowseJSON struct {
	Friends []friendBrowseEntry `json:"friends"`
}

func (b friendsBrowseJSON) find(t *testing.T, username string) *friendBrowseEntry {
	t.Helper()
	for i := range b.Friends {
		if b.Friends[i].Username == username {
			return &b.Friends[i]
		}
	}
	t.Fatalf("friend %q not in browse payload", username)
	return nil
}

type friendLookupJSON struct {
	Ranked []struct {
		BankName      string  `json:"bank_name"`
		Percent       *string `json:"percent"`
		FriendName    string  `json:"friend_name"`
		CapValue      *string `json:"cap_value"`
		OfferCapValue *string `json:"offer_cap_value"`
	} `json:"ranked"`
	Fallback  []struct{} `json:"fallback"`
	Available []struct{} `json:"available"`
}

// rawGet fetches a path and returns the raw body — for asserting what a
// payload does NOT contain.
func rawGet(t *testing.T, c *client, path string) string {
	t.Helper()
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	return string(data)
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}

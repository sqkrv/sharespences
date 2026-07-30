// TestFriendsE2E runs the friends-sharing spec's acceptance script
// (docs/specs/friends-sharing.md, «Definition of done + E2E») — the graph
// half: search, заявка lifecycle with auto-accept, invites (hash-only
// storage, burn/expire/self/revoke), unfriend, the enumeration guard.
// Grant-gated visibility lands with the sharing endpoints.
package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}

// TestModerationE2E runs the roles-moderation spec's acceptance script
// (docs/specs/roles-moderation.md, «Definition of done»): the role gate
// from both sides, promotion through the sidecar's AD-08 path, the
// anonymous queue (asserted on the raw JSON, not the typed decode),
// approve → public visibility, reject → invisible to everyone including
// the author, the published review stream with its prune, and the
// demote-without-relogin invariant.
package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sqkrv/sharespences/internal/admin"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
)

func TestModerationE2E(t *testing.T) {
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

	srv := httptest.NewServer(server.New(server.Config{Pool: pool, AttachmentsDir: t.TempDir(), InsecureCookie: true}))
	defer srv.Close()
	// The sidecar shares the pool: promotion goes through the same AD-08
	// path the operator uses, so the appointment flow is covered end to end.
	adminHandler, err := admin.New(admin.Config{Pool: pool, Version: "vtest"})
	if err != nil {
		t.Fatal(err)
	}
	adminSrv := httptest.NewServer(adminHandler)
	defer adminSrv.Close()

	author := newClient(t, srv.URL)
	mod := newClient(t, srv.URL)
	operator := newClient(t, adminSrv.URL)

	var me struct {
		Role string `json:"role"`
	}
	author.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "avtor", "display_name": "Автор", "email": "avtor@example.com", "password": "secret-123",
	}, &me, 201)
	if me.Role != "user" {
		t.Fatalf("fresh registration role = %q, want user", me.Role)
	}
	mod.must("POST", "/api/v1/auth/register", map[string]any{
		"username": "moder", "display_name": "Модератор", "email": "moder@example.com", "password": "secret-123",
	}, nil, 201)

	// --- submission: pending, visible to its author only ---
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	author.must("POST", "/api/v1/mcc/points-of-sale", map[string]any{
		"mcc": "5411", "name": "Лавка у Автора", "type": "offline",
	}, &created, 201)
	if created.Status != "pending" {
		t.Fatalf("submission status = %q, want pending", created.Status)
	}
	if n := merchantHits(t, author, "Лавка у Автора"); n != 1 {
		t.Fatalf("author sees %d hits for own pending row, want 1", n)
	}
	if n := merchantHits(t, mod, "Лавка у Автора"); n != 0 {
		t.Fatalf("stranger sees %d hits for a pending row, want 0", n)
	}

	// --- the gate: role user → the same Russian 403 on every operation ---
	if got, body := rawGetStatus(t, mod, "/api/v1/moderation/pos"); got != 403 ||
		!strings.Contains(string(body), "нужны права модератора") {
		t.Fatalf("moderation list as user: %d %s", got, body)
	}
	if got := mod.do("POST", "/api/v1/moderation/pos/"+created.ID+"/approve", nil, nil); got != 403 {
		t.Fatalf("approve as user: %d, want 403", got)
	}

	// --- promotion via the sidecar (AD-08), effective without re-login ---
	if got := operator.do("GET", "/api/users/nikto/role", nil, nil); got != 404 {
		t.Fatalf("unknown username: %d, want 404", got)
	}
	var role struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	operator.must("PUT", "/api/users/moder/role", map[string]any{"role": "moderator"}, &role, 200)
	if role.Role != "moderator" {
		t.Fatalf("promoted role = %q", role.Role)
	}
	mod.must("GET", "/api/v1/auth/me", nil, &me, 200)
	if me.Role != "moderator" {
		t.Fatalf("me.role after promotion = %q, want moderator (no re-login)", me.Role)
	}

	// --- the queue is anonymous: assert on the RAW JSON keys ---
	code, body := rawGetStatus(t, mod, "/api/v1/moderation/pos?state=pending")
	if code != 200 {
		t.Fatalf("queue as moderator: %d %s", code, body)
	}
	var page struct {
		Total int64                    `json:"total"`
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("queue: total %d, items %d, want 1/1", page.Total, len(page.Items))
	}
	for key := range page.Items[0] {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "author") || strings.Contains(lower, "user") || strings.Contains(lower, "name_display") {
			t.Errorf("moderation row leaks a submitter-shaped key: %q", key)
		}
	}
	if page.Items[0]["name"] != "Лавка у Автора" || page.Items[0]["origin"] != "user_manual" {
		t.Fatalf("queue row: %+v", page.Items[0])
	}

	// --- approve → public; the row moves to the published stream ---
	mod.must("POST", "/api/v1/moderation/pos/"+created.ID+"/approve", nil, nil, 204)
	if n := merchantHits(t, mod, "Лавка у Автора"); n != 1 {
		t.Fatalf("approved row invisible to others: %d hits", n)
	}
	var stream struct {
		Total int64 `json:"total"`
		Items []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		} `json:"items"`
	}
	mod.must("GET", "/api/v1/moderation/pos?state=published", nil, &stream, 200)
	if stream.Total != 1 || stream.Items[0].ID != created.ID || stream.Items[0].Origin != "user_manual" {
		t.Fatalf("published stream: %+v", stream)
	}

	// --- reject a fresh submission → invisible to EVERYONE, author included ---
	var second struct {
		ID string `json:"id"`
	}
	author.must("POST", "/api/v1/mcc/points-of-sale", map[string]any{
		"mcc": "5411", "name": "Сомнительный ларёк", "type": "offline",
	}, &second, 201)
	mod.must("POST", "/api/v1/moderation/pos/"+second.ID+"/reject", nil, nil, 204)
	if n := merchantHits(t, author, "Сомнительный ларёк"); n != 0 {
		t.Fatalf("rejected row still visible to its author: %d hits", n)
	}
	// A rejected row cannot be approved: it left the queue for good.
	if got := mod.do("POST", "/api/v1/moderation/pos/"+second.ID+"/approve", nil, nil); got != 404 {
		t.Fatalf("approve of a rejected row: %d, want 404", got)
	}

	// --- prune: reject works on a PUBLISHED non-scrape row too ---
	mod.must("POST", "/api/v1/moderation/pos/"+created.ID+"/reject", nil, nil, 204)
	if n := merchantHits(t, author, "Лавка у Автора"); n != 0 {
		t.Fatalf("pruned row still visible: %d hits", n)
	}

	// --- demotion is effective on the next request, no re-login ---
	operator.must("PUT", "/api/users/moder/role", map[string]any{"role": "user"}, &role, 200)
	if got := mod.do("GET", "/api/v1/moderation/pos", nil, nil); got != 403 {
		t.Fatalf("moderation after demotion: %d, want 403", got)
	}
}

// rawGetStatus fetches a path and returns the status with the RAW body —
// the anonymity assertion must see the actual JSON keys, not a typed
// decode that would silently drop an offending field, and the gate check
// needs a non-200 body (friends_test.go's rawGet insists on 200).
func rawGetStatus(t *testing.T, c *client, path string) (int, []byte) {
	t.Helper()
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

// merchantHits counts merchant-search results for an exact-ish name.
func merchantHits(t *testing.T, c *client, name string) int {
	t.Helper()
	var rows []struct {
		Name string `json:"name"`
	}
	c.must("GET", "/api/v1/mcc/merchants?query="+strings.ReplaceAll(name, " ", "+"), nil, &rows, 200)
	n := 0
	for _, r := range rows {
		if r.Name == name {
			n++
		}
	}
	return n
}

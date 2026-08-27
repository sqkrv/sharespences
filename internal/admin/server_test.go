package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The panel carries no auth (ADR-0008), so the Host check is the only thing
// standing between it and a DNS-rebinding page that resolves its own hostname
// to 127.0.0.1 and then talks to the panel same-origin.
func TestLoopbackOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	cases := []struct {
		name    string
		allowed []string
		host    string
		want    int
	}{
		{"loopback ip", nil, "127.0.0.1:8081", http.StatusTeapot},
		{"localhost", nil, "localhost:8081", http.StatusTeapot},
		{"tunnel on another local port", nil, "localhost:9000", http.StatusTeapot},
		{"ipv6 loopback", nil, "[::1]:8081", http.StatusTeapot},
		{"uppercase host", nil, "LOCALHOST:8081", http.StatusTeapot},
		{"no port", nil, "127.0.0.1", http.StatusTeapot},
		{"rebinding attacker", nil, "rebind.attacker.example:8081", http.StatusForbidden},
		{"public name", nil, "sharespences.com", http.StatusForbidden},
		{"empty host", nil, "", http.StatusForbidden},
		{"configured alias allowed", []string{"admin.internal"}, "admin.internal:8081", http.StatusTeapot},
		{"configured alias replaces defaults", []string{"admin.internal"}, "localhost:8081", http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
			req.Host = c.host
			rec := httptest.NewRecorder()
			loopbackOnly(c.allowed)(ok).ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("Host %q: got %d, want %d", c.host, rec.Code, c.want)
			}
		})
	}
}

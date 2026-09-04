package admin

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqkrv/sharespences/internal/adminweb"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/i18n"
	"github.com/sqkrv/sharespences/internal/seed"
)

type Config struct {
	Pool *pgxpool.Pool
	// Version is the ADR-0006 CalVer stamp — the same one the app binary
	// reports; one tag versions both binaries.
	Version string
	// AllowedHosts are the Host header values the panel answers, guarding
	// against DNS rebinding (see loopbackOnly). Empty means the loopback
	// defaults, which cover the documented SSH-tunnel access.
	AllowedHosts []string
	// Docs serves huma's built-in API reference at /docs. Off by default: the
	// page loads Stoplight Elements from unpkg.com and then runs it
	// same-origin with the panel's own unauthenticated API.
	Docs bool
}

// defaultAllowedHosts is what an SSH tunnel to the loopback publish produces.
// Only the hostname is compared, so any local port works.
var defaultAllowedHosts = []string{"localhost", "127.0.0.1", "::1"}

// loopbackOnly rejects requests whose Host is not one of the allowed names.
// The panel has no auth — network isolation is the control (ADR-0008) — and a
// Host check is what keeps that control honest: without it any web page can
// point a hostname it owns at 127.0.0.1, and the browser then treats the panel
// as same-origin, which defeats both CORS and the loopback bind.
func loopbackOnly(allowed []string) func(http.Handler) http.Handler {
	if len(allowed) == 0 {
		allowed = defaultAllowedHosts
	}
	ok := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		ok[strings.ToLower(h)] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.ToLower(strings.Trim(host, "[]"))
			if !ok[host] {
				http.Error(w, "недопустимый заголовок Host", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// New builds the sidecar's HTTP handler: chi + huma («Sharespences Admin»)
// + the embedded admin UI as the catch-all. Deliberately no
// sessions and no auth middleware — access control is network isolation
// (ADR-0008): the compose service publishes 127.0.0.1 only and the binary
// itself defaults to a loopback listen address.
func New(cfg Config) (http.Handler, error) {
	i18n.Install()

	seededMCC, err := seed.SeededMCCCodes()
	if err != nil {
		return nil, err
	}
	svc := &Service{
		Q: db.New(cfg.Pool), Pool: cfg.Pool,
		SeededMCC: seededMCC,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(loopbackOnly(cfg.AllowedHosts))

	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	humaCfg := huma.DefaultConfig("Sharespences Admin", version)
	if !cfg.Docs {
		humaCfg.DocsPath = ""
	}
	api := humachi.New(r, humaCfg)
	registerVersion(api, version)
	RegisterHTTP(api, svc)

	// Embedded admin UI: catch-all for everything the API and docs don't claim.
	r.Handle("/*", adminweb.Handler())

	return r, nil
}

type VersionDTO struct {
	Version string `json:"version"`
}

func registerVersion(api huma.API, version string) {
	huma.Register(api, huma.Operation{
		OperationID: "admin-version", Method: http.MethodGet,
		Path: "/api/version", Summary: "Running build version", Tags: []string{"system"},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body VersionDTO }, error) {
		return &struct{ Body VersionDTO }{VersionDTO{Version: version}}, nil
	})
}

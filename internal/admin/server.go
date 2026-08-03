package admin

import (
	"context"
	"net/http"

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
}

// New builds the sidecar's HTTP handler: chi + huma («Sharespences Admin»,
// docs at /docs) + the embedded admin UI as the catch-all. Deliberately no
// sessions and no auth middleware — access control is network isolation
// (ADR-0008): the compose service publishes 127.0.0.1 only and the binary
// itself defaults to a loopback listen address.
func New(cfg Config) (http.Handler, error) {
	i18n.Install()

	seededMCC, err := seed.SeededMCCCodes()
	if err != nil {
		return nil, err
	}
	seededMembership, err := seed.SeededMembershipKeys()
	if err != nil {
		return nil, err
	}
	svc := &Service{
		Q: db.New(cfg.Pool), Pool: cfg.Pool,
		SeededMCC: seededMCC, SeededMembership: seededMembership,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	api := humachi.New(r, huma.DefaultConfig("Sharespences Admin", version))
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

// Command sharespences-admin is the operator sidecar (ADR-0008): catalog
// browsing/CRUD and a system dashboard, served with its own embedded admin
// UI. It carries NO auth — access control is network isolation: the default
// listen address is loopback, the compose service publishes 127.0.0.1 only,
// and the operator connects through an SSH tunnel.
//
// Config via env: DATABASE_URL (required), ADMIN_LISTEN_ADDR (default
// 127.0.0.1:8081 — the compose service overrides to :8081 in-container so
// the loopback port publish can reach it), ADMIN_ALLOWED_HOSTS (comma
// separated; empty allows the loopback names only, which is what keeps a
// DNS-rebinding page from reaching an unauthenticated panel), ADMIN_DOCS
// (default false — the huma API reference loads scripts from unpkg.com).
//
// The build identifies itself through `version`, stamped at link time with
// the same tag as the app binary (ADR-0006):
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/sharespences-admin
package main

import (
	"cmp"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sqkrv/sharespences/internal/admin"
	"github.com/sqkrv/sharespences/internal/db"
)

// version is set at link time (-X main.version=…); an unstamped build says
// "dev". ADR-0006: CalVer `vYYYY.M.N`, unpadded month.
var version string

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool reads a boolean env var, falling back to def when unset or unparsable.
func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}

// splitList parses a comma-separated env var, dropping blanks.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	handler, err := admin.New(admin.Config{
		Pool: pool, Version: version,
		// Comma-separated; empty keeps the loopback defaults. Set it only
		// when the panel is reached under another name (a tunnel alias),
		// never to open it up.
		AllowedHosts: splitList(os.Getenv("ADMIN_ALLOWED_HOSTS")),
		Docs:         envBool("ADMIN_DOCS", false),
	})
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              envOr("ADMIN_LISTEN_ADDR", "127.0.0.1:8081"),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("sharespences-admin %s listening on %s", cmp.Or(version, "dev"), srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

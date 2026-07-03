// Command sharespences is the single-binary app (ADR-0001/0002):
//
//	sharespences migrate   apply embedded goose migrations
//	sharespences seed      load reference data (banks, КБ programs, categories)
//	sharespences serve     run the HTTP API + embedded SPA (default)
//	sharespences openapi   print the OpenAPI 3.1 doc (frontend type codegen)
//
// Config via env: DATABASE_URL (required), LISTEN_ADDR (default :8080),
// ATTACHMENTS_DIR (default ./attachments).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		if cmd != "openapi" {
			return errors.New("DATABASE_URL is required")
		}
		// openapi only describes the API — no DB is touched; the pool is lazy.
		dsn = "postgres://openapi@localhost:5432/openapi"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "migrate":
		if err := migrations.Up(ctx, pool); err != nil {
			return err
		}
		log.Println("migrations applied")
		return nil
	case "seed":
		if err := seed.Run(ctx, pool); err != nil {
			return err
		}
		log.Println("seed data loaded")
		return nil
	case "openapi":
		spec, err := server.OpenAPI(server.Config{Pool: pool, AttachmentsDir: "attachments"})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(spec)
		return err
	case "serve":
		handler := server.New(server.Config{
			Pool:           pool,
			AttachmentsDir: envOr("ATTACHMENTS_DIR", "attachments"),
		})
		srv := &http.Server{
			Addr:              envOr("LISTEN_ADDR", ":8080"),
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		log.Printf("listening on %s (docs at /docs)", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (want migrate|seed|serve|openapi)", cmd)
	}
}

// Command sharespences is the single-binary app (ADR-0001/0002):
//
//	sharespences migrate   apply embedded goose migrations
//	sharespences seed      load reference data (banks, КБ programs, categories)
//	sharespences serve     run the HTTP API + embedded SPA (default)
//	sharespences openapi   print the OpenAPI 3.1 doc (frontend type codegen)
//
// Config via env: DATABASE_URL (required), LISTEN_ADDR (default :8080),
// ATTACHMENTS_DIR (default ./attachments), COOKIE_SECURE (default true —
// set false only for local development over plain http).
//
// The build identifies itself through `version`, stamped at link time
// (ADR-0006 CalVer) and served at GET /api/v1/version:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/sharespences
//
// Screenshot recognizer (off unless configured):
//
//	VISION_BACKEND     ollama | anthropic (empty = feature off)
//	VISION_MODEL       model name (default qwen3-vl:4b / claude-opus-5)
//	OLLAMA_HOST        Ollama base URL (default http://localhost:11434)
//	ANTHROPIC_API_KEY  required for the anthropic backend
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/migrations"
	"github.com/sqkrv/sharespences/internal/seed"
	"github.com/sqkrv/sharespences/internal/server"
	"github.com/sqkrv/sharespences/internal/vision"
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

// visionBackend builds the screenshot-recognizer backend from env.
// Misconfiguration fails startup loudly rather than surfacing as 503s
// later; an empty VISION_BACKEND just turns the feature off.
func visionBackend() (vision.Backend, error) {
	switch kind := os.Getenv("VISION_BACKEND"); kind {
	case "":
		return nil, nil
	case "ollama":
		return vision.NewOllama(envOr("OLLAMA_HOST", "http://localhost:11434"), envOr("VISION_MODEL", "qwen3-vl:4b")), nil
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, errors.New("VISION_BACKEND=anthropic needs ANTHROPIC_API_KEY")
		}
		return vision.NewAnthropic(key, os.Getenv("VISION_MODEL")), nil
	default:
		return nil, fmt.Errorf("unknown VISION_BACKEND %q (want ollama|anthropic or empty)", kind)
	}
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
		backend, err := visionBackend()
		if err != nil {
			return err
		}
		handler := server.New(server.Config{
			Pool:           pool,
			AttachmentsDir: envOr("ATTACHMENTS_DIR", "attachments"),
			Vision:         backend,
			Version:        version,
			// Secure by default: production is HTTPS and the session cookie
			// must never ride a plaintext request. Opt out only for local
			// development over http.
			InsecureCookie: !envBool("COOKIE_SECURE", true),
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
		log.Printf("sharespences %s listening on %s (docs at /docs)", cmp.Or(version, "dev"), srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (want migrate|seed|serve|openapi)", cmd)
	}
}

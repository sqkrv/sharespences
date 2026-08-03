// Package adminweb embeds the built admin UI (admin/ Vite project, ADR-0008)
// and serves it from the sidecar binary the same single-binary way
// internal/web serves the app SPA. Trimmed relative to internal/web: no
// static .html pages, so no extensionless resolve — static assets by path,
// index.html fallback for client routes.
//
// The dist/ directory here is build output (gitignored except .placeholder);
// a binary compiled without it still runs and answers with a hint.
package adminweb

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded admin UI. Paths under /api/, /docs and
// /openapi* never reach it (routed explicitly before the catch-all).
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed guarantees dist exists
	}
	return handlerFor(dist)
}

// handlerFor is Handler's body over an arbitrary FS — the embedded dist/ is
// build output that CI's test job never produces, so the tests drive this.
func handlerFor(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(dist, "index.html"); err != nil {
			http.Error(w, "admin UI not built: run `npm run build` in admin/ and rebuild, or use the Vite dev server (admin/: npm run dev)", http.StatusNotImplemented)
			return
		}
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath != "" {
			if _, err := fs.Stat(dist, reqPath); err == nil {
				if strings.HasPrefix(reqPath, "assets/") {
					// Content-hashed filenames: safe to cache forever.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(reqPath, "assets/") {
				// Never answer a missing hashed asset with index.html.
				http.NotFound(w, r)
				return
			}
		}
		// Client-side route (or /): SPA fallback.
		r.URL.Path = "/"
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}

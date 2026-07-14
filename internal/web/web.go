// Package web embeds the built SPA (web/dist output of the Vite project,
// see spec «Frontend (SPA) — v1») and serves it single-binary style per
// ADR-0002: static assets by path, index.html fallback for client routes.
//
// The dist/ directory here is build output (gitignored except .placeholder);
// CI builds the SPA before `go build`. A binary compiled without the SPA
// still runs — it answers non-API routes with a hint instead of the app.
package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

func init() {
	// The alpine runtime image has no /etc/mime.types and Go's builtin table
	// lacks .webmanifest — without this the manifest ships as text/plain.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// Handler serves the embedded SPA. Paths under /api/, /docs, /openapi and
// /schemas never reach it (they are routed explicitly before the catch-all);
// anything else gets the static file if one exists, index.html otherwise.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed guarantees dist exists
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(dist, "index.html"); err != nil {
			http.Error(w, "SPA not built: run `npm run build` in web/ and rebuild, or use the Vite dev server (web/: npm run dev)", http.StatusNotImplemented)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(dist, path); err == nil {
				if strings.HasPrefix(path, "assets/") {
					// Content-hashed filenames: safe to cache forever.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					// index.html, sw.js, manifest, icons: revalidate every
					// time so deploys propagate (PWA update flow depends on
					// fresh sw.js/index.html — docs/specs/pwa.md).
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(path, "assets/") {
				// Never answer a missing hashed asset with index.html — a
				// service worker would cache HTML where JS was expected.
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

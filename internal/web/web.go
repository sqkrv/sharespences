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
	"path"
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
	return handlerFor(dist)
}

// handlerFor is Handler's body over an arbitrary FS — the embedded dist/ is
// build output that CI's test job never produces, so the tests drive this.
func handlerFor(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(dist, "index.html"); err != nil {
			http.Error(w, "SPA not built: run `npm run build` in web/ and rebuild, or use the Vite dev server (web/: npm run dev)", http.StatusNotImplemented)
			return
		}
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath != "" {
			if name, ok := resolve(dist, reqPath); ok {
				if strings.HasPrefix(name, "assets/") {
					// Content-hashed filenames: safe to cache forever.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					// index.html, sw.js, manifest, icons and the legal pages:
					// revalidate every time so deploys propagate (the PWA update
					// flow depends on a fresh sw.js/index.html — docs/specs/pwa.md;
					// a policy revision must never come from a stale cache).
					w.Header().Set("Cache-Control", "no-cache")
				}
				r.URL.Path = "/" + name
				fileServer.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(reqPath, "assets/") {
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

// resolve maps a request path to a file in dist, supplying the .html extension
// that a static page's URL omits: /privacy → privacy.html.
//
// The legal pages (web/public/privacy.html, terms.html) are plain documents
// served outside the SPA on purpose — ФЗ-152 ст. 18.1 ч.2 requires the policy
// to be publicly readable, and a client-rendered route answers a no-JS fetch
// (crawlers, РКН's checkers, a broken bundle) with an empty <div id="root">.
//
// ⚠️ A file added to web/public/ therefore SHADOWS the SPA route of the same
// name: dropping home.html in there would take over the app's /home.
//
// fs.Stat rejects paths containing .. elements (fs.ValidPath), so the retry
// cannot escape dist/.
func resolve(dist fs.FS, reqPath string) (string, bool) {
	if _, err := fs.Stat(dist, reqPath); err == nil {
		return reqPath, true
	}
	// Only extensionless paths get the retry — /assets/app-a1b2.js must stay a
	// miss rather than probing app-a1b2.js.html.
	if path.Ext(reqPath) != "" {
		return "", false
	}
	if _, err := fs.Stat(dist, reqPath+".html"); err == nil {
		return reqPath + ".html", true
	}
	return "", false
}

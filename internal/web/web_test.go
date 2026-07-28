package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// The embedded dist/ is build output — CI's test job never runs `npm run
// build`, so these exercise handlerFor against a synthetic FS instead. That
// is also why the extension fallback lives in handlerFor and not in Handler.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html>SPA")},
		"privacy.html":       {Data: []byte("<!doctype html>Политика")},
		"terms.html":         {Data: []byte("<!doctype html>Соглашение")},
		"legal.css":          {Data: []byte("body{}")},
		"assets/app-a1b2.js": {Data: []byte("console.log(1)")},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// The legal pages are reachable at their extensionless URLs: ФЗ-152 ст. 18.1
// ч.2 wants the policy publicly readable, and /privacy is the address printed
// in the documents themselves and in the РКН filing.
func TestExtensionlessLegalPages(t *testing.T) {
	h := handlerFor(testFS())

	for path, want := range map[string]string{
		"/privacy":      "Политика",
		"/terms":        "Соглашение",
		"/privacy.html": "Политика", // the file's own URL keeps working
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if body := rec.Body.String(); !strings.Contains(body, want) {
			t.Errorf("%s: body = %q, want it to contain %q", path, body, want)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("%s: Content-Type = %q, want text/html", path, ct)
		}
		// A policy revision must reach users on the next request, never from
		// a stale cache — everything outside assets/ revalidates.
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, cc)
		}
	}
}

// The fallback must not shadow client-side routing: /lookup has no lookup.html
// and still has to render the SPA.
func TestSPARoutesStillFallThrough(t *testing.T) {
	h := handlerFor(testFS())

	for _, path := range []string{"/", "/lookup", "/periods/new", "/services"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
		if body := rec.Body.String(); !strings.Contains(body, "SPA") {
			t.Errorf("%s: body = %q, want the SPA shell", path, body)
		}
	}
}

// Pre-existing behaviour the refactor must not break.
func TestAssetsUnchanged(t *testing.T) {
	h := handlerFor(testFS())

	rec := get(t, h, "/assets/app-a1b2.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("hashed asset: status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("hashed asset: Cache-Control = %q, want the immutable header", cc)
	}

	// A missing hashed asset must 404, never answer with index.html — a
	// service worker would cache HTML where JS was expected.
	if rec := get(t, h, "/assets/gone-c3d4.js"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset: status = %d, want 404", rec.Code)
	}
	// …and the .html fallback must not resurrect it via assets/gone-c3d4.js.html.
	if rec := get(t, h, "/assets/index"); rec.Code != http.StatusNotFound {
		t.Errorf("assets/ + fallback: status = %d, want 404", rec.Code)
	}
}

// fs.Stat rejects paths with .. elements (fs.ValidPath), so the suffix retry
// cannot be walked out of dist/. Pinned because the fallback is the only place
// that builds a filesystem path out of user input.
func TestTraversalFallsBackToSPA(t *testing.T) {
	h := handlerFor(testFS())

	for _, path := range []string{"/../secret", "/..%2fsecret", "/a/../../etc/passwd"} {
		rec := get(t, h, path)
		if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), "SPA") {
			t.Errorf("%s: served %q, want the SPA shell or an error", path, rec.Body.String())
		}
	}
}

// A binary built without the SPA still runs and says so (the committed
// .placeholder keeps a bare checkout compilable).
func TestUnbuiltSPAHint(t *testing.T) {
	h := handlerFor(fstest.MapFS{".placeholder": {Data: []byte{}}})

	rec := get(t, h, "/privacy")
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

// Handler() itself is only smoke-tested: its FS is the real embed, which is
// empty unless the SPA was built before `go test`.
func TestHandlerBuilds(t *testing.T) {
	if Handler() == nil {
		t.Fatal("Handler() = nil")
	}
	if _, err := fs.Sub(distFS, "dist"); err != nil {
		t.Fatalf("embedded dist/ missing: %v", err)
	}
}

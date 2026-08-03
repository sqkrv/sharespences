package adminweb

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, dist fstest.MapFS, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handlerFor(dist).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestUnbuiltHint(t *testing.T) {
	rec := get(t, fstest.MapFS{".placeholder": &fstest.MapFile{}}, "/")
	if rec.Code != 501 {
		t.Fatalf("unbuilt dist: got %d, want 501", rec.Code)
	}
}

func TestServesAndFallsBack(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<html>admin</html>")},
		"assets/app-ab.js":  &fstest.MapFile{Data: []byte("js")},
	}

	if rec := get(t, dist, "/assets/app-ab.js"); rec.Code != 200 ||
		rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("hashed asset: code %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	// Missing hashed asset must 404, never fall back to HTML.
	if rec := get(t, dist, "/assets/gone.js"); rec.Code != 404 {
		t.Errorf("missing asset: got %d, want 404", rec.Code)
	}
	// Client route falls back to index.html, revalidated.
	rec := get(t, dist, "/pos")
	if rec.Code != 200 || rec.Body.String() != "<html>admin</html>" ||
		rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("SPA fallback: code %d, cache %q, body %q",
			rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
	}
}

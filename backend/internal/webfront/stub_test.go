package webfront

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// stubDir lays out a small site: index.html, a/b.txt and a custom 404.html.
func stubDir(t *testing.T, with404 bool) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{"index.html": "home", "a/b.txt": "nested"}

	if with404 {
		files["404.html"] = "nope"
	}

	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func serve(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

	return rec
}

func TestStubServesIndexAtRoot(t *testing.T) {
	rec := serve(NewStub(stubDir(t, false)), http.MethodGet, "/")
	if rec.Code != http.StatusOK || rec.Body.String() != "home" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestStubServesNestedFiles(t *testing.T) {
	h := NewStub(stubDir(t, false))

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := serve(h, method, "/a/b.txt")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d", method, rec.Code)
		}

		if method == http.MethodGet && rec.Body.String() != "nested" {
			t.Fatalf("body %q", rec.Body.String())
		}

		if method == http.MethodHead && rec.Body.Len() != 0 {
			t.Fatalf("HEAD returned a body")
		}
	}
}

func TestStubRejectsEscapesAndDirectories(t *testing.T) {
	h := NewStub(stubDir(t, false))

	for _, target := range []string{"/../x", "/a/../index.html", "/a/", "/a", "/a/b.txt/", "/%00"} {
		if rec := serve(h, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", target, rec.Code)
		}
	}
}

func TestStubServesCustom404Page(t *testing.T) {
	h := NewStub(stubDir(t, true))

	rec := serve(h, http.MethodGet, "/missing")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "nope" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}

	if rec := serve(h, http.MethodHead, "/missing"); rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: got %d with %d body bytes", rec.Code, rec.Body.Len())
	}
}

func TestStubWithoutADirectoryIs404(t *testing.T) {
	rec := serve(NewStub(filepath.Join(t.TempDir(), "missing")), http.MethodGet, "/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}

	if cl := rec.Header().Get("Content-Length"); cl != "0" {
		t.Fatalf("Content-Length %q", cl)
	}
}

func TestStubRefusesPost(t *testing.T) {
	rec := serve(NewStub(stubDir(t, false)), http.MethodPost, "/")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("got %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

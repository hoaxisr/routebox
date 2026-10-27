package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/config"
)

func listDirRaw(t *testing.T, h *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ListDir(rec, httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+url.QueryEscape(path), nil))
	return rec
}

func listDir(t *testing.T, h *Handler, path string) (int, fsListing) {
	t.Helper()
	rec := listDirRaw(t, h, path)
	var resp struct {
		Data fsListing `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp.Data
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	srs := filepath.Join(dir, "srs")
	for _, d := range []string{srs, filepath.Join(srs, "old"), filepath.Join(dir, "empty")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"B.srs": "12345", "a.json": "{}", "C.SRS": "1", "notes.txt": "x", "bad\xff.srs": "x"}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(srs, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Symlinks are shown as what they point to; a dangling one, or one to a
	// file with another extension, is dropped.
	for name, target := range map[string]string{"link-dir": filepath.Join(srs, "old"), "link.srs": filepath.Join(srs, "B.srs"), "dead.srs": filepath.Join(dir, "nope"), "link.txt": filepath.Join(srs, "notes.txt")} {
		if err := os.Symlink(target, filepath.Join(srs, name)); err != nil {
			t.Fatal(err)
		}
	}
	cm, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cm}

	// Folders first, then .srs/.json (any case) by case-insensitive name; other
	// files and non-UTF-8 names hidden. Trailing slash is cleaned.
	code, got := listDir(t, h, srs+"/")
	if code != http.StatusOK {
		t.Fatalf("code %d", code)
	}
	want := []fsEntry{
		{Name: "link-dir", Dir: true}, {Name: "old", Dir: true},
		{Name: "a.json", Size: 2}, {Name: "B.srs", Size: 5}, {Name: "C.SRS", Size: 1}, {Name: "link.srs", Size: 5},
	}
	if len(got.Entries) != len(want) {
		t.Fatalf("entries %+v, want %+v", got.Entries, want)
	}
	for i := range want {
		if got.Entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got.Entries[i], want[i])
		}
	}
	if got.Path != srs || got.Parent != dir {
		t.Errorf("path %q parent %q, want %q %q", got.Path, got.Parent, srs, dir)
	}

	// Empty path → config dir; relative path resolves against it.
	if _, got := listDir(t, h, ""); got.Path != dir {
		t.Errorf("empty path → %q, want %q", got.Path, dir)
	}
	if _, got := listDir(t, h, "srs"); got.Path != srs {
		t.Errorf("relative path → %q, want %q", got.Path, srs)
	}

	// Root has no parent.
	if _, got := listDir(t, h, "/"); got.Parent != "" {
		t.Errorf("root parent %q", got.Parent)
	}

	// An empty folder is [], not null: the UI reads entries.length.
	if body := listDirRaw(t, h, filepath.Join(dir, "empty")).Body.String(); !strings.Contains(body, `"entries":[]`) {
		t.Errorf("empty dir body %s", body)
	}

	if code, _ := listDir(t, h, filepath.Join(dir, "missing")); code != http.StatusNotFound {
		t.Errorf("missing dir → %d, want 404", code)
	}

	if os.Geteuid() != 0 {
		locked := filepath.Join(dir, "locked")
		if err := os.Mkdir(locked, 0); err != nil {
			t.Fatal(err)
		}
		if code, _ := listDir(t, h, locked); code != http.StatusForbidden {
			t.Errorf("unreadable dir → %d, want 403", code)
		}
	}
}

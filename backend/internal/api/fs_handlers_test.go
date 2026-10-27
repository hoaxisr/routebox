package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/config"
)

func listDir(t *testing.T, h *Handler, path string) (int, fsListing) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ListDir(rec, httptest.NewRequest(http.MethodGet, "/api/fs/list?path="+url.QueryEscape(path), nil))
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
	for _, d := range []string{srs, filepath.Join(srs, "old")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"b.srs": "12345", "A.json": "{}", "notes.txt": "x"} {
		if err := os.WriteFile(filepath.Join(srs, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cm, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cm}

	// Folders first, then .srs/.json by name; other files hidden.
	code, got := listDir(t, h, srs)
	if code != http.StatusOK {
		t.Fatalf("code %d", code)
	}
	want := []fsEntry{{Name: "old", Dir: true}, {Name: "A.json", Size: 2}, {Name: "b.srs", Size: 5}}
	if len(got.Entries) != len(want) {
		t.Fatalf("entries %+v, want %+v", got.Entries, want)
	}
	for i := range want {
		if got.Entries[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got.Entries[i], want[i])
		}
	}
	if got.Parent != dir {
		t.Errorf("parent %q, want %q", got.Parent, dir)
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

	if code, _ := listDir(t, h, filepath.Join(dir, "missing")); code != http.StatusNotFound {
		t.Errorf("missing dir → %d, want 404", code)
	}
}

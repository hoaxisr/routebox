package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
)

// A config path mismatch is a state, not a failure: the request was fine, the
// state forbids it — the same reasoning that already makes a read-only store a
// 409 and "there is no drop-in to remove" a 409. Answering 500 told the client
// RouteBox had broken, and left it no way to tell a state it can resolve (the
// banner offers both cures) from a server that fell over.
func mismatchHandler(t *testing.T) (*Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	ours := filepath.Join(dir, "ours.json")
	if err := os.WriteFile(ours, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(dir, "unit.json")

	cfg, err := config.NewManager(ours)
	if err != nil {
		t.Fatal(err)
	}
	proc := process.NewManagerForTest("amnezia-box", t.TempDir())
	proc.SetConfigPaths(ours, unit)
	return &Handler{config: cfg, process: proc}, ours, unit
}

func TestControlEndpointsAnswer409OnAConfigPathMismatch(t *testing.T) {
	for name, call := range map[string]func(*Handler, http.ResponseWriter, *http.Request){
		"start":   (*Handler).Start,
		"restart": (*Handler).Restart,
		"reload":  (*Handler).Reload,
	} {
		t.Run(name, func(t *testing.T) {
			h, ours, unit := mismatchHandler(t)
			rec := httptest.NewRecorder()
			call(h, rec, httptest.NewRequest(http.MethodPost, "/api/control/"+name, nil))

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
			}
			var resp Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			// Both paths, because the operator has to see which two files
			// disagree before either cure in the banner means anything.
			for _, want := range []string{ours, unit} {
				if !strings.Contains(resp.Error, want) {
					t.Fatalf("error %q must name %q", resp.Error, want)
				}
			}
		})
	}
}

// Everything else these endpoints can fail with is still a 500: only the
// mismatch is a state the client can do something about.
func TestControlEndpointsKeep500ForRealFailures(t *testing.T) {
	h := &Handler{
		config:  config.NewEmptyManager(filepath.Join(t.TempDir(), "config.json")),
		process: process.NewManagerForTest("", t.TempDir()),
	}

	rec := httptest.NewRecorder()
	h.Reload(rec, httptest.NewRequest(http.MethodPost, "/api/control/reload", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a plain failure: %s", rec.Code, rec.Body.String())
	}
}

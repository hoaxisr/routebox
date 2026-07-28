package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
)

// The panel outlives the filesystem it started on: a mount goes read-only, or
// somebody sets chattr +i, while RouteBox is up. Every state store reports that
// the same way — 409 naming the file, and the path in read_only_paths. On the
// live box the config did neither: it answered 500 with a bare errno and kept
// config_read_only false, so the badge said the config was still editable.
func TestSaveConfigAnswers409AfterTheConfigDirGoesReadOnlyUnderIt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg}
	h.statusSource = func() process.Status { return process.Status{} }

	if configReadOnly, paths := statusPaths(t, h); configReadOnly || len(paths) != 0 {
		t.Fatalf("harness: a writable config must report nothing, got %v %v", configReadOnly, paths)
	}

	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	rr := httptest.NewRecorder()
	h.SaveConfig(rr, httptest.NewRequest(http.MethodPut, "/api/config",
		strings.NewReader(`{"log":{"level":"debug"},"inbounds":[],"outbounds":[{"tag":"direct","type":"direct"}]}`)))
	assert409Naming(t, rr, path)

	configReadOnly, paths := statusPaths(t, h)
	if !configReadOnly {
		t.Fatal("the config must be reported read-only once a write proved it is")
	}
	if !slices.Contains(paths, path) {
		t.Fatalf("read_only_paths %v must name the config %q", paths, path)
	}
}

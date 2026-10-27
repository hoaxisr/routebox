package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/config"
)

// A binary swapped by hand for a build older than the config: Start and
// Restart answer with the reason instead of starting a process that dies.
// The preflight answers before the process manager is touched (nil here).
func TestStartRestart_RefuseConfigTheBinaryRejects(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"},{"type":"fallback","tag":"fb","outbounds":["direct"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "amnezia-box")
	script := "#!/bin/sh\necho 'FATAL[0000] decode config at x: outbounds[1]: unknown outbound type: fallback'\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	m.SetCheckBinaryProvider(func() string { return bin })
	h := &Handler{config: m}

	for name, call := range map[string]func(http.ResponseWriter, *http.Request){"start": h.Start, "restart": h.Restart} {
		rr := httptest.NewRecorder()
		call(rr, httptest.NewRequest("POST", "/api/"+name, nil))
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "awgm.35") {
			t.Errorf("%s: status %d body %s", name, rr.Code, rr.Body.String())
		}
	}
}

// The process starts from the file, so the group check reads the file: a
// config edited by hand behind RouteBox's back is what would be started.
func TestStartPreflight_ReadsTheConfigOnDisk(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	m.SetCheckBinaryProvider(func() string { return filepath.Join(dir, "no-binary") })
	if err := os.WriteFile(cfgPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"},{"type":"fallback","tag":"fb","outbounds":["direct"],"interval":"1h"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m}
	rr := httptest.NewRecorder()
	h.Restart(rr, httptest.NewRequest("POST", "/api/control/restart", nil))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "idle_timeout") {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

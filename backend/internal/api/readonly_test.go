package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
)

// readOnlyHandler builds a handler over a config file the process cannot write.
func readOnlyHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"log":{"level":"info"},"outbounds":[{"type":"direct","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(body), 0400); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsReadOnly() {
		t.Fatal("precondition: manager should have detected the unwritable config")
	}
	h := &Handler{config: m}
	h.statusSource = func() process.Status { return process.Status{Running: false} }
	return h, path
}

func TestWriteEndpointAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := readOnlyHandler(t)

	req := httptest.NewRequest("POST", "/api/endpoints/", strings.NewReader(
		`{"type":"wireguard","tag":"wg-out","address":["10.0.0.2/32"],"private_key":"k",
		  "peers":[{"address":"1.2.3.4","port":51820,"public_key":"pk","allowed_ips":["0.0.0.0/0"]}]}`))
	rr := httptest.NewRecorder()
	h.CreateEndpoint(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on a read-only config, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, path) {
		t.Fatalf("error %q must name the config path %q", resp.Error, path)
	}
	if !strings.Contains(resp.Error, "read-only") {
		t.Fatalf("error %q must state the reason", resp.Error)
	}
}

func TestApplyConfigAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := readOnlyHandler(t)

	// Seed a draft directly: the read-only guard rejects draft writes, so the
	// draft has to be planted through the writable-mode API first.
	h.config.SetReadOnly(false)
	if err := h.config.SetDraft(map[string]interface{}{
		"log":       map[string]interface{}{"level": "debug"},
		"outbounds": []interface{}{map[string]interface{}{"type": "direct", "tag": "direct"}},
	}); err != nil {
		t.Fatal(err)
	}
	h.config.SetReadOnly(true)

	req := httptest.NewRequest("POST", "/api/config/save", nil)
	rr := httptest.NewRecorder()
	h.SaveConfigDraft(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on a read-only config, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), path) {
		t.Fatalf("response %q must name the config path %q", rr.Body.String(), path)
	}
}

// newReadOnlyAWGHandler wires the singbox AWG backend against a REAL read-only
// config manager, so the whole chain (endpoint sync → awg manager → handler) is
// exercised the way it runs in production.
func newReadOnlyAWGHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0400); err != nil {
		t.Fatal(err)
	}
	cfgMgr, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	// The secret store lives elsewhere and IS writable — that is exactly the
	// trap: peers can be persisted while the endpoint never reaches sing-box.
	awgDir := filepath.Join(dir, "amneziawg")
	if err := os.MkdirAll(awgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := awg.NewManagerForTest(awgNoopRunner{}, awgDir, serverPriv, awg.Config{
		Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1",
		ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"},
	})
	m.SetBackend("singbox")
	m.SetConfigSync(cfgMgr, func() error { return nil }, func() bool { return true })

	h := &Handler{config: cfgMgr, settings: newAWGSettings(t, dir, "vpn.example.com")}
	h.SetAWG(m)
	return h, path
}

func TestCreateAWGPeerAnswers409AndKeepsNoPeerWhenConfigIsReadOnly(t *testing.T) {
	h, path := newReadOnlyAWGHandler(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(`{"name":"alice"}`))
	h.CreateAWGPeer(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), path) {
		t.Fatalf("response %q must name the config path", rec.Body.String())
	}
	if peers := h.awg.ListPeers(req.Context()); len(peers) != 0 {
		t.Fatalf("panel would show %d peer(s) that never reached sing-box", len(peers))
	}
}

func TestEnableAWGAnswers409WhenConfigIsReadOnly(t *testing.T) {
	h, path := newReadOnlyAWGHandler(t)

	rec := httptest.NewRecorder()
	h.EnableAWG(rec, httptest.NewRequest(http.MethodPost, "/api/awg/enable", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), path) {
		t.Fatalf("response %q must name the config path", rec.Body.String())
	}
}

func TestStatusReportsReadOnlyConfig(t *testing.T) {
	h, path := readOnlyHandler(t)

	rr := httptest.NewRecorder()
	h.GetStatus(rr, httptest.NewRequest("GET", "/api/status", nil))

	var resp struct {
		Data struct {
			ConfigReadOnly     bool   `json:"config_read_only"`
			ConfigReadOnlyPath string `json:"config_read_only_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.ConfigReadOnly {
		t.Fatalf("status must report config_read_only=true, got %s", rr.Body.String())
	}
	if resp.Data.ConfigReadOnlyPath != path {
		t.Fatalf("status must name the config path, got %q want %q", resp.Data.ConfigReadOnlyPath, path)
	}
}

func TestStatusReportsWritableConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"level":"info"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: m}
	h.statusSource = func() process.Status { return process.Status{Running: false} }

	rr := httptest.NewRecorder()
	h.GetStatus(rr, httptest.NewRequest("GET", "/api/status", nil))

	var resp struct {
		Data struct {
			ConfigReadOnly     bool   `json:"config_read_only"`
			ConfigReadOnlyPath string `json:"config_read_only_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ConfigReadOnly || resp.Data.ConfigReadOnlyPath != "" {
		t.Fatalf("a writable config must not be reported as read-only: %s", rr.Body.String())
	}
}

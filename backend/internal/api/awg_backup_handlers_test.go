package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/settings"
)

func TestAWGBackupExportsSecretsAsAttachment(t *testing.T) {
	_, r := newAWGTestHandler(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/backup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "awg-backup") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store (the body is secrets)", cc)
	}
	var b awg.Backup
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Version != 1 || b.ServerKey != serverPriv || len(b.Peers) != 1 || b.Peers[0].PrivateKey != knownPriv {
		t.Fatalf("backup = %+v", b)
	}
	if b.Settings.Subnet == "" || b.Settings.Enabled {
		t.Fatalf("settings = %+v", b.Settings)
	}
}

func TestAWGRestoreReplacesPeersAndSettings(t *testing.T) {
	h, r := newAWGTestHandler(t)
	// Take a backup, move it to another subnet with a different peer — as if
	// it came from a different server.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/backup", nil))
	var b awg.Backup
	json.Unmarshal(rec.Body.Bytes(), &b)
	b.Settings.Subnet = "10.20.0.0/24"
	b.Settings.ServerHost = "old.example.org"
	b.Settings.ListenPort, b.Settings.DNS, b.Settings.Obf.H1, b.Settings.Backend = 51999, []string{"9.9.9.9"}, "7-9", "kernel"
	b.Settings.Enabled, b.Settings.Configured = true, true // must be ignored
	// The target had the server flagged enabled in settings (stale flag after a
	// crash): restore must leave it false, whatever the backup says.
	if err := h.settings.Update(map[string]interface{}{"awg.enabled": true}); err != nil {
		t.Fatal(err)
	}
	b.Peers = []awg.Peer{{PublicKey: validButUnknown, PrivateKey: knownPriv, PresharedKey: knownPSK, Address: "10.20.0.7/32", Name: "laptop", ExpiresAt: 99}}
	body, _ := json.Marshal(b)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/restore", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Data struct {
			Peers int `json:"peers"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Data.Peers != 1 {
		t.Fatalf("peers = %d body=%s", resp.Data.Peers, rec.Body)
	}
	if _, ok := h.awg.Store().Get(knownPub); ok {
		t.Fatal("old peer must be gone")
	}
	if p, ok := h.awg.Store().Get(validButUnknown); !ok || p.Name != "laptop" || p.ExpiresAt != 99 {
		t.Fatalf("restored peer = %+v ok=%v", p, ok)
	}
	got := h.settings.Get().Awg
	if got.Subnet != "10.20.0.0/24" || got.ServerHost != "old.example.org" || got.ListenPort != 51999 ||
		len(got.DNS) != 1 || got.DNS[0] != "9.9.9.9" || got.Obf.H1 != "7-9" || got.Backend != "kernel" {
		t.Fatalf("settings not restored: %+v", got)
	}
	if got.Enabled || !got.Configured {
		t.Fatalf("enabled must stay false and configured become true: %+v", got)
	}
	// The live Manager must follow the backend too, not only the file.
	if h.awg.BackendName() != "kernel" {
		t.Fatalf("manager backend = %q, want kernel", h.awg.BackendName())
	}
	// And all of it is on disk, not just staged in memory.
	reload, err := settings.NewManager(h.settings.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := reload.Load(); err != nil {
		t.Fatal(err)
	}
	if d := reload.Get().Awg; d.Subnet != "10.20.0.0/24" || !d.Configured || d.Enabled {
		t.Fatalf("settings on disk = %+v", d)
	}
}

// Restore is refused while the server is up — at the HTTP layer, as 409, so the
// UI can tell "disable first" from "your file is broken".
func TestAWGRestoreRefusedWhileEnabled(t *testing.T) {
	h, _ := newAWGSingboxHandler(t)
	rec := httptest.NewRecorder()
	h.EnableAWG(rec, httptest.NewRequest(http.MethodPost, "/api/awg/enable", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.GetAWGBackup(rec, httptest.NewRequest(http.MethodGet, "/api/awg/backup", nil))
	body := rec.Body.Bytes()
	rec = httptest.NewRecorder()
	h.RestoreAWGBackup(rec, httptest.NewRequest(http.MethodPost, "/api/awg/restore", bytes.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s, want 409", rec.Code, rec.Body)
	}
}

// A kernel host that never enabled the server has no key to export; a file
// with server_key "" would only be rejected on the target.
func TestAWGBackupRefusesColdServer(t *testing.T) {
	dir := t.TempDir()
	m := awg.NewManagerForTest(awgNoopRunner{}, filepath.Join(dir, "amneziawg"), "", awg.Config{Iface: "awg-rb0", Subnet: "10.10.0.0/24", ServerIP: "10.10.0.1", ListenPort: 51820})
	h := &Handler{settings: newAWGSettings(t, dir, "vpn.example.com")}
	h.SetAWG(m)
	rec := httptest.NewRecorder()
	h.GetAWGBackup(rec, httptest.NewRequest(http.MethodGet, "/api/awg/backup", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s, want 409", rec.Code, rec.Body)
	}
}

func TestAWGRestoreRejectsBadBackup(t *testing.T) {
	h, r := newAWGTestHandler(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/backup", nil))
	var b awg.Backup
	json.Unmarshal(rec.Body.Bytes(), &b)
	b.Peers[0].Address = "192.168.5.5/32" // outside the subnet
	body, _ := json.Marshal(b)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/restore", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if _, ok := h.awg.Store().Get(knownPub); !ok {
		t.Fatal("a rejected restore must leave the store alone")
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/restore", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage body: status = %d", rec.Code)
	}

	// A settings value the form would reject must fail BEFORE the store is
	// replaced — otherwise the old peers are gone and the subnet is still old.
	b.Peers = []awg.Peer{{PublicKey: validButUnknown, PrivateKey: knownPriv, Address: "10.10.0.3/32", Name: "laptop"}}
	b.Settings.ServerHost = "bad host!"
	body, _ = json.Marshal(b)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/restore", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad settings: status = %d body=%s", rec.Code, rec.Body)
	}
	if _, ok := h.awg.Store().Get(knownPub); !ok {
		t.Fatal("store must be untouched when the settings are rejected")
	}
	if got := h.settings.Get().Awg; got.Configured || got.ServerHost == "bad host!" {
		t.Fatalf("settings must be untouched when rejected: %+v", got)
	}
}

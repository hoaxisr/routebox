package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"routebox/backend/internal/awg"
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
	b.Settings.Enabled, b.Settings.Configured = true, true // must be ignored
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
	if got.Subnet != "10.20.0.0/24" || got.ServerHost != "old.example.org" {
		t.Fatalf("settings not restored: %+v", got)
	}
	if got.Enabled || !got.Configured {
		t.Fatalf("enabled must stay false and configured become true: %+v", got)
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
}

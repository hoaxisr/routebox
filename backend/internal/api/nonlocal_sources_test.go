package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"routebox/backend/internal/clients"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/traffic"
)

// Issue #102: sing-box reports the occasional connection whose sourceIP is a
// remote address (a Google front-end, in the report). Both the sampler and
// client discovery record every source verbatim, so those addresses became rows
// on Breakdown and entries on Clients — devices nobody owns and nobody can name.
// Both views now read through util.IsLocalClientIP.
func TestNonLocalSourcesAreNotDevices(t *testing.T) {
	dir := t.TempDir()
	store, err := traffic.OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	const lan, tunnel, remote = "192.168.1.14", "10.10.64.2", "172.217.116.4"
	bucket := (time.Now().Unix()/60)*60 - 120
	for _, src := range []string{lan, tunnel, remote, ""} {
		if err := store.Upsert(bucket, src, "example.com", "direct", 100, 200); err != nil {
			t.Fatalf("Upsert %s: %v", src, err)
		}
	}

	h := &Handler{traffic: store, clients: clients.New(filepath.Join(dir, "clients.toml"))}
	for _, src := range []string{lan, tunnel, remote} {
		h.clients.Observe(src, time.Now())
	}

	rec := httptest.NewRecorder()
	h.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history?range=1h", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("history status %d: %s", rec.Code, rec.Body)
	}
	var history struct {
		Data struct {
			Buckets []struct {
				Source string `json:"source"`
			} `json:"buckets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, b := range history.Data.Buckets {
		got[b.Source] = true
	}
	if !got[lan] || !got[tunnel] {
		t.Errorf("breakdown dropped a real device: %v", got)
	}
	if got[remote] {
		t.Errorf("breakdown kept the remote source %s", remote)
	}
	// Sing-box's own dials have no source and have shown as "unknown" since long
	// before this filter; hiding them would quietly shrink the totals.
	if !got[""] {
		t.Error("breakdown dropped the sourceless rows the UI calls unknown")
	}

	// A panel records remote clients on purpose: the filter is router-only.
	vps := &Handler{traffic: store, clients: h.clients}
	vps.SetPanelMode("vps")
	rec = httptest.NewRecorder()
	vps.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history?range=1h", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("vps history status %d: %s", rec.Code, rec.Body)
	}
	var vpsHistory struct {
		Data struct {
			Buckets []struct {
				Source string `json:"source"`
			} `json:"buckets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vpsHistory); err != nil {
		t.Fatal(err)
	}
	if len(vpsHistory.Data.Buckets) != 4 {
		t.Errorf("vps buckets = %d, want all four sources", len(vpsHistory.Data.Buckets))
	}

	// Asking for one source explicitly does not lift the filter: the remote
	// source has rows in the DB, and answering with them would put back on the
	// page exactly what the filter took off it.
	rec = httptest.NewRecorder()
	h.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history?range=1h&source="+remote, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered history status %d: %s", rec.Code, rec.Body)
	}
	var asked struct {
		Data struct {
			Buckets []struct {
				Source string `json:"source"`
			} `json:"buckets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &asked); err != nil {
		t.Fatal(err)
	}
	if len(asked.Data.Buckets) != 0 {
		t.Errorf("?source=%s returned %d buckets, want none", remote, len(asked.Data.Buckets))
	}

	// The dashboard graph is the same endpoint with ?series=1, and it must keep
	// every source: VPS mode shows that graph too, and there every byte comes
	// from an address the filter above rejects.
	rec = httptest.NewRecorder()
	h.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history?range=1h&series=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("series status %d: %s", rec.Code, rec.Body)
	}
	var withSeries struct {
		Data struct {
			Series []struct {
				Upload   int64 `json:"upload"`
				Download int64 `json:"download"`
			} `json:"series"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &withSeries); err != nil {
		t.Fatal(err)
	}
	var up, down int64
	for _, p := range withSeries.Data.Series {
		up += p.Upload
		down += p.Download
	}
	if up != 400 || down != 800 {
		t.Errorf("series = %d/%d bytes, want every source (400/800)", up, down)
	}

	rec = httptest.NewRecorder()
	h.ListClients(rec, httptest.NewRequest(http.MethodGet, "/api/clients", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("clients status %d: %s", rec.Code, rec.Body)
	}
	var roster struct {
		Data []struct {
			IP string `json:"ip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &roster); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range roster.Data {
		seen[c.IP] = true
	}
	if !seen[lan] || !seen[tunnel] {
		t.Errorf("clients dropped a real device: %v", seen)
	}
	if seen[remote] {
		t.Errorf("clients kept the remote source %s", remote)
	}
}

// The AWG tunnel subnet is a setting and need not be private — AmneziaWG's own
// docs use 26.26.26.0/24. A peer there is still a device of this box, so both
// reads have to widen the locality test with what the settings say; a filter
// that only knew RFC1918 would erase every peer of such a deployment from
// Breakdown and from the roster (#102 review).
func TestConfiguredTunnelSubnetCountsAsLocal(t *testing.T) {
	dir := t.TempDir()
	store, err := traffic.OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	const peer, remote = "26.26.26.5", "172.217.116.4"
	bucket := (time.Now().Unix()/60)*60 - 120
	for _, src := range []string{peer, remote} {
		if err := store.Upsert(bucket, src, "example.com", "direct", 100, 200); err != nil {
			t.Fatalf("Upsert %s: %v", src, err)
		}
	}

	settingsPath := filepath.Join(dir, "routebox.toml")
	if err := os.WriteFile(settingsPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	sm, err := settings.NewManager(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	if err := sm.Update(map[string]interface{}{"awg.subnet": "26.26.26.0/24"}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{traffic: store, settings: sm, clients: clients.New(filepath.Join(dir, "clients.toml"))}
	for _, ip := range []string{peer, remote} {
		h.clients.Observe(ip, time.Now())
	}

	rec := httptest.NewRecorder()
	h.GetTrafficHistory(rec, httptest.NewRequest(http.MethodGet, "/api/traffic/history?range=1h", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("history status %d: %s", rec.Code, rec.Body)
	}
	var history struct {
		Data struct {
			Buckets []struct {
				Source string `json:"source"`
			} `json:"buckets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, b := range history.Data.Buckets {
		seen[b.Source] = true
	}
	if !seen[peer] {
		t.Errorf("breakdown dropped the peer on the configured subnet: %v", seen)
	}
	if seen[remote] {
		t.Errorf("breakdown kept the remote source %s", remote)
	}

	rec = httptest.NewRecorder()
	h.ListClients(rec, httptest.NewRequest(http.MethodGet, "/api/clients", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("clients status %d: %s", rec.Code, rec.Body)
	}
	var roster struct {
		Data []struct {
			IP string `json:"ip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &roster); err != nil {
		t.Fatal(err)
	}
	inRoster := map[string]bool{}
	for _, c := range roster.Data {
		inRoster[c.IP] = true
	}
	if !inRoster[peer] || inRoster[remote] {
		t.Errorf("roster = %v, want only the peer", inRoster)
	}
}

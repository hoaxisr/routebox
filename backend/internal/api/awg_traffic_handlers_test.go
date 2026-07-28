package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/traffic"
)

// The peer address is stored masked ("10.10.64.2/32") while traffic_minute keys
// connections by the bare IP — get this wrong and every peer reads as zero.
func TestPeerSource(t *testing.T) {
	cases := map[string]string{
		"10.10.64.2/32":    "10.10.64.2",
		"10.10.64.2":       "10.10.64.2",
		"fd00:dead::2/128": "fd00:dead::2",
		"":                 "",
		"not-an-address":   "not-an-address",
	}
	for in, want := range cases {
		if got := peerSource(in); got != want {
			t.Errorf("peerSource(%q) = %q, want %q", in, got, want)
		}
	}
}

// Issue #40: the peer must arrive with the same totals+series shape the monitor
// page already renders for panel users, read out of traffic_minute under its
// tunnel IP.
func TestGetAWGPeersTraffic(t *testing.T) {
	// Serve through the SHARED harness router, which mirrors main.go: that is
	// what pins the route registration and its precedence over /peers/{publicKey}.
	h, router := newAWGTestHandler(t) // seeds peer "phone" at 10.10.0.2/32
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	h.traffic = store

	now := time.Now().Unix()
	bucket := now - 60
	if err := store.Upsert(bucket, "10.10.0.2", "a.example", "direct", 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(bucket, "10.10.0.2", "b.example", "proxy", 5, 5); err != nil {
		t.Fatal(err)
	}
	// A different source (a LAN client) must not be attributed to the peer.
	if err := store.Upsert(bucket, "192.168.1.7", "a.example", "direct", 900, 900); err != nil {
		t.Fatal(err)
	}
	// Older than the requested window.
	if err := store.Upsert(now-7200, "10.10.0.2", "a.example", "direct", 77, 77); err != nil {
		t.Fatal(err)
	}

	got := peerTrafficRows(t, router, "?range=1h")
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(got), got)
	}
	row := got[0]
	if row.Name != "phone" || row.Address != "10.10.0.2/32" || row.Source != "10.10.0.2" {
		t.Fatalf("identity fields wrong: %+v", row)
	}
	// public_key is the row key on the monitor page and online drives its dot.
	if row.PublicKey != knownPub {
		t.Fatalf("public_key = %q, want %q", row.PublicKey, knownPub)
	}
	live := h.awg.ListPeers(context.Background())
	if row.Online != live[0].Online || row.LastHandshake != live[0].LastHandshake {
		t.Fatalf("liveness fields = %v/%d, want %v/%d", row.Online, row.LastHandshake, live[0].Online, live[0].LastHandshake)
	}
	if row.Upload != 15 || row.Download != 25 {
		t.Fatalf("totals = %d/%d, want 15/25 (LAN client and out-of-window rows must not count)", row.Upload, row.Download)
	}
	if len(row.History) != 1 || row.History[0].BucketTs != bucket ||
		row.History[0].Upload != 15 || row.History[0].Download != 25 {
		t.Fatalf("history = %+v, want one collapsed bucket at %d with 15/25", row.History, bucket)
	}

	// No ?range= must mean 24h, not "whatever": the 2h-old bucket excluded above
	// is inside the default window, so the totals grow by exactly it.
	if rows := peerTrafficRows(t, router, ""); rows[0].Upload != 92 || rows[0].Download != 102 {
		t.Fatalf("default range totals = %d/%d, want 92/102 (24h includes the 2h-old bucket)",
			rows[0].Upload, rows[0].Download)
	}
	// And an unparseable range falls back to that same default rather than 400ing
	// (matching the sibling per-user endpoint).
	if rows := peerTrafficRows(t, router, "?range=nonsense"); rows[0].Upload != 92 {
		t.Fatalf("invalid range totals = %d, want the 24h default (92)", rows[0].Upload)
	}
}

// peerTrafficRows GETs the peers-traffic endpoint through router and decodes it.
func peerTrafficRows(t *testing.T, router http.Handler, query string) []awgPeerTrafficRow {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/traffic"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200; body=%q", query, rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []awgPeerTrafficRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%q", err, rec.Body.String())
	}
	return resp.Data
}

// A store that cannot answer must surface as an error, not as a peer that looks
// idle — "0 bytes" is exactly the bug this endpoint exists to fix.
func TestGetAWGPeersTrafficStoreErrorIs500(t *testing.T) {
	h, router := newAWGTestHandler(t)
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	store.Close() // every query from here on errors
	h.traffic = store

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/traffic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%q", rec.Code, rec.Body.String())
	}
}

// An empty peer list must serialise as [] — the monitor page maps over it.
func TestGetAWGPeersTrafficEmptyList(t *testing.T) {
	h, router := newAWGTestHandler(t)
	if err := h.awg.Store().Delete(knownPub); err != nil {
		t.Fatal(err)
	}
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	h.traffic = store

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/traffic", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"data":[]`) {
		t.Fatalf("body = %q, want an empty array (null breaks the page's .map)", body)
	}
}

// A peer with no address has no source key to look up: it must still be listed,
// zeroed, rather than dragging in every row whose source is "".
func TestGetAWGPeersTrafficAddresslessPeer(t *testing.T) {
	h, router := newAWGTestHandler(t)
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	h.traffic = store
	if err := h.awg.Store().Put(awg.Peer{PublicKey: "Zluwfrt+6ChDx8TJcdmDuw63AdoQDqA18LMVPr5b4Ks=", Name: "broken", Address: ""}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(time.Now().Unix()-60, "", "a.example", "direct", 42, 42); err != nil {
		t.Fatal(err)
	}

	for _, row := range peerTrafficRows(t, router, "") {
		if row.Name != "broken" {
			continue
		}
		if row.Source != "" || row.Upload != 0 || row.Download != 0 || len(row.History) != 0 {
			t.Fatalf("addressless peer picked up traffic: %+v", row)
		}
		return
	}
	t.Fatal("addressless peer missing from the response")
}

// Without a traffic store the endpoint still lists the peers — the AWG page must
// not go blank just because history is unavailable.
func TestGetAWGPeersTrafficWithoutStore(t *testing.T) {
	h, _ := newAWGTestHandler(t)
	h.traffic = nil

	r := chi.NewRouter()
	r.Get("/api/awg/peers/traffic", h.GetAWGPeersTraffic)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers/traffic", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []awgPeerTrafficRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "phone" {
		t.Fatalf("rows = %+v, want the peer with zeroed traffic", resp.Data)
	}
	if resp.Data[0].Upload != 0 || resp.Data[0].History == nil {
		t.Fatalf("want zeroed totals and a non-nil history array: %+v", resp.Data[0])
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/clients"
	"routebox/backend/internal/traffic"
)

// Issue #19: deleting a client on /config/clients left its rows in
// traffic_minute, so the Breakdown panel kept showing the IP and kept counting
// its bytes in the total. The AWG peer-removal path purged them; this one never
// did — which is why the fix shipped in 0.31.0 did not cover a delete made from
// the Clients page, for a tunnel IP or a LAN one.
func TestDeleteClientPurgesBreakdownHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := traffic.OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	const gone, kept = "10.10.64.2", "192.168.1.50"
	for _, src := range []string{gone, kept} {
		if err := store.Upsert(60, src, "example.com", "direct", 100, 200); err != nil {
			t.Fatalf("Upsert %s: %v", src, err)
		}
	}

	h := &Handler{traffic: store, clients: clients.New(filepath.Join(dir, "clients.toml"))}
	h.clients.Observe(gone, time.Unix(60, 0))
	h.clients.Observe(kept, time.Unix(60, 0))

	r := chi.NewRouter()
	r.Delete("/api/clients/{ip}", h.DeleteClient)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/clients/"+gone, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%q", rec.Code, rec.Body.String())
	}

	rows, err := store.QueryAggregate(0, 120, "", "", "")
	if err != nil {
		t.Fatalf("QueryAggregate: %v", err)
	}
	for _, row := range rows {
		if row.Source == gone {
			t.Fatalf("source %s still in traffic history after client delete: %+v", gone, row)
		}
	}
	var keptSeen bool
	for _, row := range rows {
		if row.Source == kept {
			keptSeen = true
		}
	}
	if !keptSeen {
		t.Fatalf("purge removed the wrong rows: %s is gone too", kept)
	}
}

// The purge is best-effort: the client is already gone from clients.toml by the
// time it runs, so a broken traffic DB (or none at all) must not turn a
// completed delete into a 500 the panel reports as failure.
func TestDeleteClientSurvivesAnUnusableTrafficStore(t *testing.T) {
	dir := t.TempDir()
	broken, err := traffic.OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	broken.Close() // every query from here on errors

	for _, tc := range []struct {
		name  string
		store *traffic.Store
	}{
		{"closed store", broken},
		{"no traffic store wired", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{traffic: tc.store, clients: clients.New(filepath.Join(t.TempDir(), "clients.toml"))}
			h.clients.Observe("10.10.64.9", time.Unix(60, 0))

			r := chi.NewRouter()
			r.Delete("/api/clients/{ip}", h.DeleteClient)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/clients/10.10.64.9", nil))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body=%q", rec.Code, rec.Body.String())
			}
			if _, ok := h.clients.Get("10.10.64.9"); ok {
				t.Fatal("client survived the delete")
			}
		})
	}
}

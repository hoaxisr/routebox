package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"routebox/backend/internal/traffic"
)

func TestFormatUserinfo(t *testing.T) {
	cases := []struct {
		up, down, total, expire int64
		want                    string
	}{
		{0, 0, 0, 0, "upload=0; download=0; total=0; expire=0"},
		{123, 456, 0, 0, "upload=123; download=456; total=0; expire=0"},
		{111, 222, 0, 1893456000, "upload=111; download=222; total=0; expire=1893456000"},
	}
	for _, c := range cases {
		if got := formatUserinfo(c.up, c.down, c.total, c.expire); got != c.want {
			t.Errorf("formatUserinfo(%d,%d,%d,%d) = %q, want %q",
				c.up, c.down, c.total, c.expire, got, c.want)
		}
	}
}

// TestSub_UserinfoHeader_FromTrafficStore proves the Subscription-Userinfo
// header is actually wired on the 200 path: the bytes come from h.traffic
// summed across the user's names, total is pinned to 0 (no quota), and expire
// reflects the registry user's ExpiresAt.
func TestSub_UserinfoHeader_FromTrafficStore(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")

	// Seed the reconciled user (Name "alice") with a non-zero ExpiresAt.
	u := um.List()[0]
	u.ExpiresAt = 1893456000
	if err := um.Put(&u); err != nil {
		t.Fatal(err)
	}

	// Wire a traffic store seeded under the user's display name.
	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if err := store.UpsertUser(60, "alice", 111, 222); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	h.traffic = store

	rec := serveSub(h, u.Token, "203.0.113.11")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	got := rec.Header().Get("Subscription-Userinfo")
	want := "upload=111; download=222; total=0; expire=1893456000"
	if got != want {
		t.Errorf("Subscription-Userinfo = %q, want %q", got, want)
	}
}

// TestSub_UserinfoHeader_NoTraffic proves the header is present with zeroed
// usage when the user has no recorded traffic (and no ExpiresAt set).
func TestSub_UserinfoHeader_NoTraffic(t *testing.T) {
	h, um := newSubHandler(t, "vpn.example.com")

	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	h.traffic = store

	rec := serveSub(h, tokenOf(t, um), "203.0.113.12")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	got := rec.Header().Get("Subscription-Userinfo")
	want := "upload=0; download=0; total=0; expire=0"
	if got != want {
		t.Errorf("Subscription-Userinfo = %q, want %q", got, want)
	}
}

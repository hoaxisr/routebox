package mtproto

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()

	s := NewStore(filepath.Join(t.TempDir(), "mtproto.toml"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load on a missing file must not error: %v", err)
	}

	return s
}

func mustPut(t *testing.T, s *Store, c Client) {
	t.Helper()

	if err := s.Put(c); err != nil {
		t.Fatalf("Put(%q): %v", c.Name, err)
	}
}

func TestPutAndGetRoundTrip(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "alice", Secret: "00112233445566778899aabbccddeeff", CreatedAt: 100})

	got, ok := s.Get("alice")
	if !ok {
		t.Fatal("Get(alice) missing after Put")
	}

	if got.Secret != "00112233445566778899aabbccddeeff" {
		t.Errorf("secret = %q, want the one that was stored", got.Secret)
	}
}

func TestPutPersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mtproto.toml")

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}

	mustPut(t, s, Client{Name: "bob", Secret: "00112233445566778899aabbccddeeff", Enabled: true, ExpiresAt: 42})

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}

	got, ok := reloaded.Get("bob")
	if !ok {
		t.Fatal("bob did not survive a reload")
	}

	if !got.Enabled || got.ExpiresAt != 42 {
		t.Errorf("got %+v, want enabled with expiry 42 — every field must round-trip", got)
	}
}

func TestTheFileIsNotReadableByOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mtproto.toml")

	s := NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}

	mustPut(t, s, Client{Name: "carol", Secret: "00112233445566778899aabbccddeeff"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// This file holds credentials, so it holds the same bar as awg/peers.toml.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %#o, want 0600", perm)
	}
}

func TestPutRejectsABlankName(t *testing.T) {
	s := tempStore(t)

	// The name is the store key and the display label; blank breaks both.
	if err := s.Put(Client{Name: "", Secret: "00112233445566778899aabbccddeeff"}); err == nil {
		t.Error("a blank name must be rejected")
	}
}

func TestPutReplacesAnExistingName(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "dave", Secret: "00112233445566778899aabbccddeeff", CreatedAt: 1})
	mustPut(t, s, Client{Name: "dave", Secret: "ffeeddccbbaa99887766554433221100", CreatedAt: 2})

	if got := s.List(); len(got) != 1 {
		t.Fatalf("List len = %d, want 1 — names are unique", len(got))
	}

	if c, _ := s.Get("dave"); c.Secret != "ffeeddccbbaa99887766554433221100" {
		t.Errorf("secret = %q, want the replacement", c.Secret)
	}
}

func TestDeleteRemovesTheClient(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "erin", Secret: "00112233445566778899aabbccddeeff"})

	if err := s.Delete("erin"); err != nil {
		t.Fatal(err)
	}

	if _, ok := s.Get("erin"); ok {
		t.Error("erin still present after Delete")
	}
}

func TestDeletingAnUnknownClientIsNotAnError(t *testing.T) {
	s := tempStore(t)

	if err := s.Delete("nobody"); err != nil {
		t.Errorf("Delete of an unknown name returned %v, want nil", err)
	}
}

func TestListIsSortedByName(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "zoe", Secret: "00112233445566778899aabbccddeeff"})
	mustPut(t, s, Client{Name: "adam", Secret: "ffeeddccbbaa99887766554433221100"})

	got := s.List()
	if len(got) != 2 || got[0].Name != "adam" {
		t.Errorf("List = %+v, want adam first — the roster order must be stable", got)
	}
}

func TestGetReturnsACopy(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "frank", Secret: "00112233445566778899aabbccddeeff", Enabled: true})

	got, _ := s.Get("frank")
	got.Enabled = false
	got.Secret = "tampered"

	again, _ := s.Get("frank")
	if !again.Enabled || again.Secret == got.Secret {
		t.Errorf("mutating a returned Client changed the store: %+v", again)
	}
}

func TestActiveSkipsDisabledAndExpired(t *testing.T) {
	s := tempStore(t)
	now := time.Unix(1_000_000, 0)

	mustPut(t, s, Client{Name: "on", Secret: "00112233445566778899aabbccddeeff", Enabled: true})
	mustPut(t, s, Client{Name: "off", Secret: "11112233445566778899aabbccddeeff", Enabled: false})
	mustPut(t, s, Client{Name: "lapsed", Secret: "22112233445566778899aabbccddeeff", Enabled: true, ExpiresAt: now.Unix() - 1})
	mustPut(t, s, Client{Name: "future", Secret: "33112233445566778899aabbccddeeff", Enabled: true, ExpiresAt: now.Unix() + 1})

	got := s.Active(now)

	if len(got) != 2 {
		t.Fatalf("Active = %+v, want the enabled unexpired pair", got)
	}

	if got[0].Name != "future" || got[1].Name != "on" {
		t.Errorf("Active = %+v, want future and on, sorted", got)
	}
}

func TestActiveTreatsZeroExpiryAsNever(t *testing.T) {
	s := tempStore(t)
	mustPut(t, s, Client{Name: "forever", Secret: "00112233445566778899aabbccddeeff", Enabled: true, ExpiresAt: 0})

	if got := s.Active(time.Unix(1<<40, 0)); len(got) != 1 {
		t.Errorf("Active = %+v, want the never-expiring client far in the future", got)
	}
}

func TestActiveExpiresExactlyAtTheDeadline(t *testing.T) {
	s := tempStore(t)
	deadline := int64(1_000_000)
	mustPut(t, s, Client{Name: "edge", Secret: "00112233445566778899aabbccddeeff", Enabled: true, ExpiresAt: deadline})

	if got := s.Active(time.Unix(deadline-1, 0)); len(got) != 1 {
		t.Error("a client must still be active one second before its expiry")
	}

	if got := s.Active(time.Unix(deadline, 0)); len(got) != 0 {
		t.Error("a client must be inactive once the clock reaches its expiry")
	}
}

func TestAPathlessStoreIsNeverReadOnly(t *testing.T) {
	s := NewStore("")

	if s.IsReadOnly() {
		t.Error("an unpersisted store (tests) must not report read-only")
	}
}

func TestLoadOnAMalformedFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mtproto.toml")
	if err := os.WriteFile(path, []byte("this is not toml ]["), 0o600); err != nil {
		t.Fatal(err)
	}

	// Silently starting with an empty roster would revoke every client at once.
	if err := NewStore(path).Load(); err == nil {
		t.Error("a malformed file must be an error, not an empty roster")
	}
}

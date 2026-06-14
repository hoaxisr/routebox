package users

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateToken_UniqueURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
		// base64url(32 bytes) raw (no padding) = 43 chars, URL-safe alphabet only.
		if len(tok) != 43 {
			t.Fatalf("token len = %d, want 43 (%q)", len(tok), tok)
		}
		if strings.ContainsAny(tok, "+/=") {
			t.Fatalf("token not URL-safe/raw: %q", tok)
		}
	}
}

func TestByToken_FoundReturnsDeepCopy(t *testing.T) {
	m := NewManager("") // no persistence
	u := &PanelUser{ID: "u1", Name: "alice", Token: "tok-abc",
		Bindings: []Binding{{InboundTag: "in", Credential: "c1", Protocol: "vless"}}}
	if err := m.Put(u); err != nil {
		t.Fatal(err)
	}
	got, ok := m.ByToken("tok-abc")
	if !ok {
		t.Fatal("ByToken: not found")
	}
	if got.ID != "u1" || got.Name != "alice" {
		t.Fatalf("ByToken returned wrong user: %+v", got)
	}
	// Mutating the returned copy must not affect the registry.
	got.Bindings[0].Credential = "HACKED"
	again, _ := m.ByToken("tok-abc")
	if again.Bindings[0].Credential != "c1" {
		t.Fatalf("ByToken did not deep-copy: registry mutated to %q",
			again.Bindings[0].Credential)
	}
}

func TestByToken_EmptyTokenNeverMatches(t *testing.T) {
	m := NewManager("")
	// A revoked user has Token == "".
	if err := m.Put(&PanelUser{ID: "u1", Name: "alice", Token: ""}); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.ByToken(""); ok {
		t.Fatal("ByToken(\"\") matched a revoked (empty-token) user")
	}
}

func TestByToken_Unknown(t *testing.T) {
	m := NewManager("")
	if _, ok := m.ByToken("nope"); ok {
		t.Fatal("ByToken matched an unknown token")
	}
}

func TestRotateToken(t *testing.T) {
	m := NewManager("")
	if err := m.Put(&PanelUser{ID: "u1", Name: "a", Token: "old"}); err != nil {
		t.Fatal(err)
	}
	newTok, err := m.RotateToken("u1")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if newTok == "" || newTok == "old" {
		t.Fatalf("RotateToken returned %q", newTok)
	}
	if _, ok := m.ByToken("old"); ok {
		t.Fatal("old token still valid after rotate")
	}
	if _, ok := m.ByToken(newTok); !ok {
		t.Fatal("new token not valid after rotate")
	}
	if _, err := m.RotateToken("missing"); err == nil {
		t.Fatal("RotateToken on unknown id should error")
	}
}

func TestRevokeToken(t *testing.T) {
	m := NewManager("")
	if err := m.Put(&PanelUser{ID: "u1", Name: "a", Token: "live"}); err != nil {
		t.Fatal(err)
	}
	if err := m.RevokeToken("u1"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, ok := m.ByToken("live"); ok {
		t.Fatal("token still valid after revoke")
	}
	got, _ := m.Get("u1")
	if got.Token != "" {
		t.Fatalf("revoke did not clear token, got %q", got.Token)
	}
	if err := m.RevokeToken("missing"); err == nil {
		t.Fatal("RevokeToken on unknown id should error")
	}
}

// TestRotateTokenClearsDisabled proves that rotating a revoked (TokenDisabled)
// user re-enables it: TokenDisabled flips back to false, the new token resolves,
// and a subsequent Reconcile against the still-bound active does NOT clear or
// re-mint it (the rotated token survives).
func TestRotateTokenClearsDisabled(t *testing.T) {
	m := NewManager("")
	if err := m.Put(&PanelUser{ID: "u1", Name: "a", Token: "live",
		Bindings: []Binding{{InboundTag: "vless-in", Credential: "u-1", Protocol: "vless"}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.RevokeToken("u1"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if got, _ := m.Get("u1"); !got.TokenDisabled {
		t.Fatal("revoke must set TokenDisabled=true")
	}
	newTok, err := m.RotateToken("u1")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if newTok == "" {
		t.Fatal("RotateToken returned empty token")
	}
	got, _ := m.Get("u1")
	if got.TokenDisabled {
		t.Fatal("rotate must clear TokenDisabled (re-enable)")
	}
	if _, ok := m.ByToken(newTok); !ok {
		t.Fatal("rotated token must resolve via ByToken")
	}
	// A reconcile against the still-bound active must NOT change the rotated token.
	active := cfg(vlessIn("vless-in", map[string]interface{}{"name": "a", "uuid": "u-1"}))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	after, _ := m.Get("u1")
	if after.Token != newTok {
		t.Fatalf("reconcile must not alter rotated token: %q -> %q", newTok, after.Token)
	}
	if after.TokenDisabled {
		t.Fatal("reconcile must not re-disable a rotated user")
	}
}

// TestRotateTokenPersists proves a rotated token survives a reload: a fresh
// Manager loading the same file resolves the NEW token and rejects the OLD one.
func TestRotateTokenPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.toml")
	m := NewManager(path)
	if err := m.Put(&PanelUser{ID: "u1", Name: "alice", Token: "old"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	newTok, err := m.RotateToken("u1")
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}

	m2 := NewManager(path)
	if err := m2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := m2.ByToken(newTok)
	if !ok {
		t.Fatal("rotated token not resolvable after reload")
	}
	if got.ID != "u1" {
		t.Fatalf("rotated token resolved to wrong user: %+v", got)
	}
	if _, ok := m2.ByToken("old"); ok {
		t.Fatal("old token still resolvable after reload")
	}
}

// TestRevokeTokenPersists proves a revoked token survives a reload: a fresh
// Manager loading the same file has the user with Token=="" and the old token
// no longer resolves.
func TestRevokeTokenPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.toml")
	m := NewManager(path)
	if err := m.Put(&PanelUser{ID: "u1", Name: "alice", Token: "live"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := m.RevokeToken("u1"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	m2 := NewManager(path)
	if err := m2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := m2.Get("u1")
	if !ok {
		t.Fatal("user missing after reload")
	}
	if got.Token != "" {
		t.Fatalf("revoke not persisted: token = %q, want empty", got.Token)
	}
	if !got.TokenDisabled {
		t.Fatal("TokenDisabled must round-trip to disk: reloaded user has TokenDisabled=false")
	}
	if _, ok := m2.ByToken("live"); ok {
		t.Fatal("revoked token still resolvable after reload")
	}
}

// TestRotateTokenRollsBackOnSaveError forces saveLocked to fail (by swapping the
// manager's path to one whose parent component is a regular file, so MkdirAll
// fails) and asserts RotateToken returns an error AND leaves the in-memory token
// unchanged — the OLD token must still resolve via ByToken.
func TestRotateTokenRollsBackOnSaveError(t *testing.T) {
	dir := t.TempDir()
	goodPath := filepath.Join(dir, "users.toml")
	m := NewManager(goodPath)

	// Seed a user and rotate once successfully so a real token is on disk + memory.
	if err := m.Put(&PanelUser{ID: "u1", Name: "alice", Token: "seed"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	tok, err := m.RotateToken("u1")
	if err != nil {
		t.Fatalf("initial RotateToken: %v", err)
	}

	// Now point the manager at an unwritable path: a regular file used as a
	// directory component makes os.MkdirAll (and thus saveLocked) fail reliably.
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	m.path = filepath.Join(blocker, "users.toml")

	if _, err := m.RotateToken("u1"); err == nil {
		t.Fatal("expected RotateToken save error, got nil")
	}
	// Rollback must keep memory consistent with the (unchanged) disk: the token
	// from the last successful rotate must still resolve.
	got, ok := m.ByToken(tok)
	if !ok {
		t.Fatal("RotateToken must roll back token on save failure: old token no longer resolves")
	}
	if got.ID != "u1" {
		t.Fatalf("old token resolved to wrong user after rollback: %+v", got)
	}
}

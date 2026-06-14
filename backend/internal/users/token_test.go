package users

import (
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

package awg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddPeerKeepsUnicodeName is the regression test for the bug where a peer
// named "Ноутбук" was stored (and shown) as "name": AddPeer ran the display name
// through SanitizeName, which reduces every non-[A-Za-z0-9._-] rune to '_' and
// falls back to "name" when nothing survives. Three Cyrillic-named peers all
// collapsed into three peers called "name".
func TestAddPeerKeepsUnicodeName(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	if err := os.MkdirAll(filepath.Dir(m.confPath), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)

	names := []string{"Ноутбук", "Телефон Ани", "Ноутбук №2"}
	got := make([]string, 0, len(names))
	for _, want := range names {
		sum, err := m.AddPeer(context.Background(), want)
		if err != nil {
			t.Fatalf("AddPeer(%q): %v", want, err)
		}
		if sum.Name != want {
			t.Errorf("AddPeer(%q).Name = %q; want the name as typed", want, sum.Name)
		}
		stored, ok := m.store.Get(sum.PublicKey)
		if !ok {
			t.Fatalf("AddPeer(%q): peer not persisted", want)
		}
		if stored.Name != want {
			t.Errorf("stored name for %q = %q; want the name as typed", want, stored.Name)
		}
		got = append(got, stored.Name)
	}
	// The three distinct names must stay three distinct names.
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] {
			t.Fatalf("names collapsed into duplicates: %v", got)
		}
		seen[n] = true
	}
}

// TestAddPeerRejectsHostileName: names are no longer silently rewritten, so a
// name that could inject .conf directives (or otherwise break a header) must be
// REJECTED with ErrInvalidName rather than quietly mangled.
func TestAddPeerRejectsHostileName(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)

	bad := map[string]string{
		"newline":    "a\nPublicKey = ATTACKER",
		"carriage":   "a\rb",
		"tab":        "a\tb",
		"nul":        "a\x00b",
		"empty":      "",
		"whitespace": "   ",
		"too long":   strings.Repeat("я", peerNameMaxRunes+1),
	}
	for label, name := range bad {
		if _, err := m.AddPeer(context.Background(), name); err != ErrInvalidName {
			t.Errorf("AddPeer(%s=%q) err = %v; want ErrInvalidName", label, name, err)
		}
	}
}

// TestValidatePeerNameTrims: surrounding whitespace is trimmed, the rest is kept
// verbatim (no transliteration, no case folding).
func TestValidatePeerNameTrims(t *testing.T) {
	got, err := ValidatePeerName("  Ноутбук Ани  ")
	if err != nil {
		t.Fatalf("ValidatePeerName: %v", err)
	}
	if got != "Ноутбук Ани" {
		t.Fatalf("ValidatePeerName = %q; want %q", got, "Ноутбук Ани")
	}
}

// TestPeerTagStableAndUnique pins the sing-box export tag contract:
//   - an already-safe ASCII name keeps the historical "awg-<name>" tag, so
//     upgrading an existing install does not renumber anybody's endpoint tag;
//   - names that reduce lossily (Cyrillic, spaces, punctuation) get a short
//     public-key-derived suffix, so two peers can never share a tag.
func TestPeerTagStableAndUnique(t *testing.T) {
	const pubA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaA="
	const pubB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbB="

	if got := PeerTag("alice", pubA); got != "awg-alice" {
		t.Errorf("PeerTag(alice) = %q; want awg-alice (historical form)", got)
	}
	if got := PeerTag("alice", pubA); got != PeerTag("alice", pubA) {
		t.Errorf("PeerTag not stable: %q", got)
	}
	a := PeerTag("Ноутбук", pubA)
	b := PeerTag("Телефон", pubB)
	if a == b {
		t.Errorf("distinct Cyrillic names share a tag: %q", a)
	}
	for _, tag := range []string{a, b, PeerTag("Мой Laptop", pubA)} {
		if !strings.HasPrefix(tag, "awg-") {
			t.Errorf("tag %q lost the awg- prefix", tag)
		}
		for _, r := range strings.TrimPrefix(tag, "awg-") {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
				r == '-', r == '_', r == '.':
			default:
				t.Errorf("tag %q contains unsafe rune %q", tag, r)
			}
		}
	}
	// Same name, different peers -> different tags (no collision on the wire).
	if PeerTag("Ноутбук", pubA) == PeerTag("Ноутбук", pubB) {
		t.Error("same name on two peers must still yield distinct tags")
	}
}

// TestRenderPeerBlockCommentIsSingleLine: names are validated on the way in, but
// a legacy/hand-edited peers.toml could still hold a control character. The
// [Peer] "# comment" renderer must never emit one, or the .conf gains a forged
// directive line.
func TestRenderPeerBlockCommentIsSingleLine(t *testing.T) {
	out := renderPeerBlock(PeerLine{
		Name:      "evil\nPublicKey = ATTACKER",
		PublicKey: "PUB=",
		AllowedIP: "10.10.0.2/32",
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	comments := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "#") {
			comments++
		}
		if strings.Contains(l, "ATTACKER") && !strings.HasPrefix(l, "#") {
			t.Fatalf("comment injected a directive line:\n%s", out)
		}
	}
	if comments != 1 {
		t.Fatalf("want exactly one comment line, got %d:\n%s", comments, out)
	}
	// A legitimate Cyrillic name survives into the comment as typed.
	out = renderPeerBlock(PeerLine{Name: "Ноутбук", PublicKey: "PUB=", AllowedIP: "10.10.0.3/32"})
	if !strings.Contains(out, "# Ноутбук\n") {
		t.Fatalf("Cyrillic name mangled in the [Peer] comment:\n%s", out)
	}
}

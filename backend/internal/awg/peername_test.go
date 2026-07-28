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
// through util.SanitizeName, which reduces every non-[A-Za-z0-9._-] rune to '_' and
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
		// Cf (invisible format runes): no injection risk, but pure visual spoofing.
		"rtl override":   "\u202Efnoc.exe",                                   // renders reversed in the UI / save dialog
		"zero width":     "\u041d\u043e\u0443\u0442\u200b\u0431\u0443\u043a", // looks identical to "Ноутбук"
		"soft hyphen":    "\u041d\u043e\u0443\u0442\u00ad\u0431\u0443\u043a", // also invisible
		"line separator": "a\u2028b",                                         // IsSpace, not IsControl
	}
	for label, name := range bad {
		if _, err := m.AddPeer(context.Background(), name); err != ErrInvalidName {
			t.Errorf("AddPeer(%s=%q) err = %v; want ErrInvalidName", label, name, err)
		}
	}
}

// TestValidatePeerNameAcceptsBoundaryAndEmoji pins the accepting side: the length
// limit is inclusive, and a name that is just an emoji is a legitimate name.
func TestValidatePeerNameAcceptsBoundaryAndEmoji(t *testing.T) {
	exact := strings.Repeat("я", peerNameMaxRunes) // 64 runes, 128 bytes
	got, err := ValidatePeerName(exact)
	if err != nil {
		t.Errorf("ValidatePeerName(%d runes) = %v; want accepted (the limit is inclusive)",
			peerNameMaxRunes, err)
	}
	if got != exact {
		t.Errorf("boundary name was altered")
	}
	for _, name := range []string{"🏠 Дача", "📱", "Ноутбук №2 (работа)", "工作电脑"} {
		got, err := ValidatePeerName(name)
		if err != nil {
			t.Errorf("ValidatePeerName(%q) = %v; want accepted", name, err)
		}
		if got != name {
			t.Errorf("ValidatePeerName(%q) = %q; want it verbatim", name, got)
		}
	}
}

// TestAddPeerAllowsDuplicateNames: the store keys on the public key, so naming
// two devices alike is legal and always was — it must stay two separate peers
// with two separate /32s. (It is also the case that makes PeerTag hand out one
// tag for both; see TestPeerTagStableAndCollisionScope.)
func TestAddPeerAllowsDuplicateNames(t *testing.T) {
	f := newFakeRunner()
	m := newTestManager(t, f)
	os.MkdirAll(filepath.Dir(m.confPath), 0700)
	os.WriteFile(m.confPath, []byte("[Interface]\nListenPort = 51820\n"), 0600)

	a, err := m.AddPeer(context.Background(), "laptop")
	if err != nil {
		t.Fatalf("first AddPeer: %v", err)
	}
	b, err := m.AddPeer(context.Background(), "laptop")
	if err != nil {
		t.Fatalf("second AddPeer with the same name: %v", err)
	}
	if a.PublicKey == b.PublicKey || a.Address == b.Address {
		t.Fatalf("duplicate names collapsed into one peer: %#v / %#v", a, b)
	}
	if a.Name != "laptop" || b.Name != "laptop" {
		t.Fatalf("names rewritten: %q / %q", a.Name, b.Name)
	}
	if n := len(m.store.List()); n != 2 {
		t.Fatalf("store holds %d peers; want 2", n)
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

// TestPeerTagStableAndCollisionScope pins the sing-box export tag contract:
//   - an already-safe ASCII name keeps the historical "awg-<name>" tag, so
//     upgrading an existing install does not renumber anybody's endpoint tag;
//   - names that reduce lossily (Cyrillic, spaces, punctuation) get a short
//     public-key-derived suffix, so the REDUCTION can never merge two peers;
//   - two peers the user deliberately named alike still share a tag — the
//     documented limit of the guarantee.
func TestPeerTagStableAndCollisionScope(t *testing.T) {
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
	// Same LOSSY name on two peers -> still distinct tags: the collision the
	// reduction would have created is removed by the public-key suffix.
	if PeerTag("Ноутбук", pubA) == PeerTag("Ноутбук", pubB) {
		t.Error("same lossy name on two peers must still yield distinct tags")
	}
	// Same ALREADY-SAFE name on two peers -> the SAME tag, by design. Pinning it
	// so the docstring's scope stays honest: PeerTag removes collisions its own
	// reduction creates, not collisions the user creates by naming two peers
	// alike. Suffixing here would renumber every existing export; suffixing only
	// on duplicates would make one peer's tag depend on another peer existing.
	if PeerTag("alice", pubA) != PeerTag("alice", pubB) {
		t.Error("duplicate safe names are expected to share a tag (stability over the store-dependent alternative)")
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

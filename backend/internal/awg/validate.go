package awg

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"routebox/backend/internal/util"
)

var (
	ifaceRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,14}$`) // Linux IFNAMSIZ-1, no leading dash/dot/_
	hFieldRe = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)          // single int or lo-hi range
)

// ValidateSubnet parses an IPv4 CIDR and returns its CANONICAL network form
// (e.g. "10.10.0.5/24" -> "10.10.0.0/24"). Raw input is never re-emitted.
func ValidateSubnet(s string) (string, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}
	if !p.Addr().Is4() {
		return "", fmt.Errorf("subnet must be IPv4")
	}
	if p.Bits() > 30 {
		return "", fmt.Errorf("subnet too small (need /30 or larger)")
	}
	return p.Masked().String(), nil
}

// ValidateWANIface checks the name against IFNAMSIZ + a strict charset AND that
// it exists under sysClassNet (default "/sys/class/net"). Returns the name.
func ValidateWANIface(name, sysClassNet string) (string, error) {
	if !ifaceRe.MatchString(name) {
		return "", fmt.Errorf("invalid interface name %q", name)
	}
	if sysClassNet == "" {
		sysClassNet = "/sys/class/net"
	}
	if _, err := os.Stat(filepath.Join(sysClassNet, name)); err != nil {
		return "", fmt.Errorf("interface %q not present", name)
	}
	return name, nil
}

// ValidateListenPort bounds a UDP listen port to 1..65535.
func ValidateListenPort(p int) (int, error) {
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("listen_port %d out of range (1-65535)", p)
	}
	return p, nil
}

// ValidateMTU bounds an MTU to a sane WireGuard range.
func ValidateMTU(m int) (int, error) {
	if m < 576 || m > 9000 {
		return 0, fmt.Errorf("mtu %d out of range (576-9000)", m)
	}
	return m, nil
}

// SanitizeName mirrors api.sanitizeFilename via the shared util helper: keeps
// [A-Za-z0-9._-], replaces every other rune with '_', trims leading/trailing
// '_', falls back to "name". It NEVER errors, so a hostile name is neutralised,
// not rejected.
//
// It is a TOKEN builder, not a name normaliser: never store its output as the
// peer's display name (that is what turned every Cyrillic-named peer into a peer
// called "name"). Use ValidatePeerName for what the user sees, and PeerTag for a
// safe sing-box tag.
func SanitizeName(name string) string {
	return util.SanitizeName(name, "name")
}

// peerNameMaxRunes bounds a peer display name. Generous enough for "Ноутбук
// Анастасии (работа)", small enough that the name can never bloat peers.toml,
// a .conf comment line or an HTTP header.
const peerNameMaxRunes = 64

// ErrInvalidName is returned by ValidatePeerName (and therefore AddPeer) for a
// name the user must fix: empty, over-long, or carrying a control/format
// character. The API layer maps it to 400 rather than silently rewriting the name.
var ErrInvalidName = fmt.Errorf("invalid peer name")

// ValidatePeerName accepts a peer display name AS THE USER TYPED IT — Cyrillic,
// emoji, CJK, spaces and punctuation all survive verbatim — and only rejects what
// is unsafe or useless: invalid UTF-8, control characters (a newline in a name
// would forge a directive line in awg-rb0.conf and split an HTTP header), invisible
// format characters, an empty/whitespace-only name, or one longer than
// peerNameMaxRunes.
//
// Reduction to a safe ASCII token happens at the point of use (PeerTag,
// Content-Disposition), never at the point of storage.
func ValidatePeerName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > peerNameMaxRunes {
		return "", ErrInvalidName
	}
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	for _, r := range name {
		// IsControl covers C0/C1. IsSpace+!=' ' covers the exotic whitespace
		// (line/paragraph separators, NBSP-alikes). Cf (format) covers the
		// INVISIBLE runes: U+202E RTL-override flips how the name renders in the
		// UI and in a save dialog, U+200B ZWSP makes two names that look
		// identical to a human two different entries. Neither can inject
		// anything downstream — every consumer below neutralises them — but a
		// name nobody can read for what it is has no legitimate use.
		//
		// Cost: emoji ZWJ sequences (family/profession emoji glue their parts
		// with U+200D, which is Cf) are rejected. Single-codepoint emoji are fine.
		if unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ') || unicode.Is(unicode.Cf, r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// PeerTag builds the sing-box endpoint tag for an exported client config:
// "awg-" + a safe ASCII token derived from the peer name. It is a pure function
// of (name, pub) — deliberately NOT of the rest of the store, so a peer's tag
// never changes because some other peer was added or renamed.
//
// Two properties matter and pull against each other:
//
//   - STABILITY. A name that is already a safe token ("alice", "phone-2") passes
//     through untouched and keeps the exact tag it has always had. That covers
//     every peer whose name survived the old store-time sanitisation unchanged.
//   - NO COLLISION FROM THE REDUCTION ITSELF. Once the reduction loses
//     information it stops being injective: "Ноутбук" and "Телефон" both reduce
//     to nothing, "Laptop Ани" and "Laptop Пети" both reduce to "Laptop", and the
//     receiving sing-box would see two endpoints with one tag. So a lossy
//     reduction gets a short suffix derived from the peer's public key — unique
//     per peer, and stable for the life of the peer because the key never changes.
//
// SCOPE OF THE GUARANTEE — read this before claiming tags are unique. The suffix
// removes collisions that the reduction CREATES. It does not remove collisions
// the user creates: two peers both named "alice" are both already safe tokens, so
// both take the stable branch and both get "awg-alice". That is intentional —
// duplicate names are allowed (the store keys on the public key, not the name),
// suffixing every tag would renumber existing exports, and suffixing only when a
// duplicate exists would make one peer's tag depend on another peer's existence.
// A user who exports two identically-named peers into the same box renames one.
func PeerTag(name, pub string) string {
	token := util.SanitizeName(name, "")
	if token == name && token != "" {
		return "awg-" + token // already a safe token: historical tag, unchanged
	}
	if token == "" {
		token = "peer"
	}
	sum := sha256.Sum256([]byte(pub))
	return "awg-" + token + "-" + hex.EncodeToString(sum[:3])
}

// ValidateDNS parses each entry as a netip.Addr and re-emits canonically.
func ValidateDNS(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		a, err := netip.ParseAddr(strings.TrimSpace(e))
		if err != nil {
			return nil, fmt.Errorf("invalid dns %q: %w", e, err)
		}
		out = append(out, a.String())
	}
	return out, nil
}

// ValidateAllowedIPs parses each as a netip.Prefix and re-emits canonically.
func ValidateAllowedIPs(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		p, err := netip.ParsePrefix(strings.TrimSpace(e))
		if err != nil {
			return nil, fmt.Errorf("invalid allowed_ips %q: %w", e, err)
		}
		out = append(out, p.Masked().String()) // network form: drop host bits
	}
	return out, nil
}

// ValidateHField validates an AWG H1-H4 value (digits or a "lo-hi" range).
func ValidateHField(s string) (string, error) {
	if !hFieldRe.MatchString(s) {
		return "", fmt.Errorf("invalid obfuscation H value %q", s)
	}
	return s, nil
}

// ValidateUintRange validates an AWG UintRange value: "" (unset), a single
// decimal uint32 "N", or a "lo-hi" range with lo <= hi. Mirrors the fork's
// parser (decimal, uint32 bounds). Returns the canonical form.
func ValidateUintRange(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	parts := strings.Split(s, "-")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid uint range %q", s)
	}
	vals := make([]uint64, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return "", fmt.Errorf("invalid uint range %q", s)
		}
		vals[i] = v
	}
	if len(vals) == 2 {
		if vals[0] > vals[1] {
			return "", fmt.Errorf("invalid uint range %q: lo > hi", s)
		}
		return fmt.Sprintf("%d-%d", vals[0], vals[1]), nil
	}
	return fmt.Sprintf("%d", vals[0]), nil
}

// validateHPKConstraint enforces the fork's rule that when header_protection_key
// is enabled, every S1-S4 padding must be at least 12 bytes — the fork rejects
// the endpoint otherwise, so surface it as a clear 400 instead.
func validateHPKConstraint(o Obfuscation, hpkOn bool) error {
	if !hpkOn {
		return nil
	}
	for i, s := range []int{o.S1, o.S2, o.S3, o.S4} {
		if s < 12 {
			return fmt.Errorf("S%d must be >= 12 when header protection key is enabled (got %d)", i+1, s)
		}
	}
	return nil
}

// ValidatePublicKey requires an exact std-base64 string decoding to 32 bytes.
// Used as a lookup key — never an FS/shell path.
func ValidatePublicKey(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("invalid public key")
	}
	return s, nil
}

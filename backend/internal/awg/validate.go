package awg

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
// '_', falls back to "name". Used for the [Peer] # comment AND any client-facing
// filename. It NEVER errors, so a hostile name is neutralised, not rejected.
func SanitizeName(name string) string {
	return util.SanitizeName(name, "name")
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
// is enabled, every S1-S4 padding must be at least 8 bytes — the fork rejects
// the endpoint otherwise, so surface it as a clear 400 instead.
func validateHPKConstraint(o Obfuscation, hpkOn bool) error {
	if !hpkOn {
		return nil
	}
	for i, s := range []int{o.S1, o.S2, o.S3, o.S4} {
		if s < 8 {
			return fmt.Errorf("S%d must be >= 8 when header protection key is enabled (got %d)", i+1, s)
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

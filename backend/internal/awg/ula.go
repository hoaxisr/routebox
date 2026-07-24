package awg

import (
	"crypto/rand"
	"fmt"
	"net/netip"
)

// GenerateULAPrefix returns a random RFC 4193 ULA /64: fd00::/8 + 40 random
// global-id/subnet bits. Called ONCE per server and persisted; never regenerated.
func GenerateULAPrefix() (netip.Prefix, error) {
	var b [16]byte
	if _, err := rand.Read(b[1:6]); err != nil { // 40 random bits after the 0xfd byte
		return netip.Prefix{}, err
	}
	b[0] = 0xfd
	return netip.PrefixFrom(netip.AddrFrom16(b), 64), nil
}

// MapV4ToV6 embeds the full 32-bit IPv4 into the low 32 bits of prefix's /64,
// so the mapping is collision-free for any IPv4 subnet size. prefix must be a /64.
func MapV4ToV6(prefix netip.Prefix, v4 netip.Addr) (netip.Addr, error) {
	if !v4.Is4() {
		return netip.Addr{}, fmt.Errorf("MapV4ToV6: %s is not IPv4", v4)
	}
	b := prefix.Masked().Addr().As16()
	v := v4.As4()
	copy(b[12:16], v[:])
	return netip.AddrFrom16(b).Unmap(), nil
}

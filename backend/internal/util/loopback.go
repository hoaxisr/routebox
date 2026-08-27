package util

import (
	"net"
	"strings"
)

// IsLoopbackListen reports whether a sing-box "listen" value binds an address
// that no other host can reach. Two places need exactly this question and must
// answer it identically: the validator, which exempts a plaintext trojan behind
// a TLS-terminating front, and the client-link builder, which substitutes the
// front's public port for an inbound's internal one. A disagreement between
// them means the panel refuses to save a config whose links it already treats
// as fronted, or ships links pointing at an unreachable internal port.
//
// An absent or unparseable value is NOT loopback: sing-box defaults to the
// wildcard address, and guessing loopback here would expose a public inbound.
func IsLoopbackListen(listen string) bool {
	s := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(listen), "["), "]")
	if s == "" {
		return false
	}
	// A hand-written config may name the loopback instead of numbering it.
	if strings.EqualFold(s, "localhost") {
		return true
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.IsLoopback()
}

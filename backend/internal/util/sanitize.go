// Package util holds small, dependency-light helpers shared across RouteBox
// packages. It must not import other internal packages, so leaf packages (api,
// awg, ...) can depend on it without risking import cycles.
package util

import "strings"

// SanitizeName keeps [A-Za-z0-9._-], replaces every other rune with '_', trims
// leading/trailing '_', and returns fallback when nothing usable remains. It
// NEVER errors, so a hostile name is neutralised rather than rejected. Used for
// safe Content-Disposition filenames, AmneziaWG [Peer] comments, and any other
// place a user-supplied name must be reduced to a safe token.
func SanitizeName(name, fallback string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return fallback
	}
	return out
}

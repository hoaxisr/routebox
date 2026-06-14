package v2stats

import "strings"

// parseUserStat parses a StatsService stat name of the form
// "user>>>NAME>>>traffic>>>uplink" (or downlink). Returns the user display-name,
// whether it's uplink (true) or downlink (false), and ok=false for any name that
// is not a per-user traffic counter (wrong prefix, wrong arity, unknown
// direction). PURE.
func parseUserStat(name string) (user string, uplink bool, ok bool) {
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 {
		return "", false, false
	}
	if parts[0] != "user" || parts[2] != "traffic" {
		return "", false, false
	}
	switch parts[3] {
	case "uplink":
		return parts[1], true, true
	case "downlink":
		return parts[1], false, true
	default:
		return "", false, false
	}
}

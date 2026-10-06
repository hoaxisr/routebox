// Package asnsets turns AS numbers into sing-box rule sets: it fetches the
// prefixes an AS announces (RIPEstat), keeps them in a local rule-set file that
// sing-box reloads on change, and refreshes them on a schedule (#103).
package asnsets

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalid marks bad operator input (API answers 400).
var ErrInvalid = errors.New("invalid")

// ParseASN accepts "13335", "AS13335" or "as13335" (surrounding space trimmed).
func ParseASN(s string) (uint32, error) {
	t := strings.TrimSpace(s)
	if len(t) >= 2 && strings.EqualFold(t[:2], "AS") {
		t = t[2:]
	}
	n, err := strconv.ParseUint(t, 10, 32)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("%w: %q is not an AS number", ErrInvalid, s)
	}
	return uint32(n), nil
}

// FormatASN renders n the way RIPEstat and operators write it.
func FormatASN(n uint32) string { return "AS" + strconv.FormatUint(uint64(n), 10) }

var tagRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidTag reports whether tag is safe as a rule-set tag AND a file name.
func ValidTag(tag string) bool { return tagRe.MatchString(tag) && tag != "." && tag != ".." }

// Aggregate returns the smallest equivalent prefix list: host bits masked,
// duplicates and prefixes contained in another removed, equal-length adjacent
// siblings merged until nothing changes. IPv4 first, then IPv6, ascending.
// PURE.
func Aggregate(in []netip.Prefix) []netip.Prefix {
	var v4, v6 []netip.Prefix
	for _, p := range in {
		if !p.IsValid() {
			continue
		}
		p = p.Masked()
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	return append(aggregateFamily(v4), aggregateFamily(v6)...)
}

func aggregateFamily(ps []netip.Prefix) []netip.Prefix {
	for {
		sortPrefixes(ps)
		// Drop prefixes contained in an earlier (shorter or equal) one. Sorted
		// by address, everything inside a prefix follows it directly, so the
		// last kept prefix is the only candidate container.
		out := ps[:0:0]
		for _, p := range ps {
			if len(out) > 0 && out[len(out)-1].Bits() <= p.Bits() && out[len(out)-1].Contains(p.Addr()) {
				continue
			}
			out = append(out, p)
		}
		// Merge adjacent siblings: same length, same parent.
		merged := false
		res := out[:0:0]
		for i := 0; i < len(out); i++ {
			if i+1 < len(out) && out[i].Bits() == out[i+1].Bits() && out[i].Bits() > 0 {
				parent, _ := out[i].Addr().Prefix(out[i].Bits() - 1)
				if parent.Addr() == out[i].Addr() && parent.Contains(out[i+1].Addr()) {
					res = append(res, parent)
					i++
					merged = true
					continue
				}
			}
			res = append(res, out[i])
		}
		ps = res
		if !merged {
			return ps
		}
	}
}

func sortPrefixes(ps []netip.Prefix) {
	sort.Slice(ps, func(i, j int) bool {
		if c := ps[i].Addr().Compare(ps[j].Addr()); c != 0 {
			return c < 0
		}
		return ps[i].Bits() < ps[j].Bits()
	})
}

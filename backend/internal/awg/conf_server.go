package awg

import (
	"fmt"
	"strings"
)

// ServerConf is the validated, canonical input to RenderServer. Every string
// here has already passed component-0 validation.
type ServerConf struct {
	PrivateKey string
	Address    string // e.g. "10.10.0.1/24"
	ListenPort int
	MTU        int
	Obf        Obfuscation
	WAN        string // validated iface
	Subnet     string // canonical CIDR for MASQUERADE -s
	Iface      string // "awg-rb0"
	// HeaderProtectionKey is the AWG3 shared secret ("" = off); only ever
	// non-empty when the kernel backend has confirmed awg3 capability
	// (KernelSupportsAWG3) — a pre-awg3 awg-quick hard-fails on the key.
	HeaderProtectionKey string
}

// PeerLine is one [Peer] block's renderable fields. Name is the display name as
// the user typed it; renderPeerBlock reduces it for the comment line.
type PeerLine struct {
	Name      string
	PublicKey string
	PSK       string
	AllowedIP string // "<ip>/32"
}

// RenderServer produces awg-rb0.conf. NAT lives in PostUp/PostDown using dedicated
// RBOX-AWG-* chains so RouteBox never edits operator policy in place; PostUp
// flushes-then-recreates its chains (orphan-safe across crash/reboot).
func RenderServer(s ServerConf, peers []PeerLine) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", s.PrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", s.Address)
	fmt.Fprintf(&b, "ListenPort = %d\n", s.ListenPort)
	if s.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", s.MTU)
	}
	writeObf(&b, s.Obf) // reuse the single obfuscation renderer (shared with BuildClient; emits both 3.1 flags)
	if s.HeaderProtectionKey != "" {
		fmt.Fprintf(&b, "HeaderProtectionKey = %s\n", s.HeaderProtectionKey)
	}
	b.WriteString(postUp(s))
	b.WriteString(postDown(s))
	for _, p := range peers {
		b.WriteString(renderPeerBlock(p))
	}
	return b.String()
}

// confComment reduces a peer display name to something safe to put after a
// leading "# " in awg-rb0.conf: a single line, no control characters, bounded
// length. Non-ASCII is KEPT — wg/awg ignores comment lines entirely, so "Ноутбук"
// is as harmless as "laptop", and mangling it here is what made the file useless
// for a human reading it. Names are already validated on the way in
// (ValidatePeerName); this is the belt-and-braces pass for a legacy or
// hand-edited peers.toml.
func confComment(name string) string {
	var b strings.Builder
	n := 0
	for _, r := range name {
		if n >= peerNameMaxRunes {
			break
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			r = '_'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// renderPeerBlock renders one [Peer] stanza. It is the single [Peer] renderer,
// shared by RenderServer (full-file render) and Manager.appendPeerToConf (live
// append) so the two paths can never drift. The name goes through confComment so
// the leading `# comment` cannot inject .conf directives.
func renderPeerBlock(p PeerLine) string {
	var b strings.Builder
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "# %s\n", confComment(p.Name))
	fmt.Fprintf(&b, "PublicKey = %s\n", p.PublicKey)
	if p.PSK != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", p.PSK)
	}
	fmt.Fprintf(&b, "AllowedIPs = %s\n", p.AllowedIP)
	return b.String()
}

// linkChain emits the idempotent "ensure JUMP into a builtin chain" pair: check
// the jump exists (-C), else append it (-A). table is "" for the filter table or
// e.g. "-t nat". Used by PostUp so re-runs never stack duplicate jumps.
func linkChain(table, builtin, chain string) string {
	t := ""
	if table != "" {
		t = " " + table
	}
	return fmt.Sprintf("iptables%s -C %s -j %s 2>/dev/null || iptables%s -A %s -j %s",
		t, builtin, chain, t, builtin, chain)
}

// recreateChain emits the orphan-safe "create-or-flush" pair for a dedicated
// RBOX-AWG-* chain: create if missing (-N), then flush (-F) any stale rules.
func recreateChain(table, chain string) string {
	t := ""
	if table != "" {
		t = " " + table
	}
	return fmt.Sprintf("iptables%s -N %s 2>/dev/null || true; iptables%s -F %s",
		t, chain, t, chain)
}

func postUp(s ServerConf) string {
	// One PostUp line (semicolon-joined; awg-quick runs it via /bin/sh, so every
	// interpolated value MUST be component-0 canonical — they are).
	cmds := []string{
		// Tolerated on failure: the value may already be set by something that
		// owns it — a container runtime's sysctls, or a host where /proc/sys is
		// not writable by whoever runs this. Forwarding being on already is not
		// a reason to refuse to bring the interface up.
		"sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true",
		// NAT chain
		recreateChain("-t nat", "RBOX-AWG-NAT"),
		fmt.Sprintf("iptables -t nat -A RBOX-AWG-NAT -s %s -o %s -j MASQUERADE", s.Subnet, s.WAN),
		linkChain("-t nat", "POSTROUTING", "RBOX-AWG-NAT"),
		// FORWARD chain
		recreateChain("", "RBOX-AWG-FWD"),
		fmt.Sprintf("iptables -A RBOX-AWG-FWD -i %s -j ACCEPT", s.Iface),
		fmt.Sprintf("iptables -A RBOX-AWG-FWD -o %s -j ACCEPT", s.Iface),
		linkChain("", "FORWARD", "RBOX-AWG-FWD"),
		// INPUT chain (open the UDP listen port without touching operator policy)
		recreateChain("", "RBOX-AWG-IN"),
		fmt.Sprintf("iptables -A RBOX-AWG-IN -p udp --dport %d -j ACCEPT", s.ListenPort),
		linkChain("", "INPUT", "RBOX-AWG-IN"),
	}
	return "PostUp = " + strings.Join(cmds, "; ") + "\n"
}

// teardownChain emits the orphan-safe unlink/flush/delete triple for a dedicated
// RBOX-AWG-* chain. Every step tolerates absence so PostDown is idempotent.
func teardownChain(table, builtin, chain string) string {
	t := ""
	if table != "" {
		t = " " + table
	}
	return fmt.Sprintf(
		"iptables%s -D %s -j %s 2>/dev/null || true; iptables%s -F %s 2>/dev/null || true; iptables%s -X %s 2>/dev/null || true",
		t, builtin, chain, t, chain, t, chain)
}

func postDown(s ServerConf) string {
	cmds := []string{
		teardownChain("-t nat", "POSTROUTING", "RBOX-AWG-NAT"),
		teardownChain("", "FORWARD", "RBOX-AWG-FWD"),
		teardownChain("", "INPUT", "RBOX-AWG-IN"),
	}
	return "PostDown = " + strings.Join(cmds, "; ") + "\n"
}

// parseDefaultRoute extracts the WAN iface from `ip route show default` output.
func parseDefaultRoute(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	}
	return "", fmt.Errorf("no default-route device found")
}

// parseShowPeers extracts peer public keys from `awg show <iface>` output.
func parseShowPeers(out string) []string {
	var peers []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "peer:") {
			peers = append(peers, strings.TrimSpace(strings.TrimPrefix(line, "peer:")))
		}
	}
	return peers
}

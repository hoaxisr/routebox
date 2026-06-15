package awg

import (
	"context"
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
}

// PeerLine is one [Peer] block's renderable fields (name already sanitized).
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
	writeObf(&b, s.Obf) // reuse the single obfuscation renderer (shared with BuildClient)
	b.WriteString(postUp(s))
	b.WriteString(postDown(s))
	for _, p := range peers {
		b.WriteString("\n[Peer]\n")
		fmt.Fprintf(&b, "# %s\n", p.Name)
		fmt.Fprintf(&b, "PublicKey = %s\n", p.PublicKey)
		if p.PSK != "" {
			fmt.Fprintf(&b, "PresharedKey = %s\n", p.PSK)
		}
		fmt.Fprintf(&b, "AllowedIPs = %s\n", p.AllowedIP)
	}
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
		"sysctl -w net.ipv4.ip_forward=1",
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

// DetectWAN runs `ip route show default` via the Runner and parses the device.
func DetectWAN(ctx context.Context, run Runner) (string, error) {
	out, _, err := run.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("ip route: %w", err)
	}
	return parseDefaultRoute(out)
}

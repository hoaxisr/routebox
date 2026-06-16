package awg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// iface_Up brings the interface up via systemd (the single lifecycle path).
func (m *Manager) iface_Up(ctx context.Context) error {
	_, _, err := m.run.Run(ctx, "systemctl", "enable", "--now", "awg-quick@"+m.iface)
	return err
}

// iface_Down stops+disables the interface (PostDown reverts NAT).
func (m *Manager) iface_Down(ctx context.Context) error {
	_, _, err := m.run.Run(ctx, "systemctl", "disable", "--now", "awg-quick@"+m.iface)
	return err
}

// iface_SyncConf reloads peers live: `awg-quick strip` -> 0600 temp file ->
// `awg syncconf <iface> <tempfile>`. NEVER process-substitution / sh -c.
func (m *Manager) iface_SyncConf(ctx context.Context) error {
	stripped, _, err := m.run.Run(ctx, "awg-quick", "strip", m.iface)
	if err != nil {
		return fmt.Errorf("awg-quick strip: %w", err)
	}
	tmp := filepath.Join(m.pskTmpDir, "sync-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".conf")
	if err := os.WriteFile(tmp, []byte(stripped), 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	_, _, err = m.run.Run(ctx, "awg", "syncconf", m.iface, tmp)
	return err
}

// iface_SetPeer adds/updates a peer live; PSK is passed by FILE (awg convention).
// pub is a validated 32-byte std-base64 key and allowedIP is a netip-canonical
// "<ip>/32" — neither can begin with '-', so there is no flag-injection via argv
// (and the Runner never uses a shell).
func (m *Manager) iface_SetPeer(ctx context.Context, pub, pskFile, allowedIP string) error {
	_, _, err := m.run.Run(ctx, "awg", "set", m.iface, "peer", pub,
		"preshared-key", pskFile, "allowed-ips", allowedIP)
	return err
}

// iface_RemovePeer removes a peer live.
func (m *Manager) iface_RemovePeer(ctx context.Context, pub string) error {
	_, _, err := m.run.Run(ctx, "awg", "set", m.iface, "peer", pub, "remove")
	return err
}

// iface_Handshakes returns pubkey -> last-handshake unix-seconds from
// `awg show <iface> latest-handshakes` (tab-separated "<pubkey>\t<unix>"; 0 = never).
// Degrades to an empty map on any error so callers just report everyone offline.
func (m *Manager) iface_Handshakes(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	res, _, err := m.run.Run(ctx, "awg", "show", m.iface, "latest-handshakes")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(res, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		ts, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		out[f[0]] = ts
	}
	return out
}

// iface_ShowPeers returns the live peer pubkeys (for reconcile).
func (m *Manager) iface_ShowPeers(ctx context.Context) ([]string, error) {
	out, _, err := m.run.Run(ctx, "awg", "show", m.iface)
	if err != nil {
		return nil, err
	}
	return parseShowPeers(out), nil
}

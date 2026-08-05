package awg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// awgPeerStatsHTTPClient bounds the call so a hung Clash API socket can't
// stall a roster fetch indefinitely. This is a loopback call (Clash API is
// 127.0.0.1-only), so a generous ceiling never bites in practice.
var awgPeerStatsHTTPClient = &http.Client{Timeout: 5 * time.Second}

// ErrAwgPeerStatsUnsupported means the running amnezia-box binary predates
// the /awg/{tag}/peers route (hoaxisr/amnezia-box, "expose real per-peer AWG
// handshake/tx/rx via UAPI") — an expected, steady-state condition on an
// unpatched binary, not a transient failure. Callers use it to fall back
// without treating every poll as an error worth logging.
var ErrAwgPeerStatsUnsupported = errors.New("amnezia-box binary predates the /awg/{tag}/peers route")

// PeerStat is one peer's slice of GET /awg/{tag}/peers — the WireGuard
// device's own UAPI state (see amnezia-box's transport/awg.Device.IpcGet),
// not an approximation from traffic or Clash's connection tracker. Exported
// so main can name it in the closure passed to Manager.SetPeerStats.
type PeerStat struct {
	LastHandshake int64
	TxBytes       int64
	RxBytes       int64
}

// FetchAwgPeerStats calls the AWG-endpoint peer-stats extension added to
// hoaxisr/amnezia-box, returning real per-peer handshake/traffic state keyed
// by base64 public key — the same identity ListPeers already keys the store
// by, so no address/tunnel-IP translation is needed the way the traffic-based
// fallback (SetPeerLiveness) requires.
func FetchAwgPeerStats(clashAddr, secret, tag string) (map[string]PeerStat, error) {
	if clashAddr == "" {
		return nil, fmt.Errorf("no clash api address configured")
	}
	req, err := http.NewRequest("GET", "http://"+clashAddr+"/awg/"+tag+"/peers", nil)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := awgPeerStatsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAwgPeerStatsUnsupported
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clash /awg/%s/peers: status %d", tag, resp.StatusCode)
	}
	var data struct {
		Peers []struct {
			PublicKey     string `json:"public_key"`
			LastHandshake int64  `json:"last_handshake"`
			TxBytes       int64  `json:"tx_bytes"`
			RxBytes       int64  `json:"rx_bytes"`
		} `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := make(map[string]PeerStat, len(data.Peers))
	for _, p := range data.Peers {
		out[p.PublicKey] = PeerStat{LastHandshake: p.LastHandshake, TxBytes: p.TxBytes, RxBytes: p.RxBytes}
	}
	return out, nil
}

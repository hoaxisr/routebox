package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/traffic"
)

// awgPubKeyParam reads the {publicKey} path param, URL-decodes it (the panel sends
// it via encodeURIComponent → %2B/%2F/%3D, and chi.URLParam returns the raw,
// still-encoded segment), then validates it as a 32-byte std-base64 key. Without the
// decode, any key containing +,/,= 400s (a trailing "=" is on nearly every key).
func awgPubKeyParam(r *http.Request) (string, error) {
	raw := chi.URLParam(r, "publicKey")
	if dec, err := url.PathUnescape(raw); err == nil {
		raw = dec
	}
	return awg.ValidatePublicKey(raw)
}

// GetAWGStatus reports module + interface + peer status.
func (h *Handler) GetAWGStatus(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	st := h.awg.Status(r.Context())
	// The port the deployment published, when it published one. The panel shows
	// it as fixed instead of offering a field whose every value but this one
	// makes the server answer nobody (settings_handlers refuses those anyway).
	// Marshalled alongside the status rather than inside it: which port a
	// container publishes is a deployment fact, not something the AWG manager
	// knows or should.
	if fixed := os.Getenv(awgPortEnv); fixed != "" {
		writeSuccess(w, struct {
			awg.AWGStatus
			ListenPortFixed string `json:"listen_port_fixed"`
		}{st, fixed})
		return
	}
	writeSuccess(w, st)
}

// EnableAWG starts the enable orchestrator. The request body is ignored — the
// server config is the persisted settings.awg (the panel's Save is the single
// writer), which removes the hardcoded-body footgun. Enable re-validates every field.
func (h *Handler) EnableAWG(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	in := awgEnableInput(h.settings.Get().Awg)
	if err := h.awg.Enable(r.Context(), in); err != nil {
		// Enable already validated every field; a validation error is a 400.
		writeConfigError(w, http.StatusBadRequest, err)
		return
	}
	// Sticky "configured" flag: after the first successful Enable the panel shows the
	// steady-state view instead of the setup wizard. Never reset on Disable. Best-effort
	// — a persist failure must not fail an otherwise-successful enable.
	if !h.settings.Get().Awg.Configured {
		h.persistAwgSetting("awg.configured", true)
	}
	// Persist enabled=true so a RouteBox restart rehydrates the server as enabled
	// (RehydrateSingbox reads settings.Awg.Enabled — Bug C1). Best-effort too.
	h.persistAwgSetting("awg.enabled", true)
	writeSuccess(w, h.awg.Status(r.Context()))
}

// persistAwgSetting writes one awg settings key to disk. Best-effort by design —
// a persist failure must not fail an enable/disable that already took effect —
// but never silent: when the flag does not reach /etc/routebox, a RouteBox
// restart rehydrates the OPPOSITE state, and the 30s sweep then removes a
// working endpoint (or serves one the operator switched off). "Do not fail the
// request" is not the same as "tell nobody", and the log is the only place left
// where this can be noticed.
func (h *Handler) persistAwgSetting(key string, value interface{}) {
	if err := h.settings.Update(map[string]interface{}{key: value}); err != nil {
		log.Printf("awg: could not stage %s=%v: %v — this will not survive a RouteBox restart", key, value, err)
		return
	}
	if err := h.settings.Save(); err != nil {
		log.Printf("awg: could not persist %s=%v: %v — this will not survive a RouteBox restart", key, value, err)
	}
}

// awgEnableInput maps persisted settings to the awg orchestrator input. This is
// the settings<->awg package boundary (settings stays awg-agnostic).
func awgEnableInput(s settings.AwgSettings) awg.EnableInput {
	return awg.EnableInputFromSettings(s)
}

// awgSingboxDraftBlocked reports whether an enable/disable/peer op must be
// rejected: on the singbox backend these ops rewrite the ACTIVE config, and
// SyncAwgEndpointActive silently DEFERS while a draft is pending — the op would
// flip enabled/phase without writing anything. Kernel mode is unaffected.
// BackendName (not Status().Backend) keeps this predicate cheap: on kernel the
// full Status execs `awg show`/`iptables` per call, an exec-storm for a guard
// that only needs the backend string.
func (h *Handler) awgSingboxDraftBlocked() bool {
	return h.config != nil && h.config.HasDraft() &&
		h.awg.BackendName() == "singbox"
}

// DisableAWG stops the interface (PostDown reverts NAT).
func (h *Handler) DisableAWG(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	if err := h.awg.Disable(r.Context()); err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to disable", err)
		return
	}
	// Persist enabled=false so Disable survives a RouteBox restart (Bug C1).
	// Best-effort — a persist failure must not fail an otherwise-successful disable.
	h.persistAwgSetting("awg.enabled", false)
	writeSuccess(w, h.awg.Status(r.Context()))
}

// ListAWGPeers returns secret-free summaries (PeerSummary cannot serialise keys).
func (h *Handler) ListAWGPeers(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	writeSuccess(w, h.awg.ListPeers(r.Context()))
}

// CreateAWGPeer live-adds a peer and returns a secret-free summary.
func (h *Handler) CreateAWGPeer(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	sum, err := h.awg.AddPeer(r.Context(), body.Name)
	if err == awg.ErrSubnetExhausted {
		writeError(w, http.StatusConflict, "subnet exhausted")
		return
	}
	// The name is stored as typed (any script, any alphabet), so an unusable one
	// is reported instead of being silently rewritten.
	if err == awg.ErrInvalidName {
		writeError(w, http.StatusBadRequest, "invalid name: 1-64 characters, no line breaks or control characters")
		return
	}
	if err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to add peer", err)
		return
	}
	writeSuccess(w, sum)
}

// DeleteAWGPeer validates the {publicKey} path param FIRST (exact std-base64 → 32
// bytes, no FS use on a bad key), then live-removes the peer.
func (h *Handler) DeleteAWGPeer(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	addr, err := h.awg.RemovePeer(r.Context(), pub)
	if err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to remove peer", err)
		return
	}
	if h.traffic != nil {
		purgePeerTraffic(h.traffic, pub, addr)
	}
	w.WriteHeader(http.StatusNoContent)
}

// purgePeerTraffic drops a removed peer's history: its user_traffic series
// (TrafficKey, #109) and the per-source Breakdown rows under its tunnel IP
// (#19; the IP, e.g. 10.10.64.2, is the `source` key in traffic_minute, so the
// /32 mask is stripped). Best-effort — a purge failure must not fail the delete.
func purgePeerTraffic(store *traffic.Store, pub, addr string) {
	if err := store.DeleteUsers([]string{awg.TrafficKey(pub)}); err != nil {
		log.Printf("api: purge history for removed peer %s: %v", pub, err)
	}
	if addr == "" {
		return
	}
	src := addr
	if pfx, err := netip.ParsePrefix(addr); err == nil {
		src = pfx.Addr().String()
	}
	if err := store.DeleteSource(src); err != nil {
		log.Printf("api: purge traffic for removed peer source %q: %v", src, err)
	}
}

// GetAWGPeerConfig serves the client .conf (text/plain). The {publicKey} path
// param is validated FIRST (non-base64 → 400, no FS/traversal). Mirrors /sub
// hardening: existence checked FIRST (404 before 503 when public_host is unset),
// no err.Error() echo, Cache-Control: no-store, sanitised attachment filename.
func (h *Handler) GetAWGPeerConfig(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		http.Error(w, "awg not available", http.StatusServiceUnavailable)
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	name, ok := h.awg.PeerConfig(pub) // existence first (404 before 503)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// AWG server address is a distinct concept from the panel's public host: on a
	// router PublicHost is legitimately empty (clients use the LAN/WAN IP).
	s := h.settings.Get()
	host := s.Awg.ServerHost
	if host == "" {
		host = s.Server.PublicHost
	}
	if host == "" {
		http.Error(w, "server address not set — set it on the AWG page", http.StatusServiceUnavailable)
		return
	}
	body, err := h.awg.RenderClientConf(pub, host)
	if err != nil {
		http.Error(w, "config unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", contentDispositionAttachment(name, "peer", ".conf"))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// GetAWGPeerVPNLink serves the peer as an Amnezia vpn:// link (text/plain). Same
// hardening as GetAWGPeerConfig: key validated first, existence checked before the
// host (404 before 503), no internal error echoed. The one exception is an
// unrepresentable peer — that is the operator's configuration, not an internal
// fault, so the message names what to fix. It is safe to echo: by construction it
// contains only static text and the literals "H1".."H4".
func (h *Handler) GetAWGPeerVPNLink(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		http.Error(w, "awg not available", http.StatusServiceUnavailable)
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	if _, ok := h.awg.PeerConfig(pub); !ok { // existence first (404 before 503)
		http.NotFound(w, r)
		return
	}
	s := h.settings.Get()
	host := s.Awg.ServerHost
	if host == "" {
		host = s.Server.PublicHost
	}
	if host == "" {
		http.Error(w, "server address not set — set it on the AWG page", http.StatusServiceUnavailable)
		return
	}
	link, err := h.awg.RenderVPNLink(pub, host)
	if err != nil {
		if errors.Is(err, awg.ErrLinkUnrepresentable) {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, "link unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(link))
}

// GetAWGPeerSingbox serves a client sing-box endpoint JSON for a peer (authorized).
// Mirrors GetAWGPeerConfig hardening: validate key first, existence 404 before the
// public-host 503, no err echo, Cache-Control no-store.
func (h *Handler) GetAWGPeerSingbox(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	name, ok := h.awg.PeerConfig(pub) // existence first (404 before 503)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Same host resolution as GetAWGPeerConfig: awg.server_host first, then the
	// panel's public host; only 503 when both are unset.
	s := h.settings.Get()
	host := s.Awg.ServerHost
	if host == "" {
		host = s.Server.PublicHost
	}
	if host == "" {
		writeError(w, http.StatusServiceUnavailable, "server address not set — set it on the AWG page")
		return
	}
	ep, err := h.awg.ClientEndpoint(pub, name, host)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export unavailable")
		return
	}
	// Envelope response (writeSuccess) so the frontend's request<T> helper — which
	// reads {success,data} and returns .data — works. JSON escaping of the i-field
	// "<b 0x..>" tags is harmless: JSON.parse restores them, and the client
	// re-stringifies with literal angle brackets before copy/download.
	w.Header().Set("Cache-Control", "no-store")
	writeSuccess(w, ep)
}

// SetAWGPeerExpiry writes a peer's two limits — the expiry date and the traffic
// quota (#95). Body: {"expires_at": <unix|0>, "quota_bytes": <bytes>}; BOTH
// fields are optional and an omitted one keeps its stored value, because the
// panel drives them from two independent rows ("Продлить" and "Квота", spec
// Q16/Q22) and each must be able to save alone. expires_at 0 clears the date, a
// non-zero one must be strictly in the future; quota_bytes 0 clears the limit and
// a negative value is refused rather than read as "no limit".
//
// The verdict lands at save time, not at the next sweep (spec Q19/Q20): raising a
// limit or extending a date re-admits a peer that has nothing else holding it
// back, lowering the quota below what is already spent suspends it now. It is NOT
// a plain re-admit any more — a peer over its quota stays off the interface
// however far its date is moved, which is exactly what makes the two independent.
func (h *Handler) SetAWGPeerExpiry(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	// Pointers, so "field absent" is distinguishable from "field set to 0" — 0 is
	// a meaningful value for both (no expiry / no limit), and a quota row that
	// omitted expires_at would otherwise clear the date it never showed.
	var body struct {
		ExpiresAt  *int64 `json:"expires_at"`
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.ExpiresAt != nil && *body.ExpiresAt != 0 && *body.ExpiresAt <= time.Now().Unix() {
		writeError(w, http.StatusBadRequest, "expires_at must be 0 or in the future")
		return
	}
	if body.QuotaBytes != nil && *body.QuotaBytes < 0 {
		writeError(w, http.StatusBadRequest, "quota_bytes must be >= 0 (0 = no limit)")
		return
	}
	// The pointers go through UNRESOLVED: SetPeerLimits fills the omitted half
	// from the store under the same lock it writes with. Reading it here instead
	// would lose the other operator's edit whenever the lock is busy (the 30s
	// sweep, or another change's singbox apply) — and two rows in the UI mean two
	// people saving different halves of the same peer is a normal Tuesday.
	if err := h.awg.SetPeerLimits(r.Context(), pub, body.ExpiresAt, body.QuotaBytes); err != nil {
		if errors.Is(err, awg.ErrPeerNotFound) {
			writeError(w, http.StatusNotFound, "peer not found")
			return
		}
		writeOpError(w, http.StatusInternalServerError, "failed to set peer limits", err)
		return
	}
	writeSuccess(w, h.awg.Status(r.Context()))
}

// ResetAWGPeerTraffic zeroes a peer's cumulative quota counters, stamps
// used_reset_at and puts the peer back in service when the spent allowance was
// what suspended it (spec Q9/Q19) — the "Сбросить счётчик" button. The limit
// itself is kept: raising it is the other, separate way back.
//
// Same draft guard as the limits PATCH: on the singbox backend the re-admit
// rewrites the ACTIVE config, which SyncAwgEndpointActive silently defers while a
// draft is pending. Answers with Status, like every other peer op.
func (h *Handler) ResetAWGPeerTraffic(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if h.awgSingboxDraftBlocked() {
		writeError(w, http.StatusConflict, "apply or discard pending config changes first")
		return
	}
	pub, err := awgPubKeyParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key")
		return
	}
	if err := h.awg.ResetPeerUsage(r.Context(), pub); err != nil {
		if errors.Is(err, awg.ErrPeerNotFound) {
			writeError(w, http.StatusNotFound, "peer not found")
			return
		}
		writeOpError(w, http.StatusInternalServerError, "failed to reset traffic counters", err)
		return
	}
	writeSuccess(w, h.awg.Status(r.Context()))
}

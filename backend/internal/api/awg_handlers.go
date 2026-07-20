package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/awg"
	"routebox/backend/internal/settings"
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
	writeSuccess(w, h.awg.Status(r.Context()))
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Sticky "configured" flag: after the first successful Enable the panel shows the
	// steady-state view instead of the setup wizard. Never reset on Disable. Best-effort
	// — a persist failure must not fail an otherwise-successful enable.
	if !h.settings.Get().Awg.Configured {
		if err := h.settings.Update(map[string]interface{}{"awg.configured": true}); err == nil {
			_ = h.settings.Save()
		}
	}
	// Persist enabled=true so a RouteBox restart rehydrates the server as enabled
	// (RehydrateSingbox reads settings.Awg.Enabled — Bug C1). Best-effort too.
	if err := h.settings.Update(map[string]interface{}{"awg.enabled": true}); err == nil {
		_ = h.settings.Save()
	}
	writeSuccess(w, h.awg.Status(r.Context()))
}

// awgEnableInput maps persisted settings to the awg orchestrator input. This is
// the settings<->awg package boundary (settings stays awg-agnostic).
func awgEnableInput(s settings.AwgSettings) awg.EnableInput {
	return awg.EnableInput{
		Subnet: s.Subnet, ListenPort: s.ListenPort, MTU: s.MTU,
		DNS: s.DNS, WANIface: s.WANIface, ObfPreset: s.ObfPreset,
		HeaderProtection: s.HeaderProtection,
		Obf: awg.Obfuscation{
			Jc: s.Obf.Jc, Jmin: s.Obf.Jmin, Jmax: s.Obf.Jmax,
			S1: s.Obf.S1, S2: s.Obf.S2, S3: s.Obf.S3, S4: s.Obf.S4,
			H1: s.Obf.H1, H2: s.Obf.H2, H3: s.Obf.H3, H4: s.Obf.H4,
			CPA: s.Obf.ContentPaddingAddition, RAT: s.Obf.RekeyAfterTime,
		},
	}
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
		writeError(w, http.StatusInternalServerError, "failed to disable")
		return
	}
	// Persist enabled=false so Disable survives a RouteBox restart (Bug C1).
	// Best-effort — a persist failure must not fail an otherwise-successful disable.
	if err := h.settings.Update(map[string]interface{}{"awg.enabled": false}); err == nil {
		_ = h.settings.Save()
	}
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add peer")
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
	if err := h.awg.RemovePeer(r.Context(), pub); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove peer")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	// A singbox backend with header protection ON shares an HPK the wg-quick .conf
	// format cannot carry — a client importing it would silently never connect.
	// The UI already hides QR/.conf on singbox; this closes the direct-API footgun.
	// With header protection OFF the .conf is still valid (no awg3 fields needed).
	if h.awg.BackendName() == "singbox" && h.settings.Get().Awg.HeaderProtection {
		writeError(w, http.StatusConflict, "this server uses header protection — use the JSON export")
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
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(name)+".conf\"")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
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

// SetAWGPeerExpiry sets/extends/clears a peer's expiry (one operation backed by
// RenewPeer, which also re-admits a suspended peer). Body: {"expires_at": <unix|0>}.
// 0 clears expiry; a non-zero value must be strictly in the future.
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
	var body struct {
		ExpiresAt int64 `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.ExpiresAt != 0 && body.ExpiresAt <= time.Now().Unix() {
		writeError(w, http.StatusBadRequest, "expires_at must be 0 or in the future")
		return
	}
	if err := h.awg.RenewPeer(r.Context(), pub, body.ExpiresAt); err != nil {
		if errors.Is(err, awg.ErrPeerNotFound) {
			writeError(w, http.StatusNotFound, "peer not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to set expiry")
		return
	}
	writeSuccess(w, h.awg.Status(r.Context()))
}

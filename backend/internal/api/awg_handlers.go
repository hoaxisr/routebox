package api

import (
	"encoding/json"
	"net/http"
	"net/url"

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
	writeSuccess(w, h.awg.Status(r.Context()))
}

// awgEnableInput maps persisted settings to the awg orchestrator input. This is
// the settings<->awg package boundary (settings stays awg-agnostic).
func awgEnableInput(s settings.AwgSettings) awg.EnableInput {
	return awg.EnableInput{
		Subnet: s.Subnet, ListenPort: s.ListenPort, MTU: s.MTU,
		DNS: s.DNS, WANIface: s.WANIface,
		Obf: awg.Obfuscation{
			Jc: s.Obf.Jc, Jmin: s.Obf.Jmin, Jmax: s.Obf.Jmax,
			S1: s.Obf.S1, S2: s.Obf.S2, S3: s.Obf.S3, S4: s.Obf.S4,
			H1: s.Obf.H1, H2: s.Obf.H2, H3: s.Obf.H3, H4: s.Obf.H4,
		},
	}
}

// DisableAWG stops the interface (PostDown reverts NAT).
func (h *Handler) DisableAWG(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	if err := h.awg.Disable(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable")
		return
	}
	writeSuccess(w, h.awg.Status(r.Context()))
}

// ListAWGPeers returns secret-free summaries (PeerSummary cannot serialise keys).
func (h *Handler) ListAWGPeers(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
		return
	}
	writeSuccess(w, h.awg.ListPeers())
}

// CreateAWGPeer live-adds a peer and returns a secret-free summary.
func (h *Handler) CreateAWGPeer(w http.ResponseWriter, r *http.Request) {
	if h.awg == nil {
		writeError(w, http.StatusServiceUnavailable, "awg not available")
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
	host := h.settings.Get().Server.PublicHost
	if host == "" {
		http.Error(w, "public host not configured", http.StatusServiceUnavailable)
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

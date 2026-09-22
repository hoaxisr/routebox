package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/subscriptions"
)

// --- Outbounds CRUD ---

// ListOutbounds returns all outbounds
func (h *Handler) ListOutbounds(w http.ResponseWriter, r *http.Request) {
	outbounds := h.config.ListOutbounds()
	writeSuccess(w, outbounds)
}

// GetOutbound returns a single outbound by tag
func (h *Handler) GetOutbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	outbound, found := h.config.GetOutbound(tag)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("outbound '%s' not found", tag))
		return
	}
	writeSuccess(w, outbound)
}

// CreateOutbound creates a new outbound
func (h *Handler) CreateOutbound(w http.ResponseWriter, r *http.Request) {
	var outbound map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&outbound); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateOutbound(outbound); err != nil {
		writeConfigError(w, http.StatusBadRequest, err)
		return
	}

	writeSuccess(w, outbound)
}

// UpdateOutbound updates an existing outbound
func (h *Handler) UpdateOutbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var outbound map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&outbound); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateOutbound(tag, outbound); err != nil {
		writeConfigError(w, http.StatusNotFound, err)
		return
	}

	writeSuccess(w, outbound)
}

// DeleteOutbound deletes an outbound
func (h *Handler) DeleteOutbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteOutbound(tag); err != nil {
		writeConfigError(w, http.StatusNotFound, err)
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("outbound '%s' deleted", tag)})
}

// ParseOutboundLink turns one pasted share link into sing-box outbounds with
// the same parser the subscription refresh uses. The UI needs it for formats
// it does not parse itself (TrustTunnel deep links carry a binary TLV payload
// and may expand into several outbounds).
func (h *Handler) ParseOutboundLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}
	nodes, skipped := subscriptions.ParseLinks([]string{strings.TrimSpace(req.Link)})
	if len(nodes) == 0 {
		writeError(w, http.StatusBadRequest, "unsupported or malformed link")
		return
	}
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, map[string]interface{}{"outbound": n.Outbound, "name": n.Name})
	}
	writeSuccess(w, map[string]interface{}{"outbounds": out, "skipped": skipped})
}

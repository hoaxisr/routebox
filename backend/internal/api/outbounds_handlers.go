package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
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
		writeError(w, http.StatusBadRequest, err.Error())
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
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, outbound)
}

// DeleteOutbound deletes an outbound
func (h *Handler) DeleteOutbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteOutbound(tag); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("outbound '%s' deleted", tag)})
}

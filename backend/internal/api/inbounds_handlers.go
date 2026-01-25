package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- Inbounds CRUD ---

// ListInbounds returns all inbounds
func (h *Handler) ListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds := h.config.ListInbounds()
	writeSuccess(w, inbounds)
}

// GetInbound returns a single inbound by tag
func (h *Handler) GetInbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	inbound, found := h.config.GetInbound(tag)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inbound '%s' not found", tag))
		return
	}
	writeSuccess(w, inbound)
}

// CreateInbound creates a new inbound
func (h *Handler) CreateInbound(w http.ResponseWriter, r *http.Request) {
	var inbound map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateInbound(inbound); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, inbound)
}

// UpdateInbound updates an existing inbound
func (h *Handler) UpdateInbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var inbound map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateInbound(tag, inbound); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, inbound)
}

// DeleteInbound deletes an inbound
func (h *Handler) DeleteInbound(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteInbound(tag); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("inbound '%s' deleted", tag)})
}

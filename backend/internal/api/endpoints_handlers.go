package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- Endpoints CRUD ---

// ListEndpoints returns all endpoints
func (h *Handler) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints := h.config.ListEndpoints()
	writeSuccess(w, endpoints)
}

// GetEndpoint returns a single endpoint by tag
func (h *Handler) GetEndpoint(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	endpoint, found := h.config.GetEndpoint(tag)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("endpoint '%s' not found", tag))
		return
	}
	writeSuccess(w, endpoint)
}

// CreateEndpoint creates a new endpoint
func (h *Handler) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var endpoint map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&endpoint); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateEndpoint(endpoint); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, endpoint)
}

// UpdateEndpoint updates an existing endpoint
func (h *Handler) UpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var endpoint map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&endpoint); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateEndpoint(tag, endpoint); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, endpoint)
}

// DeleteEndpoint deletes an endpoint
func (h *Handler) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteEndpoint(tag); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("endpoint '%s' deleted", tag)})
}

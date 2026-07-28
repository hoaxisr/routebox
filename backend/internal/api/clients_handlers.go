package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/clients"
)

type clientResponse struct {
	clients.Entry
	Online bool `json:"online"`
}

const onlineThresholdSeconds = 5 * 60

// ListClients returns all known clients with computed online flag.
func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	if h.clients == nil {
		writeError(w, http.StatusServiceUnavailable, "clients manager not initialized")
		return
	}
	now := time.Now().Unix()
	all := h.clients.List()
	out := make([]clientResponse, len(all))
	for i, e := range all {
		out[i] = clientResponse{Entry: e, Online: now-e.LastSeen <= onlineThresholdSeconds}
	}
	writeSuccess(w, out)
}

// UpdateClient sets name and note for an IP.
func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	if h.clients == nil {
		writeError(w, http.StatusServiceUnavailable, "clients manager not initialized")
		return
	}
	ip := chi.URLParam(r, "ip")
	var body struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// SetName only touches memory — Save below is the write that can be refused.
	if err := h.clients.SetName(ip, body.Name, body.Note); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.clients.Save(); err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	e, _ := h.clients.Get(ip)
	writeSuccess(w, e)
}

// DeleteClient removes an entry.
func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	if h.clients == nil {
		writeError(w, http.StatusServiceUnavailable, "clients manager not initialized")
		return
	}
	ip := chi.URLParam(r, "ip")
	h.clients.Forget(ip)
	if err := h.clients.Save(); err != nil {
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

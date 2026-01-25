package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// --- DNS Servers CRUD ---

// ListDnsServers returns all DNS servers
func (h *Handler) ListDnsServers(w http.ResponseWriter, r *http.Request) {
	servers := h.config.ListDnsServers()
	writeSuccess(w, servers)
}

// CreateDnsServer creates a new DNS server
func (h *Handler) CreateDnsServer(w http.ResponseWriter, r *http.Request) {
	var server map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateDnsServer(server); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, server)
}

// UpdateDnsServer updates an existing DNS server
func (h *Handler) UpdateDnsServer(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var server map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateDnsServer(tag, server); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, server)
}

// DeleteDnsServer deletes a DNS server by tag
func (h *Handler) DeleteDnsServer(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteDnsServer(tag); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("DNS server '%s' deleted", tag)})
}

// --- DNS Rules CRUD ---

// ListDnsRules returns all DNS rules
func (h *Handler) ListDnsRules(w http.ResponseWriter, r *http.Request) {
	rules := h.config.ListDnsRules()
	writeSuccess(w, rules)
}

// CreateDnsRule creates a new DNS rule (appended)
func (h *Handler) CreateDnsRule(w http.ResponseWriter, r *http.Request) {
	var rule map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateDnsRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// UpdateDnsRule updates a DNS rule at index
func (h *Handler) UpdateDnsRule(w http.ResponseWriter, r *http.Request) {
	indexStr := chi.URLParam(r, "index")
	index, err := parseInt(indexStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid index")
		return
	}

	var rule map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateDnsRule(index, rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// DeleteDnsRule deletes a DNS rule at index
func (h *Handler) DeleteDnsRule(w http.ResponseWriter, r *http.Request) {
	indexStr := chi.URLParam(r, "index")
	index, err := parseInt(indexStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid index")
		return
	}

	if err := h.config.DeleteDnsRule(index); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("DNS rule[%d] deleted", index)})
}

// ReorderDnsRules moves a DNS rule from one index to another
func (h *Handler) ReorderDnsRules(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.ReorderDnsRules(payload.From, payload.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("DNS rule moved from %d to %d", payload.From, payload.To)})
}

// --- DNS Settings ---

// GetDnsSettings returns DNS settings
func (h *Handler) GetDnsSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.config.GetDnsSettings()
	writeSuccess(w, settings)
}

// UpdateDnsSettings updates DNS settings
func (h *Handler) UpdateDnsSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateDnsSettings(settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, settings)
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/domains"
)

// ListDomainSets returns all domain set sources
func (h *Handler) ListDomainSets(w http.ResponseWriter, r *http.Request) {
	if h.domains == nil {
		writeError(w, http.StatusServiceUnavailable, "domains manager not initialized")
		return
	}

	sets, err := h.domains.ListSets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, sets)
}

// CreateDomainSet creates a new empty domain set
func (h *Handler) CreateDomainSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tag string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if body.Tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	// Check if tag conflicts with existing rule_set tags in sing-box config
	existingRuleSets := h.config.ListRuleSets()
	for _, rs := range existingRuleSets {
		if t, ok := rs["tag"].(string); ok && t == body.Tag {
			writeError(w, http.StatusConflict, fmt.Sprintf("tag '%s' is already used by an existing rule-set in config", body.Tag))
			return
		}
	}

	if err := h.domains.CreateSet(body.Tag); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"tag": body.Tag, "message": fmt.Sprintf("rule set '%s' created", body.Tag)})
}

// DeleteDomainSet deletes a domain set and its files
func (h *Handler) DeleteDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.domains.DeleteSet(tag); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule set '%s' deleted", tag)})
}

// GetDomainSet returns the full rule set source content
func (h *Handler) GetDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	src, err := h.domains.GetSet(tag)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeSuccess(w, src)
}

// SaveDomainSet saves the full rule set source (JSON mode)
func (h *Handler) SaveDomainSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var src domains.RuleSetSource
	if err := json.NewDecoder(r.Body).Decode(&src); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if err := h.domains.SaveSet(tag, &src); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule set '%s' saved", tag)})
}

// AddDomain adds a domain to a set's domain_suffix
func (h *Handler) AddDomain(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if body.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	if err := h.domains.AddDomain(tag, body.Domain); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("domain added to '%s'", tag)})
}

// RemoveDomain removes a domain from a set
func (h *Handler) RemoveDomain(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	domain := chi.URLParam(r, "domain")

	if err := h.domains.RemoveDomain(tag, domain); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("domain removed from '%s'", tag)})
}

// CompileDomains compiles a domain set's JSON to SRS binary
func (h *Handler) CompileDomains(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.domains.Compile(tag); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Auto-create rule_set entry in sing-box config if not present
	h.ensureRuleSetInConfig(tag)

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule set '%s' compiled successfully", tag)})
}

// ImportDomains bulk-imports domains into a set
func (h *Handler) ImportDomains(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var body struct {
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if len(body.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "domains list is required")
		return
	}

	added, err := h.domains.ImportDomains(tag, body.Domains)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": fmt.Sprintf("%d domains imported into '%s'", added, tag),
		"added":   added,
	})
}

// ensureRuleSetInConfig creates a local binary rule_set entry in config if tag doesn't exist
func (h *Handler) ensureRuleSetInConfig(tag string) {
	// Check if rule set already exists in config
	existing := h.config.ListRuleSets()
	for _, rs := range existing {
		if t, ok := rs["tag"].(string); ok && t == tag {
			return // already exists
		}
	}

	// Create local binary rule_set entry
	rs := map[string]interface{}{
		"tag":    tag,
		"type":   "local",
		"format": "binary",
		"path":   fmt.Sprintf("ruleset/%s.srs", tag),
	}
	h.config.CreateRuleSet(rs)
}

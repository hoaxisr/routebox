package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// --- Rule Sets CRUD ---

// ListRuleSets returns all rule sets
func (h *Handler) ListRuleSets(w http.ResponseWriter, r *http.Request) {
	ruleSets := h.config.ListRuleSets()
	writeSuccess(w, ruleSets)
}

// CreateRuleSet creates a new rule set
func (h *Handler) CreateRuleSet(w http.ResponseWriter, r *http.Request) {
	var ruleSet map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&ruleSet); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateRuleSet(ruleSet); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, ruleSet)
}

// GetRuleSetsUsage returns which route/dns rules reference each rule set
func (h *Handler) GetRuleSetsUsage(w http.ResponseWriter, r *http.Request) {
	usage := h.config.GetRuleSetsUsage()
	writeSuccess(w, usage)
}

// DeleteRuleSet deletes a rule set by tag
func (h *Handler) DeleteRuleSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	if err := h.config.DeleteRuleSet(tag); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule set '%s' deleted", tag)})
}

// UpdateRuleSet updates an existing rule set by tag
func (h *Handler) UpdateRuleSet(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")

	var ruleSet map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&ruleSet); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateRuleSet(tag, ruleSet); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeSuccess(w, ruleSet)
}

// --- Route Rules CRUD ---

// ListRules returns all route rules
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	rules := h.config.ListRules()
	writeSuccess(w, rules)
}

// CreateRule creates a new route rule (appended)
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var rule map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.CreateRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// UpdateRule updates a route rule at index
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
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

	if err := h.config.UpdateRule(index, rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// DeleteRule deletes a route rule at index
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	indexStr := chi.URLParam(r, "index")
	index, err := parseInt(indexStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid index")
		return
	}

	if err := h.config.DeleteRule(index); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule[%d] deleted", index)})
}

// ReorderRules moves a rule from one index to another
func (h *Handler) ReorderRules(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.ReorderRules(payload.From, payload.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": fmt.Sprintf("rule moved from %d to %d", payload.From, payload.To)})
}

// --- Route Settings ---

// GetRouteSettings returns route settings (final, auto_detect_interface)
func (h *Handler) GetRouteSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.config.GetRouteSettings()
	writeSuccess(w, settings)
}

// UpdateRouteSettings updates route settings
func (h *Handler) UpdateRouteSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if err := h.config.UpdateRouteSettings(settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, settings)
}

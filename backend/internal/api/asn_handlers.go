package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/asnsets"
)

// ASN rule sets (#103): a set is a list of AS numbers whose announced
// prefixes RouteBox keeps in a `local` rule-set file under the settings dir.
// The endpoints here manage the sets; the config only ever carries the
// `{"type":"local","format":"source","tag","path"}` entry pointing at the
// file, added to the draft on create. Deleting the rule set goes through the
// ordinary DELETE /rule-sets/{tag}; the entry and file are reaped by Prune
// once no config mentions the tag any more.

type asnSetRequest struct {
	Tag         string   `json:"tag"`
	ASNs        []string `json:"asns"`
	IntervalHrs int      `json:"interval_hrs"`
}

func parseASNList(in []string) ([]uint32, error) {
	out := make([]uint32, 0, len(in))
	for _, s := range in {
		n, err := asnsets.ParseASN(s)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// writeASNError maps manager errors: bad input 400, unknown set 404, RIPEstat
// 502, read-only 409 (writeConfigError), anything else the fallback.
func writeASNError(w http.ResponseWriter, fallback int, err error) {
	var fe *asnsets.FetchError
	switch {
	case errors.Is(err, asnsets.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asnsets.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &fe):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeConfigError(w, fallback, err)
	}
}

// ListAsnSets returns every ASN set with its refresh state. PROTECTED.
func (h *Handler) ListAsnSets(w http.ResponseWriter, r *http.Request) {
	if h.asn == nil {
		writeSuccess(w, []asnsets.Entry{})
		return
	}
	writeSuccess(w, h.asn.List())
}

// CreateAsnSet fetches the prefixes, writes the file, then adds a `local` rule
// set pointing at it to the config draft. If the draft write fails, the set is
// rolled back so no orphan entry or file is left. PROTECTED.
func (h *Handler) CreateAsnSet(w http.ResponseWriter, r *http.Request) {
	if h.asn == nil {
		writeError(w, http.StatusServiceUnavailable, "ASN sets not available")
		return
	}
	var req asnSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}
	asns, err := parseASNList(req.ASNs)
	if err != nil {
		writeASNError(w, http.StatusInternalServerError, err)
		return
	}
	// Rule-set tags are one namespace for sing-box, whatever their type.
	for _, rs := range h.config.ListRuleSets() {
		if rs["tag"] == req.Tag {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("rule set with tag '%s' already exists", req.Tag))
			return
		}
	}
	e, err := h.asn.Create(r.Context(), req.Tag, asns, req.IntervalHrs)
	if err != nil {
		writeASNError(w, http.StatusInternalServerError, err)
		return
	}
	rs := map[string]interface{}{"type": "local", "format": "source", "tag": e.Tag, "path": e.Path}
	if err := h.config.CreateRuleSet(rs); err != nil {
		// No config entry means no set: undo the create. Should the undo fail
		// too (the whole settings dir read-only), Prune reaps the orphan after
		// its grace period.
		if derr := h.asn.Delete(e.Tag); derr != nil {
			log.Printf("asnsets: %s: rollback after failed draft write: %v", e.Tag, derr)
		}
		writeConfigError(w, http.StatusBadRequest, err)
		return
	}
	writeSuccess(w, e)
}

// UpdateAsnSet replaces the AS list / interval and rewrites the file. The
// config is untouched (same path). PROTECTED.
func (h *Handler) UpdateAsnSet(w http.ResponseWriter, r *http.Request) {
	if h.asn == nil {
		writeError(w, http.StatusServiceUnavailable, "ASN sets not available")
		return
	}
	var req asnSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}
	asns, err := parseASNList(req.ASNs)
	if err != nil {
		writeASNError(w, http.StatusInternalServerError, err)
		return
	}
	e, err := h.asn.Update(r.Context(), chi.URLParam(r, "tag"), asns, req.IntervalHrs)
	if err != nil {
		writeASNError(w, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(w, e)
}

// RefreshAsnSet refetches now. On failure the old file stays in service and
// the error is reported (502) with the entry's last_error set; an empty union
// is a 502 too, since it is RIPEstat that answered nothing. PROTECTED.
func (h *Handler) RefreshAsnSet(w http.ResponseWriter, r *http.Request) {
	if h.asn == nil {
		writeError(w, http.StatusServiceUnavailable, "ASN sets not available")
		return
	}
	e, err := h.asn.Refresh(r.Context(), chi.URLParam(r, "tag"))
	if err != nil {
		writeASNError(w, http.StatusBadGateway, err)
		return
	}
	writeSuccess(w, e)
}

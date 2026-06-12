package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/subscriptions"
)

const subNodePrefixSep = " · "

// ListSubscriptions returns all subscriptions (node_count from the store).
func (h *Handler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	if h.subs == nil {
		writeError(w, http.StatusServiceUnavailable, "subscriptions not initialized")
		return
	}
	writeSuccess(w, h.subs.List())
}

// CreateSubscription adds a subscription then immediately refreshes it.
func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	if h.subs == nil {
		writeError(w, http.StatusServiceUnavailable, "subscriptions not initialized")
		return
	}
	var body struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		IntervalHrs int    `json:"interval_hrs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sub, err := h.subs.Add(body.Name, body.URL, body.IntervalHrs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, _, rerr := h.subsRefresh(sub)
	errMsg := ""
	if rerr != nil {
		errMsg = rerr.Error()
	}
	h.subs.SetResult(sub.ID, n, errMsg)
	if rerr != nil {
		writeError(w, http.StatusBadRequest, rerr.Error())
		return
	}
	updated, _ := h.subs.Get(sub.ID)
	writeSuccess(w, updated)
}

// UpdateSubscription changes URL and interval (name immutable).
func (h *Handler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	if h.subs == nil {
		writeError(w, http.StatusServiceUnavailable, "subscriptions not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		URL         string `json:"url"`
		IntervalHrs int    `json:"interval_hrs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.subs.Update(id, body.URL, body.IntervalHrs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, _ := h.subs.Get(id)
	writeSuccess(w, updated)
}

// DeleteSubscription removes the store entry and its outbounds from the draft.
func (h *Handler) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if h.subs == nil {
		writeError(w, http.StatusServiceUnavailable, "subscriptions not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	sub, ok := h.subs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	groupTag := subscriptions.Sanitize(sub.Name)
	if err := h.config.RemoveSubscriptionOutbounds(groupTag, groupTag+subNodePrefixSep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.subs.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"message": "subscription deleted"})
}

// RefreshSubscription re-fetches one subscription on demand.
func (h *Handler) RefreshSubscription(w http.ResponseWriter, r *http.Request) {
	if h.subs == nil {
		writeError(w, http.StatusServiceUnavailable, "subscriptions not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	sub, ok := h.subs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	n, _, rerr := h.subsRefresh(sub)
	errMsg := ""
	if rerr != nil {
		errMsg = rerr.Error()
	}
	h.subs.SetResult(id, n, errMsg)
	if rerr != nil {
		writeError(w, http.StatusBadGateway, rerr.Error())
		return
	}
	updated, _ := h.subs.Get(id)
	writeSuccess(w, updated)
}

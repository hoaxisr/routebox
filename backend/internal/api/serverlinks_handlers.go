package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"routebox/backend/internal/serverlinks"
)

// GetUserLink builds a client share-link for one user of a server inbound.
// userKey is the user's index in the inbound's users array.
func (h *Handler) GetUserLink(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		writeError(w, http.StatusBadRequest, "host query parameter is required")
		return
	}
	tag := chi.URLParam(r, "tag")
	inbound, found := h.config.GetInbound(tag)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inbound '%s' not found", tag))
		return
	}
	idx, err := strconv.Atoi(chi.URLParam(r, "userKey"))
	if err != nil || idx < 0 {
		writeError(w, http.StatusBadRequest, "invalid user index")
		return
	}
	users, _ := inbound["users"].([]interface{})
	if idx >= len(users) {
		writeError(w, http.StatusNotFound, "user index out of range")
		return
	}
	user, ok := users[idx].(map[string]interface{})
	if !ok {
		writeError(w, http.StatusInternalServerError, "malformed user entry")
		return
	}
	link, err := serverlinks.BuildShareLink(inbound, user, host)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"link": link})
}

package api

import (
	"crypto/subtle"
	"net/http"

	"routebox/backend/internal/settings"
)

// BasicAuth returns middleware enforcing HTTP Basic auth when enabled in
// settings. Settings are read per-request so runtime changes apply without a
// restart. WebSocket upgrade requests pass through the same check — browsers
// resend credentials on the upgrade request, so /api/clash/* WS proxying
// keeps working.
func BasicAuth(settingsMgr *settings.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if settingsMgr == nil {
				next.ServeHTTP(w, r)
				return
			}
			sec := settingsMgr.Get().Security
			if !sec.AuthEnabled {
				next.ServeHTTP(w, r)
				return
			}

			user, pass, ok := r.BasicAuth()
			// Constant-time comparison to avoid timing side channels.
			userOK := subtle.ConstantTimeCompare([]byte(user), []byte(sec.AuthUsername)) == 1
			passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(sec.AuthPassword)) == 1
			if !ok || !userOK || !passOK {
				w.Header().Set("WWW-Authenticate", `Basic realm="RouteBox", charset="UTF-8"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

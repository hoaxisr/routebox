package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
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

			// Fail closed on misconfiguration: auth_enabled without a password
			// would accept any client sending empty credentials.
			if sec.AuthPassword == "" {
				log.Printf("Warning: security.auth_enabled is true but auth_password is empty — denying all requests")
				w.Header().Set("WWW-Authenticate", `Basic realm="RouteBox", charset="UTF-8"`)
				http.Error(w, "Unauthorized: auth enabled without password", http.StatusUnauthorized)
				return
			}

			user, pass, ok := r.BasicAuth()
			// Hash inputs before comparing so ConstantTimeCompare cannot leak
			// credential length via a short-circuit on mismatched slice lengths.
			userOK := subtle.ConstantTimeCompare(sha256Sum(user), sha256Sum(sec.AuthUsername)) == 1
			passOK := subtle.ConstantTimeCompare(sha256Sum(pass), sha256Sum(sec.AuthPassword)) == 1
			if !ok || !userOK || !passOK {
				w.Header().Set("WWW-Authenticate", `Basic realm="RouteBox", charset="UTF-8"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sha256Sum returns the SHA-256 digest of s as a byte slice.
// Used to ensure ConstantTimeCompare always operates on fixed-length inputs,
// preventing credential-length leakage via slice-length short-circuits.
func sha256Sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

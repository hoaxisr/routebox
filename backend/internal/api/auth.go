package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"
	"strings"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
)

// SessionCookieName is the cookie holding the panel session token.
const SessionCookieName = "routebox_session"

// AuthMiddleware enforces auth when enabled. It accepts a valid session cookie
// OR HTTP Basic credentials (bcrypt-verified, lockout-limited). Settings are
// read per-request so runtime changes apply without a restart.
func AuthMiddleware(settingsMgr *settings.Manager, sessions *auth.SessionStore, limiter *auth.Limiter, verifier *auth.CachedVerifier) func(http.Handler) http.Handler {
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
			if c, err := r.Cookie(SessionCookieName); err == nil && sessions != nil && sessions.Validate(c.Value) {
				next.ServeHTTP(w, r)
				return
			}
			if sec.AuthPasswordHash == "" {
				log.Printf("Warning: auth_enabled with no password hash — denying all requests")
				unauthorized(w)
				return
			}
			user, pass, ok := r.BasicAuth()
			key := clientIP(r) + "|" + user
			if !ok || (limiter != nil && !limiter.Allowed(key)) {
				unauthorized(w)
				return
			}
			userOK := subtle.ConstantTimeCompare(sha256Sum(user), sha256Sum(sec.AuthUsername)) == 1
			passOK := verifier != nil && verifier.Verify(sec.AuthPasswordHash, pass)
			if !userOK || !passOK {
				if limiter != nil {
					limiter.Fail(key)
				}
				unauthorized(w)
				return
			}
			if limiter != nil {
				limiter.Reset(key)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="RouteBox", charset="UTF-8"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// clientIP extracts a best-effort client IP for lockout keying.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

func sha256Sum(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

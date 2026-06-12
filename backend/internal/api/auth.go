package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
)

// SessionCookieName is the cookie holding the panel session token.
const SessionCookieName = "routebox_session"

// bcryptGate bounds concurrent password verifications. Each bcrypt compare is
// deliberately expensive; without a cap, a brute-force flood (or rotating
// usernames that mint fresh lockout keys) could exhaust CPU on router-class
// hardware. Cache hits pass through near-instantly; only misses run bcrypt.
var bcryptGate = make(chan struct{}, 4)

func verifyPassword(v *auth.CachedVerifier, hash, pass string) bool {
	if v == nil {
		return false
	}
	bcryptGate <- struct{}{}
	defer func() { <-bcryptGate }()
	return v.Verify(hash, pass)
}

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
			if sec.AuthPasswordHash == "" || sec.AuthUsername == "" {
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
			passOK := verifyPassword(verifier, sec.AuthPasswordHash, pass)
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
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// clientIP returns the TCP peer IP for lockout keying. X-Forwarded-For is NOT
// trusted: it is attacker-controlled on a direct connection and would let a
// flood evade per-IP lockout. Behind a reverse proxy this collapses to the
// proxy IP, which (combined with the username in the key, and a valid session
// bypassing the limiter) is an acceptable, fail-closed trade-off.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func sha256Sum(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

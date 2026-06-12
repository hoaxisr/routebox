package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login verifies credentials and sets a session cookie.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sec := h.settings.Get().Security
	key := clientIP(r) + "|" + req.Username
	if h.limiter != nil && !h.limiter.Allowed(key) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	userOK := subtle.ConstantTimeCompare(sha256Sum(req.Username), sha256Sum(sec.AuthUsername)) == 1
	passOK := sec.AuthPasswordHash != "" && h.verifier != nil && h.verifier.Verify(sec.AuthPasswordHash, req.Password)
	if !userOK || !passOK {
		if h.limiter != nil {
			h.limiter.Fail(key)
		}
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if h.limiter != nil {
		h.limiter.Reset(key)
	}
	ttl := time.Duration(sec.SessionTimeoutMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	token, err := h.sessions.Create(ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure(r),
		MaxAge:   int(ttl.Seconds()),
	})
	writeSuccess(w, map[string]string{"username": sec.AuthUsername})
}

// Logout invalidates the current session and clears the cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil && h.sessions != nil {
		h.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeSuccess(w, map[string]string{"status": "ok"})
}

// Session reports whether the caller is authenticated (public endpoint so the
// SPA can choose between the login screen and the app).
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	sec := h.settings.Get().Security
	authed := !sec.AuthEnabled
	if !authed {
		if c, err := r.Cookie(SessionCookieName); err == nil && h.sessions != nil && h.sessions.Validate(c.Value) {
			authed = true
		}
	}
	writeSuccess(w, map[string]interface{}{
		"authenticated": authed,
		"auth_enabled":  sec.AuthEnabled,
		"username":      sec.AuthUsername,
	})
}

// isSecure reports whether the request arrived over TLS (direct or via a proxy).
func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

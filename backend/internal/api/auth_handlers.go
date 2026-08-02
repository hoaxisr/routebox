package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login verifies credentials and sets a session cookie.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sec := h.settings.Get().Security
	if sec.AuthPasswordHash == "" || sec.AuthUsername == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	key := lockKey(r, req.Username, trustedProxiesFrom(h.settings))
	if h.limiter != nil && !h.limiter.Allowed(key) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	userOK := subtle.ConstantTimeCompare(sha256Sum(req.Username), sha256Sum(sec.AuthUsername)) == 1
	passOK := sec.AuthPasswordHash != "" && verifyPassword(h.verifier, sec.AuthPasswordHash, req.Password)
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
	})
	writeSuccess(w, map[string]string{"username": sec.AuthUsername})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword verifies the current panel password (lockout-limited) and sets
// a new one. Registered in the AuthMiddleware-protected group, so the caller
// already holds a valid session; this re-checks the current password as
// defense-in-depth. The session cookie stays valid (the session token is
// independent of the password) — no forced re-login.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sec := h.settings.Get().Security
	if sec.AuthPasswordHash == "" {
		writeError(w, http.StatusBadRequest, "password auth is not configured")
		return
	}
	key := lockKey(r, sec.AuthUsername, trustedProxiesFrom(h.settings))
	if h.limiter != nil && !h.limiter.Allowed(key) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	if !verifyPassword(h.verifier, sec.AuthPasswordHash, req.CurrentPassword) {
		if h.limiter != nil {
			h.limiter.Fail(key)
		}
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if h.limiter != nil {
		h.limiter.Reset(key)
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if err := h.settings.Update(map[string]interface{}{"security.auth_password": req.NewPassword}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	if err := h.settings.Save(); err != nil {
		writeOpError(w, http.StatusInternalServerError, "failed to persist password", err)
		return
	}
	writeSuccess(w, map[string]string{"status": "ok"})
}

// Logout invalidates the current session and clears the cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if c, err := r.Cookie(SessionCookieName); err == nil && h.sessions != nil {
		h.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure(r),
	})
	writeSuccess(w, map[string]string{"status": "ok"})
}

// Session reports whether the caller is authenticated (public endpoint so the
// SPA can choose between the login screen and the app).
func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	sec := h.settings.Get().Security
	authed := !sec.AuthEnabled
	if !authed {
		if c, err := r.Cookie(SessionCookieName); err == nil && h.sessions != nil && h.sessions.Validate(c.Value) {
			authed = true
		}
	}
	resp := map[string]interface{}{
		"authenticated": authed,
		"auth_enabled":  sec.AuthEnabled,
	}
	if authed {
		resp["username"] = sec.AuthUsername
	}
	writeSuccess(w, resp)
}

// isSecure reports whether the request arrived over TLS (direct or via a proxy).
func isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.TrimSpace(proto) == "https"
}

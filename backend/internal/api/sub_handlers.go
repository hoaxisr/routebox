package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/users"
)

// GetSubscription serves the PUBLIC, unauthenticated per-user subscription at
// GET /sub/{token}. It is registered OUTSIDE AuthMiddleware (clients have no
// panel credentials). Security invariants:
//   - Anti-enumeration: unknown AND revoked tokens return a byte-identical 404
//     (a revoked user has Token=="" so ByToken rejects it via the SAME branch as
//     an unknown token — there is deliberately NO separate "revoked" path).
//   - Per-IP rate-limit via a dedicated limiter keyed by client IP. Allowed(ip)
//     then Fail(ip) on EVERY request, so valid-token floods are throttled too.
//   - The token never reaches the access log (see SubTokenScrubber, wired in
//     main.go).
//   - The 503 "public host not configured" policy lives HERE (the builder is
//     host-agnostic and pure).
//   - The body contains ONLY client share-links — never the registry, server
//     private keys, or other users' data. Errors never echo err.Error().
func (h *Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	// Per-IP rate-limit FIRST (before any registry work). Allowed → 429 check,
	// then count this request so a sustained burst from one IP trips backoff.
	ip := clientIP(r)
	if h.subLimiter != nil {
		if !h.subLimiter.Allowed(ip) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		h.subLimiter.Fail(ip) // count EVERY request (valid-token floods included)
	}

	token := chi.URLParam(r, "token")
	if h.panelUsers == nil {
		http.NotFound(w, r) // fail closed, indistinguishable from unknown token
		return
	}
	user, ok := h.panelUsers.ByToken(token)
	if !ok {
		// Unknown OR revoked: identical 404. No branch divergence.
		http.NotFound(w, r)
		return
	}

	host := ""
	if h.settings != nil {
		host = h.settings.Get().Server.PublicHost
	}
	if host == "" {
		http.Error(w, "public host not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := users.BuildSubscription(&user, h.config.GetActive(), host)
	if err != nil {
		// The builder is host-agnostic and never errors on the inputs above; treat
		// any residual as a server error WITHOUT echoing internals.
		http.Error(w, "subscription unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Profile-Update-Interval", "12")
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+sanitizeFilename(user.Name)+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body)) // may be empty base64 ("") — that is valid
}

// sanitizeFilename produces a safe Content-Disposition filename from a user name:
// keeps alphanumerics, dash, underscore, dot; replaces everything else with "_".
// Falls back to "subscription" when nothing usable remains.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "subscription"
	}
	return out
}

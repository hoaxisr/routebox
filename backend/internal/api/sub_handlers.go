package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/serverlinks"
	"routebox/backend/internal/users"
	"routebox/backend/internal/util"
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
	ip := clientIP(r, trustedProxiesFrom(h.settings))
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

	// Lifecycle gate: a disabled or expired user keeps a valid token (TokenDisabled
	// is a separate concern) but receives NO nodes. The standard headers and the
	// Userinfo expire are still emitted so clients render "expired"/empty.
	if !users.IsEffectivelyActive(user, time.Now().Unix()) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Profile-Update-Interval", "12")
		w.Header().Set("Content-Disposition",
			"attachment; filename=\""+sanitizeFilename(user.Name)+"\"")
		w.Header().Set("Cache-Control", "no-store")
		up, down := h.userAllTimeTraffic(user)
		w.Header().Set("Subscription-Userinfo", formatUserinfo(up, down, 0, user.ExpiresAt))
		w.WriteHeader(http.StatusOK)
		return // empty body
	}

	body, err := users.BuildSubscription(&user, h.config.GetActive(), serverlinks.PublicAddr{Host: host, Port: h.frontPort()})
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
	// Body carries live VPN credentials on a PUBLIC endpoint: forbid shared-cache
	// retention (proxies/CDNs) of the per-user subscription.
	w.Header().Set("Cache-Control", "no-store")
	// Per-user subscription usage (informational; total=0 = no quota this phase).
	// Bytes summed across the user's binding names; expire from ExpiresAt (0=never).
	up, down := h.userAllTimeTraffic(user)
	w.Header().Set("Subscription-Userinfo", formatUserinfo(up, down, 0, user.ExpiresAt))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body)) // may be empty base64 ("") — that is valid
}

// sanitizeFilename produces a safe Content-Disposition filename from a user name:
// keeps alphanumerics, dash, underscore, dot; replaces everything else with "_".
// Falls back to "subscription" when nothing usable remains. The sanitisation core
// lives in util so the awg package can reuse it without an import cycle.
func sanitizeFilename(name string) string {
	return util.SanitizeName(name, "subscription")
}

// contentDispositionAttachment builds an RFC 6266 attachment header for a file
// named after a user-supplied display name. It always emits the ASCII `filename=`
// form (header values are bytes; a raw "Ноутбук" there is undefined behaviour and
// several clients mangle it), and adds the RFC 5987 `filename*` (UTF-8) form whenever the
// ASCII reduction lost something — every modern browser prefers filename* and
// saves the name the user actually typed. ext is appended to both forms.
func contentDispositionAttachment(name, fallback, ext string) string {
	ascii := util.SanitizeName(name, fallback) + ext
	cd := `attachment; filename="` + ascii + `"`
	if full := name + ext; full != ascii && utf8.ValidString(full) {
		cd += `; filename*=UTF-8''` + rfc5987Encode(full)
	}
	return cd
}

// rfc5987Encode percent-encodes every byte outside RFC 5987 attr-char, so the
// result is safe as the value part of an RFC 5987 filename* parameter.
func rfc5987Encode(s string) string {
	const attrExtra = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.IndexByte(attrExtra, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

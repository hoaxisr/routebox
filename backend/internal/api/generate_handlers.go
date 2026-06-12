package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/google/uuid"
)

// GenerateUUID returns a fresh random UUIDv4 for vless users.
func (h *Handler) GenerateUUID(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]string{"uuid": uuid.NewString()})
}

// GeneratePassword returns a 16-byte cryptographically random password,
// base64url-encoded (no padding), for naive/hysteria2 users.
func (h *Handler) GeneratePassword(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate password")
		return
	}
	writeSuccess(w, map[string]string{"password": base64.RawURLEncoding.EncodeToString(b)})
}

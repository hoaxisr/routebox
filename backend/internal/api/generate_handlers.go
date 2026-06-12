package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

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

// parseRealityKeypair extracts the private and public keys from the output of
// `<binary> generate reality-keypair` (lines "PrivateKey: ..." / "PublicKey: ...").
func parseRealityKeypair(out string) (priv, pub string, err error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "PrivateKey:"):
			priv = strings.TrimSpace(strings.TrimPrefix(line, "PrivateKey:"))
		case strings.HasPrefix(line, "PublicKey:"):
			pub = strings.TrimSpace(strings.TrimPrefix(line, "PublicKey:"))
		}
	}
	if priv == "" || pub == "" {
		return "", "", fmt.Errorf("reality keypair not found in output")
	}
	return priv, pub, nil
}

// GenerateReality runs the detected binary to produce a Reality keypair.
func (h *Handler) GenerateReality(w http.ResponseWriter, r *http.Request) {
	bin := h.process.GetBinaryPath()
	if bin == "" {
		writeError(w, http.StatusServiceUnavailable, "no amnezia-box binary detected")
		return
	}
	out, err := exec.Command(bin, "generate", "reality-keypair").CombinedOutput()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable,
			fmt.Sprintf("binary does not support reality-keypair generation: %v", err))
		return
	}
	priv, pub, err := parseRealityKeypair(string(out))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"private_key": priv, "public_key": pub})
}

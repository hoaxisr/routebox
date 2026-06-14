package users

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateToken returns an opaque base64url(32 crypto/rand bytes) token — the
// same recipe as auth session tokens (auth.SessionStore.Create). 43 chars, raw
// (unpadded), URL-safe, so it is safe as a path segment in /sub/<token>.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ByToken returns a deep copy of the user whose Token equals the argument. An
// empty token NEVER matches (a revoked user has Token=="" and must not be
// reachable). Returns a value (PanelUser) to mirror Get/List deep-copy semantics
// so callers — including the PUBLIC /sub handler — can never mutate the registry
// through the result; take &user when a pointer is needed.
func (m *Manager) ByToken(token string) (PanelUser, bool) {
	if token == "" {
		return PanelUser{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.byID {
		if u.Token == token {
			return *cloneUser(u), true
		}
	}
	return PanelUser{}, false
}

// RotateToken generates a fresh token for the user, persists, and returns it.
// Unknown id is an error. The old token is invalidated immediately.
func (m *Manager) RotateToken(id string) (string, error) {
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return "", fmt.Errorf("user %q not found", id)
	}
	prevTok, prevDisabled := u.Token, u.TokenDisabled
	u.Token = tok
	u.TokenDisabled = false // rotating re-enables a revoked user (deliberate re-issue)
	if err := m.saveLocked(); err != nil {
		u.Token, u.TokenDisabled = prevTok, prevDisabled // keep memory consistent with disk on failure
		return "", err
	}
	return tok, nil
}

// RevokeToken clears the user's token (Token="") AND sets TokenDisabled=true,
// persists, and returns. After this ByToken can never find the user again
// (subscription disabled). TokenDisabled makes the revoke STICKY: the reconciler
// will not auto-re-mint a token while the user is still bound — only Rotate
// re-enables. Unknown id is an error.
func (m *Manager) RevokeToken(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return fmt.Errorf("user %q not found", id)
	}
	prevTok, prevDisabled := u.Token, u.TokenDisabled
	u.Token = ""
	u.TokenDisabled = true // sticky: the reconciler will not auto-re-mint until Rotate re-enables
	if err := m.saveLocked(); err != nil {
		u.Token, u.TokenDisabled = prevTok, prevDisabled
		return err
	}
	return nil
}

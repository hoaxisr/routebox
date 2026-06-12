package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type session struct {
	expires time.Time
	ttl     time.Duration
}

// SessionStore is an in-memory token store with sliding expiry. Sessions are
// lost on process restart (acceptable — users re-login).
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*session
	now      func() time.Time
}

// NewSessionStore creates an empty store.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: map[string]*session{}, now: time.Now}
}

// Create issues a new opaque session token valid for ttl.
func (s *SessionStore) Create(ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = &session{expires: s.now().Add(ttl), ttl: ttl}
	s.mu.Unlock()
	return token, nil
}

// Validate reports whether token is live, sliding its expiry forward on success.
func (s *SessionStore) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return false
	}
	if s.now().After(sess.expires) {
		delete(s.sessions, token)
		return false
	}
	sess.expires = s.now().Add(sess.ttl)
	return true
}

// Delete invalidates a token (logout).
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// Cleanup drops expired sessions; call periodically.
func (s *SessionStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for t, sess := range s.sessions {
		if now.After(sess.expires) {
			delete(s.sessions, t)
		}
	}
}

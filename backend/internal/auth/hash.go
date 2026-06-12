// Package auth provides password hashing, sessions, and brute-force lockout
// for the RouteBox admin panel.
package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of plain.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether plain matches the bcrypt hash.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// CachedVerifier avoids repeated bcrypt work on the hot path (scripts sending
// HTTP Basic on every request). Only SUCCESSFUL verifications are cached, keyed
// by the password digest against the active hash; the cache is dropped whenever
// the hash changes. Wrong passwords are never cached, so the cache cannot be
// poisoned.
type CachedVerifier struct {
	mu   sync.Mutex
	hash string
	ok   map[string]bool
}

// NewCachedVerifier creates an empty verifier cache.
func NewCachedVerifier() *CachedVerifier {
	return &CachedVerifier{ok: map[string]bool{}}
}

// Verify checks plain against hash, using/populating the success cache.
func (c *CachedVerifier) Verify(hash, plain string) bool {
	key := digest(plain)
	c.mu.Lock()
	if hash != c.hash {
		c.hash = hash
		c.ok = map[string]bool{}
	}
	if c.ok[key] {
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()

	if VerifyPassword(hash, plain) {
		c.mu.Lock()
		if hash == c.hash {
			c.ok[key] = true
		}
		c.mu.Unlock()
		return true
	}
	return false
}

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

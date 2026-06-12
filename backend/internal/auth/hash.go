// Package auth provides password hashing, sessions, and brute-force lockout
// for the RouteBox admin panel.
package auth

import (
	"crypto/rand"
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
// poisoned. It is intended for a single active credential (one hash at a time);
// routing multiple distinct hashes through one verifier is safe but degrades to
// no caching.
type CachedVerifier struct {
	mu   sync.Mutex
	hash string
	ok   map[string]bool
	salt []byte
}

// NewCachedVerifier creates an empty verifier cache with a per-process salt.
func NewCachedVerifier() *CachedVerifier {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand failure is fatal-grade; fall back to an empty salt
		// (digest still differs from a raw sha256 of the password is not
		// guaranteed, but rand.Read effectively never fails on supported OSes).
		salt = nil
	}
	return &CachedVerifier{ok: map[string]bool{}, salt: salt}
}

func (c *CachedVerifier) digest(plain string) string {
	h := sha256.New()
	h.Write(c.salt)
	h.Write([]byte(plain))
	return hex.EncodeToString(h.Sum(nil))
}

// Verify checks plain against hash, using/populating the success cache.
func (c *CachedVerifier) Verify(hash, plain string) bool {
	key := c.digest(plain)
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

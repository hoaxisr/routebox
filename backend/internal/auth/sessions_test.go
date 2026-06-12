package auth

import (
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	s := NewSessionStore()
	clock := time.Unix(1000, 0)
	s.now = func() time.Time { return clock }

	tok, err := s.Create(10 * time.Minute)
	if err != nil || tok == "" {
		t.Fatalf("create: %v tok=%q", err, tok)
	}
	if !s.Validate(tok) {
		t.Fatal("fresh token should validate")
	}
	if s.Validate("bogus") {
		t.Fatal("unknown token must not validate")
	}
	if s.Validate("") {
		t.Fatal("empty token must not validate")
	}

	// Sliding expiry: advancing within ttl keeps it alive and pushes expiry.
	clock = clock.Add(9 * time.Minute)
	if !s.Validate(tok) {
		t.Fatal("token within ttl should still validate")
	}
	clock = clock.Add(9 * time.Minute) // 18m from create, but slid at 9m
	if !s.Validate(tok) {
		t.Fatal("sliding expiry should keep token alive")
	}

	// Past ttl with no activity -> expired.
	clock = clock.Add(11 * time.Minute)
	if s.Validate(tok) {
		t.Fatal("token past ttl must expire")
	}

	// Delete removes a live token.
	tok2, _ := s.Create(10 * time.Minute)
	s.Delete(tok2)
	if s.Validate(tok2) {
		t.Fatal("deleted token must not validate")
	}
}

func TestSessionCleanup(t *testing.T) {
	s := NewSessionStore()
	clock := time.Unix(0, 0)
	s.now = func() time.Time { return clock }
	tok, _ := s.Create(1 * time.Minute)
	clock = clock.Add(2 * time.Minute)
	s.Cleanup()
	if s.Validate(tok) {
		t.Fatal("cleanup should drop expired sessions")
	}
}

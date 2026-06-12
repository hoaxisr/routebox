package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h == "s3cret" || h == "" {
		t.Fatalf("hash looks wrong: %q", h)
	}
	if !VerifyPassword(h, "s3cret") {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("wrong password must not verify")
	}
	if VerifyPassword("not-a-hash", "s3cret") {
		t.Fatal("garbage hash must not verify")
	}
}

func TestCachedVerifier(t *testing.T) {
	h1, _ := HashPassword("pw1")
	v := NewCachedVerifier()
	if !v.Verify(h1, "pw1") {
		t.Fatal("first verify should pass")
	}
	if !v.Verify(h1, "pw1") {
		t.Fatal("cached verify should pass")
	}
	if v.Verify(h1, "pw2") {
		t.Fatal("wrong password must not pass and must not be cached")
	}
	// Rotating the hash must invalidate the cache.
	h2, _ := HashPassword("pw2")
	if v.Verify(h2, "pw1") {
		t.Fatal("old password must not verify against new hash")
	}
	if !v.Verify(h2, "pw2") {
		t.Fatal("new password should verify against new hash")
	}
}

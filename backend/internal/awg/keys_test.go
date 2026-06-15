package awg

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// Verified real X25519 pair (priv = bytes 1..32). The '/' in the pub makes it
// DISCRIMINATE std vs base64url: a base64url impl cannot reproduce it.
const (
	keyVectorPriv = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	keyVectorPub  = "B6N8vBQgk8i3VdwbEOhstCY3StFqqFPtC9/AsrhtHHw="
)

func TestPublicFromPrivate(t *testing.T) {
	got, err := PublicFromPrivate(keyVectorPriv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != keyVectorPub {
		t.Fatalf("got %q, want %q", got, keyVectorPub)
	}
}

// TestPublicFromPrivateRejectsBase64URL proves the decoder is StdEncoding, not
// base64url. A 32-byte key of all-0xFF std-encodes WITH a '/' and url-encodes
// WITH '_' — feeding the url form (which StdEncoding cannot decode) must error.
// This is the assertion the spec mandates; a base64url impl would silently
// accept the url form and produce a wrong-but-valid-looking key.
func TestPublicFromPrivateRejectsBase64URL(t *testing.T) {
	raw := bytes.Repeat([]byte{0xff}, 32)
	stdForm := base64.StdEncoding.EncodeToString(raw)    // contains '/'
	urlForm := base64.RawURLEncoding.EncodeToString(raw) // uses '_' instead, no padding
	if stdForm == urlForm {
		t.Fatal("test setup invalid: std and url forms must differ")
	}
	if _, err := PublicFromPrivate(urlForm); err == nil {
		t.Fatalf("PublicFromPrivate must reject base64url input %q (StdEncoding-only)", urlForm)
	}
	// And the std form of the same bytes decodes fine (sanity: it's the encoding, not the bytes).
	if _, err := PublicFromPrivate(stdForm); err != nil {
		t.Fatalf("std form of 0xFF*32 should decode: %v", err)
	}
}

func TestPublicFromPrivateRejectsGarbage(t *testing.T) {
	if _, err := PublicFromPrivate("not-base64!!"); err == nil {
		t.Fatal("want error for invalid base64")
	}
	if _, err := PublicFromPrivate(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("want error for wrong key length")
	}
}

func TestGenerateInGoFallback(t *testing.T) {
	// Deterministic rand -> deterministic, derivable keypair (32 zero bytes).
	priv, pub, err := Generate(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(raw) != 32 {
		t.Fatalf("stored priv must be 32 raw std-base64 bytes: %q", priv)
	}
	derived, err := PublicFromPrivate(priv)
	if err != nil || derived != pub {
		t.Fatalf("pub mismatch: gen=%q derived=%q (%v)", pub, derived, err)
	}
}

func TestGeneratePSK(t *testing.T) {
	psk, err := GeneratePSK(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatalf("GeneratePSK: %v", err)
	}
	if raw, err := base64.StdEncoding.DecodeString(psk); err != nil || len(raw) != 32 {
		t.Fatalf("PSK must be 32 std-base64 bytes: %q", psk)
	}
}

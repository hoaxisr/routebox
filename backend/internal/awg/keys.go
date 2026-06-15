package awg

import (
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
	"io"
)

// PublicFromPrivate derives the WireGuard public key (STANDARD base64, unlike
// serverlinks.RealityPublicFromPrivate which uses base64url) from a std-base64
// private key. WG keys are X25519, so this reproduces `awg pubkey`.
func PublicFromPrivate(privStdB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(privStdB64)
	if err != nil {
		return "", fmt.Errorf("decode private key: %w", err)
	}
	priv, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("invalid x25519 private key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}

// Generate returns a fresh keypair in std-base64. The stored private key is the
// RAW 32 random bytes (matching `awg genkey`; clamping happens at use). rand is
// injected for deterministic tests (pass crypto/rand.Reader in production).
func Generate(rand io.Reader) (priv, pub string, err error) {
	b := make([]byte, 32)
	if _, err = io.ReadFull(rand, b); err != nil {
		return "", "", err
	}
	priv = base64.StdEncoding.EncodeToString(b)
	pub, err = PublicFromPrivate(priv)
	if err != nil {
		return "", "", err
	}
	return priv, pub, nil
}

// GeneratePSK returns a 32-byte std-base64 preshared key.
func GeneratePSK(rand io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

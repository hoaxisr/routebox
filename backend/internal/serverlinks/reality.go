// Package serverlinks builds client share-links from server inbound configs.
package serverlinks

import (
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
)

// RealityPublicFromPrivate derives the Reality public key (base64url, no
// padding) from a private key in the same encoding. sing-box stores only the
// private key in the inbound config; the public key needed for the client
// share-link is derived on demand. Reality keys are X25519, so this reproduces
// `privateKey.PublicKey()` (curve25519 scalar base mult) exactly.
func RealityPublicFromPrivate(privB64url string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(privB64url)
	if err != nil {
		return "", fmt.Errorf("decode private key: %w", err)
	}
	priv, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("invalid x25519 private key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}

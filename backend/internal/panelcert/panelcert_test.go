package panelcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// writeCacheEntry builds an autocert-style DirCache file: a PEM EC PRIVATE KEY block
// followed by a CERTIFICATE block (the on-disk format autocert uses).
func writeCacheEntry(t *testing.T, cacheDir, domain string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{domain}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	var buf []byte
	buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
	buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, domain), buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExportFromCacheSplitsAndDetectsChange(t *testing.T) {
	cacheDir := t.TempDir()
	out := t.TempDir()
	writeCacheEntry(t, cacheDir, "x.example.com")

	changed, err := ExportFromCache(cacheDir, "x.example.com", out)
	if err != nil || !changed {
		t.Fatalf("first export: changed=%v err=%v (want true,nil)", changed, err)
	}
	fc, key := Files(out)
	if blk, _ := pem.Decode(mustRead(t, fc)); blk == nil || blk.Type != "CERTIFICATE" {
		t.Fatalf("fullchain.pem is not a PEM CERTIFICATE")
	}
	if blk, _ := pem.Decode(mustRead(t, key)); blk == nil || blk.Type != "EC PRIVATE KEY" {
		t.Fatalf("key.pem is not a PEM EC PRIVATE KEY")
	}
	if fi, _ := os.Stat(key); fi != nil && fi.Mode().Perm() != 0o600 {
		t.Fatalf("key.pem perm = %v, want 0600", fi.Mode().Perm())
	}
	if changed, _ := ExportFromCache(cacheDir, "x.example.com", out); changed {
		t.Fatal("re-export of identical cache entry must report changed=false")
	}
}

func TestExportFromCacheMissingIsNotExist(t *testing.T) {
	if _, err := ExportFromCache(t.TempDir(), "absent", t.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("missing cache entry must return os.IsNotExist, got %v", err)
	}
}

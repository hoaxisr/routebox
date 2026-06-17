package panelcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"
)

func selfSigned(t *testing.T) *tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "x.example.com"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"x.example.com"}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestExportWritesParseablePEMAndDetectsChange(t *testing.T) {
	dir := t.TempDir()
	cert := selfSigned(t)

	changed, err := Export(cert, dir)
	if err != nil || !changed {
		t.Fatalf("first export: changed=%v err=%v (want true,nil)", changed, err)
	}
	fc, key := Files(dir)

	pemBytes, _ := os.ReadFile(fc)
	if blk, _ := pem.Decode(pemBytes); blk == nil || blk.Type != "CERTIFICATE" {
		t.Fatalf("fullchain.pem is not a PEM CERTIFICATE:\n%s", pemBytes)
	}
	keyBytes, _ := os.ReadFile(key)
	if blk, _ := pem.Decode(keyBytes); blk == nil || blk.Type != "PRIVATE KEY" {
		t.Fatalf("key.pem is not a PEM PRIVATE KEY")
	}
	if fi, _ := os.Stat(key); fi != nil && fi.Mode().Perm() != 0o600 {
		t.Fatalf("key.pem perm = %v, want 0600", fi.Mode().Perm())
	}
	if changed, _ := Export(cert, dir); changed {
		t.Fatal("re-export of identical cert must report changed=false")
	}
}

func TestExportEmptyCertErrors(t *testing.T) {
	if _, err := Export(&tls.Certificate{}, t.TempDir()); err == nil {
		t.Fatal("empty cert must error")
	}
}

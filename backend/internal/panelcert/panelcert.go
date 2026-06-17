// Package panelcert maintains the panel's TLS certificate as PEM at a canonical path
// so server inbounds can reuse it (one ACME owner = the panel). Export/CopyManual/Files
// are pure IO + testable; Refresh wires it to autocert at runtime.
package panelcert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// Files returns the canonical fullchain + key PEM paths under dir.
func Files(dir string) (fullchain, key string) {
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "key.pem")
}

// Export writes cert's chain (fullchain.pem) + private key (key.pem, 0600) under dir
// (created 0700), atomically. Returns changed=true when the on-disk fullchain differs
// from the new one (caller reloads consumers only then).
func Export(cert *tls.Certificate, dir string) (changed bool, err error) {
	if cert == nil || len(cert.Certificate) == 0 {
		return false, fmt.Errorf("empty certificate")
	}
	var chain []byte
	for _, der := range cert.Certificate {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return false, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	fullchainPath, keyPath := Files(dir)
	old, _ := os.ReadFile(fullchainPath)
	changed = string(old) != string(chain)
	if err := atomicWrite(fullchainPath, chain, 0o644); err != nil {
		return false, err
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		return false, err
	}
	return changed, nil
}

// CopyManual copies an operator-supplied cert/key PEM into the canonical path (manual
// TLS mode — no autocert). A missing source returns an error (caller logs + ignores).
func CopyManual(certPath, keyPath, dir string) error {
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fc, k := Files(dir)
	if err := atomicWrite(fc, cert, 0o644); err != nil {
		return err
	}
	return atomicWrite(k, key, 0o600)
}

// Refresh keeps the canonical PEM current from autocert: on start (which also triggers
// issuance) and every 12h, fetch the cert for domain, export it, and on change invoke
// reload (e.g. SIGHUP amnezia-box). Runs until ctx is cancelled.
func Refresh(ctx context.Context, am *autocert.Manager, domain, dir string, reload func() error) {
	tick := func() {
		cert, err := am.GetCertificate(&tls.ClientHelloInfo{ServerName: domain})
		if err != nil {
			log.Printf("panelcert: get cert for %s: %v", domain, err)
			return
		}
		changed, err := Export(cert, dir)
		if err != nil {
			log.Printf("panelcert: export: %v", err)
			return
		}
		if changed && reload != nil {
			if err := reload(); err != nil {
				log.Printf("panelcert: reload after cert change: %v", err)
			}
		}
	}
	tick()
	tk := time.NewTicker(12 * time.Hour)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			tick()
		}
	}
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

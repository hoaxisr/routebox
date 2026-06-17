// Package panelcert mirrors the panel's TLS certificate to a canonical PEM path so
// server inbounds can reuse it (one ACME owner = the panel). It reads autocert's
// DirCache entry directly (deterministic, no blocking) rather than calling
// Manager.GetCertificate, and exposes a manual-mode copy + a refresh loop.
package panelcert

import (
	"context"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files returns the canonical fullchain + key PEM paths under dir.
func Files(dir string) (fullchain, key string) {
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "key.pem")
}

// ExportFromCache reads autocert's DirCache entry for domain — a single file holding
// the PEM private key block followed by the certificate-chain blocks — and writes
// fullchain.pem + key.pem (0600) under outDir (created 0700), atomically. Returns
// changed=true when the fullchain differs from what's on disk. The ReadFile error is
// returned verbatim so callers can treat a not-yet-issued cert (os.IsNotExist) as
// normal.
func ExportFromCache(cacheDir, domain, outDir string) (changed bool, err error) {
	raw, err := os.ReadFile(filepath.Join(cacheDir, domain))
	if err != nil {
		return false, err
	}
	var certPEM, keyPEM []byte
	for rest := raw; ; {
		blk, r := pem.Decode(rest)
		if blk == nil {
			break
		}
		if strings.HasSuffix(blk.Type, "PRIVATE KEY") {
			keyPEM = append(keyPEM, pem.EncodeToMemory(blk)...)
		} else if blk.Type == "CERTIFICATE" {
			certPEM = append(certPEM, pem.EncodeToMemory(blk)...)
		}
		rest = r
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return false, fmt.Errorf("cache entry %q missing cert or key", domain)
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return false, err
	}
	fc, k := Files(outDir)
	old, _ := os.ReadFile(fc)
	changed = string(old) != string(certPEM)
	if err := atomicWrite(fc, certPEM, 0o644); err != nil {
		return false, err
	}
	if err := atomicWrite(k, keyPEM, 0o600); err != nil {
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

// Refresh keeps the canonical PEM mirrored from autocert's DirCache: on start and every
// 5 minutes, export the cache entry for domain; on change, call reload (e.g. SIGHUP
// amnezia-box) so it picks up the new cert. The frequent poll catches first issuance
// (autocert writes the cache only after the panel's first TLS handshake) and renewals.
// A not-yet-issued cache entry is normal and silent. Runs until ctx is cancelled.
func Refresh(ctx context.Context, cacheDir, domain, outDir string, reload func() error) {
	tick := func() {
		changed, err := ExportFromCache(cacheDir, domain, outDir)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("panelcert: export from cache: %v", err)
			}
			return
		}
		if changed && reload != nil {
			if err := reload(); err != nil {
				log.Printf("panelcert: reload after cert change: %v", err)
			}
		}
	}
	tick()
	tk := time.NewTicker(5 * time.Minute)
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

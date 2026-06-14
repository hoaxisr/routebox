package main

import (
	"testing"

	"routebox/backend/internal/settings"
)

func TestResolveTLSMode(t *testing.T) {
	t.Run("acme enabled with public host => acme", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.ACMEEnabled = true
		cfg.Server.PublicHost = "panel.example.com"
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeACME {
			t.Fatalf("got %v, want tlsModeACME", mode)
		}
	})

	t.Run("acme enabled without public host => hard error", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.ACMEEnabled = true
		cfg.Server.PublicHost = ""
		_, err := resolveTLSMode(cfg)
		if err == nil {
			t.Fatal("expected error when acme_enabled but public_host empty")
		}
	})

	t.Run("acme wins over manual when both configured", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.ACMEEnabled = true
		cfg.Server.PublicHost = "panel.example.com"
		cfg.Network.TLSCertPath = "/x/cert.pem"
		cfg.Network.TLSKeyPath = "/x/key.pem"
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeACME {
			t.Fatalf("priority broken: got %v, want tlsModeACME", mode)
		}
	})

	t.Run("manual when cert+key set and acme off", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.TLSCertPath = "/x/cert.pem"
		cfg.Network.TLSKeyPath = "/x/key.pem"
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeManual {
			t.Fatalf("got %v, want tlsModeManual", mode)
		}
	})

	t.Run("manual requires BOTH cert and key (only cert => off)", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.TLSCertPath = "/x/cert.pem" // key missing
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeOff {
			t.Fatalf("got %v, want tlsModeOff (incomplete manual pair)", mode)
		}
	})

	t.Run("manual requires BOTH cert and key (only key => off)", func(t *testing.T) {
		cfg := settings.Default()
		cfg.Network.TLSKeyPath = "/x/key.pem" // cert missing
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeOff {
			t.Fatalf("got %v, want tlsModeOff (incomplete manual pair)", mode)
		}
	})

	t.Run("off when nothing configured", func(t *testing.T) {
		cfg := settings.Default()
		mode, err := resolveTLSMode(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != tlsModeOff {
			t.Fatalf("got %v, want tlsModeOff", mode)
		}
	})
}

func TestACMEDirectoryURL(t *testing.T) {
	t.Run("staging => LE staging directory", func(t *testing.T) {
		want := "https://acme-staging-v02.api.letsencrypt.org/directory"
		if got := acmeDirectoryURL(true); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("production => empty (autocert default)", func(t *testing.T) {
		if got := acmeDirectoryURL(false); got != "" {
			t.Fatalf("got %q, want empty string", got)
		}
	})
}

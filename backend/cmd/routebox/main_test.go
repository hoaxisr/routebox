package main

import (
	"os"
	"path/filepath"
	"testing"

	"routebox/backend/internal/process"
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

func TestShouldAutoStartAmneziaBox(t *testing.T) {
	// The state a container comes up in: vps mode, no systemd, config scaffolded,
	// binary present, nothing running. This is the one case that must start.
	container := func() (string, process.Status, bool, bool) {
		return "vps", process.Status{}, true, true
	}

	t.Run("no systemd unit and nothing running => start", func(t *testing.T) {
		if !shouldAutoStartAmneziaBox(container()) {
			t.Fatal("a panel with no supervisor must start amnezia-box itself")
		}
	})

	t.Run("systemd unit detected => leave it to systemd", func(t *testing.T) {
		mode, st, cfgOK, binOK := container()
		st.ServiceName = "amnezia-box" // unit exists, currently stopped
		if shouldAutoStartAmneziaBox(mode, st, cfgOK, binOK) {
			t.Fatal("starting here races the unit systemd is about to start")
		}
	})

	t.Run("already running => no second process", func(t *testing.T) {
		mode, st, cfgOK, binOK := container()
		st.Running = true
		if shouldAutoStartAmneziaBox(mode, st, cfgOK, binOK) {
			t.Fatal("must not start a process that is already up")
		}
	})

	t.Run("router mode => wizard's job", func(t *testing.T) {
		_, st, cfgOK, binOK := container()
		if shouldAutoStartAmneziaBox("router", st, cfgOK, binOK) {
			t.Fatal("router mode must not bring a TUN up unprompted")
		}
	})

	t.Run("no config => nothing to start with", func(t *testing.T) {
		mode, st, _, binOK := container()
		if shouldAutoStartAmneziaBox(mode, st, false, binOK) {
			t.Fatal("must not start without a config file")
		}
	})

	t.Run("no binary => nothing to start", func(t *testing.T) {
		mode, st, cfgOK, _ := container()
		if shouldAutoStartAmneziaBox(mode, st, cfgOK, false) {
			t.Fatal("must not attempt a start with no amnezia-box installed")
		}
	})
}

func TestFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if fileExists(path) {
		t.Fatal("missing file reported as existing")
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Fatal("existing file reported as missing")
	}
	if fileExists("") {
		t.Fatal("empty path must never count as an existing file")
	}
}

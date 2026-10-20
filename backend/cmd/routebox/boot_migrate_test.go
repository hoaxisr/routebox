package main

import (
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"routebox/backend/internal/settings"
	"routebox/backend/internal/webfront"
)

// upgraded is an all-in-one install bootstrapped by a release that predates
// the Telegram WEB proxy: marked bootstrapped, dest on 127.0.0.1:9443, and the
// [mtproto] block still at its defaults.
func upgraded(t *testing.T) *settings.Manager {
	t.Helper()
	sm := newVPSSettings(t, t.TempDir())
	if err := sm.Update(map[string]interface{}{"server.public_host": "media.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := sm.SetBootstrap("/panel-secret", "127.0.0.1:9443", filepath.Join(t.TempDir(), "Caddyfile"), 443); err != nil {
		t.Fatal(err)
	}
	return sm
}

// The upgrade path: an install that never ran the new bootstrap gets the two
// keys a fresh one is written with, survives a restart with them, and does
// not find the WEB proxy switched on behind its back.
func TestMigrateBootstrapMtprotoFixesAnUpgradedInstall(t *testing.T) {
	sm := upgraded(t)

	if err := migrateBootstrapMtproto(sm); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mt := newVPSSettings(t, filepath.Dir(sm.GetPath())).Get().Mtproto
	if mt.Listen != bootstrapMtprotoListen {
		t.Errorf("listen = %q, want %q — 9443 is dest's here", mt.Listen, bootstrapMtprotoListen)
	}
	if mt.MaskingDomain != "media.example.com" {
		t.Errorf("masking_domain = %q, want server.public_host", mt.MaskingDomain)
	}
	if mt.WebEnabled {
		t.Error("an upgrade switched the WEB proxy on; the operator opts in with the toggle")
	}
}

// Anything but the two broken states is the operator's choice.
func TestMigrateBootstrapMtprotoKeepsTheOperatorsChoices(t *testing.T) {
	sm := upgraded(t)
	if err := sm.Update(map[string]interface{}{
		"mtproto.listen":         "0.0.0.0:8888",
		"mtproto.masking_domain": "other.example.org",
	}); err != nil {
		t.Fatal(err)
	}

	if err := migrateBootstrapMtproto(sm); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mt := sm.Get().Mtproto
	if mt.Listen != "0.0.0.0:8888" || mt.MaskingDomain != "other.example.org" {
		t.Errorf("migration overwrote the operator's settings: listen %q, masking_domain %q", mt.Listen, mt.MaskingDomain)
	}
}

// dest dials the front at 127.0.0.1:<port>, so a web_listen on another host
// (hand-edited, or set before the panel checked it) is moved there, port kept;
// one dest can reach, a wildcard included, stays.
func TestMigrateBootstrapMtprotoMovesAnUnreachableWebListen(t *testing.T) {
	for listen, want := range map[string]string{
		"10.0.0.5:9555":  "127.0.0.1:9555",
		"[::1]:9556":     "127.0.0.1:9556",
		"localhost:9557": "127.0.0.1:9557",
		"127.0.0.1:9558": "127.0.0.1:9558",
		"0.0.0.0:9559":   "0.0.0.0:9559",
		"[::]:9560":      "[::]:9560",
		":9561":          ":9561",
	} {
		sm := newVPSSettings(t, t.TempDir())
		// Before SetBootstrap: the panel would refuse it afterwards.
		if err := sm.Update(map[string]interface{}{
			"server.public_host": "media.example.com",
			"mtproto.web_listen": listen,
		}); err != nil {
			t.Fatal(err)
		}
		if err := sm.SetBootstrap("/panel-secret", "127.0.0.1:9443", filepath.Join(t.TempDir(), "Caddyfile"), 443); err != nil {
			t.Fatal(err)
		}

		if err := migrateBootstrapMtproto(sm); err != nil {
			t.Fatalf("%s: migrate: %v", listen, err)
		}

		if got := newVPSSettings(t, filepath.Dir(sm.GetPath())).Get().Mtproto.WebListen; got != want {
			t.Errorf("%s: web_listen = %q after a restart, want %q", listen, got, want)
		}

		// And what is there now is a value a failed save can roll back to.
		if err := sm.Update(map[string]interface{}{"mtproto.web_listen": "127.0.0.1:9600"}); err != nil {
			t.Fatal(err)
		}
		if err := sm.Update(map[string]interface{}{"mtproto.web_listen": want}); err != nil {
			t.Errorf("%s: rolling back to %q was refused: %v", listen, want, err)
		}
	}
}

// Every other install has no dest to collide with and no domain of ours.
func TestMigrateBootstrapMtprotoLeavesOtherInstallsAlone(t *testing.T) {
	sm := newVPSSettings(t, t.TempDir())
	if err := sm.Update(map[string]interface{}{"server.public_host": "media.example.com"}); err != nil {
		t.Fatal(err)
	}

	if err := migrateBootstrapMtproto(sm); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	def := settings.Default().Mtproto
	if mt := sm.Get().Mtproto; mt.Listen != def.Listen || mt.MaskingDomain != "" {
		t.Errorf("a non-bootstrapped install was migrated: %+v", mt)
	}
}

// A busy web_listen on a bootstrapped install must not take the process down,
// and must not re-plan dest onto a front that is not listening.
func TestStartBootstrapFrontSurvivesABusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	front := webfront.New(http.NotFoundHandler())
	t.Cleanup(func() { _ = front.Stop() })

	replans := 0
	startBootstrapFront(front, busy.Addr().String(), func() error { replans++; return nil })

	if front.Listen() != "" {
		t.Errorf("front listens on %q, want down", front.Listen())
	}
	if replans != 0 {
		t.Errorf("re-planned %d times onto a front that is down, want 0", replans)
	}
}

func TestStartBootstrapFrontReplansOnceItListens(t *testing.T) {
	front := webfront.New(http.NotFoundHandler())
	t.Cleanup(func() { _ = front.Stop() })

	replans := 0
	startBootstrapFront(front, "127.0.0.1:0", func() error { replans++; return nil })

	if front.Listen() == "" {
		t.Fatal("front is not listening")
	}
	if replans != 1 {
		t.Errorf("re-planned %d times, want 1", replans)
	}
}

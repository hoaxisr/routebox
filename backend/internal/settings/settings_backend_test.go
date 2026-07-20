package settings

import "testing"

func TestAwgBackend_Update(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"awg.backend": "singbox"}); err != nil {
		t.Fatal(err)
	}
	if m.settings.Awg.Backend != "singbox" {
		t.Fatalf("backend = %q, want singbox", m.settings.Awg.Backend)
	}
	if err := m.Update(map[string]interface{}{"awg.backend": "bogus"}); err == nil {
		t.Fatal("expected error for bogus backend")
	}
}

func TestAwgBackend_Default(t *testing.T) {
	if Default().Awg.Backend != "" {
		t.Fatalf("default backend should be empty (resolved by mode at wiring), got %q", Default().Awg.Backend)
	}
}

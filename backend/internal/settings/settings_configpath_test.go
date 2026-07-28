package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Принять путь конфига из юнита — это лечение расхождения, а лечение обязано
// пережить перезапуск самой панели. Если путь не лёг в settings, при следующем
// старте резолв снова возьмёт старый singbox.config_path, и расхождение
// вернётся — то есть кнопка обещала бы больше, чем сделала.
func TestSetSingboxConfigPathPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routebox.toml")
	m := &Manager{settings: Default(), path: path}
	m.settings.Singbox.ConfigPath = "/etc/amnezia-box/config.json"

	if err := m.SetSingboxConfigPath("/opt/etc/sing-box/config.json"); err != nil {
		t.Fatalf("SetSingboxConfigPath: %v", err)
	}
	if got := m.Get().Singbox.ConfigPath; got != "/opt/etc/sing-box/config.json" {
		t.Errorf("in memory = %q, want the adopted path", got)
	}

	reloaded := &Manager{settings: Default(), path: path}
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reloaded.Get().Singbox.ConfigPath; got != "/opt/etc/sing-box/config.json" {
		t.Errorf("on disk = %q, want the adopted path — the fix must survive a restart", got)
	}
}

// Настройки могут быть недоступны на запись. Врать «закреплено» нельзя: путь в
// памяти уже сменился (RouteBox правит новый файл прямо сейчас), но об
// отсутствии записи вызывающий обязан узнать и сказать пользователю.
func TestSetSingboxConfigPathReportsUnwritableSettings(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	m := &Manager{settings: Default(), path: filepath.Join(dir, "routebox.toml")}
	err := m.SetSingboxConfigPath("/opt/etc/sing-box/config.json")
	if err == nil {
		t.Fatal("an unwritable settings file must be reported, not swallowed")
	}
	if !strings.Contains(err.Error(), "/opt/etc/sing-box/config.json") {
		t.Errorf("error %q should name the path that could not be persisted", err)
	}
	if got := m.Get().Singbox.ConfigPath; got != "/opt/etc/sing-box/config.json" {
		t.Errorf("in memory = %q: the path is in use either way, so memory must match reality", got)
	}
}

package process

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigMismatchRoundTrip(t *testing.T) {
	m := NewManager()
	if _, _, ok := m.ConfigMismatch(); ok {
		t.Fatal("fresh manager must not report a mismatch")
	}
	m.SetConfigPaths("/a.json", "/b.json")
	ours, unit, ok := m.ConfigMismatch()
	if !ok || ours != "/a.json" || unit != "/b.json" {
		t.Fatalf("got %q %q %v; want /a.json /b.json true", ours, unit, ok)
	}
	st := m.GetStatus()
	if !st.ConfigPaths.UnitMismatch || st.ConfigPaths.Ours != "/a.json" || st.ConfigPaths.Unit != "/b.json" {
		t.Fatalf("status must carry the mismatch, got %+v", st.ConfigPaths)
	}
}

func TestSetConfigPathsClears(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/a.json", "/b.json")

	m.SetConfigPaths("/a.json", "")
	if _, _, ok := m.ConfigMismatch(); ok {
		t.Fatal("empty unit path must clear the mismatch")
	}

	m.SetConfigPaths("/a.json", "/b.json")
	m.SetConfigPaths("/a.json", "/a.json")
	if _, _, ok := m.ConfigMismatch(); ok {
		t.Fatal("equal paths must clear the mismatch")
	}
}

func TestSetConfigPathsNormalizesPaths(t *testing.T) {
	// Один и тот же файл, записанный по-разному, расхождением не является:
	// ложное срабатывание блокирует Start/Restart/Reload на ровном месте.
	for _, tc := range []struct{ ours, unit string }{
		{"/etc/amnezia-box/config.json", "/etc/amnezia-box//config.json"},
		{"/etc/amnezia-box/./config.json", "/etc/amnezia-box/config.json"},
		{"/etc/amnezia-box/config.json", "/etc/amnezia-box/sub/../config.json"},
	} {
		m := NewManager()
		m.SetConfigPaths(tc.ours, tc.unit)
		if ours, unit, ok := m.ConfigMismatch(); ok {
			t.Errorf("%q vs %q: same file spelled differently must not be a mismatch, got %q vs %q", tc.ours, tc.unit, ours, unit)
		}
	}

	// А настоящее расхождение запоминается в канонической форме. Пути берём внутри
	// t.TempDir(), а не системные: сравнение разрешает симлинки, и на целевом
	// роутере (Entware/OpenWrt) "/etc/amnezia-box" и "/opt/etc/sing-box" вполне
	// могут оказаться одним файлом — тест упал бы там, где код прав.
	root := t.TempDir()
	m := NewManager()
	m.SetConfigPaths(root+"/ours//config.json", root+"/theirs/./config.json")
	ours, unit, ok := m.ConfigMismatch()
	if !ok || ours != filepath.Join(root, "ours", "config.json") || unit != filepath.Join(root, "theirs", "config.json") {
		t.Fatalf("got %q %q %v; want cleaned paths and true", ours, unit, ok)
	}
}

// Менеджер отрезан от машины: с NewManager() эти тесты держатся на самом
// checkConfigMismatch, который и проверяют, — при его регрессе Start запустил бы
// настоящий процесс, а Reload на роутере ушёл бы в
// `systemctl kill --signal=SIGHUP amnezia-box.service`. Проверяемый код не может
// быть страховкой проверяющего теста.
func TestStartRefusesOnMismatch(t *testing.T) {
	m := NewManagerForTest("", t.TempDir())
	m.SetConfigPaths("/a.json", "/b.json")
	err := m.Start("/a.json")
	if err == nil {
		t.Fatal("Start must refuse while the config paths disagree")
	}
	for _, want := range []string{"/a.json", "/b.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q, got %q", want, err)
		}
	}
}

func TestRestartAndReloadRefuseOnMismatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Manager) error
	}{
		{"Restart", func(m *Manager) error { return m.Restart("/a.json") }},
		{"Reload", func(m *Manager) error { return m.Reload() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManagerForTest("", t.TempDir())
			m.SetConfigPaths("/a.json", "/b.json")
			err := tc.call(m)
			if err == nil {
				t.Fatalf("%s must refuse while the config paths disagree", tc.name)
			}
			for _, want := range []string{"/a.json", "/b.json"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error must name %q, got %q", want, err)
				}
			}
		})
	}
}

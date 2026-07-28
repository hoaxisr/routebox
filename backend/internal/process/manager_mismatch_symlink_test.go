package process

import (
	"os"
	"path/filepath"
	"testing"
)

// На Entware/OpenWrt /opt/etc/sing-box — обычно симлинк, и один и тот же файл
// приходит из двух источников под двумя именами. Расхождение блокирует Start,
// Restart и Reload, поэтому ложное срабатывание здесь глушит панель на здоровой
// установке.
func TestSetConfigPathsResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	t.Run("symlinked dir, config present", func(t *testing.T) {
		cfg := filepath.Join(real, "config.json")
		if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(cfg) })

		m := NewManager()
		m.SetConfigPaths(cfg, filepath.Join(link, "config.json"))
		if ours, unit, ok := m.ConfigMismatch(); ok {
			t.Fatalf("same file behind a symlinked dir must not be a mismatch, got %q vs %q", ours, unit)
		}
	})

	t.Run("symlinked dir, config not created yet", func(t *testing.T) {
		// Свежая установка: каталог уже симлинк, файла ещё нет. Полный путь не
		// резолвится, но каталог — резолвится, и файл всё равно один.
		m := NewManager()
		m.SetConfigPaths(filepath.Join(real, "config.json"), filepath.Join(link, "config.json"))
		if ours, unit, ok := m.ConfigMismatch(); ok {
			t.Fatalf("missing config behind a symlinked dir must not be a mismatch, got %q vs %q", ours, unit)
		}
	})

	t.Run("symlinked file", func(t *testing.T) {
		target := filepath.Join(real, "config.json")
		if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(real, "alias.json")
		if err := os.Symlink(target, alias); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Cleanup(func() { os.Remove(target); os.Remove(alias) })

		m := NewManager()
		m.SetConfigPaths(target, alias)
		if ours, unit, ok := m.ConfigMismatch(); ok {
			t.Fatalf("symlinked config file must not be a mismatch, got %q vs %q", ours, unit)
		}
	})

	t.Run("mismatch is remembered as written, not as resolved", func(t *testing.T) {
		// Баннер обязан называть те пути, которые пользователь видит у себя (свой
		// конфиг и ExecStart юнита); резолв — только для сравнения.
		ourPath := filepath.Join(real, "ours.json")
		unitPath := filepath.Join(link, "theirs.json")
		for _, p := range []string{ourPath, filepath.Join(real, "theirs.json")} {
			if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(p) })
		}

		m := NewManager()
		m.SetConfigPaths(ourPath, unitPath)
		ours, unit, ok := m.ConfigMismatch()
		if !ok {
			t.Fatal("different files must stay a mismatch")
		}
		if ours != ourPath || unit != unitPath {
			t.Fatalf("got %q %q; want the paths as given: %q %q", ours, unit, ourPath, unitPath)
		}
	})
}

// Резолв не должен превращать «не смогли проверить» в «расхождения нет».
func TestSetConfigPathsKeepsMismatchWhenPathsUnresolvable(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name       string
		ours, unit string
	}{
		{
			name: "neither path exists",
			ours: filepath.Join(root, "gone-a", "config.json"),
			unit: filepath.Join(root, "gone-b", "config.json"),
		},
		{
			name: "one side exists",
			ours: root,
			unit: filepath.Join(root, "gone", "config.json"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager()
			m.SetConfigPaths(tc.ours, tc.unit)
			ours, unit, ok := m.ConfigMismatch()
			if !ok {
				t.Fatal("unresolvable distinct paths must stay a mismatch, not be silently cleared")
			}
			if ours != tc.ours || unit != tc.unit {
				t.Fatalf("got %q %q; want %q %q", ours, unit, tc.ours, tc.unit)
			}
		})
	}
}

// Битый симлинк — «файла нет», а не «путей нет»: сравнение не должно ни падать,
// ни молча объявлять разные файлы одним.
func TestSetConfigPathsBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	dangling := filepath.Join(root, "dangling.json")
	if err := os.Symlink(filepath.Join(root, "nowhere.json"), dangling); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	other := filepath.Join(root, "other.json")
	if err := os.WriteFile(other, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	m.SetConfigPaths(dangling, other)
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Fatal("a dangling symlink and a real file are different paths; mismatch must stand")
	}

	m.SetConfigPaths(dangling, dangling)
	if _, _, ok := m.ConfigMismatch(); ok {
		t.Fatal("one and the same dangling path is not a mismatch")
	}
}

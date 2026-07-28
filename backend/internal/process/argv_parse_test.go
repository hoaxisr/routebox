package process

import (
	"os"
	"path/filepath"
	"testing"
)

// Один разбор argv на два источника. Раньше их было два: разбор
// /proc/<pid>/cmdline понимал слитную форму -c/path, а разбор ExecStart — нет.
// Расходились они молча и в опасную сторону: на юните со слитной формой
// детектор расхождения не видел пути вовсе, а «пути нет» он трактует как
// «расхождения нет» — то есть вся сверка выключалась целиком.
func TestBothArgvSourcesAgree(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "joined short flag",
			argv: []string{"/usr/bin/box", "run", "-c/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "separate short flag",
			argv: []string{"/usr/bin/box", "run", "-c", "/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "inline short flag",
			argv: []string{"/usr/bin/box", "run", "-c=/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "long flag",
			argv: []string{"/usr/bin/box", "run", "--config", "/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "inline long flag",
			argv: []string{"/usr/bin/box", "run", "--config=/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "another flag merely ending in -c",
			argv: []string{"/usr/bin/box", "run", "--workdir-c", "/var/lib", "-c", "/etc/amnezia-box/config.json"},
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "no config flag",
			argv: []string{"/usr/bin/box", "run", "-D", "/var/lib/box"},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := configPathFromArgv(tc.argv); got != tc.want {
				t.Errorf("unit argv: got %q, want %q", got, tc.want)
			}
			if got := configPathFromProcessArgv(tc.argv, ""); got != tc.want {
				t.Errorf("process argv: got %q, want %q", got, tc.want)
			}
		})
	}
}

// Слитная форма каталога тоже общая: -C<dir> сохраняет режим каталога.
func TestJoinedConfigDirectoryFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	argv := []string{"/usr/bin/box", "run", "-C" + dir}
	want := filepath.Join(dir, "config.json")

	if got := configPathFromArgv(argv); got != want {
		t.Errorf("unit argv: got %q, want %q", got, want)
	}
	if got := configPathFromProcessArgv(argv, ""); got != want {
		t.Errorf("process argv: got %q, want %q", got, want)
	}
}

// То, что есть только у процесса: рабочий каталог, относительно которого
// разворачивается относительный путь, и -D как последняя догадка.
func TestConfigPathFromProcessArgvProcessOnlyExtras(t *testing.T) {
	t.Run("relative path resolved against the process cwd", func(t *testing.T) {
		argv := []string{"/usr/bin/box", "run", "-c", "etc/config.json"}
		if got, want := configPathFromProcessArgv(argv, "/opt/box"), "/opt/box/etc/config.json"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("-D working directory as a fallback", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		argv := []string{"/usr/bin/box", "run", "-D", dir}
		if got, want := configPathFromProcessArgv(argv, ""), filepath.Join(dir, "config.json"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("-D without a config.json in it stays unknown", func(t *testing.T) {
		argv := []string{"/usr/bin/box", "run", "-D", t.TempDir()}
		if got := configPathFromProcessArgv(argv, ""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

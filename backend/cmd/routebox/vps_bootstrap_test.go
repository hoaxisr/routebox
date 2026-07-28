package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"routebox/backend/internal/auth"
	"routebox/backend/internal/settings"
)

// printedPassword достаёт пароль из баннера: тест обязан проверять именно тот
// пароль, который увидит человек, а не тот, что вернула бы внутренняя функция.
var printedPassword = regexp.MustCompile(`(?m)^\s*password:\s*(\S+)\s*$`)

func newVPSSettings(t *testing.T, dir string) *settings.Manager {
	t.Helper()
	sm, err := settings.NewManager(filepath.Join(dir, "routebox.toml"))
	if err != nil {
		t.Fatalf("settings.NewManager: %v", err)
	}
	return sm
}

// Обычный VPS: пароль сгенерирован, закреплён в настройках и лежит в файле,
// который человек найдёт рядом с routebox.toml.
func TestBootstrapVPSAuthPersistsCredentials(t *testing.T) {
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)

	var out strings.Builder
	if err := bootstrapVPSAuth(sm, &out); err != nil {
		t.Fatalf("bootstrapVPSAuth: %v", err)
	}

	sec := sm.Get().Security
	if !sec.AuthEnabled || sec.AuthPasswordHash == "" {
		t.Fatalf("auth must be on with a hash, got %+v", sec)
	}

	m := printedPassword.FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("the banner must print the password, got:\n%s", out.String())
	}
	if !auth.VerifyPassword(sec.AuthPasswordHash, m[1]) {
		t.Error("the printed password does not open the panel")
	}

	pwFile := filepath.Join(dir, "routebox-initial-password")
	data, err := os.ReadFile(pwFile)
	if err != nil {
		t.Fatalf("password file: %v", err)
	}
	if strings.TrimSpace(string(data)) != m[1] {
		t.Errorf("password file holds %q, banner printed %q", strings.TrimSpace(string(data)), m[1])
	}
	fi, err := os.Stat(pwFile)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("password file mode %v, want 0600 — it is a secret", fi.Mode().Perm())
	}
	if !strings.Contains(out.String(), pwFile) {
		t.Errorf("the banner must name the file it wrote, got:\n%s", out.String())
	}

	if raw, err := os.ReadFile(sm.GetPath()); err != nil {
		t.Fatalf("settings file: %v", err)
	} else if !strings.Contains(string(raw), "auth_enabled = true") {
		t.Errorf("auth must be persisted, got:\n%s", raw)
	}
}

// Каталог настроек недоступен на запись. Падать на старте нельзя — панель
// полезна и в режиме только-чтения, — но и промолчать нельзя: пароль обязан
// остаться на виду, а обе неудачи — названы вместе со своими следствиями.
func TestBootstrapVPSAuthSurvivesAReadOnlySettingsDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	var out strings.Builder
	if err := bootstrapVPSAuth(sm, &out); err != nil {
		t.Fatalf("an unwritable settings dir must not abort startup: %v", err)
	}

	sec := sm.Get().Security
	if !sec.AuthEnabled || sec.AuthPasswordHash == "" {
		t.Fatalf("auth must still be ON in memory — the panel is public, got %+v", sec)
	}
	m := printedPassword.FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("the banner must print the password, got:\n%s", out.String())
	}
	if !auth.VerifyPassword(sec.AuthPasswordHash, m[1]) {
		t.Error("the printed password does not open the panel")
	}

	banner := out.String()
	for _, want := range []string{
		"read-only",  // классификация, а не сырой errno
		sm.GetPath(), // какой файл чинить
		filepath.Join(dir, "routebox-initial-password"), // и какой второй
		"copy", // что делать прямо сейчас
	} {
		if !strings.Contains(banner, want) {
			t.Errorf("banner must mention %q, got:\n%s", want, banner)
		}
	}
	if !strings.Contains(banner, "next start") {
		t.Errorf("banner must say the credentials are for this run only, got:\n%s", banner)
	}
	if strings.Contains(banner, "also written to") {
		t.Errorf("banner must not claim a file it failed to write, got:\n%s", banner)
	}
}

// Пароль уже настроен — bootstrap не трогает ничего и молчит. Иначе каждый
// старт печатал бы новый пароль и переписывал файл.
func TestBootstrapVPSAuthIsANoOpWhenAuthIsConfigured(t *testing.T) {
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)
	if err := sm.Update(map[string]interface{}{
		"security.auth_enabled":  true,
		"security.auth_username": "admin",
		"security.auth_password": "already-set",
	}); err != nil {
		t.Fatal(err)
	}
	hash := sm.Get().Security.AuthPasswordHash

	var out strings.Builder
	if err := bootstrapVPSAuth(sm, &out); err != nil {
		t.Fatalf("bootstrapVPSAuth: %v", err)
	}
	if got := sm.Get().Security.AuthPasswordHash; got != hash {
		t.Error("an existing password must not be replaced on start")
	}
	if out.Len() != 0 {
		t.Errorf("nothing to announce, got:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "routebox-initial-password")); !os.IsNotExist(err) {
		t.Errorf("no password file must be written, stat err = %v", err)
	}
}

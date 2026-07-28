package process

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Единое состояние «с каким конфигом мы работаем»: наш путь, путь из ExecStart
// юнита и путь живого процесса. Раньше эти три источника жили порознь (память
// менеджера, systemctl, /proc/<pid>/cmdline) и сравнивались в разных местах — с
// предсказуемым итогом: сравнение с процессом просто пропало из интерфейса.
func TestConfigPathsReportsAllThreeSources(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	got := m.configPaths("/tmp/hand-rolled.json")
	if got.Ours != "/etc/amnezia-box/config.json" {
		t.Errorf("Ours = %q, want /etc/amnezia-box/config.json", got.Ours)
	}
	if got.Unit != "/opt/etc/sing-box/config.json" {
		t.Errorf("Unit = %q, want /opt/etc/sing-box/config.json", got.Unit)
	}
	if got.Process != "/tmp/hand-rolled.json" {
		t.Errorf("Process = %q, want /tmp/hand-rolled.json", got.Process)
	}
	if !got.UnitMismatch || !got.ProcessMismatch {
		t.Errorf("both mismatches must be reported, got %+v", got)
	}
}

// Пустое поле означает «источника нет» (юнита нет / процесс не запущен), а не
// «совпадает»: расхождения с несуществующим источником не бывает.
func TestConfigPathsMissingSourceIsNotAMismatch(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "")

	got := m.configPaths("")
	if got.Unit != "" || got.Process != "" {
		t.Fatalf("absent sources must stay empty, got %+v", got)
	}
	if got.UnitMismatch || got.ProcessMismatch {
		t.Errorf("absent source must not be reported as a mismatch, got %+v", got)
	}
}

// Тот самый регресс: юнит согласен с нами, а процесс подняли руками с третьим
// конфигом. Раньше это не показывалось нигде — старый блок дашборда был
// подавлен наличием юнита, а баннер сравнивал только с юнитом.
func TestConfigPathsAgreeingUnitDoesNotHideProcessMismatch(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/etc/amnezia-box/config.json")

	got := m.configPaths("/tmp/other.json")
	if got.UnitMismatch {
		t.Errorf("unit agrees, so no unit mismatch; got %+v", got)
	}
	if !got.ProcessMismatch {
		t.Errorf("process runs a third config — that must be reported; got %+v", got)
	}
}

// Standalone-установка: юнита нет вовсе, и сравнение с живым процессом остаётся
// единственным. Оно обязано работать без юнита.
func TestConfigPathsStandaloneProcessMismatch(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "")

	got := m.configPaths("/tmp/other.json")
	if got.UnitMismatch {
		t.Errorf("no unit — nothing to disagree with; got %+v", got)
	}
	if !got.ProcessMismatch {
		t.Errorf("process runs another config — that must be reported; got %+v", got)
	}
}

// Тот же файл, записанный иначе (двойные слэши, симлинк каталога), расхождением
// не является — иначе баннер висел бы на здоровой установке.
func TestConfigPathsNormalizesProcessPath(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "")
	if got := m.configPaths("/etc/amnezia-box//config.json"); got.ProcessMismatch {
		t.Errorf("same file spelled differently must not be a mismatch, got %+v", got)
	}

	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cfg := filepath.Join(real, "config.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	m2 := NewManager()
	m2.SetConfigPaths(cfg, "")
	if got := m2.configPaths(filepath.Join(link, "config.json")); got.ProcessMismatch {
		t.Errorf("same file behind a symlinked dir must not be a mismatch, got %+v", got)
	}
}

// Расхождение с живым процессом лечится перезапуском, а перезапуск — это
// Start/Restart. Блокировать их по нему значило бы запретить единственное
// лечение, поэтому блокировка остаётся завязанной только на юнит.
func TestProcessMismatchDoesNotBlockLifecycle(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/etc/amnezia-box/config.json")
	if got := m.configPaths("/tmp/other.json"); !got.ProcessMismatch {
		t.Fatalf("precondition: process mismatch expected, got %+v", got)
	}
	for _, verb := range []string{"starting", "restarting", "reloading"} {
		if err := m.checkConfigMismatch(verb); err != nil {
			t.Errorf("%s must stay allowed with only a process mismatch: %v", verb, err)
		}
	}
}

// Переход на путь юнита меняет состояние только по УСПЕШНОМУ чтению, поэтому
// читатель обязан различать три исхода: юнита нет, юнит не прочитался, юнит не
// называет конфиг. Свалить их в одну пустую строку — значит объявить
// расхождение разрешённым, ничего не проверив.
func TestReadUnitConfigPathDistinguishesAbsentFromUnreadable(t *testing.T) {
	absent := &Manager{unitConfigReader: func() (string, error) { return "", ErrNoSystemdUnit }}
	if _, err := absent.ReadUnitConfigPath(); !errors.Is(err, ErrNoSystemdUnit) {
		t.Errorf("no unit must be reported as ErrNoSystemdUnit, got %v", err)
	}

	broken := &Manager{unitConfigReader: func() (string, error) {
		return "", errors.New("systemctl show: exit status 1")
	}}
	_, err := broken.ReadUnitConfigPath()
	if err == nil || errors.Is(err, ErrNoSystemdUnit) {
		t.Errorf("an unreadable unit must not look like an absent one, got %v", err)
	}

	silent := &Manager{unitConfigReader: func() (string, error) { return "", nil }}
	if path, err := silent.ReadUnitConfigPath(); path != "" || err != nil {
		t.Errorf("a unit naming no config is (%q, %v), want (\"\", nil)", path, err)
	}
}

// Статус несёт состояние одним объектом: панель опрашивает только его.
func TestGetStatusCarriesConfigPaths(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/a.json", "/b.json")
	st := m.GetStatus()
	if st.ConfigPaths.Ours != "/a.json" || st.ConfigPaths.Unit != "/b.json" || !st.ConfigPaths.UnitMismatch {
		t.Fatalf("status must carry the config paths, got %+v", st.ConfigPaths)
	}
}

// Плоское config_path отдавалось статусом до объединения трёх источников и
// означало ровно одно — путь конфига ЖИВОГО процесса. Версионирования API нет,
// панель и бэкенд едут одним бинарём, поэтому у внешнего скрипта поле исчезло бы
// молча. Алиас стоит одного поля и обязан держаться за Process, а не жить своей
// жизнью.
func TestStatusKeepsDeprecatedConfigPathAlias(t *testing.T) {
	st := Status{
		Running:     true,
		ConfigPaths: ConfigPaths{Ours: "/a.json", Unit: "/a.json", Process: "/c.json"},
	}.withLegacyConfigPath()

	if st.ConfigPath != "/c.json" {
		t.Errorf("ConfigPath = %q, want the live process path /c.json", st.ConfigPath)
	}

	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["config_path"] != "/c.json" {
		t.Errorf("config_path in JSON = %v, want /c.json", out["config_path"])
	}
}

// «Источника нет» — это отсутствие поля, а не пустая строка: так поле вело себя
// и раньше (omitempty при незапущенном процессе), и панель, и скрипты вправе
// рассчитывать на прежнюю форму ответа.
func TestStatusOmitsConfigPathWithoutARunningProcess(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/a.json", "/a.json")
	m.pidFinder = func() int { return 0 }

	raw, err := json.Marshal(m.GetStatus())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["config_path"]; ok {
		t.Errorf("nothing is running — config_path must be absent, got %v", out["config_path"])
	}
}

// SetConfigPath (наш путь без пересверки юнита) обновляет именно Ours: иначе
// после смены пути состояние показывало бы устаревшую правду.
func TestSetConfigPathUpdatesOurs(t *testing.T) {
	m := NewManager()
	m.SetConfigPaths("/a.json", "/b.json")
	m.SetConfigPath("/c.json")
	if got := m.configPaths(""); got.Ours != "/c.json" || got.Unit != "/b.json" {
		t.Fatalf("got %+v; want Ours=/c.json Unit=/b.json", got)
	}
}

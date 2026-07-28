package process

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Тесты жизненного цикла drop-in: его видно в состоянии, его можно снять, и обе
// операции честно рассказывают о частичном применении. В /etc никто не пишет и
// systemctl не зовёт — всё через швы systemdRoot/daemonReload/unitExecStartReader.

const testExecStart = "ExecStart={ path=/usr/local/bin/amnezia-box ; argv[]=/usr/local/bin/amnezia-box run -c /opt/etc/sing-box/config.json ; ignore_errors=no }"

// newDropInManager собирает менеджер, у которого systemd целиком заменён швами:
// каталог юнитов — во временной папке, daemon-reload и чтение ExecStart — функции.
func newDropInManager(t *testing.T, execStart string) *Manager {
	t.Helper()
	return &Manager{
		serviceName:         "amnezia-box",
		systemdRoot:         t.TempDir(),
		unitExecStartReader: func() (string, error) { return execStart, nil },
		daemonReload:        func() error { return nil },
	}
}

// Установленный drop-in обязан быть виден в состоянии: иначе через месяц никто
// не вспомнит, откуда в юните взялся override, и снять его будет нечем.
func TestConfigPathsReportsInstalledDropIn(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	// Юнит после reload сообщает уже наш путь — так выглядит удавшаяся починка.
	m.unitConfigReader = func() (string, error) { return "/etc/amnezia-box/config.json", nil }
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	if got := m.configPaths("").DropIn; got != nil {
		t.Fatalf("no file on disk — no drop-in must be reported, got %+v", got)
	}

	if err := m.WriteConfigDropIn("/etc/amnezia-box/config.json"); err != nil {
		t.Fatalf("write drop-in: %v", err)
	}

	di := m.configPaths("").DropIn
	if di == nil {
		t.Fatal("a written drop-in must show up in the status")
	}
	if di.Path != m.dropInPath() {
		t.Errorf("Path = %q, want %q", di.Path, m.dropInPath())
	}
	if di.ConfigPath != "/etc/amnezia-box/config.json" {
		t.Errorf("ConfigPath = %q, want the path the unit is retargeted at", di.ConfigPath)
	}
	if di.PendingReload {
		t.Error("the unit already reports our path: nothing is pending")
	}
}

// Файл на диске, а юнит его ещё не подхватил — это отдельное состояние: молчать
// о нём значит показывать «починено» там, где ничего не применилось.
func TestConfigPathsFlagsDropInWaitingForReload(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")

	di := m.configPaths("").DropIn
	if di == nil {
		t.Fatal("the file is on disk, it must be reported")
	}
	if !di.PendingReload {
		t.Error("the unit still starts with the old path: the drop-in is pending a reload")
	}
}

// Читать содержимое drop-in могло не выйти — но сам факт «файл лежит» важнее
// разбора: именно он объясняет, откуда override.
func TestConfigDropInReportedEvenWhenContentSaysNothing(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	writeTestDropIn(t, m, "")

	di := m.configPaths("").DropIn
	if di == nil {
		t.Fatal("an unparsable drop-in is still an installed drop-in")
	}
	if di.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty: nothing to parse", di.ConfigPath)
	}
	if di.PendingReload {
		t.Error("unknown target cannot be declared pending")
	}
}

// Если daemon-reload упал, файл всё равно уже на диске и подхватится при любом
// следующем reload. Пользователь обязан узнать об этом из самой ошибки.
func TestWriteConfigDropInSaysFileIsOnDiskWhenReloadFails(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.daemonReload = func() error { return errors.New("Failed to reload daemon: Access denied") }
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	err := m.WriteConfigDropIn("/etc/amnezia-box/config.json")
	if err == nil {
		t.Fatal("a failed daemon-reload is not a successful fix")
	}
	msg := err.Error()
	for _, want := range []string{m.dropInPath(), "Access denied"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q must mention %q", msg, want)
		}
	}
	if !strings.Contains(msg, "next") && !strings.Contains(msg, "still") {
		t.Errorf("message %q must warn that the file stays and will be picked up later", msg)
	}
	if _, err := os.Stat(m.dropInPath()); err != nil {
		t.Fatalf("the file is expected to still be there (that is the whole point): %v", err)
	}
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Error("nothing was applied: the mismatch must stay")
	}
}

// Снятие: файл уходит, systemd перечитывается, а состояние пересверяется по
// факту юнита — вернувшееся расхождение должно быть видно сразу, без перезапуска
// панели.
func TestRemoveConfigDropInDeletesFileAndRereadsUnit(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.unitConfigReader = func() (string, error) { return "/opt/etc/sing-box/config.json", nil }
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/etc/amnezia-box/config.json")
	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")

	reloaded := false
	m.daemonReload = func() error { reloaded = true; return nil }

	res, err := m.RemoveConfigDropIn()
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !reloaded {
		t.Error("systemd must be told to forget the drop-in")
	}
	if res.Warning != "" {
		t.Errorf("a clean removal has nothing to warn about, got %q", res.Warning)
	}
	if res.UnitConfigPath != "/opt/etc/sing-box/config.json" {
		t.Errorf("UnitConfigPath = %q, want the path the unit fell back to", res.UnitConfigPath)
	}
	if _, err := os.Stat(m.dropInPath()); !os.IsNotExist(err) {
		t.Errorf("the drop-in file must be gone, stat err = %v", err)
	}
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Error("the unit is back on its own path: the mismatch must be reported again")
	}
	if m.configPaths("").DropIn != nil {
		t.Error("a removed drop-in must not linger in the status")
	}
}

// Каталог <unit>.service.d наш только если в нём больше ничего нет: чужие
// drop-in'ы удалять нельзя, и каталог с ними обязан пережить снятие.
func TestRemoveConfigDropInKeepsForeignDropIns(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.unitConfigReader = func() (string, error) { return "/opt/etc/sing-box/config.json", nil }
	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")

	foreign := filepath.Join(filepath.Dir(m.dropInPath()), "20-hand-rolled.conf")
	if err := os.WriteFile(foreign, []byte("[Service]\nRestart=always\n"), 0o644); err != nil {
		t.Fatalf("seed foreign drop-in: %v", err)
	}

	if _, err := m.RemoveConfigDropIn(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("somebody else's drop-in must survive: %v", err)
	}
}

// Снятия без drop-in не бывает: отвечать надо «нечего снимать», а не молча
// делать вид, что что-то произошло.
func TestRemoveConfigDropInWithoutFile(t *testing.T) {
	m := newDropInManager(t, testExecStart)

	if _, err := m.RemoveConfigDropIn(); !errors.Is(err, ErrNoConfigDropIn) {
		t.Fatalf("err = %v, want ErrNoConfigDropIn", err)
	}
}

func TestRemoveConfigDropInWithoutUnit(t *testing.T) {
	// Как и у записи: «юнита нет» проверяет сам код, поэтому корень временный —
	// регресс в dropInPath иначе превратил бы это в os.Remove внутри /etc.
	m := NewManagerForTest("", t.TempDir())
	if _, err := m.RemoveConfigDropIn(); !errors.Is(err, ErrNoConfigDropIn) {
		t.Fatalf("err = %v, want ErrNoConfigDropIn: no unit means no drop-in of ours", err)
	}
}

// Файл удалён, а daemon-reload упал: юнит продолжает работать с override до
// следующего успешного reload. Это не провал снятия (файла уже нет), но и не
// чистый успех — поэтому предупреждение, а не ошибка.
func TestRemoveConfigDropInWarnsWhenReloadFails(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.unitConfigReader = func() (string, error) { return "/opt/etc/sing-box/config.json", nil }
	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")
	m.daemonReload = func() error { return errors.New("Access denied") }

	res, err := m.RemoveConfigDropIn()
	if err != nil {
		t.Fatalf("the file is deleted, that is not a failure: %v", err)
	}
	if _, err := os.Stat(m.dropInPath()); !os.IsNotExist(err) {
		t.Fatalf("the file must be gone, stat err = %v", err)
	}
	if res.Warning == "" {
		t.Fatal("a unit still carrying the override must be reported")
	}
	if !strings.Contains(res.Warning, "Access denied") {
		t.Errorf("warning %q must carry why the reload failed", res.Warning)
	}
	if !strings.Contains(res.Warning, "daemon-reload") {
		t.Errorf("warning %q must name what has to succeed for the removal to take effect", res.Warning)
	}
}

// Не смогли перечитать юнит после снятия — знать, куда он вернулся, неоткуда.
// Придумывать путь нельзя: снятие состоялось, а проверка — нет.
func TestRemoveConfigDropInWarnsWhenUnitCannotBeReread(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.unitConfigReader = func() (string, error) { return "", errors.New("systemctl show: exit status 1") }
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/etc/amnezia-box/config.json")
	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")

	res, err := m.RemoveConfigDropIn()
	if err != nil {
		t.Fatalf("the removal itself went through: %v", err)
	}
	if res.UnitConfigPath != "" {
		t.Errorf("UnitConfigPath = %q, want empty: the unit was not read", res.UnitConfigPath)
	}
	if !strings.Contains(res.Warning, "exit status 1") {
		t.Errorf("warning %q must carry why the unit could not be re-read", res.Warning)
	}
}

// Менеджер для тестов обязан быть отрезан от машины ЦЕЛИКОМ, а не только от
// /etc: с найденным живым amnezia-box и пустым serviceName расхождения путей нет,
// и хендлер, дошедший до Reload, отправил бы SIGHUP боевому процессу. Поиск PID
// — такой же шов, как daemon-reload.
func TestFindPIDHonoursTheSeam(t *testing.T) {
	m := &Manager{pidFinder: func() int { return 4242 }}
	if got := m.findPID(); got != 4242 {
		t.Fatalf("findPID() = %d, want the seam's answer 4242", got)
	}
}

func TestNewManagerForTestFindsNoProcessAndNoUnit(t *testing.T) {
	root := t.TempDir()
	m := NewManagerForTest("amnezia-box", root)
	if got := m.findPID(); got != 0 {
		t.Errorf("findPID() = %d, want 0: a test manager must not adopt this machine's process", got)
	}
	if m.GetStatus().Running {
		t.Error("a test manager must never report the machine's amnezia-box as running")
	}
	// Сверяемся с ПЕРЕДАННЫМ корнем: сравнение пути с его же каталогом было бы
	// тавтологией и пропустило бы drop-in, уехавший куда угодно за пределы root.
	if !strings.HasPrefix(m.dropInPath(), root+string(filepath.Separator)) {
		t.Errorf("dropInPath() = %q, want it inside the temp root %q", m.dropInPath(), root)
	}
}

// writeTestDropIn кладёт drop-in в подменённый каталог юнитов. argv пустой —
// файл без разбираемой команды.
func writeTestDropIn(t *testing.T, m *Manager, argv string) {
	t.Helper()
	path := m.dropInPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "[Service]\nExecStart=\n"
	if argv != "" {
		content += "ExecStart=" + argv + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// Файл drop-in могли снести руками (`rm`), а «Снять» в панели нажать после —
// ответ «нечего снимать» верен, но каталог <unit>.service.d создавал RouteBox,
// и убрать его больше некому: он остаётся в /etc/systemd/system навсегда.
// Пустой каталог не меняет поведения юнита, но объяснять его через полгода
// придётся, а сделать это будет нечем.
func TestRemoveConfigDropInTakesTheEmptyDirWhenTheFileIsAlreadyGone(t *testing.T) {
	m := newDropInManager(t, testExecStart)
	m.unitConfigReader = func() (string, error) { return "/opt/etc/sing-box/config.json", nil }
	writeTestDropIn(t, m, "/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json")

	// Ровно сценарий B.3 живой проверки: файл убрали мимо панели.
	if err := os.Remove(m.dropInPath()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := m.RemoveConfigDropIn(); !errors.Is(err, ErrNoConfigDropIn) {
		t.Fatalf("err = %v, want ErrNoConfigDropIn: снимать действительно нечего", err)
	}
	if _, err := os.Stat(filepath.Dir(m.dropInPath())); !os.IsNotExist(err) {
		t.Errorf("пустой каталог, созданный RouteBox, должен уйти вместе с drop-in, stat err = %v", err)
	}
}

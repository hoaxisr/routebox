package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Status represents the amnezia-box process status
type Status struct {
	Running      bool          `json:"running"`
	PID          int           `json:"pid,omitempty"`
	Uptime       string        `json:"uptime,omitempty"`
	ManagedBy    string        `json:"managed_by,omitempty"`    // "systemd", "standalone", or ""
	ServiceName  string        `json:"service_name,omitempty"`  // systemd unit RouteBox detected for amnezia-box (independent of who started the running process — that is ManagedBy)
	SupportsHUP  bool          `json:"supports_hup"`            // whether SIGHUP reload is supported
	Version      string        `json:"version,omitempty"`       // binary version string
	BinaryPath   string        `json:"binary_path,omitempty"`   // path to binary
	SystemChecks *SystemChecks `json:"system_checks,omitempty"` // system requirements check

	ConfigPaths ConfigPaths `json:"config_paths"` // с каким конфигом работаем: мы, юнит, живой процесс

	// ConfigPath — DEPRECATED-алиас ConfigPaths.Process, то есть путь конфига
	// живого процесса, ровно с тем же значением и той же формой (пусто =
	// процесса нет = поля в ответе нет), что до объединения трёх источников в
	// ConfigPaths.
	//
	// Держится не ради панели — она читает config_paths, — а ради чужих
	// скриптов: версионирования у API нет, панель и бэкенд едут одним бинарём,
	// и молча исчезнувшее поле сломало бы их без единого сообщения. Ровно то
	// поведение, против которого написана вся остальная работа этой волны.
	//
	// Заполняется в одном месте (withLegacyConfigPath), чтобы алиас не мог
	// разойтись с оригиналом. Снять — отдельным мажорным шагом и объявлением в
	// CHANGELOG, а не заодно с рефакторингом.
	ConfigPath string `json:"config_path,omitempty"`
}

// withLegacyConfigPath проставляет DEPRECATED-алиас config_path. Единственная
// точка, где он берётся, — и берётся он из состояния, а не из отдельного
// источника: алиас, который вычисляют дважды, однажды разойдётся.
func (s Status) withLegacyConfigPath() Status {
	s.ConfigPath = s.ConfigPaths.Process
	return s
}

// ConfigPaths — единое состояние «с каким конфигом мы работаем». Правда об этом
// приходит из трёх источников: путь, которым управляет RouteBox (Ours), путь из
// ExecStart systemd-юнита (Unit) и путь командной строки живого процесса
// (Process). Пока они собирались порознь и сравнивались в разных местах,
// сравнение с процессом успело пропасть из интерфейса целиком — поэтому теперь
// это один объект, отдаваемый одним полем статуса.
//
// Пустое поле означает «источника нет» (юнита нет, процесс не запущен), а не
// «совпадает»: расхождения с несуществующим источником не бывает, и оба флага
// при пустом источнике всегда false.
//
// Лечится расхождение по-разному, и в этом смысл разделения флагов:
// UnitMismatch — правки RouteBox уходят в файл, который юнит процессу не даёт;
// лечится сменой нашего пути или drop-in'ом на юнит, и до тех пор
// Start/Restart/Reload заблокированы. ProcessMismatch — юнит уже согласован, но
// живой процесс читает тот файл, с которым его запустили; лечится ТОЛЬКО
// перезапуском, поэтому он ничего не блокирует (блокировать перезапуск значило
// бы запретить единственное лечение).
type ConfigPaths struct {
	// Ours сериализуется всегда (без omitempty): путь, который правит RouteBox,
	// есть при любом раскладе — в отличие от остальных двух источников, — и
	// панель вправе рассчитывать на его присутствие, печатая его в тексте.
	Ours    string `json:"ours"`
	Unit    string `json:"unit,omitempty"`
	Process string `json:"process,omitempty"`

	UnitMismatch    bool `json:"unit_mismatch"`
	ProcessMismatch bool `json:"process_mismatch"`

	// DropIn — наш systemd drop-in, если он лежит на диске. Присутствие
	// выводится из файла при каждом опросе статуса, а не запоминается после
	// удачной починки: после перезапуска панели память бы обнулилась, а файл —
	// нет, и override в юните остался бы без объяснения. Nil — файла нет.
	DropIn *ConfigDropIn `json:"drop_in,omitempty"`
}

// ConfigDropIn описывает установленный RouteBox'ом drop-in: единственное, что
// RouteBox пишет за пределами своих файлов, — значит единственное, что панель
// обязана уметь показать и снять.
type ConfigDropIn struct {
	// Path — файл на диске. Называется прямо: снять drop-in руками (rm +
	// daemon-reload) должно быть возможно, не зная внутренностей RouteBox.
	Path string `json:"path"`
	// ConfigPath — путь конфига, на который drop-in перенацеливает ExecStart
	// ("" — файл есть, но разобрать его не вышло; факт наличия важнее разбора).
	ConfigPath string `json:"config_path,omitempty"`
	// PendingReload — файл записан, но юнит его ещё не подхватил: ровно то
	// состояние, в котором остаётся упавший daemon-reload. Оно применится при
	// следующем reload или перезагрузке, поэтому молчать о нём нельзя.
	PendingReload bool `json:"pending_reload"`
}

// SystemChecks contains system requirement validation results
type SystemChecks struct {
	IPv4Forward     bool `json:"ipv4_forward"`      // net.ipv4.ip_forward=1
	IPv6Forward     bool `json:"ipv6_forward"`      // net.ipv6.conf.all.forwarding=1
	IPv6Disabled    bool `json:"ipv6_disabled"`     // net.ipv6.conf.all.disable_ipv6=1 (IPv6 completely off)
	IsRoot          bool `json:"is_root"`           // running as root (required for TUN)
	AllChecksPassed bool `json:"all_checks_passed"` // true if all critical checks pass
}

// Manager handles amnezia-box process lifecycle
type Manager struct {
	opMu    sync.Mutex   // serializes Start/Stop/Restart/Reload
	stateMu sync.RWMutex // guards fields below

	binaryPath   string
	binaryPinned bool   // binaryPath came from configuration (--binary / singbox.binary_path), not detection
	configPath   string // путь конфига, которым управляет RouteBox (Ours в ConfigPaths)
	unitPath     string // путь конфига из ExecStart юнита, как его прочли последний раз ("" — юнита нет)
	serviceName  string // detected systemd service name (set once in NewManager)
	startedPID   int    // PID of process started by us in standalone mode (0 if none)

	cachedVersion      string    // memoized parsed version (e.g. "1.13.13")
	cachedVersionPath  string    // binary path the cached version belongs to
	cachedVersionStamp time.Time // binary mtime at cache fill
	cachedVersionSize  int64     // binary size at cache fill

	// unitConfigReader yields the config path the systemd unit currently
	// starts amnezia-box with. Defaults to UnitConfigPath when nil; overridable
	// in tests, where shelling out to systemctl is not available.
	unitConfigReader func() (string, error)

	// Швы для работы с systemd-юнитом. Пустые/nil в бою — тогда работают
	// настоящие /etc/systemd/system, `systemctl daemon-reload` и
	// `systemctl show`. В тестах подменяются: drop-in — единственное, что
	// RouteBox пишет вне своих файлов, и проверять это в настоящем /etc нельзя.
	systemdRoot         string       // каталог юнитов ("" — defaultSystemdRoot)
	daemonReload        func() error // systemctl daemon-reload
	unitExecStartReader func() (string, error)
	// pidFinder подменяет поиск живого процесса. Тому же ряду принадлежит:
	// менеджер, отрезанный от systemd, но всё ещё находящий в /proc боевой
	// amnezia-box, отдаёт Running=true — и хендлер из теста доходит до Reload,
	// то есть до SIGHUP настоящему процессу.
	pidFinder func() int

	cachedVersionFull      string    // memoized full `<binary> version` output (incl. Tags: line)
	cachedVersionFullPath  string    // binary path the cached full output belongs to
	cachedVersionFullStamp time.Time // binary mtime at full-output cache fill
	cachedVersionFullSize  int64     // binary size at full-output cache fill
}

func (m *Manager) getBinaryPath() string {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.binaryPath
}

func (m *Manager) setBinaryPath(p string) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.binaryPath = p
	// Invalidate the version cache unless it already belongs to this path
	// (GetVersion caches via runVersion(path) right before calling us).
	if m.cachedVersionPath != p {
		m.cachedVersion = ""
		m.cachedVersionPath = ""
	}
	if m.cachedVersionFullPath != p {
		m.cachedVersionFull = ""
		m.cachedVersionFullPath = ""
	}
}

func (m *Manager) getStartedPID() int {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.startedPID
}

func (m *Manager) setStartedPID(pid int) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.startedPID = pid
}

// clearStartedPIDIf zeroes startedPID only if it still equals old (CAS).
// Prevents a stale-detection in findPID from wiping a PID that a concurrent
// Start has just recorded.
func (m *Manager) clearStartedPIDIf(old int) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.startedPID == old {
		m.startedPID = 0
	}
}

// NewManager creates a new process manager
func NewManager() *Manager {
	m := &Manager{
		binaryPath: findBinary(),
		configPath: "",
	}
	// Try to detect systemd service
	m.serviceName = m.detectSystemdService()
	// When systemd-managed, prefer the exact binary the unit's ExecStart
	// launches over the findBinary() guess. Otherwise the updater could patch a
	// stale copy at a hardcoded path while systemd keeps running the old one
	// (Dashboard and Updates page then disagree on the version). Falls back to
	// the findBinary() result if ExecStart can't be resolved.
	if m.serviceName != "" {
		if p := m.getBinaryFromSystemd(); p != "" {
			m.binaryPath = p
		}
	}
	return m
}

// PinBinaryPath overrides the detected amnezia-box binary with an explicitly
// configured one (settings singbox.binary_path / --binary). It reports the
// path it replaced when that differs, so the caller can say so: an operator
// who pins a path while a systemd unit execs another one gets a panel that
// updates one binary and reports the version of the other, and the only place
// that discrepancy is visible is right here. No-op for an empty path.
func (m *Manager) PinBinaryPath(path string) (replaced string) {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	previous := m.getBinaryPath()
	m.setBinaryPath(path)
	m.stateMu.Lock()
	m.binaryPinned = true
	m.stateMu.Unlock()
	if previous == path {
		return ""
	}
	return previous
}

// binaryIsPinned reports whether the binary came from configuration rather than
// from detection.
func (m *Manager) binaryIsPinned() bool {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.binaryPinned
}

// SetConfigPath запоминает путь конфига, которым управляет RouteBox, не трогая
// прочитанный путь юнита.
func (m *Manager) SetConfigPath(path string) {
	if path != "" {
		path = filepath.Clean(path)
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.configPath = path
}

// SetConfigPaths запоминает наш путь конфига и путь, прочитанный из ExecStart
// юнита ("" — юнита нет либо он не называет конфиг). Расхождение из них не
// хранится, а выводится: два поля — один источник правды, и «забыть снять
// расхождение» становится нечем.
//
// Пути запоминаются в канонической форме (filepath.Clean): юнит вполне может
// нести "/etc/amnezia-box//config.json", и посимвольное сравнение объявило бы
// расхождение там, где файл один и тот же — а ложное срабатывание глушит
// Start/Restart/Reload.
//
// Сравниваются же пути с разрешёнными симлинками (resolveConfigPath): на
// Entware/OpenWrt "/opt/etc/sing-box" — типичный симлинк, и тот же самый файл,
// пришедший из ExecStart под вторым именем, иначе выглядел бы расхождением.
// Запоминаем при этом именно то, что передали: баннер обязан называть пути так,
// как их видит пользователь — в своей настройке и в ExecStart юнита, — а не
// разрешённые до неузнаваемости.
func (m *Manager) SetConfigPaths(ours, unit string) {
	if ours != "" {
		ours = filepath.Clean(ours)
	}
	if unit != "" {
		unit = filepath.Clean(unit)
	}

	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.configPath = ours
	m.unitPath = unit
}

// samePathOrAbsent сообщает, что расхождения между нашим путём и путём из
// источника нет: либо источника нет вовсе (пустая строка), либо это тот же
// файл. Пустой источник — «источника нет», а не «совпадает», но в обоих случаях
// показывать пользователю нечего.
func samePathOrAbsent(ours, other string) bool {
	return other == "" || resolveConfigPath(ours) == resolveConfigPath(other)
}

// resolveConfigPath приводит путь конфига к виду, пригодному для сравнения:
// абсолютный и с разрешёнными симлинками (как exeMatches делает для бинаря).
//
// Резолв может не удаться, и это штатно: свежая установка (файла ещё нет),
// битый симлинк, нет прав на промежуточный каталог. Во всех таких случаях
// откатываемся к менее разрешённой форме — вплоть до просто канонизированного
// пути. Именно к откату, а не к «расхождения нет»: молчаливое снятие
// расхождения по неудавшейся проверке — то самое враньё, ради вычистки
// которого расхождение и заводилось. Ошибка резолва делает сравнение строже
// (пути должны совпасть текстуально), но никогда — мягче.
//
// Промежуточная ступень — резолв каталога — нужна ровно для свежей установки:
// симлинком там обычно является каталог ("/opt/etc/sing-box" → "/etc/sing-box"),
// а файла конфига может ещё не быть, и без этой ступени RouteBox встретил бы
// пользователя ложным расхождением на первом же запуске.
//
// Функция детерминирована по строке: одинаковые на входе пути дают одинаковый
// результат, так что резолв не способен породить расхождение там, где его нет.
func resolveConfigPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	// Файла нет или он битый симлинк — пробуем хотя бы каталог.
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(dir, filepath.Base(abs))
	}
	return abs
}

// ConfigMismatch выводит расхождение с ЮНИТОМ из запомненных путей. ok==false,
// когда юнита нет или он называет тот же файл.
func (m *Manager) ConfigMismatch() (ours, unit string, ok bool) {
	m.stateMu.RLock()
	ours, unit = m.configPath, m.unitPath
	m.stateMu.RUnlock()

	if samePathOrAbsent(ours, unit) {
		return "", "", false
	}
	return ours, unit, true
}

// configPaths собирает состояние целиком. processPath передаётся снаружи, а не
// читается здесь: GetStatus уже знает его из /proc, а второе чтение стоило бы
// лишнего похода в findPID на каждый опрос статуса. Пустая строка — процесс не
// запущен (либо его командная строка не называет конфиг).
func (m *Manager) configPaths(processPath string) ConfigPaths {
	if processPath != "" {
		processPath = filepath.Clean(processPath)
	}

	m.stateMu.RLock()
	ours, unit := m.configPath, m.unitPath
	m.stateMu.RUnlock()

	return ConfigPaths{
		Ours:            ours,
		Unit:            unit,
		Process:         processPath,
		UnitMismatch:    !samePathOrAbsent(ours, unit),
		ProcessMismatch: !samePathOrAbsent(ours, processPath),
		DropIn:          m.readConfigDropIn(unit),
	}
}

// checkConfigMismatch возвращает ошибку, если правки RouteBox уходят не в тот
// файл, который systemd-юнит скармливает amnezia-box. verb — глагол операции
// ("starting", "restarting", "reloading").
//
// Сверяется только юнит. Расхождение с живым процессом здесь намеренно не
// участвует: оно лечится перезапуском, а перезапуск идёт через эти же
// Start/Restart — блокировка запретила бы единственное лечение.
func (m *Manager) checkConfigMismatch(verb string) error {
	if ours, unit, ok := m.ConfigMismatch(); ok {
		return fmt.Errorf("%w: RouteBox edits %s, but the systemd unit starts amnezia-box with %s — resolve it in the panel banner before %s", ErrConfigPathMismatch, ours, unit, verb)
	}
	return nil
}

// ErrConfigPathMismatch — расхождение путей, из-за которого Start/Restart/Reload
// отказывают. Отдельная ошибка ровно затем же, зачем ErrNoConfigDropIn: это
// СОСТОЯНИЕ, а не поломка, и API обязан ответить 409, а не 500. Запрос был
// корректен, разрешить его мешает состояние, и лечится оно из панели — клиент,
// которому пришёл 500, отличить это от «сервер упал» не может.
//
// Текст ошибки начинается ровно с "config path mismatch: " — как и раньше:
// сообщение уходит в UI как есть.
var ErrConfigPathMismatch = errors.New("config path mismatch")

// UnitConfigPath возвращает путь конфига из ExecStart systemd-юнита ("" — юнита нет).
func (m *Manager) UnitConfigPath() string {
	return m.getConfigFromSystemd()
}

// GetDetectedConfigPath returns the config path from running process or systemd
func (m *Manager) GetDetectedConfigPath() string {
	// First, try to get from running process
	if configPath := m.getConfigFromProcess(); configPath != "" {
		return configPath
	}

	// Try to get from systemd service
	if m.serviceName != "" {
		if configPath := m.getConfigFromSystemd(); configPath != "" {
			return configPath
		}
	}

	return ""
}

// serviceCandidates are the unit names probed at startup, in order of preference.
// amnezia-box comes FIRST on purpose: it is the unit both installers create, while
// a host that once ran upstream sing-box keeps an enabled sing-box.service forever
// (the installer never disables it). Probing sing-box first made RouteBox adopt a
// unit it does not manage, and every decision downstream is then made about the
// wrong service: the config-path check compares our config with a foreign
// ExecStart (false mismatch, Start/Restart/Reload blocked), the "point the unit at
// our config" fix writes a drop-in into somebody else's unit, and "adopt the
// detected path" moves RouteBox onto somebody else's config.
var serviceCandidates = []string{
	"amnezia-box",
	"sing-box",
	"sing-box@config",
}

// pickServiceName returns the first candidate unit the probe reports as present.
func pickServiceName(present func(name string) bool) string {
	for _, name := range serviceCandidates {
		if present(name) {
			return name
		}
	}
	return ""
}

// systemctl runs `systemctl <args...>` and returns its stdout. A package
// variable so unit detection can be tested without a real systemd.
var systemctl = func(args ...string) ([]byte, error) {
	return exec.Command("systemctl", args...).Output()
}

// systemdUnitPresent reports whether <name>.service exists on this host.
//
// The authority is `systemctl list-unit-files`: it sees a unit file that is
// installed but disabled AND stopped, which is-enabled and is-active both miss.
// Missing it is exactly how an installed-but-idle amnezia-box.service lost to a
// leftover running sing-box.service — and every decision downstream (the config
// path check, the drop-in fix) was then made about a unit RouteBox does not
// manage.
//
// is-enabled/is-active stay as a fallback rather than being replaced: a template
// instance (sing-box@config.service) has no unit file of its own — only the
// template sing-box@.service does — so list-unit-files cannot see it, while an
// enabled or running instance is undeniably present.
func systemdUnitPresent(name string) bool {
	unit := name + ".service"
	if out, err := systemctl("list-unit-files", "--no-legend", "--no-pager", unit); err == nil && unitFileListed(string(out), unit) {
		return true
	}
	if _, err := systemctl("is-enabled", unit); err == nil {
		return true
	}
	_, err := systemctl("is-active", unit)
	return err == nil
}

// unstartableUnitFileStates are the states in which a unit file exists but the
// unit cannot be started at all: `systemctl start` on a masked unit always
// fails, and `bad` is systemd's word for a unit file it could not make sense of.
// Adopting such a unit would hand the user a service RouteBox picked for them
// and cannot start — worse than picking the next candidate, which at least runs.
//
// The list is derived from the full set systemd itself reports (`systemctl
// --state=help`, "Available unit file states"): enabled, enabled-runtime,
// linked, linked-runtime, alias, masked, masked-runtime, static, disabled,
// indirect, generated, transient, bad. Everything outside the three below
// describes a unit that starts, so nothing else is filtered — an alias, for
// instance, is a real name of a real startable unit. `not-found` is a unit LOAD
// state, not a unit FILE state: list-unit-files cannot print a row for a file
// that is not there.
var unstartableUnitFileStates = map[string]bool{
	"masked":         true,
	"masked-runtime": true,
	"bad":            true,
}

// unitFileListed reports whether `systemctl list-unit-files <unit>` listed the
// unit itself as a unit that can be started.
//
// The name is compared in full because the argument is a PATTERN, not a name:
// the command answers with a table of every unit file that matched, and a row
// about another unit is not evidence about this one. The case that motivated it
// — an instance name (sing-box@config.service) answered with the template file
// (sing-box@.service) — was checked on systemd 255 and does not happen there:
// the command prints nothing and exits 1, so err != nil and no row is read at
// all. The full comparison stays as the cheap general rule, not as a workaround
// for behaviour anyone has seen.
func unitFileListed(output, unit string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != unit {
			continue
		}
		// Row layout is "<unit file> <state> [preset]"; without a state column
		// there is nothing to judge the unit by, and the file is there.
		if len(fields) > 1 && unstartableUnitFileStates[fields[1]] {
			return false
		}
		return true
	}
	return false
}

// detectSystemdService checks if amnezia-box is managed by systemd
func (m *Manager) detectSystemdService() string {
	if name := pickServiceName(systemdUnitPresent); name != "" {
		return name
	}

	// Check for template services (sing-box@*.service)
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "sing-box@") || strings.Contains(line, "amnezia-box@") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				// Extract service name without .service suffix
				name := strings.TrimSuffix(fields[0], ".service")
				return name
			}
		}
	}

	return ""
}

// getConfigFromProcess extracts the config path from the running process's
// cmdline. The argv parsing itself lives in configPathFromProcessArgv
// (dropin.go) and shares configPathFromArgv with the systemd-unit reader, so
// the two sources of truth can no longer disagree about the very same flags.
func (m *Manager) getConfigFromProcess() string {
	pid := m.findPID()
	if pid == 0 {
		return ""
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}

	// cmdline is null-separated and null-terminated: trim the terminator so the
	// split does not yield a trailing empty argument.
	args := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")

	// A relative config path is resolved against the process's own cwd, not ours.
	cwd, _ := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	return configPathFromProcessArgv(args, cwd)
}

// getConfigFromSystemd extracts the config path from the systemd unit's
// ExecStart. The parsing lives in configPathFromExecStart (dropin.go) and is
// shared with the drop-in fix, so the mismatch detector and the fix can never
// disagree about what the unit actually starts amnezia-box with.
func (m *Manager) getConfigFromSystemd() string {
	output, err := m.unitExecStart()
	if err != nil {
		return ""
	}
	return configPathFromExecStart(output)
}

// parseExecStartBinary extracts the executable path from `systemctl show
// <svc>.service --property=ExecStart` output. That output looks like:
//
//	ExecStart={ path=/usr/local/bin/amnezia-box ; argv[]=/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json ; ignore_errors=no ; ... }
//
// It returns the `path=` value (the executable systemd actually launches), or
// "" if the line has no `path=` field. Existence on disk is enforced by the
// caller, not here.
func parseExecStartBinary(showOutput string) string {
	line := strings.TrimSpace(showOutput)
	idx := strings.Index(line, "path=")
	if idx < 0 {
		return ""
	}
	rest := line[idx+len("path="):]
	// The executable path ends at the first space or semicolon.
	endIdx := strings.IndexAny(rest, " ;")
	if endIdx == -1 {
		endIdx = len(rest)
	}
	return strings.TrimSpace(rest[:endIdx])
}

// getBinaryFromSystemd resolves the executable path from the systemd unit's
// ExecStart, so the updater and version probe target the exact binary systemd
// launches (rather than a hardcoded findBinary() guess that may point at a
// stale copy). Returns "" when there is no service, the command errors, no
// path= is present, or the resolved path does not exist on disk.
func (m *Manager) getBinaryFromSystemd() string {
	if m.serviceName == "" {
		return ""
	}

	cmd := exec.Command("systemctl", "show", m.serviceName+".service", "--property=ExecStart")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	path := parseExecStartBinary(string(output))
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// IsSystemdManaged returns true if the process is managed by systemd
func (m *Manager) IsSystemdManaged() bool {
	if m.serviceName == "" {
		return false
	}

	// Check if service is active
	cmd := exec.Command("systemctl", "is-active", m.serviceName+".service")
	return cmd.Run() == nil
}

// findBinary locates the amnezia-box binary
func findBinary() string {
	// Check common locations
	paths := []string{
		"/usr/local/bin/amnezia-box",
		"/usr/bin/amnezia-box",
		"/opt/amnezia-box/amnezia-box",
		"./amnezia-box",
		"sing-box", // fallback to original name
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		return path
	}
	if path, err := exec.LookPath("sing-box"); err == nil {
		return path
	}

	return "amnezia-box"
}

// IsBinaryInstalled checks if sing-box/amnezia-box binary is installed and can run
func (m *Manager) IsBinaryInstalled() bool {
	_, err := m.GetVersion()
	return err == nil
}

// GetBinaryPath returns the path to the sing-box/amnezia-box binary
func (m *Manager) GetBinaryPath() string {
	return m.getBinaryPath()
}

// GetVersion returns the version of installed sing-box/amnezia-box binary
func (m *Manager) GetVersion() (string, error) {
	// Try the detected binary path first
	if bp := m.getBinaryPath(); bp != "" {
		if version, err := m.runVersion(bp); err == nil {
			return version, nil
		}
	}

	// A pinned path is the answer, including when the answer is "nothing runs
	// there". Falling through to whatever else is on PATH would silently undo
	// the pin — and since the updater installs to this same path, it would then
	// report one binary's version while replacing another one's file.
	if m.binaryIsPinned() {
		return "", fmt.Errorf("configured amnezia-box binary %s is missing or does not run", m.getBinaryPath())
	}

	// Try amnezia-box in PATH
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		if version, err := m.runVersion(path); err == nil {
			m.setBinaryPath(path) // Update to working path
			return version, nil
		}
	}

	// Try sing-box in PATH
	if path, err := exec.LookPath("sing-box"); err == nil {
		if version, err := m.runVersion(path); err == nil {
			m.setBinaryPath(path) // Update to working path
			return version, nil
		}
	}

	return "", fmt.Errorf("no working sing-box/amnezia-box binary found")
}

// runVersion executes binary with "version" command and returns the output.
// Results are memoized per binary path; the cache is also invalidated when the
// binary's mtime or size changes (handles in-place binary replacement).
// setBinaryPath invalidates the cache when the path itself changes.
func (m *Manager) runVersion(binaryPath string) (string, error) {
	fi, statErr := os.Stat(binaryPath)

	m.stateMu.RLock()
	cacheHit := m.cachedVersionPath == binaryPath &&
		m.cachedVersion != "" &&
		statErr == nil &&
		fi.ModTime().Equal(m.cachedVersionStamp) &&
		fi.Size() == m.cachedVersionSize
	if cacheHit {
		v := m.cachedVersion
		m.stateMu.RUnlock()
		return v, nil
	}
	m.stateMu.RUnlock()

	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run %s version: %w", binaryPath, err)
	}

	// Parse version from output (first line contains "sing-box version X.X.X" or similar)
	version := strings.TrimSpace(string(output))
	// Take first line only
	if idx := strings.Index(version, "\n"); idx > 0 {
		version = version[:idx]
	}

	// Extract just the version number (strip "sing-box version " or "amnezia-box version " prefix)
	if idx := strings.LastIndex(version, " "); idx > 0 {
		version = version[idx+1:]
	}

	m.stateMu.Lock()
	m.cachedVersion = version
	m.cachedVersionPath = binaryPath
	if statErr == nil {
		m.cachedVersionStamp = fi.ModTime()
		m.cachedVersionSize = fi.Size()
	}
	m.stateMu.Unlock()

	return version, nil
}

// runVersionFull executes `<binary> version` and returns the FULL multi-line
// output (unlike runVersion, which keeps only the parsed version number). The
// full output includes the build "Tags: ..." line, which is what tells us
// whether the binary was compiled with optional features such as
// with_v2ray_api. Memoized per binary path and invalidated on mtime/size change
// (same scheme as runVersion), so feature detection follows in-place binary
// replacement / downgrades.
func (m *Manager) runVersionFull(binaryPath string) (string, error) {
	fi, statErr := os.Stat(binaryPath)

	m.stateMu.RLock()
	cacheHit := m.cachedVersionFullPath == binaryPath &&
		m.cachedVersionFull != "" &&
		statErr == nil &&
		fi.ModTime().Equal(m.cachedVersionFullStamp) &&
		fi.Size() == m.cachedVersionFullSize
	if cacheHit {
		v := m.cachedVersionFull
		m.stateMu.RUnlock()
		return v, nil
	}
	m.stateMu.RUnlock()

	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run %s version: %w", binaryPath, err)
	}

	full := string(output)

	m.stateMu.Lock()
	m.cachedVersionFull = full
	m.cachedVersionFullPath = binaryPath
	if statErr == nil {
		m.cachedVersionFullStamp = fi.ModTime()
		m.cachedVersionFullSize = fi.Size()
	}
	m.stateMu.Unlock()

	return full, nil
}

// BinarySupportsV2RayAPI reports whether the binary at path advertises the
// with_v2ray_api build tag, via a FRESH `<path> version` run (no cache) —
// callers pass downloaded, not-yet-installed binaries (update preflight).
// Fail-closed like SupportsV2RayAPI: any exec error reads as unsupported.
func BinarySupportsV2RayAPI(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return false
	}
	return parseSupportsV2RayAPI(string(out))
}

// parseSupportsV2RayAPI reports whether a `<binary> version` output advertises
// the with_v2ray_api build tag. It scans the "Tags:" line and matches
// with_v2ray_api as a whole comma/space-delimited token, so a longer tag that
// merely contains the string (e.g. "with_v2ray_api_extra") does NOT
// false-positive. PURE.
func parseSupportsV2RayAPI(versionOutput string) bool {
	const tag = "with_v2ray_api"
	for _, line := range strings.Split(versionOutput, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "Tags:")
		if !ok {
			continue
		}
		// Tags are comma-separated; tolerate stray surrounding whitespace.
		for _, t := range strings.Split(rest, ",") {
			if strings.TrimSpace(t) == tag {
				return true
			}
		}
	}
	return false
}

// SupportsV2RayAPI reports whether the running amnezia-box binary was built with
// the with_v2ray_api feature (required for the experimental.v2ray_api config
// block). It runs/uses the cached full `<binary> version` output and inspects
// its Tags line. FAIL-CLOSED: any exec/lookup error returns false, so RouteBox
// never writes a config block a binary cannot accept.
func (m *Manager) SupportsV2RayAPI() bool {
	if bp := m.getBinaryPath(); bp != "" {
		if out, err := m.runVersionFull(bp); err == nil {
			return parseSupportsV2RayAPI(out)
		}
	}
	// Fall back to PATH lookups, mirroring GetVersion's discovery order.
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		if out, err := m.runVersionFull(path); err == nil {
			m.setBinaryPath(path)
			return parseSupportsV2RayAPI(out)
		}
	}
	if path, err := exec.LookPath("sing-box"); err == nil {
		if out, err := m.runVersionFull(path); err == nil {
			m.setBinaryPath(path)
			return parseSupportsV2RayAPI(out)
		}
	}
	return false
}

// parseBaseAtLeast114 reports whether a "version X.Y.Z…" line is sing-box >= 1.14.
func parseBaseAtLeast114(versionOutput string) bool {
	for _, line := range strings.Split(versionOutput, "\n") {
		i := strings.Index(line, "version ")
		if i < 0 {
			continue
		}
		var maj, min int
		if _, err := fmt.Sscanf(line[i+len("version "):], "%d.%d", &maj, &min); err != nil {
			continue
		}
		if maj > 1 || (maj == 1 && min >= 14) {
			return true
		}
	}
	return false
}

// awgLevel returns (major, minor, ok) parsed from an "awgN[.M]" token in the line.
func awgLevel(line string) (maj, min int, ok bool) {
	i := strings.Index(line, "awg")
	if i < 0 {
		return 0, 0, false
	}
	rest := line[i+3:]
	// major: leading digits
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 {
		return 0, 0, false
	}
	maj, _ = strconv.Atoi(rest[:j])
	// optional ".minor"
	if j < len(rest) && rest[j] == '.' {
		k := j + 1
		for k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
			k++
		}
		if k > j+1 {
			min, _ = strconv.Atoi(rest[j+1 : k])
		}
	}
	return maj, min, true
}

// parseSupportsAWGServer: awg server works on awg2.1+ (S4-keepalive fix) or awg3+
// (bare "awg3" token) or any 1.14+ base build (all carry the fork's awg endpoint). PURE.
func parseSupportsAWGServer(versionOutput string) bool {
	for _, line := range strings.Split(versionOutput, "\n") {
		if maj, min, ok := awgLevel(line); ok {
			if maj > 2 || (maj == 2 && min >= 1) {
				return true
			}
		}
	}
	return parseBaseAtLeast114(versionOutput)
}

// parseSupportsAWG3 reports whether the binary accepts the awg3 endpoint fields
// (header_protection_key / content_padding_addition / rekey_after_time): an awg
// major >= 3 token, or a 1.14+ base build. awg2.1 and older REJECT those fields,
// so this fails closed for them. PURE.
func parseSupportsAWG3(versionOutput string) bool {
	for _, line := range strings.Split(versionOutput, "\n") {
		if maj, _, ok := awgLevel(line); ok && maj >= 3 {
			return true
		}
	}
	return parseBaseAtLeast114(versionOutput)
}

// SupportsAWGServer reports whether the running binary can host an AWG server
// endpoint. FAIL-CLOSED: any exec/lookup error returns false, mirroring
// SupportsV2RayAPI, so RouteBox never renders an awg-server endpoint a binary
// would half-accept.
func (m *Manager) SupportsAWGServer() bool {
	if bp := m.getBinaryPath(); bp != "" {
		if out, err := m.runVersionFull(bp); err == nil {
			return parseSupportsAWGServer(out)
		}
	}
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		if out, err := m.runVersionFull(path); err == nil {
			m.setBinaryPath(path)
			return parseSupportsAWGServer(out)
		}
	}
	return false
}

// SupportsAWG3 reports whether the running binary accepts the awg3-only endpoint
// fields (header_protection_key / content_padding_addition / rekey_after_time).
// FAIL-CLOSED: any exec/lookup error returns false, mirroring SupportsAWGServer,
// so RouteBox never emits fields an older binary would reject.
func (m *Manager) SupportsAWG3() bool {
	if bp := m.getBinaryPath(); bp != "" {
		if out, err := m.runVersionFull(bp); err == nil {
			return parseSupportsAWG3(out)
		}
	}
	if path, err := exec.LookPath("amnezia-box"); err == nil {
		if out, err := m.runVersionFull(path); err == nil {
			m.setBinaryPath(path)
			return parseSupportsAWG3(out)
		}
	}
	return false
}

// GetStatus returns the current process status.
//
// Обёртка ровно ради алиаса: собранное состояние проходит через одну точку,
// поэтому ни один из путей сборки статуса не может забыть проставить
// config_path — см. withLegacyConfigPath.
func (m *Manager) GetStatus() Status {
	return m.status().withLegacyConfigPath()
}

func (m *Manager) status() Status {
	// Get version info (cached in binaryPath if successful)
	version, _ := m.GetVersion()
	bp := m.getBinaryPath()

	// Always include system checks
	systemChecks := GetSystemChecks()

	// Состояние путей конфига сообщаем в любом состоянии процесса: расхождение с
	// юнитом чаще всего и проявляется как "процесс не запущен", а панели нужен
	// повод показать баннер. Пути живого процесса при этом нет — и это честное
	// "источника нет", а не совпадение.
	paths := m.configPaths("")

	pid := m.findPID()
	if pid == 0 {
		status := Status{
			Running:      false,
			SupportsHUP:  true,
			Version:      version,
			BinaryPath:   bp,
			SystemChecks: systemChecks,
			ConfigPaths:  paths,
		}
		// Still report if systemd service exists
		if m.serviceName != "" {
			status.ServiceName = m.serviceName
			status.ManagedBy = "systemd"
		}
		return status
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		return Status{Running: false, SupportsHUP: true, Version: version, BinaryPath: bp, ServiceName: m.serviceName, ConfigPaths: paths}
	}

	// On Unix, FindProcess always succeeds. Need to send signal 0 to check.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return Status{Running: false, SupportsHUP: true, Version: version, BinaryPath: bp, ServiceName: m.serviceName, ConfigPaths: paths}
	}

	// Get uptime from /proc
	uptime := m.getUptime(pid)

	// Detect management type and config. ServiceName reports the DETECTED unit
	// even when this particular process was launched by hand: the panel needs the
	// unit name to say whose ExecStart the config-mismatch banner is about, and
	// hiding it there left the banner talking about "the systemd unit" without
	// ever naming it. Who runs the process is ManagedBy's job.
	managedBy := "standalone"
	if m.IsSystemdManaged() {
		managedBy = "systemd"
	}
	serviceName := m.serviceName

	// Путь живого процесса — третий источник правды, и он попадает в то же самое
	// состояние: панель сравнивает его с нашим путём, а не гадает по отдельному
	// эндпоинту, как раньше.
	paths = m.configPaths(m.getConfigFromProcess())

	return Status{
		Running:      true,
		PID:          pid,
		Uptime:       uptime,
		ManagedBy:    managedBy,
		ServiceName:  serviceName,
		SupportsHUP:  true, // sing-box supports SIGHUP
		Version:      version,
		BinaryPath:   bp,
		SystemChecks: systemChecks,
		ConfigPaths:  paths,
	}
}

// findPID finds the PID of the running amnezia-box/sing-box process by
// resolving /proc/<pid>/exe. This cannot be confused with processes that
// merely mention the binary name in their arguments (e.g. RouteBox itself
// started with "-config /opt/etc/sing-box/config.json").
func (m *Manager) findPID() int {
	if m.pidFinder != nil {
		return m.pidFinder()
	}
	self := os.Getpid()

	// Prefer the PID we started ourselves in standalone mode. Signal(0) is a
	// cheap liveness probe; exeMatches then guards against PID reuse.
	if startedPID := m.getStartedPID(); startedPID != 0 && startedPID != self {
		if proc, err := os.FindProcess(startedPID); err == nil &&
			proc.Signal(syscall.Signal(0)) == nil && m.exeMatches(startedPID) {
			return startedPID
		}
		m.clearStartedPIDIf(startedPID) // stale: process exited or PID was reused
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		if m.exeMatches(pid) {
			return pid
		}
	}
	return 0
}

// exeMatches reports whether /proc/<pid>/exe points at the managed binary.
func (m *Manager) exeMatches(pid int) bool {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return false
	}
	exe = strings.TrimSuffix(exe, " (deleted)")
	if bp := m.getBinaryPath(); bp != "" {
		if abs, err := filepath.Abs(bp); err == nil {
			// /proc/<pid>/exe is always fully resolved; resolve symlinks on
			// our side too so a symlinked binaryPath still exact-matches.
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				abs = resolved
			}
			if exe == abs {
				return true
			}
		}
		if filepath.Base(exe) == filepath.Base(bp) {
			return true
		}
	}
	base := filepath.Base(exe)
	return base == "amnezia-box" || base == "sing-box"
}

// getUptime returns process uptime as a human-readable string
func (m *Manager) getUptime(pid int) string {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ""
	}

	// Parse stat file (field 22 is starttime in clock ticks)
	fields := strings.Fields(string(data))
	if len(fields) < 22 {
		return ""
	}

	startTime, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return ""
	}

	// Get system uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ""
	}

	var systemUptime float64
	fmt.Sscanf(string(uptimeData), "%f", &systemUptime)

	// Calculate process uptime (clock ticks are usually 100/sec)
	clkTck := int64(100)
	processStartSec := startTime / clkTck
	processUptime := time.Duration(int64(systemUptime)-processStartSec) * time.Second
	if processUptime < 0 {
		processUptime = 0
	}

	return formatDuration(processUptime)
}

// formatDuration formats duration as "Xd Xh Xm"
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// systemdReloadArgs builds the `systemctl` argv that reloads the service by
// sending SIGHUP to its main process. It deliberately does NOT use `reload`:
// the amnezia-box unit has no ExecReload= (CanReload=no), so `systemctl reload`
// is rejected; sing-box reloads its config in place on SIGHUP.
func systemdReloadArgs(serviceName string) []string {
	return []string{"kill", "--signal=SIGHUP", serviceName + ".service"}
}

// Reload sends SIGHUP to reload configuration without restart
func (m *Manager) Reload() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := m.checkConfigMismatch("reloading"); err != nil {
		return err
	}

	status := m.GetStatus()
	if !status.Running {
		return fmt.Errorf("amnezia-box is not running")
	}

	// If managed by systemd, deliver SIGHUP to the service. We deliberately do
	// NOT use `systemctl reload`: the amnezia-box unit ships with no ExecReload=
	// (CanReload=no), so systemd rejects reload with "Job type reload is not
	// applicable for unit amnezia-box.service". sing-box reloads its config
	// in place on SIGHUP (no restart, same PID), so send that to the main
	// process via `systemctl kill --signal=SIGHUP`. (--kill-whom is omitted for
	// compatibility with systemd <252; amnezia-box is a single-process
	// Type=simple unit, so the default reaches the main PID.)
	if m.IsSystemdManaged() {
		cmd := exec.Command("systemctl", systemdReloadArgs(m.serviceName)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl SIGHUP reload failed: %s", string(output))
		}
		return nil
	}

	// Otherwise, send SIGHUP directly
	process, err := os.FindProcess(status.PID)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP: %w", err)
	}

	// Give it a moment to reload
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Start starts the amnezia-box process
func (m *Manager) Start(configPath string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := m.checkConfigMismatch("starting"); err != nil {
		return err
	}

	return m.startLocked(configPath)
}

// startLocked starts the process. Caller must hold opMu.
func (m *Manager) startLocked(configPath string) error {
	if m.GetStatus().Running {
		return fmt.Errorf("amnezia-box is already running")
	}

	// If a systemd service exists, use systemctl
	if m.serviceName != "" {
		cmd := exec.Command("systemctl", "start", m.serviceName+".service")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl start failed: %s", string(output))
		}

		// Wait and verify
		time.Sleep(500 * time.Millisecond)
		if !m.GetStatus().Running {
			// Try to get more info from journalctl
			journalCmd := exec.Command("journalctl", "-u", m.serviceName+".service", "-n", "10", "--no-pager")
			if logs, err := journalCmd.Output(); err == nil {
				return fmt.Errorf("service started but exited immediately. Logs:\n%s", string(logs))
			}
			return fmt.Errorf("service started but exited immediately")
		}
		return nil
	}

	// Standalone mode
	args := []string{"run", "-c", configPath}
	cmd := exec.Command(m.getBinaryPath(), args...)

	// Detach from parent
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Redirect output to /dev/null (logs go to configured log file)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	m.setStartedPID(cmd.Process.Pid)

	// Don't wait - let it run in background
	go cmd.Wait()

	// Wait a bit and check if it's running
	time.Sleep(500 * time.Millisecond)
	if !m.GetStatus().Running {
		return fmt.Errorf("process started but exited immediately")
	}

	return nil
}

// Stop stops the amnezia-box process
func (m *Manager) Stop() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.stopLocked()
}

// stopLocked stops the process. Caller must hold opMu.
func (m *Manager) stopLocked() error {
	status := m.GetStatus()
	if !status.Running {
		return fmt.Errorf("amnezia-box is not running")
	}

	// If managed by systemd, use systemctl
	if m.IsSystemdManaged() {
		cmd := exec.Command("systemctl", "stop", m.serviceName+".service")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl stop failed: %s", string(output))
		}

		// Wait and verify
		for i := 0; i < 50; i++ {
			time.Sleep(100 * time.Millisecond)
			if !m.GetStatus().Running {
				return nil
			}
		}
		return fmt.Errorf("service stop timed out")
	}

	// Standalone mode
	process, err := os.FindProcess(status.PID)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 50; i++ { // 5 seconds timeout
		time.Sleep(100 * time.Millisecond)
		if !m.GetStatus().Running {
			m.setStartedPID(0)
			return nil
		}
	}

	// Force kill if still running
	process.Signal(syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)

	if m.GetStatus().Running {
		return fmt.Errorf("failed to stop process")
	}

	m.setStartedPID(0)
	return nil
}

// Restart restarts the amnezia-box process
func (m *Manager) Restart(configPath string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := m.checkConfigMismatch("restarting"); err != nil {
		return err
	}

	// If managed by systemd, use systemctl restart
	if m.IsSystemdManaged() {
		cmd := exec.Command("systemctl", "restart", m.serviceName+".service")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl restart failed: %s", string(output))
		}

		// Wait and verify
		time.Sleep(500 * time.Millisecond)
		if !m.GetStatus().Running {
			return fmt.Errorf("service restarted but is not running")
		}
		return nil
	}

	// Standalone mode
	if m.GetStatus().Running {
		if err := m.stopLocked(); err != nil {
			return fmt.Errorf("failed to stop: %w", err)
		}
	}

	return m.startLocked(configPath)
}

// GetJournalLogs returns recent logs from systemd journal
func (m *Manager) GetJournalLogs(lines int) (string, error) {
	if m.serviceName == "" {
		return "", fmt.Errorf("not managed by systemd")
	}

	cmd := exec.Command("journalctl", "-u", m.serviceName+".service", "-n", fmt.Sprintf("%d", lines), "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(output), nil
}

// ParseSemver extracts major, minor, patch from a version string like "sing-box version 1.12.17" or "1.12.17"
func ParseSemver(versionStr string) (major, minor, patch int) {
	parts := strings.Fields(versionStr)
	ver := ""
	for _, p := range parts {
		if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
			ver = p
			break
		}
	}
	if ver == "" {
		return 0, 0, 0
	}
	if idx := strings.IndexAny(ver, "-+"); idx >= 0 {
		ver = ver[:idx]
	}
	segments := strings.Split(ver, ".")
	if len(segments) >= 1 {
		major, _ = strconv.Atoi(segments[0])
	}
	if len(segments) >= 2 {
		minor, _ = strconv.Atoi(segments[1])
	}
	if len(segments) >= 3 {
		patch, _ = strconv.Atoi(segments[2])
	}
	return
}

// VersionAtLeast returns true if the parsed version is >= the given major.minor
func VersionAtLeast(versionStr string, minMajor, minMinor int) bool {
	major, minor, _ := ParseSemver(versionStr)
	if major > minMajor {
		return true
	}
	return major == minMajor && minor >= minMinor
}

// GetFeatureFlags returns feature flags based on the detected sing-box version
func (m *Manager) GetFeatureFlags() map[string]bool {
	version, err := m.GetVersion()
	if err != nil {
		return map[string]bool{
			"inline_rule_sets":         false,
			"client_sniff":             false,
			"process_path_regex":       false,
			"rule_set_ip_match_source": false,
			"network_strategy":         false,
			"tls_fragment":             false,
			"default_domain_resolver":  false,
			"domain_resolver":          false,
			"bypass_action":            false,
			"icmp_network":             false,
		}
	}

	return map[string]bool{
		"inline_rule_sets":         VersionAtLeast(version, 1, 10),
		"client_sniff":             VersionAtLeast(version, 1, 10),
		"process_path_regex":       VersionAtLeast(version, 1, 10),
		"rule_set_ip_match_source": VersionAtLeast(version, 1, 10),
		"network_strategy":         VersionAtLeast(version, 1, 11),
		"tls_fragment":             VersionAtLeast(version, 1, 12),
		"default_domain_resolver":  VersionAtLeast(version, 1, 12),
		"domain_resolver":          VersionAtLeast(version, 1, 12), // for outbounds/endpoints
		"bypass_action":            VersionAtLeast(version, 1, 13),
		"icmp_network":             VersionAtLeast(version, 1, 13), // network field: 'icmp'
	}
}

// GetSystemChecks checks system requirements for routing
func GetSystemChecks() *SystemChecks {
	checks := &SystemChecks{}

	// Check if running as root (required for TUN interface)
	checks.IsRoot = os.Geteuid() == 0

	// Check IPv4 forwarding: net.ipv4.ip_forward
	if data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err == nil {
		checks.IPv4Forward = strings.TrimSpace(string(data)) == "1"
	}

	// Check IPv6 forwarding: net.ipv6.conf.all.forwarding
	if data, err := os.ReadFile("/proc/sys/net/ipv6/conf/all/forwarding"); err == nil {
		checks.IPv6Forward = strings.TrimSpace(string(data)) == "1"
	}

	// Check if IPv6 is completely disabled: net.ipv6.conf.all.disable_ipv6
	// When disabled=1, sing-box cannot bind IPv6 addresses to TUN interface
	if data, err := os.ReadFile("/proc/sys/net/ipv6/conf/all/disable_ipv6"); err == nil {
		checks.IPv6Disabled = strings.TrimSpace(string(data)) == "1"
	}

	// All critical checks (root + IPv4 forwarding required)
	checks.AllChecksPassed = checks.IsRoot && checks.IPv4Forward

	return checks
}

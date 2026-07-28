package process

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// dropInName — имя нашего drop-in файла. Числовой префикс задаёт порядок
// применения: systemd читает drop-in'ы лексикографически, и 10- ставит нас
// раньше произвольных пользовательских файлов, но после самого юнита.
const dropInName = "10-routebox-config.conf"

// unsafeUnitChars — символы, с которыми путь нельзя класть в юнит: перевод
// строки и возврат каретки вписали бы в drop-in произвольные директивы
// (filepath.Clean их не убирает), пробел и таб развалили бы аргумент, а '%'
// systemd молча раскроет как спецификатор (%H и подобные).
const unsafeUnitChars = " \t\n\r%"

// defaultSystemdRoot — каталог юнитов systemd. Отдельной константой, потому что
// в тестах он подменяется: writing anywhere near /etc из тестов недопустимо.
const defaultSystemdRoot = "/etc/systemd/system"

// dropInPath возвращает путь drop-in файла для юнита service в каталоге root.
func dropInPath(root, service string) string {
	return filepath.Join(root, service+".service.d", dropInName)
}

// dropInPath — путь нашего drop-in для юнита этого менеджера ("" — юнита нет,
// значит и файла нашего быть не может).
func (m *Manager) dropInPath() string {
	if m.serviceName == "" {
		return ""
	}
	root := m.systemdRoot
	if root == "" {
		root = defaultSystemdRoot
	}
	return dropInPath(root, m.serviceName)
}

// dropInContent рендерит drop-in, заменяющий ExecStart юнита. Пустая строка
// ExecStart= обязательна: без неё systemd ДОБАВИТ команду, а не заменит её,
// и юнит попытается поднять два amnezia-box с разными конфигами.
func dropInContent(argv []string) string {
	return "[Service]\nExecStart=\nExecStart=" + joinArgv(argv) + "\n"
}

// validateUnitPath отказывает пути, который небезопасно или бессмысленно класть
// в systemd-юнит. what называет проверяемое ("config path", "binary path"),
// чтобы пользователь по тексту ошибки понял, что именно чинить.
func validateUnitPath(what, p string) error {
	if p == "" {
		return fmt.Errorf("%s is empty", what)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s %q is not absolute: systemd needs a full path in ExecStart", what, p)
	}
	if i := strings.IndexAny(p, unsafeUnitChars); i >= 0 {
		return fmt.Errorf("%s %q contains %q, which is not safe inside a systemd unit", what, p, string(p[i]))
	}
	return nil
}

// splitArgv разбирает argv из вывода systemctl. Аргументы разделены пробелами;
// systemd (в зависимости от версии) печатает их как есть либо в двойных
// кавычках, поэтому понимаем оба варианта.
func splitArgv(s string) []string {
	var (
		argv    []string
		cur     strings.Builder
		inQuote bool
		started bool
	)
	flush := func() {
		if started {
			argv = append(argv, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			started = true
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return argv
}

// joinArgv собирает argv обратно в строку ExecStart. Аргумент с пробелом
// заковычиваем — иначе он развалится на два при следующем чтении юнита.
//
// '%' экранируем в '%%': argv мы читаем из `systemctl show`, который печатает
// ExecStart с УЖЕ раскрытыми спецификаторами, так что '%' здесь — литеральный
// символ чужого аргумента. Записанный дословно, он раскрылся бы ещё раз (%i,
// %H, %n) и молча подменил бы аргумент, который мы обязались сохранить.
// Экранирование выбрано вместо валидации всех аргументов: валидация превратила
// бы законный '%' в отказ чинить юнит и оставила бы пользователя с
// расхождением и без кнопки, тогда как %% — точный обратный перевод. Наш
// собственный путь конфига при этом всё равно проверяется validateUnitPath:
// туда '%' попадать незачем, и запрет там дешевле разбирательств.
func joinArgv(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		a = strings.ReplaceAll(a, "%", "%%")
		if strings.ContainsAny(a, " \t") {
			quoted[i] = `"` + a + `"`
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}

// parseExecStartArgv достаёт argv первой команды ExecStart из вывода
// `systemctl show <svc>.service --property=ExecStart`, который выглядит так:
//
//	ExecStart={ path=/usr/local/bin/amnezia-box ; argv[]=/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json ; ignore_errors=no ; ... }
//
// Возвращает nil, если argv[] в выводе нет — юнит нестандартный, и гадать по
// нему нельзя.
func parseExecStartArgv(showOutput string) []string {
	const marker = "argv[]="
	idx := strings.Index(showOutput, marker)
	if idx < 0 {
		return nil
	}
	rest := showOutput[idx+len(marker):]
	// Значение argv[] заканчивается разделителем полей " ; ", закрывающей
	// скобкой записи или концом строки.
	if end := strings.Index(rest, " ; "); end >= 0 {
		rest = rest[:end]
	}
	if end := strings.IndexAny(rest, "\n\r"); end >= 0 {
		rest = rest[:end]
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimSuffix(rest, "}")
	argv := splitArgv(rest)
	if len(argv) == 0 {
		return nil
	}
	return argv
}

// configArg описывает, где в argv лежит путь конфига. Одна структура на два
// применения — чтение (детектор расхождения) и запись (починка), — чтобы они
// не могли разъехаться в понимании одного и того же ExecStart.
type configArg struct {
	flag       string // токен флага как написан: "-c", "--config-directory" или префикс "--config="
	valueIndex int    // индекс токена со значением; -1 — за флагом значения нет
	value      string // само значение ("" при valueIndex < 0)
	inline     bool   // форма --config=<value>: значение в том же токене
	directory  bool   // -C/--config-directory: значение — каталог, а не файл
}

// findConfigArg находит в argv флаг конфига. ok=false — флага нет вовсе.
func findConfigArg(argv []string) (configArg, bool) {
	// argv[0] — исполняемый файл, флагом он быть не может, поэтому идём с 1.
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		dir := false
		switch a {
		case "-C", "--config-directory":
			dir = true
			fallthrough
		case "-c", "--config":
			ca := configArg{flag: a, valueIndex: -1, directory: dir}
			if i+1 < len(argv) {
				ca.valueIndex = i + 1
				ca.value = argv[i+1]
			}
			return ca, true
		}
		for _, prefix := range []string{"-c=", "--config=", "-C=", "--config-directory="} {
			if strings.HasPrefix(a, prefix) {
				return configArg{
					flag:       prefix,
					valueIndex: i,
					value:      strings.TrimPrefix(a, prefix),
					inline:     true,
					directory:  strings.HasPrefix(prefix, "-C=") || strings.HasPrefix(prefix, "--config-directory="),
				}, true
			}
		}
		// Слитная короткая форма -c/etc/config.json, которую понимает cobra:
		// значение приклеено к флагу без разделителя. Длинные флаги сюда не
		// попадают — они начинаются с "--".
		for _, flag := range []string{"-c", "-C"} {
			if strings.HasPrefix(a, flag) && len(a) > len(flag) {
				return configArg{
					flag:       flag,
					valueIndex: i,
					value:      a[len(flag):],
					inline:     true,
					directory:  flag == "-C",
				}, true
			}
		}
	}
	return configArg{valueIndex: -1}, false
}

// configPathFromProcessArgv возвращает путь конфига по argv работающего
// процесса. Разбор флагов — тот же configPathFromArgv, через который проходит
// и ExecStart юнита: два разбора уже расходились (слитную форму -c/path понимал
// только этот источник), и расхождение выключало сверку целиком — путь из юнита
// не читался, «пути нет» трактуется как «расхождения нет».
//
// Своё у процесса только то, чего у юнита нет: рабочий каталог, относительно
// которого разворачивается относительный путь, и -D <dir> как последняя догадка
// (это не объявленный путь конфига, а каталог, в котором он может лежать,
// поэтому файл там обязан существовать).
func configPathFromProcessArgv(argv []string, cwd string) string {
	if p := configPathFromArgv(argv); p != "" {
		if !filepath.IsAbs(p) && cwd != "" {
			return filepath.Join(cwd, p)
		}
		return p
	}
	for i := 1; i < len(argv); i++ {
		if argv[i] == "-D" && i+1 < len(argv) {
			return existingConfigInDir(argv[i+1])
		}
		if strings.HasPrefix(argv[i], "-D") && len(argv[i]) > 2 {
			return existingConfigInDir(argv[i][2:])
		}
	}
	return ""
}

// existingConfigInDir возвращает <dir>/config.json, если такой файл есть.
func existingConfigInDir(dir string) string {
	configPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}
	return ""
}

// retargetExecStart меняет в argv ТОЛЬКО путь конфига, сохраняя бинарь,
// подкоманду и все прочие аргументы: руками правленый юнит вполне может нести
// --disable-color, --log-level или -D, и молча их срезать нельзя.
//
// Если юнит запускается через -C/--config-directory (каталог, а не файл), режим
// сохраняется: подставляется каталог нового конфига. Переключить такой юнит на
// -c значило бы сменить способ запуска, а не починить путь.
//
// Юнит без флага конфига перенацелить нечем: дописать -c значило бы сменить
// способ запуска (юнит мог жить на -D <workdir> или на пути по умолчанию), а не
// починить путь, — поэтому отказ. Практически сюда не доходят: такой юнит не
// даёт расхождения (путь из него не читается), а без расхождения хендлер
// отвечает 409 и до починки дело не идёт.
func retargetExecStart(argv []string, configPath string) ([]string, error) {
	if len(argv) == 0 {
		return nil, errors.New("the unit's ExecStart has no command to retarget")
	}
	out := append([]string(nil), argv...)

	ca, ok := findConfigArg(out)
	if !ok {
		return nil, errors.New("the unit's ExecStart names no config flag, so there is no path to retarget; point it at the config by hand")
	}
	if ca.valueIndex < 0 {
		return nil, fmt.Errorf("the unit's ExecStart ends with %s and no value; retarget the unit by hand", ca.flag)
	}
	value := configPath
	if ca.directory {
		value = filepath.Dir(configPath)
	}
	if ca.inline {
		out[ca.valueIndex] = ca.flag + value
	} else {
		out[ca.valueIndex] = value
	}
	return out, nil
}

// dropInWriteError оборачивает ошибку файловой операции над drop-in. Это
// единственное место, где RouteBox пишет за пределами своих файлов, и почти
// всегда отказ здесь — отсутствие root: голое "permission denied" пользователю
// ничего не объясняет, поэтому причину называем прямо.
func dropInWriteError(op, path string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%s %s: %w (RouteBox must run as root to change the systemd unit)", op, path, err)
	}
	return fmt.Errorf("%s %s: %w", op, path, err)
}

// ErrNoSystemdUnit — юнита нет вовсе (это не «не смогли прочитать»).
var ErrNoSystemdUnit = errors.New("amnezia-box is not managed by systemd here")

// ErrNoConfigDropIn — снимать нечего: RouteBox не ставил здесь drop-in (либо и
// юнита нет). Отдельная ошибка, чтобы API отвечал 409, а не 500: «нечего
// снимать» — это состояние, а не поломка.
var ErrNoConfigDropIn = errors.New("RouteBox has no systemd drop-in installed here")

// unitExecStart возвращает сырой вывод `systemctl show --property=ExecStart`
// для юнита ("" — юнита нет либо команда не отработала).
func (m *Manager) unitExecStart() (string, error) {
	if m.serviceName == "" {
		return "", ErrNoSystemdUnit
	}
	if m.unitExecStartReader != nil {
		return m.unitExecStartReader()
	}
	out, err := exec.Command("systemctl", "show", m.serviceName+".service", "--property=ExecStart").Output()
	if err != nil {
		return "", fmt.Errorf("systemctl show %s.service: %w", m.serviceName, err)
	}
	return string(out), nil
}

// reloadSystemd перечитывает юниты. Шов: в тестах настоящий systemctl менял бы
// состояние машины, а не проверял наш код.
func (m *Manager) reloadSystemd() error {
	if m.daemonReload != nil {
		return m.daemonReload()
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		if len(out) > 0 {
			return fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	return nil
}

// readConfigDropIn читает наш drop-in с диска. unitPath — путь конфига, который
// юнит называет ПРЯМО СЕЙЧАС: по нему видно, подхватил ли systemd файл.
//
// Состояние берётся с диска на каждом опросе, а не запоминается: файл переживает
// перезапуск панели, и «забыли, что ставили» — ровно та ситуация, из-за которой
// через месяц никто не может объяснить override в юните.
func (m *Manager) readConfigDropIn(unitPath string) *ConfigDropIn {
	path := m.dropInPath()
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		// Файл есть, но не читается (права, битый носитель). Факт наличия важнее
		// разбора: именно он объясняет, откуда взялся override.
		return &ConfigDropIn{Path: path}
	}
	configPath := configPathFromArgv(splitArgv(dropInExecStart(string(content))))
	return &ConfigDropIn{
		Path:       path,
		ConfigPath: configPath,
		// Пустые пути — «неизвестно», а не «расходится»: объявлять ожидание
		// reload по незнанию значит пугать на ровном месте.
		PendingReload: configPath != "" && unitPath != "" && resolveConfigPath(configPath) != resolveConfigPath(unitPath),
	}
}

// dropInExecStart возвращает команду из последней непустой строки ExecStart=
// drop-in'а. Именно последней: пустая ExecStart= сбрасывает список, и команда
// идёт за ней — тот же порядок, в котором файл пишет dropInContent.
func dropInExecStart(content string) string {
	cmd := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		if v := strings.TrimPrefix(line, "ExecStart="); v != "" {
			cmd = v
		}
	}
	return cmd
}

// readUnitConfigPath читает путь конфига из юнита. Поле unitConfigReader —
// шов для тестов: настоящее чтение зовёт systemctl и в тестах недоступно.
func (m *Manager) readUnitConfigPath() (string, error) {
	if m.unitConfigReader != nil {
		return m.unitConfigReader()
	}
	return m.unitConfigPath()
}

// unitConfigPath — как UnitConfigPath, но отличает «юнит не прочитался» от
// «в юните нет пути»: пересверке после починки эта разница принципиальна.
func (m *Manager) unitConfigPath() (string, error) {
	out, err := m.unitExecStart()
	if err != nil {
		return "", err
	}
	return configPathFromExecStart(out), nil
}

// verifyUnitRetargeted пересверяет расхождение ПО ФАКТУ юнита после записи
// drop-in: путь читается из ExecStart заново, а не подставляется ожидаемый.
// Если systemd drop-in не подхватил, расхождение остаётся зафиксированным и
// возвращается ошибка — соврать «починено» хуже, чем сказать «не вышло».
func (m *Manager) verifyUnitRetargeted(configPath, dropInFile string) error {
	unit, err := m.readUnitConfigPath()
	if err != nil {
		// «Не смогли проверить» — не «починено»: расхождение оставляем как есть.
		return fmt.Errorf("drop-in written to %s, but re-reading the unit failed (%w), so the fix is unconfirmed and the config path mismatch stays in place", dropInFile, err)
	}
	if unit == "" {
		return fmt.Errorf("drop-in written to %s, but the unit's ExecStart still names no config path; the mismatch stays in place", dropInFile)
	}
	m.SetConfigPaths(configPath, unit)
	if ours, unit, ok := m.ConfigMismatch(); ok {
		return fmt.Errorf("drop-in written to %s, but the unit still starts amnezia-box with %s instead of %s", dropInFile, unit, ours)
	}
	return nil
}

// ReadUnitConfigPath читает путь конфига из ExecStart юнита, отличая «юнита
// нет» (ErrNoSystemdUnit) от «не смогли прочитать» (любая другая ошибка) и от
// «юнит не называет конфиг» (пустая строка без ошибки).
//
// Разница принципиальна для перехода на путь юнита: менять что-либо можно
// только по УСПЕШНОМУ чтению. Пустая строка из упавшего systemctl, принятая за
// «юнита нет», объявила бы расхождение разрешённым, ничего не проверив.
func (m *Manager) ReadUnitConfigPath() (string, error) {
	return m.readUnitConfigPath()
}

// WriteConfigDropIn перенацеливает systemd-юнит на configPath: разбирает
// ExecStart юнита, подменяет в нём только путь конфига, пишет drop-in,
// перечитывает юниты и пересверяет результат. Сам юнит на месте не правится —
// drop-in обратим и переживает переустановку пакета.
func (m *Manager) WriteConfigDropIn(configPath string) error {
	// serviceName выставляется один раз в NewManager и дальше не меняется.
	service := m.serviceName
	if service == "" {
		return errors.New("no systemd unit to fix: amnezia-box is not managed by systemd here")
	}
	// Валидируем до любого обращения к диску: путь едет прямо в системный юнит.
	if err := validateUnitPath("config path", configPath); err != nil {
		return err
	}

	showOut, err := m.unitExecStart()
	if err != nil {
		return err
	}
	argv := parseExecStartArgv(showOut)
	if argv == nil {
		return fmt.Errorf("cannot read ExecStart of %s.service: the unit is non-standard, retarget it by hand", service)
	}
	// Бинарь берём из самого юнита, а не угадываем: fallback-имя вроде
	// "sing-box" дало бы относительный ExecStart — daemon-reload его проглотит,
	// пересверка пути конфига пройдёт, а юнит упадёт при следующем старте.
	if err := validateUnitPath("binary path", argv[0]); err != nil {
		return err
	}
	argv, err = retargetExecStart(argv, configPath)
	if err != nil {
		return err
	}

	path := m.dropInPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return dropInWriteError("create drop-in dir", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(dropInContent(argv)), 0o644); err != nil {
		return dropInWriteError("write drop-in", path, err)
	}

	if err := m.reloadSystemd(); err != nil {
		// Файл уже на диске, и удалять его молча нельзя: снятие — тоже действие
		// над чужим юнитом, и решать его пользователю. Но и умолчать нельзя:
		// systemd подхватит файл при любом следующем daemon-reload или
		// перезагрузке — то есть починка применится позже и без спроса, если
		// пользователь не знает, что она наполовину состоялась.
		return fmt.Errorf("%w — the drop-in is already written to %s and stays there, so systemd will apply it at the next daemon-reload or reboot; remove it from the panel to undo", err, path)
	}

	return m.verifyUnitRetargeted(configPath, path)
}

// DropInRemoval — итог снятия drop-in. Warning непустой, когда файл удалён, но
// что-то после удаления не отработало: это не провал (файла уже нет) и не
// чистый успех, и подавать его как одно из двух значило бы соврать.
type DropInRemoval struct {
	// UnitConfigPath — путь конфига, который юнит называет ПОСЛЕ снятия
	// ("" — перечитать юнит не вышло).
	UnitConfigPath string
	Warning        string
}

// RemoveConfigDropIn снимает наш drop-in: удаляет файл, перечитывает юниты и
// пересверяет состояние по факту юнита. Обратная операция к WriteConfigDropIn —
// без неё override снимался только руками, а его происхождение забывалось.
//
// Расхождение путей после снятия — ожидаемый исход (юнит возвращается к своему
// ExecStart), поэтому состояние обновляется здесь же: возвращённое расхождение
// обязано быть видно сразу, а не после перезапуска панели.
func (m *Manager) RemoveConfigDropIn() (DropInRemoval, error) {
	path := m.dropInPath()
	if path == "" {
		return DropInRemoval{}, ErrNoConfigDropIn
	}
	err := os.Remove(path)
	// Каталог <unit>.service.d наш только когда он опустел: os.Remove на
	// непустом каталоге откажет, и это ровно то, что нужно — чужие drop-in'ы
	// (и их каталог) не наше дело.
	//
	// Убираем его и когда файла уже не было: его могли снести руками, а «Снять»
	// нажать после — и тогда ранний возврат «нечего снимать» оставлял пустой
	// каталог в /etc/systemd/system навсегда. Создавал его RouteBox, убрать его
	// больше некому.
	_ = os.Remove(filepath.Dir(path))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DropInRemoval{}, ErrNoConfigDropIn
		}
		return DropInRemoval{}, dropInWriteError("remove drop-in", path, err)
	}

	var res DropInRemoval
	if err := m.reloadSystemd(); err != nil {
		// Файла нет, а юнит до успешного daemon-reload продолжает работать с
		// прежним override: снятие состоялось наполовину, и знать об этом надо.
		res.Warning = fmt.Sprintf("the drop-in file is deleted, but %v — until daemon-reload succeeds the unit keeps running with the override", err)
		return res, nil
	}

	unit, err := m.readUnitConfigPath()
	if err != nil {
		// Куда вернулся юнит — неизвестно. Придумывать нельзя: снятие
		// состоялось, проверка — нет.
		res.Warning = fmt.Sprintf("the drop-in is removed, but re-reading the unit failed (%v), so the config path the unit uses now is unknown", err)
		return res, nil
	}

	m.stateMu.RLock()
	ours := m.configPath
	m.stateMu.RUnlock()
	m.SetConfigPaths(ours, unit)

	res.UnitConfigPath = unit
	return res, nil
}

// configPathFromExecStart достаёт путь конфига из вывода
// `systemctl show --property=ExecStart`. Это тот же разбор argv, которым
// пользуется починка: детектор расхождения и чинилка обязаны видеть юнит
// одинаково, иначе разъедутся. "" — путь не разобран.
//
// Цена ошибки здесь выше, чем в чинилке: расхождение блокирует Start, Restart
// и Reload, так что неверно разобранный путь глушит кнопки на ровном месте.
// Поэтому строковый поиск "-c " не годится: он ловит и хвост чужого флага
// (--workdir-c), и не видит ни закавыченного argv, ни формы --config=.
func configPathFromExecStart(showOutput string) string {
	return configPathFromArgv(parseExecStartArgv(showOutput))
}

// configPathFromArgv возвращает путь конфига по разобранному argv.
func configPathFromArgv(argv []string) string {
	ca, ok := findConfigArg(argv)
	if !ok || ca.value == "" {
		return ""
	}
	if !ca.directory {
		return ca.value
	}
	// Режим каталога: sing-box читает <dir>/config.json. Поведение осознанное и
	// сохранено как было — путь отдаём только если файл действительно есть.
	return existingConfigInDir(ca.value)
}

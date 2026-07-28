package process

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDropInContent(t *testing.T) {
	got := dropInContent([]string{"/usr/local/bin/amnezia-box", "run", "-c", "/etc/amnezia-box/config.json"})
	want := "[Service]\nExecStart=\nExecStart=/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// Пустая строка ExecStart= обязана идти перед новой командой: без неё systemd
// ДОБАВИТ вторую команду вместо замены первой, и amnezia-box поднимется дважды.
func TestDropInContentResetsExecStartFirst(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(dropInContent([]string{"/bin/box", "run", "-c", "/etc/c.json"}), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(lines), lines)
	}
	if lines[0] != "[Service]" {
		t.Errorf("line 0 = %q, want [Service]", lines[0])
	}
	if lines[1] != "ExecStart=" {
		t.Errorf("line 1 = %q, want the empty ExecStart= reset", lines[1])
	}
	if !strings.HasPrefix(lines[2], "ExecStart=/bin/box ") {
		t.Errorf("line 2 = %q, want the replacement command", lines[2])
	}
}

// `systemctl show` печатает ExecStart с УЖЕ раскрытыми спецификаторами, значит
// '%' в чужом аргументе — литеральный символ. Записанный в drop-in как есть, он
// раскроется ещё раз (%i, %H, %%…) и молча подменит чужой аргумент — а чужие
// аргументы мы обязаны сохранять дословно. Отсюда %→%% на записи, а не отказ
// чинить юнит: отказ оставил бы пользователя с расхождением и без кнопки.
func TestJoinArgvEscapesPercent(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "bare percent in a foreign argument",
			argv: []string{"/usr/bin/box", "--tag=50%off", "-c", "/etc/c.json"},
			want: "/usr/bin/box --tag=50%%off -c /etc/c.json",
		},
		{
			name: "percent inside a quoted argument",
			argv: []string{"/usr/bin/box", "--ua", "Mozilla 100%", "-c", "/etc/c.json"},
			want: `/usr/bin/box --ua "Mozilla 100%%" -c /etc/c.json`,
		},
		{
			name: "nothing to escape",
			argv: []string{"/usr/bin/box", "run", "-c", "/etc/c.json"},
			want: "/usr/bin/box run -c /etc/c.json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinArgv(tc.argv); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDropInPathForService(t *testing.T) {
	got := dropInPath(defaultSystemdRoot, "amnezia-box")
	want := "/etc/systemd/system/amnezia-box.service.d/10-routebox-config.conf"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// argv юнита читаем из того же `systemctl show`, что и путь конфига.
func TestParseExecStartArgv(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{
			name: "plain",
			out:  "ExecStart={ path=/usr/local/bin/amnezia-box ; argv[]=/usr/local/bin/amnezia-box run -c /etc/amnezia-box/config.json ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] }\n",
			want: []string{"/usr/local/bin/amnezia-box", "run", "-c", "/etc/amnezia-box/config.json"},
		},
		{
			name: "quoted argv (newer systemd)",
			out:  "ExecStart={ path=/usr/bin/sing-box ; argv[]=\"/usr/bin/sing-box\" \"run\" \"-c\" \"/etc/sing-box/config.json\" ; ignore_errors=no }\n",
			want: []string{"/usr/bin/sing-box", "run", "-c", "/etc/sing-box/config.json"},
		},
		{
			name: "extra flags kept in order",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box --disable-color run --log-level debug -c /etc/box/config.json ; ignore_errors=no }",
			want: []string{"/usr/bin/box", "--disable-color", "run", "--log-level", "debug", "-c", "/etc/box/config.json"},
		},
		{name: "no argv", out: "ExecStart={ path=/usr/bin/box ; ignore_errors=no }", want: nil},
		{name: "empty", out: "", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseExecStartArgv(tc.out)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Извлечение пути конфига из ExecStart общее у детектора расхождения и у
// чинилки: разъехаться они больше не могут. Кейсы ниже — ровно те, на которых
// прежний строковый поиск "-c " ошибался, а цена ошибки у детектора выше:
// неверно разобранный путь блокирует Start/Restart/Reload на ровном месте.
func TestConfigPathFromExecStart(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "plain",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run -c /etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			// Наивный поиск "-c " цеплялся за хвост чужого флага и возвращал /var/lib.
			name: "argument ending in -c is not the config flag",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run --workdir-c /var/lib -c /etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			// В закавыченном argv нет подстроки "-c ": наивный поиск возвращал "".
			name: "quoted argv",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=\"/usr/bin/box\" \"run\" \"-c\" \"/etc/amnezia-box/config.json\" ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			// Inline-форму наивный поиск не видел вовсе.
			name: "inline value",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run --config=/etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "long flag",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run --config /etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			// Слитную форму понимал только разбор /proc/<pid>/cmdline. Здесь она
			// возвращала "" — то есть «в юните нет пути конфига», то есть
			// расхождения нет и сверка молча выключена целиком.
			name: "joined short flag",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run -c/etc/amnezia-box/config.json ; ignore_errors=no }",
			want: "/etc/amnezia-box/config.json",
		},
		{
			name: "no config flag",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run -D /var/lib/box ; ignore_errors=no }",
			want: "",
		},
		{
			name: "flag without a value",
			out:  "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run -c ; ignore_errors=no }",
			want: "",
		},
		{name: "unparseable", out: "ExecStart={ path=/usr/bin/box ; ignore_errors=no }", want: ""},
		{name: "empty", out: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configPathFromExecStart(tc.out); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Режим каталога сохранён как был: <dir>/config.json, и только если файл есть.
func TestConfigPathFromExecStartConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	out := "ExecStart={ path=/usr/bin/box ; argv[]=/usr/bin/box run -C " + dir + " ; ignore_errors=no }"

	if got := configPathFromExecStart(out); got != "" {
		t.Fatalf("no config.json in the directory yet, want %q, got %q", "", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := configPathFromExecStart(out), filepath.Join(dir, "config.json"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Меняем ТОЛЬКО значение конфига. Бинарь, подкоманда и любые прочие аргументы
// обязаны пережить починку: молча срезать --disable-color или -D нельзя.
func TestRetargetExecStartKeepsOtherArgs(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "short flag",
			argv: []string{"/usr/bin/box", "run", "-c", "/opt/old.json"},
			want: []string{"/usr/bin/box", "run", "-c", "/etc/amnezia-box/config.json"},
		},
		{
			name: "long flag and neighbours",
			argv: []string{"/usr/bin/box", "--disable-color", "run", "--config", "/opt/old.json", "--log-level", "debug"},
			want: []string{"/usr/bin/box", "--disable-color", "run", "--config", "/etc/amnezia-box/config.json", "--log-level", "debug"},
		},
		{
			name: "inline value",
			argv: []string{"/usr/bin/box", "run", "--config=/opt/old.json", "-D", "/var/lib/box"},
			want: []string{"/usr/bin/box", "run", "--config=/etc/amnezia-box/config.json", "-D", "/var/lib/box"},
		},
		{
			name: "joined short flag keeps its form",
			argv: []string{"/usr/bin/box", "run", "-c/opt/old.json"},
			want: []string{"/usr/bin/box", "run", "-c/etc/amnezia-box/config.json"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := retargetExecStart(tc.argv, "/etc/amnezia-box/config.json")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Юнит, запущенный через -C <каталог>, остаётся в режиме каталога: переключить
// его на -c <файл> — сменить способ запуска, а не починить путь.
func TestRetargetExecStartKeepsConfigDirectoryMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "-C",
			argv: []string{"/usr/bin/box", "run", "-C", "/opt/etc/box"},
			want: []string{"/usr/bin/box", "run", "-C", "/etc/amnezia-box"},
		},
		{
			name: "--config-directory=",
			argv: []string{"/usr/bin/box", "run", "--config-directory=/opt/etc/box"},
			want: []string{"/usr/bin/box", "run", "--config-directory=/etc/amnezia-box"},
		},
		{
			name: "joined -C",
			argv: []string{"/usr/bin/box", "run", "-C/opt/etc/box"},
			want: []string{"/usr/bin/box", "run", "-C/etc/amnezia-box"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := retargetExecStart(tc.argv, "/etc/amnezia-box/config.json")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Нестандартный юнит — повод отказаться, а не гадать.
func TestRetargetExecStartRefusesGarbage(t *testing.T) {
	for _, argv := range [][]string{
		nil,
		{},
		{"/usr/bin/box", "run", "-c"},       // флаг без значения
		{"/usr/bin/box", "run", "--config"}, // то же самое, длинная форма
		{"/usr/bin/box", "run", "-C"},       // и в режиме каталога
	} {
		if got, err := retargetExecStart(argv, "/etc/amnezia-box/config.json"); err == nil {
			t.Errorf("argv %q: want an error, got %q", argv, got)
		}
	}
}

// Юнит без флага конфига чинить нечем: дописать -c значило бы сменить способ
// запуска (юнит мог жить на -D <workdir> или на встроенном пути), а не поправить
// путь. Практически сюда не доходят — такой юнит не даёт расхождения, а без
// расхождения хендлер отвечает 409, — но отказ честнее выдумывания флага, если
// вызывающий когда-нибудь появится.
func TestRetargetExecStartRefusesUnitWithoutAConfigFlag(t *testing.T) {
	argv := []string{"/usr/bin/box", "run", "-D", "/var/lib/box"}
	got, err := retargetExecStart(argv, "/etc/amnezia-box/config.json")
	if err == nil {
		t.Fatalf("want an error, got %q", got)
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("error %q must say what is missing", err.Error())
	}
}

// Путь едет в системный юнит: перевод строки впишет туда произвольные
// директивы, пробел развалит аргумент, % systemd раскроет как спецификатор.
func TestValidateUnitPathRejectsUnsafe(t *testing.T) {
	bad := map[string]string{
		"space":    "/etc/amnezia box/config.json",
		"tab":      "/etc/amnezia\tbox/config.json",
		"newline":  "/etc/config.json\nExecStartPost=/bin/rm -rf /",
		"carriage": "/etc/config.json\rExecStartPost=/bin/false",
		"percent":  "/etc/%H/config.json",
		"relative": "etc/amnezia-box/config.json",
		"bare":     "sing-box",
		"empty":    "",
	}
	for name, p := range bad {
		t.Run(name, func(t *testing.T) {
			err := validateUnitPath("config path", p)
			if err == nil {
				t.Fatalf("%q must be rejected", p)
			}
			if !strings.Contains(err.Error(), "config path") {
				t.Errorf("error %q must name what was rejected", err.Error())
			}
		})
	}
	if err := validateUnitPath("config path", "/etc/amnezia-box/config.json"); err != nil {
		t.Fatalf("a normal path must pass, got %v", err)
	}
}

// Отказ по правам — самый вероятный исход не под root. Голое "permission denied"
// пользователю ничего не говорит, поэтому сообщение обязано назвать причину.
func TestDropInPermissionErrorExplainsRoot(t *testing.T) {
	err := dropInWriteError("create drop-in dir", "/etc/systemd/system/x.service.d", fs.ErrPermission)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, want := range []string{"create drop-in dir", "/etc/systemd/system/x.service.d", "root"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q must mention %q", msg, want)
		}
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Error("the underlying error must stay unwrappable")
	}

	other := dropInWriteError("write drop-in", "/etc/x.conf", errors.New("disk full"))
	if strings.Contains(other.Error(), "root") {
		t.Errorf("non-permission errors must not blame root: %q", other.Error())
	}
	if !strings.Contains(other.Error(), "disk full") {
		t.Errorf("non-permission errors must keep the cause: %q", other.Error())
	}
}

// После записи drop-in состояние пересверяется по факту юнита. Если systemd
// файл не подхватил, расхождение обязано остаться, а пользователь — узнать.
func TestVerifyUnitRetargetedKeepsMismatchWhenUnitDidNotChange(t *testing.T) {
	m := &Manager{unitConfigReader: func() (string, error) { return "/opt/etc/sing-box/config.json", nil }}
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	err := m.verifyUnitRetargeted("/etc/amnezia-box/config.json", "/etc/systemd/system/x.service.d/10-routebox-config.conf")
	if err == nil {
		t.Fatal("unit still points elsewhere: verification must fail")
	}
	if !strings.Contains(err.Error(), "/opt/etc/sing-box/config.json") {
		t.Errorf("error %q must name what the unit still uses", err.Error())
	}
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Error("the mismatch must survive a failed fix, not be cleared by wishful thinking")
	}
}

func TestVerifyUnitRetargetedClearsMismatchWhenUnitFollowed(t *testing.T) {
	m := &Manager{unitConfigReader: func() (string, error) { return "/etc/amnezia-box//config.json", nil }}
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	if err := m.verifyUnitRetargeted("/etc/amnezia-box/config.json", "/etc/systemd/system/x.service.d/10-routebox-config.conf"); err != nil {
		t.Fatalf("unit now uses our config, want no error, got %v", err)
	}
	if _, _, ok := m.ConfigMismatch(); ok {
		t.Error("mismatch must be gone once the unit actually followed")
	}
}

// «Не смогли проверить» — это не «починено». Если systemctl упал сразу после
// daemon-reload, расхождение снимать нельзя, а пользователю надо сказать, что
// drop-in записан, но подтвердить результат не вышло.
func TestVerifyUnitRetargetedKeepsMismatchWhenUnitCannotBeRead(t *testing.T) {
	m := &Manager{unitConfigReader: func() (string, error) {
		return "", errors.New("systemctl show: exit status 1")
	}}
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	err := m.verifyUnitRetargeted("/etc/amnezia-box/config.json", "/etc/systemd/system/x.service.d/10-routebox-config.conf")
	if err == nil {
		t.Fatal("an unreadable unit must not be reported as fixed")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("error %q must carry why the check failed", err.Error())
	}
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Error("a failed check must leave the mismatch in place")
	}
}

// Юнит прочитался, но конфига в ExecStart нет вовсе — тоже не повод объявлять
// починку удавшейся: SetConfigPaths(ours, "") молча снял бы расхождение.
func TestVerifyUnitRetargetedKeepsMismatchWhenUnitNamesNoConfig(t *testing.T) {
	m := &Manager{unitConfigReader: func() (string, error) { return "", nil }}
	m.SetConfigPaths("/etc/amnezia-box/config.json", "/opt/etc/sing-box/config.json")

	if err := m.verifyUnitRetargeted("/etc/amnezia-box/config.json", "/etc/x.conf"); err == nil {
		t.Fatal("a unit that names no config path must not count as fixed")
	}
	if _, _, ok := m.ConfigMismatch(); !ok {
		t.Error("the mismatch must survive")
	}
}

func TestWriteConfigDropInRefusesWithoutUnit(t *testing.T) {
	// Отсутствие юнита проверяется САМИМ кодом, поэтому корень тоже временный:
	// иначе регресс в dropInPath (пустое имя юнита → "/etc/systemd/system/
	// .service.d/…") превратил бы этот тест в запись в /etc.
	m := NewManagerForTest("", t.TempDir())
	err := m.WriteConfigDropIn("/etc/amnezia-box/config.json")
	if err == nil {
		t.Fatal("without a systemd unit there is nothing to fix")
	}
	if !strings.Contains(err.Error(), "systemd") {
		t.Errorf("error should say there is no unit, got %q", err.Error())
	}
}

// Валидация пути обязана сработать ДО любого обращения к диску и к systemd.
//
// Менеджер здесь отрезан от машины намеренно: сам по себе этот тест был защищён
// ровно той валидацией, которую и проверяет, — при её регрессе на роутере под
// root он дописал бы drop-in в настоящий /etc/systemd/system и дёрнул настоящий
// daemon-reload. Тест, чья безопасность держится на проверяемом им же коде, не
// защищён ничем.
func TestWriteConfigDropInRefusesUnsafePathBeforeTouchingDisk(t *testing.T) {
	m := NewManagerForTest("amnezia-box", t.TempDir())
	err := m.WriteConfigDropIn("/etc/amnezia-box/config.json\nExecStartPost=/bin/rm -rf /")
	if err == nil {
		t.Fatal("a path with a newline must never reach /etc")
	}
	if !strings.Contains(err.Error(), "config path") {
		t.Errorf("error %q must name the rejected input", err.Error())
	}
}

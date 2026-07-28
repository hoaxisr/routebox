package process

// NewManagerForTest собирает менеджер, полностью отрезанный от этой машины:
// каталог юнитов — переданный (обычно t.TempDir()), daemon-reload — пустышка,
// чтение ExecStart и пути конфига юнита — «юнита нет», поиск живого процесса —
// «не запущен».
//
// Про процесс — не мелочь: менеджер, отрезанный только от systemd, всё равно
// нашёл бы в /proc боевой amnezia-box и отдал Running=true, а хендлер вроде
// ApplyConfig на этом дошёл бы до Reload — SIGHUP настоящему процессу прямо из
// `go test`.
//
// Нужен другим пакетам: швы менеджера неэкспортированы, а тест API-хендлера с
// настоящим process.NewManager() детектит РЕАЛЬНЫЙ юнит — и на роутере, где
// RouteBox действительно держит drop-in, обычный `go test ./backend/...` под
// root снёс бы боевой /etc/systemd/system/<unit>.service.d/10-routebox-config.conf
// и выполнил настоящий daemon-reload. Тест, ломающий продакшн при обычном
// прогоне, — не мелочь, поэтому шов доведён до экспортированного конструктора.
//
// В боевом коде не использовать: там менеджер собирается NewManager().
func NewManagerForTest(serviceName, systemdRoot string) *Manager {
	// «Юнита нет» и «юнит ничего не называет» — разные состояния и в бою, и для
	// вызывающих (первое отличается ErrNoSystemdUnit). Пустое имя означает
	// первое; иначе шов вернул бы «юнит есть, но пустой» там, где юнита нет.
	noUnit := func() (string, error) {
		if serviceName == "" {
			return "", ErrNoSystemdUnit
		}
		return "", nil
	}
	return &Manager{
		serviceName:         serviceName,
		systemdRoot:         systemdRoot,
		daemonReload:        func() error { return nil },
		unitExecStartReader: noUnit,
		unitConfigReader:    noUnit,
		pidFinder:           func() int { return 0 },
	}
}

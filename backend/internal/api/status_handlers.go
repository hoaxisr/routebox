package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"routebox/backend/internal/process"
)

// GetVersion returns the sing-box version, feature flags and RouteBox version
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	version, err := h.process.GetVersion()
	if err != nil {
		version = ""
	}
	features := h.process.GetFeatureFlags()
	writeSuccess(w, map[string]interface{}{
		"version":          version,
		"features":         features,
		"routebox_version": h.routeboxVersion,
	})
}

// statusResponse is the process status plus the on-disk facts the panel needs on
// every poll. They belong here rather than in process.Status: they describe
// files, not the process.
type statusResponse struct {
	process.Status
	// ConfigReadOnly is specifically about the sing-box config, and stays
	// specific on purpose: it is what greys out the config save buttons, and an
	// unwritable users.toml must not disable config editing.
	ConfigReadOnly bool `json:"config_read_only"`
	// ConfigReadOnlyPath names the config file to make writable; empty unless
	// the config is read-only.
	ConfigReadOnlyPath string `json:"config_read_only_path,omitempty"`
	// ReadOnlyPaths is every file RouteBox knows it cannot write — the config
	// among them. This is what the badge stands for: the state RouteBox persists
	// is spread over three directories that can be mounted separately, so a
	// single boolean about the config was never the whole answer, and a single
	// boolean about all of them would not tell the operator what to fix.
	//
	// "Knows", not "currently": the two directions are not symmetric, and the
	// wording used to promise the symmetry the code does not have.
	//
	//   read-only -> writable is noticed by the poll. Each guard re-probes its
	//   own standing negative verdict (rate-limited), so remounting rw clears
	//   the badge within a couple of seconds and no restart is needed.
	//
	//   writable -> read-only is noticed by the next write. Nothing here probes
	//   a path that was writable last time it was asked, so a mount that goes ro
	//   under a running RouteBox stays invisible until something tries to save —
	//   which then fails with a 409 naming the file, and the path appears here.
	//
	// The asymmetry is the price of the probe: it works by creating and removing
	// a file, and this endpoint is polled every few seconds. Probing six healthy
	// paths on every poll would mean thousands of pointless writes an hour on a
	// flash-backed router, forever, to detect a state that is almost never true
	// and that the next save reports anyway.
	ReadOnlyPaths []string `json:"read_only_paths,omitempty"`
}

// stateFile is a file RouteBox persists and can therefore be refused.
type stateFile interface {
	IsReadOnly() bool
	GetPath() string
}

// readOnlyPaths collects every state file whose guard says it cannot be written
// (see ReadOnlyPaths for what that verdict does and does not see), sorted and
// deduplicated — several stores share one directory, but each has its own file,
// so duplicates only appear if two managers are pointed at one path.
//
// The stores are listed explicitly rather than discovered: a store that is
// missing from this list is a file whose read-only state the panel never shows,
// and that has to be a visible omission in one place.
func (h *Handler) readOnlyPaths() []string {
	var stores []stateFile
	if h.config != nil {
		stores = append(stores, h.config)
	}
	if h.settings != nil {
		stores = append(stores, h.settings)
	}
	if h.clients != nil {
		stores = append(stores, h.clients)
	}
	if h.panelUsers != nil {
		stores = append(stores, h.panelUsers)
	}
	if h.subs != nil {
		stores = append(stores, h.subs)
	}
	if h.awg != nil {
		stores = append(stores, h.awg.Store())
	}
	if h.mtproto != nil {
		stores = append(stores, h.mtproto.Store())
	}

	var paths []string
	for _, s := range stores {
		if p := s.GetPath(); p != "" && s.IsReadOnly() {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// GetStatus returns amnezia-box process status.
//
// When the process is running, the reported Version is overlaid with the
// running process's self-reported version (via the Clash API /version endpoint)
// so swapping the on-disk binary without a restart does not mislabel the running
// process. When the process is not running, or the Clash query fails, the
// on-disk binary version (from process.GetStatus) is used unchanged.
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := h.getProcessStatus()
	if status.Running {
		if v, ok := runningSingboxVersion(h.getClashAddr(), h.getClashSecret()); ok {
			status.Version = v
		}
	}

	// SystemChecks answer one question — can this host route a LAN through a TUN
	// interface — and a VPS panel never does that. Reported there, they render a
	// red "System Requirements Not Met / run routebox with sudo" banner on a
	// panel that is working exactly as designed, telling the operator to undo
	// the very thing that lets it run unprivileged.
	if h.panelMode == "vps" {
		status.SystemChecks = nil
	}

	resp := statusResponse{Status: status, ReadOnlyPaths: h.readOnlyPaths()}
	if h.config != nil && h.config.IsReadOnly() {
		resp.ConfigReadOnly = true
		resp.ConfigReadOnlyPath = h.config.GetPath()
	}
	writeSuccess(w, resp)
}

// NeedsSetup checks if the setup wizard should be shown
// Returns needs_setup=true if:
// 1. sing-box binary is not installed, OR
// 2. sing-box is not running AND config has no VPN (vless/hy2 outbound or awg endpoint), OR
// 3. sing-box is running AND config has no VPN configuration
func (h *Handler) NeedsSetup(w http.ResponseWriter, r *http.Request) {
	binaryInstalled := h.process.IsBinaryInstalled()
	status := h.process.GetStatus()
	hasVpnConfig := h.config.HasVpnConfig()

	needsSetup := false
	reason := ""

	if !binaryInstalled {
		needsSetup = true
		reason = "binary_not_installed"
	} else if !status.Running && !hasVpnConfig {
		needsSetup = true
		reason = "not_running_no_vpn"
	} else if status.Running && !hasVpnConfig {
		needsSetup = true
		reason = "running_no_vpn"
	}

	writeSuccess(w, map[string]interface{}{
		"needs_setup":          needsSetup,
		"reason":               reason,
		"binary_installed":     binaryInstalled,
		"running":              status.Running,
		"has_vpn_config":       hasVpnConfig,
		"clash_api_configured": h.getClashAddr() != "",
		"version":              status.Version,
		"binary_path":          status.BinaryPath,
	})
}

// writeControlError answers a Start/Restart/Reload failure.
//
// A config path mismatch is the same kind of answer a read-only store gets (see
// readonly.go): the request was fine, the state forbids it, and the state is one
// the user resolves from the banner — that is 409. Anything else really is a
// failure to carry the request out, and keeps its 500.
func writeControlError(w http.ResponseWriter, err error) {
	if errors.Is(err, process.ErrConfigPathMismatch) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// Start starts the amnezia-box process
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Start(h.config.GetPath()); err != nil {
		writeControlError(w, err)
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Started",
	})
}

// Stop stops the amnezia-box process
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]string{"message": "Stopped"})
}

// Restart restarts the amnezia-box process
func (h *Handler) Restart(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Restart(h.config.GetPath()); err != nil {
		writeControlError(w, err)
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message": "Restarted",
	})
}

// Reload sends SIGHUP to reload config without restarting
func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.process.Reload(); err != nil {
		writeControlError(w, err)
		return
	}

	writeSuccess(w, map[string]string{"message": "Configuration reloaded"})
}

// AdoptUnitConfigPath переводит RouteBox на путь конфига из ExecStart юнита —
// одно из двух лечений расхождения с юнитом (второе, FixUnitConfigPath, двигает
// юнит на наш путь).
//
// Путь читается здесь и сейчас, а состояние меняется только после успешного
// чтения: переключиться, не зная, что в юните, значит объявить расхождение
// разрешённым, ничего не проверив. Переходим именно на путь ЮНИТА, а не на путь
// живого процесса: тот может оказаться третьим файлом, и переход на него
// оставил бы расхождение с юнитом стоять — с заблокированными
// Start/Restart/Reload и баннером на всех страницах.
func (h *Handler) AdoptUnitConfigPath(w http.ResponseWriter, r *http.Request) {
	unitPath, err := h.process.ReadUnitConfigPath()
	switch {
	case errors.Is(err, process.ErrNoSystemdUnit):
		writeError(w, http.StatusConflict, "there is no systemd unit here to adopt a config path from")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not read the systemd unit: %v", err))
		return
	case unitPath == "":
		writeError(w, http.StatusConflict, "the systemd unit's ExecStart names no config path")
		return
	}

	if err := h.config.SetPath(unitPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load config from %s: %v", unitPath, err))
		return
	}

	// Наш путь и путь юнита теперь совпадают по построению — расхождение с
	// юнитом снято, и Start/Restart/Reload разблокированы без перезапуска
	// RouteBox. С живым процессом расхождение может остаться: он читает файл, с
	// которым его запустили, — это видно в статусе и лечится перезапуском.
	h.process.SetConfigPaths(unitPath, unitPath)

	// Закрепляем путь в настройках, иначе лечение живёт до первого перезапуска
	// самой панели: при старте путь берётся из singbox.config_path, и явно
	// заданный там старый файл вернул бы расхождение. Кнопка подаётся как
	// лечение — обещать больше, чем сделано, здесь нельзя.
	//
	// Настройки могут быть недоступны на запись, и это НЕ повод откатывать
	// переход: RouteBox уже правит новый файл, расхождение с юнитом уже снято.
	// Поэтому успех с warning'ом, а не ошибка — но молчать нельзя.
	//
	// В warning уходит только ФАКТ (что не записалось и почему). Следствие —
	// «после перезапуска RouteBox вернётся к прежнему пути, починку придётся
	// повторить» — живёт в локализованном тексте панели, в единственном
	// экземпляре и на языке пользователя.
	resp := map[string]interface{}{
		"message": "Switched to the config path from the systemd unit",
		"path":    unitPath,
	}
	if h.settings == nil {
		// Ошибка сборки RouteBox, а не состояние установки: main всегда передаёт
		// менеджер настроек. Промолчать здесь значило бы выдать наполовину
		// сделанное лечение за целое, поэтому случай называется вслух.
		resp["warning"] = "no settings manager is wired up, so the new config path was not saved"
	} else if err := h.settings.SetSingboxConfigPath(unitPath); err != nil {
		resp["warning"] = err.Error()
	}

	writeSuccess(w, resp)
}

// GetJournalLogs returns recent logs from systemd journal
func (h *Handler) GetJournalLogs(w http.ResponseWriter, r *http.Request) {
	linesParam := r.URL.Query().Get("lines")
	lines := 50
	if linesParam != "" {
		if n, err := fmt.Sscanf(linesParam, "%d", &lines); err != nil || n != 1 {
			lines = 50
		}
	}
	if lines > 500 {
		lines = 500
	}

	logs, err := h.process.GetJournalLogs(lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, map[string]interface{}{
		"logs":  logs,
		"lines": lines,
	})
}

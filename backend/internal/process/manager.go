package process

import (
	"context"
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
	ServiceName  string        `json:"service_name,omitempty"`  // systemd service name if applicable
	ConfigPath   string        `json:"config_path,omitempty"`   // detected config path
	SupportsHUP  bool          `json:"supports_hup"`            // whether SIGHUP reload is supported
	Version      string        `json:"version,omitempty"`       // binary version string
	BinaryPath   string        `json:"binary_path,omitempty"`   // path to binary
	SystemChecks *SystemChecks `json:"system_checks,omitempty"` // system requirements check
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

	binaryPath      string
	configPath      string
	serviceName     string // detected systemd service name (set once in NewManager)
	forceStandalone bool   // force standalone mode even if systemd service exists
	startedPID      int    // PID of process started by us in standalone mode (0 if none)

	cachedVersion      string    // memoized parsed version (e.g. "1.13.13")
	cachedVersionPath  string    // binary path the cached version belongs to
	cachedVersionStamp time.Time // binary mtime at cache fill
	cachedVersionSize  int64     // binary size at cache fill

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
	return m
}

// SetConfigPath sets the config path for the process
func (m *Manager) SetConfigPath(path string) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.configPath = path
}

// SetForceStandalone enables standalone mode even if systemd service exists
// Use this when running with a local config that differs from systemd config
func (m *Manager) SetForceStandalone(force bool) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.forceStandalone = force
}

// IsForceStandalone returns true if standalone mode is forced
func (m *Manager) IsForceStandalone() bool {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.forceStandalone
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

// detectSystemdService checks if sing-box is managed by systemd
func (m *Manager) detectSystemdService() string {
	// Check common service names
	serviceNames := []string{
		"sing-box",
		"amnezia-box",
		"sing-box@config",
	}

	for _, name := range serviceNames {
		cmd := exec.Command("systemctl", "is-enabled", name+".service")
		if err := cmd.Run(); err == nil {
			return name
		}
		// Also check if it's active even if not enabled
		cmd = exec.Command("systemctl", "is-active", name+".service")
		if err := cmd.Run(); err == nil {
			return name
		}
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

// getConfigFromProcess extracts config path from running process cmdline
func (m *Manager) getConfigFromProcess() string {
	pid := m.findPID()
	if pid == 0 {
		return ""
	}

	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return ""
	}

	// cmdline is null-separated
	args := strings.Split(string(data), "\x00")

	// Look for -c or --config flag (direct config file path)
	// Note: -C is config DIRECTORY in sing-box, handled separately below
	for i, arg := range args {
		if (arg == "-c" || arg == "--config") && i+1 < len(args) {
			configPath := args[i+1]
			if configPath != "" {
				// Resolve to absolute path if relative
				if !filepath.IsAbs(configPath) {
					// Try to get working directory
					cwdPath := fmt.Sprintf("/proc/%d/cwd", pid)
					if cwd, err := os.Readlink(cwdPath); err == nil {
						configPath = filepath.Join(cwd, configPath)
					}
				}
				return configPath
			}
		}
		// Handle -c/path/to/config format (no space)
		if strings.HasPrefix(arg, "-c") && len(arg) > 2 && arg[1] != 'C' {
			return arg[2:]
		}
	}

	// Look for -C (config directory) pattern used by official sing-box service
	// -C specifies directory containing config.json
	var workDir, configDir string
	for i, arg := range args {
		if arg == "-D" && i+1 < len(args) {
			workDir = args[i+1]
		}
		if arg == "-C" && i+1 < len(args) {
			configDir = args[i+1]
		}
		// Handle -C/path format (no space)
		if strings.HasPrefix(arg, "-C") && len(arg) > 2 {
			configDir = arg[2:]
		}
	}
	if configDir != "" {
		// Check for config.json in config directory
		configPath := filepath.Join(configDir, "config.json")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}
	if workDir != "" {
		configPath := filepath.Join(workDir, "config.json")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	return ""
}

// getConfigFromSystemd extracts config path from systemd service file
func (m *Manager) getConfigFromSystemd() string {
	if m.serviceName == "" {
		return ""
	}

	// Use systemctl show to get ExecStart
	cmd := exec.Command("systemctl", "show", m.serviceName+".service", "--property=ExecStart")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	line := strings.TrimSpace(string(output))
	// Parse ExecStart line to find -c flag
	if idx := strings.Index(line, "-c "); idx >= 0 {
		rest := line[idx+3:]
		// Extract path (ends at space or semicolon or end)
		endIdx := strings.IndexAny(rest, " ;")
		if endIdx == -1 {
			endIdx = len(rest)
		}
		return strings.TrimSpace(rest[:endIdx])
	}

	// Try -C flag (config directory)
	if idx := strings.Index(line, "-C "); idx >= 0 {
		rest := line[idx+3:]
		endIdx := strings.IndexAny(rest, " ;")
		if endIdx == -1 {
			endIdx = len(rest)
		}
		configDir := strings.TrimSpace(rest[:endIdx])
		configPath := filepath.Join(configDir, "config.json")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	return ""
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

// GetStatus returns the current process status
func (m *Manager) GetStatus() Status {
	// Get version info (cached in binaryPath if successful)
	version, _ := m.GetVersion()
	bp := m.getBinaryPath()

	// Always include system checks
	systemChecks := GetSystemChecks()

	pid := m.findPID()
	if pid == 0 {
		status := Status{
			Running:      false,
			SupportsHUP:  true,
			Version:      version,
			BinaryPath:   bp,
			SystemChecks: systemChecks,
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
		return Status{Running: false, SupportsHUP: true, Version: version, BinaryPath: bp}
	}

	// On Unix, FindProcess always succeeds. Need to send signal 0 to check.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return Status{Running: false, SupportsHUP: true, Version: version, BinaryPath: bp}
	}

	// Get uptime from /proc
	uptime := m.getUptime(pid)

	// Detect management type and config
	managedBy := "standalone"
	serviceName := ""
	if m.IsSystemdManaged() {
		managedBy = "systemd"
		serviceName = m.serviceName
	}

	configPath := m.getConfigFromProcess()

	return Status{
		Running:      true,
		PID:          pid,
		Uptime:       uptime,
		ManagedBy:    managedBy,
		ServiceName:  serviceName,
		ConfigPath:   configPath,
		SupportsHUP:  true, // sing-box supports SIGHUP
		Version:      version,
		BinaryPath:   bp,
		SystemChecks: systemChecks,
	}
}

// findPID finds the PID of the running amnezia-box/sing-box process by
// resolving /proc/<pid>/exe. This cannot be confused with processes that
// merely mention the binary name in their arguments (e.g. RouteBox itself
// started with "-config /opt/etc/sing-box/config.json").
func (m *Manager) findPID() int {
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
	return m.startLocked(configPath)
}

// startLocked starts the process. Caller must hold opMu.
func (m *Manager) startLocked(configPath string) error {
	if m.GetStatus().Running {
		return fmt.Errorf("amnezia-box is already running")
	}

	// If systemd service exists and standalone mode is not forced, use systemctl
	if m.serviceName != "" && !m.IsForceStandalone() {
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

	// If managed by systemd and standalone mode is not forced, use systemctl
	if m.IsSystemdManaged() && !m.IsForceStandalone() {
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

	// If managed by systemd and standalone mode is not forced, use systemctl restart
	if m.IsSystemdManaged() && !m.IsForceStandalone() {
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

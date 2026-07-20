package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Manager handles config file operations with draft/staged editing support.
// Changes are accumulated in a draft file (config.json.bak) and only applied
// to the active config (config.json) when explicitly saved or applied.
type Manager struct {
	path          string                 // config.json path
	draftPath     string                 // config.json.bak path
	activeConfig  map[string]interface{} // config on disk (read-only during editing)
	draftConfig   map[string]interface{} // draft config (nil if no changes)
	hasDraft      bool
	draftGen      int64 // generation counter, bumped on every draft mutation (guarded by mu)
	mu            sync.RWMutex
	readOnly      bool
	checkBinaryFn func() string // provider for the `check` validation binary (re-evaluated per check)
}

// ErrDraftChanged is returned by SaveToDiskIfGen when the draft was mutated
// after the caller captured its generation (validate-then-apply TOCTOU).
var ErrDraftChanged = errors.New("draft changed since validation")

// ErrInboundNotFound is returned by MutateInbound when no inbound matches the
// given tag, so callers can distinguish "absent" from a real failure.
var ErrInboundNotFound = errors.New("inbound not found")

// NewManager creates a new config manager and loads the config
func NewManager(path string) (*Manager, error) {
	m := &Manager{
		path:         path,
		draftPath:    path + ".bak",
		activeConfig: make(map[string]interface{}),
	}

	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// NewEmptyManager creates a manager with empty config
func NewEmptyManager(path string) *Manager {
	return &Manager{
		path:         path,
		draftPath:    path + ".bak",
		activeConfig: make(map[string]interface{}),
		readOnly:     false,
	}
}

// NewReadOnlyManager creates a manager that can read but not write
func NewReadOnlyManager(path string) (*Manager, error) {
	m := &Manager{
		path:         path,
		draftPath:    path + ".bak",
		activeConfig: make(map[string]interface{}),
		readOnly:     true,
	}

	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// IsReadOnly returns true if manager is in read-only mode
func (m *Manager) IsReadOnly() bool {
	return m.readOnly
}

// SetReadOnly sets the read-only mode
func (m *Manager) SetReadOnly(readOnly bool) {
	m.readOnly = readOnly
}

// SetPathWithoutLoad sets the config path without loading
func (m *Manager) SetPathWithoutLoad(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = path
	m.draftPath = path + ".bak"
}

// atomicWriteFile writes data to path atomically: write to a temp file in the
// same directory, fsync, set permissions, then rename over the target. A crash
// mid-write leaves the original file intact.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = "" // success: disable deferred cleanup
	// fsync the directory so the rename itself survives power loss
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// pruneBackups keeps only the newest `keep` timestamped backups of base
// (files named <base>.<unix-ts>.bak) in dir, deleting the rest. The draft
// file <base>.bak has no numeric timestamp and is never touched. Best-effort:
// errors are ignored.
func pruneBackups(dir, base string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := base + "."
	type backup struct {
		name string
		ts   int64
	}
	var backups []backup
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".bak") {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".bak")
		ts, err := strconv.ParseInt(mid, 10, 64)
		if err != nil {
			continue // not a timestamped backup (e.g. the draft "config.json.bak")
		}
		backups = append(backups, backup{name: name, ts: ts})
	}
	if len(backups) <= keep {
		return
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].ts > backups[j].ts })
	for _, b := range backups[keep:] {
		os.Remove(filepath.Join(dir, b.name))
	}
}

// SetCheckBinaryProvider sets a provider for the binary used in `check`
// validation. It is re-evaluated on every check so a binary detected or
// installed after startup is picked up without restarting RouteBox.
func (m *Manager) SetCheckBinaryProvider(fn func() string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkBinaryFn = fn
}

// Load reads config from file into activeConfig and clears any draft
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	m.activeConfig = config
	m.draftConfig = nil
	m.hasDraft = false
	m.draftGen++
	return nil
}

// Save writes config to file with backup (saves to active config, clears draft)
func (m *Manager) Save(config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked(config)
}

// saveLocked is the body of Save without locking (caller must hold m.mu write lock).
func (m *Manager) saveLocked(config map[string]interface{}) error {
	// Check if read-only
	if m.readOnly {
		return fmt.Errorf("cannot save: config is read-only (run with sudo or stop the systemd service)")
	}

	// Check if path is set
	if m.path == "" {
		return fmt.Errorf("config path not set")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create timestamped backup if file exists
	if _, err := os.Stat(m.path); err == nil {
		backupPath := fmt.Sprintf("%s.%d.bak", m.path, time.Now().Unix())
		if data, err := os.ReadFile(m.path); err == nil {
			os.WriteFile(backupPath, data, 0600)
		}
	}

	// Write new config without HTML escaping (important for AWG binary data like i1, i2, etc.)
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data := buf.Bytes()

	if err := atomicWriteFile(m.path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	pruneBackups(dir, filepath.Base(m.path), 5)

	m.activeConfig = config
	// Clear draft after successful save to active
	m.draftConfig = nil
	m.hasDraft = false
	m.draftGen++
	// Remove draft file if exists
	os.Remove(m.draftPath)
	return nil
}

// Get returns the current working config (draft if exists, otherwise active)
func (m *Manager) Get() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.deepCopy(m.getWorkingConfig())
}

// GetActive returns the active config on disk (ignoring any draft)
func (m *Manager) GetActive() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.deepCopy(m.activeConfig)
}

// getWorkingConfig returns draft if exists, otherwise active (no lock, internal use)
func (m *Manager) getWorkingConfig() map[string]interface{} {
	if m.hasDraft && m.draftConfig != nil {
		return m.draftConfig
	}
	return m.activeConfig
}

// deepCopy creates a deep copy of a config map
func (m *Manager) deepCopy(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	// Use encoder with SetEscapeHTML(false) to preserve special characters
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(config)
	var copy map[string]interface{}
	json.Unmarshal(buf.Bytes(), &copy)
	return copy
}

// GetPath returns the config file path
func (m *Manager) GetPath() string {
	return m.path
}

// HasVpnConfig checks if config has VLESS/Hysteria2 outbound or AWG endpoint
func (m *Manager) HasVpnConfig() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config := m.getWorkingConfig()

	// Check for AWG/WireGuard endpoints
	if endpoints, ok := config["endpoints"].([]interface{}); ok {
		for _, ep := range endpoints {
			if obj, ok := ep.(map[string]interface{}); ok {
				if epType, ok := obj["type"].(string); ok {
					if epType == "awg" || epType == "wireguard" {
						return true
					}
				}
			}
		}
	}

	// Check for VPN outbounds (vless, hysteria2, naive)
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for _, ob := range outbounds {
			if obj, ok := ob.(map[string]interface{}); ok {
				if obType, ok := obj["type"].(string); ok {
					// vless, hysteria2 and naive are common VPN protocols
					if obType == "vless" || obType == "hysteria2" || obType == "naive" {
						return true
					}
				}
			}
		}
	}

	return false
}

// SetPath changes the config file path and reloads
func (m *Manager) SetPath(path string) error {
	m.mu.Lock()
	m.path = path
	m.draftPath = path + ".bak"
	m.mu.Unlock()
	return m.Load()
}

// Diff returns the difference between current and new config
func (m *Manager) Diff(newConfig map[string]interface{}) (string, error) {
	m.mu.RLock()
	current := m.getWorkingConfig()
	m.mu.RUnlock()

	// Use encoder with SetEscapeHTML(false) to preserve special characters
	var currentBuf bytes.Buffer
	currentEncoder := json.NewEncoder(&currentBuf)
	currentEncoder.SetEscapeHTML(false)
	currentEncoder.SetIndent("", "  ")
	if err := currentEncoder.Encode(current); err != nil {
		return "", err
	}

	var newBuf bytes.Buffer
	newEncoder := json.NewEncoder(&newBuf)
	newEncoder.SetEscapeHTML(false)
	newEncoder.SetIndent("", "  ")
	if err := newEncoder.Encode(newConfig); err != nil {
		return "", err
	}

	// Simple text diff (in production, use a proper diff library)
	return fmt.Sprintf("Current:\n%s\n\nNew:\n%s", currentBuf.String(), newBuf.String()), nil
}

// --- Draft System Methods ---

// HasDraft returns true if there are uncommitted changes
func (m *Manager) HasDraft() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hasDraft
}

// CleanupDraft removes any existing draft file and orphaned atomic-write temp
// files (<base>.tmp-* / <base>.bak.tmp-*) left by a crash (call on startup)
func (m *Manager) CleanupDraft() {
	m.mu.Lock()
	defer m.mu.Unlock()

	os.Remove(m.draftPath)
	m.draftConfig = nil
	m.hasDraft = false
	m.draftGen++

	dir := filepath.Dir(m.path)
	base := filepath.Base(m.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(name, base+".tmp-") || strings.HasPrefix(name, base+".bak.tmp-") {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

// EnsureDraft creates a draft from active config if one doesn't exist (lazy copy)
func (m *Manager) EnsureDraft() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hasDraft {
		return nil // Draft already exists
	}

	// Deep copy active config to draft
	m.draftConfig = m.deepCopy(m.activeConfig)
	m.hasDraft = true

	// Save draft to disk
	return m.saveDraftToDisk()
}

// saveDraftToDisk writes draft config to .bak file (internal, must hold lock)
func (m *Manager) saveDraftToDisk() error {
	if m.draftConfig == nil {
		return nil
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(m.draftConfig); err != nil {
		return fmt.Errorf("failed to marshal draft config: %w", err)
	}

	m.draftGen++
	if err := atomicWriteFile(m.draftPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write draft config: %w", err)
	}
	return nil
}

// SaveDraft persists current draft to disk
func (m *Manager) SaveDraft() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.hasDraft {
		return nil
	}

	return m.saveDraftToDisk()
}

// SetDraft replaces the entire draft config (used by JSON editor)
func (m *Manager) SetDraft(config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.draftConfig = m.deepCopy(config)
	m.hasDraft = true

	return m.saveDraftToDisk()
}

// DiscardDraft removes draft and reverts to active config
func (m *Manager) DiscardDraft() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.draftConfig = nil
	m.hasDraft = false
	m.draftGen++

	// Remove draft file
	if err := os.Remove(m.draftPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove draft file: %w", err)
	}

	return nil
}

// ApplyDraft saves draft as active config and clears the draft
func (m *Manager) ApplyDraft() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyDraftLocked()
}

// applyDraftLocked is the body of ApplyDraft without locking (caller must hold m.mu write lock).
func (m *Manager) applyDraftLocked() error {
	if !m.hasDraft || m.draftConfig == nil {
		return nil // Nothing to apply
	}

	if m.readOnly {
		return fmt.Errorf("cannot apply: config is read-only")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create timestamped backup of current active config
	if _, err := os.Stat(m.path); err == nil {
		backupPath := fmt.Sprintf("%s.%d.bak", m.path, time.Now().Unix())
		if data, err := os.ReadFile(m.path); err == nil {
			os.WriteFile(backupPath, data, 0600)
		}
	}

	// Write draft to active config file
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(m.draftConfig); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := atomicWriteFile(m.path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	pruneBackups(dir, filepath.Base(m.path), 5)

	// Update active config and clear draft
	m.activeConfig = m.draftConfig
	m.draftConfig = nil
	m.hasDraft = false
	m.draftGen++

	// Remove draft file
	os.Remove(m.draftPath)

	return nil
}

// GetDiff returns a unified diff between active and draft config
func (m *Manager) GetDiff() (string, int, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.hasDraft || m.draftConfig == nil {
		return "", 0, 0, nil
	}

	// Encode both configs
	var activeBuf bytes.Buffer
	activeEncoder := json.NewEncoder(&activeBuf)
	activeEncoder.SetEscapeHTML(false)
	activeEncoder.SetIndent("", "  ")
	if err := activeEncoder.Encode(m.activeConfig); err != nil {
		return "", 0, 0, err
	}

	var draftBuf bytes.Buffer
	draftEncoder := json.NewEncoder(&draftBuf)
	draftEncoder.SetEscapeHTML(false)
	draftEncoder.SetIndent("", "  ")
	if err := draftEncoder.Encode(m.draftConfig); err != nil {
		return "", 0, 0, err
	}

	// Write to temp files and use system diff for proper unified diff
	activeFile, err := os.CreateTemp("", "routebox-active-*.json")
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(activeFile.Name())
	defer activeFile.Close()

	draftFile, err := os.CreateTemp("", "routebox-draft-*.json")
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(draftFile.Name())
	defer draftFile.Close()

	if _, err := activeFile.Write(activeBuf.Bytes()); err != nil {
		return "", 0, 0, fmt.Errorf("failed to write temp file: %w", err)
	}
	activeFile.Close()

	if _, err := draftFile.Write(draftBuf.Bytes()); err != nil {
		return "", 0, 0, fmt.Errorf("failed to write temp file: %w", err)
	}
	draftFile.Close()

	// Run unified diff; exit code 1 means differences found (not an error)
	cmd := exec.Command("diff", "-u", "--label", "active", "--label", "draft", activeFile.Name(), draftFile.Name())
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Exit code 1 = differences found, output contains the diff
			output = []byte(string(output))
		} else {
			return "", 0, 0, fmt.Errorf("diff command failed: %w", err)
		}
	}

	// Count additions and deletions from diff output (skip header lines starting with ---, +++, @@)
	additions := 0
	deletions := 0
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		} else if strings.HasPrefix(line, "-") {
			deletions++
		}
	}

	return string(output), additions, deletions, nil
}

// ansiEscapeRegex matches ANSI SGR escape sequences (e.g. "\x1b[31m") so we
// can strip colors from sing-box stderr before surfacing it to the UI.
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(s string) string {
	return ansiEscapeRegex.ReplaceAllString(s, "")
}

// CheckConfig validates config using the detected sing-box/amnezia-box binary
func (m *Manager) CheckConfig(configPath string) (bool, []string) {
	m.mu.RLock()
	fn := m.checkBinaryFn
	m.mu.RUnlock()

	binary := ""
	if fn != nil {
		binary = fn() // outside m.mu: provider takes the process manager's lock
	}
	if binary == "" {
		binary = "sing-box"
	}

	if _, err := exec.LookPath(binary); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			log.Printf("Warning: config validation skipped: binary not found: %s", binary)
			return true, nil
		}
		// Permission problems etc. are real failures — surface, don't skip
		return false, []string{fmt.Sprintf("cannot run config check with %s: %v", binary, err)}
	}

	cmd := exec.Command(binary, "check", "-c", configPath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Parse error output — strip ANSI color codes so the UI shows readable text
		cleaned := stripAnsi(string(output))
		lines := strings.Split(strings.TrimSpace(cleaned), "\n")
		checkErrs := make([]string, 0, len(lines))
		for _, line := range lines {
			if line != "" {
				checkErrs = append(checkErrs, line)
			}
		}
		if len(checkErrs) == 0 {
			checkErrs = append(checkErrs, err.Error())
		}
		return false, checkErrs
	}

	return true, nil
}

// CheckDraft validates draft config using sing-box check command. It returns
// the draft generation observed at check time, for use with SaveToDiskIfGen.
func (m *Manager) CheckDraft() (bool, []string, int64) {
	m.mu.RLock()
	hasDraft := m.hasDraft
	draftPath := m.draftPath
	gen := m.draftGen
	m.mu.RUnlock()

	if !hasDraft {
		return true, nil, gen
	}

	valid, errs := m.CheckConfig(draftPath)
	return valid, errs, gen
}

// GetDraftGen returns the current draft generation counter.
func (m *Manager) GetDraftGen() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.draftGen
}

// SaveToDiskIfGen is SaveToDisk guarded by a draft generation check: if the
// draft was mutated after gen was captured (e.g. between validation and
// apply), it returns ErrDraftChanged and writes nothing.
func (m *Manager) SaveToDiskIfGen(gen int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.draftGen != gen {
		return ErrDraftChanged
	}

	if m.hasDraft {
		return m.applyDraftLocked()
	}
	return m.saveLocked(m.deepCopy(m.activeConfig))
}

// --- Internal helpers ---

// getArray returns array from working config at given key
func (m *Manager) getArray(key string) []interface{} {
	config := m.getWorkingConfig()
	if arr, ok := config[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// ensureDraftUnlocked creates draft if needed (internal, must hold write lock)
func (m *Manager) ensureDraftUnlocked() error {
	if m.hasDraft {
		return nil
	}

	m.draftConfig = m.deepCopy(m.activeConfig)
	m.hasDraft = true
	return m.saveDraftToDisk()
}

// setDraftValue sets a value in the draft config (must call ensureDraftUnlocked first, must hold write lock)
func (m *Manager) setDraftValue(key string, value interface{}) {
	if m.draftConfig != nil {
		m.draftConfig[key] = value
	}
}

// getDraftArray returns array from draft config (must hold lock, draft must exist)
func (m *Manager) getDraftArray(key string) []interface{} {
	if m.draftConfig == nil {
		return []interface{}{}
	}
	if arr, ok := m.draftConfig[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// --- Draft-aware nested section helpers ---

// getDraftRoute returns route section from draft, creating if needed (must hold lock, draft must exist)
func (m *Manager) getDraftRoute() map[string]interface{} {
	if m.draftConfig == nil {
		return make(map[string]interface{})
	}
	if route, ok := m.draftConfig["route"].(map[string]interface{}); ok {
		return route
	}
	route := make(map[string]interface{})
	m.draftConfig["route"] = route
	return route
}

// getDraftRouteArray returns array from draft route section (must hold lock, draft must exist)
func (m *Manager) getDraftRouteArray(key string) []interface{} {
	route := m.getDraftRoute()
	if arr, ok := route[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// getDraftDns returns dns section from draft, creating if needed (must hold lock, draft must exist)
func (m *Manager) getDraftDns() map[string]interface{} {
	if m.draftConfig == nil {
		return make(map[string]interface{})
	}
	if dns, ok := m.draftConfig["dns"].(map[string]interface{}); ok {
		return dns
	}
	dns := make(map[string]interface{})
	m.draftConfig["dns"] = dns
	return dns
}

// getDraftDnsArray returns array from draft dns section (must hold lock, draft must exist)
func (m *Manager) getDraftDnsArray(key string) []interface{} {
	dns := m.getDraftDns()
	if arr, ok := dns[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// getDraftLog returns log section from draft, creating if needed (must hold lock, draft must exist)
func (m *Manager) getDraftLog() map[string]interface{} {
	if m.draftConfig == nil {
		return make(map[string]interface{})
	}
	if log, ok := m.draftConfig["log"].(map[string]interface{}); ok {
		return log
	}
	log := make(map[string]interface{})
	m.draftConfig["log"] = log
	return log
}

// getDraftExperimental returns experimental section from draft, creating if needed (must hold lock, draft must exist)
func (m *Manager) getDraftExperimental() map[string]interface{} {
	if m.draftConfig == nil {
		return make(map[string]interface{})
	}
	if exp, ok := m.draftConfig["experimental"].(map[string]interface{}); ok {
		return exp
	}
	exp := make(map[string]interface{})
	m.draftConfig["experimental"] = exp
	return exp
}

// SaveToDisk persists current draft (or active if no draft) to file.
// The draft check and the write happen in a single critical section so a
// concurrent ApplyDraft/DiscardDraft cannot change state in between.
func (m *Manager) SaveToDisk() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hasDraft {
		// Apply draft to active
		return m.applyDraftLocked()
	}

	return m.saveLocked(m.deepCopy(m.activeConfig))
}

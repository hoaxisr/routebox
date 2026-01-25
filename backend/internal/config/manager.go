package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles config file operations
type Manager struct {
	path     string
	config   map[string]interface{}
	mu       sync.RWMutex
	readOnly bool
}

// NewManager creates a new config manager and loads the config
func NewManager(path string) (*Manager, error) {
	m := &Manager{
		path:   path,
		config: make(map[string]interface{}),
	}

	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// NewEmptyManager creates a manager with empty config
func NewEmptyManager(path string) *Manager {
	return &Manager{
		path:     path,
		config:   make(map[string]interface{}),
		readOnly: false,
	}
}

// NewReadOnlyManager creates a manager that can read but not write
func NewReadOnlyManager(path string) (*Manager, error) {
	m := &Manager{
		path:     path,
		config:   make(map[string]interface{}),
		readOnly: true,
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
}

// Load reads config from file
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

	m.config = config
	return nil
}

// Save writes config to file with backup
func (m *Manager) Save(config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	// Create backup if file exists
	if _, err := os.Stat(m.path); err == nil {
		backupPath := fmt.Sprintf("%s.%d.bak", m.path, time.Now().Unix())
		if data, err := os.ReadFile(m.path); err == nil {
			os.WriteFile(backupPath, data, 0644)
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

	if err := os.WriteFile(m.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	m.config = config
	return nil
}

// Get returns the current config
func (m *Manager) Get() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy (use encoder with SetEscapeHTML(false) to preserve special characters)
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(m.config)
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

	// Check for AWG/WireGuard endpoints
	if endpoints, ok := m.config["endpoints"].([]interface{}); ok {
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

	// Check for VPN outbounds (vless, hysteria2)
	if outbounds, ok := m.config["outbounds"].([]interface{}); ok {
		for _, ob := range outbounds {
			if obj, ok := ob.(map[string]interface{}); ok {
				if obType, ok := obj["type"].(string); ok {
					// vless and hysteria2 are common VPN protocols
					if obType == "vless" || obType == "hysteria2" {
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
	m.mu.Unlock()
	return m.Load()
}

// Diff returns the difference between current and new config
func (m *Manager) Diff(newConfig map[string]interface{}) (string, error) {
	m.mu.RLock()
	current := m.config
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

// --- Internal helpers ---

// getArray returns array from config at given key
func (m *Manager) getArray(key string) []interface{} {
	if arr, ok := m.config[key].([]interface{}); ok {
		return arr
	}
	return []interface{}{}
}

// SaveToDisk persists current in-memory config to file
func (m *Manager) SaveToDisk() error {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	return m.Save(config)
}

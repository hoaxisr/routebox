package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"routebox/backend/internal/auth"
)

// Settings represents the complete RouteBox configuration
type Settings struct {
	GeoIP      GeoIPSettings      `toml:"geoip" json:"geoip"`
	UI         UISettings         `toml:"ui" json:"ui"`
	Monitoring MonitoringSettings `toml:"monitoring" json:"monitoring"`
	Logging    LoggingSettings    `toml:"logging" json:"logging"`
	Security   SecuritySettings   `toml:"security" json:"security"`
	Network    NetworkSettings    `toml:"network" json:"network"`
	Server     ServerSettings     `toml:"server" json:"server"`
	Singbox    SingboxSettings    `toml:"singbox" json:"singbox"`
	Advanced   AdvancedSettings   `toml:"advanced" json:"advanced"`
	Updates    UpdatesSettings    `toml:"updates" json:"updates"`
}

// UpdatesSettings configures binary update checks
type UpdatesSettings struct {
	AutoCheck bool `toml:"auto_check" json:"auto_check"`
}

// GeoIPSettings configures GeoIP enrichment
type GeoIPSettings struct {
	Path       string `toml:"path" json:"path"`
	Enabled    bool   `toml:"enabled" json:"enabled"`
	AutoReload bool   `toml:"auto_reload" json:"auto_reload"`
}

// UISettings configures user interface defaults
type UISettings struct {
	Theme      string `toml:"theme" json:"theme"`
	Language   string `toml:"language" json:"language"`
	SpeedUnit  string `toml:"speed_unit" json:"speed_unit"`
	TimeFormat string `toml:"time_format" json:"time_format"`
}

// MonitoringSettings configures monitoring features
type MonitoringSettings struct {
	EnrichmentEnabled    bool `toml:"enrichment_enabled" json:"enrichment_enabled"`
	MaxClosedConnections int  `toml:"max_closed_connections" json:"max_closed_connections"`
	PollIntervalMs       int  `toml:"poll_interval_ms" json:"poll_interval_ms"`
	ProxiesRefreshMs     int  `toml:"proxies_refresh_ms" json:"proxies_refresh_ms"`
}

// LoggingSettings configures application logging
type LoggingSettings struct {
	Level      string `toml:"level" json:"level"`
	Output     string `toml:"output" json:"output"`
	Timestamps bool   `toml:"timestamps" json:"timestamps"`
	Format     string `toml:"format" json:"format"`
}

// SecuritySettings configures access control
type SecuritySettings struct {
	AuthEnabled           bool   `toml:"auth_enabled" json:"auth_enabled"`
	AuthUsername          string `toml:"auth_username" json:"auth_username"`
	AuthPassword          string `toml:"auth_password" json:"-"`      // Never expose in JSON; cleared after migration
	AuthPasswordHash      string `toml:"auth_password_hash" json:"-"` // bcrypt hash; never expose in JSON
	CorsOrigins           string `toml:"cors_origins" json:"cors_origins"`
	SessionTimeoutMinutes int    `toml:"session_timeout_minutes" json:"session_timeout_minutes"`
}

// NetworkSettings configures network parameters
type NetworkSettings struct {
	Listen             string `toml:"listen" json:"listen"`
	ReadTimeoutSec     int    `toml:"read_timeout_sec" json:"read_timeout_sec"`
	WriteTimeoutSec    int    `toml:"write_timeout_sec" json:"write_timeout_sec"`
	CompressionEnabled bool   `toml:"compression_enabled" json:"compression_enabled"`
	TLSCertPath        string `toml:"tls_cert_path" json:"tls_cert_path"`
	TLSKeyPath         string `toml:"tls_key_path" json:"tls_key_path"`
}

// ServerSettings holds panel operating-mode configuration.
type ServerSettings struct {
	Mode string `toml:"mode" json:"mode"` // "router" (default) | "vps"
}

// SingboxSettings configures sing-box integration
type SingboxSettings struct {
	ConfigPath  string `toml:"config_path" json:"config_path"`
	ClashAPI    string `toml:"clash_api" json:"clash_api"`
	BinaryName  string `toml:"binary_name" json:"binary_name"`
	ServiceName string `toml:"service_name" json:"service_name"`
}

// AdvancedSettings for power users
type AdvancedSettings struct {
	DebugEndpoints    bool `toml:"debug_endpoints" json:"debug_endpoints"`
	PprofEnabled      bool `toml:"pprof_enabled" json:"pprof_enabled"`
	MaxBodySize       int  `toml:"max_body_size" json:"max_body_size"`
	WsPingIntervalSec int  `toml:"ws_ping_interval_sec" json:"ws_ping_interval_sec"`
	WsPongTimeoutSec  int  `toml:"ws_pong_timeout_sec" json:"ws_pong_timeout_sec"`
}

// Manager handles settings loading, saving, and runtime updates
type Manager struct {
	settings Settings
	path     string
	mu       sync.RWMutex
}

// Default returns settings with default values
func Default() Settings {
	return Settings{
		GeoIP: GeoIPSettings{
			Path:       "",
			Enabled:    true,
			AutoReload: false,
		},
		UI: UISettings{
			Theme:      "system",
			Language:   "en",
			SpeedUnit:  "bytes",
			TimeFormat: "relative",
		},
		Monitoring: MonitoringSettings{
			EnrichmentEnabled:    true,
			MaxClosedConnections: 500,
			PollIntervalMs:       2000,
			ProxiesRefreshMs:     5000,
		},
		Logging: LoggingSettings{
			Level:      "info",
			Output:     "stdout",
			Timestamps: true,
			Format:     "text",
		},
		Security: SecuritySettings{
			AuthEnabled:           false,
			AuthUsername:          "",
			AuthPassword:          "",
			CorsOrigins:           "*",
			SessionTimeoutMinutes: 720,
		},
		Network: NetworkSettings{
			Listen:             "0.0.0.0:8080",
			ReadTimeoutSec:     30,
			WriteTimeoutSec:    30,
			CompressionEnabled: true,
		},
		Singbox: SingboxSettings{
			ConfigPath:  "",
			ClashAPI:    "",
			BinaryName:  "amnezia-box",
			ServiceName: "",
		},
		Advanced: AdvancedSettings{
			DebugEndpoints:    false,
			PprofEnabled:      false,
			MaxBodySize:       10485760,
			WsPingIntervalSec: 30,
			WsPongTimeoutSec:  10,
		},
		Server:  ServerSettings{Mode: "router"},
		Updates: UpdatesSettings{AutoCheck: true},
	}
}

// NewManager creates a settings manager, loading from file if exists
func NewManager(path string) (*Manager, error) {
	m := &Manager{
		settings: Default(),
		path:     path,
	}

	// If path is empty, try to find settings file next to binary
	if path == "" {
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			candidates := []string{
				filepath.Join(execDir, "routebox.toml"),
				filepath.Join(execDir, "config.toml"),
				"/etc/routebox/routebox.toml",
			}
			for _, candidate := range candidates {
				if _, err := os.Stat(candidate); err == nil {
					path = candidate
					m.path = path
					break
				}
			}
		}
	}

	// Load from file if exists
	if path != "" {
		if err := m.Load(); err != nil {
			// File doesn't exist - that's OK, use defaults
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to load settings: %w", err)
			}
			log.Printf("Settings file not found at %s, using defaults", path)
		}
	}

	return m, nil
}

// Load reads settings from file. A decode failure leaves m.settings unchanged.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.path == "" {
		return nil
	}

	// Decode into a local temp value so a failure leaves m.settings intact.
	tmp := Default()
	_, err := toml.DecodeFile(m.path, &tmp)
	if err != nil {
		return err
	}

	// Commit the successfully decoded settings.
	m.settings = tmp

	log.Printf("Loaded settings from %s", m.path)

	// Migrate plaintext password to bcrypt hash whenever plaintext is present
	// (covers both first-time migration and "forgotten password" resets where a
	// new plaintext is written alongside an existing hash). Must run before
	// releasing the lock.
	if m.settings.Security.AuthPassword != "" {
		h, err := auth.HashPassword(m.settings.Security.AuthPassword)
		if err != nil {
			return fmt.Errorf("migrate password hash: %w", err)
		}
		m.settings.Security.AuthPasswordHash = h
		m.settings.Security.AuthPassword = ""
		// Fix 3: a read-only config file must not abort startup — keep the
		// in-memory hash and warn instead of returning an error.
		if err := m.saveLocked(); err != nil {
			log.Printf("WARNING: migrated password to bcrypt in memory but could not persist (config may be read-only): %v — plaintext remains on disk until writable", err)
		}
	}

	return nil
}

// saveLocked writes current settings to disk atomically (write to temp file,
// fsync, chmod 0600, rename over target). The caller must hold m.mu.Lock
// (exclusive).
func (m *Manager) saveLocked() error {
	if m.path == "" {
		return fmt.Errorf("no settings path configured")
	}

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(m.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Deferred cleanup: remove the temp file if we haven't renamed it away.
	defer func() {
		if tmpName != "" {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	encoder := toml.NewEncoder(tmp)
	if err := encoder.Encode(m.settings); err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		tmpName = "" // already closed; skip double-close in defer
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	tmpName = "" // success: disable deferred cleanup

	log.Printf("Saved settings to %s", m.path)
	return nil
}

// Save writes current settings to file.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

// Get returns a copy of current settings
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// GetPath returns the settings file path
func (m *Manager) GetPath() string {
	return m.path
}

// toInt converts a JSON-decoded numeric value to int. JSON numbers decode to
// float64; TOML and direct callers may pass int; UseNumber decoders pass
// json.Number. Fractional values are rejected.
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

// Update applies partial updates to settings (runtime-safe fields only)
func (m *Manager) Update(updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Apply updates to runtime-safe fields
	for key, value := range updates {
		switch key {
		// GeoIP runtime settings
		case "geoip.enabled":
			if v, ok := value.(bool); ok {
				m.settings.GeoIP.Enabled = v
			}
		case "geoip.auto_reload":
			if v, ok := value.(bool); ok {
				m.settings.GeoIP.AutoReload = v
			}

		// UI runtime settings
		case "ui.theme":
			if v, ok := value.(string); ok {
				m.settings.UI.Theme = v
			}
		case "ui.language":
			if v, ok := value.(string); ok {
				m.settings.UI.Language = v
			}
		case "ui.speed_unit":
			if v, ok := value.(string); ok {
				m.settings.UI.SpeedUnit = v
			}
		case "ui.time_format":
			if v, ok := value.(string); ok {
				m.settings.UI.TimeFormat = v
			}

		// Monitoring runtime settings
		case "monitoring.enrichment_enabled":
			if v, ok := value.(bool); ok {
				m.settings.Monitoring.EnrichmentEnabled = v
			}
		case "monitoring.max_closed_connections":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Monitoring.MaxClosedConnections = v
		case "monitoring.poll_interval_ms":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Monitoring.PollIntervalMs = v
		case "monitoring.proxies_refresh_ms":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Monitoring.ProxiesRefreshMs = v

		// Security runtime settings
		case "security.auth_enabled":
			if v, ok := value.(bool); ok {
				m.settings.Security.AuthEnabled = v
			}
		case "security.auth_username":
			if v, ok := value.(string); ok {
				m.settings.Security.AuthUsername = v
			}
		case "security.session_timeout_minutes":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Security.SessionTimeoutMinutes = v
		case "security.auth_password":
			if v, ok := value.(string); ok && v != "" {
				h, err := auth.HashPassword(v)
				if err != nil {
					return fmt.Errorf("hash password: %w", err)
				}
				m.settings.Security.AuthPasswordHash = h
				m.settings.Security.AuthPassword = ""
			}
		case "network.tls_cert_path":
			if v, ok := value.(string); ok {
				m.settings.Network.TLSCertPath = v
			}
		case "network.tls_key_path":
			if v, ok := value.(string); ok {
				m.settings.Network.TLSKeyPath = v
			}
		case "server.mode":
			// Fix 4: reject invalid values instead of silently ignoring them.
			v, ok := value.(string)
			if !ok || (v != "router" && v != "vps") {
				return fmt.Errorf("invalid server.mode %q (want router|vps)", value)
			}
			m.settings.Server.Mode = v

		// Advanced runtime settings
		case "advanced.ws_ping_interval_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Advanced.WsPingIntervalSec = v
		case "advanced.ws_pong_timeout_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			m.settings.Advanced.WsPongTimeoutSec = v

		// Updates runtime settings
		case "updates.auto_check":
			if v, ok := value.(bool); ok {
				m.settings.Updates.AutoCheck = v
			}

		default:
			return fmt.Errorf("unknown or non-runtime setting: %s", key)
		}
	}

	return nil
}

// UpdateSection updates an entire section (for UI forms)
func (m *Manager) UpdateSection(section string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch section {
	case "geoip":
		if v, ok := data.(GeoIPSettings); ok {
			// Only update runtime fields
			m.settings.GeoIP.Enabled = v.Enabled
			m.settings.GeoIP.AutoReload = v.AutoReload
		}
	case "ui":
		if v, ok := data.(UISettings); ok {
			m.settings.UI = v
		}
	case "monitoring":
		if v, ok := data.(MonitoringSettings); ok {
			m.settings.Monitoring = v
		}
	default:
		return fmt.Errorf("unknown or non-runtime section: %s", section)
	}

	return nil
}

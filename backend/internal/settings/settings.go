package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
)

// Settings represents the complete RouteBox configuration
type Settings struct {
	GeoIP      GeoIPSettings      `toml:"geoip" json:"geoip"`
	UI         UISettings         `toml:"ui" json:"ui"`
	Monitoring MonitoringSettings `toml:"monitoring" json:"monitoring"`
	Logging    LoggingSettings    `toml:"logging" json:"logging"`
	Security   SecuritySettings   `toml:"security" json:"security"`
	Network    NetworkSettings    `toml:"network" json:"network"`
	Singbox    SingboxSettings    `toml:"singbox" json:"singbox"`
	Advanced   AdvancedSettings   `toml:"advanced" json:"advanced"`
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
	AuthPassword          string `toml:"auth_password" json:"-"` // Never expose in JSON
	CorsOrigins           string `toml:"cors_origins" json:"cors_origins"`
	SessionTimeoutMinutes int    `toml:"session_timeout_minutes" json:"session_timeout_minutes"`
}

// NetworkSettings configures network parameters
type NetworkSettings struct {
	Listen             string `toml:"listen" json:"listen"`
	ReadTimeoutSec     int    `toml:"read_timeout_sec" json:"read_timeout_sec"`
	WriteTimeoutSec    int    `toml:"write_timeout_sec" json:"write_timeout_sec"`
	CompressionEnabled bool   `toml:"compression_enabled" json:"compression_enabled"`
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
	DebugEndpoints   bool `toml:"debug_endpoints" json:"debug_endpoints"`
	PprofEnabled     bool `toml:"pprof_enabled" json:"pprof_enabled"`
	MaxBodySize      int  `toml:"max_body_size" json:"max_body_size"`
	WsPingIntervalSec int `toml:"ws_ping_interval_sec" json:"ws_ping_interval_sec"`
	WsPongTimeoutSec  int `toml:"ws_pong_timeout_sec" json:"ws_pong_timeout_sec"`
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
			SessionTimeoutMinutes: 0,
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

// Load reads settings from file
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.path == "" {
		return nil
	}

	// Start with defaults
	m.settings = Default()

	// Decode TOML on top of defaults
	_, err := toml.DecodeFile(m.path, &m.settings)
	if err != nil {
		return err
	}

	log.Printf("Loaded settings from %s", m.path)
	return nil
}

// Save writes current settings to file
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.path == "" {
		return fmt.Errorf("no settings path configured")
	}

	// Ensure directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(m.path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(m.settings); err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
	}

	log.Printf("Saved settings to %s", m.path)
	return nil
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
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Monitoring.MaxClosedConnections = v
		case "monitoring.poll_interval_ms":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Monitoring.PollIntervalMs = v
		case "monitoring.proxies_refresh_ms":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Monitoring.ProxiesRefreshMs = v

		// Security runtime settings
		case "security.session_timeout_minutes":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Security.SessionTimeoutMinutes = v

		// Advanced runtime settings
		case "advanced.ws_ping_interval_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Advanced.WsPingIntervalSec = v
		case "advanced.ws_pong_timeout_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s requires an integer value, got %T", key, value)
			}
			m.settings.Advanced.WsPongTimeoutSec = v

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

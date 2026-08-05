package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"routebox/backend/internal/auth"
	"routebox/backend/internal/util"
)

// Settings represents the complete RouteBox configuration
type Settings struct {
	GeoIP      GeoIPSettings      `toml:"geoip" json:"geoip"`
	UI         UISettings         `toml:"ui" json:"ui"`
	Monitoring MonitoringSettings `toml:"monitoring" json:"monitoring"`
	Security   SecuritySettings   `toml:"security" json:"security"`
	Network    NetworkSettings    `toml:"network" json:"network"`
	Server     ServerSettings     `toml:"server" json:"server"`
	Singbox    SingboxSettings    `toml:"singbox" json:"singbox"`
	Advanced   AdvancedSettings   `toml:"advanced" json:"advanced"`
	Updates    UpdatesSettings    `toml:"updates" json:"updates"`
	Awg        AwgSettings        `toml:"awg" json:"awg"`
	Mtproto    MtprotoSettings    `toml:"mtproto" json:"mtproto"`
}

// MtprotoSettings configures the built-in Telegram MTProto proxy.
//
// Client secrets are NOT here: they live in mtproto.toml, the same way AWG peer
// keys live in peers.toml rather than in this file.
type MtprotoSettings struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Listen  string `toml:"listen" json:"listen"` // "host:port" the proxy binds

	// MaskingDomain is the site FakeTLS impersonates, and where connections
	// that authenticate against no secret are fronted to.
	//
	// It is encoded into every issued secret, so changing it invalidates every
	// link already handed out. There is no safe default: it has to be a real
	// host that plausibly receives HTTPS from your users.
	MaskingDomain string `toml:"masking_domain" json:"masking_domain"`

	// PublicHost and PublicPort are what go into tg:// links. Empty falls back
	// to the panel's own public address, as subscription URLs do — they differ
	// whenever a reverse proxy or SNI router sits in front.
	PublicHost string `toml:"public_host" json:"public_host"`
	PublicPort int    `toml:"public_port" json:"public_port"`

	Concurrency        int    `toml:"concurrency" json:"concurrency"`
	IdleTimeoutSec     int    `toml:"idle_timeout_sec" json:"idle_timeout_sec"`
	PreferIP           string `toml:"prefer_ip" json:"prefer_ip"` // ""|prefer-ipv4|prefer-ipv6|only-ipv4|only-ipv6
	DomainFrontingPort int    `toml:"domain_fronting_port" json:"domain_fronting_port"`

	// Outbound is the sing-box outbound or endpoint tag the proxy's Telegram
	// connections leave through. Empty — the default — dials Telegram straight
	// out of the box, which is what every install did before this existed.
	//
	// Anything else makes RouteBox manage a loopback SOCKS inbound in the
	// sing-box config and a route rule pinning it to this tag; see
	// config.SyncMtprotoSocksActive.
	Outbound string `toml:"outbound" json:"outbound"`

	// SocksPort is the loopback port that managed inbound binds. It is a
	// setting only so a collision with something else on the host is fixable;
	// nothing outside the box can reach it.
	SocksPort int `toml:"socks_port" json:"socks_port"`
}

// AwgSettings configures the RouteBox-owned AmneziaWG server interface.
type AwgSettings struct {
	Enabled    bool     `toml:"enabled" json:"enabled"`
	Interface  string   `toml:"interface" json:"interface"`
	Subnet     string   `toml:"subnet" json:"subnet"`
	ListenPort int      `toml:"listen_port" json:"listen_port"`
	MTU        int      `toml:"mtu" json:"mtu"`
	DNS        []string `toml:"dns" json:"dns"`
	// ClientKeepalive is the PersistentKeepalive handed to clients: "N" seconds or,
	// on AWG 3.0, a "lo-hi" range the peer redraws on every timer arm. "" => 25.
	ClientKeepalive  string `toml:"client_keepalive" json:"client_keepalive"`
	WANIface         string `toml:"wan_iface" json:"wan_iface"`
	Obf              AwgObf `toml:"obf" json:"obf"`
	ObfPreset        string `toml:"obf_preset" json:"obf_preset"`               // "off"|"dns"|"web"|"stealth"|"custom" — selects param ranges + client CPS mimicry
	Backend          string `toml:"backend" json:"backend"`                     // "kernel"|"singbox"; "" => singbox (kernel is opt-in only, never the auto-default)
	ServerHost       string `toml:"server_host" json:"server_host"`             // client-facing address of the AWG server (host/IP clients connect to); falls back to Server.PublicHost; router LAN/WAN IP typically
	HeaderProtection bool   `toml:"header_protection" json:"header_protection"` // AWG3 additional header protection toggle
	IPv6Broker       bool   `toml:"ipv6_broker" json:"ipv6_broker"`             // dual-stack IPv6 broker; gated by egress preflight
	Configured       bool   `toml:"configured" json:"configured"`               // sticky: set true after first successful Enable; drives wizard-vs-steady UI (never reset on Disable)
}

// AwgObf holds AmneziaWG obfuscation values. Numeric J/S fields; string H fields
// (digits or "lo-hi" range). The awg package validates these before they reach a conf.
type AwgObf struct {
	Jc   int    `toml:"jc" json:"jc"`
	Jmin int    `toml:"jmin" json:"jmin"`
	Jmax int    `toml:"jmax" json:"jmax"`
	S1   int    `toml:"s1" json:"s1"`
	S2   int    `toml:"s2" json:"s2"`
	S3   int    `toml:"s3" json:"s3"`
	S4   int    `toml:"s4" json:"s4"`
	H1   string `toml:"h1" json:"h1"`
	H2   string `toml:"h2" json:"h2"`
	H3   string `toml:"h3" json:"h3"`
	H4   string `toml:"h4" json:"h4"`
	// AWG3 params: string form (digits or "lo-hi" range), validated by the awg package.
	ContentPaddingAddition string `toml:"content_padding_addition" json:"content_padding_addition"`
	RekeyAfterTime         string `toml:"rekey_after_time" json:"rekey_after_time"`
	// AWG3 device-таймеры: строка (число или "lo-hi"-диапазон), валидируется awg-пакетом.
	RekeyTimeout         string `toml:"rekey_timeout" json:"rekey_timeout"`
	RejectAfterTime      string `toml:"reject_after_time" json:"reject_after_time"`
	KeepaliveTimeout     string `toml:"keepalive_timeout" json:"keepalive_timeout"`
	MaxHandshakeAttempts string `toml:"max_handshake_attempts" json:"max_handshake_attempts"`
}

// UpdatesSettings configures binary update checks
type UpdatesSettings struct {
	AutoCheck bool `toml:"auto_check" json:"auto_check"`
}

// GeoIPSettings configures GeoIP enrichment
type GeoIPSettings struct {
	Path    string `toml:"path" json:"path"`
	Enabled bool   `toml:"enabled" json:"enabled"`
}

// UISettings configures user interface defaults
type UISettings struct {
	Language  string `toml:"language" json:"language"`
	SpeedUnit string `toml:"speed_unit" json:"speed_unit"`
}

// MonitoringSettings configures monitoring features
type MonitoringSettings struct {
	EnrichmentEnabled bool `toml:"enrichment_enabled" json:"enrichment_enabled"`
}

// SecuritySettings configures access control
type SecuritySettings struct {
	AuthEnabled           bool   `toml:"auth_enabled" json:"auth_enabled"`
	AuthUsername          string `toml:"auth_username" json:"auth_username"`
	AuthPassword          string `toml:"auth_password" json:"-"`      // Never expose in JSON; cleared after migration
	AuthPasswordHash      string `toml:"auth_password_hash" json:"-"` // bcrypt hash; never expose in JSON
	CorsOrigins           string `toml:"cors_origins" json:"cors_origins"`
	SessionTimeoutMinutes int    `toml:"session_timeout_minutes" json:"session_timeout_minutes"`
	// TrustedProxies lists the reverse proxies (IPs or CIDRs) whose
	// X-Forwarded-For RouteBox may believe. It decides ONE thing: which address
	// login lockout and the subscription rate limiter are keyed on. Behind a
	// proxy with this unset, every client shares the proxy's address and
	// throttles as one — the subscription limiter counts every fetch, so a
	// handful of users is enough to trip it for all of them.
	//
	// File-level only, never PUT /api/settings: it decides whose word RouteBox
	// takes about who is calling, and an empty list is the safe default that a
	// request should not be able to widen.
	TrustedProxies []string `toml:"trusted_proxies" json:"trusted_proxies"`
}

// NetworkSettings configures network parameters
type NetworkSettings struct {
	Listen             string `toml:"listen" json:"listen"`
	ReadTimeoutSec     int    `toml:"read_timeout_sec" json:"read_timeout_sec"`
	CompressionEnabled bool   `toml:"compression_enabled" json:"compression_enabled"`
	TLSCertPath        string `toml:"tls_cert_path" json:"tls_cert_path"`
	TLSKeyPath         string `toml:"tls_key_path" json:"tls_key_path"`
	// Embedded ACME (Let's Encrypt) — issues/renews the panel cert in-process.
	// Domain = Server.PublicHost (no separate field). HTTP-01 challenge listens
	// on ACMEHTTPAddr (default ":80"). Must remain externally reachable on port
	// 80 for the challenge and renewals — in Docker that's usually satisfied by
	// mapping the host's port 80 to whatever ACMEHTTPAddr binds inside the
	// container, so the listener itself doesn't need a privileged port.
	ACMEEnabled  bool   `toml:"acme_enabled" json:"acme_enabled"`     // false = manual cert/key or plain HTTP
	ACMEEmail    string `toml:"acme_email" json:"acme_email"`         // LE account contact
	ACMEStaging  bool   `toml:"acme_staging" json:"acme_staging"`     // true = LE staging directory (testing)
	ACMECacheDir string `toml:"acme_cache_dir" json:"acme_cache_dir"` // cert/account cache (perm 0700)
	// ACMEHTTPAddr is the HTTP-01 challenge listen address; "" = ":80". File-
	// and flag-level only, like Listen and singbox.binary_path: it is a listen
	// address the process binds at startup, so an unbindable value does not
	// fail a request — it fails the next boot, in a loop under systemd's
	// Restart= or a container's restart policy, recoverable only by editing
	// the file by hand. Not a setting to hand to a PUT.
	ACMEHTTPAddr string `toml:"acme_http_addr" json:"acme_http_addr"`
}

// ServerSettings holds panel operating-mode configuration.
type ServerSettings struct {
	Mode       string `toml:"mode" json:"mode"`               // "router" (default) | "vps"
	PublicHost string `toml:"public_host" json:"public_host"` // domain or IP for client share-links; "" = unset
	PublicPort int    `toml:"public_port" json:"public_port"` // external panel port for sub-URLs; 0 = none/443
}

// SingboxSettings configures sing-box integration
type SingboxSettings struct {
	ConfigPath string `toml:"config_path" json:"config_path"`
	// BinaryPath pins the amnezia-box executable RouteBox manages (status,
	// version, start/stop, and the update swap) instead of auto-detecting it.
	// Empty => auto-detect. Like ConfigPath it is settable from the file or the
	// --binary flag only, never through PUT /api/settings: this path is exec'd,
	// so letting a request choose it would turn a panel session into arbitrary
	// code execution.
	BinaryPath string `toml:"binary_path" json:"binary_path"`
	ClashAPI   string `toml:"clash_api" json:"clash_api"`
	// V2RayAPI is the loopback gRPC StatsService listen address RouteBox writes
	// into experimental.v2ray_api and dials for per-user traffic. MUST be
	// loopback (never exposed). Empty => 127.0.0.1:8081.
	V2RayAPI string `toml:"v2ray_api" json:"v2ray_api"`
}

// AdvancedSettings for power users
type AdvancedSettings struct {
	WsPingIntervalSec int `toml:"ws_ping_interval_sec" json:"ws_ping_interval_sec"`
	WsPongTimeoutSec  int `toml:"ws_pong_timeout_sec" json:"ws_pong_timeout_sec"`
}

// Manager handles settings loading, saving, and runtime updates
type Manager struct {
	settings Settings
	path     string
	mu       sync.RWMutex
	guard    *util.WriteGuard
}

// IsReadOnly reports whether routebox.toml cannot be written, so the panel can
// show one read-only state for every file RouteBox persists.
func (m *Manager) IsReadOnly() bool { return m.guard.IsReadOnly() }

// Default returns settings with default values
func Default() Settings {
	return Settings{
		GeoIP: GeoIPSettings{
			Path:    "",
			Enabled: true,
		},
		UI: UISettings{
			Language:  "en",
			SpeedUnit: "bytes",
		},
		Monitoring: MonitoringSettings{
			EnrichmentEnabled: true,
		},
		Security: SecuritySettings{
			AuthEnabled:           false,
			AuthUsername:          "",
			AuthPassword:          "",
			CorsOrigins:           "",
			SessionTimeoutMinutes: 720,
		},
		Network: NetworkSettings{
			Listen:             "0.0.0.0:8080",
			ReadTimeoutSec:     30,
			CompressionEnabled: true,
			ACMEEnabled:        false,
			ACMEEmail:          "",
			ACMEStaging:        false,
			ACMECacheDir:       "/etc/routebox/acme",
			ACMEHTTPAddr:       ":80",
		},
		Singbox: SingboxSettings{
			ConfigPath: "",
			ClashAPI:   "",
			V2RayAPI:   "127.0.0.1:8081",
		},
		Advanced: AdvancedSettings{
			WsPingIntervalSec: 30,
			WsPongTimeoutSec:  10,
		},
		Server:  ServerSettings{Mode: "router", PublicHost: "", PublicPort: 0},
		Updates: UpdatesSettings{AutoCheck: true},
		Awg:     AwgSettings{Enabled: false, Interface: "awg-rb0", Subnet: "10.10.0.0/24", ListenPort: 51820, MTU: 1420, DNS: []string{"1.1.1.1"}, ObfPreset: "off", IPv6Broker: true},
		Mtproto: MtprotoSettings{
			Enabled: false,
			// Neither 443 nor 8443. A reverse proxy usually owns 443, and 8443
			// is the panel's own port in the Docker image (EXPOSE 8443), so
			// both would collide the first time this is enabled.
			Listen: "0.0.0.0:9443",
			// No default masking domain — it has to be a real host that
			// plausibly receives HTTPS from your users, and guessing one would
			// bake a wrong guess into every issued secret.
			MaskingDomain: "",
			// mtglib's own defaults, restated so the panel can show them.
			Concurrency:        4096,
			IdleTimeoutSec:     300,
			DomainFrontingPort: 443,
			// Telegram is reached directly until an operator picks an exit.
			Outbound: "",
			// Loopback-only, so the usual SOCKS port is free of the collision
			// risk that makes 1080 a poor choice on a public interface.
			SocksPort: 1080,
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
		// No existing file found at any candidate — default to the standard
		// system location so Save() works on a fresh VPS (no config file yet).
		if path == "" {
			path = "/etc/routebox/routebox.toml"
			m.path = path
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

	// After the path is resolved, not before: the guard's verdict is about the
	// file we will actually write.
	m.guard = util.NewWriteGuard(m.path)

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

	// Defense-in-depth: a hand-edited file could set singbox.v2ray_api to a
	// non-loopback address (e.g. "0.0.0.0:8081"), which would later be written
	// into experimental.v2ray_api.listen and dialed — exposing the
	// unauthenticated StatsService gRPC socket off-host. Update() validates this,
	// but Load() must too. Fail closed: reset any non-loopback value to the
	// default before committing.
	if tmp.Singbox.V2RayAPI != "" && validateLoopbackAddr(tmp.Singbox.V2RayAPI) != nil {
		log.Printf("settings: singbox.v2ray_api %q is not loopback; resetting to 127.0.0.1:8081", tmp.Singbox.V2RayAPI)
		tmp.Singbox.V2RayAPI = "127.0.0.1:8081"
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
// fsync, chmod 0600, rename over target). A failure that is about writability
// comes back as util.ErrReadOnly naming routebox.toml, so the API answers 409
// with something the operator can act on instead of a raw 500. The caller must
// hold m.mu.Lock (exclusive).
func (m *Manager) saveLocked() error {
	if m.path == "" {
		return fmt.Errorf("no settings path configured")
	}
	return m.guard.Note(m.writeLocked())
}

func (m *Manager) writeLocked() error {
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

// SetSingboxConfigPath запоминает путь конфига, на который RouteBox перешёл, и
// тут же сохраняет настройки: смена пути на лету живёт только в памяти, а при
// следующем старте резолв снова возьмёт singbox.config_path из TOML — и
// расхождение, которое пользователь только что вылечил, вернётся.
//
// Намеренно отдельный метод, а не ключ generic Update: тот обслуживает
// PUT /api/settings, и пускать туда путь конфига значило бы разрешить сменить
// его мимо всякого переключения на лету — то есть создать расхождение к
// следующей загрузке одним PUT'ом.
//
// Ошибка означает «в памяти сменили, на диск не записали»: путь уже в работе,
// поэтому откатывать память нельзя — она обязана совпадать с тем, что RouteBox
// правит прямо сейчас. Вызывающий обязан сказать пользователю, что смена не
// закреплена.
func (m *Manager) SetSingboxConfigPath(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.settings.Singbox.ConfigPath = path
	if err := m.saveLocked(); err != nil {
		// Только факт: какой путь не лёг на диск и почему. Что из этого следует
		// для пользователя (после перезапуска вернётся старый путь, починку
		// придётся повторить) — забота вызывающего, у него это сказано на языке
		// пользователя, и повторять то же самое здесь по-английски значило бы
		// показать одно следствие дважды.
		return fmt.Errorf("saving %s to the settings file: %w", path, err)
	}
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

// validateKeepaliveRange checks a PersistentKeepalive value: "" (unset => the
// 25s default), "N", or the AWG 3.0 "lo-hi" range, both bounds uint16 seconds
// with lo <= hi. The general UintRange parser lives in the awg package, which
// settings must not import (awg imports settings) — and a value rejected here is
// a value the operator gets told about instead of one that silently reverts.
func validateKeepaliveRange(s string) error {
	if s == "" {
		return nil
	}
	bad := fmt.Errorf("keepalive must be seconds (\"25\") or a range (\"22-30\"), 0-65535")
	parts := strings.Split(s, "-")
	if len(parts) > 2 {
		return bad
	}
	bounds := make([]uint64, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 10, 16)
		if err != nil {
			return bad
		}
		bounds[i] = v
	}
	if len(bounds) == 2 && bounds[0] > bounds[1] {
		return fmt.Errorf("keepalive range %q: lower bound is above the upper one", s)
	}
	return nil
}

// SanitizePublicHost normalizes a user-entered public host for share links:
// strips an optional scheme, userinfo, path, query, port, and a single trailing
// FQDN dot, leaving a bare hostname or IP literal. Empty input is allowed
// (clears the setting). A value that is neither a valid hostname nor IP is
// rejected. Exported so the api package's share-link handler can apply the same
// validation to a query-provided host.
func SanitizePublicHost(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", nil
	}
	// Strip scheme by parsing as URL when a scheme is present. url.Parse already
	// drops userinfo into u.Host's User component, so u.Host is authority only.
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("invalid host %q", in)
		}
		s = u.Host // host[:port]
	}
	// Strip a trailing path if any survived (no-scheme case e.g. "host/path").
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	// Drop userinfo: a host must never contain "@". In the no-scheme case
	// net.SplitHostPort would otherwise split on the first colon and silently
	// accept "user:pass@host" as "user". Mirror URL semantics by keeping only
	// the substring after the LAST "@".
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	// Strip a port if present (handles bracketed IPv6 too).
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	} else {
		// Not host:port — strip surrounding brackets for a bare IPv6 literal.
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	}
	s = strings.TrimSpace(s)
	// Strip a single trailing dot so a legal absolute FQDN is accepted.
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return "", fmt.Errorf("invalid host %q", in)
	}
	// Validate: either a parseable IP or a plausible hostname.
	if net.ParseIP(s) != nil {
		return s, nil
	}
	if !isPlausibleHostname(s) {
		return "", fmt.Errorf("invalid host %q", in)
	}
	return s, nil
}

// validateLoopbackAddr ensures a host:port address binds a loopback interface
// only. The v2ray_api StatsService exposes per-user traffic over an
// unauthenticated gRPC socket and MUST never be reachable off-host. Accepts a
// bracketed IPv6 literal (net.SplitHostPort strips the brackets).
func validateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid host:port address %q", addr)
	}
	if port == "" {
		return fmt.Errorf("address %q missing port", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("address %q must be loopback (127.0.0.0/8 or ::1)", addr)
	}
	return nil
}

func isPlausibleHostname(s string) bool {
	if len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" {
			return false
		}
		// RFC 952/1123: a label must not begin or end with a hyphen.
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
				!(r >= '0' && r <= '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

// Update applies partial updates to settings (runtime-safe fields only).
//
// All-or-nothing: keys are validated and staged against a private copy, and
// m.settings is replaced only once the LAST key has been accepted. A payload
// with one bad key therefore leaves the settings byte-for-byte as they were,
// instead of leaving a subset applied for the next successful Save() to flush
// to disk. The live trigger is a browser on a cached SPA build posting keys
// this build no longer knows.
//
// An unknown key stays an error — tolerating one would swallow typos.
//
// Keys are walked in sorted order so that a payload carrying several bad keys
// always reports the same one, rather than whichever the map handed over
// first. Nothing about correctness depends on the order — it exists so the
// error a user sees is reproducible.
//
// Staging is deep enough to be safe on its own: the struct copy is shallow, so
// the one reference-typed field is cloned below and every case then writes
// only into memory m.settings does not share.
func (m *Manager) Update(updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	staged := m.settings
	// Awg.DNS is the only slice/map in Settings; the shallow copy above would
	// otherwise share its backing array with m.settings. Today every case
	// replaces the slice wholesale, so the clone is redundant — it is here so
	// that a future case writing THROUGH the slice cannot break atomicity
	// silently. Cheap: a handful of resolver addresses.
	staged.Awg.DNS = slices.Clone(staged.Awg.DNS)

	// Deferred until every key has passed: bcrypt is expensive and hashing is
	// the one transform here with a cost worth not paying for a doomed update.
	newPassword := ""

	// Validate and stage each runtime-safe field
	for _, key := range slices.Sorted(maps.Keys(updates)) {
		value := updates[key]
		switch key {
		// GeoIP runtime settings
		case "geoip.enabled":
			if v, ok := value.(bool); ok {
				staged.GeoIP.Enabled = v
			}

		// UI runtime settings
		case "ui.language":
			if v, ok := value.(string); ok {
				staged.UI.Language = v
			}
		case "ui.speed_unit":
			if v, ok := value.(string); ok {
				staged.UI.SpeedUnit = v
			}

		// Monitoring runtime settings
		case "monitoring.enrichment_enabled":
			if v, ok := value.(bool); ok {
				staged.Monitoring.EnrichmentEnabled = v
			}

		// Security runtime settings
		case "security.auth_enabled":
			if v, ok := value.(bool); ok {
				staged.Security.AuthEnabled = v
			}
		case "security.auth_username":
			if v, ok := value.(string); ok {
				staged.Security.AuthUsername = v
			}
		case "security.session_timeout_minutes":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Security.SessionTimeoutMinutes = v
		case "security.auth_password":
			// Hashed after the loop; a blank or non-string value is ignored,
			// as before.
			if v, ok := value.(string); ok && v != "" {
				newPassword = v
			}
		case "network.tls_cert_path":
			if v, ok := value.(string); ok {
				staged.Network.TLSCertPath = v
			}
		case "network.tls_key_path":
			if v, ok := value.(string); ok {
				staged.Network.TLSKeyPath = v
			}
		case "network.acme_enabled":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Network.ACMEEnabled = v
		case "network.acme_email":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Network.ACMEEmail = v
		case "network.acme_staging":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Network.ACMEStaging = v
		case "network.acme_cache_dir":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Network.ACMECacheDir = v
		case "server.mode":
			// Fix 4: reject invalid values instead of silently ignoring them.
			v, ok := value.(string)
			if !ok || (v != "router" && v != "vps") {
				return fmt.Errorf("invalid server.mode %q (want router|vps)", value)
			}
			staged.Server.Mode = v
		case "server.public_host":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			host, err := SanitizePublicHost(v)
			if err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			staged.Server.PublicHost = host

		case "server.public_port":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			if v < 0 || v > 65535 {
				return fmt.Errorf("setting %s: port %d out of range (0-65535)", key, v)
			}
			staged.Server.PublicPort = v

		// Advanced runtime settings
		case "advanced.ws_ping_interval_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Advanced.WsPingIntervalSec = v
		case "advanced.ws_pong_timeout_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Advanced.WsPongTimeoutSec = v

		// Updates runtime settings
		case "updates.auto_check":
			if v, ok := value.(bool); ok {
				staged.Updates.AutoCheck = v
			}

		// Singbox runtime settings
		case "singbox.v2ray_api":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			if err := validateLoopbackAddr(v); err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			staged.Singbox.V2RayAPI = v

		// AmneziaWG runtime settings. awg.interface is set at enable time
		// (validated by the awg orchestrator), not via the generic Update.
		case "awg.enabled":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Awg.Enabled = v
		case "awg.backend":
			v, ok := value.(string)
			if !ok || (v != "kernel" && v != "singbox") {
				return fmt.Errorf("invalid awg.backend %q (want kernel|singbox)", value)
			}
			// Guard "only switch while disabled" is enforced by the caller (handler),
			// which has the live server status; settings stays status-agnostic.
			staged.Awg.Backend = v
		case "awg.header_protection":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Awg.HeaderProtection = v
		case "awg.ipv6_broker":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Awg.IPv6Broker = v
		case "awg.subnet":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Awg.Subnet = v
		case "awg.listen_port":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Awg.ListenPort = v
		case "awg.mtu":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Awg.MTU = v
		case "awg.server_host":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			host, err := SanitizePublicHost(v)
			if err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			staged.Awg.ServerHost = host
		case "awg.wan_iface":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Awg.WANIface = v
		case "awg.dns":
			arr, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("setting %s: value must be an array", key)
			}
			dns := make([]string, 0, len(arr))
			for _, e := range arr {
				s, ok := e.(string)
				if !ok {
					return fmt.Errorf("setting %s: entries must be strings", key)
				}
				dns = append(dns, s)
			}
			staged.Awg.DNS = dns
		case "awg.client_keepalive":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			ka := strings.TrimSpace(v)
			if err := validateKeepaliveRange(ka); err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			staged.Awg.ClientKeepalive = ka
		case "awg.obf_preset":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Awg.ObfPreset = v
		case "awg.configured":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Awg.Configured = v
		case "awg.obf":
			// Re-marshal the nested object into AwgObf (driftless: one decode).
			raw, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			var obf AwgObf
			if err := json.Unmarshal(raw, &obf); err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
			staged.Awg.Obf = obf

		// Telegram MTProto proxy runtime settings. Malformed values are an
		// error rather than a silent no-op: the panel would otherwise report a
		// successful save for a setting that never changed.
		case "mtproto.enabled":
			v, ok := value.(bool)
			if !ok {
				return fmt.Errorf("setting %s: value must be a boolean", key)
			}
			staged.Mtproto.Enabled = v
		case "mtproto.listen":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Mtproto.Listen = v
		case "mtproto.masking_domain":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Mtproto.MaskingDomain = v
		case "mtproto.public_host":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Mtproto.PublicHost = v
		case "mtproto.public_port":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Mtproto.PublicPort = v
		case "mtproto.concurrency":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Mtproto.Concurrency = v
		case "mtproto.idle_timeout_sec":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Mtproto.IdleTimeoutSec = v
		case "mtproto.prefer_ip":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Mtproto.PreferIP = v
		case "mtproto.domain_fronting_port":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			staged.Mtproto.DomainFrontingPort = v
		case "mtproto.outbound":
			v, ok := value.(string)
			if !ok {
				return fmt.Errorf("setting %s: value must be a string", key)
			}
			staged.Mtproto.Outbound = v
		case "mtproto.socks_port":
			v, ok := toInt(value)
			if !ok {
				return fmt.Errorf("setting %s: value must be a whole number", key)
			}
			if v != 0 && (v < 1 || v > 65535) {
				return fmt.Errorf("setting %s: must be a port between 1 and 65535", key)
			}
			staged.Mtproto.SocksPort = v

		default:
			return fmt.Errorf("unknown or non-runtime setting: %s", key)
		}
	}

	// Every key accepted — run the deferred transform, then commit. The commit
	// is a single assignment under the write lock, so a Get() (RLock) never
	// observes a half-applied update.
	if newPassword != "" {
		h, err := auth.HashPassword(newPassword)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		staged.Security.AuthPasswordHash = h
		staged.Security.AuthPassword = ""
	}

	m.settings = staged
	return nil
}

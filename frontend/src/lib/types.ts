// Toast notification
export interface Toast {
	id: string;
	type: 'success' | 'error' | 'warning' | 'info';
	message: string;
	duration: number;
}

// API Response
export interface ApiResponse<T = unknown> {
	success: boolean;
	data?: T;
	error?: string;
}

// sing-box version and feature flags
export interface SingBoxVersion {
	version: string;
	features: Record<string, boolean>;
}

// System requirements check
export interface SystemChecks {
	ipv4_forward: boolean;
	ipv6_forward: boolean;
	ipv6_disabled: boolean; // IPv6 completely disabled at kernel level
	is_root: boolean;
	all_checks_passed: boolean;
}

// Process status
export interface ProcessStatus {
	running: boolean;
	pid?: number;
	uptime?: string;
	managed_by?: 'systemd' | 'standalone' | '';
	service_name?: string;
	supports_hup?: boolean;
	version?: string;
	binary_path?: string;
	system_checks?: SystemChecks;
	/**
	 * Which config file each of the three sources points at. Always present.
	 */
	config_paths?: ConfigPaths;
	/**
	 * Set when RouteBox cannot write the config file or its directory. Every
	 * write endpoint answers 409 while this is up, so the UI blocks saving
	 * up front instead of letting the user find out after the click.
	 */
	config_read_only?: boolean;
	/** The config path RouteBox failed to open for writing. Empty unless read-only. */
	config_read_only_path?: string;
	/**
	 * Every file RouteBox cannot write right now: the sing-box config plus its own
	 * state files (routebox.toml, users.toml, subscriptions.toml, clients.toml,
	 * peers.toml). They sit in different directories, so this is not derivable
	 * from `config_read_only` — the config can be fine while a store is not.
	 * Absent when everything is writable.
	 */
	read_only_paths?: string[];
}

/**
 * Which config file each source points at: the one RouteBox edits (`ours`), the
 * one in the systemd unit's ExecStart (`unit`) and the one the live process was
 * started with (`process`). An empty field means that source does not exist —
 * no unit, or nothing running — never "it agrees".
 *
 * The two verdicts are computed by the backend (it resolves symlinks) and mean
 * different things because they are cured differently: `unit_mismatch` sends
 * every edit into a file the process is never given, and blocks
 * Start/Restart/Reload until resolved; `process_mismatch` means the running
 * process is still reading the file it was started with, which only a restart
 * can change — so it blocks nothing.
 */
export interface ConfigPaths {
	ours: string;
	unit?: string;
	process?: string;
	unit_mismatch: boolean;
	process_mismatch: boolean;
	/**
	 * The systemd drop-in RouteBox installed to repoint the unit, when the file
	 * is on disk. Absent means there is none — the backend reads the file on
	 * every status poll rather than remembering a past fix, so this survives a
	 * panel restart exactly as the file does.
	 */
	drop_in?: ConfigDropIn;
}

/**
 * The one file RouteBox writes outside its own: a drop-in that replaces the
 * unit's `ExecStart` with the same command pointed at `config_path`.
 *
 * `pending_reload` means the file is written but the unit is still starting
 * amnezia-box with something else — the state a failed `daemon-reload` leaves
 * behind. It applies at the next reload or reboot, so it is never silent.
 */
export interface ConfigDropIn {
	path: string;
	config_path?: string;
	pending_reload: boolean;
}

// Setup wizard check response
export interface NeedsSetupResponse {
	needs_setup: boolean;
	reason: string;
	binary_installed: boolean;
	running: boolean;
	has_vpn_config: boolean;
	clash_api_configured: boolean;
	version?: string;
	binary_path?: string;
}

// Sing-box config types
export interface SingboxConfig {
	log?: LogConfig;
	dns?: DnsConfig;
	inbounds?: Inbound[];
	outbounds?: Outbound[];
	endpoints?: Endpoint[];
	route?: RouteConfig;
	experimental?: ExperimentalConfig;
}

export interface LogConfig {
	level?: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal' | 'panic';
	output?: string;
	timestamp?: boolean;
}

export interface LogSettings {
	level: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal' | 'panic';
	timestamp?: boolean;
	output?: string;
}

export interface DnsConfig {
	servers?: DnsServer[];
	rules?: DnsRule[];
	strategy?: 'prefer_ipv4' | 'prefer_ipv6' | 'ipv4_only' | 'ipv6_only';
	disable_cache?: boolean;
	disable_expire?: boolean;
	independent_cache?: boolean;
	final?: string;
	// New fields from sing-box docs
	cache_capacity?: number;      // Max cache entries
	reverse_mapping?: boolean;    // IP → domain reverse mapping
	client_subnet?: string;       // EDNS Client Subnet
}

export interface DnsServer {
	tag: string;
	type: 'udp' | 'tcp' | 'tls' | 'https' | 'local' | 'fakeip';
	server?: string;
	server_port?: number;
	detour?: string;
	// Domain resolver - required when server is a domain name
	domain_resolver?: string;
	domain_strategy?: 'prefer_ipv4' | 'prefer_ipv6' | 'ipv4_only' | 'ipv6_only';
	// FakeIP specific
	inet4_range?: string;
	inet6_range?: string;
}

export interface DnsRule {
	// Match conditions
	domain?: string[];
	domain_suffix?: string[];
	domain_keyword?: string[];
	domain_regex?: string[];
	ip_cidr?: string[];
	query_type?: string[];
	rule_set?: string[];
	// Action
	action?: 'route' | 'reject' | 'predefined';
	server?: string;              // Required only for action: route
	// Reject action options
	method?: 'default' | 'drop';  // default = reply REFUSED, drop = silently drop
	no_drop?: boolean;            // Prevent auto-switching to drop after 50 triggers
	// Predefined action options (sing-box 1.12+)
	rcode?: 'NOERROR' | 'FORMERR' | 'SERVFAIL' | 'NXDOMAIN' | 'NOTIMP' | 'REFUSED';
	answer?: string[];            // DNS records to respond as answers
	// Common options
	disable_cache?: boolean;
}

export interface DnsSettings {
	strategy?: 'prefer_ipv4' | 'prefer_ipv6' | 'ipv4_only' | 'ipv6_only';
	disable_cache?: boolean;
	disable_expire?: boolean;
	independent_cache?: boolean;
	final?: string;
	// New fields from sing-box docs
	cache_capacity?: number;      // Max cache entries
	reverse_mapping?: boolean;    // IP → domain reverse mapping
	client_subnet?: string;       // EDNS Client Subnet
}

export interface Inbound {
	type: string;
	tag: string;
	listen?: string;
	listen_port?: number;
	// TUN specific
	interface_name?: string;
	address?: string[];          // NEW: unified address array (sing-box 1.8+)
	mtu?: number;
	auto_route?: boolean;
	auto_redirect?: boolean;
	strict_route?: boolean;
	stack?: 'system' | 'gvisor' | 'mixed';
	// DEPRECATED: keep for backward compat
	inet4_address?: string;
	inet6_address?: string;
	// Common
	tcp_fast_open?: boolean;
	// Server inbound fields
	users?: ServerInboundUser[];
	tls?: ServerTlsConfig;
	up_mbps?: number;     // hysteria2
	down_mbps?: number;   // hysteria2
	obfs?: { type: string; password?: string; min_packet_size?: number; max_packet_size?: number }; // hysteria2
	transport?: TransportConfig | string; // vless/trojan stream transport object; mieru emits a "TCP"/"UDP" string
	traffic_pattern?: string;           // mieru
	user_hint_is_mandatory?: boolean;   // mieru
	// mieru: extra "lo-hi" ranges the server binds on top of listen_port. Either
	// may be omitted as long as one is set.
	listen_ports?: string[];
}

export interface ServerInboundUser {
	name?: string;       // vless / hysteria2
	uuid?: string;       // vless
	flow?: string;       // vless
	username?: string;   // naive
	password?: string;   // naive / hysteria2
}

export interface PanelBinding {
	inbound_tag: string;
	credential: string;
	protocol: string;
	name: string;
	flow: string;
}

export interface PanelUser {
	id: string;
	name: string;
	enabled: boolean;
	expires_at: number;
	pending: boolean;
	token?: string;
	token_disabled?: boolean;
	bindings: PanelBinding[];
	upload?: number;
	download?: number;
}

export interface UserTrafficPoint {
	ts: number;
	upload: number;
	download: number;
}

export interface UserTrafficResponse {
	upload: number;
	download: number;
	history: UserTrafficPoint[];
}

export interface ServerRealityConfig {
	enabled: boolean;
	private_key?: string;
	short_id?: string;
	handshake?: { server: string; server_port: number };
}

export interface ServerTlsConfig {
	enabled?: boolean;
	server_name?: string;
	acme?: { domain: string; email: string };
	reality?: ServerRealityConfig;
	certificate_path?: string;
	key_path?: string;
}

// TLS configuration for outbound proxies
export interface TLSConfig {
	enabled?: boolean;
	server_name?: string;
	insecure?: boolean;
	alpn?: string[];
	min_version?: string;
	max_version?: string;
	cipher_suites?: string[];
	certificate_path?: string;
	certificate?: string[];
	key_path?: string;
	utls?: {
		enabled?: boolean;
		fingerprint?: string;
	};
	reality?: {
		enabled?: boolean;
		public_key?: string;
		short_id?: string;
	};
	ech?: {
		enabled?: boolean;
		config?: string[];
	};
}

// Multiplex configuration
export interface MultiplexConfig {
	enabled?: boolean;
	protocol?: 'smux' | 'yamux' | 'h2mux';
	max_connections?: number;
	min_streams?: number;
	max_streams?: number;
	padding?: boolean;
	brutal?: {
		enabled?: boolean;
		up_mbps?: number;
		down_mbps?: number;
	};
}

// Transport configuration for VLESS/VMess
export interface TransportConfig {
	type: 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade' | 'xhttp';
	path?: string;
	headers?: Record<string, string>;
	host?: string[];
	service_name?: string;
	idle_timeout?: string;
	ping_timeout?: string;
	// xhttp only, and NOT optional there: the fork refuses to load a config whose
	// xhttp transport has no padding range. See XHTTP_DEFAULT_PADDING.
	x_padding_bytes?: string;
}

// Obfuscation configuration for Hysteria2
export type ObfsType = 'salamander' | 'gecko';
export interface ObfsConfig {
	type: ObfsType;
	password: string;
	// gecko only: packet size bounds. Both ends must use the same values;
	// unset means hysteria's own defaults (512 / 1200).
	min_packet_size?: number;
	max_packet_size?: number;
}

// Base outbound interface (all outbounds share these)
export interface Outbound {
	type: string;
	tag: string;
	// For selector/urltest
	outbounds?: string[];
	default?: string;
	interrupt_exist_connections?: boolean;
	// URLTest specific
	url?: string;           // Test URL (default: https://www.gstatic.com/generate_204)
	interval?: string;      // Test interval (e.g., "3m")
	tolerance?: number;     // Tolerance in ms
	idle_timeout?: string;  // Idle timeout (e.g., "30m")
	// For direct
	// (no extra fields)
	// For vless/hy2
	server?: string;
	server_port?: number;
	// Hysteria2 specific
	server_ports?: string[];  // Port hopping ranges (e.g., ["2080:3000", "4000:5000"])
	hop_interval?: string;  // Port hop interval (e.g., "30s")
	up_mbps?: number;       // Upload limit
	down_mbps?: number;     // Download limit
	// Shadowsocks
	method?: string;
	password?: string;
	plugin?: string;
	plugin_opts?: string;
	network?: string;
	udp_over_tcp?: boolean;
	multiplex?: MultiplexConfig;
	// ShadowTLS
	version?: number;
	tls?: TLSConfig;
	detour?: string;
	// AnyTLS
	idle_session_check_interval?: string;
	idle_session_timeout?: string;
	min_idle_session?: number;
	// Naive specific
	username?: string;
	insecure_concurrency?: number;
	extra_headers?: Record<string, string>;
	quic?: boolean;
	quic_congestion_control?: string;
	// VLESS specific
	uuid?: string;
	flow?: string;
	packet_encoding?: string;
	// vless/trojan: stream transport object; mieru: plain string "TCP"|"UDP"
	transport?: TransportConfig | string;
	// Mieru specific
	multiplexing?: string;      // MULTIPLEXING_{DEFAULT,OFF,LOW,MIDDLE,HIGH}
	traffic_pattern?: string;   // opaque base64
	// Hysteria2 obfuscation
	obfs?: ObfsConfig;
	// Domain resolver (sing-box 1.12+) - DNS for resolving server domain
	domain_resolver?: string;
}

// Protocol-specific outbound types for type-safe access
export interface OutboundDirect extends Outbound {
	type: 'direct';
}

export interface OutboundBlock extends Outbound {
	type: 'block';
}

export interface OutboundDns extends Outbound {
	type: 'dns';
}

export interface OutboundSelector extends Outbound {
	type: 'selector';
	outbounds: string[];
	default?: string;
	interrupt_exist_connections?: boolean;
}

export interface OutboundUrltest extends Outbound {
	type: 'urltest';
	outbounds: string[];
	url?: string;
	interval?: string;
	tolerance?: number;
	idle_timeout?: string;
}

export interface OutboundVless extends Outbound {
	type: 'vless';
	server: string;
	server_port: number;
	uuid: string;
	flow?: string;
	packet_encoding?: string;
	tls?: TLSConfig;
	transport?: TransportConfig;
}

export interface OutboundHysteria2 extends Outbound {
	type: 'hysteria2';
	server: string;
	server_port: number;
	password: string;
	up_mbps?: number;
	down_mbps?: number;
	server_ports?: string[];
	hop_interval?: string;
	obfs?: ObfsConfig;
	tls?: TLSConfig;
}

export interface OutboundShadowsocks extends Outbound {
	type: 'shadowsocks';
	server: string;
	server_port: number;
	method: string;
	password: string;
	plugin?: string;
	plugin_opts?: string;
	network?: string;
	udp_over_tcp?: boolean;
	multiplex?: MultiplexConfig;
}

export interface OutboundShadowtls extends Outbound {
	type: 'shadowtls';
	server: string;
	server_port: number;
	version: number;
	password?: string;
	tls?: TLSConfig;
	detour?: string;
}

export interface OutboundAnytls extends Outbound {
	type: 'anytls';
	server: string;
	server_port: number;
	password: string;
	tls?: TLSConfig;
	idle_session_check_interval?: string;
	idle_session_timeout?: string;
	min_idle_session?: number;
}

export interface OutboundNaive extends Outbound {
	type: 'naive';
	server: string;
	server_port: number;
	username?: string;
	password?: string;
	tls?: TLSConfig;
	insecure_concurrency?: number;
	extra_headers?: Record<string, string>;
	quic?: boolean;
	quic_congestion_control?: string;
	udp_over_tcp?: boolean;
}

export interface OutboundMieru extends Outbound {
	type: 'mieru';
	server: string;
	server_port?: number;
	server_ports?: string[]; // dash ranges only, degenerate "N-N" for singles
	transport: 'TCP' | 'UDP';
	username: string;
	password: string;
	multiplexing?: string; // MULTIPLEXING_{DEFAULT,OFF,LOW,MIDDLE,HIGH}
	traffic_pattern?: string; // opaque base64
}

// Discriminated union for all typed outbounds
export type OutboundTyped =
	| OutboundDirect
	| OutboundBlock
	| OutboundDns
	| OutboundSelector
	| OutboundUrltest
	| OutboundVless
	| OutboundHysteria2
	| OutboundShadowsocks
	| OutboundShadowtls
	| OutboundAnytls
	| OutboundNaive
	| OutboundMieru;

export interface Endpoint {
	type: string;
	tag: string;
	// AWG/WireGuard specific
	system?: boolean;              // Use system interface
	name?: string;                 // Interface name override
	private_key?: string;
	address?: string[];
	mtu?: number;
	listen_port?: number;
	peers?: AWGPeer[];
	udp_timeout?: string;          // UDP timeout (e.g., "5m")
	workers?: number;              // Worker count
	// AWG obfuscation
	jc?: number;
	jmin?: number;
	jmax?: number;
	s1?: number;
	s2?: number;
	s3?: number;
	s4?: number;
	h1?: string;
	h2?: string;
	h3?: string;
	h4?: string;
	i1?: string;
	i2?: string;
	i3?: string;
	i4?: string;
	i5?: string;
	// AWG3
	header_protection_key?: string;
	content_padding_addition?: string; // UintRange: "N" or "lo-hi"
	rekey_after_time?: string;         // UintRange: "N" or "lo-hi" (seconds)
	// AWG3 device timers: UintRange strings ("N" or "lo-hi"); seconds (max_handshake_attempts = count)
	rekey_timeout?: string;
	reject_after_time?: string;
	keepalive_timeout?: string;
	max_handshake_attempts?: string;
	// Dialer options
	detour?: string;
	bind_interface?: string;
	routing_mark?: number;
	connect_timeout?: string;
	// Domain resolver (sing-box 1.12+) - DNS for resolving peer domains
	domain_resolver?: string;
}

export interface AWGPeer {
	address: string;
	port: number;
	public_key: string;
	preshared_key?: string;
	allowed_ips: string[];
	// Seconds, or an AWG 3.0 "lo-hi" range the device redraws every time it arms
	// the timer. Pre-3.0 configs carry a number; the fork reads both.
	persistent_keepalive_interval?: number | string;
	reserved?: number[];      // 3-byte reserved field for WARP
}

export interface RouteConfig {
	auto_detect_interface?: boolean;
	default_domain_resolver?: string;
	final?: string;
	rules?: RouteRule[];
	rule_set?: RuleSet[];
}

export interface RouteRule {
	// Action
	action?: 'route' | 'reject' | 'sniff' | 'hijack-dns' | 'route-options' | 'resolve';
	outbound?: string;

	// Sniff action options
	timeout?: string;  // e.g., "300ms"

	// Reject action options
	method?: 'default' | 'drop';
	no_drop?: boolean;

	// Resolve action options
	server?: string;
	strategy?: 'prefer_ipv4' | 'prefer_ipv6' | 'ipv4_only' | 'ipv6_only';

	// Route / route-options action options
	override_address?: string;
	override_port?: number;
	network_strategy?: string;
	network_type?: string[];
	fallback_network_type?: string[];
	fallback_delay?: string;
	udp_connect?: boolean;
	udp_timeout?: string;
	udp_disable_domain_unmapping?: boolean;

	// TLS Fragment (≥1.12)
	tls_fragment?: boolean;
	tls_fragment_fallback_delay?: string;
	tls_record_fragment?: boolean;

	// Logical rule support
	type?: 'default' | 'logical';
	mode?: 'and' | 'or';
	rules?: RouteRule[];
	invert?: boolean;

	// Inbound filter
	inbound?: string[];

	// Match conditions - destination
	ip_is_private?: boolean;
	source_ip_is_private?: boolean;
	domain?: string[];
	domain_suffix?: string[];
	domain_keyword?: string[];
	domain_regex?: string[];
	ip_cidr?: string[];
	source_ip_cidr?: string[];
	port?: number[];
	port_range?: string[];
	source_port?: number[];
	source_port_range?: string[];
	protocol?: string[];
	rule_set?: string[];
	process_name?: string[];
	process_path?: string[];
	process_path_regex?: string[];
	network?: 'tcp' | 'udp' | 'icmp';
	ip_version?: number;           // 4 | 6
	clash_mode?: string;
	rule_set_ip_cidr_match_source?: boolean;
	client?: string[];
	auth_user?: string[];
	user?: string[];
	user_id?: number[];
}

export interface RouteSettings {
	final: string;
	auto_detect_interface?: boolean;
	default_interface?: string;
	default_mark?: number;
	default_domain_resolver?: string;
	default_domain_strategy?: string;
	default_network_strategy?: string;
	default_network_type?: string[];
	default_fallback_network_type?: string[];
	default_fallback_delay?: string;
}

export interface RuleSet {
	tag: string;
	type: 'remote' | 'local' | 'inline';
	format?: 'binary' | 'source';
	url?: string;
	path?: string;
	// Remote-specific options
	download_detour?: string;    // Outbound for downloading
	update_interval?: string;    // e.g., "24h"
	// Inline-specific options (≥1.10)
	rules?: HeadlessRule[];
}

// Headless rule: matching conditions only (no action/outbound)
// Used for inline rule-sets compilation - limited set of fields
export interface HeadlessRule {
	domain?: string[];
	domain_suffix?: string[];
	domain_keyword?: string[];
	domain_regex?: string[];
	ip_cidr?: string[];
	source_ip_cidr?: string[];
	port?: number[];
	port_range?: string[];
	process_name?: string[];
	process_path?: string[];
}

// Full rule conditions for route rules UI
// Extends HeadlessRule with additional fields not supported in rule-sets
export interface RuleConditions extends HeadlessRule {
	// Inbound filter
	inbound?: string[];

	// Boolean flags
	ip_is_private?: boolean;
	source_ip_is_private?: boolean;
	invert?: boolean;

	// Source conditions
	source_port?: number[];
	source_port_range?: string[];

	// Protocol & network
	protocol?: string[];
	network?: 'tcp' | 'udp' | 'icmp';
	ip_version?: number; // 4 | 6

	// Rule sets
	rule_set?: string[];
	rule_set_ip_cidr_match_source?: boolean;

	// Process (additional)
	process_path_regex?: string[];

	// Clash integration
	clash_mode?: string;
	client?: string[];

	// User matching (Linux)
	auth_user?: string[];
	user?: string[];
	user_id?: number[];
}

// Rule set usage: which route/dns rules reference each rule set
export interface RuleSetUsage {
	route_rules: number[];
	dns_rules: number[];
}

// Domain set (custom rule set source) info
export interface DomainSetInfo {
	tag: string;
	domain_count: number;
	rules_count: number;
}

export interface Subscription {
	id: string;
	name: string;
	url: string;
	interval_hrs: number;
	last_updated: number;
	last_error: string;
	node_count: number;
}

export interface SubscriptionInput {
	name: string;
	url: string;
	interval_hrs: number;
}

// Client (LAN device) entry
export interface ClientEntry {
	ip: string;
	name: string;
	note: string;
	first_seen: number;
	last_seen: number;
	online: boolean;
}

// Traffic history (aggregated buckets for breakdowns)
export interface TrafficBucket {
	source: string;
	domain: string;
	chain: string;
	upload: number;
	download: number;
}

export interface TrafficHistoryResponse {
	range: string;
	start_ts: number;
	end_ts: number;
	buckets: TrafficBucket[];
}

export type TrafficRange = '1h' | '3h' | '24h' | 'week' | 'month';

// Rule set source file (JSON format for sing-box compilation)
export interface RuleSetSource {
	version: number;
	rules: HeadlessRule[];
}

export interface CacheFileSettings {
	enabled?: boolean;
	path?: string;
	cache_id?: string;
	store_fakeip?: boolean;
	store_rdrc?: boolean;
}

export interface ClashApiSettings {
	external_controller?: string;
	external_ui?: string;
	external_ui_download_url?: string;
	external_ui_download_detour?: string;
	secret?: string;
	default_mode?: 'rule' | 'global' | 'direct';
}

export interface ExperimentalSettings {
	cache_file?: CacheFileSettings;
	clash_api?: ClashApiSettings;
}

export interface ExperimentalConfig {
	cache_file?: CacheFileSettings;
	clash_api?: ClashApiSettings;
}

// Clash API types
export interface ClashProxy {
	name: string;
	type: string;
	now?: string;
	all?: string[];
	history?: { delay: number; time: string }[];
}

// GeoIP information
export interface GeoInfo {
	country_code?: string;
	country?: string;
	continent?: string;
	continent_code?: string;
	asn?: string;
	as_name?: string;
	as_domain?: string;
}

export interface ClashConnection {
	id: string;
	metadata: {
		network: string;
		type: string;
		sourceIP: string;
		sourcePort: string;
		destinationIP: string;
		destinationPort: string;
		host: string;
		dnsMode?: string;
		processPath?: string;
	};
	upload: number;
	download: number;
	start: string;
	chains: string[];
	rule: string;
	rulePayload: string;
	geoip?: GeoInfo;
}

export interface ConnectionsResponse {
	connections: ClashConnection[];
	downloadTotal: number;
	uploadTotal: number;
}

export interface ProxiesResponse {
	proxies: Record<string, ClashProxy>;
}

// RouteBox Settings
export interface ServerSettings {
	mode?: 'router' | 'vps';
	public_host?: string;
	public_port?: number; // external panel port for sub-URLs; 0/undefined = none/443
}

export interface RouteBoxSettings {
	geoip: GeoIPSettings;
	ui: UISettings;
	monitoring: MonitoringSettings;
	security: SecuritySettings;
	network: NetworkSettings;
	singbox: SingboxSettings;
	advanced: AdvancedSettings;
	updates?: UpdatesSettings;
	server?: ServerSettings; // panel mode + public_host (Phase 1/2)
	awg?: AwgServerSettings; // AmneziaWG server inbound (panel/vps mode)
}

export interface GeoIPSettings {
	path: string;
	enabled: boolean;
}

export interface UISettings {
	language: string;
	speed_unit: 'bits' | 'bytes';
}

export interface MonitoringSettings {
	enrichment_enabled: boolean;
}

export interface SecuritySettings {
	auth_enabled: boolean;
	auth_username: string;
	cors_origins: string;
	session_timeout_minutes: number;
}

export interface NetworkSettings {
	listen: string;
	read_timeout_sec: number;
	compression_enabled: boolean;
	// Panel TLS / embedded ACME (backend serializes these; whitelisted as network.*)
	acme_enabled?: boolean;
	acme_email?: string;
	acme_staging?: boolean;
	acme_cache_dir?: string;
	tls_cert_path?: string;
	tls_key_path?: string;
}

export interface SingboxSettings {
	config_path: string;
	clash_api: string;
}

export interface AdvancedSettings {
	ws_ping_interval_sec: number;
	ws_pong_timeout_sec: number;
}

export interface SettingsResponse {
	settings: RouteBoxSettings;
	settings_path: string;
	geoip_loaded: boolean;
}

// Binary updates
export type UpdateTargetName = 'amnezia-box' | 'routebox';
export type UpdatePhase = 'idle' | 'download' | 'verify' | 'swap' | 'restart' | 'done' | 'error';

export interface UpdateTarget {
	name: UpdateTargetName;
	supported: boolean;
	current: string;
	latest?: string;
	notes?: string;
	published_at?: string;
	last_checked?: string;
	update_available: boolean;
	error?: string;
	/** True when this target's Apply is refused because RouteBox runs in Docker. */
	docker_managed?: boolean;
	/** Command to run instead, shown when docker_managed is true. */
	update_command?: string;
}

export interface UpdatesStatus {
	targets: UpdateTarget[];
}

export interface UpdateProgress {
	target: UpdateTargetName | '';
	phase: UpdatePhase;
	downloaded_bytes: number;
	total_bytes: number;
	error?: string;
}

export interface UpdatesSettings {
	auto_check: boolean;
}

// Route Inspector
export interface RuleMatch {
	index: number;
	matched: boolean;
	action: string;
	outbound?: string;
	reason?: string;
	conditions?: string[];
	rule_keys?: string[];
}

export interface InspectorDebug {
	rule_count: number;
	rule_set_count: number;
	rule_set_tags: string[];
	final_outbound: string;
}

export interface TestRouteResponse {
	input: string;
	input_type: 'domain' | 'ip';
	matches: RuleMatch[];
	destination: string;
	matched_rule: number;
	debug?: InspectorDebug;
}

export interface ConnectTestResponse {
	success: boolean;
	latency_ms?: number;
	resolved_ip?: string;
	error?: string;
	country?: string;
	country_code?: string;
	matched_rule?: {
		index: number;
		action: string;
		outbound?: string;
		reason?: string;
	};
}

// AmneziaWG server inbound (panel/vps mode)
export interface AwgStatus {
	module: string;
	enabled: boolean;
	phase: string;
	iface_up: boolean;
	listen_port: number;
	public_host: string;
	peer_count: number;
	online: number; // peers with a live handshake (real connections, not just provisioned)
	rx: number; // server total bytes received (since iface up)
	tx: number; // server total bytes sent (since iface up)
	wan_iface: string;
	nat_orphan: boolean;
	config_dirty: boolean;
	backend: string; // "kernel" | "singbox" — authoritative (always set by the backend)
	last_error?: string;
	ipv6_active?: boolean; // broker desired AND egress preflight passed
	// True when the kernel backend's host has a confirmed awg3-capable module +
	// awg-quick/tools pairing (always false/omitted on the singbox backend).
	kernel_awg3_available?: boolean;
}

export interface AwgPeer {
	name: string;
	public_key: string;
	address: string;
	// A real handshake either way — off the kernel interface on that backend,
	// off sing-box's WireGuard device via its UAPI on the other (see
	// backend/internal/awg/singbox_peers.go). Falls back to a traffic-derived
	// approximation only on a sing-box install running an amnezia-box binary
	// that predates that route.
	last_handshake: number; // unix seconds; 0 = never
	online: boolean;        // within the server's online window
	rx: number;             // cumulative bytes received; 0 on the traffic-fallback path
	tx: number;             // cumulative bytes sent; 0 on the traffic-fallback path
	expires_at: number;     // unix seconds; 0 = never expires
}

// One AWG peer's traffic over a range, in the same shape /monitor/users renders
// for panel users. The bytes come from the per-source history (the peer's tunnel
// IP), not from sing-box user stats — see GetAWGPeersTraffic.
export interface AwgPeerTraffic {
	public_key: string;
	name: string;
	address: string; // "<ip>/32" as stored
	source: string;  // the tunnel IP the bytes are keyed by
	last_handshake: number;
	online: boolean;
	upload: number;
	download: number;
	history: UserTrafficPoint[];
}

// AmneziaWG obfuscation parameters (junk packets, init-packet sizes, magic headers)
export interface AwgObf {
	jc: number;
	jmin: number;
	jmax: number;
	s1: number;
	s2: number;
	s3: number;
	s4: number;
	h1: string;
	h2: string;
	h3: string;
	h4: string;
	// AWG3: UintRange strings ("N" or "lo-hi"); backend sends "" when unset
	content_padding_addition?: string;
	rekey_after_time?: string;
	// AWG3 device timers: UintRange strings ("N" or "lo-hi"); seconds (max_handshake_attempts = count)
	rekey_timeout?: string;
	reject_after_time?: string;
	keepalive_timeout?: string;
	max_handshake_attempts?: string;
}

export interface AwgServerSettings {
	enabled: boolean;
	interface: string;
	subnet: string;
	listen_port: number;
	mtu: number;
	dns: string[];
	wan_iface: string;
	obf: AwgObf;
	obf_preset: string;
	header_protection: boolean;
	ipv6_broker: boolean;
	configured: boolean;
	// PersistentKeepalive for client exports: "N" seconds or an AWG 3.0 "lo-hi"
	// range; "" => 25.
	client_keepalive?: string;
	backend: string; // may be "" on fresh deploy; status.backend is authoritative
	server_host: string; // client-facing AWG address; falls back to server.public_host when empty
}

// ---- Telegram MTProto proxy (panel/vps mode) --------------------------------

export interface MtprotoSettings {
	enabled: boolean;
	listen: string;
	/** The site FakeTLS impersonates. Encoded into every secret, so changing it
	 *  invalidates every link already handed out. */
	masking_domain: string;
	/** Empty falls back to the panel's own public address, as subscriptions do. */
	public_host: string;
	public_port: number;
	concurrency: number;
	idle_timeout_sec: number;
	prefer_ip: string;
	domain_fronting_port: number;
	/** sing-box outbound or endpoint tag the proxy's Telegram traffic leaves
	 *  through. Empty — the default — dials Telegram directly. */
	outbound: string;
	/** Loopback port of the managed SOCKS inbound that carries it there. Only a
	 *  setting so a collision with something else on the host is fixable. */
	socks_port: number;
}

/** One exit the Telegram proxy can be routed through. Outbounds and endpoints
 *  are interchangeable to a route rule, so they share one list. */
export interface RoutableTag {
	tag: string;
	/** sing-box type ("wireguard", "selector", …) — shown beside the tag so two
	 *  similarly named exits are tellable apart. */
	type: string;
	kind: 'outbound' | 'endpoint';
}

export interface MtprotoStatus {
	running: boolean;
	listen: string;
	clients: number;
	connected: number;
	started_at: string;
}

/** GET /api/mtproto — status plus everything the page needs in one round trip. */
export interface MtprotoState {
	status: MtprotoStatus;
	settings: MtprotoSettings;
	/** Resolved public address, so the page need not duplicate the fallback. */
	public_host: string;
	public_port: number;
	/** False when a masking domain or public host is missing — a link built
	 *  without them is well-formed and then fails silently inside Telegram. */
	can_issue_link: boolean;
	read_only: boolean;
	/** Every outbound and endpoint the proxy could be routed through. Rides along
	 *  with the status so the picker's options and the chosen tag above can never
	 *  disagree. Empty in router mode, where there is no sing-box config. */
	outbounds: RoutableTag[];
}

/** One roster entry. The secret is deliberately absent: this is polled. */
export interface MtprotoClient {
	name: string;
	enabled: boolean;
	created_at: number;
	expires_at: number; // unix seconds; 0 = never
	online: boolean;
}

export interface MtprotoConnection {
	stream_id: string;
	client: string;
	client_ip: string;
	started_at: string;
}

export interface MtprotoLink {
	tg: string;
	web: string;
}

/** One Telegram proxy client's traffic over a range, in the same shape
 *  /monitor/users renders for panel users and AWG peers. */
export interface MtprotoClientTraffic {
	name: string;
	upload: number;
	download: number;
	history: UserTrafficPoint[];
}

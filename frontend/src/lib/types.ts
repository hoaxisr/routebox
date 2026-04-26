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
	config_path?: string;
	supports_hup?: boolean;
	version?: string;
	binary_path?: string;
	system_checks?: SystemChecks;
}

// Detected config info
export interface DetectedConfig {
	detected_path: string;
	current_path: string;
	match: boolean;
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
	strict_route?: boolean;
	stack?: 'system' | 'gvisor' | 'mixed';
	// DEPRECATED: keep for backward compat
	inet4_address?: string;
	inet6_address?: string;
	// Common
	tcp_fast_open?: boolean;
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
	type: 'tcp' | 'ws' | 'http' | 'grpc' | 'quic' | 'httpupgrade';
	path?: string;
	headers?: Record<string, string>;
	host?: string[];
	service_name?: string;
	idle_timeout?: string;
	ping_timeout?: string;
}

// Obfuscation configuration for Hysteria2
export interface ObfsConfig {
	type: 'salamander';
	password: string;
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
	// VLESS specific
	uuid?: string;
	flow?: string;
	packet_encoding?: string;
	transport?: TransportConfig;
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
	| OutboundAnytls;

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
	persistent_keepalive_interval?: number;
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

// Client (LAN device) entry
export interface ClientEntry {
	ip: string;
	name: string;
	note: string;
	first_seen: number;
	last_seen: number;
	online: boolean;
}

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
export interface RouteBoxSettings {
	geoip: GeoIPSettings;
	ui: UISettings;
	monitoring: MonitoringSettings;
	logging: LoggingSettings;
	security: SecuritySettings;
	network: NetworkSettings;
	singbox: SingboxSettings;
	advanced: AdvancedSettings;
}

export interface GeoIPSettings {
	path: string;
	enabled: boolean;
	auto_reload: boolean;
}

export interface UISettings {
	theme: 'dark' | 'light' | 'system';
	language: string;
	speed_unit: 'bits' | 'bytes';
	time_format: 'relative' | 'absolute' | 'both';
}

export interface MonitoringSettings {
	enrichment_enabled: boolean;
	max_closed_connections: number;
	poll_interval_ms: number;
	proxies_refresh_ms: number;
}

export interface LoggingSettings {
	level: 'debug' | 'info' | 'warn' | 'error';
	output: string;
	timestamps: boolean;
	format: 'text' | 'json';
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
	write_timeout_sec: number;
	compression_enabled: boolean;
}

export interface SingboxSettings {
	config_path: string;
	clash_api: string;
	binary_name: string;
	service_name: string;
}

export interface AdvancedSettings {
	debug_endpoints: boolean;
	pprof_enabled: boolean;
	max_body_size: number;
	ws_ping_interval_sec: number;
	ws_pong_timeout_sec: number;
}

export interface SettingsResponse {
	settings: RouteBoxSettings;
	settings_path: string;
	geoip_loaded: boolean;
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

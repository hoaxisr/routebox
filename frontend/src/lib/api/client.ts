import type { ApiResponse, ProcessStatus, DetectedConfig, SingboxConfig, Endpoint, Outbound, Inbound, RuleSet, RuleSetUsage, RouteRule, RouteSettings, DnsServer, DnsRule, DnsSettings, LogSettings, ExperimentalSettings, ConnectionsResponse, ProxiesResponse, ClashProxy, TestRouteResponse, ConnectTestResponse, SettingsResponse, RouteBoxSettings, SingBoxVersion, DomainSetInfo, RuleSetSource, ClientEntry, TrafficHistoryResponse, TrafficRange, UpdatesStatus, UpdateProgress, UpdateTargetName, Subscription, SubscriptionInput } from '$lib/types';

const API_BASE = '/api';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const url = `${API_BASE}${path}`;

	const response = await fetch(url, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options.headers
		}
	});

	if (!response.ok) {
		let errorMessage = `HTTP ${response.status}`;
		try {
			const error = await response.json();
			errorMessage = error.error || errorMessage;
		} catch {
			// ignore parse error
		}
		throw new Error(errorMessage);
	}

	const data: ApiResponse<T> = await response.json();

	if (!data.success) {
		throw new Error(data.error || 'Unknown error');
	}

	return data.data as T;
}

// Raw request for Clash API endpoints (no wrapper expected)
async function requestRaw<T>(path: string, options: RequestInit = {}): Promise<T> {
	const url = `${API_BASE}${path}`;

	const response = await fetch(url, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options.headers
		}
	});

	if (!response.ok) {
		let errorMessage = `HTTP ${response.status}`;
		try {
			const error = await response.json();
			errorMessage = error.error || error.message || errorMessage;
		} catch {
			// ignore parse error
		}
		throw new Error(errorMessage);
	}

	// Handle empty responses (e.g., 204 No Content from DELETE)
	const contentLength = response.headers.get('Content-Length');
	if (response.status === 204 || contentLength === '0') {
		return undefined as T;
	}

	const text = await response.text();
	if (!text) {
		return undefined as T;
	}

	return JSON.parse(text);
}

export const api = {
	// Config (full)
	getConfig: () => request<SingboxConfig>('/config'),

	saveConfig: (config: SingboxConfig) =>
		request<{ message: string }>('/config', {
			method: 'PUT',
			body: JSON.stringify(config)
		}),

	validateConfig: (config: SingboxConfig) =>
		request<{ valid: boolean; errors: string[] }>('/config/validate', {
			method: 'POST',
			body: JSON.stringify(config)
		}),

	applyConfig: (mode: 'reload' | 'restart' = 'reload') =>
		request<{ message: string; reloaded?: boolean; restarted?: boolean }>(`/config/apply?mode=${mode}`, {
			method: 'POST'
		}),

	// Config backup/restore
	exportConfig: () => {
		window.location.href = '/api/config/export';
	},
	importConfig: (config: object) =>
		request<{ valid: boolean; errors: string[]; config: SingboxConfig }>('/config/import', {
			method: 'POST',
			body: JSON.stringify(config)
		}),

	// Config draft system
	getConfigStatus: () =>
		request<{ hasDraft: boolean; changeCount: number }>('/config/status'),

	discardConfig: () =>
		request<{ message: string }>('/config/discard', {
			method: 'POST'
		}),

	getDraftDiff: () =>
		request<{ diff: string; additions: number; deletions: number }>('/config/draft-diff'),

	saveConfigDraft: () =>
		request<{ message: string }>('/config/save', {
			method: 'POST'
		}),

	checkConfig: () =>
		request<{ valid: boolean; errors: string[] }>('/config/check', {
			method: 'POST'
		}),

	getActiveConfig: () => request<SingboxConfig>('/config/active'),

	// Endpoints CRUD
	listEndpoints: () => request<Endpoint[]>('/endpoints'),
	getEndpoint: (tag: string) => request<Endpoint>(`/endpoints/${encodeURIComponent(tag)}`),
	createEndpoint: (endpoint: Endpoint) =>
		request<Endpoint>('/endpoints', {
			method: 'POST',
			body: JSON.stringify(endpoint)
		}),
	updateEndpoint: (tag: string, endpoint: Endpoint) =>
		request<Endpoint>(`/endpoints/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(endpoint)
		}),
	deleteEndpoint: (tag: string) =>
		request<{ message: string }>(`/endpoints/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),

	// Outbounds CRUD
	listOutbounds: () => request<Outbound[]>('/outbounds'),
	getOutbound: (tag: string) => request<Outbound>(`/outbounds/${encodeURIComponent(tag)}`),
	createOutbound: (outbound: Outbound) =>
		request<Outbound>('/outbounds', {
			method: 'POST',
			body: JSON.stringify(outbound)
		}),
	updateOutbound: (tag: string, outbound: Outbound) =>
		request<Outbound>(`/outbounds/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(outbound)
		}),
	deleteOutbound: (tag: string) =>
		request<{ message: string }>(`/outbounds/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),

	// Inbounds CRUD
	listInbounds: () => request<Inbound[]>('/inbounds'),
	getInbound: (tag: string) => request<Inbound>(`/inbounds/${encodeURIComponent(tag)}`),
	createInbound: (inbound: Inbound) =>
		request<Inbound>('/inbounds', {
			method: 'POST',
			body: JSON.stringify(inbound)
		}),
	updateInbound: (tag: string, inbound: Inbound) =>
		request<Inbound>(`/inbounds/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(inbound)
		}),
	deleteInbound: (tag: string) =>
		request<{ message: string }>(`/inbounds/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),

	// Rule Sets CRUD
	listRuleSets: () => request<RuleSet[]>('/route/rule-sets'),
	createRuleSet: (ruleSet: RuleSet) =>
		request<RuleSet>('/route/rule-sets', {
			method: 'POST',
			body: JSON.stringify(ruleSet)
		}),
	getRuleSetsUsage: () => request<Record<string, RuleSetUsage>>('/route/rule-sets/usage'),
	updateRuleSet: (tag: string, ruleSet: RuleSet) =>
		request<RuleSet>(`/route/rule-sets/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(ruleSet)
		}),
	deleteRuleSet: (tag: string) =>
		request<{ message: string }>(`/route/rule-sets/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),

	// Route Rules CRUD
	listRules: () => request<RouteRule[]>('/route/rules'),
	createRule: (rule: RouteRule) =>
		request<RouteRule>('/route/rules', {
			method: 'POST',
			body: JSON.stringify(rule)
		}),
	updateRule: (index: number, rule: RouteRule) =>
		request<RouteRule>(`/route/rules/${index}`, {
			method: 'PUT',
			body: JSON.stringify(rule)
		}),
	deleteRule: (index: number) =>
		request<{ message: string }>(`/route/rules/${index}`, {
			method: 'DELETE'
		}),
	reorderRules: (from: number, to: number) =>
		request<{ message: string }>('/route/rules/reorder', {
			method: 'PUT',
			body: JSON.stringify({ from, to })
		}),

	// Route Settings
	getRouteSettings: () => request<RouteSettings>('/route/settings'),
	updateRouteSettings: (settings: Partial<RouteSettings>) =>
		request<RouteSettings>('/route/settings', {
			method: 'PUT',
			body: JSON.stringify(settings)
		}),

	// Route Inspector
	testRoute: (domain: string, port = 443) =>
		request<TestRouteResponse>('/route/test', {
			method: 'POST',
			body: JSON.stringify({ domain, port })
		}),

	// Connection Test (diagnostics)
	connectionTest: (host: string, port = 443, timeout = 5) =>
		request<ConnectTestResponse>('/diagnostics/connect', {
			method: 'POST',
			body: JSON.stringify({ host, port, timeout })
		}),

	// DNS Servers CRUD
	listDnsServers: () => request<DnsServer[]>('/dns/servers'),
	createDnsServer: (server: DnsServer) =>
		request<DnsServer>('/dns/servers', {
			method: 'POST',
			body: JSON.stringify(server)
		}),
	updateDnsServer: (tag: string, server: DnsServer) =>
		request<DnsServer>(`/dns/servers/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(server)
		}),
	deleteDnsServer: (tag: string) =>
		request<{ message: string }>(`/dns/servers/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),

	// DNS Rules CRUD
	listDnsRules: () => request<DnsRule[]>('/dns/rules'),
	createDnsRule: (rule: DnsRule) =>
		request<DnsRule>('/dns/rules', {
			method: 'POST',
			body: JSON.stringify(rule)
		}),
	updateDnsRule: (index: number, rule: DnsRule) =>
		request<DnsRule>(`/dns/rules/${index}`, {
			method: 'PUT',
			body: JSON.stringify(rule)
		}),
	deleteDnsRule: (index: number) =>
		request<{ message: string }>(`/dns/rules/${index}`, {
			method: 'DELETE'
		}),
	reorderDnsRules: (from: number, to: number) =>
		request<{ message: string }>('/dns/rules/reorder', {
			method: 'PUT',
			body: JSON.stringify({ from, to })
		}),

	// DNS Settings
	getDnsSettings: () => request<DnsSettings>('/dns/settings'),
	updateDnsSettings: (settings: Partial<DnsSettings>) =>
		request<DnsSettings>('/dns/settings', {
			method: 'PUT',
			body: JSON.stringify(settings)
		}),

	// Log Settings
	getLogSettings: () => request<LogSettings>('/log'),
	updateLogSettings: (settings: Partial<LogSettings>) =>
		request<LogSettings>('/log', {
			method: 'PUT',
			body: JSON.stringify(settings)
		}),

	// Experimental Settings
	getExperimental: () => request<ExperimentalSettings>('/experimental'),
	updateExperimental: (settings: ExperimentalSettings) =>
		request<ExperimentalSettings>('/experimental', {
			method: 'PUT',
			body: JSON.stringify(settings)
		}),

	// Version & Features
	getVersion: () => request<SingBoxVersion>('/version'),

	// Status & Control
	getStatus: () => request<ProcessStatus>('/status'),

	start: () =>
		request<{ message: string; warning?: string }>('/control/start', {
			method: 'POST'
		}),

	stop: () =>
		request<{ message: string }>('/control/stop', {
			method: 'POST'
		}),

	restart: () =>
		request<{ message: string; warning?: string }>('/control/restart', {
			method: 'POST'
		}),

	reload: () =>
		request<{ message: string }>('/control/reload', {
			method: 'POST'
		}),

	// Config detection
	getDetectedConfig: () => request<DetectedConfig>('/config/detected'),

	useDetectedConfig: () =>
		request<{ message: string; path: string }>('/config/use-detected', {
			method: 'POST'
		}),

	// Systemd journal logs
	getJournalLogs: (lines = 50) =>
		request<{ logs: string; lines: number }>(`/logs/journal?lines=${lines}`),

	// Clash API proxy - Proxies (raw Clash responses, no wrapper)
	getProxies: () => requestRaw<ProxiesResponse>('/clash/proxies'),
	getProxy: (name: string) => requestRaw<ClashProxy>(`/clash/proxies/${encodeURIComponent(name)}`),
	switchProxy: (selector: string, target: string) =>
		requestRaw(`/clash/proxies/${encodeURIComponent(selector)}`, {
			method: 'PUT',
			body: JSON.stringify({ name: target })
		}),
	testLatency: (name: string, url = 'http://www.gstatic.com/generate_204', timeout = 5000) =>
		requestRaw<{ delay: number }>(`/clash/proxies/${encodeURIComponent(name)}/delay?url=${encodeURIComponent(url)}&timeout=${timeout}`),

	// Clash API proxy - Connections (raw Clash responses, no wrapper)
	getConnections: () => requestRaw<ConnectionsResponse>('/clash/connections'),
	closeConnection: (id: string) =>
		requestRaw(`/clash/connections/${encodeURIComponent(id)}`, { method: 'DELETE' }),
	closeAllConnections: () =>
		requestRaw('/clash/connections', { method: 'DELETE' }),

	// Traffic history (aggregated buckets for breakdowns)
	getTrafficHistory: (range: TrafficRange, opts: { source?: string; domain?: string; chain?: string } = {}) => {
		const qs = new URLSearchParams({ range });
		if (opts.source) qs.set('source', opts.source);
		if (opts.domain) qs.set('domain', opts.domain);
		if (opts.chain) qs.set('chain', opts.chain);
		return request<TrafficHistoryResponse>(`/traffic/history?${qs.toString()}`);
	},
	resetTrafficHistory: () =>
		request<{ message: string }>('/traffic/reset', { method: 'POST' }),

	// Health
	health: () => request<{ status: string }>('/health'),

	// RouteBox Settings
	getSettings: () => request<SettingsResponse>('/settings'),
	updateSettings: (updates: Record<string, unknown>) =>
		request<{ message: string; settings: RouteBoxSettings }>('/settings', {
			method: 'PUT',
			body: JSON.stringify(updates)
		}),
	reloadSettings: () =>
		request<{ message: string; settings: RouteBoxSettings }>('/settings/reload', {
			method: 'POST'
		}),

	// Binary updates
	getUpdatesStatus: () => request<UpdatesStatus>('/updates/status'),
	checkUpdates: () =>
		request<UpdatesStatus>('/updates/check', {
			method: 'POST'
		}),
	applyUpdate: (target: UpdateTargetName) =>
		request<{ restarting?: boolean; version?: string }>('/updates/apply', {
			method: 'POST',
			body: JSON.stringify({ target })
		}),
	getUpdateProgress: () => request<UpdateProgress>('/updates/progress'),

	// Domain Sets (custom rule set sources)
	listDomainSets: () => request<DomainSetInfo[]>('/domains'),
	createDomainSet: (tag: string) =>
		request<{ tag: string; message: string }>('/domains', {
			method: 'POST',
			body: JSON.stringify({ tag })
		}),
	deleteDomainSet: (tag: string) =>
		request<{ message: string }>(`/domains/${encodeURIComponent(tag)}`, {
			method: 'DELETE'
		}),
	getDomainSet: (tag: string) => request<RuleSetSource>(`/domains/${encodeURIComponent(tag)}`),
	saveDomainSet: (tag: string, data: RuleSetSource) =>
		request<{ message: string }>(`/domains/${encodeURIComponent(tag)}`, {
			method: 'PUT',
			body: JSON.stringify(data)
		}),
	addDomain: (tag: string, domain: string) =>
		request<{ message: string }>(`/domains/${encodeURIComponent(tag)}/domain`, {
			method: 'POST',
			body: JSON.stringify({ domain })
		}),
	removeDomain: (tag: string, domain: string) =>
		request<{ message: string }>(`/domains/${encodeURIComponent(tag)}/domain/${encodeURIComponent(domain)}`, {
			method: 'DELETE'
		}),
	importDomains: (tag: string, domains: string[]) =>
		request<{ message: string; added: number }>(`/domains/${encodeURIComponent(tag)}/import`, {
			method: 'POST',
			body: JSON.stringify({ domains })
		}),

	// Clients (LAN devices) CRUD
	listClients: () => request<ClientEntry[]>('/clients'),
	updateClient: (ip: string, body: { name: string; note: string }) =>
		request<ClientEntry>(`/clients/${encodeURIComponent(ip)}`, {
			method: 'PUT',
			body: JSON.stringify(body)
		}),
	deleteClient: (ip: string) =>
		requestRaw<void>(`/clients/${encodeURIComponent(ip)}`, { method: 'DELETE' }),

	// Subscriptions CRUD + refresh
	getSubscriptions: () => request<Subscription[]>('/subscriptions'),
	createSubscription: (body: SubscriptionInput) =>
		request<Subscription>('/subscriptions', { method: 'POST', body: JSON.stringify(body) }),
	updateSubscription: (id: string, body: { url: string; interval_hrs: number }) =>
		request<Subscription>(`/subscriptions/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
	deleteSubscription: (id: string) =>
		request<{ message: string }>(`/subscriptions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
	refreshSubscription: (id: string) =>
		request<Subscription>(`/subscriptions/${encodeURIComponent(id)}/refresh`, { method: 'POST' })
};

// WebSocket helpers

export type StreamStatus = 'connected' | 'reconnecting' | 'closed';

export interface StreamHandle {
	close(): void;
}

interface StreamOptions {
	path: string; // e.g. '/api/clash/traffic'
	onMessage(data: unknown): void;
	onStatus?(status: StreamStatus): void;
	onError?(error: string): void; // backend in-band {error: "..."} payloads
}

const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 15000;

function createReconnectingStream(opts: StreamOptions): StreamHandle {
	let ws: WebSocket | null = null;
	let disposed = false;
	let attempt = 0;
	let timer: ReturnType<typeof setTimeout> | null = null;

	const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
	const url = `${proto}://${window.location.host}${opts.path}`;

	function connect() {
		if (disposed) return;
		ws = new WebSocket(url);

		ws.onopen = () => {
			opts.onStatus?.('connected');
		};

		ws.onmessage = (event) => {
			let data: unknown;
			try {
				data = JSON.parse(event.data);
			} catch {
				return; // ignore non-JSON frames
			}
			if (data && typeof data === 'object' && 'error' in data && (data as { error?: unknown }).error) {
				const message = String((data as { error: unknown }).error);
				console.error(`Stream error (${opts.path}):`, message);
				opts.onError?.(message);
				return;
			}
			// Reset backoff only on real data, not on open: a connection that
			// upgrades fine but immediately sends an in-band error and closes
			// must keep backing off (stopped amnezia-box case).
			attempt = 0;
			opts.onMessage(data);
		};

		ws.onerror = () => {
			// onclose always follows; reconnect is scheduled there
		};

		ws.onclose = (event) => {
			ws = null;
			if (disposed) return;
			if (!event.wasClean) {
				console.warn(`WebSocket closed (${opts.path}):`, event.code);
			}
			scheduleReconnect();
		};
	}

	function scheduleReconnect() {
		if (disposed || timer) return;
		const delay = Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_MAX_MS); // 1s,2s,4s,...,15s
		attempt += 1;
		opts.onStatus?.('reconnecting');
		timer = setTimeout(() => {
			timer = null;
			connect();
		}, delay);
	}

	connect();

	return {
		close() {
			if (disposed) return;
			disposed = true;
			if (timer) {
				clearTimeout(timer);
				timer = null;
			}
			ws?.close();
			ws = null;
			opts.onStatus?.('closed');
		}
	};
}

export function createTrafficStream(
	onMessage: (data: { up: number; down: number }) => void,
	onError?: (error: string) => void,
	onClose?: () => void,
	onStatus?: (status: StreamStatus) => void
): StreamHandle {
	return createReconnectingStream({
		path: '/api/clash/traffic',
		onMessage: (data) => onMessage(data as { up: number; down: number }),
		onError,
		onStatus: (status) => {
			onStatus?.(status);
			// Legacy semantics: onClose fired on connection loss. Do NOT fire it on
			// intentional close ('closed') — that's what caused consumers'
			// setTimeout-reconnect to resurrect streams after onDestroy.
			if (status === 'reconnecting') onClose?.();
		}
	});
}

export function createLogsStream(
	onMessage: (data: { type: string; payload: string }) => void,
	level = 'info',
	onError?: (error: string) => void,
	onStatus?: (status: StreamStatus) => void
): StreamHandle {
	return createReconnectingStream({
		path: `/api/clash/logs?level=${encodeURIComponent(level)}`,
		onMessage: (data) => onMessage(data as { type: string; payload: string }),
		onError,
		onStatus
	});
}

export function createConnectionsStream(
	onMessage: (data: ConnectionsResponse) => void,
	onError?: (error: string) => void,
	onClose?: () => void,
	onStatus?: (status: StreamStatus) => void
): StreamHandle {
	return createReconnectingStream({
		path: '/api/clash/connections',
		onMessage: (data) => onMessage(data as ConnectionsResponse),
		onError,
		onStatus: (status) => {
			onStatus?.(status);
			if (status === 'reconnecting') onClose?.();
		}
	});
}

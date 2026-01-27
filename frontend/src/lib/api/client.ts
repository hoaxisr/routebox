import type { ApiResponse, ProcessStatus, DetectedConfig, NeedsSetupResponse, SingboxConfig, Endpoint, Outbound, Inbound, RuleSet, RuleSetUsage, RouteRule, RouteSettings, DnsServer, DnsRule, DnsSettings, LogSettings, ExperimentalSettings, ConnectionsResponse, ProxiesResponse, ClashProxy, TestRouteResponse, SettingsResponse, RouteBoxSettings, SingBoxVersion } from '$lib/types';

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

	return await response.json();
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
		request<{ valid: boolean; errors: string[]; config: object }>('/config/import', {
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

	// Health
	health: () => request<{ status: string }>('/health'),

	// Setup wizard
	needsSetup: () => request<NeedsSetupResponse>('/needs-setup'),

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
		})
};

// WebSocket helpers
export function createTrafficStream(
	onMessage: (data: { up: number; down: number }) => void,
	onError?: (error: string) => void,
	onClose?: () => void
) {
	const ws = new WebSocket(`ws://${window.location.host}/api/clash/traffic`);
	let connected = false;

	ws.onopen = () => {
		connected = true;
	};

	ws.onmessage = (event) => {
		try {
			const data = JSON.parse(event.data);
			// Check if it's an error message from backend
			if (data.error) {
				console.error('Traffic stream error:', data.error);
				onError?.(data.error);
				return;
			}
			onMessage(data);
		} catch {
			// ignore parse errors
		}
	};

	ws.onerror = () => {
		if (!connected) {
			onError?.('Failed to connect to traffic stream');
		}
	};

	ws.onclose = (event) => {
		if (!event.wasClean && connected) {
			console.warn('Traffic WebSocket closed:', event.code);
		}
		onClose?.();
	};

	return {
		close: () => ws.close()
	};
}

export function createLogsStream(
	onMessage: (data: { type: string; payload: string }) => void,
	level = 'info',
	onError?: (error: string) => void
) {
	const ws = new WebSocket(`ws://${window.location.host}/api/clash/logs?level=${level}`);

	ws.onmessage = (event) => {
		try {
			const data = JSON.parse(event.data);
			onMessage(data);
		} catch {
			// ignore parse errors
		}
	};

	ws.onerror = (event) => {
		console.error('Logs WebSocket error:', event);
		onError?.('WebSocket connection failed');
	};

	ws.onclose = (event) => {
		if (!event.wasClean) {
			console.warn('Logs WebSocket closed unexpectedly:', event.code, event.reason);
		}
	};

	return {
		close: () => ws.close()
	};
}

export function createConnectionsStream(
	onMessage: (data: ConnectionsResponse) => void,
	onError?: (error: string) => void,
	onClose?: () => void
) {
	const ws = new WebSocket(`ws://${window.location.host}/api/clash/connections`);
	let connected = false;

	ws.onopen = () => {
		connected = true;
	};

	ws.onmessage = (event) => {
		try {
			const data = JSON.parse(event.data);
			// Check if it's an error message from backend
			if (data.error) {
				console.error('Connections stream error:', data.error);
				onError?.(data.error);
				return;
			}
			onMessage(data);
		} catch {
			// ignore parse errors
		}
	};

	ws.onerror = () => {
		if (!connected) {
			onError?.('Failed to connect to connections stream');
		}
	};

	ws.onclose = (event) => {
		if (!event.wasClean && connected) {
			console.warn('Connections WebSocket closed:', event.code);
		}
		onClose?.();
	};

	return {
		close: () => ws.close()
	};
}

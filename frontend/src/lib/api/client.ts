import type { ApiResponse, ProcessStatus, SingboxConfig, Endpoint, Outbound, Inbound, RuleSet, RuleSetUsage, RouteRule, RouteSettings, DnsServer, DnsRule, DnsSettings, LogSettings, ExperimentalSettings, ConnectionsResponse, ProxiesResponse, ClashProxy, TestRouteResponse, ConnectTestResponse, SettingsResponse, RouteBoxSettings, SingBoxVersion, DomainSetInfo, RuleSetSource, ClientEntry, TrafficHistoryResponse, TrafficRange, UpdatesStatus, UpdateProgress, UpdateTargetName, Subscription, SubscriptionInput, PanelUser, UserTrafficResponse, AwgStatus, AwgPeer, AwgPeerTraffic, MtprotoState, MtprotoStatus, MtprotoSettings, MtprotoClient, MtprotoConnection, MtprotoLink, MtprotoClientTraffic } from '$lib/types';

const API_BASE = '/api';

// A write refused with 409 is the backend telling us the config turned
// read-only. Re-read the status so the badge lights up and the save buttons
// go dead, instead of waiting for the next Dashboard poll or a page reload:
// outside the Dashboard the status is only seeded once, on layout mount, so
// without this the very next form would let the user fill it in and fail again.
//
// Any 409 triggers it, not just the read-only one. The backend also answers
// 409 for duplicate tags and pending changes, so this over-polls slightly —
// deliberately. The alternative is matching the backend's English error text,
// and paying one deduped GET on a request that already failed is cheaper than
// a UI that silently stops noticing when that wording changes.
//
// Imported lazily because stores/status.ts imports this module — a static
// import would close the cycle. Fire-and-forget: this runs on the error path
// of someone else's request, and a failed refresh must not replace the real
// error with its own.
let refreshingStatus = false;
function noteWriteRefused(): void {
	if (refreshingStatus) return; // one refresh per burst of refusals
	refreshingStatus = true;
	import('$lib/stores/status')
		.then((m) => m.refreshStatus())
		.catch(() => {})
		.finally(() => {
			refreshingStatus = false;
		});
}

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
		if (response.status === 401 && typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
			window.location.href = '/login';
		}
		if (response.status === 409) {
			noteWriteRefused();
		}
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
		if (response.status === 401 && !path.startsWith('/clash') && typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
			window.location.href = '/login';
		}
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

// Raw text/plain GET (e.g. an AmneziaWG client .conf). Neither request<T> nor
// requestRaw<T> works — both assume a JSON body.
async function requestText(path: string): Promise<string> {
	const response = await fetch(`${API_BASE}${path}`);
	if (!response.ok) {
		if (response.status === 401 && typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
			window.location.href = '/login';
		}
		// The backend sends actionable messages here; a bare status code would
		// strand the operator with "HTTP 503". writeError wraps them in JSON,
		// http.Error does not — accept either. 404 is the one exception: on
		// these routes it only ever comes from the router's stock
		// http.NotFound handler ("404 page not found"), never from our own
		// code, so its body carries nothing operator-actionable — do not
		// "restore the symmetry" by echoing it, surface the plain status
		// instead, same as before this change.
		if (response.status === 404) {
			throw new Error(`HTTP ${response.status}`);
		}
		const body = (await response.text().catch(() => '')).trim();
		let message = body;
		try {
			const parsed = JSON.parse(body);
			message = parsed?.error || parsed?.message || body;
		} catch {
			/* plain text, use as-is */
		}
		throw new Error(message || `HTTP ${response.status}`);
	}
	return response.text();
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

	// Server inbound: credential generators + share link
	generateReality: () =>
		request<{ private_key: string; public_key: string }>('/generate/reality', { method: 'POST' }),
	generateUuid: () =>
		request<{ uuid: string }>('/generate/uuid', { method: 'POST' }),
	generatePassword: () =>
		request<{ password: string }>('/generate/password', { method: 'POST' }),

	// Panel users
	getUsers: () => request<PanelUser[]>('/users'),
	getUserTraffic: (id: string, range: TrafficRange = '24h') =>
		request<UserTrafficResponse>(
			`/users/${encodeURIComponent(id)}/traffic?range=${encodeURIComponent(range)}`
		),
	createUser: (body: { name: string; protocol: string; inbound_tag: string }) =>
		request<PanelUser>('/users', { method: 'POST', body: JSON.stringify(body) }),
	addUserBinding: (id: string, body: { protocol: string; inbound_tag: string }) =>
		request<{ message: string }>(`/users/${encodeURIComponent(id)}/bindings`, {
			method: 'POST',
			body: JSON.stringify(body)
		}),
	deleteUser: (id: string) =>
		request<{ message: string }>(`/users/${encodeURIComponent(id)}`, { method: 'DELETE' }),
	getUserLink: (id: string, tag: string, host: string) =>
		request<{ link: string }>(
			`/users/${encodeURIComponent(id)}/link?tag=${encodeURIComponent(tag)}&host=${encodeURIComponent(host)}`
		),
	rotateUserToken: (id: string) =>
		request<{ token: string }>(`/users/${encodeURIComponent(id)}/token/rotate`, {
			method: 'POST'
		}),
	revokeUserToken: (id: string) =>
		request<{ message: string }>(`/users/${encodeURIComponent(id)}/token`, {
			method: 'DELETE'
		}),
	updateUser: (id: string, body: { enabled?: boolean; expires_at?: number }) =>
		request<PanelUser>(`/users/${encodeURIComponent(id)}`, {
			method: 'PATCH',
			body: JSON.stringify(body)
		}),

	// AmneziaWG server inbound (panel/vps mode)
	awgStatus: () => request<AwgStatus>('/awg/status'),
	awgEnable: () => request<AwgStatus>('/awg/enable', { method: 'POST' }),
	awgDisable: () => request<AwgStatus>('/awg/disable', { method: 'POST' }),
	getAwgPeers: () => request<AwgPeer[]>('/awg/peers'),
	getAwgPeersTraffic: (range: TrafficRange = '24h') =>
		request<AwgPeerTraffic[]>(`/awg/peers/traffic?range=${encodeURIComponent(range)}`),
	createAwgPeer: (name: string) =>
		request<AwgPeer>('/awg/peers', { method: 'POST', body: JSON.stringify({ name }) }),
	deleteAwgPeer: (pk: string) =>
		requestRaw<void>(`/awg/peers/${encodeURIComponent(pk)}`, { method: 'DELETE' }),
	getAwgPeerConfig: (pk: string) => requestText(`/awg/peers/${encodeURIComponent(pk)}/config`),
	getAwgPeerVpnLink: (pk: string) => requestText(`/awg/peers/${encodeURIComponent(pk)}/vpn-link`),
	getAwgPeerSingbox: (pk: string) =>
		request<Record<string, unknown>>(`/awg/peers/${encodeURIComponent(pk)}/singbox`),
	setAwgPeerExpiry: (pk: string, expiresAt: number) =>
		requestRaw<void>(`/awg/peers/${encodeURIComponent(pk)}/expiry`, {
			method: 'PATCH',
			body: JSON.stringify({ expires_at: expiresAt })
		}),

	// Telegram MTProto proxy (panel/vps mode)
	mtprotoStatus: () => request<MtprotoState>('/mtproto'),
	mtprotoEnable: () => request<MtprotoStatus>('/mtproto/enable', { method: 'POST' }),
	mtprotoDisable: () => request<MtprotoStatus>('/mtproto/disable', { method: 'POST' }),
	updateMtprotoSettings: (patch: Partial<MtprotoSettings>) =>
		request<MtprotoSettings>('/mtproto', { method: 'PUT', body: JSON.stringify(patch) }),
	getMtprotoClients: () => request<MtprotoClient[]>('/mtproto/clients'),
	getMtprotoConnections: () => request<MtprotoConnection[]>('/mtproto/connections'),
	getMtprotoClientsTraffic: (range: TrafficRange = '24h') =>
		request<MtprotoClientTraffic[]>(`/mtproto/clients/traffic?range=${encodeURIComponent(range)}`),
	createMtprotoClient: (name: string) =>
		request<{ name: string }>('/mtproto/clients', { method: 'POST', body: JSON.stringify({ name }) }),
	deleteMtprotoClient: (name: string) =>
		request<{ deleted: string }>(`/mtproto/clients/${encodeURIComponent(name)}`, { method: 'DELETE' }),
	rotateMtprotoClient: (name: string) =>
		request<{ name: string }>(`/mtproto/clients/${encodeURIComponent(name)}/rotate`, { method: 'POST' }),
	updateMtprotoClient: (name: string, patch: { enabled?: boolean; expires_at?: number }) =>
		request<{ name: string }>(`/mtproto/clients/${encodeURIComponent(name)}`, {
			method: 'PATCH',
			body: JSON.stringify(patch)
		}),
	// The only endpoint that discloses a secret, so it is fetched per share
	// action rather than with the roster.
	getMtprotoClientLink: (name: string) =>
		request<MtprotoLink>(`/mtproto/clients/${encodeURIComponent(name)}/link`),

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

	// Moves RouteBox onto the config path from the unit's ExecStart (409 when
	// there is no unit, or its ExecStart names no config). Which file each side
	// points at comes from `config_paths` in /api/status — there is no separate
	// "detected config" endpoint to disagree with it.
	// `warning` is set when the path is in use but could not be written to the
	// settings file — the switch then does not survive a RouteBox restart.
	adoptUnitConfigPath: () =>
		request<{ message: string; path: string; warning?: string }>('/config/adopt-unit-path', {
			method: 'POST'
		}),

	// Repoints the systemd unit at the config path RouteBox manages (409 when
	// there is no mismatch to fix).
	fixUnitConfigPath: () =>
		request<{ message: string }>('/config/fix-unit', {
			method: 'POST'
		}),

	// Takes that drop-in back off: deletes the file, reloads systemd and reports
	// the config path the unit fell back to (409 when nothing is installed).
	// `warning` is set when the file is gone but the removal is not fully in
	// effect — daemon-reload failed, or the unit could not be re-read — in which
	// case `unit_path` is empty because it is genuinely unknown.
	removeUnitConfigDropIn: () =>
		request<{ message: string; unit_path: string; warning?: string }>('/config/unit-dropin', {
			method: 'DELETE'
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
		request<Subscription>(`/subscriptions/${encodeURIComponent(id)}/refresh`, { method: 'POST' }),

	// Panel auth
	login: (username: string, password: string) =>
		request<{ username: string }>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
	logout: () => request<{ status: string }>('/auth/logout', { method: 'POST' }),
	getSession: () =>
		request<{ authenticated: boolean; auth_enabled: boolean; username?: string }>('/auth/session'),
	changePassword: (current_password: string, new_password: string) =>
		request<{ status: string }>('/auth/change-password', {
			method: 'POST',
			body: JSON.stringify({ current_password, new_password })
		})
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

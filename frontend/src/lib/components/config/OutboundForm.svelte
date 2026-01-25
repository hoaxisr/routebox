<script lang="ts">
	import type { Outbound, Endpoint } from '$lib/types';
	import { notifications } from '$lib/stores';
	import { parseVless, parseHysteria2, toSingboxConfig, type ParsedVless, type ParsedHysteria2 } from '$lib/utils/parsers';

	interface Props {
		outbound?: Outbound;
		endpoints: Endpoint[];
		outbounds: Outbound[];
		onSave: (outbound: Outbound) => void;
		onCancel: () => void;
	}

	let { outbound, endpoints, outbounds, onSave, onCancel }: Props = $props();

	// Form state
	let tag = $state(outbound?.tag ?? '');
	let type = $state(outbound?.type ?? 'direct');
	let selectedOutbounds = $state<string[]>(outbound?.outbounds ?? []);
	let defaultOutbound = $state(outbound?.default ?? '');
	let interruptExistConnections = $state(outbound?.interrupt_exist_connections ?? false);

	// VLESS fields
	let server = $state((outbound as any)?.server ?? '');
	let serverPort = $state((outbound as any)?.server_port ?? 443);
	let uuid = $state((outbound as any)?.uuid ?? '');
	let flow = $state((outbound as any)?.flow ?? '');
	// TLS
	let tlsEnabled = $state((outbound as any)?.tls?.enabled ?? true);
	let tlsSni = $state((outbound as any)?.tls?.server_name ?? '');
	let tlsFingerprint = $state((outbound as any)?.tls?.utls?.fingerprint ?? 'chrome');
	let tlsAlpn = $state<string[]>((outbound as any)?.tls?.alpn ?? []);
	// Reality
	let realityEnabled = $state((outbound as any)?.tls?.reality?.enabled ?? false);
	let realityPublicKey = $state((outbound as any)?.tls?.reality?.public_key ?? '');
	let realityShortId = $state((outbound as any)?.tls?.reality?.short_id ?? '');
	// Transport
	let transportType = $state<'tcp' | 'ws' | 'grpc' | 'http'>((outbound as any)?.transport?.type ?? 'tcp');
	let wsPath = $state((outbound as any)?.transport?.path ?? '/');
	let wsHost = $state((outbound as any)?.transport?.headers?.Host ?? '');
	let grpcServiceName = $state((outbound as any)?.transport?.service_name ?? '');

	// Hysteria2 fields
	let hy2Password = $state((outbound as any)?.password ?? '');
	let hy2Insecure = $state((outbound as any)?.tls?.insecure ?? false);
	let hy2ObfsType = $state((outbound as any)?.obfs?.type ?? '');
	let hy2ObfsPassword = $state((outbound as any)?.obfs?.password ?? '');
	// Hysteria2 port hopping
	let hy2ServerPorts = $state((outbound as any)?.server_ports ?? '');
	let hy2HopInterval = $state((outbound as any)?.hop_interval ?? '');
	let hy2UpMbps = $state((outbound as any)?.up_mbps ?? 0);
	let hy2DownMbps = $state((outbound as any)?.down_mbps ?? 0);

	// URLTest specific fields
	let urltestUrl = $state(outbound?.url ?? '');
	let urltestInterval = $state(outbound?.interval ?? '3m');
	let urltestTolerance = $state(outbound?.tolerance ?? 150);
	let urltestIdleTimeout = $state(outbound?.idle_timeout ?? '');

	// Import modal state
	let showImport = $state(false);
	let importText = $state('');
	let importError = $state('');

	let errors = $state<Record<string, string>>({});

	// Available outbounds for selector (exclude self)
	$effect(() => {
		if (type === 'selector' || type === 'urltest') {
			// Include endpoints as available options
			const endpointTags = endpoints.map((e) => e.tag);
			// Include other outbounds (exclude current one being edited)
			const otherOutbounds = outbounds.filter((o) => o.tag !== outbound?.tag).map((o) => o.tag);
			availableForSelection = [...endpointTags, ...otherOutbounds];
		}
	});

	let availableForSelection = $state<string[]>([]);

	function toggleOutboundSelection(t: string) {
		if (selectedOutbounds.includes(t)) {
			selectedOutbounds = selectedOutbounds.filter((o) => o !== t);
			if (defaultOutbound === t) {
				defaultOutbound = selectedOutbounds[0] ?? '';
			}
		} else {
			selectedOutbounds = [...selectedOutbounds, t];
			if (!defaultOutbound) {
				defaultOutbound = t;
			}
		}
	}

	function parseImportLink() {
		importError = '';
		const text = importText.trim();

		if (!text) {
			importError = 'Please paste a VLESS or Hysteria2 link';
			return;
		}

		// Try VLESS
		if (text.startsWith('vless://')) {
			const result = parseVless(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse VLESS link';
				return;
			}
			applyVlessConfig(result.config as ParsedVless);
			showImport = false;
			importText = '';
			notifications.success('VLESS configuration imported');
			return;
		}

		// Try Hysteria2
		if (text.startsWith('hy2://') || text.startsWith('hysteria2://')) {
			const result = parseHysteria2(text);
			if (!result.success || !result.config) {
				importError = result.error || 'Failed to parse Hysteria2 link';
				return;
			}
			applyHysteria2Config(result.config as ParsedHysteria2);
			showImport = false;
			importText = '';
			notifications.success('Hysteria2 configuration imported');
			return;
		}

		importError = 'Unknown link format. Supported: vless://, hy2://, hysteria2://';
	}

	function applyVlessConfig(config: ParsedVless) {
		type = 'vless';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		uuid = config.uuid;
		flow = config.flow || '';

		// TLS
		tlsEnabled = config.security === 'tls' || config.security === 'reality';
		tlsSni = config.sni || config.server;
		tlsFingerprint = config.fingerprint || 'chrome';
		tlsAlpn = config.alpn || [];

		// Reality
		realityEnabled = config.security === 'reality';
		realityPublicKey = config.pbk || '';
		realityShortId = config.sid || '';

		// Transport
		transportType = config.transport || 'tcp';
		wsPath = config.path || '/';
		wsHost = config.host || '';
		grpcServiceName = config.serviceName || '';
	}

	function applyHysteria2Config(config: ParsedHysteria2) {
		type = 'hysteria2';
		tag = config.name.replace(/[^a-zA-Z0-9-_]/g, '-');
		server = config.server;
		serverPort = config.port;
		hy2Password = config.password;
		tlsSni = config.sni || config.server;
		hy2Insecure = config.insecure || false;
		hy2ObfsType = config.obfs || '';
		hy2ObfsPassword = config.obfsPassword || '';
	}

	function validate(): boolean {
		errors = {};

		if (!tag.trim()) {
			errors['tag'] = 'Tag is required';
		}

		if (type === 'selector' || type === 'urltest') {
			if (selectedOutbounds.length === 0) {
				errors['outbounds'] = 'Select at least one outbound';
			}
		}

		if (type === 'vless') {
			if (!server.trim()) {
				errors['server'] = 'Server is required';
			}
			if (!serverPort || serverPort < 1 || serverPort > 65535) {
				errors['serverPort'] = 'Valid port is required (1-65535)';
			}
			if (!uuid.trim()) {
				errors['uuid'] = 'UUID is required';
			}
			if (realityEnabled && !realityPublicKey.trim()) {
				errors['realityPublicKey'] = 'Reality public key is required';
			}
		}

		if (type === 'hysteria2') {
			if (!server.trim()) {
				errors['server'] = 'Server is required';
			}
			if (!serverPort || serverPort < 1 || serverPort > 65535) {
				errors['serverPort'] = 'Valid port is required (1-65535)';
			}
			if (!hy2Password.trim()) {
				errors['password'] = 'Password is required';
			}
		}

		const errorKeys = Object.keys(errors);
		if (errorKeys.length > 0) {
			notifications.error(errors[errorKeys[0]]);
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const ob: Outbound = {
			type,
			tag: tag.trim()
		};

		if (type === 'selector' || type === 'urltest') {
			ob.outbounds = selectedOutbounds;
			if (defaultOutbound) {
				ob.default = defaultOutbound;
			}
			if (interruptExistConnections) {
				ob.interrupt_exist_connections = true;
			}
		}

		// URLTest specific fields
		if (type === 'urltest') {
			if (urltestUrl.trim()) ob.url = urltestUrl.trim();
			if (urltestInterval && urltestInterval !== '3m') ob.interval = urltestInterval;
			if (urltestTolerance && urltestTolerance !== 150) ob.tolerance = urltestTolerance;
			if (urltestIdleTimeout.trim()) ob.idle_timeout = urltestIdleTimeout.trim();
		}

		if (type === 'vless') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).uuid = uuid.trim();
			if (flow) (ob as any).flow = flow;

			if (tlsEnabled) {
				(ob as any).tls = {
					enabled: true,
					server_name: tlsSni || server.trim()
				};

				if (tlsFingerprint) {
					(ob as any).tls.utls = {
						enabled: true,
						fingerprint: tlsFingerprint
					};
				}

				if (tlsAlpn.length > 0) {
					(ob as any).tls.alpn = tlsAlpn;
				}

				if (realityEnabled) {
					(ob as any).tls.reality = {
						enabled: true,
						public_key: realityPublicKey,
						short_id: realityShortId || ''
					};
				}
			}

			if (transportType !== 'tcp') {
				if (transportType === 'ws') {
					(ob as any).transport = {
						type: 'ws',
						path: wsPath || '/'
					};
					if (wsHost) {
						(ob as any).transport.headers = { Host: wsHost };
					}
				} else if (transportType === 'grpc') {
					(ob as any).transport = {
						type: 'grpc',
						service_name: grpcServiceName || ''
					};
				} else if (transportType === 'http') {
					(ob as any).transport = {
						type: 'http',
						path: wsPath || '/'
					};
					if (wsHost) {
						(ob as any).transport.host = [wsHost];
					}
				}
			}
		}

		if (type === 'hysteria2') {
			(ob as any).server = server.trim();
			(ob as any).server_port = serverPort;
			(ob as any).password = hy2Password.trim();

			// Port hopping
			if (hy2ServerPorts.trim()) (ob as any).server_ports = hy2ServerPorts.trim();
			if (hy2HopInterval.trim()) (ob as any).hop_interval = hy2HopInterval.trim();

			// Bandwidth limits
			if (hy2UpMbps > 0) (ob as any).up_mbps = hy2UpMbps;
			if (hy2DownMbps > 0) (ob as any).down_mbps = hy2DownMbps;

			(ob as any).tls = {
				enabled: true,
				server_name: tlsSni || server.trim(),
				insecure: hy2Insecure
			};

			if (hy2ObfsType) {
				(ob as any).obfs = {
					type: hy2ObfsType,
					password: hy2ObfsPassword || ''
				};
			}
		}

		onSave(ob);
	}

	const outboundTypes = [
		{ value: 'direct', label: 'Direct', description: 'Connect directly without proxy' },
		{ value: 'block', label: 'Block', description: 'Block all connections' },
		{ value: 'selector', label: 'Selector', description: 'Manual proxy selection' },
		{ value: 'urltest', label: 'URL Test', description: 'Auto-select fastest proxy' },
		{ value: 'vless', label: 'VLESS', description: 'VLESS proxy protocol' },
		{ value: 'hysteria2', label: 'Hysteria2', description: 'Hysteria2 proxy protocol' }
	];

	const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'];
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Tag -->
	<div>
		<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Tag *</label>
		<input
			id="tag"
			type="text"
			bind:value={tag}
			placeholder="proxy"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['tag']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
		{/if}
	</div>

	<!-- Type -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Type</label>
		<div class="grid grid-cols-2 gap-2">
			{#each outboundTypes as t}
				<button
					type="button"
					onclick={() => type = t.value}
					class="type-btn {type === t.value ? 'selected' : ''}"
				>
					<div class="type-label">{t.label}</div>
					<div class="type-desc">{t.description}</div>
				</button>
			{/each}
		</div>
	</div>

	<!-- Type-specific options -->
	{#if type === 'selector' || type === 'urltest'}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
				Select Outbounds *
				<span class="font-normal text-[var(--ctp-overlay0)]">({selectedOutbounds.length} selected)</span>
			</label>
			{#if errors['outbounds']}
				<p class="mb-2 text-sm text-[var(--ctp-red)]">{errors['outbounds']}</p>
			{/if}
			<div class="max-h-48 overflow-y-auto bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)]">
				{#if availableForSelection.length === 0}
					<div class="p-4 text-center text-[var(--ctp-overlay0)]">
						No endpoints or outbounds available
					</div>
				{:else}
					{#each availableForSelection as t}
						<button
							type="button"
							onclick={() => toggleOutboundSelection(t)}
							class="w-full px-4 py-2 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors {selectedOutbounds.includes(t) ? 'bg-[var(--ctp-surface1)]' : ''}"
						>
							<span class="text-[var(--ctp-text)]">{t}</span>
							<span class="flex items-center gap-2">
								{#if defaultOutbound === t}
									<span class="status-badge">default</span>
								{/if}
								{#if selectedOutbounds.includes(t)}
									<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</span>
						</button>
					{/each}
				{/if}
			</div>
		</div>

		{#if selectedOutbounds.length > 0}
			<div>
				<label for="default" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Default Outbound</label>
				<select
					id="default"
					bind:value={defaultOutbound}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					{#each selectedOutbounds as t}
						<option value={t}>{t}</option>
					{/each}
				</select>
			</div>

			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={interruptExistConnections}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				Interrupt existing connections on switch
			</label>
		{/if}

		<!-- URLTest specific settings -->
		{#if type === 'urltest'}
			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">URL Test Settings</h3>
				<div>
					<label for="urltestUrl" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Test URL</label>
					<input
						id="urltestUrl"
						type="url"
						bind:value={urltestUrl}
						placeholder="https://www.gstatic.com/generate_204"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div class="grid grid-cols-3 gap-4">
					<div>
						<label for="urltestInterval" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Interval</label>
						<input
							id="urltestInterval"
							type="text"
							bind:value={urltestInterval}
							placeholder="3m"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
					<div>
						<label for="urltestTolerance" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Tolerance (ms)</label>
						<input
							id="urltestTolerance"
							type="number"
							bind:value={urltestTolerance}
							min="0"
							placeholder="150"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
					<div>
						<label for="urltestIdleTimeout" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Idle Timeout</label>
						<input
							id="urltestIdleTimeout"
							type="text"
							bind:value={urltestIdleTimeout}
							placeholder="30m"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
						/>
					</div>
				</div>
			</div>
		{/if}
	{/if}

	{#if type === 'vless'}
		<!-- Import Button -->
		<button
			type="button"
			onclick={() => showImport = true}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			Import from vless:// link
		</button>

		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Server *</label>
				<input
					id="server"
					type="text"
					bind:value={server}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
			<div>
				<label for="port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Port *</label>
				<input
					id="port"
					type="number"
					bind:value={serverPort}
					min="1"
					max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
		</div>

		<!-- UUID -->
		<div>
			<label for="uuid" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">UUID *</label>
			<input
				id="uuid"
				type="text"
				bind:value={uuid}
				placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['uuid'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
		</div>

		<!-- Flow -->
		<div>
			<label for="flow" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Flow</label>
			<select
				id="flow"
				bind:value={flow}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="">None</option>
				<option value="xtls-rprx-vision">xtls-rprx-vision</option>
			</select>
		</div>

		<!-- TLS Settings -->
		<div class="space-y-4">
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input type="checkbox" bind:checked={tlsEnabled} />
				Enable TLS
			</label>

			{#if tlsEnabled}
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">SNI</label>
						<input
							id="sni"
							type="text"
							bind:value={tlsSni}
							placeholder="Server name"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="fingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Fingerprint</label>
						<select
							id="fingerprint"
							bind:value={tlsFingerprint}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							{#each fingerprints as fp}
								<option value={fp}>{fp}</option>
							{/each}
						</select>
					</div>
				</div>

				<!-- Reality -->
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input type="checkbox" bind:checked={realityEnabled} />
					Enable Reality
				</label>

				{#if realityEnabled}
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="pbk" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Public Key *</label>
							<input
								id="pbk"
								type="text"
								bind:value={realityPublicKey}
								placeholder="Reality public key"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['realityPublicKey'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
							/>
						</div>
						<div>
							<label for="sid" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Short ID</label>
							<input
								id="sid"
								type="text"
								bind:value={realityShortId}
								placeholder="Short ID"
								class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
						</div>
					</div>
				{/if}
			{/if}
		</div>

		<!-- Transport -->
		<div class="space-y-4">
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)]">Transport</label>
			<div class="grid grid-cols-4 gap-2">
				{#each ['tcp', 'ws', 'grpc', 'http'] as t}
					<button
						type="button"
						onclick={() => transportType = t as any}
						class="toggle-btn text-sm {transportType === t ? 'selected' : ''}"
					>
						{t.toUpperCase()}
					</button>
				{/each}
			</div>

			{#if transportType === 'ws' || transportType === 'http'}
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="wsPath" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Path</label>
						<input
							id="wsPath"
							type="text"
							bind:value={wsPath}
							placeholder="/"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="wsHost" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Host</label>
						<input
							id="wsHost"
							type="text"
							bind:value={wsHost}
							placeholder="Host header"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			{/if}

			{#if transportType === 'grpc'}
				<div>
					<label for="serviceName" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Service Name</label>
					<input
						id="serviceName"
						type="text"
						bind:value={grpcServiceName}
						placeholder="grpc service name"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			{/if}
		</div>
	{/if}

	{#if type === 'hysteria2'}
		<!-- Import Button -->
		<button
			type="button"
			onclick={() => showImport = true}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			Import from hy2:// link
		</button>

		<!-- Server & Port -->
		<div class="grid grid-cols-3 gap-4">
			<div class="col-span-2">
				<label for="hy2server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Server *</label>
				<input
					id="hy2server"
					type="text"
					bind:value={server}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
			<div>
				<label for="hy2port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Port *</label>
				<input
					id="hy2port"
					type="number"
					bind:value={serverPort}
					min="1"
					max="65535"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
			</div>
		</div>

		<!-- Password -->
		<div>
			<label for="hy2password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Password *</label>
			<input
				id="hy2password"
				type="password"
				bind:value={hy2Password}
				placeholder="Authentication password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
		</div>

		<!-- TLS SNI -->
		<div>
			<label for="hy2sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">SNI</label>
			<input
				id="hy2sni"
				type="text"
				bind:value={tlsSni}
				placeholder="Server name (optional)"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>

		<!-- Insecure -->
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" bind:checked={hy2Insecure} />
			Skip certificate verification (insecure)
		</label>

		<!-- Port Hopping -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Port Hopping (optional)</h3>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="hy2ServerPorts" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Server Ports</label>
					<input
						id="hy2ServerPorts"
						type="text"
						bind:value={hy2ServerPorts}
						placeholder="1000-2000,3000-4000"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">Port ranges for hopping</p>
				</div>
				<div>
					<label for="hy2HopInterval" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Hop Interval</label>
					<input
						id="hy2HopInterval"
						type="text"
						bind:value={hy2HopInterval}
						placeholder="30s"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
			</div>
		</div>

		<!-- Bandwidth Limits -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Bandwidth Limits (optional)</h3>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="hy2UpMbps" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Upload (Mbps)</label>
					<input
						id="hy2UpMbps"
						type="number"
						bind:value={hy2UpMbps}
						min="0"
						placeholder="0 = unlimited"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div>
					<label for="hy2DownMbps" class="block text-xs text-[var(--ctp-overlay0)] mb-1">Download (Mbps)</label>
					<input
						id="hy2DownMbps"
						type="number"
						bind:value={hy2DownMbps}
						min="0"
						placeholder="0 = unlimited"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
			</div>
		</div>

		<!-- Obfuscation -->
		<div class="space-y-4">
			<div>
				<label for="obfsType" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Obfuscation Type</label>
				<select
					id="obfsType"
					bind:value={hy2ObfsType}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">None</option>
					<option value="salamander">Salamander</option>
				</select>
			</div>

			{#if hy2ObfsType}
				<div>
					<label for="obfsPassword" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Obfuscation Password</label>
					<input
						id="obfsPassword"
						type="text"
						bind:value={hy2ObfsPassword}
						placeholder="Obfuscation password"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			{/if}
		</div>
	{/if}

	{#if type === 'direct' || type === 'block'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
			{#if type === 'direct'}
				Direct outbound connects to destinations without using a proxy.
			{:else}
				Block outbound drops all connections.
			{/if}
		</div>
	{/if}

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			Cancel
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{outbound ? 'Save Changes' : 'Create Outbound'}
		</button>
	</div>
</form>

<!-- Import Modal -->
{#if showImport}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-full max-w-lg mx-4 shadow-xl">
			<div class="flex items-center justify-between mb-4">
				<h3 class="text-lg font-semibold text-[var(--ctp-text)]">
					Import {type === 'vless' ? 'VLESS' : 'Hysteria2'} Configuration
				</h3>
				<button
					onclick={() => { showImport = false; importText = ''; importError = ''; }}
					class="p-1 hover:bg-[var(--ctp-surface1)] rounded transition-colors"
				>
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<p class="text-sm text-[var(--ctp-subtext0)] mb-4">
				Paste a {type === 'vless' ? 'vless://' : 'hy2:// or hysteria2://'} link to import configuration.
			</p>

			<textarea
				bind:value={importText}
				placeholder="{type === 'vless' ? 'vless://uuid@server:port?params#name' : 'hy2://password@server:port?params#name'}"
				rows="4"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm resize-none"
			></textarea>

			{#if importError}
				<p class="mt-2 text-sm text-[var(--ctp-red)]">{importError}</p>
			{/if}

			<div class="flex justify-end gap-3 mt-4">
				<button
					onclick={() => { showImport = false; importText = ''; importError = ''; }}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					Cancel
				</button>
				<button
					onclick={parseImportLink}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					Import
				</button>
			</div>
		</div>
	</div>
{/if}

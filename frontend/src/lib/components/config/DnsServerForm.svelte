<script lang="ts">
	import type { DnsServer, Outbound } from '$lib/types';
	import { notifications } from '$lib/stores';
	import { t } from 'svelte-i18n';

	interface Props {
		server?: DnsServer;
		existingTags: string[];
		existingServers: DnsServer[];
		outbounds: Outbound[];
		onSave: (server: DnsServer) => void;
		onCancel: () => void;
	}

	let { server, existingTags, existingServers, outbounds, onSave, onCancel }: Props = $props();

	// Popular presets - some with domains (need bootstrap), some with IPs
	const PRESETS = [
		{ tag: 'google-ip', name: 'Google (IP)', type: 'tls' as const, server: '8.8.8.8' },
		{ tag: 'cloudflare-ip', name: 'Cloudflare (IP)', type: 'tls' as const, server: '1.1.1.1' },
		{ tag: 'google', name: 'Google DNS', type: 'tls' as const, server: 'dns.google', needsResolver: true },
		{ tag: 'cloudflare', name: 'Cloudflare', type: 'tls' as const, server: '1.1.1.1' },
		{ tag: 'local', name: 'System DNS', type: 'local' as const, server: '' }
	];

	// Form state
	let tag = $state(server?.tag ?? '');
	let type = $state<DnsServer['type']>(server?.type ?? 'tls');
	let serverAddr = $state(server?.server ?? '');
	let serverPort = $state(server?.server_port?.toString() ?? '');
	let detour = $state(server?.detour ?? '');
	let domainResolver = $state(server?.domain_resolver ?? '');
	let domainStrategy = $state(server?.domain_strategy ?? '');

	// FakeIP specific
	let inet4Range = $state(server?.inet4_range ?? '198.18.0.0/15');
	let inet6Range = $state(server?.inet6_range ?? 'fc00::/18');

	let errors = $state<Record<string, string>>({});

	// Check if server address is a domain name (not IP)
	function isDomainName(addr: string): boolean {
		if (!addr.trim()) return false;
		// Simple check: if it contains letters and is not an IPv6 address
		const trimmed = addr.trim();
		// IPv4 pattern
		const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
		// IPv6 pattern (simplified)
		const ipv6Regex = /^[\da-fA-F:]+$/;

		if (ipv4Regex.test(trimmed)) return false;
		if (ipv6Regex.test(trimmed) && trimmed.includes(':')) return false;

		// If it contains letters, it's likely a domain
		return /[a-zA-Z]/.test(trimmed);
	}

	// Get available bootstrap DNS servers (only IP-based or local)
	const bootstrapServers = $derived(
		existingServers.filter(s => {
			if (s.tag === tag) return false; // Can't reference self
			if (s.type === 'local') return true;
			if (s.type === 'fakeip') return false;
			// Check if server address is IP-based
			return s.server && !isDomainName(s.server);
		})
	);

	// Check if current server needs domain resolver
	const needsDomainResolver = $derived(
		type !== 'local' && type !== 'fakeip' && isDomainName(serverAddr)
	);

	function applyPreset(preset: typeof PRESETS[0]) {
		tag = preset.tag;
		type = preset.type;
		serverAddr = preset.server;
		serverPort = '';
		detour = '';
		domainResolver = '';
	}

	function validate(): boolean {
		errors = {};

		if (!tag.trim()) {
			errors['tag'] = $t('dns.validation.tagRequired');
		} else if (!server && existingTags.includes(tag.trim())) {
			errors['tag'] = $t('dns.validation.tagExists');
		}

		if (type !== 'local' && type !== 'fakeip' && !serverAddr.trim()) {
			errors['server'] = $t('dns.validation.serverRequired');
		}

		if (serverPort && (isNaN(parseInt(serverPort)) || parseInt(serverPort) < 1 || parseInt(serverPort) > 65535)) {
			errors['port'] = $t('dns.validation.invalidPort');
		}

		// Validate domain resolver requirement
		if (needsDomainResolver && !domainResolver) {
			errors['domain_resolver'] = $t('dns.validation.domainResolverRequired');
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

		const newServer: DnsServer = {
			tag: tag.trim(),
			type
		};

		if (type === 'fakeip') {
			if (inet4Range.trim()) newServer.inet4_range = inet4Range.trim();
			if (inet6Range.trim()) newServer.inet6_range = inet6Range.trim();
		} else if (type !== 'local') {
			newServer.server = serverAddr.trim();

			if (serverPort) {
				newServer.server_port = parseInt(serverPort);
			}

			if (needsDomainResolver && domainResolver) {
				newServer.domain_resolver = domainResolver;
				if (domainStrategy) {
					newServer.domain_strategy = domainStrategy as DnsServer['domain_strategy'];
				}
			}
		}

		if (detour && type !== 'local') {
			newServer.detour = detour;
		}

		onSave(newServer);
	}

	const serverTypes: { value: DnsServer['type']; labelKey: string; descriptionKey: string }[] = [
		{ value: 'tls', labelKey: 'dns.serverTypes.tls', descriptionKey: 'dns.serverTypes.tlsDesc' },
		{ value: 'https', labelKey: 'dns.serverTypes.https', descriptionKey: 'dns.serverTypes.httpsDesc' },
		{ value: 'udp', labelKey: 'dns.serverTypes.udp', descriptionKey: 'dns.serverTypes.udpDesc' },
		{ value: 'tcp', labelKey: 'dns.serverTypes.tcp', descriptionKey: 'dns.serverTypes.tcpDesc' },
		{ value: 'local', labelKey: 'dns.serverTypes.local', descriptionKey: 'dns.serverTypes.localDesc' },
		{ value: 'fakeip', labelKey: 'dns.serverTypes.fakeip', descriptionKey: 'dns.serverTypes.fakeipDesc' }
	];

	const strategyOptions = [
		{ value: '', labelKey: 'common.default' },
		{ value: 'prefer_ipv4', labelKey: 'dns.strategies.preferIpv4' },
		{ value: 'prefer_ipv6', labelKey: 'dns.strategies.preferIpv6' },
		{ value: 'ipv4_only', labelKey: 'dns.strategies.ipv4Only' },
		{ value: 'ipv6_only', labelKey: 'dns.strategies.ipv6Only' }
	];
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Presets -->
	{#if !server}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('dns.quickAddPreset')}</label>
			<div class="flex flex-wrap gap-2">
				{#each PRESETS as preset}
					<button
						type="button"
						onclick={() => applyPreset(preset)}
						disabled={existingTags.includes(preset.tag)}
						class="preset-btn"
					>
						{preset.name}
					</button>
				{/each}
			</div>
		</div>

		<div class="border-t border-[var(--ctp-surface2)] pt-4">
			<p class="text-sm text-[var(--ctp-overlay0)] mb-4">{$t('dns.orConfigureManually')}</p>
		</div>
	{/if}

	<!-- Tag -->
	<div>
		<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
		<input
			id="tag"
			type="text"
			bind:value={tag}
			placeholder="my-dns"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['tag']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
		{/if}
	</div>

	<!-- Type -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('common.type')}</label>
		<div class="grid grid-cols-2 gap-2">
			{#each serverTypes as st}
				<button
					type="button"
					onclick={() => type = st.value}
					class="type-btn {type === st.value ? 'selected' : ''}"
				>
					<div class="type-label">{$t(st.labelKey)}</div>
					<div class="type-desc">{$t(st.descriptionKey)}</div>
				</button>
			{/each}
		</div>
	</div>

	<!-- Server Address -->
	{#if type !== 'local' && type !== 'fakeip'}
		<div>
			<label for="server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('dns.serverAddress')} *</label>
			<input
				id="server"
				type="text"
				bind:value={serverAddr}
				placeholder={type === 'tls' || type === 'https' ? '8.8.8.8 or dns.google' : '8.8.8.8'}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['server']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['server']}</p>
			{/if}
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
				{$t('dns.serverAddressHint')}
			</p>
		</div>

		<!-- Domain Resolver Warning & Selection -->
		{#if needsDomainResolver}
			<div class="alert-box primary">
				<div class="flex items-start gap-2">
					<svg class="w-5 h-5 text-[var(--ctp-primary)] flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<div>
						<p class="text-sm font-medium text-[var(--ctp-text)]">{$t('dns.domainResolverRequired')}</p>
						<p class="text-xs text-[var(--ctp-overlay1)] mt-1">
							{$t('dns.domainResolverRequiredHint')}
						</p>
					</div>
				</div>
			</div>

			<div>
				<label for="domain_resolver" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('dns.domainResolver')} *
				</label>
				{#if bootstrapServers.length === 0}
					<div class="alert-box error">
						<p class="text-sm text-[var(--ctp-red)]">
							{$t('dns.noBootstrapDns')}
						</p>
					</div>
				{:else}
					<select
						id="domain_resolver"
						bind:value={domainResolver}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['domain_resolver'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
					>
						<option value="">{$t('dns.selectBootstrapDns')}</option>
						{#each bootstrapServers as bs}
							<option value={bs.tag}>
								{bs.tag} ({bs.type}{bs.server ? ` - ${bs.server}` : ''})
							</option>
						{/each}
					</select>
					{#if errors['domain_resolver']}
						<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['domain_resolver']}</p>
					{/if}
				{/if}
			</div>

			<!-- Domain Strategy (optional) -->
			<div>
				<label for="domain_strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('dns.domainStrategy')}
					<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
				</label>
				<select
					id="domain_strategy"
					bind:value={domainStrategy}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					{#each strategyOptions as opt}
						<option value={opt.value}>{$t(opt.labelKey)}</option>
					{/each}
				</select>
			</div>
		{/if}

		<!-- Port -->
		<div>
			<label for="port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('common.port')}
				<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
			</label>
			<input
				id="port"
				type="number"
				bind:value={serverPort}
				placeholder={type === 'tls' ? '853' : type === 'https' ? '443' : '53'}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['port'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['port']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['port']}</p>
			{/if}
		</div>
	{/if}

	<!-- FakeIP Configuration -->
	{#if type === 'fakeip'}
		<div class="alert-box primary">
			<div class="flex items-start gap-2">
				<svg class="w-5 h-5 text-[var(--ctp-primary)] flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<div>
					<p class="text-sm font-medium text-[var(--ctp-text)]">{$t('dns.fakeipRequiresRules')}</p>
					<p class="text-xs text-[var(--ctp-overlay1)] mt-1">
						{$t('dns.fakeipRequiresRulesHint')}
					</p>
				</div>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="inet4_range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('dns.ipv4Range')}
				</label>
				<input
					id="inet4_range"
					type="text"
					bind:value={inet4Range}
					placeholder="198.18.0.0/15"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
				/>
			</div>
			<div>
				<label for="inet6_range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('dns.ipv6Range')}
				</label>
				<input
					id="inet6_range"
					type="text"
					bind:value={inet6Range}
					placeholder="fc00::/18"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
				/>
			</div>
		</div>
		<p class="text-xs text-[var(--ctp-overlay0)]">
			{$t('dns.fakeipRangeHint')}
		</p>
	{/if}

	<!-- Detour -->
	{#if type !== 'local' && type !== 'fakeip'}
		<div>
			<label for="detour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('dns.detour')}
				<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
			</label>
			<select
				id="detour"
				bind:value={detour}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="">{$t('dns.noneDirect')}</option>
				{#each outbounds as ob}
					<option value={ob.tag}>{ob.tag} ({ob.type})</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('dns.detourHint')}</p>
		</div>
	{/if}

	<!-- Info for local type -->
	{#if type === 'local'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
			{$t('dns.localDnsHint')}
		</div>
	{/if}

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('common.cancel')}
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{server ? $t('common.saveChanges') : $t('dns.addServer')}
		</button>
	</div>
</form>

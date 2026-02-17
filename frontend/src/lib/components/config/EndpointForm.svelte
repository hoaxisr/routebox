<script lang="ts">
	import type { Endpoint, AWGPeer, DnsServer } from '$lib/types';
	import { notifications, featureFlags } from '$lib/stores';
	import { t } from 'svelte-i18n';
	import {
		validateRequired,
		validateBase64Key,
		validateOptionalPort,
		parseCSV,
		hasValidationErrors,
		isDomain
	} from '$lib/utils';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';
	import ObfuscationFields from './endpoint/ObfuscationFields.svelte';
	import PeerList from './endpoint/PeerList.svelte';
	import ImportDialog from './endpoint/ImportDialog.svelte';

	interface Props {
		endpoint?: Endpoint;
		dnsServers?: DnsServer[];
		hasDefaultResolver?: boolean;
		onSave: (endpoint: Endpoint) => void;
		onCancel: () => void;
	}

	let { endpoint, dnsServers = [], hasDefaultResolver = false, onSave, onCancel }: Props = $props();

	// Form state
	let tag = $state(endpoint?.tag ?? '');
	let type = $state(endpoint?.type ?? 'awg');
	let privateKey = $state(endpoint?.private_key ?? '');
	let addresses = $state(endpoint?.address?.join(', ') ?? '');
	let mtu = $state(endpoint?.mtu ?? 1280);
	let listenPort = $state(endpoint?.listen_port ?? 0);

	// Advanced options
	let systemInterface = $state(endpoint?.system ?? false);
	let interfaceName = $state(endpoint?.name ?? '');
	let udpTimeout = $state(endpoint?.udp_timeout ?? '');
	let workers = $state(endpoint?.workers ?? 0);

	// AWG obfuscation
	let jc = $state(endpoint?.jc ?? 0);
	let jmin = $state(endpoint?.jmin ?? 0);
	let jmax = $state(endpoint?.jmax ?? 0);
	let s1 = $state(endpoint?.s1 ?? 0);
	let s2 = $state(endpoint?.s2 ?? 0);
	let s3 = $state(endpoint?.s3 ?? 0);
	let s4 = $state(endpoint?.s4 ?? 0);
	let h1 = $state(endpoint?.h1 ?? '');
	let h2 = $state(endpoint?.h2 ?? '');
	let h3 = $state(endpoint?.h3 ?? '');
	let h4 = $state(endpoint?.h4 ?? '');
	let i1 = $state(endpoint?.i1 ?? '');
	let i2 = $state(endpoint?.i2 ?? '');
	let i3 = $state(endpoint?.i3 ?? '');
	let i4 = $state(endpoint?.i4 ?? '');
	let i5 = $state(endpoint?.i5 ?? '');

	let peers = $state<AWGPeer[]>(endpoint?.peers ?? [
		{ address: '', port: 51820, public_key: '', allowed_ips: ['0.0.0.0/0', '::/0'] }
	]);

	// Domain resolver (sing-box 1.12+)
	let domainResolver = $state(endpoint?.domain_resolver ?? '');

	// Show domain resolver if any peer address is a domain and no default resolver
	let showDomainResolver = $derived(
		$featureFlags['domain_resolver'] &&
		!hasDefaultResolver &&
		peers.some(p => isDomain(p.address))
	);

	// UI state
	let activeTab = $state<'basic' | 'obfuscation' | 'peers' | 'advanced'>('basic');
	let errors = $state<Record<string, string>>({});
	let showImport = $state(false);

	function parseAwgConfig(text: string) {
		const lines = text.split('\n').map(l => l.trim());
		const config: Record<string, string> = {};
		let currentSection = '';
		const peerConfigs: Record<string, string>[] = [];
		let currentPeer: Record<string, string> | null = null;

		for (const line of lines) {
			if (!line || line.startsWith('#')) continue;

			if (line.startsWith('[') && line.endsWith(']')) {
				const section = line.slice(1, -1).toLowerCase();
				if (section === 'peer') {
					if (currentPeer) peerConfigs.push(currentPeer);
					currentPeer = {};
				}
				currentSection = section;
				continue;
			}

			const match = line.match(/^(\w+)\s*=\s*(.+)$/);
			if (match) {
				const [, key, value] = match;
				const k = key.toLowerCase();
				if (currentSection === 'peer' && currentPeer) {
					currentPeer[k] = value.trim();
				} else {
					config[k] = value.trim();
				}
			}
		}
		if (currentPeer) peerConfigs.push(currentPeer);

		// Apply parsed values
		if (config['privatekey']) privateKey = config['privatekey'];
		if (config['address']) addresses = config['address'];
		if (config['mtu']) mtu = parseInt(config['mtu']) || 1280;
		if (config['listenport']) listenPort = parseInt(config['listenport']) || 0;

		// AWG obfuscation
		if (config['jc']) jc = parseInt(config['jc']) || 0;
		if (config['jmin']) jmin = parseInt(config['jmin']) || 0;
		if (config['jmax']) jmax = parseInt(config['jmax']) || 0;
		if (config['s1']) s1 = parseInt(config['s1']) || 0;
		if (config['s2']) s2 = parseInt(config['s2']) || 0;
		if (config['s3']) s3 = parseInt(config['s3']) || 0;
		if (config['s4']) s4 = parseInt(config['s4']) || 0;
		if (config['h1']) h1 = config['h1'];
		if (config['h2']) h2 = config['h2'];
		if (config['h3']) h3 = config['h3'];
		if (config['h4']) h4 = config['h4'];
		if (config['i1']) i1 = config['i1'];
		if (config['i2']) i2 = config['i2'];
		if (config['i3']) i3 = config['i3'];
		if (config['i4']) i4 = config['i4'];
		if (config['i5']) i5 = config['i5'];

		// Parse peers
		if (peerConfigs.length > 0) {
			peers = peerConfigs.map(p => ({
				address: p['endpoint']?.split(':')[0] ?? '',
				port: parseInt(p['endpoint']?.split(':')[1] ?? '51820') || 51820,
				public_key: p['publickey'] ?? '',
				preshared_key: p['presharedkey'] || undefined,
				allowed_ips: (p['allowedips'] ?? '0.0.0.0/0, ::/0').split(',').map(s => s.trim()),
				persistent_keepalive_interval: parseInt(p['persistentkeepalive'] ?? '0') || undefined
			}));
		}

		showImport = false;
	}

	function validate(): boolean {
		errors = {};

		const tagResult = validateRequired(tag, 'Tag');
		if (!tagResult.valid) errors['tag'] = tagResult.error!;

		const keyResult = validateBase64Key(privateKey, 'Private key');
		if (!keyResult.valid) errors['privateKey'] = keyResult.error!;

		const addressResult = validateRequired(addresses, 'Address');
		if (!addressResult.valid) errors['addresses'] = addressResult.error!;

		if (listenPort > 0) {
			const portResult = validateOptionalPort(listenPort);
			if (!portResult.valid) errors['listenPort'] = portResult.error!;
		}

		if (peers.length === 0) {
			errors['peers'] = 'At least one peer is required';
		}
		peers.forEach((peer, i) => {
			const addrResult = validateRequired(peer.address, 'Server address');
			if (!addrResult.valid) errors[`peer_${i}_address`] = addrResult.error!;

			const pubKeyResult = validateBase64Key(peer.public_key, 'Public key');
			if (!pubKeyResult.valid) errors[`peer_${i}_public_key`] = pubKeyResult.error!;
		});

		if (hasValidationErrors(errors)) {
			const firstError = Object.keys(errors)[0];
			if (firstError === 'tag' || firstError === 'privateKey' || firstError === 'addresses' || firstError === 'listenPort') {
				activeTab = 'basic';
			} else if (firstError === 'peers' || firstError.startsWith('peer_')) {
				activeTab = 'peers';
			}
			notifications.error(errors[firstError]);
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const ep: Endpoint = {
			type,
			tag: tag.trim(),
			private_key: privateKey.trim(),
			address: parseCSV(addresses),
			mtu,
			peers: peers.map((p) => ({
				address: p.address.trim(),
				port: p.port,
				public_key: p.public_key.trim(),
				preshared_key: p.preshared_key?.trim() || undefined,
				allowed_ips: p.allowed_ips,
				persistent_keepalive_interval: p.persistent_keepalive_interval || undefined
			}))
		};

		if (listenPort > 0) ep.listen_port = listenPort;

		// AWG obfuscation - only add non-zero/non-empty values
		if (jc > 0) ep.jc = jc;
		if (jmin > 0) ep.jmin = jmin;
		if (jmax > 0) ep.jmax = jmax;
		if (s1 > 0) ep.s1 = s1;
		if (s2 > 0) ep.s2 = s2;
		if (s3 > 0) ep.s3 = s3;
		if (s4 > 0) ep.s4 = s4;
		if (h1.trim()) ep.h1 = h1.trim();
		if (h2.trim()) ep.h2 = h2.trim();
		if (h3.trim()) ep.h3 = h3.trim();
		if (h4.trim()) ep.h4 = h4.trim();
		if (i1.trim()) ep.i1 = i1.trim();
		if (i2.trim()) ep.i2 = i2.trim();
		if (i3.trim()) ep.i3 = i3.trim();
		if (i4.trim()) ep.i4 = i4.trim();
		if (i5.trim()) ep.i5 = i5.trim();

		// Advanced options
		if (systemInterface) ep.system = true;
		if (interfaceName.trim()) ep.name = interfaceName.trim();
		if (udpTimeout.trim()) ep.udp_timeout = udpTimeout.trim();
		if (workers > 0) ep.workers = workers;

		// Domain resolver (sing-box 1.12+)
		if (domainResolver.trim()) ep.domain_resolver = domainResolver.trim();

		onSave(ep);
	}

	const tabIds = ['basic', 'obfuscation', 'peers', 'advanced'] as const;
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Tabs -->
	<div class="flex gap-1 bg-[var(--ctp-surface0)] p-1 rounded-lg">
		{#each tabIds as tabId}
			<button
				type="button"
				onclick={() => activeTab = tabId}
				class="flex-1 px-3 py-2 text-sm rounded-md transition-colors {activeTab === tabId ? 'bg-[var(--ctp-primary)] text-white' : 'text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)]'}"
			>
				{$t(`endpoints.tabs.${tabId}`)}
				{#if tabId === 'peers'}
					<span class="ml-1 text-xs opacity-70">({peers.length})</span>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Basic Tab -->
	{#if activeTab === 'basic'}
		<div class="space-y-4">
			{#if !endpoint}
				<button type="button" onclick={() => showImport = true} class="import-btn">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
					</svg>
					{$t('endpoints.importFromConfig')}
				</button>
			{/if}

			<div>
				<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
				<input
					id="tag"
					type="text"
					bind:value={tag}
					placeholder="awg-vpn"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				{#if errors['tag']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
				{/if}
			</div>

			<div>
				<label for="type" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.type')}</label>
				<select
					id="type"
					bind:value={type}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="awg">AmneziaWG (AWG)</option>
					<option value="wireguard">WireGuard</option>
				</select>
			</div>

			<div>
				<label for="privateKey" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('endpoints.privateKey')} *</label>
				<input
					id="privateKey"
					type="password"
					bind:value={privateKey}
					placeholder={$t('endpoints.placeholders.privateKey')}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono {errors['privateKey'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				{#if errors['privateKey']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['privateKey']}</p>
				{/if}
			</div>

			<div>
				<label for="addresses" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('endpoints.addresses')} *</label>
				<input
					id="addresses"
					type="text"
					bind:value={addresses}
					placeholder="10.0.0.2/32, fd00::2/128"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['addresses'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('endpoints.addressesHint')}</p>
				{#if errors['addresses']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['addresses']}</p>
				{/if}
			</div>

			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<label for="mtu" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('endpoints.mtu')}</label>
					<input
						id="mtu"
						type="number"
						bind:value={mtu}
						min="576"
						max="1500"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
				<div>
					<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenPort')}</label>
					<input
						id="listenPort"
						type="number"
						bind:value={listenPort}
						min="0"
						max="65535"
						placeholder={$t('endpoints.placeholders.random')}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			</div>
		</div>
	{/if}

	<!-- Obfuscation Tab -->
	{#if activeTab === 'obfuscation'}
		<ObfuscationFields
			bind:jc bind:jmin bind:jmax
			bind:s1 bind:s2 bind:s3 bind:s4
			bind:h1 bind:h2 bind:h3 bind:h4
			bind:i1 bind:i2 bind:i3 bind:i4 bind:i5
		/>
	{/if}

	<!-- Peers Tab -->
	{#if activeTab === 'peers'}
		<PeerList bind:peers {errors} />
	{/if}

	<!-- Advanced Tab -->
	{#if activeTab === 'advanced'}
		<div class="space-y-4">
			<p class="text-sm text-[var(--ctp-overlay0)]">
				{$t('endpoints.advancedHint')}
			</p>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('endpoints.interfaceOptions')}</h3>

				<label class="flex items-center gap-3 p-3 bg-[var(--ctp-mantle)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
					<input
						type="checkbox"
						bind:checked={systemInterface}
						class="w-5 h-5 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					<div>
						<span class="text-[var(--ctp-text)] font-medium">{$t('endpoints.systemInterface')}</span>
						<p class="text-sm text-[var(--ctp-overlay0)]">{$t('endpoints.systemInterfaceHint')}</p>
					</div>
				</label>

				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
					<div>
						<label for="interfaceName" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('inbounds.interfaceName')}</label>
						<input
							id="interfaceName"
							type="text"
							bind:value={interfaceName}
							placeholder="wg0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="workers" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.workers')}</label>
						<input
							id="workers"
							type="number"
							bind:value={workers}
							min="0"
							placeholder={$t('common.auto')}
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>

				<div>
					<label for="udpTimeout" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.udpTimeout')}</label>
					<input
						id="udpTimeout"
						type="text"
						bind:value={udpTimeout}
						placeholder={$t('endpoints.placeholders.udpTimeout')}
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('endpoints.durationFormatHint')}</p>
				</div>

				{#if showDomainResolver}
					<div>
						<label for="domainResolver" class="flex items-center gap-1 text-xs text-[var(--ctp-overlay0)] mb-1">
							{$t('endpoints.domainResolver')}
							<HelpTooltip text={$t('help.domainResolver')} />
						</label>
						<select
							id="domainResolver"
							bind:value={domainResolver}
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							<option value="">{$t('common.none')}</option>
							{#each dnsServers as dns}
								<option value={dns.tag}>{dns.tag}</option>
							{/each}
						</select>
						<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('endpoints.domainResolverHint')}</p>
					</div>
				{/if}
			</div>
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
			{endpoint ? $t('common.saveChanges') : $t('endpoints.createEndpoint')}
		</button>
	</div>
</form>

<!-- Import Dialog -->
{#if showImport}
	<ImportDialog
		onImport={parseAwgConfig}
		onClose={() => showImport = false}
	/>
{/if}

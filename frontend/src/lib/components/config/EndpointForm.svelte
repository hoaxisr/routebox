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

	// AWG obfuscation - Junk packets
	let jc = $state(endpoint?.jc ?? 0);
	let jmin = $state(endpoint?.jmin ?? 0);
	let jmax = $state(endpoint?.jmax ?? 0);

	// AWG obfuscation - Init packet sizes (S1-S4)
	let s1 = $state(endpoint?.s1 ?? 0);
	let s2 = $state(endpoint?.s2 ?? 0);
	let s3 = $state(endpoint?.s3 ?? 0);
	let s4 = $state(endpoint?.s4 ?? 0);

	// AWG obfuscation - Header parameters (H1-H4) - strings
	let h1 = $state(endpoint?.h1 ?? '');
	let h2 = $state(endpoint?.h2 ?? '');
	let h3 = $state(endpoint?.h3 ?? '');
	let h4 = $state(endpoint?.h4 ?? '');

	// AWG obfuscation - Init parameters (I1-I5) - advanced packet manipulation
	let i1 = $state(endpoint?.i1 ?? '');
	let i2 = $state(endpoint?.i2 ?? '');
	let i3 = $state(endpoint?.i3 ?? '');
	let i4 = $state(endpoint?.i4 ?? '');
	let i5 = $state(endpoint?.i5 ?? '');

	// Peers - migrate legacy preshared_key to pre_shared_key
	let peers = $state<AWGPeer[]>(endpoint?.peers?.map(p => ({
		...p,
		pre_shared_key: p.pre_shared_key ?? (p as any).preshared_key
	})) ?? [{ address: '', port: 51820, public_key: '', allowed_ips: ['0.0.0.0/0', '::/0'] }]);

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
	let importText = $state('');
	let fileInput: HTMLInputElement;

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		const reader = new FileReader();
		reader.onload = (event) => {
			importText = event.target?.result as string || '';
		};
		reader.readAsText(file);
	}

	function addPeer() {
		peers = [...peers, { address: '', port: 51820, public_key: '', pre_shared_key: undefined, allowed_ips: ['0.0.0.0/0', '::/0'] }];
	}

	function removePeer(index: number) {
		peers = peers.filter((_, i) => i !== index);
	}

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
				pre_shared_key: p['presharedkey'] || undefined,
				allowed_ips: (p['allowedips'] ?? '0.0.0.0/0, ::/0').split(',').map(s => s.trim()),
				persistent_keepalive_interval: parseInt(p['persistentkeepalive'] ?? '0') || undefined
			}));
		}

		showImport = false;
		importText = '';
	}

	function validate(): boolean {
		errors = {};

		// Basic fields
		const tagResult = validateRequired(tag, 'Tag');
		if (!tagResult.valid) errors['tag'] = tagResult.error!;

		const keyResult = validateBase64Key(privateKey, 'Private key');
		if (!keyResult.valid) errors['privateKey'] = keyResult.error!;

		const addressResult = validateRequired(addresses, 'Address');
		if (!addressResult.valid) errors['addresses'] = addressResult.error!;

		// Optional port validation
		if (listenPort > 0) {
			const portResult = validateOptionalPort(listenPort);
			if (!portResult.valid) errors['listenPort'] = portResult.error!;
		}

		// Peers
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
			// Switch to tab with first error
			const firstError = Object.keys(errors)[0];
			if (firstError === 'tag' || firstError === 'privateKey' || firstError === 'addresses' || firstError === 'listenPort') {
				activeTab = 'basic';
			} else if (firstError === 'peers' || firstError.startsWith('peer_')) {
				activeTab = 'peers';
			}

			// Show notification with first error
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
				pre_shared_key: p.pre_shared_key?.trim() || undefined,
				allowed_ips: p.allowed_ips,
				persistent_keepalive_interval: p.persistent_keepalive_interval || undefined
			}))
		};

		// Add optional fields
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
			<!-- Import button -->
			{#if !endpoint}
				<button
					type="button"
					onclick={() => showImport = true}
					class="import-btn"
				>
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
		<div class="space-y-4">
			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.junkPackets')}</h3>
				<div class="grid grid-cols-3 gap-4">
					<div>
						<label for="jc" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jc')}</label>
						<input
							id="jc"
							type="number"
							bind:value={jc}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="jmin" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jmin')}</label>
						<input
							id="jmin"
							type="number"
							bind:value={jmin}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="jmax" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jmax')}</label>
						<input
							id="jmax"
							type="number"
							bind:value={jmax}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			</div>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.initPacketParams')}</h3>
				<div class="grid grid-cols-4 gap-4">
					<div>
						<label for="s1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S1</label>
						<input
							id="s1"
							type="number"
							bind:value={s1}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="s2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S2</label>
						<input
							id="s2"
							type="number"
							bind:value={s2}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="s3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S3</label>
						<input
							id="s3"
							type="number"
							bind:value={s3}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
					<div>
						<label for="s4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S4</label>
						<input
							id="s4"
							type="number"
							bind:value={s4}
							min="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			</div>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.headerParams')}</h3>
				<div class="grid grid-cols-4 gap-4">
					<div>
						<label for="h1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H1</label>
						<input
							id="h1"
							type="text"
							bind:value={h1}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="h2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H2</label>
						<input
							id="h2"
							type="text"
							bind:value={h2}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="h3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H3</label>
						<input
							id="h3"
							type="text"
							bind:value={h3}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="h4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H4</label>
						<input
							id="h4"
							type="text"
							bind:value={h4}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
				</div>
			</div>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.initParams')}</h3>
				<div class="grid grid-cols-5 gap-3">
					<div>
						<label for="i1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I1</label>
						<input
							id="i1"
							type="text"
							bind:value={i1}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="i2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I2</label>
						<input
							id="i2"
							type="text"
							bind:value={i2}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="i3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I3</label>
						<input
							id="i3"
							type="text"
							bind:value={i3}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="i4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I4</label>
						<input
							id="i4"
							type="text"
							bind:value={i4}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
					<div>
						<label for="i5" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I5</label>
						<input
							id="i5"
							type="text"
							bind:value={i5}
							placeholder="0"
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>
				</div>
			</div>

			<p class="text-sm text-[var(--ctp-overlay0)]">
				{$t('endpoints.obfuscation.hint')}
			</p>
		</div>
	{/if}

	<!-- Peers Tab -->
	{#if activeTab === 'peers'}
		<div class="space-y-4">
			{#if errors['peers']}
				<p class="text-sm text-[var(--ctp-red)]">{errors['peers']}</p>
			{/if}

			{#each peers as peer, i}
				<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('endpoints.peer')} {i + 1}</h3>
						{#if peers.length > 1}
							<button
								type="button"
								onclick={() => removePeer(i)}
								class="action-btn-danger"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						{/if}
					</div>

					<div class="grid grid-cols-3 gap-4">
						<div class="col-span-2">
							<label for="peer_{i}_address" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.serverAddress')} *</label>
							<input
								id="peer_{i}_address"
								type="text"
								bind:value={peer.address}
								placeholder="vpn.example.com"
								class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors[`peer_${i}_address`] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
							/>
						</div>
						<div>
							<label for="peer_{i}_port" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('common.port')}</label>
							<input
								id="peer_{i}_port"
								type="number"
								bind:value={peer.port}
								min="1"
								max="65535"
								class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
						</div>
					</div>

					<div>
						<label for="peer_{i}_public_key" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.publicKey')} *</label>
						<input
							id="peer_{i}_public_key"
							type="text"
							bind:value={peer.public_key}
							placeholder={$t('endpoints.placeholders.publicKey')}
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm {errors[`peer_${i}_public_key`] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
						/>
					</div>

					<div>
						<label for="peer_{i}_psk" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.presharedKey')} ({$t('common.optional')})</label>
						<input
							id="peer_{i}_psk"
							type="password"
							bind:value={peer.pre_shared_key}
							placeholder={$t('endpoints.placeholders.psk')}
							class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
						/>
					</div>

					<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
						<div>
							<label for="peer_{i}_allowed_ips" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.allowedIPs')}</label>
							<input
								id="peer_{i}_allowed_ips"
								type="text"
								value={peer.allowed_ips.join(', ')}
								oninput={(e) => peer.allowed_ips = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean)}
								class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
							/>
						</div>
						<div>
							<label for="peer_{i}_keepalive" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.keepalive')}</label>
							<input
								id="peer_{i}_keepalive"
								type="number"
								bind:value={peer.persistent_keepalive_interval}
								min="0"
								placeholder="25"
								class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
						</div>
					</div>
				</div>
			{/each}

			<button
				type="button"
				onclick={addPeer}
				class="w-full py-2 border-2 border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors"
			>
				+ {$t('endpoints.addPeer')}
			</button>
		</div>
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

				<!-- Domain Resolver -->
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
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
		onclick={(e) => { if (e.target === e.currentTarget) showImport = false; }}
		role="dialog"
		aria-modal="true"
	>
		<div class="bg-[var(--ctp-base)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
			<div class="flex items-center justify-between px-6 py-4 border-b border-[var(--ctp-surface2)]">
				<h2 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('endpoints.importDialogTitle')}</h2>
				<button
					onclick={() => showImport = false}
					class="p-1 hover:bg-[var(--ctp-surface1)] rounded-lg transition-colors"
					aria-label={$t('common.close')}
				>
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<div class="flex-1 overflow-y-auto p-6 space-y-4">
				<p class="text-sm text-[var(--ctp-overlay1)]">
					{$t('endpoints.importDialogHint')}
				</p>

				<!-- File upload -->
				<input
					type="file"
					accept=".conf,.txt"
					bind:this={fileInput}
					onchange={handleFileSelect}
					class="hidden"
				/>
				<button
					type="button"
					onclick={() => fileInput.click()}
					class="import-btn"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
					</svg>
					{$t('endpoints.selectConfFile')}
				</button>

				<div class="relative flex items-center">
					<div class="flex-1 border-t border-[var(--ctp-surface2)]"></div>
					<span class="px-3 text-sm text-[var(--ctp-overlay0)]">{$t('endpoints.orPasteConfig')}</span>
					<div class="flex-1 border-t border-[var(--ctp-surface2)]"></div>
				</div>

				<textarea
					bind:value={importText}
					placeholder="[Interface]
PrivateKey = ...
Address = 10.0.0.2/32
MTU = 1280
Jc = 4
Jmin = 50
Jmax = 1000
...

[Peer]
PublicKey = ...
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0"
					class="w-full h-48 px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm resize-none"
				></textarea>
			</div>

			<div class="px-6 py-4 border-t border-[var(--ctp-surface2)] flex justify-end gap-3">
				<button
					onclick={() => showImport = false}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{$t('common.cancel')}
				</button>
				<button
					onclick={() => parseAwgConfig(importText)}
					disabled={!importText.trim()}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{$t('common.import')}
				</button>
			</div>
		</div>
	</div>
{/if}

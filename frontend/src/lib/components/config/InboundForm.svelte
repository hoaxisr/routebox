<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Inbound } from '$lib/types';
	import { notifications } from '$lib/stores';
	import ServerVlessInbound from './inbound/ServerVlessInbound.svelte';
	import ServerNaiveInbound from './inbound/ServerNaiveInbound.svelte';
	import ServerHysteria2Inbound from './inbound/ServerHysteria2Inbound.svelte';
	import ShareLinkModal from './inbound/ShareLinkModal.svelte';
	import { buildServerInbound, parseServerInbound, type ServerFormState, type ServerInboundType } from '$lib/utils/serverInbound';

	interface Props {
		inbound?: Inbound;
		onSave: (inbound: Inbound) => void;
		onCancel: () => void;
	}

	let { inbound, onSave, onCancel }: Props = $props();

	// Form state
	let tag = $state(inbound?.tag ?? '');
	let type = $state(inbound?.type ?? 'tun');

	// TUN specific - migrate legacy inet4/inet6_address to unified address array
	let interfaceName = $state(inbound?.interface_name ?? 'sing0');
	// Combine existing address array or legacy fields
	function getInitialAddresses(): string {
		if (inbound?.address?.length) {
			return inbound.address.join(', ');
		}
		const parts: string[] = [];
		if (inbound?.inet4_address) parts.push(inbound.inet4_address);
		if (inbound?.inet6_address) parts.push(inbound.inet6_address);
		return parts.length > 0 ? parts.join(', ') : '172.19.0.1/30';
	}
	let addresses = $state(getInitialAddresses());
	let mtu = $state(inbound?.mtu ?? 9000);
	let autoRoute = $state(inbound?.auto_route ?? true);
	let strictRoute = $state(inbound?.strict_route ?? true);
	let stack = $state<'system' | 'gvisor' | 'mixed'>(inbound?.stack ?? 'system');

	// Mixed/SOCKS/HTTP specific
	let listen = $state(inbound?.listen ?? '127.0.0.1');
	let listenPort = $state(inbound?.listen_port ?? 1080);
	let tcpFastOpen = $state(inbound?.tcp_fast_open ?? false);

	let errors = $state<Record<string, string>>({});

	const inboundTypes = ['tun', 'mixed', 'socks', 'http', 'vless', 'naive', 'hysteria2'] as const;
	const serverTypes = ['vless', 'naive', 'hysteria2'] as const;
	function isServerType(t: string): t is ServerInboundType {
		return (serverTypes as readonly string[]).includes(t);
	}

	const stackOptions = ['system', 'gvisor', 'mixed'] as const;

	let serverState = $state<ServerFormState>(
		inbound && isServerType(inbound.type)
			? parseServerInbound(inbound)
			: {
					type: 'vless', tag: '', listen: '::', listenPort: 443, tlsMode: 'reality',
					tls: {
						server_name: '', acme: { domain: '', email: '' },
						reality: { enabled: true, private_key: '', short_id: '' },
						certificate_path: '', key_path: ''
					},
					handshakeServer: '', handshakePort: 443,
					users: [], upMbps: 0, downMbps: 0, obfsType: '', obfsPassword: ''
				}
	);

	const editingExisting = !!inbound?.tag;
	let shareUserIndex = $state<number | null>(null);
	function openShare(index: number) {
		shareUserIndex = index;
	}

	// Switching protocol clears users, since credential fields differ per protocol
	// (a vless user has uuid/flow, naive has username/password) and mixing them
	// produces an invalid sing-box config.
	function selectType(newType: (typeof inboundTypes)[number]) {
		if (newType !== type && isServerType(newType)) {
			serverState.users = [];
		}
		type = newType;
	}

	function validate(): boolean {
		errors = {};

		if (isServerType(type)) {
			if (!tag.trim()) errors['tag'] = $t('errors.requiredField');
			if (serverState.listenPort < 1 || serverState.listenPort > 65535) {
				errors['port'] = $t('form.minValue', { values: { value: 1 } });
			}
			if (serverState.users.length === 0) errors['users'] = $t('inbounds.server.needUser');
			const keys = Object.keys(errors);
			if (keys.length > 0) {
				notifications.error(errors[keys[0]]);
				return false;
			}
			return true;
		}

		if (!tag.trim()) {
			errors['tag'] = $t('errors.requiredField');
		}

		if (type === 'tun') {
			if (!interfaceName.trim()) {
				errors['interfaceName'] = $t('errors.requiredField');
			}
			if (!addresses.trim()) {
				errors['addresses'] = $t('errors.requiredField');
			}
		} else {
			if (!listen.trim()) {
				errors['listen'] = $t('errors.requiredField');
			}
			if (listenPort < 1 || listenPort > 65535) {
				errors['port'] = $t('form.minValue', { values: { value: 1 } }) + ', ' + $t('form.maxValue', { values: { value: 65535 } });
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
		if (isServerType(type)) {
			onSave(buildServerInbound({ ...serverState, type: type as ServerInboundType, tag: tag.trim() }));
			return;
		}

		const ib: Inbound = {
			type,
			tag: tag.trim()
		};

		if (type === 'tun') {
			ib.interface_name = interfaceName.trim();
			// Use unified address array (sing-box 1.8+ format)
			ib.address = addresses.split(',').map(a => a.trim()).filter(Boolean);
			if (mtu !== 9000) ib.mtu = mtu;  // Only include if not default
			ib.auto_route = autoRoute;
			ib.strict_route = strictRoute;
			ib.stack = stack;
		} else {
			ib.listen = listen.trim();
			ib.listen_port = listenPort;
			if (tcpFastOpen) ib.tcp_fast_open = true;
		}

		onSave(ib);
	}
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Tag -->
	<div>
		<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
		<input
			id="tag"
			type="text"
			bind:value={tag}
			placeholder="tun-in"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['tag']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
		{/if}
	</div>

	<!-- Type -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('common.type')}</label>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
			{#each inboundTypes as inboundType}
				<button
					type="button"
					onclick={() => selectType(inboundType)}
					class="type-btn {type === inboundType ? 'selected' : ''}"
				>
					<div class="type-label">{$t(`inbounds.types.${inboundType}`)}</div>
					<div class="type-desc">{$t(`inbounds.typeDescriptions.${inboundType}`)}</div>
				</button>
			{/each}
		</div>
	</div>

	<!-- TUN specific options -->
	{#if type === 'tun'}
		<div class="space-y-4">
			<div class="grid grid-cols-3 gap-4">
				<div class="col-span-2">
					<label for="interfaceName" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.interfaceName')} *</label>
					<input
						id="interfaceName"
						type="text"
						bind:value={interfaceName}
						placeholder="sing0"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['interfaceName'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
					/>
					{#if errors['interfaceName']}
						<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['interfaceName']}</p>
					{/if}
				</div>
				<div>
					<label for="mtu" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.mtu')}</label>
					<input
						id="mtu"
						type="number"
						bind:value={mtu}
						min="576"
						max="65535"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			</div>

			<div>
				<label for="addresses" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.addresses')} *</label>
				<input
					id="addresses"
					type="text"
					bind:value={addresses}
					placeholder="172.19.0.1/30, fd00::1/126"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['addresses'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('inbounds.addressesHint')}</p>
				{#if errors['addresses']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['addresses']}</p>
				{/if}
			</div>

			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('inbounds.stack')}</label>
				<div class="grid grid-cols-3 gap-2">
					{#each stackOptions as stackOption}
						<button
							type="button"
							onclick={() => stack = stackOption}
							class="type-btn text-center {stack === stackOption ? 'selected' : ''}"
						>
							<div class="type-label text-sm">{$t(`inbounds.stacks.${stackOption}`)}</div>
							<div class="type-desc">{$t(`inbounds.stackDescriptions.${stackOption}`)}</div>
						</button>
					{/each}
				</div>
			</div>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('inbounds.routingOptions')}</h3>
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
					<input
						type="checkbox"
						bind:checked={autoRoute}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					{$t('inbounds.autoRoute')}
					<span class="text-[var(--ctp-overlay0)]">— {$t('inbounds.autoRouteHint')}</span>
				</label>
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
					<input
						type="checkbox"
						bind:checked={strictRoute}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					{$t('inbounds.strictRoute')}
					<span class="text-[var(--ctp-overlay0)]">— {$t('inbounds.strictRouteHint')}</span>
				</label>
			</div>
		</div>
	{/if}

	<!-- Proxy inbound options (mixed, socks, http) -->
	{#if type !== 'tun' && !isServerType(type)}
		<div class="space-y-4">
			<div class="grid grid-cols-3 gap-4">
				<div class="col-span-2">
					<label for="listen" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenAddress')} *</label>
					<input
						id="listen"
						type="text"
						bind:value={listen}
						placeholder="127.0.0.1"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['listen'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
					/>
					{#if errors['listen']}
						<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['listen']}</p>
					{/if}
				</div>
				<div>
					<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenPort')} *</label>
					<input
						id="listenPort"
						type="number"
						bind:value={listenPort}
						min="1"
						max="65535"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['port'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
					/>
					{#if errors['port']}
						<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['port']}</p>
					{/if}
				</div>
			</div>

			<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
				<input
					type="checkbox"
					bind:checked={tcpFastOpen}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('inbounds.tcpFastOpen')}
				<span class="text-[var(--ctp-overlay0)]">— {$t('inbounds.tcpFastOpenHint')}</span>
			</label>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
				{$t(`inbounds.typeHints.${type}`)}
			</div>
		</div>
	{/if}

	{#if type === 'vless'}
		<ServerVlessInbound bind:state={serverState} onShare={openShare} canShare={editingExisting} />
	{:else if type === 'naive'}
		<ServerNaiveInbound bind:state={serverState} onShare={openShare} canShare={editingExisting} />
	{:else if type === 'hysteria2'}
		<ServerHysteria2Inbound bind:state={serverState} onShare={openShare} canShare={editingExisting} />
	{/if}

	{#if shareUserIndex !== null && inbound?.tag}
		<ShareLinkModal tag={inbound.tag} userIndex={shareUserIndex} onClose={() => (shareUserIndex = null)} />
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
			{inbound ? $t('common.saveChanges') : $t('inbounds.addInbound')}
		</button>
	</div>
</form>

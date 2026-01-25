<script lang="ts">
	import type { Inbound } from '$lib/types';
	import { notifications } from '$lib/stores';

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
		if ((inbound as any)?.inet4_address) parts.push((inbound as any).inet4_address);
		if ((inbound as any)?.inet6_address) parts.push((inbound as any).inet6_address);
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

	const inboundTypes = [
		{ value: 'tun', label: 'TUN', description: 'System-level tunnel interface' },
		{ value: 'mixed', label: 'Mixed', description: 'HTTP + SOCKS5 proxy' },
		{ value: 'socks', label: 'SOCKS5', description: 'SOCKS5 proxy server' },
		{ value: 'http', label: 'HTTP', description: 'HTTP proxy server' }
	];

	const stackOptions: { value: 'system' | 'gvisor' | 'mixed'; label: string; description: string }[] = [
		{ value: 'system', label: 'System', description: 'Uses system network stack' },
		{ value: 'gvisor', label: 'gVisor', description: 'Userspace network stack' },
		{ value: 'mixed', label: 'Mixed', description: 'System for TCP, gVisor for UDP' }
	];

	function validate(): boolean {
		errors = {};

		if (!tag.trim()) {
			errors['tag'] = 'Tag is required';
		}

		if (type === 'tun') {
			if (!interfaceName.trim()) {
				errors['interfaceName'] = 'Interface name is required';
			}
			if (!addresses.trim()) {
				errors['addresses'] = 'At least one address is required';
			}
		} else {
			if (!listen.trim()) {
				errors['listen'] = 'Listen address is required';
			}
			if (listenPort < 1 || listenPort > 65535) {
				errors['port'] = 'Port must be between 1 and 65535';
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
		<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Tag *</label>
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
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Type</label>
		<div class="grid grid-cols-2 gap-2">
			{#each inboundTypes as t}
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

	<!-- TUN specific options -->
	{#if type === 'tun'}
		<div class="space-y-4">
			<div class="grid grid-cols-3 gap-4">
				<div class="col-span-2">
					<label for="interfaceName" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Interface Name *</label>
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
					<label for="mtu" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">MTU</label>
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
				<label for="addresses" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Addresses *</label>
				<input
					id="addresses"
					type="text"
					bind:value={addresses}
					placeholder="172.19.0.1/30, fd00::1/126"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['addresses'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">Comma-separated CIDR addresses (IPv4 and/or IPv6)</p>
				{#if errors['addresses']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['addresses']}</p>
				{/if}
			</div>

			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Stack</label>
				<div class="grid grid-cols-3 gap-2">
					{#each stackOptions as s}
						<button
							type="button"
							onclick={() => stack = s.value}
							class="type-btn text-center {stack === s.value ? 'selected' : ''}"
						>
							<div class="type-label text-sm">{s.label}</div>
							<div class="type-desc">{s.description}</div>
						</button>
					{/each}
				</div>
			</div>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Routing Options</h3>
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
					<input
						type="checkbox"
						bind:checked={autoRoute}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					Auto Route
					<span class="text-[var(--ctp-overlay0)]">— automatically configure system routes</span>
				</label>
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
					<input
						type="checkbox"
						bind:checked={strictRoute}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					Strict Route
					<span class="text-[var(--ctp-overlay0)]">— prevent traffic leaks</span>
				</label>
			</div>
		</div>
	{/if}

	<!-- Proxy inbound options (mixed, socks, http) -->
	{#if type !== 'tun'}
		<div class="space-y-4">
			<div class="grid grid-cols-3 gap-4">
				<div class="col-span-2">
					<label for="listen" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Listen Address *</label>
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
					<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Port *</label>
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
				TCP Fast Open
				<span class="text-[var(--ctp-overlay0)]">— reduces connection latency</span>
			</label>

			<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
				{#if type === 'mixed'}
					Mixed inbound accepts both HTTP and SOCKS5 connections on the same port.
				{:else if type === 'socks'}
					SOCKS5 proxy server. Configure applications to use this as their SOCKS5 proxy.
				{:else}
					HTTP proxy server. Configure applications to use this as their HTTP proxy.
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
			Cancel
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{inbound ? 'Save Changes' : 'Create Inbound'}
		</button>
	</div>
</form>

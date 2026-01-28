<script lang="ts">
	import type { MultiplexConfig } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';

	interface Props {
		server: string;
		serverPort: number;
		method: string;
		password: string;
		plugin: string;
		pluginOpts: string;
		network: string;
		udpOverTcp: boolean;
		multiplex: MultiplexConfig;
		errors?: Record<string, string>;
		onImport?: () => void;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		method = $bindable(),
		password = $bindable(),
		plugin = $bindable(),
		pluginOpts = $bindable(),
		network = $bindable(),
		udpOverTcp = $bindable(),
		multiplex = $bindable(),
		errors = {},
		onImport
	}: Props = $props();

	const methods = [
		'2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305',
		'aes-128-gcm', 'aes-192-gcm', 'aes-256-gcm',
		'chacha20-ietf-poly1305', 'xchacha20-ietf-poly1305', 'none'
	];

	const muxProtocols = ['smux', 'yamux', 'h2mux'];
</script>

<div class="space-y-4">
	<!-- Import Button -->
	{#if onImport}
		<button
			type="button"
			onclick={onImport}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromSs')}
		</button>
	{/if}

	<!-- Server & Port -->
	<ServerConfig bind:server bind:serverPort {errors} />

	<!-- Method & Password -->
	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="ss-method" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.method')} *
			</label>
			<select
				id="ss-method"
				bind:value={method}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['method'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			>
				{#each methods as m}
					<option value={m}>{m}</option>
				{/each}
			</select>
			{#if errors['method']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['method']}</p>
			{/if}
		</div>
		<div>
			<label for="ss-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.password')} *
			</label>
			<input
				id="ss-password"
				type="password"
				bind:value={password}
				placeholder="password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['password']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
			{/if}
		</div>
	</div>

	<!-- Plugin -->
	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="ss-plugin" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.plugin')}
			</label>
			<input
				id="ss-plugin"
				type="text"
				bind:value={plugin}
				placeholder="obfs-local"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
		<div>
			<label for="ss-plugin-opts" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.pluginOpts')}
			</label>
			<input
				id="ss-plugin-opts"
				type="text"
				bind:value={pluginOpts}
				placeholder="obfs=http;host=example.com"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
	</div>

	<!-- Network & UDP over TCP -->
	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="ss-network" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.networkType')}
			</label>
			<select
				id="ss-network"
				bind:value={network}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="">{$t('common.default')}</option>
				<option value="tcp">TCP</option>
				<option value="udp">UDP</option>
			</select>
		</div>
		<div class="flex items-end pb-2">
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={udpOverTcp}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('outbounds.udpOverTcp')}
			</label>
		</div>
	</div>

	<!-- Multiplexing -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<label class="flex items-center gap-2 text-sm font-medium text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={multiplex.enabled}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('outbounds.multiplexing')}
		</label>

		{#if multiplex.enabled}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="ss-mux-proto" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('outbounds.muxProtocol')}
					</label>
					<select
						id="ss-mux-proto"
						bind:value={multiplex.protocol}
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						{#each muxProtocols as proto}
							<option value={proto}>{proto}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="ss-mux-conns" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('outbounds.maxConnections')}
					</label>
					<input
						id="ss-mux-conns"
						type="number"
						bind:value={multiplex.max_connections}
						min="0"
						placeholder="4"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			</div>

			<div class="grid grid-cols-3 gap-4">
				<div>
					<label for="ss-mux-min" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('outbounds.minStreams')}
					</label>
					<input
						id="ss-mux-min"
						type="number"
						bind:value={multiplex.min_streams}
						min="0"
						placeholder="4"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
				<div>
					<label for="ss-mux-max" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('outbounds.maxStreams')}
					</label>
					<input
						id="ss-mux-max"
						type="number"
						bind:value={multiplex.max_streams}
						min="0"
						placeholder="0"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
				<div class="flex items-end pb-2">
					<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
						<input
							type="checkbox"
							bind:checked={multiplex.padding}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						{$t('outbounds.padding')}
					</label>
				</div>
			</div>
		{/if}
	</div>
</div>

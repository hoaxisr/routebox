<script lang="ts">
	import type { TLSConfig, ObfsConfig, DnsServer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';
	import DomainResolverField from './DomainResolverField.svelte';

	interface Props {
		server: string;
		serverPort: number;
		password: string;
		tls: TLSConfig;
		obfs: ObfsConfig | undefined;
		serverPorts: string;
		hopInterval: string;
		upMbps: number;
		downMbps: number;
		domainResolver: string;
		dnsServers: DnsServer[];
		hasDefaultResolver: boolean;
		errors?: Record<string, string>;
		onImport?: () => void;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		password = $bindable(),
		tls = $bindable(),
		obfs = $bindable(),
		serverPorts = $bindable(),
		hopInterval = $bindable(),
		upMbps = $bindable(),
		downMbps = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		errors = {},
		onImport
	}: Props = $props();

	const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'];

	// Initialize obfs if needed
	let obfsEnabled = $state(!!obfs?.type);
	let obfsType = $state(obfs?.type ?? '');
	let obfsPassword = $state(obfs?.password ?? '');

	$effect(() => {
		if (obfsEnabled && obfsType) {
			obfs = { type: obfsType as 'salamander', password: obfsPassword };
		} else {
			obfs = undefined;
		}
	});
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
			{$t('outbounds.importFromHy2')}
		</button>
	{/if}

	<!-- Server & Port -->
	<ServerConfig bind:server bind:serverPort {errors} />

	<!-- Domain Resolver -->
	<DomainResolverField
		bind:value={domainResolver}
		serverAddress={server}
		{dnsServers}
		{hasDefaultResolver}
	/>

	<!-- Password -->
	<div>
		<label for="hy2-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.password')} *
		</label>
		<input
			id="hy2-password"
			type="password"
			bind:value={password}
			placeholder="password"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['password']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
		{/if}
	</div>

	<!-- TLS Settings -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">TLS</h3>

		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="hy2-sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.sni')}
				</label>
				<input
					id="hy2-sni"
					type="text"
					bind:value={tls.server_name}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
			<div>
				<label for="hy2-fp" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.fingerprint')}
				</label>
				<select
					id="hy2-fp"
					bind:value={tls.utls!.fingerprint}
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each fingerprints as fp}
						<option value={fp}>{fp}</option>
					{/each}
				</select>
			</div>
		</div>

		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={tls.insecure}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('outbounds.skipCertVerification')}
		</label>
	</div>

	<!-- Port Hopping -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.portHopping')}</h3>

		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="hy2-ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.serverPorts')}
				</label>
				<input
					id="hy2-ports"
					type="text"
					bind:value={serverPorts}
					placeholder="1000-2000,3000-4000"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.portRangesHint')}</p>
			</div>
			<div>
				<label for="hy2-hop" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.hopInterval')}
				</label>
				<input
					id="hy2-hop"
					type="text"
					bind:value={hopInterval}
					placeholder="30s"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
		</div>
	</div>

	<!-- Bandwidth Limits -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.bandwidthLimits')}</h3>

		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="hy2-up" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.uploadMbps')}
				</label>
				<input
					id="hy2-up"
					type="number"
					bind:value={upMbps}
					min="0"
					placeholder="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
			<div>
				<label for="hy2-down" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.downloadMbps')}
				</label>
				<input
					id="hy2-down"
					type="number"
					bind:value={downMbps}
					min="0"
					placeholder="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
		</div>
	</div>

	<!-- Obfuscation -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<label class="flex items-center gap-2 text-sm font-medium text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={obfsEnabled}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('outbounds.obfuscationType')}
		</label>

		{#if obfsEnabled}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="hy2-obfs-type" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('common.type')}
					</label>
					<select
						id="hy2-obfs-type"
						bind:value={obfsType}
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="salamander">salamander</option>
					</select>
				</div>
				<div>
					<label for="hy2-obfs-pw" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('outbounds.obfuscationPassword')}
					</label>
					<input
						id="hy2-obfs-pw"
						type="password"
						bind:value={obfsPassword}
						placeholder="password"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>
			</div>
		{/if}
	</div>
</div>

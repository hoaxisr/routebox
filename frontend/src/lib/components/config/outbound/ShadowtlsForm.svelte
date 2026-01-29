<script lang="ts">
	import type { TLSConfig, DnsServer, Outbound } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';
	import DomainResolverField from './DomainResolverField.svelte';

	interface Props {
		server: string;
		serverPort: number;
		version: number;
		password: string;
		tls: TLSConfig;
		detour: string;
		domainResolver: string;
		dnsServers: DnsServer[];
		hasDefaultResolver: boolean;
		outbounds: Outbound[];
		currentTag?: string;
		errors?: Record<string, string>;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		version = $bindable(),
		password = $bindable(),
		tls = $bindable(),
		detour = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		outbounds = [],
		currentTag = '',
		errors = {}
	}: Props = $props();

	const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'];
	const versions = [1, 2, 3];

	// Filter out current outbound from detour options
	let detourOptions = $derived(outbounds.filter(o => o.tag !== currentTag));
</script>

<div class="space-y-4">
	<!-- Server & Port -->
	<ServerConfig bind:server bind:serverPort {errors} />

	<!-- Domain Resolver -->
	<DomainResolverField
		bind:value={domainResolver}
		serverAddress={server}
		{dnsServers}
		{hasDefaultResolver}
	/>

	<!-- Version -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('outbounds.version')}</label>
		<div class="grid grid-cols-3 gap-2">
			{#each versions as v}
				<button
					type="button"
					onclick={() => version = v}
					class="toggle-btn text-sm {version === v ? 'selected' : ''}"
				>
					v{v}
				</button>
			{/each}
		</div>
	</div>

	<!-- Password (v2/v3) -->
	{#if version >= 2}
		<div>
			<label for="stls-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.password')} *
			</label>
			<input
				id="stls-password"
				type="password"
				bind:value={password}
				placeholder="Password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['password']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
			{/if}
		</div>
	{/if}

	<!-- TLS Settings -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="stls-sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.sni')}
			</label>
			<input
				id="stls-sni"
				type="text"
				bind:value={tls.server_name}
				placeholder="example.com"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
		<div>
			<label for="stls-fingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.fingerprint')}
			</label>
			<select
				id="stls-fingerprint"
				bind:value={tls.utls!.fingerprint}
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			>
				<option value="">{$t('common.none')}</option>
				{#each fingerprints as fp}
					<option value={fp}>{fp}</option>
				{/each}
			</select>
		</div>
	</div>

	<!-- Insecure -->
	<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
		<input
			type="checkbox"
			bind:checked={tls.insecure}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
		/>
		{$t('outbounds.skipCertVerification')}
	</label>

	<!-- Detour -->
	<div>
		<label for="stls-detour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.detour')}
		</label>
		<select
			id="stls-detour"
			bind:value={detour}
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		>
			<option value="">{$t('common.none')}</option>
			{#each detourOptions as ob}
				<option value={ob.tag}>{ob.tag}</option>
			{/each}
		</select>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.detourHint')}</p>
	</div>

	<!-- Chain hint -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 text-sm text-[var(--ctp-overlay1)]">
		{$t('outbounds.shadowtlsChainHint')}
	</div>
</div>

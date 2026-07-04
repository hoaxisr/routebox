<script lang="ts">
	import type { DnsServer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';
	import DomainResolverField from './DomainResolverField.svelte';

	interface Props {
		server: string;
		serverPort: number;
		username: string;
		password: string;
		sni: string;
		caCert: string;
		insecureConcurrency: number;
		extraHeaders: string;
		quic: boolean;
		quicCongestionControl: string;
		udpOverTcp: boolean;
		domainResolver: string;
		dnsServers: DnsServer[];
		hasDefaultResolver: boolean;
		errors?: Record<string, string>;
		onImport?: () => void;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		username = $bindable(),
		password = $bindable(),
		sni = $bindable(),
		caCert = $bindable(),
		insecureConcurrency = $bindable(),
		extraHeaders = $bindable(),
		quic = $bindable(),
		quicCongestionControl = $bindable(),
		udpOverTcp = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		errors = {},
		onImport
	}: Props = $props();

	const congestionOptions = [
		{ value: '', label: 'Default' },
		{ value: 'bbr', label: 'BBR' },
		{ value: 'bbr2', label: 'BBRv2' },
		{ value: 'cubic', label: 'CUBIC' },
		{ value: 'reno', label: 'Reno' }
	];
</script>

<div class="space-y-4">
	{#if onImport}
		<button
			type="button"
			onclick={onImport}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromNaive')}
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

	<!-- Credentials -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="nv-username" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.username')}
			</label>
			<input
				id="nv-username"
				type="text"
				bind:value={username}
				placeholder="user"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['username'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['username']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['username']}</p>
			{/if}
		</div>
		<div>
			<label for="nv-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.password')}
			</label>
			<input
				id="nv-password"
				type="password"
				bind:value={password}
				placeholder="Password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['password']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
			{/if}
		</div>
	</div>

	<!-- SNI -->
	<div>
		<label for="nv-sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.sni')}
		</label>
		<input
			id="nv-sni"
			type="text"
			bind:value={sni}
			placeholder="example.com"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		/>
	</div>

	<!-- CA Certificate -->
	<div>
		<label for="nv-cacert" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.caCertificate')}
		</label>
		<textarea
			id="nv-cacert"
			bind:value={caCert}
			rows="3"
			placeholder="-----BEGIN CERTIFICATE-----"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-xs resize-y"
		></textarea>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.caCertificateHint')}</p>
	</div>

	<!-- Advanced -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">
			{$t('common.advanced')} ({$t('common.optional').toLowerCase()})
		</h3>

		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<label for="nv-concurrency" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
					{$t('outbounds.insecureConcurrency')}
				</label>
				<input
					id="nv-concurrency"
					type="number"
					bind:value={insecureConcurrency}
					min="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
				/>
			</div>
			<div>
				<label for="nv-quic-cc" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
					{$t('outbounds.quicCongestionControl')}
				</label>
				<select
					id="nv-quic-cc"
					bind:value={quicCongestionControl}
					disabled={!quic}
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm disabled:opacity-50"
				>
					{#each congestionOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
		</div>

		<div>
			<label for="nv-headers" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
				{$t('outbounds.extraHeaders')}
			</label>
			<textarea
				id="nv-headers"
				bind:value={extraHeaders}
				rows="2"
				placeholder="X-Custom=value"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-xs resize-y"
			></textarea>
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.extraHeadersHint')}</p>
		</div>

		<div class="flex flex-wrap gap-4">
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={quic}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('outbounds.enableQuic')}
			</label>
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
</div>

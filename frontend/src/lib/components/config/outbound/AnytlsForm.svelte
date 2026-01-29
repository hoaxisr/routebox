<script lang="ts">
	import type { TLSConfig, DnsServer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';
	import DomainResolverField from './DomainResolverField.svelte';

	interface Props {
		server: string;
		serverPort: number;
		password: string;
		tls: TLSConfig;
		idleCheckInterval: string;
		idleTimeout: string;
		minIdleSession: number;
		domainResolver: string;
		dnsServers: DnsServer[];
		hasDefaultResolver: boolean;
		errors?: Record<string, string>;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		password = $bindable(),
		tls = $bindable(),
		idleCheckInterval = $bindable(),
		idleTimeout = $bindable(),
		minIdleSession = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		errors = {}
	}: Props = $props();

	const fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'];
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

	<!-- Password -->
	<div>
		<label for="at-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.password')} *
		</label>
		<input
			id="at-password"
			type="password"
			bind:value={password}
			placeholder="Password"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['password']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
		{/if}
	</div>

	<!-- TLS Settings -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="at-sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.sni')}
			</label>
			<input
				id="at-sni"
				type="text"
				bind:value={tls.server_name}
				placeholder="example.com"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
		<div>
			<label for="at-fingerprint" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.fingerprint')}
			</label>
			<select
				id="at-fingerprint"
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

	<!-- Idle Session Settings -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">
			{$t('common.advanced')} ({$t('common.optional').toLowerCase()})
		</h3>
		<div class="grid grid-cols-3 gap-4">
			<div>
				<label for="at-idle-check" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
					{$t('outbounds.idleSessionCheckInterval')}
				</label>
				<input
					id="at-idle-check"
					type="text"
					bind:value={idleCheckInterval}
					placeholder="30s"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
				/>
			</div>
			<div>
				<label for="at-idle-timeout" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
					{$t('outbounds.idleSessionTimeout')}
				</label>
				<input
					id="at-idle-timeout"
					type="text"
					bind:value={idleTimeout}
					placeholder="30s"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
				/>
			</div>
			<div>
				<label for="at-min-idle" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
					{$t('outbounds.minIdleSession')}
				</label>
				<input
					id="at-min-idle"
					type="number"
					bind:value={minIdleSession}
					min="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
				/>
			</div>
		</div>
	</div>
</div>

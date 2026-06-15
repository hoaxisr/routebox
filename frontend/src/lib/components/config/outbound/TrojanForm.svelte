<script lang="ts">
	import type { TLSConfig, DnsServer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import ServerConfig from './ServerConfig.svelte';
	import TlsConfig from './TlsConfig.svelte';
	import DomainResolverField from './DomainResolverField.svelte';

	interface TrojanTransport {
		type: 'tcp' | 'ws' | 'grpc' | 'httpupgrade';
		path?: string;
		host?: string;
		service_name?: string;
	}

	interface Props {
		server: string;
		serverPort: number;
		password: string;
		tls: TLSConfig;
		transport: TrojanTransport;
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
		transport = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		errors = {},
		onImport
	}: Props = $props();

	const transports: TrojanTransport['type'][] = ['tcp', 'ws', 'grpc', 'httpupgrade'];
</script>

<div class="space-y-4">
	{#if onImport}
		<button type="button" onclick={onImport}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2">
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromTrojan')}
		</button>
	{/if}

	<ServerConfig bind:server bind:serverPort {errors} />

	<DomainResolverField bind:value={domainResolver} serverAddress={server} {dnsServers} {hasDefaultResolver} />

	<div>
		<label for="trojan-pw" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.password')} *</label>
		<input id="trojan-pw" type="text" bind:value={password} placeholder="password"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
		{#if errors['password']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
		{/if}
	</div>

	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-4">TLS</h3>
		<TlsConfig bind:tls showReality={true} />
	</div>

	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.transport')}</h3>
		<div class="flex gap-2">
			{#each transports as tr}
				<button type="button" onclick={() => transport.type = tr}
					class="px-3 py-1 rounded-lg text-sm transition-colors {transport.type === tr ? 'bg-[var(--ctp-primary)] text-white' : 'bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)]'}">
					{tr.toUpperCase()}
				</button>
			{/each}
		</div>

		{#if transport.type === 'ws' || transport.type === 'httpupgrade'}
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<label for="trj-path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.path')}</label>
					<input id="trj-path" type="text" bind:value={transport.path} placeholder="/"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
				<div>
					<label for="trj-host" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.host')}</label>
					<input id="trj-host" type="text" bind:value={transport.host} placeholder="example.com"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
			</div>
		{/if}

		{#if transport.type === 'grpc'}
			<div>
				<label for="trj-sn" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('outbounds.serviceName')}</label>
				<input id="trj-sn" type="text" bind:value={transport.service_name} placeholder="grpc-service"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		{/if}
	</div>
</div>

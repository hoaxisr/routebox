<script lang="ts">
	import { t } from 'svelte-i18n';
	import { featureFlags } from '$lib/stores';
	import type { TlsFragment, TlsRecordFragment } from '$lib/types';
	import TlsFragmentForm from '../../TlsFragmentForm.svelte';

	interface Props {
		overrideAddress: string;
		overridePort: number | undefined;
		networkStrategy: string;
		udpConnect: boolean;
		udpTimeout: string;
		udpDisableDomainUnmapping: boolean;
		tlsFragment: TlsFragment;
		tlsRecordFragment: TlsRecordFragment;
		showDescription?: boolean;
		collapsible?: boolean;
	}

	let {
		overrideAddress = $bindable(),
		overridePort = $bindable(),
		networkStrategy = $bindable(),
		udpConnect = $bindable(),
		udpTimeout = $bindable(),
		udpDisableDomainUnmapping = $bindable(),
		tlsFragment = $bindable(),
		tlsRecordFragment = $bindable(),
		showDescription = false,
		collapsible = false
	}: Props = $props();

	let isOpen = $state(!collapsible);
</script>

{#if collapsible}
	<details class="group" bind:open={isOpen}>
		<summary class="cursor-pointer text-sm font-medium text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] flex items-center gap-1">
			<svg class="w-4 h-4 transition-transform {isOpen ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
			</svg>
			{$t('routes.routeOptionsTitle')}
		</summary>
		<div class="mt-3 bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
			{@render content()}
		</div>
	</details>
{:else}
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
		{#if showDescription}
			<p class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.routeOptionsDesc')}</p>
		{/if}
		{@render content()}
	</div>
{/if}

{#snippet content()}
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="override-address" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('routes.overrideAddress')}
			</label>
			<input id="override-address" type="text" bind:value={overrideAddress} placeholder="1.1.1.1"
				class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="override-port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('routes.overridePort')}
			</label>
			<input id="override-port" type="number" min="1" max="65535" bind:value={overridePort} placeholder="443"
				class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
	</div>

	{#if $featureFlags['network_strategy']}
		<div>
			<label for="network-strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('routes.defaultNetworkStrategy')}
			</label>
			<select id="network-strategy" bind:value={networkStrategy}
				class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				<option value="">{$t('common.default')}</option>
				<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
				<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
				<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
				<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
			</select>
		</div>
	{/if}

	<details class="group">
		<summary class="cursor-pointer text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]">
			{$t('routes.udpOptions')}
		</summary>
		<div class="mt-3 space-y-3 pl-4 border-l-2 border-[var(--ctp-surface2)]">
			<label class="flex items-center gap-2 cursor-pointer">
				<input type="checkbox" bind:checked={udpConnect}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
				<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpConnect')}</span>
			</label>
			<div>
				<label for="udp-timeout" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('routes.udpTimeout')}
				</label>
				<input id="udp-timeout" type="text" bind:value={udpTimeout} placeholder="5m"
					class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<label class="flex items-center gap-2 cursor-pointer">
				<input type="checkbox" bind:checked={udpDisableDomainUnmapping}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
				<span class="text-sm text-[var(--ctp-text)]">{$t('routes.udpDisableDomainUnmapping')}</span>
			</label>
		</div>
	</details>

	{#if $featureFlags['tls_fragment']}
		<TlsFragmentForm bind:tlsFragment bind:tlsRecordFragment />
	{/if}
{/snippet}

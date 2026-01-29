<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { DnsServer } from '$lib/types';

	interface Props {
		server: string;
		strategy: string;
		dnsServers: DnsServer[];
	}

	let {
		server = $bindable(),
		strategy = $bindable(),
		dnsServers
	}: Props = $props();
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3">
	<div>
		<label for="resolve-server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.resolveServer')}
		</label>
		<select id="resolve-server" bind:value={server}
			class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
			<option value="">{$t('common.default')}</option>
			{#each dnsServers as srv}
				<option value={srv.tag}>{srv.tag} ({srv.type})</option>
			{/each}
		</select>
	</div>
	<div>
		<label for="resolve-strategy" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.resolveStrategy')}
		</label>
		<select id="resolve-strategy" bind:value={strategy}
			class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
			<option value="">{$t('common.default')}</option>
			<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
			<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
			<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
			<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
		</select>
	</div>
</div>

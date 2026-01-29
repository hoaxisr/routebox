<script lang="ts">
	import { t } from 'svelte-i18n';
	import { featureFlags } from '$lib/stores';
	import { isDomain } from '$lib/utils/validators';
	import type { DnsServer } from '$lib/types';

	interface Props {
		/** Current value of domain_resolver */
		value: string;
		/** Server address to check if it's a domain */
		serverAddress: string;
		/** List of available DNS servers */
		dnsServers: DnsServer[];
		/** Whether default_domain_resolver is set in route settings */
		hasDefaultResolver: boolean;
		/** Custom hint text (optional) */
		hint?: string;
	}

	let {
		value = $bindable(),
		serverAddress,
		dnsServers = [],
		hasDefaultResolver = false,
		hint
	}: Props = $props();

	// Show field only when:
	// 1. Feature flag is enabled (sing-box 1.12+)
	// 2. Server is a domain (not IP)
	// 3. No default_domain_resolver is set
	let shouldShow = $derived(
		$featureFlags['domain_resolver'] &&
		isDomain(serverAddress) &&
		!hasDefaultResolver
	);
</script>

{#if shouldShow}
	<div>
		<label for="domain_resolver" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.domainResolver')}
		</label>
		<select
			id="domain_resolver"
			bind:value
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		>
			<option value="">{$t('common.none')}</option>
			{#each dnsServers as server}
				<option value={server.tag}>{server.tag}</option>
			{/each}
		</select>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{hint || $t('outbounds.domainResolverHint')}
		</p>
	</div>
{/if}

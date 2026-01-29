<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Outbound } from '$lib/types';

	interface Props {
		outbound: string;
		outbounds: Outbound[];
		error?: string;
	}

	let {
		outbound = $bindable(),
		outbounds,
		error
	}: Props = $props();
</script>

<div>
	<label for="outbound" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
		{$t('routes.routeToOutbound')} *
	</label>
	<select
		id="outbound"
		bind:value={outbound}
		class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {error ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
	>
		<option value="">{$t('routes.selectOutbound')}</option>
		{#each outbounds as ob}
			<option value={ob.tag}>{ob.tag} ({ob.type})</option>
		{/each}
	</select>
	{#if error}
		<p class="mt-1 text-sm text-[var(--ctp-red)]">{error}</p>
	{/if}
</div>

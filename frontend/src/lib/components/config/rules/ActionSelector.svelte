<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RouteRule } from '$lib/types';

	type Action = RouteRule['action'];

	interface Props {
		action: Action;
	}

	let { action = $bindable() }: Props = $props();
</script>

<div>
	<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.ruleAction')}</label>
	<div class="flex flex-wrap gap-2">
		<button type="button" onclick={() => action = 'route'}
			class="toggle-btn {action === 'route' ? 'selected' : ''}">
			{$t('routes.actionRoute')}
		</button>
		<button type="button" onclick={() => action = 'reject'}
			class="toggle-btn-danger {action === 'reject' ? 'selected' : ''}">
			{$t('routes.actionReject')}
		</button>
		<button type="button" onclick={() => action = 'resolve'}
			class="toggle-btn {action === 'resolve' ? 'selected' : ''}">
			{$t('routes.actionResolve')}
		</button>
		<button type="button" onclick={() => action = 'sniff'}
			class="toggle-btn {action === 'sniff' ? 'selected' : ''}">
			{$t('routes.actionSniff')}
		</button>
		<button type="button" onclick={() => action = 'hijack-dns'}
			class="toggle-btn {action === 'hijack-dns' ? 'selected' : ''}">
			{$t('routes.actionHijackDns')}
		</button>
	</div>
	<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
		{#if action === 'route'}{$t('routes.actionRouteDesc')}
		{:else if action === 'reject'}{$t('routes.actionRejectDesc')}
		{:else if action === 'route-options'}{$t('routes.actionRouteOptionsDesc')}
		{:else if action === 'resolve'}{$t('routes.actionResolveDesc')}
		{:else if action === 'sniff'}{$t('routes.actionSniffDesc')}
		{:else if action === 'hijack-dns'}{$t('routes.actionHijackDnsDesc')}
		{/if}
	</p>
</div>

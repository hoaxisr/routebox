<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ServerTransportState, TransportType } from '$lib/utils/serverInbound';

	interface Props {
		transport: ServerTransportState;
	}
	let { transport = $bindable() }: Props = $props();

	const types: TransportType[] = ['raw', 'ws', 'grpc', 'httpupgrade', 'xhttp'];
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
	<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('inbounds.server.transport')}</h3>

	<div class="flex flex-wrap gap-2">
		{#each types as ty}
			<button type="button" onclick={() => (transport.type = ty)}
				class="toggle-btn {transport.type === ty ? 'selected' : ''}">
				{$t(`inbounds.server.transports.${ty}`)}
			</button>
		{/each}
	</div>

	{#if transport.type === 'ws' || transport.type === 'httpupgrade' || transport.type === 'xhttp'}
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.transportPath')}</label>
				<input type="text" bind:value={transport.path} placeholder="/"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.transportHost')}</label>
				<input type="text" bind:value={transport.host} placeholder="cdn.example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		</div>
	{/if}

	{#if transport.type === 'grpc'}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.transportServiceName')}</label>
			<input type="text" bind:value={transport.service_name} placeholder="grpc-service"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
	{/if}
</div>

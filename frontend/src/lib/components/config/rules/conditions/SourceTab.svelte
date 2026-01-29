<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Inbound } from '$lib/types';

	interface Props {
		sourceIpIsPrivate: boolean;
		inbounds: Inbound[];
		selectedInbounds: string[];
		sourceIpCidr: string;
		sourcePorts: string;
		sourcePortRange: string;
		authUser: string;
	}

	let {
		sourceIpIsPrivate = $bindable(),
		inbounds,
		selectedInbounds = $bindable(),
		sourceIpCidr = $bindable(),
		sourcePorts = $bindable(),
		sourcePortRange = $bindable(),
		authUser = $bindable()
	}: Props = $props();

	function toggleInbound(tag: string) {
		if (selectedInbounds.includes(tag)) {
			selectedInbounds = selectedInbounds.filter((t) => t !== tag);
		} else {
			selectedInbounds = [...selectedInbounds, tag];
		}
	}
</script>

<div class="space-y-4">
	<!-- Source IP is Private -->
	<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
		<input type="checkbox" bind:checked={sourceIpIsPrivate}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
		<div>
			<span class="text-sm text-[var(--ctp-text)]">{$t('routes.sourceIpIsPrivate')}</span>
			<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.sourceIpIsPrivateHint')}</p>
		</div>
	</label>

	<!-- Inbound Filter -->
	{#if inbounds.length > 0}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('routes.matchFromInbound')}</label>
			<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] divide-y divide-[var(--ctp-surface2)]">
				{#each inbounds as ib}
					<button type="button" onclick={() => toggleInbound(ib.tag)}
						class="w-full px-4 py-3 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors text-left {selectedInbounds.includes(ib.tag) ? 'bg-[var(--ctp-surface1)]' : ''}">
						<div class="flex items-center gap-2">
							<span class="font-medium text-[var(--ctp-text)]">{ib.tag}</span>
							<span class="selection-chip">{ib.type}</span>
						</div>
						{#if selectedInbounds.includes(ib.tag)}
							<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
							</svg>
						{/if}
					</button>
				{/each}
			</div>
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.matchFromInboundHint')}</p>
		</div>
	{/if}

	<!-- Source IP -->
	<div>
		<label for="source-ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.sourceIpCidr')}
			<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.clientAddress')})</span>
		</label>
		<textarea id="source-ip-cidr" bind:value={sourceIpCidr} rows={2}
			placeholder="192.168.1.100&#10;10.0.0.0/24"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		></textarea>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.sourceIpCidrHint')}</p>
	</div>

	<!-- Source Ports -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="source-ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePort')}</label>
			<input id="source-ports" type="text" bind:value={sourcePorts} placeholder="1024, 8080"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="source-port-range" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.sourcePortRange')}</label>
			<input id="source-port-range" type="text" bind:value={sourcePortRange} placeholder="1000:2000"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
	</div>

	<!-- Auth User -->
	<div>
		<label for="auth-user" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.authUser')}
			<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
		</label>
		<textarea id="auth-user" bind:value={authUser} rows={2} placeholder="admin&#10;user1"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		></textarea>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.authUserHint')}</p>
	</div>
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import { isWildcardListen } from '$lib/utils/inboundAddress';

	// Shared Listen Address + Listen Port row for every server inbound form.
	// When the listen value is a wildcard bind ("::" etc.) users read it as "the
	// address clients dial" and try to put the VPS IP there (#37) — so under the
	// field we spell out the actual connect target from server.public_host.
	interface Props {
		listen: string;
		listenPort: number;
		errors?: Record<string, string>;
		/** server.public_host from settings; '' hides the connect hint. */
		publicHost?: string;
		/** mieru may bind only ranges, leaving the single port empty. */
		portRequired?: boolean;
	}
	let {
		listen = $bindable(),
		listenPort = $bindable(),
		errors = {},
		publicHost = '',
		portRequired = true
	}: Props = $props();

	// Bracket a bare IPv6 literal so host:port stays unambiguous.
	let connectHost = $derived(
		publicHost.includes(':') && !publicHost.startsWith('[') ? `[${publicHost}]` : publicHost
	);
	// Port 0/empty (mieru ranges-only) → hint shows just the host.
	let connectTarget = $derived(listenPort > 0 ? `${connectHost}:${listenPort}` : connectHost);
</script>

<div class="grid grid-cols-3 gap-4">
	<div class="col-span-2">
		<label for="listen" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenAddress')} *</label>
		<input id="listen" type="text" bind:value={listen} placeholder="::"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		{#if publicHost && isWildcardListen(listen)}
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
				{$t('inbounds.server.clientsConnect', { values: { address: connectTarget } })}
			</p>
		{/if}
	</div>
	<div>
		<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenPort')}{portRequired ? ' *' : ''}</label>
		<input id="listenPort" type="number" bind:value={listenPort} min={portRequired ? 1 : 0} max="65535"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['port'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
		{#if errors['port']}<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['port']}</p>{/if}
	</div>
</div>

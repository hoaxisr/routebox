<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerTlsConfig from './ServerTlsConfig.svelte';
	import ServerUsers from './ServerUsers.svelte';
	import ListenAddressFields from './ListenAddressFields.svelte';
	import { BBR_PROFILES, hy2CongestionSummary, type ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
		errors?: Record<string, string>;
		publicHost?: string;
	}
	let { state = $bindable(), errors = {}, publicHost = '' }: Props = $props();

	// The fork knows two hysteria2 obfs types, so the type is an
	// Off/Salamander/Gecko toggle (#48). A stored value that is none of them
	// keeps the original free-text input instead — silently rewriting a
	// hand-edited config on the next save would be the wrong kind of helpful.
	const OBFS_TYPES = ['salamander', 'gecko'];
	let legacyObfsType = $derived(state.obfsType !== '' && !OBFS_TYPES.includes(state.obfsType));

	// Congestion control (#59). Nothing here is a mode switch — the rates ARE the
	// switch; this only names what the combination does. The table lives in
	// serverInbound.ts so it can be pinned against sing-quic's own branches.
	let ccSummary = $derived(hy2CongestionSummary(state.ignoreClientBandwidth, state.downMbps));
	// One of the three states locks clients out rather than merely shaping them,
	// and the panel's own share links are exactly the ones it locks out — that
	// cannot read as ordinary grey hint text.
	let ccLocksOut = $derived(ccSummary === 'ccBrutalOnly');
</script>

<div class="space-y-4">
	<ListenAddressFields bind:listen={state.listen} bind:listenPort={state.listenPort} {errors} {publicHost} />

	<div class="grid grid-cols-2 gap-4">
		<div>
			<label for="upMbps" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.upMbps')}</label>
			<input id="upMbps" type="number" bind:value={state.upMbps} min="0"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="downMbps" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.downMbps')}</label>
			<input id="downMbps" type="number" bind:value={state.downMbps} min="0"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
	</div>
	<p class="-mt-2 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.bandwidthHint')}</p>

	<div>
		<label class="flex items-center gap-2 text-sm font-medium text-[var(--ctp-subtext1)]">
			<input type="checkbox" bind:checked={state.ignoreClientBandwidth}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
			{$t('inbounds.server.ignoreClientBandwidth')}
		</label>
		<p class="mt-1 text-xs {ccLocksOut ? 'text-[var(--ctp-yellow)]' : 'text-[var(--ctp-subtext0)]'}">
			{$t(`inbounds.server.${ccSummary}`)}
		</p>
	</div>

	<div>
		<span class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.bbrProfile')}</span>
		<div class="flex gap-2 flex-wrap" role="group" aria-label={$t('inbounds.server.bbrProfile')}>
			<button type="button" class="toggle-btn {state.bbrProfile === '' ? 'selected' : ''}"
				onclick={() => (state.bbrProfile = '')}>{$t('common.default')}</button>
			{#each BBR_PROFILES as p (p)}
				<button type="button" class="toggle-btn {state.bbrProfile === p ? 'selected' : ''}"
					onclick={() => (state.bbrProfile = p)}>{$t(`inbounds.server.bbr.${p}`)}</button>
			{/each}
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.bbrProfileHint')}</p>
	</div>
	<div class="grid grid-cols-2 gap-4">
		<div>
			{#if legacyObfsType}
				<label for="obfsType" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsType')}</label>
				<input id="obfsType" type="text" bind:value={state.obfsType}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			{:else}
				<span class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsType')}</span>
				<div class="flex gap-2" role="group" aria-label={$t('inbounds.server.obfsType')}>
					<button type="button" class="toggle-btn {state.obfsType === '' ? 'selected' : ''}"
						onclick={() => (state.obfsType = '')}>{$t('inbounds.server.obfsOff')}</button>
					<button type="button" class="toggle-btn {state.obfsType === 'salamander' ? 'selected' : ''}"
						onclick={() => (state.obfsType = 'salamander')}>Salamander</button>
					<button type="button" class="toggle-btn {state.obfsType === 'gecko' ? 'selected' : ''}"
						onclick={() => (state.obfsType = 'gecko')}>Gecko</button>
				</div>
			{/if}
		</div>
		{#if state.obfsType !== ''}
			<div>
				<label for="obfsPw" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsPassword')} *</label>
				<input id="obfsPw" type="text" bind:value={state.obfsPassword}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['obfsPassword'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
				{#if errors['obfsPassword']}<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['obfsPassword']}</p>{/if}
			</div>
		{/if}
	</div>

	{#if state.obfsType === 'gecko'}
		<div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="obfsMinPkt" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsMinPacketSize')}</label>
					<input id="obfsMinPkt" type="number" min="0" bind:value={state.obfsMinPacketSize}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['obfsPacketSize'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
				</div>
				<div>
					<label for="obfsMaxPkt" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsMaxPacketSize')}</label>
					<input id="obfsMaxPkt" type="number" min="0" bind:value={state.obfsMaxPacketSize}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['obfsPacketSize'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
				</div>
			</div>
			{#if errors['obfsPacketSize']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['obfsPacketSize']}</p>
			{:else}
				<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.obfsPacketSizeHint')}</p>
			{/if}
		</div>
	{/if}

	<ServerTlsConfig
		bind:tlsMode={state.tlsMode}
		bind:serverName={state.tls.server_name}
		bind:acmeDomain={state.tls.acme.domain}
		bind:acmeEmail={state.tls.acme.email}
		bind:realityPrivateKey={state.tls.reality.private_key}
		bind:realityShortId={state.tls.reality.short_id}
		bind:handshakeServer={state.handshakeServer}
		bind:handshakePort={state.handshakePort}
		bind:certificatePath={state.tls.certificate_path}
		bind:keyPath={state.tls.key_path}
		allowReality={false}
		{errors}
	/>

	<ServerUsers bind:users={state.users} protocol="hysteria2" error={errors['userCred'] ?? errors['users']} />
</div>

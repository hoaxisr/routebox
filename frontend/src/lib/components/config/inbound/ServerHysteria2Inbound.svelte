<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerTlsConfig from './ServerTlsConfig.svelte';
	import ServerUsers from './ServerUsers.svelte';
	import ListenAddressFields from './ListenAddressFields.svelte';
	import type { ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
		errors?: Record<string, string>;
		publicHost?: string;
	}
	let { state = $bindable(), errors = {}, publicHost = '' }: Props = $props();

	// sing-box knows exactly one hysteria2 obfs type ("salamander"), so the type
	// is an Off/Salamander toggle (#48). A stored value that is neither keeps the
	// original free-text input instead — silently rewriting a hand-edited config
	// to '' or 'salamander' on the next save would be the wrong kind of helpful.
	let legacyObfsType = $derived(state.obfsType !== '' && state.obfsType !== 'salamander');
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

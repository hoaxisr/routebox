<script lang="ts">
	import ServerTlsConfig from './ServerTlsConfig.svelte';
	import ServerUsers from './ServerUsers.svelte';
	import ListenAddressFields from './ListenAddressFields.svelte';
	import { NAIVE_QUIC_CC } from '$lib/utils/serverInbound';
	import type { ServerFormState } from '$lib/utils/serverInbound';
	import { t } from 'svelte-i18n';

	interface Props {
		state: ServerFormState;
		errors?: Record<string, string>;
		publicHost?: string;
	}
	let { state = $bindable(), errors = {}, publicHost = '' }: Props = $props();
</script>

<div class="space-y-4">
	<ListenAddressFields bind:listen={state.listen} bind:listenPort={state.listenPort} {errors} {publicHost} />

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

	<ServerUsers bind:users={state.users} protocol="naive" error={errors['userCred'] ?? errors['users']} />

	<div>
		<span class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.quicCC')}</span>
		<div class="flex gap-2 flex-wrap" role="group" aria-label={$t('inbounds.server.quicCC')}>
			<button type="button" class="toggle-btn {state.naiveQuicCC === '' ? 'selected' : ''}"
				onclick={() => (state.naiveQuicCC = '')}>{$t('common.default')}</button>
			{#each NAIVE_QUIC_CC as cc (cc)}
				<button type="button" class="toggle-btn {state.naiveQuicCC === cc ? 'selected' : ''}"
					onclick={() => (state.naiveQuicCC = cc)}>{$t(`inbounds.server.quicCCName.${cc}`)}</button>
			{/each}
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.quicCCHint')}</p>
	</div>
</div>

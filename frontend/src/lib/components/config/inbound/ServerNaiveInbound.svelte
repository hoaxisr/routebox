<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerTlsConfig from './ServerTlsConfig.svelte';
	import ServerUsers from './ServerUsers.svelte';
	import type { ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
	}
	let { state = $bindable() }: Props = $props();
</script>

<div class="space-y-4">
	<div class="grid grid-cols-3 gap-4">
		<div class="col-span-2">
			<label for="listen" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenAddress')} *</label>
			<input id="listen" type="text" bind:value={state.listen} placeholder="::"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenPort')} *</label>
			<input id="listenPort" type="number" bind:value={state.listenPort} min="1" max="65535"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
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
	/>

	<ServerUsers bind:users={state.users} protocol="naive" />
</div>

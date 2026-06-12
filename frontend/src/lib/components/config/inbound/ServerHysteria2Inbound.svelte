<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerTlsConfig from './ServerTlsConfig.svelte';
	import ServerUsers from './ServerUsers.svelte';
	import type { ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
		onShare?: (index: number) => void;
		canShare: boolean;
	}
	let { state = $bindable(), onShare, canShare }: Props = $props();
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
			<label for="obfsType" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsType')}</label>
			<input id="obfsType" type="text" bind:value={state.obfsType} placeholder="salamander"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="obfsPw" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.obfsPassword')}</label>
			<input id="obfsPw" type="text" bind:value={state.obfsPassword}
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

	<ServerUsers bind:users={state.users} protocol="hysteria2" {onShare} {canShare} />
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import { notifications } from '$lib/stores';
	import { api } from '$lib/api/client';
	import type { TlsMode } from '$lib/utils/serverInbound';

	interface Props {
		tlsMode: TlsMode;
		serverName: string;
		acmeDomain: string;
		acmeEmail: string;
		realityPrivateKey: string;
		realityShortId: string;
		handshakeServer: string;
		handshakePort: number;
		certificatePath: string;
		keyPath: string;
		allowReality: boolean; // only vless
	}

	let {
		tlsMode = $bindable(),
		serverName = $bindable(),
		acmeDomain = $bindable(),
		acmeEmail = $bindable(),
		realityPrivateKey = $bindable(),
		realityShortId = $bindable(),
		handshakeServer = $bindable(),
		handshakePort = $bindable(),
		certificatePath = $bindable(),
		keyPath = $bindable(),
		allowReality
	}: Props = $props();

	let realityPublicKey = $state('');
	let generating = $state(false);

	const modes: { id: TlsMode; reality?: boolean }[] = [
		{ id: 'acme' },
		{ id: 'reality', reality: true },
		{ id: 'manual' }
	];

	async function generateKeypair() {
		generating = true;
		try {
			const res = await api.generateReality();
			realityPrivateKey = res.private_key;
			realityPublicKey = res.public_key;
			if (!realityShortId) {
				const pw = await api.generatePassword();
				realityShortId = pw.password.slice(0, 8).toLowerCase().replace(/[^0-9a-f]/g, '0');
			}
		} catch {
			notifications.error($t('inbounds.server.realityGenFailed'));
		} finally {
			generating = false;
		}
	}
</script>

<div class="space-y-4">
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('inbounds.server.tlsMode')}</label>
		<div class="grid grid-cols-3 gap-2">
			{#each modes as m}
				{#if !m.reality || allowReality}
					<button type="button" onclick={() => (tlsMode = m.id)}
						class="type-btn text-center {tlsMode === m.id ? 'selected' : ''}">
						<div class="type-label text-sm">{$t(`inbounds.server.tlsModes.${m.id}`)}</div>
					</button>
				{/if}
			{/each}
		</div>
	</div>

	{#if tlsMode === 'acme'}
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="acmeDomain" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.domain')} *</label>
				<input id="acmeDomain" type="text" bind:value={acmeDomain} placeholder="vpn.example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="acmeEmail" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.email')} *</label>
				<input id="acmeEmail" type="email" bind:value={acmeEmail} placeholder="admin@example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		</div>
	{:else if tlsMode === 'reality'}
		<div class="space-y-3">
			<div>
				<label for="rServerName" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.handshakeServer')} *</label>
				<input id="rServerName" type="text" bind:value={serverName} placeholder="www.microsoft.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('inbounds.server.handshakeHint')}</p>
			</div>
			<div class="grid grid-cols-3 gap-2">
				<div class="col-span-2">
					<label for="hsServer" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.handshakeDial')}</label>
					<input id="hsServer" type="text" bind:value={handshakeServer} placeholder={serverName || 'www.microsoft.com'}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('inbounds.server.handshakeDialHint')}</p>
				</div>
				<div>
					<label for="hsPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.handshakePort')}</label>
					<input id="hsPort" type="number" bind:value={handshakePort} min="1" max="65535"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
			</div>
			<div class="flex items-end gap-2">
				<div class="flex-1">
					<label for="rShortId" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.shortId')}</label>
					<input id="rShortId" type="text" bind:value={realityShortId} placeholder="0123abcd"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</div>
				<button type="button" onclick={generateKeypair} disabled={generating}
					class="px-3 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50">
					{generating ? $t('common.loading') : $t('inbounds.server.generateKeypair')}
				</button>
			</div>
			{#if realityPrivateKey}
				<div class="bg-[var(--ctp-surface0)] rounded-lg p-3 text-xs space-y-1 break-all">
					<div><span class="text-[var(--ctp-overlay0)]">private_key:</span> <span class="text-[var(--ctp-text)]">{realityPrivateKey}</span></div>
					{#if realityPublicKey}
						<div><span class="text-[var(--ctp-overlay0)]">public_key:</span> <span class="text-[var(--ctp-text)]">{realityPublicKey}</span></div>
						<p class="text-[var(--ctp-overlay0)]">{$t('inbounds.server.publicKeyHint')}</p>
					{/if}
				</div>
			{/if}
		</div>
	{:else}
		<div class="space-y-3">
			<div>
				<label for="certPath" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.certificatePath')} *</label>
				<input id="certPath" type="text" bind:value={certificatePath} placeholder="/etc/ssl/fullchain.pem"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="keyPath" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.keyPath')} *</label>
				<input id="keyPath" type="text" bind:value={keyPath} placeholder="/etc/ssl/privkey.pem"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		</div>
	{/if}
</div>

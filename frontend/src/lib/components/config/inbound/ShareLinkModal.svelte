<script lang="ts">
	import { t } from 'svelte-i18n';
	import QRCode from 'qrcode';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';

	interface Props {
		tag: string;
		userIndex: number;
		onClose: () => void;
	}
	let { tag, userIndex, onClose }: Props = $props();

	const HOST_KEY = 'routebox:serverPublicHost';
	let host = $state(
		typeof localStorage !== 'undefined' ? (localStorage.getItem(HOST_KEY) ?? '') : ''
	);
	let link = $state('');
	let qrDataUrl = $state('');
	let loading = $state(false);

	async function generate() {
		if (!host.trim()) {
			notifications.error($t('inbounds.server.hostRequired'));
			return;
		}
		loading = true;
		try {
			localStorage.setItem(HOST_KEY, host.trim());
			const res = await api.getUserLink(tag, userIndex, host.trim());
			link = res.link;
			qrDataUrl = await QRCode.toDataURL(link, { width: 256, margin: 1 });
		} catch (e) {
			notifications.error(e instanceof Error ? e.message : $t('inbounds.server.linkFailed'));
			link = '';
			qrDataUrl = '';
		} finally {
			loading = false;
		}
	}

	async function copyLink() {
		await navigator.clipboard.writeText(link);
		notifications.success($t('common.copied'));
	}
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
	<div class="bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-xl p-6 w-full max-w-md space-y-4">
		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('inbounds.server.clientLink')}</h2>
			<button
				type="button"
				onclick={onClose}
				class="p-1 hover:bg-[var(--ctp-surface1)] rounded transition-colors"
				aria-label="Close modal"
			>
				<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>

		<div class="flex items-end gap-2">
			<div class="flex-1">
				<label for="shareHost" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.publicHost')}</label>
				<input id="shareHost" type="text" bind:value={host} placeholder="vpn.example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<button type="button" onclick={generate} disabled={loading}
				class="px-3 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50">
				{loading ? $t('common.loading') : $t('common.generate')}
			</button>
		</div>

		{#if link}
			<div class="flex justify-center">
				<img src={qrDataUrl} alt="QR code for client link" class="rounded-lg bg-white p-2" width="256" height="256" />
			</div>
			<div class="bg-[var(--ctp-surface0)] rounded-lg p-2 text-xs font-mono break-all text-[var(--ctp-text)]">{link}</div>
			<button type="button" onclick={copyLink}
				class="w-full px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
				{$t('common.copy')}
			</button>
		{/if}

		<div class="flex justify-end">
			<button type="button" onclick={onClose}
				class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
				{$t('common.close')}
			</button>
		</div>
	</div>
</div>

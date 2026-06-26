<script lang="ts">
	import { t } from 'svelte-i18n';
	import QRCode from 'qrcode';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { PanelUser } from '$lib/types';
	import { effectiveSubUrl } from './subscription-url';
	import { pillLabel, defaultBinding, loadConnectionLink } from './share-links';

	interface Props {
		user: PanelUser;
		publicHost: string; // server.public_host ("" if unset)
		publicPort?: number; // server.public_port (0/undefined = none/443)
		onClose: () => void;
		onChanged: () => void; // parent reloads the user list
	}
	let { user, publicHost, publicPort, onClose, onChanged }: Props = $props();

	let token = $state(user.token ?? '');
	let disabled = $state(user.token_disabled === true);
	let qrDataUrl = $state('');
	let busy = $state(false);
	let mode = $state<'sub' | 'single'>('sub');

	let selectedTag = $state('');
	let link = $state('');
	let linkError = $state('');
	let singleQr = $state('');
	let loadingLink = $state(false);
	let loadSeq = 0; // request-sequencing token for loadLink (not reactive)

	const subUrl = $derived(
		effectiveSubUrl({ token, token_disabled: disabled }, publicHost, publicPort)
	);

	async function loadLink(tag: string) {
		const seq = ++loadSeq;
		selectedTag = tag;
		linkError = '';
		link = '';
		singleQr = '';
		if (!publicHost) return; // banner handles this; never call the API without a host
		loadingLink = true;
		const res = await loadConnectionLink(
			api.getUserLink,
			user.id,
			tag,
			publicHost,
			$t('users.linkBuildError')
		);
		if (seq !== loadSeq) return; // a newer pill selection superseded this request
		link = res.link;
		linkError = res.error;
		if (res.link) singleQr = await QRCode.toDataURL(res.link, { width: 256, margin: 1 });
		if (seq !== loadSeq) return; // re-check after the QR await
		loadingLink = false;
	}

	// Auto-select the first protocol the first time Single mode is opened. The
	// `!selectedTag` guard is load-bearing: loadLink() sets selectedTag synchronously
	// before its first await, so the effect re-runs as a no-op and never loops. Do NOT
	// reset selectedTag on mode-switch or this will re-fetch on every toggle.
	$effect(() => {
		if (mode === 'single' && !selectedTag) {
			const b = defaultBinding(user.bindings);
			if (b) void loadLink(b.inbound_tag);
		}
	});

	async function renderQr() {
		if (!subUrl) {
			qrDataUrl = '';
			return;
		}
		try {
			qrDataUrl = await QRCode.toDataURL(subUrl, { width: 256, margin: 1 });
		} catch {
			qrDataUrl = '';
		}
	}

	$effect(() => {
		void subUrl; // re-render QR whenever the URL changes (token rotation); covers initial mount
		renderQr();
	});

	async function copy(text: string) {
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(text);
			} else {
				const ta = document.createElement('textarea');
				ta.value = text;
				ta.style.position = 'fixed';
				ta.style.opacity = '0';
				document.body.appendChild(ta);
				ta.select();
				const ok = document.execCommand('copy');
				document.body.removeChild(ta);
				if (!ok) throw new Error('copy failed');
			}
			notifications.success($t('common.copied'));
		} catch {
			notifications.error($t('common.copyFailed'));
		}
	}

	async function rotate() {
		if (!confirm($t('users.rotateConfirm'))) return;
		busy = true;
		try {
			const res = await api.rotateUserToken(user.id);
			token = res.token;
			disabled = false; // rotate clears the sticky-revoke flag server-side
			notifications.success($t('users.rotated'));
			onChanged();
		} catch (e) {
			notifications.error(e instanceof Error ? e.message : $t('users.rotateFailed'));
		} finally {
			busy = false;
		}
	}

	async function revoke() {
		if (!confirm($t('users.revokeConfirm'))) return;
		busy = true;
		try {
			await api.revokeUserToken(user.id);
			token = '';
			disabled = true; // sticky: stays off until a rotate re-issues a token
			notifications.success($t('users.revoked'));
			onChanged();
		} catch (e) {
			notifications.error(e instanceof Error ? e.message : $t('users.revokeFailed'));
		} finally {
			busy = false;
		}
	}
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
	<div class="bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-xl p-6 w-full max-w-md space-y-4">
		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('users.share')}</h2>
			<button type="button" onclick={onClose} aria-label="Close modal"
				class="p-1 hover:bg-[var(--ctp-surface1)] rounded transition-colors">
				<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>

		<div class="flex gap-1 p-1 bg-[var(--ctp-surface0)] rounded-lg">
			<button type="button" onclick={() => (mode = 'sub')}
				class="flex-1 px-3 py-1.5 rounded-md text-sm transition-colors {mode === 'sub' ? 'bg-[var(--ctp-primary)] text-white' : 'text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)]'}">
				{$t('users.subscription')}
			</button>
			<button type="button" onclick={() => (mode = 'single')}
				class="flex-1 px-3 py-1.5 rounded-md text-sm transition-colors {mode === 'single' ? 'bg-[var(--ctp-primary)] text-white' : 'text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)]'}">
				{$t('users.singleConnection')}
			</button>
		</div>

		{#if mode === 'sub'}
		{#if !publicHost}
			<!-- no public_host configured -->
			<div class="text-sm text-[var(--ctp-overlay1)]">
				{$t('users.noPublicHost')}
				<a href="/config/settings" class="text-[var(--ctp-primary)] underline ml-1">{$t('users.openSettings')}</a>
			</div>
		{:else if disabled}
			<!-- revoked (sticky): no URL; offer rotate to re-enable -->
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('users.revokedState')}</div>
			<div class="flex justify-end">
				<button type="button" onclick={rotate} disabled={busy}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50">
					{$t('users.rotateReenable')}
				</button>
			</div>
		{:else if !token}
			<!-- pending: not yet materialized -->
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('users.noToken')}</div>
		{:else}
			<!-- active token: URL + QR + copy + rotate + revoke -->
			<p class="text-xs text-[var(--ctp-overlay1)]">{$t('users.subscriptionHint')}</p>
			{#if qrDataUrl}
				<div class="flex justify-center">
					<img src={qrDataUrl} alt="Subscription QR" class="rounded-lg bg-white p-2" width="256" height="256" />
				</div>
			{/if}
			<label class="block text-xs font-medium text-[var(--ctp-overlay1)]">{$t('users.subscriptionUrl')}</label>
			<div class="bg-[var(--ctp-surface0)] rounded-lg p-2 text-xs font-mono break-all text-[var(--ctp-text)]">{subUrl}</div>
			<div class="flex gap-2">
				<button type="button" onclick={() => copy(subUrl)}
					class="flex-1 px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
					{$t('common.copy')}
				</button>
				<button type="button" onclick={rotate} disabled={busy}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50">
					{$t('users.rotate')}
				</button>
				<button type="button" onclick={revoke} disabled={busy}
					class="px-4 py-2 text-[var(--ctp-red)] border border-[var(--ctp-red)]/40 rounded-lg hover:bg-[var(--ctp-red)]/10 disabled:opacity-50">
					{$t('users.revoke')}
				</button>
			</div>
		{/if}
		{:else}
			{#if !publicHost}
				<div class="text-sm text-[var(--ctp-overlay1)]">
					{$t('users.noPublicHost')}
					<a href="/config/settings" class="text-[var(--ctp-primary)] underline ml-1">{$t('users.openSettings')}</a>
				</div>
			{:else if user.bindings.length === 0}
				<div class="text-sm text-[var(--ctp-overlay1)]">{$t('users.noConnections')}</div>
			{:else}
				<label class="block text-xs font-medium text-[var(--ctp-overlay1)]">{$t('users.selectProtocol')}</label>
				<div class="flex flex-wrap gap-2">
					{#each user.bindings as b (b.inbound_tag)}
						<button type="button" onclick={() => loadLink(b.inbound_tag)}
							class="px-3 py-1.5 rounded-lg text-sm border transition-colors {selectedTag === b.inbound_tag ? 'bg-[var(--ctp-primary)] text-white border-[var(--ctp-primary)]' : 'text-[var(--ctp-text)] border-[var(--ctp-surface2)] hover:bg-[var(--ctp-surface0)]'}">
							{pillLabel(b)}
						</button>
					{/each}
				</div>

				{#if linkError}
					<div class="text-sm text-[var(--ctp-red)]">{linkError}</div>
				{:else if loadingLink}
					<div class="text-sm text-[var(--ctp-overlay1)]">{$t('common.loading')}</div>
				{:else if link}
					{#if singleQr}
						<div class="flex justify-center">
							<img src={singleQr} alt="Connection QR" class="rounded-lg bg-white p-2" width="256" height="256" />
						</div>
					{/if}
					<label class="block text-xs font-medium text-[var(--ctp-overlay1)]">{$t('users.connectionLink')}</label>
					<div class="bg-[var(--ctp-surface0)] rounded-lg p-2 text-xs font-mono break-all text-[var(--ctp-text)]">{link}</div>
					<button type="button" onclick={() => copy(link)}
						class="w-full px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
						{$t('common.copy')}
					</button>
				{/if}
			{/if}
		{/if}

		<div class="flex justify-end">
			<button type="button" onclick={onClose}
				class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
				{$t('common.close')}
			</button>
		</div>
	</div>
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import QRCode from 'qrcode';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import type { MtprotoClient } from '$lib/types';
	import { copyText } from '$lib/utils/clipboard';
	import { unixToDateInput, dateInputToUnix, presetExpiry } from '$lib/components/awg/peerExpiry';
	import { clientStatus, relExpiry } from './roster';

	interface Props {
		clients: MtprotoClient[];
		/** False while a masking domain or public host is missing — the link and
		 *  QR actions stay dead rather than handing out something that fails
		 *  silently inside Telegram. */
		canShare: boolean;
		readOnly?: boolean;
		onChange: () => void | Promise<void>;
	}

	let { clients, canShare, readOnly = false, onChange }: Props = $props();

	let newName = $state('');
	let adding = $state(false);

	let renaming = $state<string | null>(null); // client whose expiry row is open
	let renewDate = $state('');
	let savingExpiry = $state(false);
	let dateEl = $state<HTMLInputElement | null>(null);

	// Share modal state. The secret only ever lives here, for as long as the
	// modal is open — it is fetched per share action, never with the roster.
	let shareName = $state<string | null>(null);
	let shareTg = $state('');
	let shareWeb = $state('');
	let qrDataUrl = $state('');

	function nowSec(): number {
		return Math.floor(Date.now() / 1000);
	}

	function openDatePicker() {
		if (dateEl?.showPicker) dateEl.showPicker();
		else dateEl?.focus();
	}

	function openRenew(c: MtprotoClient) {
		renaming = c.name;
		renewDate = unixToDateInput(c.expires_at);
	}

	function applyPreset(days: number) {
		renewDate = unixToDateInput(presetExpiry(days, nowSec()));
	}

	async function saveExpiry(c: MtprotoClient) {
		savingExpiry = true;
		try {
			await api.updateMtprotoClient(c.name, { expires_at: dateInputToUnix(renewDate) });
			renaming = null;
			await onChange();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		} finally {
			savingExpiry = false;
		}
	}

	async function addClient() {
		const name = newName.trim();
		if (!name) return;
		adding = true;
		try {
			await api.createMtprotoClient(name);
			notifications.success($t('telegram.added'));
			newName = '';
			await onChange();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		} finally {
			adding = false;
		}
	}

	async function toggle(c: MtprotoClient) {
		try {
			await api.updateMtprotoClient(c.name, { enabled: !c.enabled });
			notifications.success(c.enabled ? $t('telegram.disabledClient') : $t('telegram.enabledClient'));
			await onChange();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		}
	}

	async function rotate(c: MtprotoClient) {
		// Confirmed, because it revokes a link somebody is using right now.
		if (!confirm($t('telegram.rotateConfirm', { values: { name: c.name } }))) return;
		try {
			await api.rotateMtprotoClient(c.name);
			notifications.success($t('telegram.rotated'));
			await onChange();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		}
	}

	async function remove(c: MtprotoClient) {
		if (!confirm($t('telegram.deleteConfirm', { values: { name: c.name } }))) return;
		try {
			await api.deleteMtprotoClient(c.name);
			notifications.success($t('telegram.deleted'));
			await onChange();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		}
	}

	async function share(c: MtprotoClient, withQr: boolean) {
		try {
			const link = await api.getMtprotoClientLink(c.name);
			shareTg = link.tg;
			shareWeb = link.web;
			// The tg:// form is what a phone should scan: it opens Telegram
			// directly instead of a browser tab that then redirects.
			qrDataUrl = withQr
				? await QRCode.toDataURL(link.tg, { width: 512, margin: 2, errorCorrectionLevel: 'M' })
				: '';
			shareName = c.name;
		} catch (e) {
			notifications.error(`${$t('telegram.loadFailed')}: ${e}`);
		}
	}

	function closeShare() {
		shareName = null;
		shareTg = '';
		shareWeb = '';
		qrDataUrl = '';
	}

	async function copy(value: string) {
		if (await copyText(value)) notifications.success($t('telegram.copied'));
		else notifications.error($t('common.copyFailed'));
	}
</script>

<div class="add-row">
	<input
		bind:value={newName}
		type="text"
		maxlength="64"
		disabled={readOnly}
		placeholder={$t('telegram.newClientPlaceholder')}
		onkeydown={(e) => e.key === 'Enter' && addClient()}
	/>
	<button type="button" class="btn-add-client" onclick={addClient} disabled={adding || readOnly || !newName.trim()}>
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
		{adding ? $t('telegram.adding') : $t('telegram.addClient')}
	</button>
</div>

{#if clients.length === 0}
	<div class="empty-state">
		<strong>{$t('telegram.emptyTitle')}</strong>
		{$t('telegram.emptyDesc')}
	</div>
{:else}
	<div class="peer-list">
		{#each clients as c (c.name)}
			{@const st = clientStatus(c, nowSec())}
			<div class="peer-row" class:dimmed={st === 'disabled' || st === 'expired'}>
				<span class="icon-badge">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><path d="M21.5 4.5 2.5 11.2l5.6 1.9m13.4-8.6-3 14.6-6-4.3m9-10.3-10.9 9.6m0 0-.5 5 3.4-3.4" /></svg>
				</span>
				<div class="peer-info">
					<div class="peer-name">
						<span class="conn-dot" class:on={st === 'online'} title={st === 'online' ? $t('telegram.online') : $t('telegram.offline')}></span>
						{c.name}
						{#if st === 'expired'}
							<span class="susp-badge">{$t('telegram.expiredLabel')}</span>
						{:else if st === 'disabled'}
							<span class="susp-badge muted">{$t('telegram.disabled')}</span>
						{/if}
					</div>
					<div class="peer-meta">
						{#if !c.expires_at}
							<span class="exp">{$t('telegram.noExpiry')}</span>
						{:else if st === 'expired'}
							<span class="exp">{$t('telegram.expiredLabel')}</span>
						{:else}
							<span class="exp">{$t('telegram.expiresIn', { values: { rel: relExpiry(c.expires_at, nowSec()) } })}</span>
						{/if}
					</div>
				</div>
				<div class="peer-actions">
					<button type="button" class="peer-btn" disabled={readOnly} onclick={() => openRenew(c)}>
						{c.expires_at ? $t('telegram.renew') : $t('telegram.setExpiry')}
					</button>
					<button type="button" class="peer-btn primary" disabled={!canShare} title={canShare ? '' : $t('telegram.linkBlocked')} onclick={() => share(c, true)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><line x1="14" y1="14" x2="14" y2="21" /><line x1="21" y1="14" x2="21" y2="21" /><line x1="14" y1="17.5" x2="21" y2="17.5" /></svg>
						{$t('telegram.qr')}
					</button>
					<button type="button" class="peer-btn" disabled={!canShare} title={canShare ? '' : $t('telegram.linkBlocked')} onclick={() => share(c, false)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" /><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" /></svg>
						{$t('telegram.share')}
					</button>
					<button type="button" class="peer-btn" disabled={readOnly} onclick={() => toggle(c)}>
						{c.enabled ? $t('telegram.disable') : $t('telegram.enable')}
					</button>
					<button type="button" class="peer-btn" disabled={readOnly} onclick={() => rotate(c)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><polyline points="23 4 23 10 17 10" /><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" /></svg>
						{$t('telegram.rotate')}
					</button>
					<button type="button" class="action-btn-danger" disabled={readOnly} title={$t('telegram.delete')} aria-label={$t('telegram.delete')} onclick={() => remove(c)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16"><polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /></svg>
					</button>
				</div>
			</div>
			{#if renaming === c.name}
				<div class="renew-row">
					<input type="date" bind:value={renewDate} bind:this={dateEl} class="renew-date-native" tabindex="-1" aria-hidden="true" />
					<button type="button" class="renew-date-btn" onclick={openDatePicker}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
						{renewDate || $t('telegram.setDate')}
					</button>
					<button type="button" class="renew-preset" onclick={() => applyPreset(30)}>+30 days</button>
					<button type="button" class="renew-preset" onclick={() => applyPreset(90)}>+90 days</button>
					<button type="button" class="renew-preset" onclick={() => (renewDate = '')}>{$t('telegram.neverExpiry')}</button>
					<div class="renew-spacer"></div>
					<button type="button" class="peer-btn" onclick={() => (renaming = null)}>{$t('telegram.cancel')}</button>
					<button type="button" class="peer-btn primary" disabled={savingExpiry} onclick={() => saveExpiry(c)}>{$t('telegram.save')}</button>
				</div>
			{/if}
		{/each}
	</div>
{/if}

{#if shareName}
	<Modal open title={shareName} onClose={closeShare}>
		<div class="qr-modal">
			{#if qrDataUrl}
				<img class="qr-img" src={qrDataUrl} alt={shareName} />
			{/if}
			<div class="qr-text">
				<div class="qr-title">{$t('telegram.share')}</div>
				<div class="qr-hint">{$t('telegram.maskingDomainWarning')}</div>
			</div>
		</div>
		<div class="link-modal">
			<div class="link-hint">tg://proxy</div>
			<textarea class="link-text" rows="2" readonly value={shareTg}></textarea>
			<div class="link-hint">https://t.me/proxy</div>
			<textarea class="link-text" rows="2" readonly value={shareWeb}></textarea>
		</div>
		{#snippet footer()}
			<div class="modal-foot">
				<button type="button" class="btn-ghost-sm" onclick={() => copy(shareTg)}>{$t('telegram.copyLink')}</button>
				<button type="button" class="btn-ghost-sm" onclick={closeShare}>{$t('telegram.cancel')}</button>
			</div>
		{/snippet}
	</Modal>
{/if}

<style>
	.add-row {
		display: flex;
		gap: 0.6rem;
		margin-bottom: 1rem;
	}
	.add-row input {
		flex: 1;
		background: var(--ctp-base);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.375rem;
		padding: 0.55rem 0.7rem;
		color: var(--ctp-text);
		font-size: 0.9rem;
	}
	.add-row input:focus {
		outline: none;
		border-color: var(--ctp-primary);
	}
	.btn-add-client {
		padding: 0.55rem 1.1rem;
		border-radius: 0.5rem;
		border: none;
		background: var(--ctp-primary);
		color: #1a1a1a;
		font-weight: 600;
		font-size: 0.9rem;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
		white-space: nowrap;
		transition: background 0.15s ease;
	}
	.btn-add-client:hover {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-add-client:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.peer-list {
		display: flex;
		flex-direction: column;
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		overflow: hidden;
	}
	.peer-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.55rem 1rem;
		padding: 0.85rem 1rem;
		border-top: 1px solid var(--ctp-surface0);
		transition: background 0.12s ease;
	}
	.peer-row:first-child {
		border-top: none;
	}
	.peer-row:hover {
		background: var(--ctp-base);
	}
	.peer-row.dimmed {
		opacity: 0.6;
	}
	.icon-badge {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border-radius: 0.5rem;
		background: color-mix(in srgb, var(--ctp-primary) 12%, transparent);
		color: var(--ctp-primary);
		flex: none;
	}
	.peer-info {
		flex: 1 1 14rem;
		min-width: 0;
	}
	.peer-name {
		font-weight: 600;
		font-size: 0.9375rem;
		display: flex;
		align-items: center;
		gap: 0.45rem;
	}
	.conn-dot {
		width: 8px;
		height: 8px;
		border-radius: 9999px;
		background: var(--ctp-overlay0);
		flex: none;
	}
	.conn-dot.on {
		background: var(--ctp-green);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--ctp-green) 25%, transparent);
	}
	.peer-meta {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		margin-top: 2px;
		color: var(--ctp-overlay1);
		font-size: 0.8125rem;
		min-width: 0;
		overflow: hidden;
	}
	.peer-meta .exp {
		color: var(--ctp-overlay1);
	}
	.susp-badge {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--ctp-red);
		border: 1px solid color-mix(in srgb, var(--ctp-red) 40%, transparent);
		border-radius: 0.3rem;
		padding: 0.05rem 0.35rem;
	}
	/* Disabled is a deliberate admin action, expired is not — so it does not
	   get the red that means "something needs attention". */
	.susp-badge.muted {
		color: var(--ctp-overlay1);
		border-color: var(--ctp-surface2);
	}
	.renew-row {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.6rem 1rem;
		border-top: 1px dashed var(--ctp-surface2);
		background: var(--ctp-base);
		flex-wrap: wrap;
	}
	/* Native date input kept for its picker only; visually collapsed. */
	.renew-date-native {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		pointer-events: none;
	}
	.renew-date-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.375rem;
		padding: 0.35rem 0.6rem;
		color: var(--ctp-text);
		font-size: 0.85rem;
		cursor: pointer;
	}
	.renew-date-btn:hover {
		border-color: var(--ctp-primary);
	}
	.renew-preset {
		padding: 0.35rem 0.6rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface0);
		color: var(--ctp-subtext1);
		font-size: 0.8rem;
		cursor: pointer;
	}
	.renew-preset:hover {
		border-color: var(--ctp-primary);
		color: var(--ctp-primary);
	}
	.renew-spacer {
		flex: 1;
	}
	.peer-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.375rem;
		margin-left: auto;
	}
	.peer-btn {
		padding: 0.4rem 0.7rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface0);
		color: var(--ctp-subtext1);
		font-weight: 500;
		font-size: 0.8125rem;
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		cursor: pointer;
		transition: all 0.15s ease;
	}
	.peer-btn:hover:not(:disabled) {
		border-color: var(--ctp-primary);
		color: var(--ctp-primary);
	}
	.peer-btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.peer-btn.primary {
		border-color: var(--ctp-primary);
		background: color-mix(in srgb, var(--ctp-primary) 12%, transparent);
		color: var(--ctp-primary);
	}
	.peer-btn.primary:hover:not(:disabled) {
		background: color-mix(in srgb, var(--ctp-primary) 22%, transparent);
	}
	.action-btn-danger {
		padding: 0.4rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface0);
		color: var(--ctp-subtext1);
		display: inline-flex;
		align-items: center;
		cursor: pointer;
		transition: all 0.15s ease;
	}
	.action-btn-danger:hover:not(:disabled) {
		border-color: var(--ctp-red);
		color: var(--ctp-red);
	}
	.action-btn-danger:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.empty-state {
		border: 2px dashed var(--ctp-surface2);
		border-radius: 0.5rem;
		padding: 1.6rem;
		text-align: center;
		color: var(--ctp-overlay1);
		font-size: 0.85rem;
	}
	.empty-state strong {
		color: var(--ctp-subtext1);
		display: block;
		margin-bottom: 0.2rem;
	}
	.qr-modal {
		display: flex;
		gap: 1.25rem;
		align-items: flex-start;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}
	.qr-img {
		border-radius: 0.5rem;
		background: #fff;
		padding: 0.75rem;
		flex-shrink: 0;
		width: 100%;
		max-width: 320px;
		height: auto;
		image-rendering: pixelated;
	}
	.qr-text {
		flex: 1;
		min-width: 12rem;
	}
	.qr-title {
		font-weight: 600;
		margin-bottom: 0.25rem;
	}
	.qr-hint {
		color: var(--ctp-overlay1);
		font-size: 0.8125rem;
	}
	.link-modal {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.link-hint {
		font-size: 0.8rem;
		color: var(--ctp-overlay1);
	}
	.link-text {
		width: 100%;
		padding: 0.5rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-mantle);
		color: var(--ctp-text);
		font-family: ui-monospace, monospace;
		font-size: 0.75rem;
		word-break: break-all;
		resize: vertical;
	}
	.modal-foot {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.btn-ghost-sm {
		padding: 0.5rem 0.9rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface1);
		color: var(--ctp-text);
		font-weight: 500;
		cursor: pointer;
	}
	.btn-ghost-sm:hover {
		background: var(--ctp-surface2);
	}
	@media (max-width: 720px) {
		.peer-actions {
			flex-basis: 100%;
			margin-left: 0;
		}
	}
</style>

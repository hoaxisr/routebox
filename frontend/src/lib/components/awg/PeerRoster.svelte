<script lang="ts">
	import { t } from 'svelte-i18n';
	import QRCode from 'qrcode';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import type { AwgPeer } from '$lib/types';
	import { formatBytes } from '$lib/stores/settings';
	import { expiryStatus, unixToDateInput, dateInputToUnix, presetExpiry } from './peerExpiry';
	import { copyText } from '$lib/utils/clipboard';

	interface Props {
		peers: AwgPeer[];
		subnet?: string;
		singbox?: boolean;
		onChange: () => void | Promise<void>;
	}

	let { peers, subnet = '', singbox = false, onChange }: Props = $props();

	// A peer is "live" when the server says so — a real handshake either way,
	// off the kernel interface on that backend and off sing-box's WireGuard
	// device via its UAPI on the other (see backend/internal/awg/singbox_peers.go).
	const isLive = (p: AwgPeer) => p.online;

	let newName = $state('');
	let adding = $state(false);

	let renewing = $state<string | null>(null); // public_key of the peer whose renew row is open
	let renewDate = $state(''); // YYYY-MM-DD bound to the date input (empty = never)
	let savingExpiry = $state(false);
	let dateEl = $state<HTMLInputElement | null>(null); // only one renew-row is open at a time

	// Open the native date picker on click. showPicker() needs a user gesture (we have one);
	// fall back to focus for the odd browser without it. ponytail: native picker, no lib.
	function openDatePicker() {
		if (dateEl?.showPicker) dateEl.showPicker();
		else dateEl?.focus();
	}

	function nowSec(): number {
		return Math.floor(Date.now() / 1000);
	}

	// Short relative expiry phrase: "30d" when >=1 day out, else "5h".
	function relExpiry(expires: number, now: number): string {
		const s = Math.max(0, expires - now);
		if (s >= 86400) return `${Math.floor(s / 86400)}d`;
		return `${Math.max(1, Math.floor(s / 3600))}h`;
	}

	function openRenew(p: AwgPeer) {
		renewing = p.public_key;
		renewDate = unixToDateInput(p.expires_at);
	}
	function applyPreset(days: number) {
		renewDate = unixToDateInput(presetExpiry(days, nowSec()));
	}
	async function saveExpiry(p: AwgPeer) {
		savingExpiry = true;
		try {
			await api.setAwgPeerExpiry(p.public_key, dateInputToUnix(renewDate));
			notifications.success($t('awg.expirySaved'));
			renewing = null;
			await onChange();
		} catch (e) {
			notifications.error(`${$t('awg.expiryFailed')}: ${e}`);
		} finally {
			savingExpiry = false;
		}
	}

	// Every row of one fetch shares a provenance, so the note covers the list
	// rather than repeating per line. Keyed on the REASON, not on how degraded the
	// numbers are: "this build has no per-peer route" and "the proxy did not
	// answer" need opposite advice, and both can land on either kind.
	const degradedReason = $derived(
		peers.find((p) => p.stats && p.stats !== 'live')?.stats_reason ?? ''
	);
	const degradedNote = $derived(
		degradedReason === 'unsupported'
			? $t('awg.statsNoteUnsupported')
			: degradedReason === 'unreachable'
				? $t('awg.statsNoteUnreachable')
				: degradedReason === 'no_source'
					? $t('awg.statsNoteNoSource')
					: ''
	);

	// Why a row shows a dash instead of a figure (#75). Empty for the live path,
	// where every number on the row was actually measured.
	function statsHint(p: AwgPeer): string {
		if (!p.stats || p.stats === 'live') return '';
		if (p.stats_reason === 'unsupported') return $t('awg.statsApproximate');
		return $t('awg.statsUnavailable');
	}

	// "last seen" relative label from a unix-seconds handshake (0 = never).
	function lastSeen(ts: number): string {
		if (!ts) return $t('awg.never');
		const s = Math.max(0, Math.floor(Date.now() / 1000 - ts));
		if (s < 60) return $t('awg.secondsAgo', { values: { n: s } });
		if (s < 3600) return $t('awg.minutesAgo', { values: { n: Math.floor(s / 60) } });
		if (s < 86400) return $t('awg.hoursAgo', { values: { n: Math.floor(s / 3600) } });
		return $t('awg.daysAgo', { values: { n: Math.floor(s / 86400) } });
	}

	let qrPeer = $state<AwgPeer | null>(null);
	let qrDataUrl = $state('');

	let linkPeer = $state<AwgPeer | null>(null);
	let linkText = $state('');

	async function showLink(p: AwgPeer) {
		try {
			linkText = await api.getAwgPeerVpnLink(p.public_key);
			linkPeer = p;
		} catch (e) {
			notifications.error(`${$t('awg.vpnLinkFailed')}: ${e}`);
		}
	}

	async function copyLink() {
		if (await copyText(linkText)) {
			notifications.success($t('common.copied'));
		} else {
			notifications.error($t('common.copyFailed'));
		}
	}

	async function addPeer() {
		const name = newName.trim();
		if (!name) return;
		adding = true;
		try {
			await api.createAwgPeer(name);
			notifications.success($t('awg.clientAdded', { values: { name } }));
			newName = '';
			await onChange();
		} catch (e) {
			notifications.error(`${$t('awg.addFailed')}: ${e}`);
		} finally {
			adding = false;
		}
	}

	async function removePeer(p: AwgPeer) {
		if (!confirm($t('awg.deleteConfirm', { values: { name: p.name } }))) return;
		try {
			await api.deleteAwgPeer(p.public_key);
			notifications.success($t('awg.deleted', { values: { name: p.name } }));
			await onChange();
		} catch (e) {
			notifications.error(`${$t('awg.deleteFailed')}: ${e}`);
		}
	}

	async function showQR(p: AwgPeer) {
		try {
			const conf = await api.getAwgPeerConfig(p.public_key);
			// The config is large (~1.3KB with CPS mimicry), so use low error-correction
			// ('L') to keep the module count down and render big — otherwise the QR is too
			// dense to scan from a phone.
			qrDataUrl = await QRCode.toDataURL(conf, { width: 512, margin: 2, errorCorrectionLevel: 'L' });
			qrPeer = p;
		} catch (e) {
			notifications.error(`${$t('awg.configFailed')}: ${e}`);
		}
	}

	async function download(p: AwgPeer) {
		try {
			const conf = await api.getAwgPeerConfig(p.public_key);
			const blob = new Blob([conf], { type: 'text/plain' });
			const a = document.createElement('a');
			a.href = URL.createObjectURL(blob);
			a.download = `${p.name}.conf`;
			a.click();
			URL.revokeObjectURL(a.href);
		} catch (e) {
			notifications.error(`${$t('awg.configFailed')}: ${e}`);
		}
	}

	async function copyJson(p: AwgPeer) {
		try {
			const ep = await api.getAwgPeerSingbox(p.public_key);
			if (await copyText(JSON.stringify(ep, null, 2))) {
				notifications.success($t('awg.exportedJson'));
			} else {
				notifications.error($t('common.copyFailed'));
			}
		} catch (e) {
			notifications.error(`${$t('awg.configFailed')}: ${e}`);
		}
	}

	async function downloadJson(p: AwgPeer) {
		try {
			const ep = await api.getAwgPeerSingbox(p.public_key);
			const blob = new Blob([JSON.stringify(ep, null, 2)], { type: 'application/json' });
			const a = document.createElement('a');
			a.href = URL.createObjectURL(blob);
			a.download = `${p.name}.endpoint.json`;
			a.click();
			URL.revokeObjectURL(a.href);
		} catch (e) {
			notifications.error(`${$t('awg.configFailed')}: ${e}`);
		}
	}
</script>

<div class="add-row">
	<input
		bind:value={newName}
		type="text"
		maxlength="64"
		placeholder={$t('awg.newClientPlaceholder')}
		onkeydown={(e) => e.key === 'Enter' && addPeer()}
	/>
	<button type="button" class="btn-add-client" onclick={addPeer} disabled={adding || !newName.trim()}>
		<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
		{adding ? $t('awg.adding') : $t('awg.addClient')}
	</button>
</div>

<!-- One line for the whole roster: a dash per row says "not known", this says
     why, and for the common cause it names the fix (#75). -->
{#if degradedNote}
	<div class="stats-note">{degradedNote}</div>
{/if}

{#if peers.length === 0}
	<div class="empty-state">
		<strong>{$t('awg.emptyTitle')}</strong>
		{$t('awg.emptyDesc')}
	</div>
{:else}
	<div class="peer-list">
		{#each peers as p (p.public_key)}
			<div class="peer-row" class:dimmed={expiryStatus(p.expires_at, nowSec()) === 'suspended'}>
				<span class="icon-badge">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20"><rect x="5" y="2" width="14" height="20" rx="2" /><line x1="12" y1="18" x2="12.01" y2="18" /></svg>
				</span>
				<div class="peer-info">
					<div class="peer-name">
						<!-- The dot is an assertion about connectivity. When the state
						     could not be read at all it must not make one — grey there
						     would read as "offline", which is precisely the claim we
						     do not have (#75). -->
						<span
							class="conn-dot"
							class:on={isLive(p) && p.stats !== 'unavailable'}
							class:unknown={p.stats === 'unavailable'}
							title={p.stats === 'unavailable'
								? $t('awg.statsUnavailable')
								: isLive(p)
									? $t('awg.online')
									: $t('awg.offline')}
						></span>
						{p.name || '(unnamed)'}
						{#if expiryStatus(p.expires_at, nowSec()) === 'suspended'}
							<span class="susp-badge">{$t('awg.suspended')}</span>
						{/if}
					</div>
					<div class="peer-meta">
						<span class="addr">{p.address}</span>
						<span class="dot-sep">·</span>
						<span class="seen" title={statsHint(p)}>
							{#if p.stats === 'unavailable'}—{:else}{isLive(p) ? $t('awg.online') : lastSeen(p.last_handshake)}{/if}
						</span>
						<span class="dot-sep">·</span>
						<!-- Byte counts exist only on the live path. The fallback infers
						     liveness from recorded traffic and has no counters at all, so
						     printing 0 B there states a measurement nobody took (#75). -->
						<span class="xfer" title={statsHint(p)}>
							{#if p.stats && p.stats !== 'live'}—{:else}↓ {formatBytes(p.rx)} &nbsp;↑ {formatBytes(p.tx)}{/if}
						</span>
						{#if singbox}
							<span class="dot-sep">·</span>
							{#if !p.expires_at}
								<span class="exp">{$t('awg.noExpiry')}</span>
							{:else if expiryStatus(p.expires_at, nowSec()) === 'active'}
								<span class="exp">{$t('awg.expiresIn', { values: { rel: relExpiry(p.expires_at, nowSec()) } })}</span>
							{:else}
								<span class="exp">{$t('awg.expiredLabel')}</span>
							{/if}
						{:else if expiryStatus(p.expires_at, nowSec()) === 'active'}
							<span class="dot-sep">·</span>
							<span class="exp">{$t('awg.expires', { values: { date: unixToDateInput(p.expires_at) } })}</span>
						{/if}
					</div>
				</div>
				<div class="peer-actions">
					<button type="button" class="peer-btn {expiryStatus(p.expires_at, nowSec()) === 'suspended' ? 'primary' : ''}" onclick={() => openRenew(p)}>
						{expiryStatus(p.expires_at, nowSec()) === 'none' ? $t('awg.setExpiry') : $t('awg.renew')}
					</button>
					{#if singbox}
						<button type="button" class="peer-btn primary" onclick={() => showQR(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><line x1="14" y1="14" x2="14" y2="21" /><line x1="21" y1="14" x2="21" y2="21" /><line x1="14" y1="17.5" x2="21" y2="17.5" /></svg>
							{$t('awg.qr')}
						</button>
						<button type="button" class="peer-btn" onclick={() => download(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
							{$t('awg.conf')}
						</button>
						<button type="button" class="peer-btn" title={$t('awg.exportJson')} aria-label={$t('awg.exportJson')} onclick={() => copyJson(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>
							JSON
						</button>
						<button type="button" class="peer-btn" title={$t('awg.downloadJson')} aria-label={$t('awg.downloadJson')} onclick={() => downloadJson(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
							JSON
						</button>
					{:else}
						<button type="button" class="peer-btn primary" onclick={() => showQR(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><line x1="14" y1="14" x2="14" y2="21" /><line x1="21" y1="14" x2="21" y2="21" /><line x1="14" y1="17.5" x2="21" y2="17.5" /></svg>
							{$t('awg.qr')}
						</button>
						<button type="button" class="peer-btn" onclick={() => download(p)}>
							<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
							{$t('awg.conf')}
						</button>
					{/if}
					<button type="button" class="peer-btn" onclick={() => showLink(p)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" /><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" /></svg>
						{$t('awg.vpnLink')}
					</button>
					<button type="button" class="action-btn-danger" title={$t('awg.delete')} onclick={() => removePeer(p)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16"><polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /></svg>
					</button>
				</div>
			</div>
			{#if renewing === p.public_key}
				<div class="renew-row">
					<input type="date" bind:value={renewDate} bind:this={dateEl} class="renew-date-native" tabindex="-1" aria-hidden="true" />
					<button type="button" class="renew-date-btn" onclick={openDatePicker}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
						{renewDate || $t('awg.setDate')}
					</button>
					<button type="button" class="renew-preset" onclick={() => applyPreset(30)}>+30 days</button>
					<button type="button" class="renew-preset" onclick={() => applyPreset(90)}>+90 days</button>
					<button type="button" class="renew-preset" onclick={() => (renewDate = '')}>{$t('awg.neverExpiry')}</button>
					<span class="renew-spacer"></span>
					<button type="button" class="peer-btn primary" disabled={savingExpiry} onclick={() => saveExpiry(p)}>{$t('awg.saveExpiry')}</button>
					<button type="button" class="peer-btn" onclick={() => (renewing = null)}>{$t('awg.cancel')}</button>
				</div>
			{/if}
		{/each}
	</div>
{/if}

{#if subnet}
	<div class="roster-foot">{$t('awg.addressFoot', { values: { subnet } })}</div>
{/if}

{#if qrPeer}
	<Modal open={!!qrPeer} title={$t('awg.qrTitle', { values: { name: qrPeer.name } })} size="lg" onClose={() => (qrPeer = null)}>
		<div class="qr-modal">
			<img src={qrDataUrl} alt="AmneziaWG client config QR" class="qr-img" />
			<div class="qr-text">
				<div class="qr-title">{$t('awg.scanTitle')}</div>
				<div class="qr-hint">{$t('awg.scanHint')}</div>
			</div>
		</div>
		{#snippet footer()}
			<div class="modal-foot">
				{#if qrPeer}
					<button type="button" class="peer-btn primary" onclick={() => qrPeer && download(qrPeer)}>
						<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="15" height="15"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
						{$t('awg.downloadConf')}
					</button>
				{/if}
				<button type="button" class="btn-ghost-sm" onclick={() => (qrPeer = null)}>{$t('common.close')}</button>
			</div>
		{/snippet}
	</Modal>
{/if}

{#if linkPeer}
	<Modal open={!!linkPeer} title={$t('awg.vpnLinkTitle', { values: { name: linkPeer.name } })} size="md" onClose={() => (linkPeer = null)}>
		<div class="link-modal">
			<p class="link-hint">{$t('awg.vpnLinkHint')}</p>
			<textarea class="link-text" readonly rows="4" value={linkText}
				onfocus={(e) => (e.currentTarget as HTMLTextAreaElement).select()}></textarea>
			<button type="button" class="peer-btn primary" onclick={copyLink}>{$t('common.copy')}</button>
		</div>
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
	.conn-dot.unknown {
		background: transparent;
		box-shadow: inset 0 0 0 1.5px var(--ctp-overlay0);
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
	.peer-meta .dot-sep {
		color: var(--ctp-overlay0);
	}
	.peer-meta .seen {
		color: var(--ctp-overlay1);
	}
	.peer-meta .xfer {
		font-family: inherit;
		color: var(--ctp-overlay1);
		font-size: 0.78rem;
	}
	.peer-meta {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		margin-top: 2px;
		color: var(--ctp-overlay1);
		font-size: 0.8125rem;
		/* Clip the address/expiry line here (not on .peer-info) so the LED's glow,
		   which sits at the left edge of .peer-name, is never cut off (#34). */
		min-width: 0;
		overflow: hidden;
	}
	.peer-meta .addr {
		font-family: inherit;
		color: var(--ctp-subtext0);
	}
	.susp-badge {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--ctp-red);
		border: 1px solid color-mix(in srgb, var(--ctp-red) 40%, transparent);
		border-radius: 0.3rem;
		padding: 0.05rem 0.35rem;
	}
	.peer-meta .exp {
		color: var(--ctp-overlay1);
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
	.peer-btn:hover {
		border-color: var(--ctp-primary);
		color: var(--ctp-primary);
	}
	.peer-btn.primary {
		border-color: var(--ctp-primary);
		background: color-mix(in srgb, var(--ctp-primary) 12%, transparent);
		color: var(--ctp-primary);
	}
	.peer-btn.primary:hover {
		background: color-mix(in srgb, var(--ctp-primary) 22%, transparent);
	}
	.stats-note {
		margin-bottom: 0.75rem;
		padding: 0.5rem 0.75rem;
		border-radius: 0.5rem;
		background: color-mix(in srgb, var(--ctp-overlay1) 12%, transparent);
		color: var(--ctp-subtext1);
		font-size: 0.8125rem;
		line-height: 1.4;
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
	.roster-foot {
		margin-top: 0.85rem;
		font-size: 0.78rem;
		color: var(--ctp-overlay0);
	}
	.qr-modal {
		display: flex;
		gap: 1.25rem;
		align-items: flex-start;
		flex-wrap: wrap;
	}
	.qr-img {
		border-radius: 0.5rem;
		background: #fff;
		padding: 0.75rem;
		flex-shrink: 0;
		width: 100%;
		max-width: 360px;
		height: auto;
		image-rendering: pixelated; /* keep modules crisp when scaled */
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
	.modal-foot {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	.link-modal {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.link-hint {
		font-size: 0.85rem;
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
	/* Phones: force the action buttons onto their own full-width line, left-aligned.
	   Above this the row wraps intrinsically (actions drop below only when they don't fit). */
	@media (max-width: 720px) {
		.peer-actions {
			flex-basis: 100%;
			margin-left: 0;
		}
	}
</style>

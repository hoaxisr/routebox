<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import QRCode from 'qrcode';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import type { AwgStatus, AwgPeer } from '$lib/types';

	let status = $state<AwgStatus | null>(null);
	let peers = $state<AwgPeer[]>([]);
	let loading = $state(true);

	let newName = $state('');
	let adding = $state(false);
	let enabling = $state(false);

	// QR modal: the peer being shown and the rendered data URL.
	let qrPeer = $state<AwgPeer | null>(null);
	let qrDataUrl = $state('');

	async function refresh() {
		loading = true;
		try {
			status = await api.awgStatus();
			peers = status.enabled ? await api.getAwgPeers() : [];
		} catch (e) {
			notifications.error(`${$t('awg.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	async function enable() {
		enabling = true;
		try {
			status = await api.awgEnable({
				subnet: '10.10.0.0/24',
				listen_port: 51820,
				mtu: 1420,
				dns: ['1.1.1.1']
			});
			notifications.success($t('awg.enabled'));
			peers = status.enabled ? await api.getAwgPeers() : [];
		} catch (e) {
			notifications.error(`${$t('awg.enableFailed')}: ${e}`);
		} finally {
			enabling = false;
		}
	}

	async function disable() {
		if (!confirm($t('awg.disableConfirm'))) return;
		try {
			status = await api.awgDisable();
			peers = [];
			notifications.success($t('awg.disabled'));
		} catch (e) {
			notifications.error(`${$t('awg.disableFailed')}: ${e}`);
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
			await refresh();
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
			await refresh();
		} catch (e) {
			notifications.error(`${$t('awg.deleteFailed')}: ${e}`);
		}
	}

	async function showQR(p: AwgPeer) {
		try {
			const conf = await api.getAwgPeerConfig(p.public_key);
			qrDataUrl = await QRCode.toDataURL(conf, { width: 256, margin: 1 });
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

	onMount(refresh);
</script>

<svelte:head><title>{$t('awg.title')} - RouteBox</title></svelte:head>

<div class="space-y-4 max-w-4xl">
	<div>
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('awg.title')}</h1>
		<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('awg.description')}</p>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if status}
		<!-- Status panel -->
		<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)] space-y-3">
			<div class="flex items-center justify-between gap-4 flex-wrap">
				<div class="flex items-center gap-2 flex-wrap">
					{#if status.enabled}
						<span class="status-badge success">{status.iface_up ? $t('awg.up') : $t('awg.down')}</span>
					{:else}
						<span class="status-badge info">{$t('awg.down')}</span>
					{/if}
					<span class="status-badge">{$t('awg.module')}: {status.module}</span>
					<span class="status-badge info">{$t('awg.peers')}: {status.peer_count}</span>
					{#if status.wan_iface}
						<span class="status-badge info">{$t('awg.wanIface')}: {status.wan_iface}</span>
					{/if}
				</div>
				{#if status.enabled}
					<button onclick={disable}
						class="px-3 py-1.5 text-[var(--ctp-text)] border border-[var(--ctp-surface2)] rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors text-sm">
						{$t('awg.disable')}
					</button>
				{/if}
			</div>

			{#if status.module !== 'loaded'}
				<p class="status-badge error">{$t('awg.moduleMissing')}</p>
			{/if}
			{#if status.enabled && status.nat_orphan}
				<p class="status-badge error">{$t('awg.natOrphan')}</p>
			{/if}
			{#if status.last_error}
				<p class="status-badge error break-all">{status.last_error}</p>
			{/if}

			{#if !status.enabled}
				<button onclick={enable} disabled={enabling}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
					{enabling ? $t('awg.enabling') : $t('awg.enable')}
				</button>
			{/if}
		</section>

		{#if status.enabled}
			<!-- Add client -->
			<div class="flex items-end gap-2 flex-wrap">
				<label class="flex-1 min-w-[12rem]">
					<span class="text-sm text-[var(--ctp-subtext1)]">{$t('awg.clientName')}</span>
					<input bind:value={newName} type="text" onkeydown={(e) => e.key === 'Enter' && addPeer()}
						class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
				</label>
				<button onclick={addPeer} disabled={adding || !newName.trim()}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
					{adding ? $t('awg.adding') : $t('awg.addClient')}
				</button>
			</div>

			<!-- Peers roster -->
			{#if peers.length === 0}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('awg.noPeers')}</div>
			{:else}
				<div class="space-y-3">
					{#each peers as p (p.public_key)}
						<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
							<div class="flex items-start justify-between gap-4 flex-wrap">
								<div class="min-w-0">
									<h2 class="text-lg font-medium text-[var(--ctp-text)]">{p.name || '(unnamed)'}</h2>
									<div class="mt-2 flex flex-wrap gap-1">
										<span class="status-badge info">{p.address}</span>
									</div>
								</div>
								<div class="flex items-center gap-2 flex-wrap">
									<button onclick={() => showQR(p)}
										class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm">
										{$t('awg.qr')}
									</button>
									<button onclick={() => download(p)}
										class="px-3 py-1.5 text-[var(--ctp-text)] border border-[var(--ctp-surface2)] rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors text-sm">
										{$t('awg.conf')}
									</button>
									<button onclick={() => removePeer(p)}
										class="px-3 py-1.5 text-[var(--ctp-red)] hover:bg-[var(--ctp-red)]/10 rounded-lg transition-colors text-sm">
										{$t('awg.delete')}
									</button>
								</div>
							</div>
						</section>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>

{#if qrPeer}
	<Modal open={!!qrPeer} title={$t('awg.qrTitle', { values: { name: qrPeer.name } })} size="md" onClose={() => (qrPeer = null)}>
		<div class="flex justify-center">
			<img src={qrDataUrl} alt="AmneziaWG client config QR" class="rounded-lg bg-white p-2" width="256" height="256" />
		</div>
		{#snippet footer()}
			<div class="flex justify-end">
				<button onclick={() => (qrPeer = null)}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">
					{$t('common.close')}
				</button>
			</div>
		{/snippet}
	</Modal>
{/if}

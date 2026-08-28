<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import { t } from 'svelte-i18n';
	import { api, createConnectionsStream } from '$lib/api/client';
	import { notifications, formatBytes } from '$lib/stores';
	import type { ClashConnection, ConnectionsResponse } from '$lib/types';
	import ConnectionTable from '$lib/components/monitor/ConnectionTable.svelte';
	import { groupTags } from '$lib/utils/chainLabel';

	// State
	let connections = $state<ClashConnection[]>([]);
	let downloadTotal = $state(0);
	let uploadTotal = $state(0);
	let loading = $state(true);
	let useWebSocket = $state(
		browser ? localStorage.getItem('connections.liveUpdates') !== 'false' : true
	);
	let stream: { close: () => void } | null = null;

	async function fetchConnections() {
		try {
			const data = await api.getConnections();
			updateData(data);
		} catch (err) {
			console.error('Failed to fetch connections:', err);
		} finally {
			loading = false;
		}
	}

	function updateData(data: ConnectionsResponse) {
		connections = data.connections || [];
		downloadTotal = data.downloadTotal || 0;
		uploadTotal = data.uploadTotal || 0;
		loading = false;
	}

	function startStream() {
		if (stream) return;
		stream = createConnectionsStream(
			updateData,
			(error) => {
				console.error('Connections stream error:', error);
			},
			undefined,
			(status) => {
				if (status === 'connected') {
					// WS (re)established — error-fallback polling must not keep running
					stopPolling();
				} else if (status === 'reconnecting') {
					// Keep data fresh while the helper reconnects
					fetchConnections();
					startPolling();
				}
			}
		);
	}

	function handleLiveUpdatesToggle(event: Event) {
		const enabled = (event.currentTarget as HTMLInputElement).checked;
		useWebSocket = enabled;
		if (browser) localStorage.setItem('connections.liveUpdates', String(enabled));
		if (enabled) {
			stopPolling();
			startStream();
		} else {
			stopStream();
			fetchConnections();
			startPolling();
		}
	}

	let pollInterval: ReturnType<typeof setInterval> | null = null;

	function startPolling() {
		if (pollInterval) return;
		pollInterval = setInterval(fetchConnections, 2000);
	}

	function stopPolling() {
		if (pollInterval) {
			clearInterval(pollInterval);
			pollInterval = null;
		}
	}

	function stopStream() {
		if (stream) {
			stream.close();
			stream = null;
		}
	}

	async function closeConnection(id: string) {
		try {
			await api.closeConnection(id);
			connections = connections.filter(c => c.id !== id);
			notifications.success($t('connections.connectionClosed'));
		} catch (err) {
			notifications.error(`${$t('errors.deleteFailed')}: ${err}`);
		}
	}

	async function closeAllConnections() {
		if (!confirm($t('connections.confirmCloseAll'))) return;
		try {
			await api.closeAllConnections();
			connections = [];
			notifications.success($t('connections.allConnectionsClosed'));
		} catch (err) {
			notifications.error(`${$t('errors.deleteFailed')}: ${err}`);
		}
	}

	// Only to tell a group tag from a plain outbound when marking a chain that
	// names one and nothing else (#79). A failure leaves the set empty, which
	// marks nothing — better than mislabelling.
	let groups = $state<Set<string>>(new Set());

	onMount(() => {
		api.listOutbounds()
			.then((obs) => (groups = groupTags(obs)))
			.catch(() => {});
		if (useWebSocket) {
			startStream();
		} else {
			fetchConnections();
			startPolling();
		}
	});

	onDestroy(() => {
		stopStream();
		stopPolling();
	});
</script>

<svelte:head>
	<title>{$t('connections.title')} - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-6xl mx-auto">
	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-3 mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('connections.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{connections.length === 1
					? $t('connections.activeConnection', { values: { count: connections.length } })
					: $t('connections.activeConnectionPlural', { values: { count: connections.length } })}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<!-- WebSocket toggle -->
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					checked={useWebSocket}
					onchange={handleLiveUpdatesToggle}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('connections.liveUpdates')}
			</label>

			<!-- Refresh (only when not using WebSocket) -->
			{#if !useWebSocket}
				<button
					onclick={fetchConnections}
					class="px-3 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
					aria-label={$t('common.refresh')}
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
					</svg>
				</button>
			{/if}

			<!-- Close All -->
			<button
				onclick={closeAllConnections}
				disabled={connections.length === 0}
				class="px-4 py-2 bg-[var(--ctp-red)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center gap-2"
			>
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
				{$t('connections.closeAll')}
			</button>
		</div>
	</div>

	<!-- Stats -->
	<div class="grid grid-cols-1 sm:grid-cols-3 gap-3 sm:gap-4 mb-6">
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('connections.activeConnections')}</div>
			<div class="text-2xl font-semibold text-[var(--ctp-text)]">{connections.length}</div>
		</div>
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('connections.totalUpload')}</div>
			<div class="text-2xl font-semibold text-[var(--ctp-text)]">{formatBytes(uploadTotal)}</div>
		</div>
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">{$t('connections.totalDownload')}</div>
			<div class="text-2xl font-semibold text-[var(--ctp-text)]">{formatBytes(downloadTotal)}</div>
		</div>
	</div>

	<!-- Table -->
	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="animate-spin h-8 w-8 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else}
		<ConnectionTable {connections} onClose={closeConnection} {groups} />
	{/if}
</div>

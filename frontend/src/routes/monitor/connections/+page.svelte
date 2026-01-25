<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, createConnectionsStream } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { ClashConnection, ConnectionsResponse } from '$lib/types';
	import ConnectionTable from '$lib/components/monitor/ConnectionTable.svelte';

	// State
	let connections = $state<ClashConnection[]>([]);
	let downloadTotal = $state(0);
	let uploadTotal = $state(0);
	let loading = $state(true);
	let useWebSocket = $state(true);
	let stream: { close: () => void } | null = null;

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
	}

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
				console.error('WebSocket error, falling back to polling:', error);
				useWebSocket = false;
				stopStream();
				fetchConnections();
				startPolling();
			},
			() => {
				// On close, try to reconnect after a delay if still using WebSocket
				if (useWebSocket) {
					setTimeout(() => {
						if (useWebSocket && !stream) {
							startStream();
						}
					}, 3000);
				}
			}
		);
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
			notifications.success('Connection closed');
		} catch (err) {
			notifications.error(`Failed to close connection: ${err}`);
		}
	}

	async function closeAllConnections() {
		try {
			await api.closeAllConnections();
			connections = [];
			notifications.success('All connections closed');
		} catch (err) {
			notifications.error(`Failed to close connections: ${err}`);
		}
	}

	onMount(() => {
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

	onDestroy(() => {
		stopStream();
	});

	$effect(() => {
		if (useWebSocket && !stream) {
			startStream();
		} else if (!useWebSocket && stream) {
			stopStream();
			fetchConnections();
		}
	});
</script>

<svelte:head>
	<title>Connections - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-6xl mx-auto">
	<!-- Header -->
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">Connections</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{connections.length} active connection{connections.length !== 1 ? 's' : ''}
			</p>
		</div>
		<div class="flex items-center gap-3">
			<!-- WebSocket toggle -->
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={useWebSocket}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				Live updates
			</label>

			<!-- Refresh (only when not using WebSocket) -->
			{#if !useWebSocket}
				<button
					onclick={fetchConnections}
					class="px-3 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
					aria-label="Refresh"
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
				Close All
			</button>
		</div>
	</div>

	<!-- Stats -->
	<div class="grid grid-cols-3 gap-4 mb-6">
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">Active Connections</div>
			<div class="text-2xl font-semibold text-[var(--ctp-text)]">{connections.length}</div>
		</div>
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">Total Upload</div>
			<div class="text-2xl font-semibold text-[var(--ctp-text)]">{formatBytes(uploadTotal)}</div>
		</div>
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
			<div class="text-sm text-[var(--ctp-overlay1)]">Total Download</div>
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
		<ConnectionTable {connections} onClose={closeConnection} />
	{/if}
</div>

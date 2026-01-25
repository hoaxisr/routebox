<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, createTrafficStream } from '$lib/api/client';
	import type { ClashConnection } from '$lib/types';

	// Svelte 5 reactive state
	let trafficUp = $state(0);
	let trafficDown = $state(0);
	let totalUp = $state(0);
	let totalDown = $state(0);
	let history = $state<{ up: number; down: number; time: number }[]>([]);
	let stream: { close: () => void } | null = null;
	let connected = $state(false);

	// Traffic by outbound
	interface OutboundTraffic {
		name: string;
		upload: number;
		download: number;
		connections: number;
	}
	let trafficByOutbound = $state<OutboundTraffic[]>([]);
	let refreshTimer: ReturnType<typeof setInterval> | null = null;

	const MAX_HISTORY = 60; // 60 data points

	// Derived state for max traffic
	let maxTraffic = $derived(Math.max(
		...history.map((h) => Math.max(h.up, h.down)),
		1024 // minimum 1KB for scale
	));

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
	}

	function formatSpeed(bytesPerSec: number): string {
		return `${formatBytes(bytesPerSec)}/s`;
	}

	let wsError = $state('');

	function startStream() {
		stream = createTrafficStream(
			(data) => {
				trafficUp = data.up;
				trafficDown = data.down;
				totalUp += data.up;
				totalDown += data.down;
				connected = true;
				wsError = '';

				history = [
					...history.slice(-(MAX_HISTORY - 1)),
					{ up: data.up, down: data.down, time: Date.now() }
				];
			},
			(error) => {
				wsError = error;
				connected = false;
			},
			() => {
				// On close, try to reconnect after a delay
				connected = false;
				setTimeout(() => {
					if (!stream) {
						startStream();
					}
				}, 3000);
			}
		);
	}

	function stopStream() {
		if (stream) {
			stream.close();
			stream = null;
			connected = false;
		}
	}

	async function fetchTrafficByOutbound() {
		try {
			const data = await api.getConnections();
			const connections = data.connections || [];

			// Aggregate traffic by outbound (first item in chains)
			const byOutbound = new Map<string, OutboundTraffic>();

			for (const conn of connections) {
				const outbound = conn.chains[0] || 'unknown';

				if (!byOutbound.has(outbound)) {
					byOutbound.set(outbound, { name: outbound, upload: 0, download: 0, connections: 0 });
				}

				const stats = byOutbound.get(outbound)!;
				stats.upload += conn.upload;
				stats.download += conn.download;
				stats.connections += 1;
			}

			// Sort by total traffic (download + upload) descending
			trafficByOutbound = Array.from(byOutbound.values())
				.sort((a, b) => (b.upload + b.download) - (a.upload + a.download));
		} catch (err) {
			console.error('Failed to fetch outbound traffic:', err);
		}
	}

	function startRefreshTimer() {
		fetchTrafficByOutbound();
		refreshTimer = setInterval(fetchTrafficByOutbound, 5000); // Refresh every 5 seconds
	}

	function stopRefreshTimer() {
		if (refreshTimer) {
			clearInterval(refreshTimer);
			refreshTimer = null;
		}
	}

	onMount(() => {
		startStream();
		startRefreshTimer();
	});

	onDestroy(() => {
		stopStream();
		stopRefreshTimer();
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">Traffic Monitor</h1>
		<div class="flex items-center gap-2">
			<span
				class="w-2 h-2 rounded-full"
				class:bg-[var(--ctp-green)]={connected}
				class:bg-[var(--ctp-yellow)]={!connected && !wsError}
				class:bg-[var(--ctp-red)]={wsError}
			></span>
			<span class="text-sm text-[var(--ctp-overlay1)]">
				{connected ? 'Connected' : wsError ? 'Error' : 'Connecting...'}
			</span>
			{#if wsError}
				<span class="text-xs text-[var(--ctp-red)]" title={wsError}>Clash API unavailable</span>
			{/if}
		</div>
	</div>

	<!-- Current speed -->
	<div class="grid grid-cols-2 gap-4">
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-6">
			<div class="flex items-center gap-3 mb-2">
				<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 10l7-7m0 0l7 7m-7-7v18" />
				</svg>
				<span class="text-[var(--ctp-overlay1)]">Upload</span>
			</div>
			<div class="text-3xl font-bold text-[var(--ctp-text)]">{formatSpeed(trafficUp)}</div>
			<div class="text-sm text-[var(--ctp-overlay0)] mt-1">Total: {formatBytes(totalUp)}</div>
		</div>

		<div class="bg-[var(--ctp-surface0)] rounded-xl p-6">
			<div class="flex items-center gap-3 mb-2">
				<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
				</svg>
				<span class="text-[var(--ctp-overlay1)]">Download</span>
			</div>
			<div class="text-3xl font-bold text-[var(--ctp-text)]">{formatSpeed(trafficDown)}</div>
			<div class="text-sm text-[var(--ctp-overlay0)] mt-1">Total: {formatBytes(totalDown)}</div>
		</div>
	</div>

	<!-- Traffic chart -->
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6">
		<h2 class="text-lg font-semibold text-[var(--ctp-subtext1)] mb-4">Traffic History</h2>

		<div class="h-40 flex items-end gap-px">
			{#each history as point}
				<div class="flex-1 h-full flex flex-col justify-end gap-px">
					<!-- Upload bar (primary color) -->
					<div
						class="w-full bg-[var(--ctp-primary)] rounded-sm min-h-[1px]"
						style="height: {Math.max((point.up / maxTraffic) * 50, 0.5)}%"
					></div>
					<!-- Download bar (green) -->
					<div
						class="w-full bg-[var(--ctp-green)] rounded-sm min-h-[1px]"
						style="height: {Math.max((point.down / maxTraffic) * 50, 0.5)}%"
					></div>
				</div>
			{/each}

			{#if history.length === 0}
				<div class="flex-1 flex items-center justify-center text-[var(--ctp-overlay0)]">
					Waiting for data...
				</div>
			{/if}
		</div>

		<div class="flex justify-between mt-2 text-xs text-[var(--ctp-overlay0)]">
			<span>60 seconds ago</span>
			<span>Now</span>
		</div>
	</div>

	<!-- Traffic by Outbound -->
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6">
		<h2 class="text-lg font-semibold text-[var(--ctp-subtext1)] mb-4">Traffic by Outbound</h2>

		{#if trafficByOutbound.length === 0}
			<div class="text-center py-4 text-[var(--ctp-overlay0)]">
				No active connections
			</div>
		{:else}
			<div class="space-y-3">
				{#each trafficByOutbound as outbound}
					{@const total = outbound.upload + outbound.download}
					{@const maxTotal = trafficByOutbound[0].upload + trafficByOutbound[0].download}
					{@const percentage = maxTotal > 0 ? (total / maxTotal) * 100 : 0}

					<div class="space-y-1">
						<div class="flex items-center justify-between text-sm">
							<div class="flex items-center gap-2">
								<span class="font-medium text-[var(--ctp-text)]">{outbound.name}</span>
								<span class="text-xs text-[var(--ctp-overlay0)]">
									({outbound.connections} conn{outbound.connections !== 1 ? 's' : ''})
								</span>
							</div>
							<div class="flex items-center gap-4 font-mono text-xs text-[var(--ctp-subtext1)]">
								<span>↑ {formatBytes(outbound.upload)}</span>
								<span>↓ {formatBytes(outbound.download)}</span>
							</div>
						</div>
						<div class="h-2 bg-[var(--ctp-surface2)] rounded-full overflow-hidden">
							<div class="h-full flex">
								<div
									class="bg-[var(--ctp-primary)] transition-all duration-300"
									style="width: {maxTotal > 0 ? (outbound.upload / maxTotal) * 100 : 0}%"
								></div>
								<div
									class="bg-[var(--ctp-overlay0)] transition-all duration-300"
									style="width: {maxTotal > 0 ? (outbound.download / maxTotal) * 100 : 0}%"
								></div>
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

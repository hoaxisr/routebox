<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api, createTrafficStream, createConnectionsStream } from '$lib/api/client';
	import { notifications, formatBytes, formatSpeed, clientNames } from '$lib/stores';
	import { singboxVersion, loadVersion } from '$lib/stores/version';
	import PendingChanges from '$lib/components/shared/PendingChanges.svelte';
	import type { ProcessStatus, DetectedConfig, ClashConnection } from '$lib/types';

	// Svelte 5 reactive state
	let status = $state<ProcessStatus>({ running: false });
	let detectedConfig = $state<DetectedConfig | null>(null);
	let loading = $state(true);
	let actionLoading = $state('');
	let trafficUp = $state(0);
	let trafficDown = $state(0);
	let uploadTotal = $state(0);
	let downloadTotal = $state(0);
	let connectionCount = $state(0);
	let topConnections = $state<ClashConnection[]>([]);
	let trafficStream: { close: () => void } | null = null;
	let connectionsStream: { close: () => void } | null = null;

	async function fetchStatus() {
		try {
			status = await api.getStatus();
			// Check for config mismatch if running
			if (status.running) {
				try {
					detectedConfig = await api.getDetectedConfig();
				} catch {
					detectedConfig = null;
				}
				// Load sing-box version once
				if (!$singboxVersion) {
					loadVersion();
				}
			}
		} catch (e) {
			console.error('Failed to fetch status:', e);
		} finally {
			loading = false;
		}
	}

	async function handleStart() {
		actionLoading = 'start';
		try {
			const result = await api.start();
			notifications.success('amnezia-box started');
			if (result.warning) {
				notifications.warning(result.warning);
			}
			await fetchStatus();
			startTrafficStream();
		} catch (e) {
			notifications.error(`Failed to start: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	async function handleStop() {
		actionLoading = 'stop';
		try {
			await api.stop();
			notifications.success('amnezia-box stopped');
			await fetchStatus();
			stopTrafficStream();
		} catch (e) {
			notifications.error(`Failed to stop: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	async function handleRestart() {
		actionLoading = 'restart';
		try {
			const result = await api.restart();
			notifications.success('amnezia-box restarted');
			if (result.warning) {
				notifications.warning(result.warning);
			}
			// Reset totals and restart streams after sing-box restart
			stopTrafficStream();
			stopConnectionsStream();
			uploadTotal = 0;
			downloadTotal = 0;
			await fetchStatus();
			startTrafficStream();
			startConnectionsStream();
		} catch (e) {
			notifications.error(`Failed to restart: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	async function handleReload() {
		actionLoading = 'reload';
		try {
			await api.reload();
			notifications.success('Configuration reloaded (SIGHUP)');
			await fetchStatus();
		} catch (e) {
			notifications.error(`Failed to reload: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	async function handleUseDetectedConfig() {
		actionLoading = 'switch';
		try {
			const result = await api.useDetectedConfig();
			notifications.success(`Switched to config: ${result.path}`);
			detectedConfig = null;
			await fetchStatus();
		} catch (e) {
			notifications.error(`Failed to switch config: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	function startTrafficStream() {
		if (trafficStream) return;
		trafficStream = createTrafficStream((data) => {
			trafficUp = data.up;
			trafficDown = data.down;
		});
	}

	function stopTrafficStream() {
		if (trafficStream) {
			trafficStream.close();
			trafficStream = null;
			trafficUp = 0;
			trafficDown = 0;
		}
	}

	function startConnectionsStream() {
		if (connectionsStream) return;
		connectionsStream = createConnectionsStream((data) => {
			connectionCount = data.connections?.length ?? 0;
			uploadTotal = data.uploadTotal ?? 0;
			downloadTotal = data.downloadTotal ?? 0;
			// Get top 5 by download
			topConnections = (data.connections ?? [])
				.sort((a, b) => b.download - a.download)
				.slice(0, 5);
		});
	}

	function stopConnectionsStream() {
		if (connectionsStream) {
			connectionsStream.close();
			connectionsStream = null;
			connectionCount = 0;
			topConnections = [];
		}
	}

	onMount(() => {
		fetchStatus();
		// Poll status every 5 seconds
		const interval = setInterval(fetchStatus, 5000);

		return () => {
			clearInterval(interval);
			stopTrafficStream();
			stopConnectionsStream();
		};
	});

	// Start/stop streams when status changes
	$effect(() => {
		if (status.running && !trafficStream) {
			startTrafficStream();
			startConnectionsStream();
		}
		if (!status.running && trafficStream) {
			stopTrafficStream();
			stopConnectionsStream();
		}
	});
</script>

<div class="space-y-6">
	<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('dashboard.title')}</h1>

	<!-- System Requirements Warning -->
	{#if status.system_checks && !status.system_checks.all_checks_passed}
		<div class="bg-[var(--ctp-red)] rounded-xl p-6 shadow-lg">
			<div class="flex items-start gap-4">
				<svg class="w-10 h-10 text-white flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
				</svg>
				<div class="flex-1">
					<h2 class="text-xl font-bold text-white mb-2">System Requirements Not Met</h2>
					<p class="text-white/90 text-lg mb-3">
						amnezia-box cannot work as a router without the following requirements.
					</p>
					<div class="space-y-2 text-white/80 text-sm">
						{#if !status.system_checks.is_root}
							<div class="flex items-center gap-2">
								<svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
								<span class="font-medium">Not running as root</span>
								<span class="text-white/70">(required for TUN interface)</span>
							</div>
						{/if}
						{#if !status.system_checks.ipv4_forward}
							<div class="flex items-center gap-2">
								<svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
								<code class="bg-white/20 px-2 py-0.5 rounded">net.ipv4.ip_forward = 0</code>
								<span class="text-white/70">(required)</span>
							</div>
						{/if}
						{#if !status.system_checks.ipv6_forward}
							<div class="flex items-center gap-2">
								<svg class="w-5 h-5 text-white/60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01" />
								</svg>
								<code class="bg-white/20 px-2 py-0.5 rounded">net.ipv6.conf.all.forwarding = 0</code>
								<span class="text-white/70">(optional, for IPv6)</span>
							</div>
						{/if}
						{#if status.system_checks.ipv6_disabled}
							<div class="flex items-center gap-2">
								<svg class="w-5 h-5 text-white/60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
								</svg>
								<code class="bg-white/20 px-2 py-0.5 rounded">net.ipv6.conf.all.disable_ipv6 = 1</code>
								<span class="text-white/70">(IPv6 addresses will be auto-removed from TUN)</span>
							</div>
						{/if}
					</div>
					<div class="mt-4 space-y-3">
						{#if !status.system_checks.is_root}
							<div class="p-3 bg-white/10 rounded-lg">
								<p class="text-white font-medium mb-2">Run routebox with sudo:</p>
								<code class="block text-white/90 font-mono text-sm">
									sudo ./routebox --config /path/to/config.json
								</code>
							</div>
						{/if}
						{#if !status.system_checks.ipv4_forward}
							<div class="p-3 bg-white/10 rounded-lg">
								<p class="text-white font-medium mb-2">Enable IP forwarding:</p>
								<code class="block text-white/90 font-mono text-sm">
									echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf && sysctl -p
								</code>
							</div>
						{/if}
					</div>
				</div>
			</div>
		</div>
	{/if}

	<!-- IPv6 Disabled Info (shown when main checks pass but IPv6 is disabled) -->
	{#if status.system_checks?.all_checks_passed && status.system_checks?.ipv6_disabled}
		<div class="bg-[var(--ctp-blue)]/20 border border-[var(--ctp-blue)]/30 rounded-xl p-4">
			<div class="flex items-start gap-3">
				<svg class="w-5 h-5 text-[var(--ctp-blue)] flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<div>
					<p class="text-[var(--ctp-text)] font-medium">IPv6 is disabled in your system</p>
					<p class="text-[var(--ctp-subtext0)] text-sm mt-1">
						<code class="bg-[var(--ctp-surface1)] px-1.5 py-0.5 rounded text-xs">net.ipv6.conf.all.disable_ipv6 = 1</code>
						— IPv6 addresses will be automatically removed from TUN interface on start to prevent errors.
					</p>
				</div>
			</div>
		</div>
	{/if}

	<!-- Pending Changes (draft config) -->
	<PendingChanges />

	<!-- Status Card -->
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6">
		<div class="flex items-center justify-between mb-4">
			<h2 class="text-lg font-semibold text-[var(--ctp-subtext1)]">amnezia-box</h2>
			{#if loading}
				<span class="text-[var(--ctp-overlay1)]">{$t('common.loading')}</span>
			{:else}
				<span
					class="px-3 py-1 rounded-full text-sm font-medium text-white"
					class:bg-[var(--ctp-green)]={status.running}
					class:bg-[var(--ctp-red)]={!status.running}
				>
					{status.running ? $t('status.running') : $t('status.stopped')}
				</span>
			{/if}
		</div>

		<!-- Config mismatch warning -->
		{#if detectedConfig && !detectedConfig.match && detectedConfig.detected_path}
			<div class="mb-4 p-4 bg-[color-mix(in_srgb,var(--ctp-red)_10%,transparent)] border border-[var(--ctp-red)] rounded-lg">
				<div class="flex items-start gap-3">
					<svg class="w-5 h-5 text-[var(--ctp-red)] flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					<div class="flex-1">
						<p class="text-sm font-medium text-[var(--ctp-text)]">Config path mismatch detected</p>
						<p class="text-xs text-[var(--ctp-overlay1)] mt-1">
							Running process uses: <code class="bg-[var(--ctp-surface1)] px-1 rounded">{detectedConfig.detected_path}</code>
						</p>
						<p class="text-xs text-[var(--ctp-overlay1)]">
							UI is editing: <code class="bg-[var(--ctp-surface1)] px-1 rounded">{detectedConfig.current_path}</code>
						</p>
						<button
							onclick={handleUseDetectedConfig}
							disabled={actionLoading !== ''}
							class="mt-2 px-3 py-1 text-sm bg-[var(--ctp-primary)] text-white rounded hover:opacity-90 disabled:opacity-50"
						>
							{actionLoading === 'switch' ? 'Switching...' : 'Switch to detected config'}
						</button>
					</div>
				</div>
			</div>
		{/if}

		{#if status.running}
			<!-- Control buttons first -->
			<div class="flex gap-3 flex-wrap mb-6">
				<button
					onclick={handleStop}
					disabled={actionLoading !== ''}
					class="px-4 py-2 bg-[var(--ctp-red)] text-white rounded-lg font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
				>
					{actionLoading === 'stop' ? $t('common.stopping') : $t('dashboard.stop')}
				</button>
				<button
					onclick={handleReload}
					disabled={actionLoading !== ''}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
					title="Hot reload configuration (SIGHUP)"
				>
					{actionLoading === 'reload' ? 'Reloading...' : 'Reload Config'}
				</button>
				<button
					onclick={handleRestart}
					disabled={actionLoading !== ''}
					class="px-4 py-2 bg-[var(--ctp-surface2)] text-[var(--ctp-text)] rounded-lg font-medium hover:bg-[var(--ctp-overlay0)] disabled:opacity-50 transition-colors"
					title="Full process restart"
				>
					{actionLoading === 'restart' ? $t('common.restarting') : $t('dashboard.restart')}
				</button>
			</div>

			<!-- Version + Config path bar -->
			{#if $singboxVersion || status.config_path}
				<div class="bg-[var(--ctp-surface1)] rounded-lg px-4 py-2 flex items-center gap-4 flex-wrap mb-3 text-xs">
					{#if $singboxVersion}
						<div class="flex items-center gap-1.5">
							<span class="text-[var(--ctp-overlay1)]">sing-box</span>
							<span class="text-[var(--ctp-subtext1)]">{$singboxVersion.version}</span>
						</div>
					{/if}
					{#if $singboxVersion && status.config_path}
						<div class="w-px h-[14px] bg-[var(--ctp-surface2)]"></div>
					{/if}
					{#if status.config_path}
						<div class="flex items-center gap-1.5 min-w-0">
							<span class="text-[var(--ctp-overlay1)] flex-shrink-0">Config</span>
							<span class="text-[var(--ctp-subtext1)] truncate">{status.config_path}</span>
						</div>
					{/if}
				</div>
			{/if}

			<!-- System metrics bar -->
			<div class="bg-[var(--ctp-surface1)] rounded-lg px-4 py-3 flex items-center gap-4 sm:gap-5 flex-wrap mb-6">
				<div class="flex items-baseline gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">Managed by</span>
					{#if status.managed_by === 'systemd'}
						<span class="text-sm text-[var(--ctp-primary)]">systemd</span>
						{#if status.service_name}
							<span class="text-[10px] text-[var(--ctp-overlay0)]">({status.service_name})</span>
						{/if}
					{:else}
						<span class="text-sm text-[var(--ctp-text)]">standalone</span>
					{/if}
				</div>
				<div class="w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="flex items-baseline gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">PID</span>
					<span class="text-sm text-[var(--ctp-text)]">{status.pid || '-'}</span>
				</div>
				<div class="w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="flex items-baseline gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">Uptime</span>
					<span class="text-sm text-[var(--ctp-text)]">{status.uptime || '-'}</span>
				</div>
				<div class="w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="flex items-baseline gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">Connections</span>
					<span class="text-sm text-[var(--ctp-text)]">{connectionCount}</span>
				</div>
			</div>

			<!-- Traffic stats -->
			<div class="grid grid-cols-2 gap-3 sm:gap-4 mb-6">
				<div class="bg-[var(--ctp-surface1)] rounded-lg p-3 sm:p-4">
					<div class="text-xs sm:text-sm text-[var(--ctp-overlay1)]">Traffic Rate</div>
					<div class="text-[var(--ctp-text)] text-sm sm:text-lg">
						<div><span class="text-[var(--ctp-overlay0)]">↑</span> {formatSpeed(trafficUp)}</div>
						<div><span class="text-[var(--ctp-overlay0)]">↓</span> {formatSpeed(trafficDown)}</div>
					</div>
				</div>
				<div class="bg-[var(--ctp-surface1)] rounded-lg p-3 sm:p-4">
					<div class="text-xs sm:text-sm text-[var(--ctp-overlay1)]">Total Transfer</div>
					<div class="text-[var(--ctp-text)] text-sm sm:text-lg">
						<div><span class="text-[var(--ctp-overlay0)]">↑</span> {formatBytes(uploadTotal)}</div>
						<div><span class="text-[var(--ctp-overlay0)]">↓</span> {formatBytes(downloadTotal)}</div>
					</div>
				</div>
			</div>

			<!-- Top Connections Preview -->
			{#if topConnections.length > 0}
				<div>
					<div class="flex items-center justify-between mb-2">
						<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Top Connections</h3>
						<a href="/monitor/connections" class="text-sm text-[var(--ctp-primary)] hover:underline">View all</a>
					</div>
					<div class="bg-[var(--ctp-surface1)] rounded-lg divide-y divide-[var(--ctp-surface2)]">
						{#each topConnections as conn}
							{@const sourceName = $clientNames.get(conn.metadata.sourceIP)}
							<div class="px-3 sm:px-4 py-2 flex items-center gap-2 sm:gap-4">
								<div class="min-w-0 flex-1 truncate text-sm text-[var(--ctp-text)]">
									{conn.metadata.host || conn.metadata.destinationIP}
								</div>
								<div class="hidden sm:block w-[8rem] text-right text-xs tabular-nums text-[var(--ctp-overlay1)] flex-shrink-0 truncate" title={conn.metadata.sourceIP}>
									{#if sourceName}{sourceName}
									{:else}<span class="font-mono">{conn.metadata.sourceIP}</span>{/if}
								</div>
								<div class="hidden sm:flex items-center justify-end gap-1 w-[10rem] flex-shrink-0">
									{#each conn.chains as chain}
										<span class="selection-chip">{chain}</span>
									{/each}
								</div>
								<div class="w-[5.5rem] text-right text-xs sm:text-sm font-mono text-[var(--ctp-subtext1)] tabular-nums flex-shrink-0">
									{formatBytes(conn.download)}
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{:else}
			<div class="flex gap-3 flex-wrap">
				<button
					onclick={handleStart}
					disabled={actionLoading !== ''}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
				>
					{actionLoading === 'start' ? $t('common.starting') : $t('dashboard.start')}
				</button>
			</div>
		{/if}
	</div>

	<!-- Quick Links -->
	<div class="grid grid-cols-3 gap-2 sm:gap-4">
		<a
			href="/config/endpoints"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex flex-col items-center gap-1.5 sm:flex-row sm:items-center sm:gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
					</svg>
				</div>
				<div class="min-w-0 text-center sm:text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Endpoints
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate hidden sm:block">AWG, WireGuard</p>
				</div>
			</div>
		</a>

		<a
			href="/config/outbounds"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex flex-col items-center gap-1.5 sm:flex-row sm:items-center sm:gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
					</svg>
				</div>
				<div class="min-w-0 text-center sm:text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Outbounds
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate hidden sm:block">VLESS, Hysteria2</p>
				</div>
			</div>
		</a>

		<a
			href="/config/routes"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex flex-col items-center gap-1.5 sm:flex-row sm:items-center sm:gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
					</svg>
				</div>
				<div class="min-w-0 text-center sm:text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Routes
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate hidden sm:block">Rules & rule sets</p>
				</div>
			</div>
		</a>
	</div>
</div>

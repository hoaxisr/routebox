<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api, createTrafficStream, createConnectionsStream } from '$lib/api/client';
	import { notifications, formatBytes, formatSpeed, clientNames, panelMode, routerMode, behindFront, refreshStatus } from '$lib/stores';
	import { singboxVersion, loadVersion } from '$lib/stores/version';
	import PendingChanges from '$lib/components/shared/PendingChanges.svelte';
	import PieChart from '$lib/components/monitor/PieChart.svelte';
	import { splitUnit, areaPaths } from '$lib/utils/sparkline';
	import { seriesRates } from '$lib/utils/trafficSeries';
	import { liveHistory, type DashboardPeriod, type DashboardDim } from '$lib/stores/liveHistory';
	import { localSourceConnections } from '$lib/utils/clientIp';
	import type { ProcessStatus, ClashConnection, SystemInfo, TrafficBucket } from '$lib/types';

	// Svelte 5 reactive state
	let status = $state<ProcessStatus>({ running: false });
	let loading = $state(true);
	let actionLoading = $state('');
	let trafficUp = $state(0);
	let trafficDown = $state(0);
	let uploadTotal = $state(0);
	let downloadTotal = $state(0);
	let connectionCount = $state(0);
	let topConnections = $state<ClashConnection[]>([]);
	// Every live connection, for the breakdown beside the graph (#101). Filtered
	// the same way the Breakdown page filters its live view: a remote source is
	// not a device of this box (#102).
	let liveConns = $state<ClashConnection[]>([]);
	let trafficStream: { close: () => void } | null = null;
	let connectionsStream: { close: () => void } | null = null;

	// Live strips: the last minute of each metric. Traffic ticks once a second
	// from the stream (60 points); host metrics are polled every 2 s (30 points).
	const TRAFFIC_POINTS = 60;
	const SYSTEM_POINTS = 30;
	let downHist = $state<number[]>(liveHistory.down);
	let upHist = $state<number[]>(liveHistory.up);
	let cpuHist = $state<number[]>(liveHistory.cpu);
	let system = $state<SystemInfo | null>(null);

	// The traffic graph shows either the live minute above or, for 1h/24h, the
	// minute buckets of /traffic/history as B/s — the same SQLite history the
	// Breakdown panel reads, so no second time series is kept (#99).
	const PERIODS: DashboardPeriod[] = ['60s', '1h', '24h'];
	let period = $state<DashboardPeriod>(liveHistory.period);
	let histDown = $state<number[]>([]);
	let histUp = $state<number[]>([]);
	let histError = $state('');
	// The same response already carries the per (source, domain, chain) buckets
	// the breakdown beside the graph needs, so 1h/24h costs no extra request.
	let histBuckets = $state<TrafficBucket[]>([]);
	async function loadHistory(p: '1h' | '24h') {
		try {
			const r = await api.getTrafficHistory(p, { series: true });
			({ down: histDown, up: histUp } = seriesRates(r.series, r.start_ts, r.end_ts, r.step ?? 60, 240));
			histBuckets = r.buckets ?? [];
			histError = '';
		} catch (e) {
			// 503 = no traffic store (Clash API address unset); anything else is
			// still "no graph", and the message says which.
			histError = String(e);
			histDown = [];
			histUp = [];
			histBuckets = [];
		}
	}
	$effect(() => {
		liveHistory.period = period;
		if (period === '60s') return;
		const p = period;
		loadHistory(p);
		// Buckets are minutes, so once a minute is as fresh as it gets.
		const timer = setInterval(() => loadHistory(p), 60_000);
		return () => clearInterval(timer);
	});
	let graphDown = $derived(period === '60s' ? downHist : histDown);
	let graphUp = $derived(period === '60s' ? upHist : histUp);
	// One scale for both series, so a 6 KB/s upload does not look as tall as a
	// 60 KB/s download drawn over it.
	let graphMax = $derived(Math.max(1024, ...graphDown, ...graphUp) * 1.15);
	const GW = 600;
	const GH = 88;
	let downPaths = $derived(areaPaths(graphDown, graphMax, GW, GH));
	let upPaths = $derived(areaPaths(graphUp, graphMax, GW, GH));
	let periodLabel = $derived(
		period === '60s' ? $t('dashboard.lastMinute') : period === '1h' ? $t('dashboard.lastHour') : $t('dashboard.lastDay')
	);
	// Breakdown beside the graph (#101): one ring, switched between clients and
	// chains, counted over the SAME period the graph shows — live connections for
	// the minute, history buckets for 1h/24h.
	let sideDim = $state<DashboardDim>(liveHistory.dim);
	$effect(() => {
		liveHistory.dim = sideDim;
	});
	let sideItems = $derived.by(() => {
		const totals = new Map<string, number>();
		const add = (key: string, bytes: number) => totals.set(key, (totals.get(key) ?? 0) + bytes);
		if (period === '60s') {
			for (const c of liveConns) {
				add(
					sideDim === 'source'
						? c.metadata.sourceIP || 'unknown'
						: c.chains?.length
							? c.chains.join(' → ')
							: '-',
					c.upload + c.download
				);
			}
		} else {
			for (const b of histBuckets) {
				add(sideDim === 'source' ? b.source || 'unknown' : b.chain || '-', b.upload + b.download);
			}
		}
		return [...totals]
			.filter(([, value]) => value > 0)
			.map(([key, value]) => ({
				key,
				label: sideDim === 'source' ? ($clientNames.get(key) ?? key) : key,
				value
			}));
	});

	// Hovering the graph reads out the moment under the cursor. The readout sits
	// in the fixed slot at the end of the legend row, where the period label
	// otherwise is: numbers that chase the cursor cover the very line being read.
	let hoverIdx = $state<number | null>(null);
	let hoverPoint = $derived.by(() => {
		const i = hoverIdx;
		if (i == null || i >= graphDown.length) return null;
		const points = graphDown.length;
		const windowSec = period === '60s' ? 60 : period === '1h' ? 3600 : 86400;
		// Per point, not per gap: the live minute is one sample a second even
		// while it is still filling, and a resampled 24 h is one point per step.
		const step = period === '60s' ? 1 : windowSec / points;
		const back = Math.round((points - 1 - i) * step);
		// Wall-clock time of the point, not "−7 min": the reader compares it with
		// logs and connection times, which are absolute (#108). Seconds only
		// where the resolution is seconds.
		const at = new Date(Date.now() - back * 1000);
		return {
			time: at.toLocaleTimeString([], period === '24h' ? { hourCycle: 'h23', hour: '2-digit', minute: '2-digit' } : { hourCycle: 'h23', hour: '2-digit', minute: '2-digit', second: '2-digit' }),
			down: formatSpeed(graphDown[i] ?? 0),
			up: formatSpeed(graphUp[i] ?? 0)
		};
	});
	function trackHover(ev: PointerEvent) {
		const box = (ev.currentTarget as HTMLElement).getBoundingClientRect();
		if (!box.width || graphDown.length < 2) return;
		const ratio = Math.min(Math.max((ev.clientX - box.left) / box.width, 0), 1);
		hoverIdx = Math.round(ratio * (graphDown.length - 1));
	}

	const avg = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : 0);
	const peak = (xs: number[]) => (xs.length ? Math.max(...xs) : 0);
	const trafficNote = (xs: number[]) => `${$t('dashboard.avg')} ${formatSpeed(avg(xs))} · ${$t('dashboard.peak')} ${formatSpeed(peak(xs))}`;
	let rate = $derived({ down: splitUnit(formatSpeed(trafficDown)), up: splitUnit(formatSpeed(trafficUp)) });
	let memPct = $derived(system && system.mem_total ? Math.round((system.mem_used / system.mem_total) * 100) : null);
	let cpuPct = $derived(system?.cpu_percent == null ? null : Math.round(system.cpu_percent));
	// CPU keeps its shape in the footer, at 40x12. Scaled to its own peak with a
	// floor of 20 %, so an idle box is a quiet line instead of magnified noise.
	let cpuSpark = $derived(areaPaths(cpuHist, Math.max(20, ...cpuHist), 40, 12));

	async function pollSystem() {
		try {
			const s = await api.getSystem();
			system = s;
			if (s.cpu_percent != null) cpuHist = liveHistory.cpu = [...cpuHist.slice(-(SYSTEM_POINTS - 1)), s.cpu_percent];
		} catch {
			// Host metrics are a nicety: keep the last reading, say nothing.
		}
	}

	// The config file the LIVE process was started with, straight from the one
	// config-path state in the status. It is not necessarily the file RouteBox
	// edits — when they differ, the layout banner says so on every page and
	// asks for the restart that is the only cure.
	let processConfigPath = $derived(status.config_paths?.process ?? '');

	// Panel-mode Overview summary (MVP: counts/host/links only; per-user telemetry is Part B).
	let userCount = $state<number | null>(null);
	let publicHost = $state('');

	async function fetchStatus() {
		try {
			// Publishes to the shared store too, so the layout's config-mismatch
			// banner stays fresh off this one poll instead of a second timer.
			status = await refreshStatus();
			if (status.running && !$singboxVersion) {
				// Load sing-box version once
				loadVersion();
			}
		} catch (e) {
			console.error('Failed to fetch status:', e);
		} finally {
			loading = false;
		}
	}

	async function loadPanelOverview() {
		try {
			const [users, settings] = await Promise.all([api.getUsers(), api.getSettings()]);
			userCount = users.length;
			publicHost = settings.settings.server?.public_host ?? '';
		} catch {
			userCount = null;
		}
	}

	async function handleStart() {
		actionLoading = 'start';
		try {
			const result = await api.start();
			notifications.success($t('dashboard.started'));
			if (result.warning) {
				notifications.warning(result.warning);
			}
			await fetchStatus();
			startTrafficStream();
			startConnectionsStream();
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
			notifications.success($t('dashboard.stopped'));
			await fetchStatus();
			stopTrafficStream();
			stopConnectionsStream();
		} catch (e) {
			notifications.error(`Failed to stop: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	async function handleRestart() {
		if (!confirm($t('dashboard.confirmRestart'))) return;
		actionLoading = 'restart';
		try {
			const result = await api.restart();
			notifications.success($t('dashboard.restarted'));
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
			notifications.success($t('dashboard.configReloaded'));
			await fetchStatus();
		} catch (e) {
			notifications.error(`Failed to reload: ${e}`);
		} finally {
			actionLoading = '';
		}
	}

	function startTrafficStream() {
		if (trafficStream) return;
		trafficStream = createTrafficStream((data) => {
			trafficUp = data.up;
			trafficDown = data.down;
			downHist = liveHistory.down = [...downHist.slice(-(TRAFFIC_POINTS - 1)), data.down];
			upHist = liveHistory.up = [...upHist.slice(-(TRAFFIC_POINTS - 1)), data.up];
		});
	}

	function stopTrafficStream() {
		if (trafficStream) {
			trafficStream.close();
			trafficStream = null;
			trafficUp = 0;
			trafficDown = 0;
			// History stays: this also runs on leaving the page (#99).
		}
	}

	function startConnectionsStream() {
		if (connectionsStream) return;
		connectionsStream = createConnectionsStream((data) => {
			uploadTotal = data.uploadTotal ?? 0;
			downloadTotal = data.downloadTotal ?? 0;
			// Get top 5 by download
			// Router mode only: on a panel every client's source IS a public
			// address, and filtering here would leave the ring empty beside a busy
			// graph — the same disagreement #102 removed, mirror-imaged.
			liveConns = $routerMode ? localSourceConnections(data.connections ?? []) : (data.connections ?? []);
			// Same roster as the ring, so the card does not name a source the ring
			// above it just excluded. Copy before sorting: sort mutates.
			topConnections = [...liveConns].sort((a, b) => b.download - a.download).slice(0, 5);
			connectionCount = liveConns.length;
		});
	}

	function stopConnectionsStream() {
		if (connectionsStream) {
			connectionsStream.close();
			connectionsStream = null;
			connectionCount = 0;
			topConnections = [];
			liveConns = [];
		}
	}

	onMount(() => {
		fetchStatus();
		// Poll status every 5 seconds
		const interval = setInterval(fetchStatus, 5000);
		// Host metrics every 2 s while the page is open — CPU is a delta
		// between polls, so the cadence is the sparkline's resolution.
		pollSystem();
		const sysInterval = setInterval(pollSystem, 2000);

		return () => {
			clearInterval(interval);
			clearInterval(sysInterval);
			stopTrafficStream();
			stopConnectionsStream();
		};
	});

	// Start/stop streams when status changes (start/stop fns are idempotent)
	$effect(() => {
		if (status.running) {
			startTrafficStream();
			startConnectionsStream();
		} else {
			stopTrafficStream();
			stopConnectionsStream();
		}
	});

	// Load the panel summary once mode resolves to panel (not in onMount, which
	// runs before refreshMode/loadMode settles the mode).
	$effect(() => {
		if ($panelMode && userCount === null) loadPanelOverview();
	});
</script>

<!-- No page heading: the process card is the headline, and on a 1080p screen
     the heading was what pushed the top connections under the fold (#108). -->
<!-- Desktop: the page is exactly the viewport minus the header and the main
     padding, and only the Top Connections list gives way (scrolls inside).
     Everything else keeps its height; the card cannot go below its fixed
     content plus two connection rows (min-h), so on a too-short screen the
     page scrolls instead of the graph overlapping the links below. -->
<div class="space-y-4 lg:h-[calc(100dvh-6.5rem)] lg:flex lg:flex-col">
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
								<code class="block text-white/90 text-sm">
									sudo ./routebox --config /path/to/config.json
								</code>
							</div>
						{/if}
						{#if !status.system_checks.ipv4_forward}
							<div class="p-3 bg-white/10 rounded-lg">
								<p class="text-white font-medium mb-2">Enable IP forwarding:</p>
								<code class="block text-white/90 text-sm">
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
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 lg:flex lg:flex-col {topConnections.length > 0 ? 'lg:min-h-[39.5rem]' : ''}">
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

		{#if status.running}
			<!-- Control buttons first -->
			<div class="flex gap-3 flex-wrap mb-4">
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
			{#if $singboxVersion || processConfigPath}
				<div class="bg-[var(--ctp-surface1)] rounded-lg px-4 py-2 flex items-center gap-4 flex-wrap mb-3 text-xs">
					{#if $singboxVersion}
						<div class="flex items-center gap-1.5">
							<span class="text-[var(--ctp-overlay1)]">sing-box</span>
							<span class="text-[var(--ctp-subtext1)]">{$singboxVersion.version}</span>
						</div>
					{/if}
					{#if $singboxVersion && processConfigPath}
						<div class="w-px h-[14px] bg-[var(--ctp-surface2)]"></div>
					{/if}
					{#if processConfigPath}
						<div class="flex items-center gap-1.5 min-w-0">
							<span class="text-[var(--ctp-overlay1)] flex-shrink-0">Config</span>
							<span class="text-[var(--ctp-subtext1)] truncate">{processConfigPath}</span>
						</div>
					{/if}
				</div>
			{/if}

			<!-- System metrics bar -->
			<div class="bg-[var(--ctp-surface1)] rounded-lg px-4 py-3 grid grid-cols-2 gap-x-3 gap-y-2.5 sm:flex sm:items-center sm:gap-5 sm:flex-wrap mb-4">
				<div class="min-w-0 flex flex-col sm:flex-row sm:items-baseline sm:gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)] flex-shrink-0">Managed by</span>
					{#if status.managed_by === 'systemd'}
						<span class="text-sm text-[var(--ctp-primary)] truncate">systemd{#if status.service_name}<span class="ml-1 text-[10px] text-[var(--ctp-overlay0)]">({status.service_name})</span>{/if}</span>
					{:else}
						<span class="text-sm text-[var(--ctp-text)]">standalone</span>
					{/if}
				</div>
				<div class="hidden sm:block w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="min-w-0 flex flex-col sm:flex-row sm:items-baseline sm:gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">PID</span>
					<span class="text-sm text-[var(--ctp-text)]">{status.pid || '-'}</span>
				</div>
				<div class="hidden sm:block w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="min-w-0 flex flex-col sm:flex-row sm:items-baseline sm:gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">Uptime</span>
					<span class="text-sm text-[var(--ctp-text)]">{status.uptime || '-'}</span>
				</div>
				<div class="hidden sm:block w-px h-[18px] bg-[var(--ctp-surface2)]"></div>
				<div class="min-w-0 flex flex-col sm:flex-row sm:items-baseline sm:gap-1.5">
					<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay1)]">Connections</span>
					<span class="text-sm text-[var(--ctp-text)]">{connectionCount}</span>
				</div>
			</div>

			<!-- Traffic graph with the host beside it (#99): one graph for both
			     directions with a period switch; CPU keeps a mini graph, memory is a
			     number; totals and disk in the footer. -->
			<div class="bg-[var(--ctp-surface1)] rounded-lg mb-4">
				<div class="flex flex-col sm:flex-row">
					<div class="flex-1 min-w-0 px-4 sm:px-5 pt-4 sm:pt-5">
						<div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
							<div class="flex flex-wrap items-baseline gap-x-5 gap-y-1 min-w-0">
								<div class="flex items-baseline gap-x-1.5 min-w-0">
									<span class="text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]">↓ {$t('dashboard.download')}</span>
									<span class="text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-text)]">{rate.down.value}</span>
									<span class="text-xs text-[var(--ctp-overlay1)]">{rate.down.unit}</span>
								</div>
								<div class="flex items-baseline gap-x-1.5 min-w-0">
									<span class="text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]">↑ {$t('dashboard.upload')}</span>
									<span class="text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-text)]">{rate.up.value}</span>
									<span class="text-xs text-[var(--ctp-overlay1)]">{rate.up.unit}</span>
								</div>
							</div>
							<div class="flex gap-1" role="group" aria-label={periodLabel}>
								{#each PERIODS as p (p)}
									<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs whitespace-nowrap {period === p ? 'selected' : ''}" onclick={() => (period = p)}>{$t(`dashboard.period${p}`)}</button>
								{/each}
							</div>
						</div>
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div class="mt-3" onpointerdown={trackHover} onpointermove={trackHover} onpointerleave={() => (hoverIdx = null)}>
						<svg viewBox="0 0 {GW} {GH}" preserveAspectRatio="none" class="block w-full h-24 sm:h-28" aria-hidden="true">
							{#if downPaths.line || upPaths.line}
								<path d={downPaths.area} fill="var(--ctp-primary)" opacity="0.14" />
								<path d={upPaths.area} fill="var(--ctp-upload)" opacity="0.14" />
								<path d={downPaths.line} fill="none" stroke="var(--ctp-primary)" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
								<path d={upPaths.line} fill="none" stroke="var(--ctp-upload)" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
							{:else}
								<line x1="0" y1={GH - 0.5} x2={GW} y2={GH - 0.5} stroke="var(--ctp-surface2)" stroke-width="1" vector-effect="non-scaling-stroke" />
							{/if}
							{#if hoverIdx != null && graphDown.length > 1}
								{@const x = (hoverIdx / (graphDown.length - 1)) * GW}
								<!-- Quiet cursor: the line being read must stay the loudest thing (#108). -->
								<line x1={x} y1="0" x2={x} y2={GH} stroke="var(--ctp-overlay0)" stroke-opacity="0.45" stroke-width="1" stroke-dasharray="3 3" vector-effect="non-scaling-stroke" />
								<circle cx={x} cy={GH - (Math.min(graphDown[hoverIdx] ?? 0, graphMax) / graphMax) * GH} r="2" fill="var(--ctp-primary)" fill-opacity="0.7" vector-effect="non-scaling-stroke" />
								<circle cx={x} cy={GH - (Math.min(graphUp[hoverIdx] ?? 0, graphMax) / graphMax) * GH} r="2" fill="var(--ctp-upload)" fill-opacity="0.7" vector-effect="non-scaling-stroke" />
							{/if}
						</svg>
					</div>
						<div class="flex flex-wrap gap-x-5 gap-y-1 mt-2 pb-4 sm:pb-5 text-xs text-[var(--ctp-overlay1)]">
							<span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full inline-block" style="background: var(--ctp-primary)"></span>{$t('dashboard.download')} · {trafficNote(graphDown)}</span>
							<span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full inline-block" style="background: var(--ctp-upload)"></span>{$t('dashboard.upload')} · {trafficNote(graphUp)}</span>
							<!-- One slot, always the same width: a pill that appears on hover
							     would otherwise re-wrap this row and push the card down under
							     the very cursor reading it. -->
							<span class="ml-auto flex min-h-[22px] w-[230px] max-w-full items-center justify-end">
								{#if hoverPoint}
									<span class="inline-flex items-baseline gap-2 whitespace-nowrap rounded-full border border-[var(--ctp-surface2)] bg-[var(--ctp-base)] px-2.5 py-0.5 tabular-nums">
										<span class="text-[var(--ctp-overlay1)]">{hoverPoint.time}</span>
										<span class="text-[var(--ctp-primary)]">↓ {hoverPoint.down}</span>
										<span class="text-[var(--ctp-upload)]">↑ {hoverPoint.up}</span>
									</span>
								{:else}
									<span class="truncate text-[var(--ctp-overlay0)]" title={histError}>{period !== '60s' && histError ? $t('dashboard.noHistory') : periodLabel}</span>
								{/if}
							</span>
						</div>
					</div>
					<!-- Wide enough for a chain name next to the ring; the ring itself is
					     centred in what is left under the switch (#108). -->
					<div class="sm:w-80 shrink-0 border-t sm:border-t-0 sm:border-l border-[var(--ctp-surface2)] px-4 sm:px-5 py-4 sm:py-5 flex flex-col gap-3">
						<div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
							<div class="flex gap-1" role="group" aria-label={$t('dashboard.breakdown')}>
								<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs {sideDim === 'source' ? 'selected' : ''}" onclick={() => (sideDim = 'source')}>{$t('dashboard.byClients')}</button>
								<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs {sideDim === 'chain' ? 'selected' : ''}" onclick={() => (sideDim = 'chain')}>{$t('dashboard.byChains')}</button>
							</div>
							<!-- The minute view sums the counters of connections that are open
							     NOW, each since it opened — the Breakdown page calls that Live.
							     Only 1h/24h are the graph's own window. -->
							<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay0)]">{period === '60s' ? $t('dashboard.ringLive') : periodLabel}</span>
						</div>
						<!-- Fixed box: clients and chains rarely have the same number of rows,
						     and without it switching moved everything below (#101). -->
						<div class="min-h-[196px] sm:min-h-[112px] flex-1 flex items-center">
							{#if sideItems.length > 0}
								<PieChart items={sideItems} centerNumber={sideItems.length} topN={4} size={112} />
							{:else}
								<div class="text-xs text-[var(--ctp-overlay0)]">{$t('dashboard.noTrafficYet')}</div>
							{/if}
						</div>
					</div>
				</div>
				<div class="flex flex-wrap gap-x-6 gap-y-1 px-4 sm:px-5 py-3 border-t border-[var(--ctp-surface2)] text-xs text-[var(--ctp-overlay1)]">
					<span>{$t('dashboard.totalDown')} <span class="text-[var(--ctp-text)] tabular-nums">{formatBytes(downloadTotal)}</span></span>
					<span>{$t('dashboard.totalUp')} <span class="text-[var(--ctp-text)] tabular-nums">{formatBytes(uploadTotal)}</span></span>
					{#if system?.disk_total}
						<span>{$t('dashboard.disk')} <span class="text-[var(--ctp-text)] tabular-nums">{$t('dashboard.ofTotal', { values: { used: formatBytes(system.disk_used), total: formatBytes(system.disk_total) } })}</span></span>
					{/if}
					<!-- CPU and memory moved down here from the column the breakdown now
					     occupies (#101). CPU keeps a sparkline: the spike is what it is
					     looked at for, and a bare number never shows one. -->
					<span class="flex items-center gap-1.5">CPU
						<span class="text-[var(--ctp-text)] tabular-nums">{cpuPct == null ? '—' : `${cpuPct} %`}</span>
						{#if cpuSpark.line}
							<svg viewBox="0 0 40 12" class="w-10 h-3 shrink-0" aria-hidden="true">
								<path d={cpuSpark.area} fill="var(--ctp-primary)" opacity="0.14" />
								<path d={cpuSpark.line} fill="none" stroke="var(--ctp-primary)" stroke-width="1" vector-effect="non-scaling-stroke" />
							</svg>
						{/if}
						{#if system}<span>{system.cores} {$t('dashboard.cores')} · {$t('dashboard.load')} {system.load1.toFixed(2)}</span>{/if}
					</span>
					<span>{$t('dashboard.memory')}
						<span class="text-[var(--ctp-text)] tabular-nums">{memPct == null ? '—' : `${memPct} %`}</span>
						{#if system?.mem_total}<span>{$t('dashboard.ofTotal', { values: { used: formatBytes(system.mem_used), total: formatBytes(system.mem_total) } })}</span>{/if}
					</span>
				</div>
			</div>

			<!-- Top Connections Preview -->
			{#if topConnections.length > 0}
				<div class="lg:min-h-0 lg:flex lg:flex-col">
					<div class="flex items-center justify-between mb-2 shrink-0">
						<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Top Connections</h3>
						<a href="/monitor/connections" class="text-sm text-[var(--ctp-primary)] hover:underline">View all</a>
					</div>
					<div class="bg-[var(--ctp-surface1)] rounded-lg divide-y divide-[var(--ctp-surface2)] lg:min-h-[4.75rem] lg:overflow-y-auto">
						{#each topConnections as conn}
							{@const sourceName = $clientNames.get(conn.metadata.sourceIP)}
							<!-- Behind the front every source is the loopback (see the
							     connections monitor), so the column is dropped rather
							     than filled with 127.0.0.1 for every row. -->
							<div class="px-3 sm:px-4 py-2 flex items-center gap-2 sm:gap-4">
								<div class="min-w-0 flex-1 truncate text-sm text-[var(--ctp-text)]">
									{conn.metadata.host || conn.metadata.destinationIP}
								</div>
								{#if !$behindFront}
									<div class="hidden sm:block w-[8rem] text-right text-xs tabular-nums text-[var(--ctp-overlay1)] flex-shrink-0 truncate" title={conn.metadata.sourceIP}>
										{#if sourceName}{sourceName}
										{:else}<span class="font-mono">{conn.metadata.sourceIP}</span>{/if}
									</div>
								{/if}
								<div class="hidden sm:flex items-center justify-end gap-1 w-[13.5rem] flex-shrink-0">
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

	<!-- Panel-mode summary (MVP: users count + public host + quick links) -->
	{#if $panelMode}
		<div class="grid grid-cols-1 sm:grid-cols-3 gap-2 sm:gap-4">
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
				<div class="text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.panelUsers')}</div>
				<div class="text-2xl font-semibold text-[var(--ctp-text)] mt-1">{userCount ?? '-'}</div>
				<a href="/config/users" class="text-sm text-[var(--ctp-primary)] hover:underline">{$t('dashboard.manageUsers')}</a>
			</div>
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
				<div class="text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.publicHost')}</div>
				<div class="text-sm font-mono text-[var(--ctp-text)] mt-1 truncate" title={publicHost || '-'}>{publicHost || $t('dashboard.publicHostUnset')}</div>
				<a href="/config/app" class="text-sm text-[var(--ctp-primary)] hover:underline">{$t('dashboard.editSettings')}</a>
			</div>
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
				<div class="text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.serverInbounds')}</div>
				<div class="text-sm text-[var(--ctp-text)] mt-1">{$t('dashboard.serverInboundsHint')}</div>
				<a href="/config/inbounds" class="text-sm text-[var(--ctp-primary)] hover:underline">{$t('dashboard.manageInbounds')}</a>
			</div>
		</div>
	{/if}

	{#if $routerMode}
	<!-- Quick Links -->
	<div class="grid grid-cols-1 sm:grid-cols-3 gap-2 sm:gap-4">
		<a
			href="/config/endpoints"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex items-center gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
					</svg>
				</div>
				<div class="min-w-0 text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Endpoints
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate">AWG, WireGuard</p>
				</div>
			</div>
		</a>

		<a
			href="/config/outbounds"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex items-center gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
					</svg>
				</div>
				<div class="min-w-0 text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Outbounds
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate">VLESS, Hysteria2, NaiveProxy, Mieru</p>
				</div>
			</div>
		</a>

		<a
			href="/config/routes"
			class="bg-[var(--ctp-surface0)] rounded-xl p-3 sm:p-4 hover:bg-[var(--ctp-surface1)] transition-colors group"
		>
			<div class="flex items-center gap-3">
				<div class="p-2 sm:p-2.5 bg-[var(--ctp-surface2)] rounded-lg flex-shrink-0">
					<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
					</svg>
				</div>
				<div class="min-w-0 text-left">
					<h3 class="text-sm sm:text-base font-semibold text-[var(--ctp-text)] group-hover:text-[var(--ctp-primary)] transition-colors">
						Routes
					</h3>
					<p class="text-xs text-[var(--ctp-overlay1)] truncate">Rules & rule sets</p>
				</div>
			</div>
		</a>
	</div>
	{/if}
</div>

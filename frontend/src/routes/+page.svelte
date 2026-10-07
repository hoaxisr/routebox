<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api, createTrafficStream, createConnectionsStream } from '$lib/api/client';
	import { notifications, formatBytes, formatSpeed, clientNames, panelMode, routerMode, behindFront, refreshStatus } from '$lib/stores';
	import { singboxVersion, loadVersion } from '$lib/stores/version';
	import PendingChanges from '$lib/components/shared/PendingChanges.svelte';
	import { splitUnit, areaPaths } from '$lib/utils/sparkline';
	import { seriesRates } from '$lib/utils/trafficSeries';
	import { directTags, leafRates, liveSplit, bucketFlows, localFlows, rankLeaves, rankClients, type RouteFlow } from '$lib/utils/routeSplit';
	import { liveHistory, type DashboardPeriod } from '$lib/stores/liveHistory';
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
	// Every live connection the list below picks its top from. Filtered the same
	// way the Breakdown page filters its live view: a remote source is not a
	// device of this box (#102).
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
	// Route graph (#110): download through a direct outbound vs through a proxy
	// or endpoint. Which tags are direct comes from the config, once per visit.
	let directSet = $state(directTags(undefined));
	let directHist = $state<number[]>(liveHistory.direct);
	let proxyHist = $state<number[]>(liveHistory.proxy);
	let flowHist = $state<RouteFlow[][]>(liveHistory.flows);
	// Each connection's download counter at the previous tick. The first tick
	// after opening only fills it: every open connection would otherwise count
	// its whole lifetime as one second.
	const splitSeen = new Map<string, number>();
	let splitPrimed = false;
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
	// the rankings beside the graph need, so 1h/24h costs no extra request.
	let histBuckets = $state<TrafficBucket[]>([]);
	let histDirect = $state<number[]>([]);
	let histProxy = $state<number[]>([]);
	async function loadHistory(p: '1h' | '24h') {
		try {
			const r = await api.getTrafficHistory(p, { series: true });
			({ down: histDown, up: histUp } = seriesRates(r.series, r.start_ts, r.end_ts, r.step ?? 60, 240));
			({ direct: histDirect, proxy: histProxy } = leafRates(r.leaves, directSet, r.start_ts, r.end_ts, r.step ?? 60, 240));
			histBuckets = r.buckets ?? [];
			histError = '';
		} catch (e) {
			// 503 = no traffic store (Clash API address unset); anything else is
			// still "no graph", and the message says which.
			histError = String(e);
			histDown = [];
			histUp = [];
			histBuckets = [];
			histDirect = [];
			histProxy = [];
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
	const GW = 600;
	let splitDirect = $derived(period === '60s' ? directHist : histDirect);
	let splitProxy = $derived(period === '60s' ? proxyHist : histProxy);
	const pctOf = (d: number, p: number) => (d + p > 0 ? Math.round((d / (d + p)) * 100) : null);
	// Volume over the shown period, not a speed: "how much went where" is what
	// the share is read for. A point is one second live and an even slice of
	// the window for 1h/24h (leafRates resamples to at most 240 points).
	let secsPerPoint = $derived(period === '60s' ? 1 : (period === '1h' ? 3600 : 86400) / Math.max(1, splitDirect.length));
	let splitBytes = $derived({
		direct: splitDirect.reduce((a, b) => a + b, 0) * secsPerPoint,
		proxy: splitProxy.reduce((a, b) => a + b, 0) * secsPerPoint
	});
	let periodPct = $derived(pctOf(splitBytes.direct, splitBytes.proxy));
	// Where the download went, over the period the graph shows: the live
	// minute's ticks, or the history buckets for 1h/24h. Remote sources stay in
	// the share and the exits but are not devices of this box, so they are not
	// listed as clients (#102).
	let flows = $derived(period === '60s' ? flowHist.flat() : bucketFlows(histBuckets));
	let exitRank = $derived(rankLeaves(flows, directSet, 5));
	let exitTotal = $derived(exitRank.items.reduce((s, x) => s + x.bytes, 0) + exitRank.rest.bytes);
	let clientRank = $derived(rankClients($routerMode ? localFlows(flows) : flows, directSet, 5));
	let clientMax = $derived(clientRank.items.length ? clientRank.items[0].direct + clientRank.items[0].proxy : 0);
	const pctOfTotal = (x: number, total: number) => (total > 0 ? Math.round((x / total) * 100) : 0);
	// Mirror graph: download above the axis, upload below it on its own scale —
	// upload is a fraction of download and on one scale it lies flat on the
	// axis. The scale labels at the right edge say the two halves differ.
	const DH = 64;
	const UH = 28;
	let downMax = $derived(Math.max(1024, ...graphDown) * 1.15);
	let upMax = $derived(Math.max(1024, ...graphUp) * 1.15);
	let downPaths = $derived(areaPaths(graphDown, downMax, GW, DH));
	let upPaths = $derived(areaPaths(graphUp, upMax, GW, UH));
	let periodLabel = $derived(
		period === '60s' ? $t('dashboard.lastMinute') : period === '1h' ? $t('dashboard.lastHour') : $t('dashboard.lastDay')
	);

	// Hovering the graph reads out the moment under the cursor. The readout sits
	// in the fixed slot at the end of the legend row, where the period label
	// otherwise is: numbers that chase the cursor cover the very line being read.
	// The cursor is kept as a fraction of the width and turned into a point of
	// whatever series is shown, so both halves of the mirror read one moment.
	let hoverRatio = $state<number | null>(null);
	const idxAt = (n: number) => (hoverRatio == null || n < 2 ? null : Math.round(hoverRatio * (n - 1)));
	let hoverIdx = $derived(idxAt(graphDown.length));
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
		if (!box.width) return;
		hoverRatio = Math.min(Math.max((ev.clientX - box.left) / box.width, 0), 1);
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
			// Every connection, unfiltered: the route graph splits the same total
			// the speed graph beside it draws.
			const split = liveSplit(data.connections ?? [], directSet, splitSeen);
			if (splitPrimed) {
				directHist = liveHistory.direct = [...directHist.slice(-(TRAFFIC_POINTS - 1)), split.direct];
				proxyHist = liveHistory.proxy = [...proxyHist.slice(-(TRAFFIC_POINTS - 1)), split.proxy];
				flowHist = liveHistory.flows = [...flowHist.slice(-(TRAFFIC_POINTS - 1)), split.flows];
			}
			splitPrimed = true;
			downloadTotal = data.downloadTotal ?? 0;
			// Get top 5 by download
			// Router mode only: on a panel every client's source IS a public
			// address, and filtering here would leave the list empty beside a busy
			// graph — the same disagreement #102 removed, mirror-imaged.
			liveConns = $routerMode ? localSourceConnections(data.connections ?? []) : (data.connections ?? []);
			// Same roster as the client ranking beside it, so the list does not
			// name a source the ranking just excluded. Copy before sorting: sort
			// mutates.
			topConnections = [...liveConns].sort((a, b) => b.download - a.download).slice(0, 5);
			connectionCount = liveConns.length;
		});
	}

	function stopConnectionsStream() {
		if (connectionsStream) {
			connectionsStream.close();
			connectionsStream = null;
			splitSeen.clear();
			splitPrimed = false;
			connectionCount = 0;
			topConnections = [];
			liveConns = [];
		}
	}

	async function loadDirectTags() {
		try {
			const cfg = await api.getConfig();
			directSet = directTags(cfg.outbounds);
			// History fetched before the config arrived was split with the default.
			if (period !== '60s') loadHistory(period);
		} catch {
			// Without the config only sing-box's implicit "direct" counts as direct.
		}
	}

	onMount(() => {
		fetchStatus();
		loadDirectTags();
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
     content plus the bottom card, which never shrinks under its own minimum
     (#110), so on a too-short screen the page scrolls instead of the graph
     overlapping the links below. -->
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
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 lg:flex lg:flex-col">
		<!-- One header row (#B): name and state, what is running and how, the
		     controls. The version, config and metrics bars it replaces were three
		     grey strips saying one thing. -->
		<div class="flex flex-wrap items-center gap-x-6 gap-y-3 mb-4">
			<div class="flex items-center gap-3">
				<h2 class="text-lg font-semibold text-[var(--ctp-text)]">amnezia-box</h2>
				{#if loading}
					<span class="text-[var(--ctp-overlay1)]">{$t('common.loading')}</span>
				{:else}
					<span class="px-3 py-0.5 rounded-full text-xs font-semibold text-white" class:bg-[var(--ctp-green)]={status.running} class:bg-[var(--ctp-red)]={!status.running}>
						{status.running ? $t('status.running') : $t('status.stopped')}
					</span>
				{/if}
			</div>
			{#if status.running}
				<div class="flex flex-1 basis-[26rem] min-w-0 flex-wrap items-center gap-x-3.5 gap-y-1.5 text-[13px] text-[var(--ctp-overlay1)]">
					{#if $singboxVersion}
						<!-- The config path rides on the version as a tooltip: spelled out
						     it was what broke the header onto a second row at 1366 px. -->
						<span title={processConfigPath || undefined}>sing-box <span class="text-[var(--ctp-text)]">{$singboxVersion.version}</span></span>
						<span class="w-px h-3.5 bg-[var(--ctp-surface2)]"></span>
					{/if}
					{#if status.managed_by === 'systemd'}
						<span>systemd <span class="text-[var(--ctp-text)]">{status.service_name || ''}</span></span>
					{:else}
						<span class="text-[var(--ctp-text)]">standalone</span>
					{/if}
					<span class="w-px h-3.5 bg-[var(--ctp-surface2)]"></span>
					<span>PID <span class="tabular-nums text-[var(--ctp-text)]">{status.pid || '-'}</span></span>
					<span class="w-px h-3.5 bg-[var(--ctp-surface2)]"></span>
					<span>Uptime <span class="tabular-nums text-[var(--ctp-text)]">{status.uptime || '-'}</span></span>
					<span class="w-px h-3.5 bg-[var(--ctp-surface2)]"></span>
					<span>Connections <span class="tabular-nums text-[var(--ctp-text)]">{connectionCount}</span></span>
				</div>
				<div class="flex gap-2 w-full sm:w-auto">
					<button onclick={handleReload} disabled={actionLoading !== ''} title="Hot reload configuration (SIGHUP)" class="flex-1 sm:flex-none min-h-11 sm:min-h-0 px-3.5 py-2 bg-[var(--ctp-primary)] text-white rounded-lg text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity">
						{actionLoading === 'reload' ? 'Reloading...' : 'Reload Config'}
					</button>
					<button onclick={handleRestart} disabled={actionLoading !== ''} title="Full process restart" class="flex-1 sm:flex-none min-h-11 sm:min-h-0 px-3.5 py-2 bg-[var(--ctp-surface2)] text-[var(--ctp-text)] rounded-lg text-sm font-medium hover:bg-[var(--ctp-overlay0)] disabled:opacity-50 transition-colors">
						{actionLoading === 'restart' ? $t('common.restarting') : $t('dashboard.restart')}
					</button>
					<button onclick={handleStop} disabled={actionLoading !== ''} class="flex-1 sm:flex-none min-h-11 sm:min-h-0 px-3.5 py-2 border border-[var(--ctp-red)]/40 text-[var(--ctp-red)] rounded-lg text-sm font-medium hover:bg-[var(--ctp-red)]/10 disabled:opacity-50 transition-colors">
						{actionLoading === 'stop' ? $t('common.stopping') : $t('dashboard.stop')}
					</button>
				</div>
			{/if}
		</div>

		{#if status.running}
			<!-- Traffic graph with the host beside it (#99): one graph for both
			     directions with a period switch; CPU keeps a mini graph, memory is a
			     number; totals and disk in the footer. -->
			<!-- Both columns share one vertical rhythm — a 52px header (label over
			     number), the graph at the same height, then two fixed rows — so
			     their headers, graphs and captions line up (#110). -->
			<!-- The same 2fr/1fr grid as the bottom card, so the two dividers are
			     one vertical line by construction, not by flex coincidence. -->
			<div class="bg-[var(--ctp-surface1)] rounded-lg mb-4">
				<div class="flex flex-col sm:grid sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
					<div class="min-w-0 px-4 sm:px-5 pt-4 sm:pt-5 pb-3">
						<div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
							<!-- Label over number in fixed boxes: in a free-flowing row a longer
							     number pushed Upload onto its own line and the graph jumped (#110). -->
							<div class="grid grid-cols-2 gap-x-6 w-full sm:w-auto min-w-0">
								<div class="min-w-0 whitespace-nowrap">
									<div class="h-4 flex items-center gap-1.5 text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]"><span class="w-2 h-2 rounded-full" style="background: var(--ctp-text)"></span>↓ {$t('dashboard.download')}</div>
									<div class="mt-1 h-8 flex items-end"><span><span class="text-[22px] sm:text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-text)]">{rate.down.value}</span> <span class="text-xs text-[var(--ctp-overlay1)]">{rate.down.unit}</span></span></div>
								</div>
								<div class="min-w-0 whitespace-nowrap">
									<div class="h-4 flex items-center gap-1.5 text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]"><span class="w-2 h-2 rounded-full" style="background: var(--ctp-overlay0)"></span>↑ {$t('dashboard.upload')}</div>
									<div class="mt-1 h-8 flex items-end"><span><span class="text-[22px] sm:text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-text)]">{rate.up.value}</span> <span class="text-xs text-[var(--ctp-overlay1)]">{rate.up.unit}</span></span></div>
								</div>
							</div>
							<div class="flex gap-1" role="group" aria-label={periodLabel}>
								{#each PERIODS as p (p)}
									<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs whitespace-nowrap {period === p ? 'selected' : ''}" onclick={() => (period = p)}>{$t(`dashboard.period${p}`)}</button>
								{/each}
							</div>
						</div>
						<!-- The scale labels sit in their own rows above and below the plot,
						     right-aligned, so no curve ever runs into them: the download scale
						     alone on top, the upload scale sharing the legend row. The rows
						     span the full width like the svgs, so the hover ratio is unchanged. -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div class="mt-3" onpointerdown={trackHover} onpointermove={trackHover} onpointerleave={() => (hoverRatio = null)}>
							<div class="h-4 flex justify-end text-[10px] tabular-nums text-[var(--ctp-overlay1)]">↓ {formatSpeed(downMax)}</div>
							<svg viewBox="0 0 {GW} {DH}" preserveAspectRatio="none" class="block w-full h-[4.5rem] sm:h-20" aria-hidden="true">
								<line x1="0" y1={DH / 2} x2={GW} y2={DH / 2} stroke="var(--ctp-surface2)" stroke-dasharray="3 4" vector-effect="non-scaling-stroke" />
								{#if downPaths.line}
									<path d={downPaths.area} fill="var(--ctp-text)" fill-opacity="0.10" />
									<path d={downPaths.line} fill="none" stroke="var(--ctp-text)" stroke-width="2" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
								{/if}
								{#if hoverIdx != null && graphDown.length > 1}
									{@const x = (hoverIdx / (graphDown.length - 1)) * GW}
									<line x1={x} y1="0" x2={x} y2={DH} stroke="var(--ctp-overlay0)" stroke-opacity="0.45" stroke-width="1" stroke-dasharray="3 3" vector-effect="non-scaling-stroke" />
								{/if}
							</svg>
							<div class="h-px bg-[var(--ctp-overlay1)] opacity-60"></div>
							<!-- Drawn top-down like the half above, then flipped: the area closes
							     on the axis and grows downward. -->
							<svg viewBox="0 0 {GW} {UH}" preserveAspectRatio="none" class="block w-full h-8 sm:h-9" style="transform: scaleY(-1)" aria-hidden="true">
								{#if upPaths.line}
									<path d={upPaths.area} fill="var(--ctp-overlay0)" fill-opacity="0.75" />
									<path d={upPaths.line} fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.5" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
								{/if}
								{#if hoverIdx != null && graphDown.length > 1}
									{@const x = (hoverIdx / (graphDown.length - 1)) * GW}
									<line x1={x} y1="0" x2={x} y2={UH} stroke="var(--ctp-overlay0)" stroke-opacity="0.45" stroke-width="1" stroke-dasharray="3 3" vector-effect="non-scaling-stroke" />
								{/if}
							</svg>
							<!-- One line, never wrapping: the legend on the left, the upload
							     scale on the right. A wrapped legend made this column taller
							     than the one beside it. -->
							<div class="mt-1 h-5 flex items-center justify-between gap-3 whitespace-nowrap text-xs text-[var(--ctp-overlay1)]">
								<span class="truncate">↓ {trafficNote(graphDown)}</span>
								<span class="shrink-0 text-[10px] tabular-nums">↑ {formatSpeed(upMax)}</span>
							</div>
						</div>
						<!-- One slot of fixed height: the hover pill replaces the period label
						     instead of re-wrapping anything under the cursor reading it. -->
						<div class="mt-1 h-[22px] flex items-center justify-end text-xs">
							{#if hoverPoint}
								<span class="inline-flex items-baseline gap-2 whitespace-nowrap rounded-full border border-[var(--ctp-surface2)] bg-[var(--ctp-base)] px-2.5 py-0.5 tabular-nums">
									<span class="text-[var(--ctp-overlay1)]">{hoverPoint.time}</span>
									<span class="text-[var(--ctp-text)]">↓ {hoverPoint.down}</span>
									<span class="text-[var(--ctp-overlay1)]">↑ {hoverPoint.up}</span>
								</span>
							{:else}
								<span class="truncate text-[var(--ctp-overlay0)]" title={histError}>{period !== '60s' && histError ? $t('dashboard.noHistory') : periodLabel}</span>
							{/if}
						</div>
					</div>
					<!-- Where the download goes (#B): the period's direct/proxy share as two
					     numbers and a bar, then the exits ranked by bytes. Same period as the
					     speed graph; download only. -->
					<div class="min-w-0 border-t sm:border-t-0 sm:border-l border-[var(--ctp-surface2)] px-4 sm:px-5 pt-4 sm:pt-5 pb-3">
						<div class="flex items-center justify-between gap-2 text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]">
							<span class="truncate">{$t('dashboard.whereDownload')}</span>
							<span class="shrink-0">{periodLabel}</span>
						</div>
						{#if periodPct != null}
							<div class="mt-2.5 flex items-baseline gap-5 whitespace-nowrap">
								<span><span class="text-[22px] xl:text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-upload)]">{periodPct}</span> <span class="text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.pctDirect')}</span></span>
								<span><span class="text-[22px] xl:text-[28px] leading-none font-semibold tabular-nums text-[var(--ctp-primary)]">{100 - periodPct}</span> <span class="text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.pctProxy')}</span></span>
							</div>
							<div class="mt-3 flex h-2 w-full rounded-full overflow-hidden bg-[var(--ctp-surface2)]">
								<div style="width: {periodPct}%; background: var(--ctp-upload)"></div>
								<div style="width: {100 - periodPct}%; background: var(--ctp-primary)"></div>
							</div>
							<div class="mt-5 text-xs uppercase tracking-wide text-[var(--ctp-overlay1)]">{$t('dashboard.byExit')}</div>
							<div class="mt-2.5 flex flex-col gap-2.5">
								{#each exitRank.items as e (e.leaf)}
									<div>
										<div class="flex items-center justify-between gap-2 text-[13px]">
											<span class="selection-chip truncate" class:route-direct={e.direct}>{e.leaf}</span>
											<span class="shrink-0 tabular-nums text-[var(--ctp-subtext1)]">{formatBytes(e.bytes)} · {pctOfTotal(e.bytes, exitTotal)} %</span>
										</div>
										<div class="mt-1 h-1.5 rounded-full bg-[var(--ctp-surface2)] overflow-hidden">
											<div class="h-full rounded-full" style="width: {Math.max(1.5, pctOfTotal(e.bytes, exitTotal))}%; background: var({e.direct ? '--ctp-upload' : '--ctp-primary'})"></div>
										</div>
									</div>
								{/each}
								{#if exitRank.rest.count > 0}
									<div class="text-xs tabular-nums text-[var(--ctp-overlay1)]">{$t('dashboard.moreItems', { values: { count: exitRank.rest.count } })} · {formatBytes(exitRank.rest.bytes)}</div>
								{/if}
							</div>
						{:else}
							<div class="mt-3 text-xs text-[var(--ctp-overlay0)]" title={histError}>{period !== '60s' && histError ? $t('dashboard.noHistory') : $t('dashboard.noTrafficYet')}</div>
						{/if}
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

			<!-- Same columns as the traffic card above (#B): one card, one divider,
			     so the left and right edges line up instead of two cards and a gap.
			     Desktop: the card takes what the page has left and the list scrolls
			     (the single row is minmax(0,1fr), so it may shrink below content). -->
			<div class="bg-[var(--ctp-surface1)] rounded-lg flex flex-col sm:grid sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] lg:grid-rows-[minmax(0,1fr)] lg:flex-1 lg:min-h-[9.75rem]">
				<div class="min-w-0 flex flex-col lg:min-h-0 pt-3 sm:pt-4 pb-1">
					<div class="h-7 mb-1 px-4 sm:px-5 flex items-center justify-between shrink-0">
						<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">Top Connections</h3>
						<a href="/monitor/connections" class="text-sm text-[var(--ctp-primary)] hover:underline">View all</a>
					</div>
					<div class="flex-1 divide-y divide-[var(--ctp-surface2)] border-t border-[var(--ctp-surface2)] overflow-x-hidden lg:min-h-0 lg:overflow-y-auto">
						{#each topConnections as conn}
							{@const sourceName = $clientNames.get(conn.metadata.sourceIP)}
							<!-- Behind the front every source is the loopback (see the
							     connections monitor), so the column is dropped rather
							     than filled with 127.0.0.1 for every row. -->
							<div class="px-3 sm:px-4 py-2 flex items-center gap-2 sm:gap-4">
								<div class="min-w-[6rem] flex-1 truncate text-sm text-[var(--ctp-text)]">
									{conn.metadata.host || conn.metadata.destinationIP}
								</div>
								{#if !$behindFront}
									<div class="hidden xl:block w-[8rem] text-right text-xs tabular-nums text-[var(--ctp-overlay1)] flex-shrink-0 truncate" title={conn.metadata.sourceIP}>
										{#if sourceName}{sourceName}
										{:else}<span class="font-mono">{conn.metadata.sourceIP}</span>{/if}
									</div>
								{/if}
								<!-- The list is two thirds wide since #110: below xl the source column
								     goes and the chain column gives way before the domain does. -->
								<div class="hidden md:flex items-center justify-end gap-1 min-w-0 max-w-[13.5rem] overflow-hidden xl:w-[13.5rem] xl:flex-shrink-0">
									{#each conn.chains as chain}
										<span class="selection-chip">{chain}</span>
									{/each}
								</div>
								<div class="w-[5.5rem] text-right text-xs sm:text-sm font-mono text-[var(--ctp-subtext1)] tabular-nums flex-shrink-0">
									{formatBytes(conn.download)}
								</div>
							</div>
						{:else}
							<div class="px-4 sm:px-5 py-3 text-xs text-[var(--ctp-overlay0)]">{$t('dashboard.noTrafficYet')}</div>
						{/each}
					</div>
				</div>
				<div class="min-w-0 border-t sm:border-t-0 sm:border-l border-[var(--ctp-surface2)] px-4 sm:px-5 pt-3 sm:pt-4 pb-3 lg:min-h-0 lg:overflow-y-auto">
					<div class="h-7 flex items-center justify-between gap-2">
						<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('dashboard.byClients')}</h3>
						<span class="text-[10px] uppercase tracking-wide text-[var(--ctp-overlay0)] truncate">↓ {periodLabel}</span>
					</div>
					{#if $behindFront}
						<div class="mt-2 text-xs text-[var(--ctp-overlay1)]">{$t('dashboard.clientsBehindFront')}</div>
					{:else if clientRank.items.length === 0}
						<div class="mt-2 text-xs text-[var(--ctp-overlay0)]">{$t('dashboard.noTrafficYet')}</div>
					{:else}
						<div class="mt-1 mb-3 flex gap-3.5 text-[11px] text-[var(--ctp-overlay1)]">
							<span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full" style="background: var(--ctp-upload)"></span>{$t('dashboard.legendDirect')}</span>
							<span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full" style="background: var(--ctp-primary)"></span>{$t('dashboard.legendProxy')}</span>
						</div>
						<div class="flex flex-col gap-3">
							{#each clientRank.items as c (c.source)}
								{@const total = c.direct + c.proxy}
								{@const proxyPct = pctOfTotal(c.proxy, total)}
								<div>
									<div class="flex items-center justify-between gap-2 text-[13px]">
										<span class="min-w-0 truncate text-[var(--ctp-text)]">{c.source ? ($clientNames.get(c.source) ?? c.source) : 'unknown'}</span>
										<span class="shrink-0 tabular-nums text-[var(--ctp-subtext1)]">{formatBytes(total)}</span>
									</div>
									<div class="mt-1 h-1.5 rounded-full bg-[var(--ctp-surface2)]">
										<div class="flex h-full rounded-full overflow-hidden" style="width: {Math.max(1.5, (total / clientMax) * 100)}%">
											<div style="width: {100 - proxyPct}%; background: var(--ctp-upload)"></div>
											<div style="width: {proxyPct}%; background: var(--ctp-primary)"></div>
										</div>
									</div>
									<div class="mt-0.5 text-[11px] tabular-nums text-[var(--ctp-overlay1)]">
										{c.proxy === 0 ? $t('dashboard.allDirect') : c.direct === 0 ? $t('dashboard.allProxy') : $t('dashboard.proxyShare', { values: { pct: proxyPct } })}
									</div>
								</div>
							{/each}
							{#if clientRank.rest.count > 0}
								<div class="text-xs tabular-nums text-[var(--ctp-overlay1)]">{$t('dashboard.moreItems', { values: { count: clientRank.rest.count } })} · {formatBytes(clientRank.rest.bytes)}</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>
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

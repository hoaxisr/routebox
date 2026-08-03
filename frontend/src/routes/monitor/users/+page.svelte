<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, formatBytes } from '$lib/stores';
	import type { PanelUser, UserTrafficResponse, UserTrafficPoint, TrafficRange } from '$lib/types';
	import { peersToRows, mtprotoClientsToRows, mergeMonitorRows, type MonitorRow } from '$lib/utils/monitorRows';

	// AWG peers share this list (#40) but not the accounting source; the row
	// shape and the peer mapping live in monitorRows.ts so they can be tested.
	type Row = MonitorRow;

	// UI range → API range. Bars/series are scoped to the selected window.
	const ranges: { key: string; label: string; api: TrafficRange }[] = [
		{ key: '24h', label: '24h', api: '24h' },
		{ key: '7d', label: '7d', api: 'week' },
		{ key: '30d', label: '30d', api: 'month' }
	];
	let range = $state('24h');
	let rows = $state<Row[]>([]);
	let open = $state<Record<string, boolean>>({});
	let loading = $state(true);

	const nowSec = () => Math.floor(Date.now() / 1000);
	// "active" = the user's most recent per-minute bucket is within the last two
	// minutes (they were transferring just now). sing-box exposes no handshake, so
	// this is an activity signal, not a tunnel-online one.
	const isActive = (h: UserTrafficPoint[]) =>
		h.length > 0 && nowSec() - h[h.length - 1].ts < 120;

	let maxTotal = $derived(Math.max(1, ...rows.map((r) => r.total)));
	let grandTotal = $derived(rows.reduce((s, r) => s + r.total, 0));

	async function load(uiRange: string) {
		loading = true;
		const apiRange = ranges.find((r) => r.key === uiRange)?.api ?? '24h';
		try {
			const users = (await api.getUsers()).filter((u: PanelUser) => !u.pending && u.id);
			const built = await Promise.all(
				users.map(async (u): Promise<Row> => {
					let up = 0,
						down = 0,
						series: UserTrafficPoint[] = [];
					try {
						const r: UserTrafficResponse = await api.getUserTraffic(u.id!, apiRange);
						up = r.upload;
						down = r.download;
						series = r.history;
					} catch {
						/* fork without v2ray_api → zeros */
					}
					return { id: u.id!, name: u.name, upload: up, download: down, total: up + down, series, active: isActive(series), kind: 'user' as const };
				})
			);
			// AWG peers, same shape. Absent (no AWG server, older backend) is not an
			// error — the page is still a per-user view without them.
			let peerRows: Row[] = [];
			try {
				peerRows = peersToRows(await api.getAwgPeersTraffic(apiRange));
			} catch {
				/* AWG unavailable → panel users only */
			}
			// Telegram proxy clients, same shape again. Absent in router mode and
			// on older backends, which is not an error either.
			let mtprotoRows: Row[] = [];
			try {
				mtprotoRows = mtprotoClientsToRows(await api.getMtprotoClientsTraffic(apiRange));
			} catch {
				/* Telegram proxy unavailable → the other two sources still render */
			}
			rows = mergeMonitorRows(built, peerRows, mtprotoRows);
		} catch (e) {
			notifications.error(`${$t('monitor.usersLoadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	function setRange(k: string) {
		if (k === range) return;
		range = k;
		load(k);
	}

	// Downsample a per-minute series to N combined (up+down) columns for the chart.
	function downsample(h: UserTrafficPoint[], n = 60): number[] {
		if (h.length === 0) return [];
		if (h.length <= n) return h.map((p) => p.upload + p.download);
		const first = h[0].ts,
			span = h[h.length - 1].ts - first || 1;
		const cols = new Array(n).fill(0);
		for (const p of h) {
			const i = Math.min(n - 1, Math.floor(((p.ts - first) / span) * n));
			cols[i] += p.upload + p.download;
		}
		return cols;
	}

	// SVG area + line paths from a value series (100×40 viewBox).
	function chartPaths(vals: number[]) {
		const W = 100,
			H = 40,
			n = vals.length,
			max = Math.max(1, ...vals);
		const x = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * W);
		const y = (v: number) => H - (v / max) * (H - 4) - 2;
		let area = `M0 ${H} `,
			line = '';
		vals.forEach((v, i) => {
			area += `L${x(i).toFixed(1)} ${y(v).toFixed(1)} `;
			line += `${i ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)} `;
		});
		area += `L${W} ${H} Z`;
		return { area, line, ex: x(n - 1).toFixed(1), ey: y(vals[n - 1] ?? 0).toFixed(1) };
	}

	// Hour label of the peak bucket, e.g. "20:00" ('' if no data).
	function peakLabel(h: UserTrafficPoint[]): string {
		if (h.length === 0) return '';
		let best = h[0];
		for (const p of h) if (p.upload + p.download > best.upload + best.download) best = p;
		const d = new Date(best.ts * 1000);
		return `${String(d.getHours()).padStart(2, '0')}:00`;
	}

	onMount(() => load(range));
</script>

<svelte:head><title>{$t('monitor.perUserTitle')} - RouteBox</title></svelte:head>

<div class="wrap">
	<div class="head">
		<h1>{$t('monitor.perUserTitle')}</h1>
		<div class="seg" role="group" aria-label="Time range">
			{#each ranges as r}
				<button type="button" class:on={range === r.key} onclick={() => setRange(r.key)}>{r.label}</button>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="muted">{$t('common.loading')}</div>
	{:else if rows.length === 0}
		<div class="empty">{$t('monitor.usersEmpty')}</div>
	{:else}
		<p class="sub">{$t('monitor.usageSummary', { values: { count: rows.length, total: formatBytes(grandTotal) } })}</p>

		<div class="card">
			<div class="legend">
				<span class="k"><span class="sw up"></span>↑ {$t('users.upload')}</span>
				<span class="k"><span class="sw dl"></span>↓ {$t('users.download')}</span>
				<span class="spacer"></span>
				<span class="k"><span class="dot"></span>{$t('monitor.usageActive')}</span>
			</div>

			{#each rows as r (r.id)}
				{@const paths = chartPaths(downsample(r.series))}
				{@const peak = peakLabel(r.series)}
				<div class="row" class:open={open[r.id]}>
					<button class="rowhead" aria-expanded={!!open[r.id]} onclick={() => (open[r.id] = !open[r.id])}>
						<span class="name">
							<svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5l7 7-7 7" /></svg>
							<span class="dot" class:idle={!r.active} title={r.active ? $t('monitor.usageActive') : ''}></span>
							{r.name}
							{#if r.kind === 'peer'}<span class="kind">{$t('monitor.usageAwgPeer')}</span>{:else if r.kind === 'mtproto'}<span class="kind">{$t('monitor.usageMtprotoClient')}</span>{/if}
						</span>
						<span class="bar" style="width:{(r.total / maxTotal) * 100}%">
							{#if r.total > 0}
								<span class="up" style="width:{(r.upload / r.total) * 100}%"></span>
								<span class="dl" style="left:{(r.upload / r.total) * 100}%"></span>
							{/if}
						</span>
						<span class="total tabnum">{formatBytes(r.total)}<small>↑{formatBytes(r.upload)} ↓{formatBytes(r.download)}</small></span>
					</button>
					<div class="detail">
						<div class="detail-in">
							{#if r.total > 0}
								<div class="chart">
									<svg viewBox="0 0 100 40" preserveAspectRatio="none">
										<line x1="0" y1="13.2" x2="100" y2="13.2" stroke="var(--ctp-surface2)" stroke-width=".4" opacity="0.5" />
										<line x1="0" y1="26.4" x2="100" y2="26.4" stroke="var(--ctp-surface2)" stroke-width=".4" opacity="0.5" />
										<path d={paths.area} fill="var(--ctp-primary)" opacity="0.18" />
										<path d={paths.line} fill="none" stroke="var(--ctp-primary)" stroke-width="1.2" vector-effect="non-scaling-stroke" />
										<circle cx={paths.ex} cy={paths.ey} r="1.6" fill="var(--ctp-primary)" />
									</svg>
								</div>
								<div class="stats">
									<div class="stat"><span class="l">{$t('users.upload')}</span><span class="v up tabnum">{formatBytes(r.upload)}</span></div>
									<div class="stat"><span class="l">{$t('users.download')}</span><span class="v dl tabnum">{formatBytes(r.download)}</span></div>
									{#if peak}<span class="peak">{$t('monitor.usagePeak', { values: { time: peak } })}</span>{/if}
								</div>
							{:else}
								<div class="nodata">{$t('monitor.usageNoData')}</div>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.wrap {
		max-width: 56rem;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.tabnum {
		font-variant-numeric: tabular-nums;
	}
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
	}
	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		margin: 0;
		color: var(--ctp-text);
		letter-spacing: -0.01em;
	}
	.sub {
		color: var(--ctp-overlay1);
		font-size: 0.85rem;
		margin: 0 0 1rem;
	}
	.muted {
		color: var(--ctp-overlay0);
	}
	.empty {
		background: var(--ctp-surface0);
		border-radius: 0.75rem;
		padding: 2rem;
		text-align: center;
		color: var(--ctp-overlay1);
	}
	.seg {
		display: inline-flex;
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.5rem;
		padding: 2px;
	}
	.seg button {
		border: 0;
		background: transparent;
		color: var(--ctp-overlay1);
		font: inherit;
		font-size: 0.8rem;
		padding: 0.3rem 0.7rem;
		border-radius: 0.375rem;
		cursor: pointer;
	}
	.seg button.on {
		background: var(--ctp-primary);
		color: #fff;
	}

	.card {
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.75rem;
		overflow: hidden;
	}
	.legend {
		display: flex;
		gap: 1.25rem;
		align-items: center;
		padding: 0.75rem 1.1rem;
		border-bottom: 1px solid var(--ctp-surface0);
		font-size: 0.75rem;
		color: var(--ctp-overlay1);
	}
	.legend .k {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.legend .spacer {
		flex: 1;
	}
	.sw {
		width: 10px;
		height: 10px;
		border-radius: 3px;
	}
	.sw.up {
		background: var(--ctp-primary);
	}
	.sw.dl {
		background: var(--ctp-overlay1);
	}

	.row {
		border-bottom: 1px solid var(--ctp-surface0);
	}
	.row:last-child {
		border-bottom: 0;
	}
	.rowhead {
		display: grid;
		grid-template-columns: 8.5rem 1fr 6rem;
		align-items: center;
		gap: 1rem;
		width: 100%;
		text-align: left;
		background: transparent;
		border: 0;
		color: inherit;
		font: inherit;
		padding: 0.7rem 1.1rem;
		cursor: pointer;
	}
	.rowhead:hover {
		background: var(--ctp-surface0);
	}
	.rowhead:focus-visible {
		outline: 2px solid var(--ctp-primary);
		outline-offset: -2px;
	}
	.name {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 500;
		color: var(--ctp-text);
		min-width: 0;
	}
	/* Marks the rows whose bytes come from the tunnel IP, not sing-box user stats. */
	.name .kind {
		flex-shrink: 0;
		font-size: 0.65rem;
		font-weight: 400;
		padding: 0.05rem 0.35rem;
		border-radius: 0.25rem;
		color: var(--ctp-overlay1);
		background-color: color-mix(in srgb, var(--ctp-overlay1) 18%, transparent);
	}
	.name .chev {
		width: 14px;
		height: 14px;
		flex-shrink: 0;
		color: var(--ctp-overlay0);
		transition: transform 0.15s;
	}
	.row.open .chev {
		transform: rotate(90deg);
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--ctp-green);
		flex-shrink: 0;
	}
	.dot.idle {
		background: var(--ctp-overlay0);
	}
	.bar {
		position: relative;
		height: 1.1rem;
		background: var(--ctp-surface0);
		border-radius: 0.3rem;
		overflow: hidden;
		min-width: 2px;
	}
	.bar .up,
	.bar .dl {
		position: absolute;
		top: 0;
		bottom: 0;
	}
	.bar .up {
		left: 0;
		background: var(--ctp-primary);
	}
	.bar .dl {
		right: 0;
		background: var(--ctp-overlay1);
	}
	.total {
		text-align: right;
		font-weight: 600;
		color: var(--ctp-text);
	}
	.total small {
		display: block;
		font-weight: 400;
		color: var(--ctp-overlay1);
		font-size: 0.7rem;
	}

	.detail {
		max-height: 0;
		overflow: hidden;
		transition: max-height 0.2s ease;
		background: var(--ctp-base);
	}
	.row.open .detail {
		max-height: 320px;
	}
	.detail-in {
		padding: 1rem 1.1rem 1.25rem;
		display: grid;
		grid-template-columns: 1fr 12rem;
		gap: 1.25rem;
	}
	.chart {
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.5rem;
		padding: 0.5rem;
	}
	.chart svg {
		display: block;
		width: 100%;
		height: 96px;
	}
	.stats {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		align-self: center;
	}
	.stat {
		display: flex;
		flex-direction: column;
	}
	.stat .l {
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--ctp-overlay0);
	}
	.stat .v {
		font-size: 1.05rem;
		font-weight: 600;
	}
	.stat .v.up {
		color: var(--ctp-primary);
	}
	.stat .v.dl {
		color: var(--ctp-overlay2);
	}
	.peak {
		font-size: 0.72rem;
		color: var(--ctp-overlay1);
	}
	.nodata {
		color: var(--ctp-overlay0);
		font-size: 0.85rem;
		padding: 0.5rem 0;
	}
	@media (max-width: 640px) {
		.rowhead {
			grid-template-columns: 6.5rem 1fr 5rem;
			gap: 0.6rem;
		}
		.detail-in {
			grid-template-columns: 1fr;
		}
		.row.open .detail {
			max-height: 420px;
		}
	}
</style>

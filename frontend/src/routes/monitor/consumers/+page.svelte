<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { formatBytes, formatSpeed, notifications } from '$lib/stores';
	import TrafficChart from '$lib/components/shared/TrafficChart.svelte';
	import { seriesRates } from '$lib/utils/trafficSeries';
	import {
		PERIODS, KINDS, periodRange, consumerKey, liveIndex, kindCounts, filterRows, sortRows,
		isActive, sumHistory, sumTrend, trendBytes, peakOf, manageHref, type Period, type SortKey
	} from '$lib/utils/consumers';
	import type { Consumer, ConsumerKind, ConsumersLive, ConsumersResponse } from '$lib/types';

	// One list of everyone who moves bytes through the box (#109). Rows come from
	// /consumers (period totals + history), rates from /consumers/live (2 s poll,
	// only while the tab is visible — the backend sampler sleeps without it).
	const PERIOD_KEY = 'routebox.consumers.period';
	function savedPeriod(): Period {
		try {
			const v = localStorage.getItem(PERIOD_KEY) as Period | null;
			if (v && PERIODS.includes(v)) return v;
		} catch {
			/* storage blocked: default */
		}
		return '24h';
	}

	const q = page.url.searchParams;
	const qKind = q.get('kind') as ConsumerKind | null;
	const focus = q.get('focus');

	let period = $state<Period>(savedPeriod());
	let kind = $state<ConsumerKind | 'all'>(qKind && KINDS.includes(qKind) ? qKind : 'all');
	let sortKey = $state<SortKey>('now');
	let data = $state<ConsumersResponse | null>(null);
	let live = $state<ConsumersLive | null>(null);
	let loading = $state(true);
	let open = $state<Record<string, boolean>>(focus ? { [focus]: true } : {});

	async function load(p: Period) {
		try {
			data = await api.getConsumers(periodRange(p));
		} catch (e) {
			notifications.error(`${$t('consumers.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}
	async function poll() {
		if (document.visibilityState !== 'visible') return;
		try {
			live = await api.getConsumersLive();
		} catch {
			/* keep the last reading; the next tick retries */
		}
	}

	$effect(() => {
		const p = period;
		try {
			localStorage.setItem(PERIOD_KEY, p);
		} catch {
			/* not remembered, still works */
		}
		load(p);
		const timer = setInterval(() => load(p), 60_000);
		return () => clearInterval(timer);
	});

	onMount(() => {
		poll();
		const timer = setInterval(poll, 2000);
		document.addEventListener('visibilitychange', poll);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', poll);
		};
	});

	// ?focus= scrolls once, after the first rows render.
	let scrolled = false;
	$effect(() => {
		if (!data || scrolled || !focus) return;
		scrolled = true;
		requestAnimationFrame(() => document.getElementById(`c-${focus}`)?.scrollIntoView({ block: 'center' }));
	});

	let rows = $derived(data?.rows ?? []);
	let liveMap = $derived(liveIndex(live));
	let counts = $derived(kindCounts(rows));
	let shown = $derived(sortRows(filterRows(rows, kind), liveMap, sortKey));
	let liveRows = $derived(shown.map((r) => liveMap.get(consumerKey(r))).filter((r) => r !== undefined));
	let nowDown = $derived(liveRows.reduce((s, r) => s + r.down_bps, 0));
	let nowUp = $derived(liveRows.reduce((s, r) => s + r.up_bps, 0));
	let activeCount = $derived(shown.filter((r) => isActive(r, liveMap.get(consumerKey(r)))).length);
	let periodDown = $derived(shown.reduce((s, r) => s + r.download, 0));
	let periodUp = $derived(shown.reduce((s, r) => s + r.upload, 0));
	let summary = $derived(
		period === 'live'
			? sumTrend(liveRows)
			: data
				? seriesRates(sumHistory(shown), data.start_ts, data.end_ts, data.step, 240)
				: { down: [], up: [] }
	);
	// Live: the bar column is bytes over the last minute; otherwise period totals.
	const amount = (r: Consumer) => {
		if (period !== 'live') return { down: r.download, up: r.upload };
		return trendBytes(liveMap.get(consumerKey(r)));
	};
	let maxAmount = $derived(Math.max(1, ...shown.map((r) => amount(r).down + amount(r).up)));

	function rowSeries(r: Consumer) {
		if (period === 'live') {
			const l = liveMap.get(consumerKey(r));
			return l ? sumTrend([l]) : { down: [], up: [] };
		}
		return data ? seriesRates(r.history, data.start_ts, data.end_ts, data.step, 120) : { down: [], up: [] };
	}
	const hhmm = (ts: number) => new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
	const date = (ts: number) => new Date(ts * 1000).toLocaleDateString();
	function stateLabel(r: Consumer) {
		if (r.state === 'disabled') return $t('consumers.state.disabled');
		return $t(`consumers.state.${r.state_reason === 'quota' ? 'quota' : 'expired'}`);
	}
	function typeChips(r: Consumer): string[] {
		return r.kind === 'user' && r.tags?.length ? r.tags : [$t(`consumers.kind.${r.kind}`)];
	}
</script>

<svelte:head><title>{$t('consumers.title')} - RouteBox</title></svelte:head>

<div class="wrap">
	<div class="head">
		<h1>{$t('consumers.title')}</h1>
		<div class="flex flex-wrap gap-1" role="group">
			<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs {kind === 'all' ? 'selected' : ''}" onclick={() => (kind = 'all')}>
				{$t('consumers.filterAll')} {rows.length}
			</button>
			{#each counts as k (k.kind)}
				<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs {kind === k.kind ? 'selected' : ''}" onclick={() => (kind = k.kind)}>
					{$t(`consumers.kind.${k.kind}`)} {k.count}
				</button>
			{/each}
		</div>
		<div class="flex flex-wrap gap-1" role="group">
			{#each PERIODS as p (p)}
				<button type="button" class="toggle-btn !py-1 !px-2.5 text-xs {period === p ? 'selected' : ''}" onclick={() => (period = p)}>
					{$t(`consumers.period.${p}`)}
				</button>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="muted">{$t('common.loading')}</div>
	{:else if rows.length === 0}
		<div class="empty">{$t('consumers.empty')}</div>
	{:else}
		<div class="card summary">
			<div class="live">
				<span class="l">{$t('consumers.nowAll')}</span>
				<span class="n dl tabnum">↓ {formatSpeed(nowDown)}</span>
				<span class="n ul tabnum">↑ {formatSpeed(nowUp)}</span>
				<span class="l right">
					{$t('consumers.activeOf', { values: { active: activeCount, total: shown.length } })}
					{#if period !== 'live'}
						· {$t('consumers.periodTotal', { values: { down: formatBytes(periodDown), up: formatBytes(periodUp) } })}
					{/if}
				</span>
			</div>
			<TrafficChart down={summary.down} up={summary.up} />
		</div>

		<div class="card">
			<div class="grid-row hdr">
				<span>{$t('consumers.colWho')}</span>
				<span class="type">{$t('consumers.colType')}</span>
				<button type="button" class="sort" class:on={sortKey === 'now'} onclick={() => (sortKey = 'now')}>{$t('consumers.colNow')} ▾</button>
				<button type="button" class="sort period" class:on={sortKey === 'period'} onclick={() => (sortKey = 'period')}>
					{period === 'live' ? $t('consumers.colLive') : $t('consumers.colPeriod')} ▾
				</button>
				<span class="r">↓ / ↑</span>
			</div>
			{#each shown as r (consumerKey(r))}
				{@const key = consumerKey(r)}
				{@const l = liveMap.get(key)}
				{@const a = amount(r)}
				{@const why = live?.unavailable?.[r.kind]}
				<div class="row" class:open={open[key]} id="c-{key}">
					<!-- mousedown preventDefault: a pointer click toggles the row without taking focus,
					     so no focus ring stays behind whatever the browser's :focus-visible heuristic does;
					     Tab still focuses it and Enter/Space still activate it. -->
					<button type="button" class="grid-row" aria-expanded={!!open[key]} onmousedown={(e) => e.preventDefault()} onclick={() => (open[key] = !open[key])}>
						<span class="name">
							<span class="dot" class:idle={!isActive(r, l)} class:off={r.state !== 'active'}></span>
							{r.name}
						</span>
						<span class="type">{#each typeChips(r) as chip}<span class="chip">{chip}</span>{/each}</span>
						<span class="tabnum now">
							{#if r.state !== 'active'}
								<span class="muted">{stateLabel(r)}</span>
							{:else if why}
								<span class="muted" title={$t('consumers.unavailable', { values: { reason: why } })}>—</span>
							{:else if l}
								<span class="dl">↓ {formatSpeed(l.down_bps)}</span> <span class="ul">↑ {formatSpeed(l.up_bps)}</span>
							{:else}
								<span class="muted">—</span>
							{/if}
						</span>
						<span class="period">
							{#if period === 'live'}
								<TrafficChart down={rowSeries(r).down} up={rowSeries(r).up} class="h-5" />
							{:else}
								<span class="bar" style="width:{((a.down + a.up) / maxAmount) * 100}%">
									<b style="width:{a.down + a.up > 0 ? (a.down / (a.down + a.up)) * 100 : 0}%"></b>
								</span>
							{/if}
						</span>
						<span class="r tabnum">{formatBytes(a.down)} / {formatBytes(a.up)}</span>
					</button>
					{#if open[key]}
						{@const s = rowSeries(r)}
						{@const peak = period === 'live' ? null : peakOf(r.history)}
						<div class="detail">
							<div>
								{#if s.down.length > 1}
									<TrafficChart down={s.down} up={s.up} class="h-20" />
									<div class="legend">
										<span><i class="sw dl-bg"></i>{$t('consumers.download')}</span>
										<span><i class="sw ul-bg"></i>{$t('consumers.upload')}</span>
										{#if peak}<span class="right">{$t('consumers.peak', { values: { time: hhmm(peak.ts) } })}</span>{/if}
									</div>
								{:else}
									<div class="muted">{$t('consumers.noData')}</div>
								{/if}
							</div>
							<dl class="kv">
								{#if r.address}<dt>{$t('consumers.address')}</dt><dd>{r.address}</dd>{/if}
								{#if r.kind === 'awg'}<dt>{$t('consumers.handshake')}</dt><dd>{r.last_handshake ? hhmm(r.last_handshake) : '—'}</dd>{/if}
								{#if r.quota_bytes}<dt>{$t('consumers.quota')}</dt><dd>{formatBytes(r.used ?? 0)} / {formatBytes(r.quota_bytes)}</dd>{/if}
								{#if r.kind !== 'lan'}<dt>{$t('consumers.expires')}</dt><dd>{r.expires_at ? date(r.expires_at) : $t('consumers.never')}</dd>{/if}
								<dt></dt><dd><a class="link" href={manageHref(r)}>{$t('consumers.manage')}</a></dd>
							</dl>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.wrap { max-width: 64rem; display: flex; flex-direction: column; gap: 0.75rem; }
	.head { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; }
	.head h1 { font-size: 1.5rem; font-weight: 700; margin: 0 auto 0 0; color: var(--ctp-text); }
	/* Global .toggle-btn has flex: 1 (stretches in form rows); here every pill is content-sized, one line. */
	.head .toggle-btn { flex: 0 0 auto; white-space: nowrap; }
	.tabnum { font-variant-numeric: tabular-nums; }
	.muted { color: var(--ctp-overlay1); }
	.empty { background: var(--ctp-surface0); border-radius: 0.75rem; padding: 2rem; text-align: center; color: var(--ctp-overlay1); }
	.card { background: var(--ctp-mantle); border: 1px solid var(--ctp-surface0); border-radius: 0.75rem; overflow: hidden; }
	.summary { padding: 0.75rem 1rem; }
	.live { display: flex; align-items: baseline; gap: 1rem; flex-wrap: wrap; margin-bottom: 0.4rem; }
	.live .n { font-size: 1.25rem; font-weight: 700; }
	.l { font-size: 0.75rem; color: var(--ctp-overlay1); }
	.right { margin-left: auto; }
	.dl { color: var(--ctp-primary); }
	.ul { color: var(--ctp-upload); }
	.grid-row { display: grid; grid-template-columns: minmax(8rem, 1.4fr) 7rem 11rem minmax(5rem, 1.2fr) 9rem; align-items: center; gap: 0.75rem; width: 100%; padding: 0.6rem 1rem; background: transparent; border: 0; color: inherit; font: inherit; text-align: left; }
	.hdr { font-size: 0.7rem; color: var(--ctp-overlay1); border-bottom: 1px solid var(--ctp-surface0); }
	.sort { background: none; border: 0; color: inherit; font: inherit; text-align: left; cursor: pointer; padding: 0; }
	.sort.on { color: var(--ctp-text); }
	.row { border-bottom: 1px solid var(--ctp-surface0); }
	.row:last-child { border-bottom: 0; }
	.row > .grid-row { cursor: pointer; font-size: 0.85rem; }
	.row > .grid-row:hover { background: var(--ctp-surface0); }
	/* Keyboard focus ring drawn inside the row: the card clips overflow, so the global
	   outline-offset: 2px showed only as two lines above and below the row. */
	.row > .grid-row:focus-visible { outline-offset: -2px; }
	.name { display: flex; align-items: center; gap: 0.5rem; min-width: 0; color: var(--ctp-text); font-weight: 500; }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--ctp-green); flex-shrink: 0; }
	.dot.idle { background: var(--ctp-overlay0); }
	.dot.off { background: var(--ctp-red); }
	.type { display: flex; gap: 0.25rem; flex-wrap: wrap; }
	.chip { font-size: 0.65rem; padding: 0.05rem 0.35rem; border-radius: 0.25rem; color: var(--ctp-overlay1); background: color-mix(in srgb, var(--ctp-overlay1) 18%, transparent); }
	.bar { display: block; height: 0.5rem; min-width: 2px; border-radius: 0.25rem; background: var(--ctp-upload); overflow: hidden; }
	.bar b { display: block; height: 100%; background: var(--ctp-primary); }
	.r { text-align: right; color: var(--ctp-subtext1); font-size: 0.8rem; }
	.detail { display: grid; grid-template-columns: 1fr 14rem; gap: 1rem; padding: 0.75rem 1rem 1rem; background: var(--ctp-surface0); }
	.legend { display: flex; gap: 1rem; font-size: 0.7rem; color: var(--ctp-overlay1); margin-top: 0.3rem; }
	.sw { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 0.3rem; }
	.dl-bg { background: var(--ctp-primary); }
	.ul-bg { background: var(--ctp-upload); }
	.kv { display: grid; grid-template-columns: auto 1fr; gap: 0.2rem 0.75rem; font-size: 0.8rem; margin: 0; }
	.kv dt { color: var(--ctp-overlay1); }
	.kv dd { margin: 0; }
	.link { color: var(--ctp-primary); }
	@media (max-width: 480px) {
		.grid-row { grid-template-columns: 1fr auto auto; }
		.type, .period { display: none; }
		.detail { grid-template-columns: 1fr; }
	}
</style>

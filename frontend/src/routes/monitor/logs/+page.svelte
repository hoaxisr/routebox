<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { t } from 'svelte-i18n';
	import { createLogsStream, createMtprotoLogsStream } from '$lib/api/client';
	import { panelMode } from '$lib/stores';

	interface LogEntry {
		id: number;
		type: string;
		payload: string;
		time: Date;
	}

	let logs = $state<LogEntry[]>([]);
	let stream: { close: () => void } | null = null;
	let connected = $state(false);
	let logId = 0;
	let filter = $state(browser ? (localStorage.getItem('logs.level') ?? 'all') : 'all');
	// Which process we are watching. amnezia-box comes over the Clash API; the
	// Telegram proxy runs inside RouteBox and has its own stream.
	type Source = 'singbox' | 'mtproto';
	let source = $state<Source>(browser ? ((localStorage.getItem('logs.source') as Source) ?? 'singbox') : 'singbox');
	let search = $state('');
	let autoScroll = $state(true);
	let paused = $state(false);
	let logContainer: HTMLDivElement;

	const MAX_LOGS = 500;
	const FLUSH_INTERVAL_MS = 150;

	const LOG_LEVELS = ['all', 'trace', 'debug', 'info', 'warn', 'error'] as const;

	// Batch buffer — incoming logs accumulate here between flushes
	let pendingLogs: LogEntry[] = [];
	let flushTimer: ReturnType<typeof setInterval> | null = null;

	function getLogColor(type: string): string {
		switch (type.toLowerCase()) {
			case 'error':
				return 'var(--ctp-red)';
			case 'warn':
			case 'warning':
				return 'var(--ctp-overlay1)';
			case 'info':
				return 'var(--ctp-subtext1)';
			case 'debug':
				return 'var(--ctp-subtext0)';
			case 'trace':
				return 'var(--ctp-overlay0)';
			default:
				return 'var(--ctp-text)';
		}
	}

	function flushPending() {
		if (pendingLogs.length === 0 || paused) return;
		const batch = pendingLogs;
		pendingLogs = [];
		const merged = logs.concat(batch);
		logs = merged.length > MAX_LOGS ? merged.slice(-MAX_LOGS) : merged;

		if (autoScroll && logContainer) {
			requestAnimationFrame(() => {
				logContainer.scrollTop = 0;
			});
		}
	}

	function startFlushTimer() {
		if (flushTimer) return;
		flushTimer = setInterval(flushPending, FLUSH_INTERVAL_MS);
	}

	function stopFlushTimer() {
		if (flushTimer) {
			clearInterval(flushTimer);
			flushTimer = null;
		}
	}

	function startStream(level = 'info') {
		stopStream();
		const push = (data: { type: string; payload: string }) => {
			pendingLogs.push({ id: ++logId, type: data.type, payload: data.payload, time: new Date() });
		};
		const onStatus = (status: string) => {
			connected = status === 'connected';
		};
		stream =
			source === 'mtproto'
				? createMtprotoLogsStream(push, undefined, onStatus)
				: createLogsStream(push, level, undefined, onStatus);
		startFlushTimer();
	}

	// Switching source starts a different stream against a different process, so
	// the old process's lines are cleared rather than interleaved.
	function setSource(next: Source) {
		if (next === source) return;
		source = next;
		if (browser) localStorage.setItem('logs.source', next);
		clearLogs();
		startStream(filter === 'all' ? 'info' : filter);
	}

	function stopStream() {
		if (stream) {
			stream.close();
			stream = null;
			connected = false;
		}
		stopFlushTimer();
		flushPending();
	}

	function clearLogs() {
		logs = [];
		pendingLogs = [];
	}

	function togglePause() {
		paused = !paused;
		if (!paused) {
			// Flush accumulated logs on unpause, but cap to MAX_LOGS
			if (pendingLogs.length > MAX_LOGS) {
				pendingLogs = pendingLogs.slice(-MAX_LOGS);
			}
			flushPending();
		}
	}

	function formatTime(date: Date): string {
		return date.toLocaleTimeString('en-US', {
			hour12: false,
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	let filteredLogs = $derived(
		logs
			.filter((log) => {
				if (filter !== 'all' && log.type.toLowerCase() !== filter) return false;
				if (search && !log.payload.toLowerCase().includes(search.toLowerCase())) return false;
				return true;
			})
			.slice()
			.reverse()
	);

	let prevFilter = filter;
	$effect(() => {
		if (filter !== prevFilter) {
			prevFilter = filter;
			if (browser) localStorage.setItem('logs.level', filter);
			// Only the Clash stream takes a level server-side; for the proxy the
			// filter is applied to what has already arrived.
			if (source !== 'mtproto') startStream(filter === 'all' ? 'info' : filter);
		}
	});

	onMount(() => {
		startStream(filter === 'all' ? 'info' : filter);
		return () => {
			stopStream();
		};
	});
</script>

<div class="space-y-4 h-full flex flex-col min-w-0">
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-4 min-w-0">
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('logs.title')}</h1>
			{#if $panelMode}
				<div class="src-tabs">
					<button type="button" class="src-tab" class:active={source === 'singbox'} onclick={() => setSource('singbox')}>
						{$t('logs.sourceSingbox')}
					</button>
					<button type="button" class="src-tab" class:active={source === 'mtproto'} onclick={() => setSource('mtproto')}>
						{$t('logs.sourceMtproto')}
					</button>
				</div>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<span
				class="w-2 h-2 rounded-full"
				class:bg-[var(--ctp-green)]={connected}
				class:bg-[var(--ctp-red)]={!connected}
			></span>
			<span class="text-sm text-[var(--ctp-overlay1)]">
				{connected ? $t('logs.streaming') : $t('status.disconnected')}
			</span>
		</div>
	</div>

	<!-- Filters -->
	<div class="flex flex-wrap items-center gap-x-4 gap-y-2 w-full">
		<select
			bind:value={filter}
			class="px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		>
			{#each LOG_LEVELS as level}
				<option value={level}>{level.charAt(0).toUpperCase() + level.slice(1)}</option>
			{/each}
		</select>

		<input
			type="text"
			bind:value={search}
			placeholder={$t('logs.search')}
			class="flex-1 min-w-0 basis-40 px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		/>

		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={autoScroll}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('logs.autoScroll')}
		</label>

		<button
			onclick={togglePause}
			class="px-3 py-2 rounded-lg transition-colors {paused ? 'bg-[var(--ctp-primary)] text-white' : 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface2)]'}"
		>
			{paused ? $t('logs.resume') : $t('logs.pause')}
		</button>

		<button
			onclick={clearLogs}
			class="px-3 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('logs.clear')}
		</button>
	</div>

	<!-- Logs -->
	<div
		bind:this={logContainer}
		class="flex-1 bg-[var(--ctp-mantle)] rounded-xl overflow-auto font-mono text-sm"
	>
		{#if filteredLogs.length === 0}
			<div class="p-4 text-center text-[var(--ctp-overlay0)]">
				{logs.length === 0 ? $t('logs.waitingForLogs') : $t('logs.noLogsMatchFilter')}
			</div>
		{:else}
			<div class="p-2 space-y-0.5">
				{#each filteredLogs as log (log.id)}
					<div class="flex gap-3 px-2 py-1 hover:bg-[var(--ctp-surface0)] rounded">
						<span class="text-[var(--ctp-overlay0)] shrink-0">{formatTime(log.time)}</span>
						<span
							class="shrink-0 w-12 text-right"
							style="color: {getLogColor(log.type)}"
						>
							[{log.type}]
						</span>
						<span class="text-[var(--ctp-text)] break-all">{log.payload}</span>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	/* Segmented control, matching the range picker on the traffic pages. */
	.src-tabs {
		display: inline-flex;
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.5rem;
		padding: 2px;
		gap: 2px;
	}
	.src-tab {
		padding: 0.3rem 0.7rem;
		border: none;
		background: transparent;
		border-radius: 0.375rem;
		color: var(--ctp-subtext0);
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		white-space: nowrap;
		transition: all 0.15s ease;
	}
	.src-tab:hover:not(.active) {
		color: var(--ctp-text);
	}
	.src-tab.active {
		background: var(--ctp-primary);
		color: #1a1a1a;
	}
</style>

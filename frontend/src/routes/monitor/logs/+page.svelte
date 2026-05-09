<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { t } from 'svelte-i18n';
	import { createLogsStream } from '$lib/api/client';

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
		stream = createLogsStream((data) => {
			connected = true;
			const entry: LogEntry = {
				id: ++logId,
				type: data.type,
				payload: data.payload,
				time: new Date()
			};
			pendingLogs.push(entry);
		}, level);
		startFlushTimer();
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

	let prevFilter = 'all';
	$effect(() => {
		if (filter !== prevFilter) {
			prevFilter = filter;
			if (browser) localStorage.setItem('logs.level', filter);
			startStream(filter === 'all' ? 'info' : filter);
		}
	});

	onMount(() => {
		startStream('info');
		return () => {
			stopStream();
		};
	});
</script>

<div class="space-y-4 h-full flex flex-col">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('logs.title')}</h1>
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
	<div class="flex items-center gap-4">
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
			class="flex-1 px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
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

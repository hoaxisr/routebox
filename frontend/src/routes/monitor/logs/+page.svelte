<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { createLogsStream } from '$lib/api/client';

	interface LogEntry {
		id: number;
		type: string;
		payload: string;
		time: Date;
	}

	let logs: LogEntry[] = [];
	let stream: { close: () => void } | null = null;
	let connected = false;
	let logId = 0;
	let filter = 'all';
	let search = '';
	let autoScroll = true;
	let logContainer: HTMLDivElement;

	const MAX_LOGS = 500;

	const LOG_LEVELS = ['all', 'trace', 'debug', 'info', 'warn', 'error'] as const;

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

			logs = [...logs.slice(-(MAX_LOGS - 1)), entry];

			if (autoScroll && logContainer) {
				setTimeout(() => {
					logContainer.scrollTop = logContainer.scrollHeight;
				}, 0);
			}
		}, level);
	}

	function stopStream() {
		if (stream) {
			stream.close();
			stream = null;
			connected = false;
		}
	}

	function clearLogs() {
		logs = [];
	}

	function formatTime(date: Date): string {
		return date.toLocaleTimeString('en-US', {
			hour12: false,
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	$: filteredLogs = logs.filter((log) => {
		if (filter !== 'all' && log.type.toLowerCase() !== filter) return false;
		if (search && !log.payload.toLowerCase().includes(search.toLowerCase())) return false;
		return true;
	});

	$: if (filter !== 'all') {
		startStream(filter);
	}

	onMount(() => {
		startStream('info');
	});

	onDestroy(() => {
		stopStream();
	});
</script>

<div class="space-y-4 h-full flex flex-col">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">Live Logs</h1>
		<div class="flex items-center gap-2">
			<span
				class="w-2 h-2 rounded-full"
				class:bg-[var(--ctp-green)]={connected}
				class:bg-[var(--ctp-red)]={!connected}
			></span>
			<span class="text-sm text-[var(--ctp-overlay1)]">
				{connected ? 'Streaming' : 'Disconnected'}
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
			placeholder="Search logs..."
			class="flex-1 px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		/>

		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={autoScroll}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			Auto-scroll
		</label>

		<button
			onclick={clearLogs}
			class="px-3 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			Clear
		</button>
	</div>

	<!-- Logs -->
	<div
		bind:this={logContainer}
		class="flex-1 bg-[var(--ctp-mantle)] rounded-xl overflow-auto font-mono text-sm"
	>
		{#if filteredLogs.length === 0}
			<div class="p-4 text-center text-[var(--ctp-overlay0)]">
				{logs.length === 0 ? 'Waiting for logs...' : 'No logs match the filter'}
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

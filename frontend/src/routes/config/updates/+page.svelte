<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import { t } from 'svelte-i18n';
	import type { UpdatesStatus, UpdateTarget, UpdateProgress, UpdateTargetName } from '$lib/types';

	let loading = $state(true);
	let checking = $state(false);
	let status = $state<UpdatesStatus | null>(null);
	let applying = $state<UpdateTargetName | null>(null);
	let progress = $state<UpdateProgress | null>(null);
	let restartWait = $state(false);
	let updatedToVersion = $state('');
	let manualRestart = $state(false);
	let now = $state(Date.now());

	let progressTimer: ReturnType<typeof setInterval> | null = null;
	let clockTimer: ReturnType<typeof setInterval> | null = null;

	let lastCheckedAt = $derived.by(() => {
		if (!status) return null;
		let max = 0;
		for (const target of status.targets) {
			if (target.last_checked) {
				const ts = new Date(target.last_checked).getTime();
				if (ts > max) max = ts;
			}
		}
		return max > 0 ? max : null;
	});

	let lastCheckedText = $derived.by(() => {
		void now; // refresh label as time passes
		if (!lastCheckedAt) return $t('updates.neverChecked');
		const diffMs = now - lastCheckedAt;
		const min = Math.floor(diffMs / 60000);
		let time: string;
		if (min < 1) {
			time = $t('updates.timeAgo.justNow');
		} else if (min < 60) {
			time = $t('updates.timeAgo.minutes', { values: { n: min } });
		} else if (min < 1440) {
			time = $t('updates.timeAgo.hours', { values: { n: Math.floor(min / 60) } });
		} else {
			time = $t('updates.timeAgo.days', { values: { n: Math.floor(min / 1440) } });
		}
		return $t('updates.lastChecked', { values: { time } });
	});

	let progressPercent = $derived(
		progress && progress.total_bytes > 0
			? Math.min(100, Math.round((progress.downloaded_bytes / progress.total_bytes) * 100))
			: null
	);

	onMount(async () => {
		clockTimer = setInterval(() => {
			now = Date.now();
		}, 30000);
		try {
			status = await api.getUpdatesStatus();
		} catch (err) {
			notifications.error($t('updates.loadFailed', { values: { error: String(err) } }));
		} finally {
			loading = false;
		}
	});

	onDestroy(() => {
		stopProgressPolling();
		if (clockTimer) clearInterval(clockTimer);
	});

	async function checkNow() {
		checking = true;
		try {
			status = await api.checkUpdates();
			now = Date.now();
		} catch (err) {
			notifications.error($t('updates.checkFailed', { values: { error: String(err) } }));
		} finally {
			checking = false;
		}
	}

	function startProgressPolling() {
		stopProgressPolling();
		progressTimer = setInterval(async () => {
			try {
				progress = await api.getUpdateProgress();
			} catch {
				// transient failures (server busy/restarting) — keep last known progress
			}
		}, 500);
	}

	function stopProgressPolling() {
		if (progressTimer) {
			clearInterval(progressTimer);
			progressTimer = null;
		}
	}

	function sleep(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}

	// After a routebox self-update with {restarting: true}: poll status with
	// backoff until the new process answers (up to ~60s), then show the banner.
	async function waitForRestart(expectedVersion: string) {
		const deadline = Date.now() + 60000;
		let delay = 1000;
		while (Date.now() < deadline) {
			await sleep(delay);
			try {
				const fresh = await api.getUpdatesStatus();
				status = fresh;
				now = Date.now();
				restartWait = false;
				updatedToVersion =
					fresh.targets.find((target) => target.name === 'routebox')?.current ||
					expectedVersion;
				return;
			} catch {
				delay = Math.min(Math.round(delay * 1.5), 5000);
			}
		}
		restartWait = false;
		manualRestart = true;
	}

	async function applyUpdate(target: UpdateTarget) {
		if (target.name === 'amnezia-box' && !confirm($t('updates.confirmProxyRestart'))) {
			return;
		}
		applying = target.name;
		progress = null;
		manualRestart = false;
		updatedToVersion = '';
		startProgressPolling();
		try {
			const result = await api.applyUpdate(target.name);
			stopProgressPolling();
			if (target.name === 'routebox') {
				if (result.restarting) {
					restartWait = true;
					await waitForRestart(target.latest || '');
				} else {
					manualRestart = true;
				}
			} else {
				notifications.success(
					$t('updates.updatedTo', { values: { version: target.latest || '' } })
				);
				try {
					status = await api.getUpdatesStatus();
					now = Date.now();
				} catch {
					// status refresh is best-effort
				}
			}
		} catch (err) {
			stopProgressPolling();
			notifications.error($t('updates.updateFailed', { values: { error: String(err) } }));
		} finally {
			stopProgressPolling();
			applying = null;
			progress = null;
		}
	}
</script>

<svelte:head>
	<title>{$t('updates.title')} - RouteBox</title>
</svelte:head>

<div class="space-y-6 max-w-4xl">
	<!-- Header -->
	<div class="flex items-center justify-between flex-wrap gap-3">
		<div>
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('updates.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('updates.description')}</p>
		</div>
		<div class="flex items-center gap-3">
			<span class="text-sm text-[var(--ctp-overlay1)]">{lastCheckedText}</span>
			<button
				onclick={checkNow}
				disabled={checking || loading || applying !== null}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
			>
				{#if checking}
					<svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
					{$t('updates.checking')}
				{:else}
					{$t('updates.checkNow')}
				{/if}
			</button>
		</div>
	</div>

	<!-- Self-update banners -->
	{#if restartWait}
		<div class="px-4 py-3 rounded-lg bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] flex items-center gap-3">
			<svg class="animate-spin h-5 w-5 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
			<span class="text-sm text-[var(--ctp-text)]">{$t('updates.restartingBanner')}</span>
		</div>
	{/if}
	{#if updatedToVersion}
		<div class="px-4 py-3 rounded-lg bg-[var(--ctp-green)]/20 border border-[var(--ctp-green)] text-[var(--ctp-green)] text-sm">
			{$t('updates.updatedTo', { values: { version: updatedToVersion } })}
		</div>
	{/if}
	{#if manualRestart}
		<div class="px-4 py-3 rounded-lg bg-[var(--ctp-yellow)]/20 border border-[var(--ctp-yellow)] text-[var(--ctp-yellow)] text-sm">
			{$t('updates.manualRestartNeeded')}
		</div>
	{/if}

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if status}
		<div class="space-y-4">
			{#each status.targets.filter((target) => target.supported) as target (target.name)}
				<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
					<div class="flex items-start justify-between gap-4 flex-wrap">
						<div>
							<h2 class="text-lg font-medium text-[var(--ctp-text)]">
								{$t(`updates.targets.${target.name}`)}
							</h2>
							<div class="mt-1 text-sm text-[var(--ctp-subtext1)]">
								{$t('updates.currentVersion')}:
								<span class="font-mono">{target.current || '—'}</span>
								<span class="mx-1 text-[var(--ctp-overlay0)]">→</span>
								{$t('updates.latestVersion')}:
								<span class="font-mono">{target.latest || '—'}</span>
							</div>
							{#if target.published_at}
								<div class="text-xs text-[var(--ctp-overlay1)] mt-1">
									{$t('updates.publishedAt', {
										values: { date: new Date(target.published_at).toLocaleDateString() }
									})}
								</div>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							{#if target.update_available}
								<span class="px-2 py-0.5 text-xs rounded-full bg-[var(--ctp-yellow)]/20 text-[var(--ctp-yellow)]">
									{$t('updates.updateAvailable')}
								</span>
								<button
									onclick={() => applyUpdate(target)}
									disabled={applying !== null || restartWait}
									class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50"
								>
									{applying === target.name ? $t('updates.updating') : $t('updates.update')}
								</button>
							{:else}
								<span class="px-2 py-0.5 text-xs rounded-full bg-[var(--ctp-green)]/20 text-[var(--ctp-green)]">
									{$t('updates.upToDate')}
								</span>
							{/if}
						</div>
					</div>

					{#if target.error}
						<div class="mt-3 text-sm text-[var(--ctp-red)]">{target.error}</div>
					{/if}

					<!-- Progress bar while this target is updating -->
					{#if applying === target.name && progress && progress.phase !== 'idle'}
						<div class="mt-4">
							<div class="flex items-center justify-between text-xs text-[var(--ctp-subtext1)] mb-1">
								<span>{$t(`updates.phases.${progress.phase}`)}</span>
								{#if progress.phase === 'download' && progressPercent !== null}
									<span>{progressPercent}%</span>
								{/if}
							</div>
							<div class="h-2 rounded-full bg-[var(--ctp-surface0)] overflow-hidden">
								{#if progress.phase === 'download' && progressPercent !== null}
									<div
										class="h-full bg-[var(--ctp-primary)] transition-all duration-300"
										style="width: {progressPercent}%"
									></div>
								{:else}
									<div class="h-full w-1/3 bg-[var(--ctp-primary)] animate-pulse"></div>
								{/if}
							</div>
						</div>
					{/if}

					<!-- Release notes (plain text, preserved line breaks) -->
					{#if target.update_available && target.notes}
						<div class="mt-4">
							<div class="text-xs font-medium text-[var(--ctp-overlay1)] uppercase tracking-wider mb-2">
								{$t('updates.releaseNotes')}
							</div>
							<pre class="text-sm text-[var(--ctp-subtext1)] whitespace-pre-wrap break-words bg-[var(--ctp-surface0)] rounded-lg p-3 max-h-64 overflow-y-auto font-sans">{target.notes}</pre>
						</div>
					{/if}
				</section>
			{/each}
		</div>
	{/if}
</div>

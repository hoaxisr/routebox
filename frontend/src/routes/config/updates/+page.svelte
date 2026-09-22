<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import { t } from 'svelte-i18n';
	import { copyText } from '$lib/utils/clipboard';
	import { recoverAfterDisconnect as recoverLostApply } from '$lib/utils/updateRecovery';
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
	// Last apply outcome on the server, shown on the card after a lost
	// connection or a page reload (the toast alone is gone by then).
	let lastProgress = $state<UpdateProgress | null>(null);
	let alive = true;

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
		try {
			lastProgress = await api.getUpdateProgress();
		} catch {
			// best-effort
		}
	});

	onDestroy(() => {
		alive = false;
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

	async function copyCommand(command: string) {
		if (await copyText(command)) {
			notifications.success($t('common.copied'));
		} else {
			notifications.error($t('common.copyFailed'));
		}
	}

	// The apply response never arrived: no HTTP response at all (TypeError —
	// the panel is usually reached through the very proxy being restarted),
	// or a reverse proxy in front gave up on the long request.
	function outcomeUnknown(err: unknown): boolean {
		return err instanceof TypeError || /^HTTP 50[234]$|^HTTP 524$/.test(String((err as Error)?.message));
	}

	async function refreshStatus() {
		try {
			status = await api.getUpdatesStatus();
			now = Date.now();
		} catch {
			// status refresh is best-effort
		}
	}

	async function applyUpdate(target: UpdateTarget) {
		if (target.name === 'amnezia-box' && !confirm($t('updates.confirmProxyRestart'))) {
			return;
		}
		applying = target.name;
		progress = null;
		manualRestart = false;
		updatedToVersion = '';
		// Baseline: an apply that never reached the server leaves seq unchanged.
		let seqBefore = -1;
		try {
			seqBefore = (await api.getUpdateProgress()).seq;
		} catch {
			// unreachable now — the apply itself will tell
		}
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
				await refreshStatus();
			}
		} catch (err) {
			stopProgressPolling();
			if (!outcomeUnknown(err)) {
				notifications.error($t('updates.updateFailed', { values: { error: String(err) } }), 0);
			} else if (target.name === 'routebox') {
				// Self-update re-execs the process: sessions are gone and the new
				// updater starts at seq 0, so progress cannot be trusted. Wait for
				// the new process the same way a successful response does.
				restartWait = true;
				await waitForRestart(target.latest || '');
			} else {
				await recoverAfterDisconnect(target, seqBefore, String(err));
			}
		} finally {
			stopProgressPolling();
			applying = null;
			progress = null;
		}
	}

	// Outcome of an apply whose response was lost — see $lib/utils/updateRecovery.
	async function recoverAfterDisconnect(target: UpdateTarget, seqBefore: number, origError: string) {
		const out = await recoverLostApply(target.name, seqBefore, {
			poll: api.getUpdateProgress,
			sleep,
			now: Date.now,
			alive: () => alive,
			onProgress: (p) => {
				progress = p;
			}
		});
		if (!alive) return;
		switch (out.kind) {
			case 'done':
				lastProgress = out.progress;
				notifications.success($t('updates.updatedTo', { values: { version: target.latest || '' } }));
				await refreshStatus();
				break;
			case 'error':
				lastProgress = out.progress;
				notifications.error($t('updates.updateFailed', { values: { error: out.progress.error || '' } }), 0);
				await refreshStatus();
				break;
			case 'not-started':
				notifications.error(
					$t('updates.updateFailed', {
						values: { error: $t('updates.notStarted', { values: { error: origError } }) }
					}),
					0
				);
				break;
			case 'lost':
				notifications.error($t('updates.connectionLost'), 0);
				break;
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
								{#if !target.docker_managed}
									<button
										onclick={() => applyUpdate(target)}
										disabled={applying !== null || restartWait}
										class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50"
									>
										{applying === target.name ? $t('updates.updating') : $t('updates.update')}
									</button>
								{/if}
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
					{#if applying !== target.name && lastProgress?.target === target.name && lastProgress.phase === 'error'}
						<div class="mt-3 text-sm text-[var(--ctp-red)] whitespace-pre-wrap break-words">
							{$t('updates.lastAttemptFailed', { values: { error: lastProgress.error || '' } })}
						</div>
					{/if}

					{#if target.update_available && target.docker_managed && target.update_command}
						<div class="mt-3">
							<p class="text-sm text-[var(--ctp-subtext1)] mb-2">{$t('updates.dockerManagedHint')}</p>
							<div class="flex items-center gap-2">
								<code
									class="flex-1 text-sm font-mono bg-[var(--ctp-surface0)] rounded-lg px-3 py-2 overflow-x-auto whitespace-nowrap"
								>
									{target.update_command}
								</code>
								<button
									onclick={() => copyCommand(target.update_command ?? '')}
									class="px-3 py-2 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface1)] transition-colors"
								>
									{$t('common.copy')}
								</button>
							</div>
						</div>
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

<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, configPaths, processStatus, refreshStatus } from '$lib/stores';

	// '' | 'use' | 'fix' | 'restart' | 'dropin' — one action at a time, all
	// buttons disabled meanwhile.
	let busy = $state('');

	// The single state the banner renders: which config file RouteBox, the
	// systemd unit and the live process point at. Both disagreements come from
	// here, so neither can go unreported while the other is shown — and neither
	// survives a page reload as a leftover of a click, because nothing about
	// them is remembered in this component.
	let paths = $derived($configPaths);

	// The systemd unit both fixes would modify.
	let serviceName = $derived($processStatus?.service_name ?? '');

	// The drop-in RouteBox installed, while the file is on disk. Comes from the
	// same status as the paths, so it is present exactly as long as the file is —
	// including after a fix whose daemon-reload failed, where the file is there
	// and nothing about it was visible before.
	let dropIn = $derived(paths?.drop_in ?? null);

	// amnezia-box is the unit both installers create. Anything else is most
	// likely a foreign installation the host happens to carry (an upstream
	// sing-box left enabled), and BOTH fixes write to it — one drops a drop-in
	// into its unit directory, the other moves RouteBox onto its config. That is
	// the user's call to make, but only if they are told whose unit it is.
	let foreignUnit = $derived(serviceName !== '' && !serviceName.startsWith('amnezia-box'));

	function errorText(e: unknown): string {
		return e instanceof Error ? e.message : String(e);
	}

	function fail(e: unknown) {
		notifications.error($t('configMismatch.failed', { values: { error: errorText(e) } }));
	}

	// Re-read the status after an action that already succeeded. A failure here
	// says nothing about the action itself, so it must not be reported as one.
	async function refreshAfterSuccess(): Promise<void> {
		try {
			await refreshStatus();
		} catch {
			notifications.error($t('configMismatch.refreshFailed'));
		}
	}

	// Move RouteBox onto the config path from the unit's ExecStart. The unit is
	// left alone; the live process keeps reading whatever it was started with,
	// which is then reported as the process disagreement below.
	async function useUnitConfig() {
		busy = 'use';
		try {
			const result = await api.adoptUnitConfigPath();
			notifications.success($t('configMismatch.switched', { values: { path: result.path } }));
			// The switch is live but not written down: it holds until RouteBox
			// itself restarts, and then the old path comes back. Saying so is the
			// difference between a fix and a fix-shaped promise.
			//
			// duration 0 = stays until dismissed. The default five seconds is for
			// "saved" — this is two sentences with a filesystem error inside, it
			// is the only notice that the fix did not stick, and once it fades
			// there is nowhere left to read it.
			if (result.warning) {
				notifications.warning(
					$t('configMismatch.notPersisted', { values: { error: result.warning } }),
					0
				);
			}
		} catch (e) {
			fail(e);
			busy = '';
			return;
		}
		await refreshAfterSuccess();
		busy = '';
	}

	// Repoint the unit at our config. Succeeds only if systemd actually picked
	// the drop-in up (the backend re-reads ExecStart before reporting success).
	async function fixUnit() {
		busy = 'fix';
		try {
			await api.fixUnitConfigPath();
		} catch (e) {
			fail(e);
			busy = '';
			return;
		}
		notifications.success($t('configMismatch.fixed'));
		await refreshAfterSuccess();
		busy = '';
	}

	// Take the drop-in back off. The unit returns to its own ExecStart, which is
	// why the mismatch may come back — said next to the button, before the click,
	// rather than appearing as a surprise banner afterwards.
	async function removeDropIn() {
		busy = 'dropin';
		try {
			const result = await api.removeUnitConfigDropIn();
			// One message, not two: "the unit is back on its own ExecStart" and the
			// warning that it is NOT (the reload failed, the override still applies)
			// contradict each other, and shown together the reassuring one is the
			// one that gets believed. The file is gone in both cases, which is what
			// the partial message says.
			//
			// duration 0 = stays until dismissed: this half-state is reported
			// nowhere else.
			if (result.warning) {
				notifications.warning(
					$t('configDropIn.removedPartly', { values: { error: result.warning } }),
					0
				);
			} else {
				notifications.success($t('configDropIn.removed', { values: { service: serviceName } }));
			}
		} catch (e) {
			notifications.error($t('configDropIn.failed', { values: { error: errorText(e) } }));
			busy = '';
			return;
		}
		await refreshAfterSuccess();
		busy = '';
	}

	// The cure for a process that reads the old file. Deliberately a button and
	// not something we do on the user's behalf: a restart drops every active
	// connection, and that is their call.
	async function restartNow() {
		busy = 'restart';
		try {
			await api.restart();
		} catch (e) {
			fail(e);
			busy = '';
			return;
		}
		notifications.success($t('dashboard.restarted'));
		await refreshAfterSuccess();
		busy = '';
	}
</script>

{#if paths && (paths.unit_mismatch || paths.process_mismatch)}
	<div class="alert-box {paths.unit_mismatch ? 'error' : 'primary'} mb-4">
		<div class="flex items-start gap-3">
			{#if paths.unit_mismatch}
				<svg
					class="w-5 h-5 text-[var(--ctp-red)] flex-shrink-0 mt-0.5"
					fill="none"
					stroke="currentColor"
					viewBox="0 0 24 24"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
					/>
				</svg>
			{:else}
				<svg
					class="w-5 h-5 text-[var(--ctp-primary)] flex-shrink-0 mt-0.5"
					fill="none"
					stroke="currentColor"
					viewBox="0 0 24 24"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
					/>
				</svg>
			{/if}
			<div class="flex-1 min-w-0">
				<!-- Disagreement with the unit: every edit lands in a file the unit
				     never hands the process, and both cures change a path. -->
				{#if paths.unit_mismatch}
					<p class="text-sm font-medium text-[var(--ctp-text)]">{$t('configMismatch.title')}</p>
					<p class="text-xs text-[var(--ctp-subtext0)] mt-1 break-words">
						{$t('configMismatch.body', {
							values: { ours: paths.ours, unit: paths.unit, service: serviceName }
						})}
					</p>
					{#if foreignUnit}
						<p class="text-xs text-[var(--ctp-red)] mt-2 break-words">
							{$t('configMismatch.foreignUnit', { values: { service: serviceName } })}
						</p>
					{/if}
					<div class="flex flex-wrap gap-2 mt-3">
						<button
							class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
							disabled={busy !== ''}
							onclick={useUnitConfig}
						>
							{busy === 'use' ? $t('common.applying') : $t('configMismatch.useUnit')}
						</button>
						<button
							class="px-3 py-1.5 text-sm bg-[var(--ctp-surface2)] text-[var(--ctp-text)] rounded hover:bg-[var(--ctp-overlay0)] disabled:opacity-50 transition-colors"
							disabled={busy !== ''}
							onclick={fixUnit}
						>
							{busy === 'fix'
								? $t('common.applying')
								: $t('configMismatch.fixUnit', { values: { service: serviceName } })}
						</button>
					</div>
					<p class="text-xs text-[var(--ctp-overlay1)] mt-2 break-words">
						{$t('configMismatch.useUnitHint', { values: { service: serviceName } })}
					</p>
				{/if}

				<!-- Disagreement with the LIVE PROCESS: it reads the file it was
				     started with, whatever the unit says now. The only cure is a
				     restart, so there is deliberately no "adopt this path" button
				     here — that is the one that used to undo a just-fixed unit. -->
				{#if paths.process_mismatch}
					<p
						class="text-sm font-medium text-[var(--ctp-text)]"
						class:mt-4={paths.unit_mismatch}
					>
						{$t('configMismatch.restartTitle')}
					</p>
					<p class="text-xs text-[var(--ctp-subtext0)] mt-1 break-words">
						{$t('configMismatch.restartBody', {
							values: { ours: paths.ours, process: paths.process }
						})}
					</p>
					<!-- No restart button while the unit disagrees: the backend
					     refuses Start/Restart/Reload until that is resolved, so the
					     button could only ever produce an error. Resolving the unit
					     first is the order the two cures have to happen in. -->
					{#if !paths.unit_mismatch}
						<div class="flex flex-wrap gap-2 mt-3">
							<button
								class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded hover:opacity-90 disabled:opacity-50 transition-opacity"
								disabled={busy !== ''}
								onclick={restartNow}
							>
								{busy === 'restart' ? $t('common.restarting') : $t('configMismatch.restartNow')}
							</button>
						</div>
					{/if}
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- The drop-in RouteBox installed, shown for as long as the file exists — also
     when nothing disagrees any more, which is the normal state after a
     successful fix. Muted on purpose: it is a standing note about a file, not an
     alert. Without it the override in the unit has no explanation anywhere, and
     no way off except rm + daemon-reload by hand. -->
{#if dropIn}
	<div class="alert-box mb-4 border-[var(--ctp-surface2)] bg-[var(--ctp-surface0)]">
		<div class="flex items-start gap-3">
			<svg
				class="w-5 h-5 text-[var(--ctp-overlay1)] flex-shrink-0 mt-0.5"
				fill="none"
				stroke="currentColor"
				viewBox="0 0 24 24"
			>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="2"
					d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
				/>
			</svg>
			<div class="flex-1 min-w-0">
				<p class="text-sm font-medium text-[var(--ctp-text)]">
					{$t('configDropIn.title', { values: { service: serviceName } })}
				</p>
				<p class="text-xs text-[var(--ctp-subtext0)] mt-1 break-words">
					{#if dropIn.config_path}
						{$t('configDropIn.body', {
							values: { file: dropIn.path, path: dropIn.config_path, service: serviceName }
						})}
					{:else}
						{$t('configDropIn.bodyUnknown', {
							values: { file: dropIn.path, service: serviceName }
						})}
					{/if}
				</p>
				<!-- Written, but systemd has not read it yet: it applies at the next
				     daemon-reload or reboot, so it must not pass for "nothing
				     happened". -->
				{#if dropIn.pending_reload}
					<p class="text-xs text-[var(--ctp-primary)] mt-2 break-words">
						{$t('configDropIn.pending', { values: { unit: paths?.unit ?? '' } })}
					</p>
				{/if}
				<div class="flex flex-wrap gap-2 mt-3">
					<button
						class="px-3 py-1.5 text-sm bg-[var(--ctp-surface2)] text-[var(--ctp-text)] rounded hover:bg-[var(--ctp-overlay0)] disabled:opacity-50 transition-colors"
						disabled={busy !== ''}
						onclick={removeDropIn}
					>
						{busy === 'dropin' ? $t('common.applying') : $t('configDropIn.remove')}
					</button>
				</div>
				<p class="text-xs text-[var(--ctp-overlay1)] mt-2 break-words">
					{$t('configDropIn.removeHint', {
						values: { service: serviceName, ours: paths?.ours ?? '' }
					})}
				</p>
			</div>
		</div>
	</div>
{/if}

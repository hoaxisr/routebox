<script lang="ts">
	import { t } from 'svelte-i18n';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges, configReadOnly } from '$lib/stores';
	import SideBySideDiff from '$lib/components/shared/SideBySideDiff.svelte';

	let loading = $state(true);
	let hasDraft = $state(false);
	let changeCount = $state(0);
	let activeText = $state('');
	let draftText = $state('');
	let showDiff = $state(false);
	let applying = $state(false);
	let discarding = $state(false);

	async function fetchStatus() {
		try {
			const status = await api.getConfigStatus();
			hasDraft = status.hasDraft;
			changeCount = status.changeCount;

			if (hasDraft) {
				const [active, draft] = await Promise.all([
					api.getActiveConfig(),
					api.getConfig()
				]);
				activeText = JSON.stringify(active, null, 2);
				draftText = JSON.stringify(draft, null, 2);
			}
		} catch (err) {
			console.error('Failed to fetch draft status:', err);
		} finally {
			loading = false;
		}
	}

	async function handleApply() {
		applying = true;
		try {
			const result = await api.applyConfig();
			notifications.success($t('changes.configApplied'));
			// duration 0 = stays until dismissed: this is the one place the
			// operator learns that naive is still on the previous user list, and
			// a notice that fades leaves the panel claiming a success it only
			// half had.
			if (result.warning) {
				notifications.warning($t('changes.destWarning', { values: { error: result.warning } }), 0);
			}
			unsavedChanges.clearChanges();
			hasDraft = false;
			activeText = '';
			draftText = '';
		} catch (err) {
			notifications.error(`Failed to apply: ${err}`);
		} finally {
			applying = false;
		}
	}

	async function handleDiscard() {
		discarding = true;
		try {
			await api.discardConfig();
			notifications.success($t('changes.configDiscarded'));
			unsavedChanges.clearChanges();
			hasDraft = false;
			activeText = '';
			draftText = '';
		} catch (err) {
			notifications.error(`Failed to discard: ${err}`);
		} finally {
			discarding = false;
		}
	}

	onMount(() => {
		fetchStatus();
	});
</script>

{#if !loading && hasDraft}
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-6 border-l-4 border-[var(--ctp-yellow)]">
		<div class="flex items-start justify-between gap-4">
			<div class="flex items-start gap-4">
				<div class="p-2 bg-[var(--ctp-yellow)]/20 rounded-lg">
					<svg class="w-5 h-5 text-[var(--ctp-yellow)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
				</div>
				<div>
					<h3 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('changes.pending')}</h3>
					<p class="text-sm text-[var(--ctp-subtext0)] mt-1">
						{#if changeCount > 0}
							{$t('changes.linesChanged', { values: { count: changeCount } })}
						{/if}
					</p>
				</div>
			</div>

			<div class="flex items-center gap-2">
				<button
					type="button"
					onclick={() => showDiff = !showDiff}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{showDiff ? $t('changes.hideDiff') : $t('changes.viewDiff')}
				</button>
				<button
					type="button"
					onclick={handleDiscard}
					disabled={applying || discarding}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
				>
					{$t('changes.discardChanges')}{discarding ? '...' : ''}
				</button>
				<button
					type="button"
					onclick={handleApply}
					disabled={applying || discarding || $configReadOnly}
					title={$configReadOnly ? $t('readOnly.saveBlocked') : ''}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{applying ? $t('common.saving') + '...' : $t('changes.applyChanges')}
				</button>
			</div>
		</div>

		{#if showDiff && activeText && draftText}
			<div class="mt-4">
				<SideBySideDiff oldText={activeText} newText={draftText} maxHeight="400px" />
			</div>
		{/if}
	</div>
{/if}

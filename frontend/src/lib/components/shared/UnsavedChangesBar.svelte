<script lang="ts">
	import { t } from 'svelte-i18n';
	import { unsavedChanges } from '$lib/stores';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import { onMount } from 'svelte';

	let saving = $state(false);
	let discarding = $state(false);
	let showDetails = $state(false);

	// Subscribe to store
	let changesState = $derived($unsavedChanges);

	// Refresh status from backend on mount and after operations
	onMount(() => {
		unsavedChanges.refresh();
	});

	async function handleApply() {
		saving = true;
		try {
			await api.applyConfig();
			notifications.success($t('changes.configApplied'));
			unsavedChanges.clearChanges();
		} catch (err) {
			notifications.error(`Failed to apply: ${err}`);
		} finally {
			saving = false;
		}
	}

	async function handleDiscard() {
		discarding = true;
		try {
			await api.discardConfig();
			notifications.success($t('changes.configDiscarded'));
			unsavedChanges.clearChanges();
		} catch (err) {
			notifications.error(`Failed to discard: ${err}`);
		} finally {
			discarding = false;
		}
	}

	// Display count: prefer backend count, fallback to local changes
	let displayCount = $derived(changesState.changeCount > 0 ? changesState.changeCount : changesState.changes.length);
</script>

{#if changesState.hasChanges}
	<div class="fixed bottom-0 left-0 right-0 z-50 bg-[var(--ctp-yellow)] text-[var(--ctp-base)] shadow-lg border-t-2 border-[var(--ctp-yellow)]">
		<div class="max-w-7xl mx-auto px-4 py-3">
			<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 sm:gap-4">
				<!-- Left: Warning icon and text -->
				<div class="flex items-center gap-3 flex-1 min-w-0">
					<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					<div class="flex-1 min-w-0">
						<span class="font-medium">{$t('changes.pending')}</span>
						{#if displayCount > 0}
							<span class="ml-2 text-sm opacity-80">
								({changesState.changeCount > 0
									? $t('changes.linesChanged', { values: { count: displayCount } })
									: $t('changes.changesCount', { values: { count: displayCount } })})
							</span>
						{/if}
						<a
							href="/config?tab=diff"
							class="ml-2 text-sm underline hover:no-underline"
						>
							{$t('changes.viewDiff')}
						</a>
						{#if changesState.changes.length > 0}
							<button
								type="button"
								onclick={() => showDetails = !showDetails}
								class="ml-2 text-sm underline hover:no-underline"
							>
								{showDetails ? $t('changes.hideDetails') : $t('changes.showDetails')}
							</button>
						{/if}
					</div>
				</div>

				<!-- Right: Action buttons -->
				<div class="flex items-center gap-2 flex-shrink-0 self-end sm:self-auto">
					<button
						type="button"
						onclick={handleDiscard}
						disabled={saving || discarding}
						class="px-4 py-1.5 text-sm font-medium rounded-lg bg-transparent border border-current hover:bg-black/10 transition-colors disabled:opacity-50"
					>
						{$t('changes.discardChanges')}{discarding ? '...' : ''}
					</button>
					<button
						type="button"
						onclick={handleApply}
						disabled={saving || discarding}
						class="px-4 py-1.5 text-sm font-medium rounded-lg bg-[var(--ctp-base)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface0)] transition-colors disabled:opacity-50 flex items-center gap-2"
					>
						{#if saving}
							<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
							</svg>
							{$t('common.saving')}...
						{:else}
							{$t('changes.applyChanges')}
						{/if}
					</button>
				</div>
			</div>

			<!-- Details dropdown -->
			{#if showDetails && changesState.changes.length > 0}
				<div class="mt-3 pt-3 border-t border-black/20">
					<ul class="text-sm space-y-1">
						{#each changesState.changes as change}
							<li class="flex items-center gap-2">
								<span class="w-2 h-2 rounded-full bg-current"></span>
								<span class="font-medium">{change.section}:</span>
								<span>{change.description}</span>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	</div>
{/if}

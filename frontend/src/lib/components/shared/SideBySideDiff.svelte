<script lang="ts">
	import { t } from 'svelte-i18n';
	import { computeSideBySideDiff, type DiffLine, type DiffSegment } from '$lib/utils/diff';

	interface Props {
		oldText: string;
		newText: string;
		oldLabel?: string;
		newLabel?: string;
		maxHeight?: string;
	}

	let {
		oldText,
		newText,
		oldLabel = $t('changes.activeConfig'),
		newLabel = $t('changes.draftConfig'),
		maxHeight = 'calc(100vh - 280px)'
	}: Props = $props();

	let lines = $derived(computeSideBySideDiff(oldText, newText));

	let stats = $derived.by(() => {
		let added = 0, removed = 0, changed = 0;
		for (const l of lines) {
			if (l.type === 'added') added++;
			else if (l.type === 'removed') removed++;
			else if (l.type === 'changed') changed++;
		}
		return { added, removed, changed };
	});
</script>

<div class="diff-container bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
	<!-- Header -->
	<div class="px-4 py-2.5 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
		<div class="flex items-center gap-4 text-xs">
			{#if stats.added > 0}
				<span class="text-[var(--ctp-green)]">+{stats.added} {$t('changes.added')}</span>
			{/if}
			{#if stats.removed > 0}
				<span class="text-[var(--ctp-red)]">-{stats.removed} {$t('changes.removed')}</span>
			{/if}
			{#if stats.changed > 0}
				<span class="text-[var(--ctp-yellow)]">~{stats.changed} {$t('changes.modified')}</span>
			{/if}
		</div>
		<div class="flex items-center gap-3 text-xs text-[var(--ctp-overlay0)]">
			<span class="flex items-center gap-1">
				<span class="w-3 h-3 rounded bg-[var(--ctp-green)]/20 border border-[var(--ctp-green)]"></span>
				{$t('changes.added')}
			</span>
			<span class="flex items-center gap-1">
				<span class="w-3 h-3 rounded bg-[var(--ctp-red)]/20 border border-[var(--ctp-red)]"></span>
				{$t('changes.removed')}
			</span>
			<span class="flex items-center gap-1">
				<span class="w-3 h-3 rounded bg-[var(--ctp-yellow)]/20 border border-[var(--ctp-yellow)]"></span>
				{$t('changes.modified')}
			</span>
		</div>
	</div>

	<!-- Column headers -->
	<div class="grid grid-cols-2 border-b border-[var(--ctp-surface2)] text-xs font-medium text-[var(--ctp-overlay1)]">
		<div class="px-4 py-1.5 border-r border-[var(--ctp-surface2)] bg-[var(--ctp-mantle)]">
			{oldLabel}
		</div>
		<div class="px-4 py-1.5 bg-[var(--ctp-mantle)]">
			{newLabel}
		</div>
	</div>

	<!-- Diff content -->
	<div class="overflow-auto font-mono text-xs leading-5" style="max-height: {maxHeight}">
		{#each lines as line}
			{#if line.type === 'same'}
				<div class="grid grid-cols-2">
					<div class="diff-line px-2 border-r border-[var(--ctp-surface2)]">
						<span class="diff-num">{line.leftNum}</span>
						<span class="diff-text text-[var(--ctp-overlay1)]">{line.leftText}</span>
					</div>
					<div class="diff-line px-2">
						<span class="diff-num">{line.rightNum}</span>
						<span class="diff-text text-[var(--ctp-overlay1)]">{line.rightText}</span>
					</div>
				</div>
			{:else if line.type === 'removed'}
				<div class="grid grid-cols-2">
					<div class="diff-line diff-line-removed px-2 border-r border-[var(--ctp-surface2)]">
						<span class="diff-num">{line.leftNum}</span>
						<span class="diff-text">{line.leftText}</span>
					</div>
					<div class="diff-line diff-line-empty px-2">
						<span class="diff-num"></span>
						<span class="diff-text"></span>
					</div>
				</div>
			{:else if line.type === 'added'}
				<div class="grid grid-cols-2">
					<div class="diff-line diff-line-empty px-2 border-r border-[var(--ctp-surface2)]">
						<span class="diff-num"></span>
						<span class="diff-text"></span>
					</div>
					<div class="diff-line diff-line-added px-2">
						<span class="diff-num">{line.rightNum}</span>
						<span class="diff-text">{line.rightText}</span>
					</div>
				</div>
			{:else if line.type === 'changed'}
				<div class="grid grid-cols-2">
					<div class="diff-line diff-line-changed-old px-2 border-r border-[var(--ctp-surface2)]">
						<span class="diff-num">{line.leftNum}</span>
						<span class="diff-text">
							{#if line.leftSegments}
								{#each line.leftSegments as seg}
									{#if seg.highlight}
										<span class="diff-highlight-old">{seg.text}</span>
									{:else}
										{seg.text}
									{/if}
								{/each}
							{:else}
								{line.leftText}
							{/if}
						</span>
					</div>
					<div class="diff-line diff-line-changed-new px-2">
						<span class="diff-num">{line.rightNum}</span>
						<span class="diff-text">
							{#if line.rightSegments}
								{#each line.rightSegments as seg}
									{#if seg.highlight}
										<span class="diff-highlight-new">{seg.text}</span>
									{:else}
										{seg.text}
									{/if}
								{/each}
							{:else}
								{line.rightText}
							{/if}
						</span>
					</div>
				</div>
			{/if}
		{/each}
	</div>
</div>

<style>
	.diff-line {
		display: flex;
		align-items: baseline;
		min-height: 1.25rem;
		white-space: pre;
	}
	.diff-num {
		width: 3rem;
		flex-shrink: 0;
		text-align: right;
		padding-right: 0.75rem;
		color: var(--ctp-overlay0);
		user-select: none;
	}
	.diff-text {
		flex: 1;
		overflow: hidden;
	}

	/* Removed line (left side) */
	.diff-line-removed {
		background-color: color-mix(in srgb, var(--ctp-red) 10%, transparent);
		color: var(--ctp-red);
	}

	/* Added line (right side) */
	.diff-line-added {
		background-color: color-mix(in srgb, var(--ctp-green) 10%, transparent);
		color: var(--ctp-green);
	}

	/* Changed line - old (left) */
	.diff-line-changed-old {
		background-color: color-mix(in srgb, var(--ctp-yellow) 8%, transparent);
		color: var(--ctp-text);
	}

	/* Changed line - new (right) */
	.diff-line-changed-new {
		background-color: color-mix(in srgb, var(--ctp-yellow) 8%, transparent);
		color: var(--ctp-text);
	}

	/* Word-level highlighting */
	.diff-highlight-old {
		background-color: color-mix(in srgb, var(--ctp-red) 30%, transparent);
		border-radius: 2px;
		padding: 0 1px;
	}
	.diff-highlight-new {
		background-color: color-mix(in srgb, var(--ctp-green) 30%, transparent);
		border-radius: 2px;
		padding: 0 1px;
	}

	/* Empty placeholder line */
	.diff-line-empty {
		background-color: color-mix(in srgb, var(--ctp-surface2) 30%, transparent);
	}
</style>

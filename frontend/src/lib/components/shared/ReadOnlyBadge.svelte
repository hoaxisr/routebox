<script lang="ts">
	import { t } from 'svelte-i18n';
	import { anyReadOnly, readOnlyPaths } from '$lib/stores';

	// Deliberately a badge and not a banner. The config-path mismatch already
	// owns the banner slot above the page, and the two conditions can be up at
	// the same time; a second full-width block would turn the top of every page
	// into a wall of warnings. The mismatch is actionable from the UI (it has
	// buttons), read-only is not — it needs a shell — so it gets the quieter
	// spot in the header chrome, always visible, never in the way.

	// The badge opens. It used to carry the path in a `title` alone, which is a
	// mouse-only affordance: on a phone the one actionable fact — WHICH file to
	// make writable — was unreachable. The title stays for pointer users; the
	// panel is what makes the same facts readable by tapping.
	let open = $state(false);

	// The backend's own message is English; we only reuse the paths from it.
	let tooltip = $derived(
		[
			$t('readOnly.reason'),
			...$readOnlyPaths.map((p) => $t('readOnly.path', { values: { path: p } })),
			$t('readOnly.hint')
		]
			.filter(Boolean)
			.join('\n')
	);

	function onWindowKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') open = false;
	}
</script>

<svelte:window onkeydown={onWindowKeydown} />

{#if $anyReadOnly}
	<div class="relative">
		<button
			type="button"
			onclick={() => (open = !open)}
			class="status-badge error flex items-center gap-1.5 whitespace-nowrap cursor-pointer"
			title={tooltip}
			aria-expanded={open}
			aria-label={$t('readOnly.details')}
		>
			<svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="2"
					d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
				/>
			</svg>
			<span class="hidden sm:inline">{$t('readOnly.badge')}</span>
			{#if $readOnlyPaths.length > 1}
				<span class="tabular-nums">({$readOnlyPaths.length})</span>
			{/if}
		</button>

		{#if open}
			<!-- Click-away layer: a plain overlay beats a document listener here,
			     it cannot outlive the panel. -->
			<button
				type="button"
				class="fixed inset-0 z-40 cursor-default"
				aria-label={$t('common.close')}
				onclick={() => (open = false)}
			></button>
			<div
				class="absolute right-0 top-full mt-2 z-50 w-[min(22rem,calc(100vw-2rem))] rounded-lg border border-[var(--ctp-surface1)] bg-[var(--ctp-base)] p-3 shadow-lg text-left"
			>
				<p class="text-sm text-[var(--ctp-text)]">{$t('readOnly.reason')}</p>
				<p class="mt-3 text-xs font-medium uppercase tracking-wide text-[var(--ctp-subtext0)]">
					{$t('readOnly.paths')}
				</p>
				<ul class="mt-1 space-y-1">
					{#each $readOnlyPaths as path (path)}
						<li class="font-mono text-xs text-[var(--ctp-text)] break-all">{path}</li>
					{/each}
				</ul>
				<p class="mt-3 text-xs text-[var(--ctp-subtext1)]">{$t('readOnly.hint')}</p>
			</div>
		{/if}
	</div>
{/if}

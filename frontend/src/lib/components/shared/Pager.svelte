<script lang="ts">
	// ‹ 2/5 › for the dashboard lists (#110): they page instead of scrolling.
	// Hidden when everything fits on one page.
	let { page = $bindable(0), pages }: { page: number; pages: number } = $props();
	// The live lists shrink under the reader: stay on the last page that exists.
	$effect(() => {
		if (page > 0 && page >= pages) page = Math.max(0, pages - 1);
	});
</script>

{#if pages > 1}
	<span class="inline-flex items-center gap-1 shrink-0 text-xs tabular-nums text-[var(--ctp-overlay1)]">
		<button type="button" class="px-1.5 rounded hover:text-[var(--ctp-text)] disabled:opacity-30" disabled={page === 0} onclick={() => (page -= 1)} aria-label="Previous page">‹</button>
		<span>{page + 1}/{pages}</span>
		<button type="button" class="px-1.5 rounded hover:text-[var(--ctp-text)] disabled:opacity-30" disabled={page >= pages - 1} onclick={() => (page += 1)} aria-label="Next page">›</button>
	</span>
{/if}

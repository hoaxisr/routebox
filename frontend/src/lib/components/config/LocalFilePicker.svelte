<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { formatBytes } from '$lib/stores/settings';
	import { formatError } from '$lib/utils/errorHandler';
	import type { FsListing } from '$lib/types';

	interface Props {
		// Current path from the field: the picker opens in its directory.
		start: string;
		onPick: (path: string) => void;
		onCancel: () => void;
	}

	let { start, onPick, onCancel }: Props = $props();

	let listing = $state<FsListing | null>(null);
	let selected = $state('');
	let error = $state('');

	const join = (dir: string, name: string) => (dir.endsWith('/') ? dir : dir + '/') + name;

	async function open(path: string, fallback = '') {
		error = '';
		try {
			listing = await api.listDir(path);
			selected = '';
		} catch (e) {
			// A stale path in the field (folder gone) falls back to the config dir.
			if (fallback !== path) return open(fallback, fallback);
			error = formatError(e);
		}
	}

	// File path → its directory; a bare value with no slash → config dir.
	const startDir = start.includes('/') ? start.slice(0, start.lastIndexOf('/')) || '/' : '';
	open(startDir).then(() => {
		if (listing && start && join(listing.path, start.split('/').pop()!) === start) selected = start;
	});

	const crumbs = $derived.by(() => {
		if (!listing) return [];
		const parts = listing.path.split('/').filter(Boolean);
		return [{ name: '/', path: '/' }, ...parts.map((p, i) => ({ name: p, path: '/' + parts.slice(0, i + 1).join('/') }))];
	});
</script>

<div class="border border-[var(--ctp-surface2)] rounded-lg p-3 mt-2 bg-[var(--ctp-mantle)]">
	<div class="flex flex-wrap items-center gap-1 text-sm mb-2">
		{#each crumbs as c, i}
			{#if i > 1}<span class="text-[var(--ctp-overlay0)]">›</span>{/if}
			<button type="button" class="hover:text-[var(--ctp-primary)] {i === crumbs.length - 1 ? 'font-semibold' : 'text-[var(--ctp-subtext1)]'}" onclick={() => open(c.path)}>{c.name}</button>
		{/each}
	</div>

	{#if error}
		<p class="text-sm text-[var(--ctp-red)]">{error}</p>
	{:else if listing}
		<ul class="max-h-64 overflow-y-auto text-sm">
			{#if listing.parent}
				<li><button type="button" class="picker-row" onclick={() => open(listing!.parent!)}>📁 ..</button></li>
			{/if}
			{#each listing.entries as e (e.name)}
				{@const full = join(listing.path, e.name)}
				<li>
					{#if e.dir}
						<button type="button" class="picker-row" onclick={() => open(full)}>📁 {e.name}</button>
					{:else}
						<button
							type="button"
							class="picker-row {selected === full ? 'bg-[var(--ctp-primary)]/20' : ''}"
							onclick={() => (selected = full)}
							ondblclick={() => onPick(full)}
						>
							📄 {e.name} <span class="text-[var(--ctp-overlay0)]">· {formatBytes(e.size ?? 0)}</span>
						</button>
					{/if}
				</li>
			{/each}
			{#if !listing.entries.length}
				<li class="px-2 py-1 text-[var(--ctp-overlay0)]">{$t('routes.localPicker.empty')}</li>
			{/if}
		</ul>
	{/if}

	<p class="mt-2 text-xs text-[var(--ctp-overlay0)]">{$t('routes.localPicker.hint')}</p>
	<div class="flex justify-end gap-2 mt-2">
		<button type="button" class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] rounded-lg hover:bg-[var(--ctp-surface2)]" onclick={onCancel}>{$t('common.cancel')}</button>
		<button type="button" class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg disabled:opacity-50" disabled={!selected} onclick={() => onPick(selected)}>{$t('routes.localPicker.select')}</button>
	</div>
</div>

<style>
	.picker-row {
		width: 100%;
		text-align: left;
		padding: 0.25rem 0.5rem;
		border-radius: 0.375rem;
	}
	.picker-row:hover {
		background: var(--ctp-surface0);
	}
</style>

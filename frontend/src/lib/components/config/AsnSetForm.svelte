<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, configReadOnly } from '$lib/stores';
	import type { AsnSet } from '$lib/types';
	import { ASN_INTERVALS, parseAsn, formatAsn, splitAsnInput } from '$lib/utils/asn';

	// ASN rule set (#103): the form talks to the ASN endpoints itself.
	// Create adds a `local` entry to the config draft; edit only rewrites
	// the prefix file, so the tag is frozen while editing.
	interface Props {
		existingTags: string[];
		asnSet?: AsnSet | null;
		onSaved: (s: AsnSet) => void;
		onCancel: () => void;
	}
	let { existingTags, asnSet = null, onSaved, onCancel }: Props = $props();
	const editing = !!asnSet;

	let tag = $state(asnSet?.tag ?? '');
	let asns = $state<number[]>(asnSet?.asns ?? []);
	let holders = $state<Record<string, string>>(asnSet?.holders ?? {});
	let interval = $state<number>(asnSet?.interval_hrs ?? 24);
	let draft = $state('');
	let inputError = $state('');
	let tagError = $state('');
	let serverError = $state('');
	let saving = $state(false);

	function addFromDraft() {
		inputError = '';
		for (const part of splitAsnInput(draft)) {
			const n = parseAsn(part);
			if (n === null) {
				inputError = $t('asnSets.invalid', { values: { value: part } });
				return;
			}
			if (!asns.includes(n)) asns = [...asns, n];
		}
		draft = '';
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			addFromDraft();
		}
	}
	const remove = (n: number) => (asns = asns.filter((a) => a !== n));

	async function submit() {
		if (draft.trim()) addFromDraft();
		if (inputError) return;
		tagError = '';
		const tg = tag.trim();
		if (!editing && (!/^[A-Za-z0-9._-]{1,64}$/.test(tg) || tg === '.' || tg === '..')) {
			tagError = $t('dns.validation.tagRequired');
			return;
		}
		if (!editing && existingTags.includes(tg)) {
			tagError = $t('dns.validation.tagExists');
			return;
		}
		if (asns.length === 0) {
			inputError = $t('asnSets.numbersHint');
			return;
		}
		saving = true;
		serverError = '';
		try {
			const body = { asns: asns.map(formatAsn), interval_hrs: interval };
			const s = editing ? await api.updateAsnSet(tg, body) : await api.createAsnSet({ tag: tg, ...body });
			holders = s.holders;
			onSaved(s);
		} catch (e) {
			serverError = e instanceof Error ? e.message : String(e);
			notifications.error(serverError);
		} finally {
			saving = false;
		}
	}
</script>

<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
	<div>
		<label for="asn-tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
		<input
			id="asn-tag"
			type="text"
			bind:value={tag}
			disabled={editing}
			placeholder="cloudflare"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] disabled:opacity-60 {tagError ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if tagError}<p class="text-xs text-[var(--ctp-red)] mt-1">{tagError}</p>{/if}
	</div>

	<div>
		<label for="asn-input" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('asnSets.numbers')} *</label>
		{#if asns.length > 0}
			<div class="flex flex-wrap gap-1.5 mb-2">
				{#each asns as n (n)}
					<span class="status-badge info inline-flex items-center gap-1 max-w-full">
						<span class="whitespace-nowrap">{formatAsn(n)}</span>
						{#if holders[String(n)]}<span class="text-[var(--ctp-overlay1)] truncate min-w-0">— {holders[String(n)]}</span>{/if}
						<button type="button" class="ml-0.5 px-0.5 leading-none hover:text-[var(--ctp-red)]" aria-label={$t('asnSets.removeAsn')} onclick={() => remove(n)}>×</button>
					</span>
				{/each}
			</div>
		{/if}
		<input
			id="asn-input"
			type="text"
			bind:value={draft}
			onkeydown={onKey}
			onblur={() => draft.trim() && addFromDraft()}
			placeholder="AS13335"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {inputError ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		<p class="text-xs mt-1 {inputError ? 'text-[var(--ctp-red)]' : 'text-[var(--ctp-overlay0)]'}">{inputError || $t('asnSets.numbersHint')}</p>
	</div>

	<div>
		<span class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('asnSets.interval')}</span>
		<div class="flex gap-2">
			{#each ASN_INTERVALS as h (h)}
				<button type="button" class="toggle-btn whitespace-nowrap {interval === h ? 'selected' : ''}" onclick={() => (interval = h)}>
					{h < 168 ? $t('asnSets.intervalH', { values: { n: h } }) : $t('asnSets.intervalD', { values: { n: 7 } })}
				</button>
			{/each}
		</div>
	</div>

	<!-- RIPEstat errors name the AS ("AS64512: announces no prefixes"), so the
	     message under the form already points at the offending number. -->
	{#if serverError}<p class="text-sm text-[var(--ctp-red)] break-words">{serverError}</p>{/if}

	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button type="button" onclick={onCancel} class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">{$t('common.cancel')}</button>
		<button
			type="submit"
			disabled={saving || (!editing && $configReadOnly)}
			title={!editing && $configReadOnly ? $t('readOnly.saveBlocked') : ''}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
		>
			{editing ? $t('asnSets.save') : $t('asnSets.create')}
		</button>
	</div>
</form>

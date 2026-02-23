<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { RuleSet, RuleSetUsage, Outbound, Endpoint } from '$lib/types';
	import RuleSetForm from '$lib/components/config/RuleSetForm.svelte';

	let ruleSets = $state<RuleSet[]>([]);
	let usage = $state<Record<string, RuleSetUsage>>({});
	let outbounds = $state<Outbound[]>([]);
	let endpoints = $state<Endpoint[]>([]);
	let loading = $state(true);

	// Combined list: outbounds + endpoints (endpoints can be used as detour targets)
	let allOutbounds = $derived([
		...outbounds,
		...endpoints.map(e => ({ tag: e.tag, type: e.type } as Outbound))
	]);

	// Modal state
	let showForm = $state(false);
	let editingRuleSet = $state<RuleSet | null>(null);
	let viewingRuleSet = $state<RuleSet | null>(null);

	async function fetchData() {
		try {
			const [rs, us, ob, ep] = await Promise.all([
				api.listRuleSets(),
				api.getRuleSetsUsage(),
				api.listOutbounds(),
				api.listEndpoints()
			]);
			ruleSets = rs;
			usage = us;
			outbounds = ob;
			endpoints = ep;
		} catch (e) {
			notifications.error($t('errors.loadFailed') + `: ${e}`);
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingRuleSet = null;
		showForm = true;
	}

	function openEdit(ruleSet: RuleSet) {
		editingRuleSet = ruleSet;
		showForm = true;
	}

	function closeForm() {
		showForm = false;
		editingRuleSet = null;
	}

	async function handleSave(ruleSet: RuleSet) {
		try {
			if (editingRuleSet) {
				// Delete old and create new (no update endpoint for rule sets)
				await api.deleteRuleSet(editingRuleSet.tag);
				await api.createRuleSet(ruleSet);
				ruleSets = ruleSets.map(rs => rs.tag === editingRuleSet!.tag ? ruleSet : rs);
				unsavedChanges.markChanged($t('ruleSets.title'), `${$t('common.update')} "${ruleSet.tag}"`);
				notifications.success($t('common.saved'));
			} else {
				await api.createRuleSet(ruleSet);
				ruleSets = [...ruleSets, ruleSet];
				unsavedChanges.markChanged($t('ruleSets.title'), `${$t('common.create')} "${ruleSet.tag}"`);
				notifications.success($t('ruleSets.ruleSetCreated'));
			}
			// Refresh usage
			usage = await api.getRuleSetsUsage();
			closeForm();
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleDelete(tag: string) {
		if (!confirm($t('ruleSets.deleteConfirm', { values: { tag } }))) return;
		try {
			await api.deleteRuleSet(tag);
			ruleSets = ruleSets.filter(rs => rs.tag !== tag);
			delete usage[tag];
			usage = { ...usage };
			unsavedChanges.markChanged($t('ruleSets.title'), `${$t('common.delete')} "${tag}"`);
			notifications.success($t('ruleSets.ruleSetDeleted'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function getUsageCount(tag: string): { route: number; dns: number } {
		const u = usage[tag];
		if (!u) return { route: 0, dns: 0 };
		return { route: u.route_rules.length, dns: u.dns_rules.length };
	}

	function isUnused(tag: string): boolean {
		const { route, dns } = getUsageCount(tag);
		return route === 0 && dns === 0;
	}

	onMount(fetchData);
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('ruleSets.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('ruleSets.description')}</p>
		</div>
		<button
			onclick={openCreate}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity flex items-center gap-2"
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
			</svg>
			{$t('ruleSets.createNew')}
		</button>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if ruleSets.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<svg class="w-12 h-12 mx-auto text-[var(--ctp-overlay0)] mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
			</svg>
			<p class="text-[var(--ctp-overlay1)]">{$t('routes.noRuleSets')}</p>
			<p class="text-sm text-[var(--ctp-overlay0)] mt-1">{$t('routes.noRuleSetsHint')}</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each ruleSets as ruleSet}
				{@const counts = getUsageCount(ruleSet.tag)}
				{@const unused = isUnused(ruleSet.tag)}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 group {unused ? 'opacity-70' : ''}">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-3 min-w-0 flex-1">
							<!-- Type badge -->
							<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] flex-shrink-0">
								{ruleSet.type}
							</span>
							<!-- Tag -->
							<span class="font-medium text-[var(--ctp-text)] truncate">{ruleSet.tag}</span>
							<!-- Format badge -->
							{#if ruleSet.format}
								<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface1)] text-[var(--ctp-overlay0)] flex-shrink-0">
									{ruleSet.format}
								</span>
							{/if}
						</div>

						<!-- Usage badges -->
						<div class="flex items-center gap-2 mr-3">
							{#if counts.route > 0}
								<span class="status-badge" title="{$t('ruleSets.routeRules')}">
									{$t('ruleSets.routeRules')}: {counts.route}
								</span>
							{/if}
							{#if counts.dns > 0}
								<span class="status-badge info" title="{$t('ruleSets.dnsRules')}">
									{$t('ruleSets.dnsRules')}: {counts.dns}
								</span>
							{/if}
							{#if unused}
								<span class="text-xs text-[var(--ctp-overlay0)] italic">
									{$t('ruleSets.unusedWarning')}
								</span>
							{/if}
						</div>

						<!-- Actions -->
						<div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
							<button
								onclick={() => viewingRuleSet = ruleSet}
								class="p-1.5 rounded hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)]"
								title={$t('routes.viewDetails')}
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
								</svg>
							</button>
							<button
								onclick={() => openEdit(ruleSet)}
								class="p-1.5 rounded hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)]"
								title={$t('common.edit')}
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
								</svg>
							</button>
							<button
								onclick={() => handleDelete(ruleSet.tag)}
								class="p-1.5 rounded hover:bg-[var(--ctp-red)] hover:bg-opacity-20 text-[var(--ctp-red)]"
								title={$t('common.delete')}
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						</div>
					</div>

					<!-- URL/Path info -->
					{#if ruleSet.url}
						<p class="mt-2 text-xs text-[var(--ctp-overlay0)] truncate" title={ruleSet.url}>{ruleSet.url}</p>
					{:else if ruleSet.path}
						<p class="mt-2 text-xs text-[var(--ctp-overlay0)] font-mono truncate" title={ruleSet.path}>{ruleSet.path}</p>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create/Edit Form Modal -->
{#if showForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">
					{editingRuleSet ? `${$t('routes.editRuleSet')}: ${editingRuleSet.tag}` : $t('routes.addRuleSet')}
				</h2>
				<button
					onclick={closeForm}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<RuleSetForm
					existingTags={ruleSets.filter(rs => rs.tag !== editingRuleSet?.tag).map(rs => rs.tag)}
					outbounds={allOutbounds}
					ruleSet={editingRuleSet}
					onSave={handleSave}
					onCancel={closeForm}
				/>
			</div>
		</div>
	</div>
{/if}

<!-- View Details Modal -->
{#if viewingRuleSet}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-md">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('routes.ruleSetDetails')}</h2>
				<button
					onclick={() => viewingRuleSet = null}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label="Close"
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4 space-y-3">
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('common.tag')}</span>
					<p class="font-medium text-[var(--ctp-text)]">{viewingRuleSet.tag}</p>
				</div>
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('common.type')}</span>
					<p class="text-[var(--ctp-text)]">{viewingRuleSet.type}</p>
				</div>
				<div>
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetFormat')}</span>
					<p class="text-[var(--ctp-text)]">{viewingRuleSet.format}</p>
				</div>
				{#if viewingRuleSet.url}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetUrl')}</span>
						<p class="text-[var(--ctp-primary)] break-all text-sm">{viewingRuleSet.url}</p>
					</div>
				{/if}
				{#if viewingRuleSet.path}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.ruleSetPath')}</span>
						<p class="text-[var(--ctp-text)] font-mono text-sm">{viewingRuleSet.path}</p>
					</div>
				{/if}
				{#if viewingRuleSet.download_detour}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.downloadDetour')}</span>
						<p class="text-[var(--ctp-text)]">{viewingRuleSet.download_detour}</p>
					</div>
				{/if}
				{#if viewingRuleSet.update_interval}
					<div>
						<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.updateInterval')}</span>
						<p class="text-[var(--ctp-text)]">{viewingRuleSet.update_interval}</p>
					</div>
				{/if}
				<!-- Usage info -->
				<div class="border-t border-[var(--ctp-surface2)] pt-3">
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('ruleSets.usage')}</span>
					<div class="flex gap-3 mt-1">
						<span class="text-sm text-[var(--ctp-text)]">{$t('ruleSets.routeRules')}: {getUsageCount(viewingRuleSet.tag).route}</span>
						<span class="text-sm text-[var(--ctp-text)]">{$t('ruleSets.dnsRules')}: {getUsageCount(viewingRuleSet.tag).dns}</span>
					</div>
				</div>
			</div>
			<div class="px-4 py-3 border-t border-[var(--ctp-surface2)] flex justify-end gap-2">
				<button
					onclick={() => { openEdit(viewingRuleSet!); viewingRuleSet = null; }}
					class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90"
				>
					{$t('common.edit')}
				</button>
				<button
					onclick={() => viewingRuleSet = null}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]"
				>
					{$t('common.close')}
				</button>
			</div>
		</div>
	</div>
{/if}

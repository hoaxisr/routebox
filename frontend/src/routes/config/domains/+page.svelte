<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import { t } from 'svelte-i18n';
	import type { DomainSetInfo, RuleSetSource } from '$lib/types';

	let sets = $state<DomainSetInfo[]>([]);
	let loading = $state(true);
	let selectedTag = $state<string | null>(null);
	let selectedSet = $state<RuleSetSource | null>(null);
	let loadingSet = $state(false);

	// Create set modal
	let showCreateModal = $state(false);
	let newTag = $state('');
	let creating = $state(false);

	// Combined input: adds domain on Enter, filters list as you type
	let inputValue = $state('');
	let addingDomain = $state(false);

	// Import modal
	let showImportModal = $state(false);
	let importText = $state('');
	let importing = $state(false);

	// Tab state
	let activeTab = $state<'domains' | 'json'>('domains');
	let jsonText = $state('');
	let jsonError = $state('');
	let savingJson = $state(false);

	// Delete confirmation
	let deleteConfirmTag = $state<string | null>(null);

	async function fetchSets() {
		try {
			sets = await api.listDomainSets();
			// Auto-select first set if none selected
			if (sets.length > 0 && !selectedTag) {
				selectSet(sets[0].tag);
			}
		} catch (e) {
			notifications.error(`${$t('errors.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	async function selectSet(tag: string) {
		selectedTag = tag;
		loadingSet = true;
		inputValue = '';
		try {
			selectedSet = await api.getDomainSet(tag);
			jsonText = JSON.stringify(selectedSet, null, 2);
			jsonError = '';
		} catch (e) {
			notifications.error(`${e}`);
			selectedSet = null;
		} finally {
			loadingSet = false;
		}
	}

	async function createSet() {
		if (!newTag.trim()) return;
		creating = true;
		try {
			await api.createDomainSet(newTag.trim());
			notifications.success($t('domains.setCreated'));
			showCreateModal = false;
			const createdTag = newTag.trim();
			newTag = '';
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `${$t('common.create')} "${createdTag}"`);
			selectSet(createdTag);
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			creating = false;
		}
	}

	async function deleteSet(tag: string) {
		try {
			await api.deleteDomainSet(tag);
			notifications.success($t('domains.setDeleted'));
			if (selectedTag === tag) {
				selectedTag = null;
				selectedSet = null;
			}
			deleteConfirmTag = null;
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `${$t('common.delete')} "${tag}"`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function addDomain() {
		if (!inputValue.trim() || !selectedTag) return;
		addingDomain = true;
		try {
			await api.addDomain(selectedTag, inputValue.trim());
			inputValue = '';
			await selectSet(selectedTag);
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `add domain to "${selectedTag}"`);
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			addingDomain = false;
		}
	}

	async function removeDomain(domain: string) {
		if (!selectedTag) return;
		try {
			await api.removeDomain(selectedTag, domain);
			await selectSet(selectedTag);
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `remove domain from "${selectedTag}"`);
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function importDomains() {
		if (!importText.trim() || !selectedTag) return;
		importing = true;
		try {
			const domainList = importText
				.split('\n')
				.map((d) => d.trim())
				.filter((d) => d.length > 0);
			const result = await api.importDomains(selectedTag, domainList);
			notifications.success(result.message);
			showImportModal = false;
			importText = '';
			await selectSet(selectedTag);
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `import domains into "${selectedTag}"`);
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			importing = false;
		}
	}

	async function saveJson() {
		if (!selectedTag) return;
		savingJson = true;
		jsonError = '';
		try {
			const parsed = JSON.parse(jsonText) as RuleSetSource;
			await api.saveDomainSet(selectedTag, parsed);
			notifications.success($t('domains.jsonSaved'));
			await selectSet(selectedTag);
			await fetchSets();
			unsavedChanges.markChanged($t('domains.title'), `${$t('common.update')} "${selectedTag}"`);
		} catch (e) {
			if (e instanceof SyntaxError) {
				jsonError = $t('domains.invalidJson');
			} else {
				notifications.error(`${e}`);
			}
		} finally {
			savingJson = false;
		}
	}

	function handleInputKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && inputValue.trim()) {
			e.preventDefault();
			addDomain();
		}
	}

	function handleCreateKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			createSet();
		}
	}

	// Get all domain_suffix entries from all rules
	let allDomains = $derived.by(() => {
		if (!selectedSet) return [];
		const domains: string[] = [];
		for (const rule of selectedSet.rules) {
			if (rule.domain_suffix) {
				domains.push(...rule.domain_suffix);
			}
		}
		return domains;
	});

	// Filter domains based on input (used for both search and display)
	let filteredDomains = $derived.by(() => {
		if (!inputValue) return allDomains;
		const q = inputValue.toLowerCase();
		return allDomains.filter((d) => d.includes(q));
	});

	onMount(fetchSets);
</script>

<div class="space-y-4">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">{$t('domains.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('domains.description')}</p>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else}
		<!-- Rule Set Tabs -->
		<div class="flex items-center gap-2 flex-wrap">
			{#each sets as set (set.tag)}
				<button
					onclick={() => selectSet(set.tag)}
					class="px-3 py-1.5 text-sm rounded-lg transition-colors flex items-center gap-2 {selectedTag === set.tag
						? 'bg-[var(--ctp-primary)] text-white'
						: 'bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)] hover:text-[var(--ctp-text)]'}"
				>
					<span>{set.tag}</span>
					<span class="text-xs opacity-70">({set.domain_count})</span>
				</button>
			{/each}
			<button
				onclick={() => { showCreateModal = true; newTag = ''; }}
				class="px-3 py-1.5 text-sm rounded-lg bg-[var(--ctp-surface0)] text-[var(--ctp-overlay1)] hover:bg-[var(--ctp-surface1)] hover:text-[var(--ctp-text)] transition-colors"
			>
				+ {$t('common.create')}
			</button>
		</div>

		{#if !selectedTag}
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-12 text-center">
				<p class="text-[var(--ctp-overlay1)]">{$t('domains.selectOrCreate')}</p>
			</div>
		{:else if loadingSet}
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-12 flex items-center justify-center">
				<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
			</div>
		{:else if selectedSet}
			<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
				<!-- Toolbar -->
				<div class="px-4 py-3 border-b border-[var(--ctp-surface1)] flex items-center justify-between gap-4">
					<div class="flex items-center gap-3">
						<!-- Domains/JSON toggle -->
						<div class="flex rounded-lg overflow-hidden border border-[var(--ctp-surface1)]">
							<button
								onclick={() => (activeTab = 'domains')}
								class="px-3 py-1 text-sm transition-colors {activeTab === 'domains'
									? 'bg-[var(--ctp-primary)] text-white'
									: 'bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)]'}"
							>
								{$t('domains.domainsTab')}
							</button>
							<button
								onclick={() => { activeTab = 'json'; jsonText = JSON.stringify(selectedSet, null, 2); jsonError = ''; }}
								class="px-3 py-1 text-sm transition-colors {activeTab === 'json'
									? 'bg-[var(--ctp-primary)] text-white'
									: 'bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)]'}"
							>
								{$t('domains.jsonTab')}
							</button>
						</div>
					</div>

					<div class="flex items-center gap-2">
						<button
							onclick={() => deleteConfirmTag = selectedTag}
							class="p-1.5 rounded-lg text-[var(--ctp-red)] hover:bg-[var(--ctp-surface1)] transition-colors"
							title={$t('common.delete')}
						>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					</div>
				</div>

				{#if activeTab === 'domains'}
					<!-- Combined input: filter + add -->
					<div class="px-4 py-3 border-b border-[var(--ctp-surface1)]">
						<div class="flex gap-2">
							<input
								type="text"
								bind:value={inputValue}
								onkeydown={handleInputKeydown}
								placeholder="{$t('domains.search')} / {$t('domains.addPlaceholder')}"
								class="flex-1 px-3 py-2 text-sm rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface1)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)]"
							/>
							<button
								onclick={addDomain}
								disabled={addingDomain || !inputValue.trim()}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
							>
								{$t('common.add')}
							</button>
							<button
								onclick={() => { showImportModal = true; importText = ''; }}
								class="px-3 py-2 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)] transition-colors"
							>
								{$t('common.import')}
							</button>
						</div>
						<div class="text-xs text-[var(--ctp-overlay1)] mt-2">
							{filteredDomains.length} {$t('domains.domainsCount')}
							{#if inputValue && filteredDomains.length !== allDomains.length}
								({$t('domains.of')} {allDomains.length})
							{/if}
						</div>
					</div>

					<!-- Domain list -->
					<div class="max-h-96 overflow-y-auto">
						{#if allDomains.length === 0}
							<div class="px-4 py-8 text-center text-[var(--ctp-overlay1)]">
								<p class="text-sm">{$t('domains.noDomains')}</p>
							</div>
						{:else}
							<div class="divide-y divide-[var(--ctp-surface1)]">
								{#each filteredDomains as domain (domain)}
									<div class="px-4 py-2 flex items-center justify-between group hover:bg-[var(--ctp-surface1)] transition-colors">
										<span class="text-sm font-mono text-[var(--ctp-text)]">{domain}</span>
										<button
											onclick={() => removeDomain(domain)}
											class="opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-[var(--ctp-surface2)] transition-all"
											title={$t('common.delete')}
										>
											<svg class="w-4 h-4 text-[var(--ctp-red)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
											</svg>
										</button>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{:else}
					<!-- JSON tab -->
					<div class="flex flex-col">
						<div class="px-4 py-2 text-xs text-[var(--ctp-overlay1)] border-b border-[var(--ctp-surface1)]">
							{$t('domains.jsonHint')}
						</div>
						<textarea
							bind:value={jsonText}
							class="w-full h-80 p-4 font-mono text-sm bg-[var(--ctp-mantle)] text-[var(--ctp-text)] border-none resize-none focus:outline-none"
							spellcheck="false"
						></textarea>
						{#if jsonError}
							<div class="px-4 py-2 text-xs text-[var(--ctp-red)] bg-[var(--ctp-surface1)]">
								{jsonError}
							</div>
						{/if}
						<div class="px-4 py-3 border-t border-[var(--ctp-surface1)] flex items-center justify-end">
							<button
								onclick={saveJson}
								disabled={savingJson}
								class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
							>
								{savingJson ? $t('common.saving') : $t('common.save')}
							</button>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<!-- Create modal -->
{#if showCreateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		onclick={() => showCreateModal = false}
	>
		<div
			class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-96 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold mb-4">{$t('domains.createSet')}</h3>
			<input
				type="text"
				bind:value={newTag}
				onkeydown={handleCreateKeydown}
				placeholder={$t('domains.tagPlaceholder')}
				class="w-full px-3 py-2 text-sm rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface1)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)] mb-4"
			/>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => showCreateModal = false}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{$t('common.cancel')}
				</button>
				<button
					onclick={createSet}
					disabled={creating || !newTag.trim()}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
				>
					{creating ? $t('common.saving') : $t('common.create')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Import modal -->
{#if showImportModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		onclick={() => showImportModal = false}
	>
		<div
			class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-[500px] shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold mb-2">{$t('domains.importTitle')}</h3>
			<p class="text-sm text-[var(--ctp-overlay1)] mb-4">{$t('domains.importHint')}</p>
			<textarea
				bind:value={importText}
				placeholder={$t('domains.importPlaceholder')}
				class="w-full h-48 px-3 py-2 text-sm font-mono rounded-lg bg-[var(--ctp-mantle)] border border-[var(--ctp-surface1)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)] resize-none mb-4"
			></textarea>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => showImportModal = false}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{$t('common.cancel')}
				</button>
				<button
					onclick={importDomains}
					disabled={importing || !importText.trim()}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
				>
					{importing ? $t('common.saving') : $t('common.import')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete confirmation modal -->
{#if deleteConfirmTag}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		onclick={() => deleteConfirmTag = null}
	>
		<div
			class="bg-[var(--ctp-surface0)] rounded-xl p-6 w-96 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="text-lg font-semibold mb-2">{$t('modal.confirmDelete')}</h3>
			<p class="text-sm text-[var(--ctp-overlay1)] mb-4">
				{$t('domains.deleteConfirm', { values: { tag: deleteConfirmTag } })}
			</p>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => deleteConfirmTag = null}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{$t('common.cancel')}
				</button>
				<button
					onclick={() => deleteConfirmTag && deleteSet(deleteConfirmTag)}
					class="action-btn-danger px-4 py-2 text-sm rounded-lg"
				>
					{$t('common.delete')}
				</button>
			</div>
		</div>
	</div>
{/if}

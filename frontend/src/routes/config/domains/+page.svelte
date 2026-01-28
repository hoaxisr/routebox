<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
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

	// Domain input
	let domainInput = $state('');
	let addingDomain = $state(false);
	let searchQuery = $state('');

	// Import modal
	let showImportModal = $state(false);
	let importText = $state('');
	let importing = $state(false);

	// Tab state
	let activeTab = $state<'domains' | 'json'>('domains');
	let jsonText = $state('');
	let jsonError = $state('');
	let savingJson = $state(false);

	// Compile state
	let compiling = $state(false);

	// Delete confirmation
	let deleteConfirmTag = $state<string | null>(null);

	async function fetchSets() {
		try {
			sets = await api.listDomainSets();
		} catch (e) {
			notifications.error(`${$t('errors.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	async function selectSet(tag: string) {
		selectedTag = tag;
		loadingSet = true;
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
			newTag = '';
			await fetchSets();
			selectSet(newTag.trim() || sets[sets.length - 1]?.tag);
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
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function addDomain() {
		if (!domainInput.trim() || !selectedTag) return;
		addingDomain = true;
		try {
			await api.addDomain(selectedTag, domainInput.trim());
			domainInput = '';
			await selectSet(selectedTag);
			await fetchSets();
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
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function compileDomains() {
		if (!selectedTag) return;
		compiling = true;
		try {
			await api.compileDomains(selectedTag);
			notifications.success($t('domains.compiled'));
			await fetchSets();
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			compiling = false;
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

	function handleDomainKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
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

	let filteredDomains = $derived.by(() => {
		if (!searchQuery) return allDomains;
		const q = searchQuery.toLowerCase();
		return allDomains.filter((d) => d.includes(q));
	});

	let selectedSetInfo = $derived(sets.find((s) => s.tag === selectedTag));

	onMount(fetchSets);
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">{$t('domains.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('domains.description')}</p>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg
				class="w-8 h-8 animate-spin text-[var(--ctp-primary)]"
				fill="none"
				viewBox="0 0 24 24"
			>
				<circle
					class="opacity-25"
					cx="12"
					cy="12"
					r="10"
					stroke="currentColor"
					stroke-width="4"
				></circle>
				<path
					class="opacity-75"
					fill="currentColor"
					d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
				></path>
			</svg>
		</div>
	{:else}
		<div class="flex gap-6" style="min-height: 600px;">
			<!-- Left panel: Set list -->
			<div
				class="w-64 flex-shrink-0 bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)] overflow-hidden flex flex-col"
			>
				<div class="p-3 border-b border-[var(--ctp-surface0)] flex items-center justify-between">
					<span class="text-sm font-medium text-[var(--ctp-subtext1)]"
						>{$t('domains.ruleSets')}</span
					>
					<button
						onclick={() => {
							showCreateModal = true;
							newTag = '';
						}}
						class="px-2 py-1 text-xs rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity"
					>
						+ {$t('common.create')}
					</button>
				</div>

				<div class="flex-1 overflow-y-auto p-2 space-y-1">
					{#if sets.length === 0}
						<div class="text-center py-8 text-[var(--ctp-overlay1)] text-sm">
							{$t('domains.empty')}
						</div>
					{:else}
						{#each sets as set (set.tag)}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								onclick={() => selectSet(set.tag)}
								class="w-full text-left px-3 py-2 rounded-lg transition-colors flex items-center justify-between group cursor-pointer {selectedTag ===
								set.tag
									? 'bg-[var(--ctp-surface0)] text-[var(--ctp-text)]'
									: 'text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface0)] hover:text-[var(--ctp-text)]'}"
							>
								<div class="min-w-0">
									<div class="text-sm font-medium truncate">{set.tag}</div>
									<div class="text-xs text-[var(--ctp-overlay1)]">
										{set.domain_count}
										{$t('domains.domainsCount')}
									</div>
								</div>
								<div class="flex items-center gap-1">
									{#if set.has_compiled && !set.needs_recompile}
										<span
											class="w-2 h-2 rounded-full bg-[var(--ctp-green)]"
											title={$t('domains.compiled')}
										></span>
									{:else if set.has_compiled && set.needs_recompile}
										<span
											class="w-2 h-2 rounded-full bg-[var(--ctp-yellow)]"
											title={$t('domains.needsRecompile')}
										></span>
									{:else}
										<span
											class="w-2 h-2 rounded-full bg-[var(--ctp-overlay0)]"
											title={$t('domains.notCompiled')}
										></span>
									{/if}
									<button
										onclick={(e) => {
											e.stopPropagation();
											deleteConfirmTag = set.tag;
										}}
										class="opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-[var(--ctp-surface1)] transition-all"
										title={$t('common.delete')}
									>
										<svg
											class="w-3.5 h-3.5 text-[var(--ctp-red)]"
											fill="none"
											stroke="currentColor"
											viewBox="0 0 24 24"
										>
											<path
												stroke-linecap="round"
												stroke-linejoin="round"
												stroke-width="2"
												d="M6 18L18 6M6 6l12 12"
											/>
										</svg>
									</button>
								</div>
							</div>
						{/each}
					{/if}
				</div>
			</div>

			<!-- Right panel: Editor -->
			<div class="flex-1 min-w-0">
				{#if !selectedTag}
					<div
						class="h-full flex items-center justify-center bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)]"
					>
						<div class="text-center text-[var(--ctp-overlay1)]">
							<svg
								class="w-12 h-12 mx-auto mb-3 opacity-50"
								fill="none"
								stroke="currentColor"
								viewBox="0 0 24 24"
							>
								<path
									stroke-linecap="round"
									stroke-linejoin="round"
									stroke-width="1.5"
									d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9"
								/>
							</svg>
							<p>{$t('domains.selectOrCreate')}</p>
						</div>
					</div>
				{:else if loadingSet}
					<div
						class="h-full flex items-center justify-center bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)]"
					>
						<svg
							class="w-8 h-8 animate-spin text-[var(--ctp-primary)]"
							fill="none"
							viewBox="0 0 24 24"
						>
							<circle
								class="opacity-25"
								cx="12"
								cy="12"
								r="10"
								stroke="currentColor"
								stroke-width="4"
							></circle>
							<path
								class="opacity-75"
								fill="currentColor"
								d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
							></path>
						</svg>
					</div>
				{:else if selectedSet}
					<div
						class="bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)] overflow-hidden flex flex-col"
						style="height: 600px;"
					>
						<!-- Editor header with tabs -->
						<div
							class="px-4 py-3 border-b border-[var(--ctp-surface0)] flex items-center justify-between"
						>
							<div class="flex items-center gap-4">
								<h2 class="text-lg font-semibold">{selectedTag}</h2>
								<div class="flex rounded-lg overflow-hidden border border-[var(--ctp-surface0)]">
									<button
										onclick={() => (activeTab = 'domains')}
										class="px-3 py-1 text-sm transition-colors {activeTab === 'domains'
											? 'bg-[var(--ctp-primary)] text-white'
											: 'bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)]'}"
									>
										{$t('domains.domainsTab')}
									</button>
									<button
										onclick={() => {
											activeTab = 'json';
											jsonText = JSON.stringify(selectedSet, null, 2);
											jsonError = '';
										}}
										class="px-3 py-1 text-sm transition-colors {activeTab === 'json'
											? 'bg-[var(--ctp-primary)] text-white'
											: 'bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)]'}"
									>
										{$t('domains.jsonTab')}
									</button>
								</div>
							</div>

							<!-- Compile button -->
							<div class="flex items-center gap-2">
								{#if selectedSetInfo?.has_compiled && !selectedSetInfo?.needs_recompile}
									<span class="text-xs text-[var(--ctp-green)]">{$t('domains.upToDate')}</span>
								{:else if selectedSetInfo?.needs_recompile}
									<span class="text-xs text-[var(--ctp-yellow)]"
										>{$t('domains.needsRecompile')}</span
									>
								{/if}
								<button
									onclick={compileDomains}
									disabled={compiling}
									class="px-3 py-1.5 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-text)] hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
								>
									{compiling ? $t('domains.compiling') : $t('domains.compile')}
								</button>
							</div>
						</div>

						{#if activeTab === 'domains'}
							<!-- Domains tab -->
							<div class="flex-1 flex flex-col overflow-hidden">
								<!-- Add domain input -->
								<div class="px-4 py-3 border-b border-[var(--ctp-surface0)]">
									<div class="flex gap-2">
										<input
											type="text"
											bind:value={domainInput}
											onkeydown={handleDomainKeydown}
											placeholder={$t('domains.addPlaceholder')}
											class="flex-1 px-3 py-2 text-sm rounded-lg bg-[var(--ctp-base)] border border-[var(--ctp-surface0)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)]"
										/>
										<button
											onclick={addDomain}
											disabled={addingDomain || !domainInput.trim()}
											class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-primary)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
										>
											{$t('common.add')}
										</button>
										<button
											onclick={() => {
												showImportModal = true;
												importText = '';
											}}
											class="px-3 py-2 text-sm rounded-lg bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)] transition-colors"
										>
											{$t('common.import')}
										</button>
									</div>
								</div>

								<!-- Search -->
								{#if allDomains.length > 10}
									<div class="px-4 py-2 border-b border-[var(--ctp-surface0)]">
										<input
											type="text"
											bind:value={searchQuery}
											placeholder={$t('domains.search')}
											class="w-full px-3 py-1.5 text-sm rounded-lg bg-[var(--ctp-base)] border border-[var(--ctp-surface0)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)]"
										/>
									</div>
								{/if}

								<!-- Domain list -->
								<div class="flex-1 overflow-y-auto">
									{#if allDomains.length === 0}
										<div class="flex items-center justify-center h-full text-[var(--ctp-overlay1)]">
											<p class="text-sm">{$t('domains.noDomains')}</p>
										</div>
									{:else}
										<div class="px-4 py-1">
											<div
												class="text-xs text-[var(--ctp-overlay1)] py-2 border-b border-[var(--ctp-surface0)]"
											>
												{filteredDomains.length}
												{$t('domains.domainsCount')}
												{#if searchQuery && filteredDomains.length !== allDomains.length}
													({$t('domains.of')}
													{allDomains.length})
												{/if}
											</div>
										</div>
										<div class="divide-y divide-[var(--ctp-surface0)]">
											{#each filteredDomains as domain (domain)}
												<div
													class="px-4 py-2 flex items-center justify-between group hover:bg-[var(--ctp-surface0)] transition-colors"
												>
													<span class="text-sm font-mono text-[var(--ctp-text)]">{domain}</span
													>
													<button
														onclick={() => removeDomain(domain)}
														class="opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-[var(--ctp-surface1)] transition-all"
														title={$t('common.delete')}
													>
														<svg
															class="w-4 h-4 text-[var(--ctp-red)]"
															fill="none"
															stroke="currentColor"
															viewBox="0 0 24 24"
														>
															<path
																stroke-linecap="round"
																stroke-linejoin="round"
																stroke-width="2"
																d="M6 18L18 6M6 6l12 12"
															/>
														</svg>
													</button>
												</div>
											{/each}
										</div>
									{/if}
								</div>
							</div>
						{:else}
							<!-- JSON tab -->
							<div class="flex-1 flex flex-col overflow-hidden">
								<div
									class="px-4 py-2 text-xs text-[var(--ctp-overlay1)] border-b border-[var(--ctp-surface0)]"
								>
									{$t('domains.jsonHint')}
								</div>
								<div class="flex-1 relative">
									<textarea
										bind:value={jsonText}
										class="w-full h-full p-4 font-mono text-sm bg-[var(--ctp-base)] text-[var(--ctp-text)] border-none resize-none focus:outline-none"
										spellcheck="false"
									></textarea>
								</div>
								{#if jsonError}
									<div
										class="px-4 py-2 text-xs text-[var(--ctp-red)] bg-[var(--ctp-surface0)] border-t border-[var(--ctp-surface0)]"
									>
										{jsonError}
									</div>
								{/if}
								<div
									class="px-4 py-3 border-t border-[var(--ctp-surface0)] flex items-center justify-end gap-2"
								>
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
			</div>
		</div>
	{/if}
</div>

<!-- Create modal -->
{#if showCreateModal}
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		role="dialog"
		aria-modal="true"
	>
		<div
			class="bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)] p-6 w-96 shadow-xl"
		>
			<h3 class="text-lg font-semibold mb-4">{$t('domains.createSet')}</h3>
			<input
				type="text"
				bind:value={newTag}
				onkeydown={handleCreateKeydown}
				placeholder={$t('domains.tagPlaceholder')}
				class="w-full px-3 py-2 text-sm rounded-lg bg-[var(--ctp-base)] border border-[var(--ctp-surface0)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)] mb-4"
			/>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => (showCreateModal = false)}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)] transition-colors"
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
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		role="dialog"
		aria-modal="true"
	>
		<div
			class="bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)] p-6 w-[500px] shadow-xl"
		>
			<h3 class="text-lg font-semibold mb-2">{$t('domains.importTitle')}</h3>
			<p class="text-sm text-[var(--ctp-overlay1)] mb-4">{$t('domains.importHint')}</p>
			<textarea
				bind:value={importText}
				placeholder={$t('domains.importPlaceholder')}
				class="w-full h-48 px-3 py-2 text-sm font-mono rounded-lg bg-[var(--ctp-base)] border border-[var(--ctp-surface0)] text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:border-[var(--ctp-primary)] resize-none mb-4"
			></textarea>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => (showImportModal = false)}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)] transition-colors"
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
	<div
		class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
		role="dialog"
		aria-modal="true"
	>
		<div
			class="bg-[var(--ctp-mantle)] rounded-xl border border-[var(--ctp-surface0)] p-6 w-96 shadow-xl"
		>
			<h3 class="text-lg font-semibold mb-2">{$t('modal.confirmDelete')}</h3>
			<p class="text-sm text-[var(--ctp-overlay1)] mb-4">
				{$t('domains.deleteConfirm', { values: { tag: deleteConfirmTag } })}
			</p>
			<div class="flex justify-end gap-2">
				<button
					onclick={() => (deleteConfirmTag = null)}
					class="px-4 py-2 text-sm rounded-lg bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface1)] transition-colors"
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

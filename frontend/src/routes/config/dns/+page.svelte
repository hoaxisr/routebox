<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { DnsServer, DnsRule, DnsSettings, RuleSet, Outbound } from '$lib/types';
	import DnsServerForm from '$lib/components/config/DnsServerForm.svelte';
	import DnsRuleForm from '$lib/components/config/DnsRuleForm.svelte';
	import HelpTooltip from '$lib/components/shared/HelpTooltip.svelte';

	let dnsServers = $state<DnsServer[]>([]);
	let dnsRules = $state<DnsRule[]>([]);
	let settings = $state<DnsSettings>({});
	let ruleSets = $state<RuleSet[]>([]);
	let outbounds = $state<Outbound[]>([]);
	let loading = $state(true);
	let hasChanges = $state(false);
	let applying = $state(false);

	// Modal states
	let showServerForm = $state(false);
	let editingServer = $state<DnsServer | undefined>(undefined);
	let showRuleForm = $state(false);
	let editingRuleIndex = $state<number | null>(null);

	// Drag state
	let draggedIndex = $state<number | null>(null);
	let dropTargetIndex = $state<number | null>(null);

	async function fetchData() {
		try {
			const [serversData, rulesData, settingsData, ruleSetsData, outboundsData] = await Promise.all([
				api.listDnsServers(),
				api.listDnsRules(),
				api.getDnsSettings(),
				api.listRuleSets(),
				api.listOutbounds()
			]);
			dnsServers = serversData;
			dnsRules = rulesData;
			settings = settingsData;
			ruleSets = ruleSetsData;
			outbounds = outboundsData;
		} catch (e) {
			notifications.error($t('errors.loadFailed'));
		} finally {
			loading = false;
		}
	}

	async function applyChanges() {
		applying = true;
		try {
			await api.applyConfig();
			notifications.success($t('changes.configApplied'));
			hasChanges = false;
			unsavedChanges.clearChanges();
		} catch (e) {
			notifications.error($t('changes.failedApply'));
		} finally {
			applying = false;
		}
	}

	// Server handlers
	async function handleCreateServer(server: DnsServer) {
		try {
			await api.createDnsServer(server);
			dnsServers = [...dnsServers, server];
			showServerForm = false;
			editingServer = undefined;
			hasChanges = true;
			unsavedChanges.markChanged('DNS', `Created server "${server.tag}"`);
			notifications.success($t('dns.serverCreated'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleUpdateServer(server: DnsServer) {
		if (!editingServer) return;
		try {
			await api.updateDnsServer(editingServer.tag, server);
			dnsServers = dnsServers.map(s => s.tag === editingServer!.tag ? server : s);
			showServerForm = false;
			editingServer = undefined;
			hasChanges = true;
			unsavedChanges.markChanged('DNS', `Updated server "${server.tag}"`);
			notifications.success($t('dns.serverUpdated'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleDeleteServer(tag: string) {
		if (!confirm($t('dns.confirmDeleteServer', { values: { name: tag } }))) return;
		try {
			await api.deleteDnsServer(tag);
			dnsServers = dnsServers.filter(s => s.tag !== tag);
			hasChanges = true;
			unsavedChanges.markChanged('DNS', `Deleted server "${tag}"`);
			notifications.success($t('dns.serverDeleted'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function openEditServer(server: DnsServer) {
		editingServer = server;
		showServerForm = true;
	}

	// Rule handlers
	async function handleCreateRule(rule: DnsRule) {
		try {
			await api.createDnsRule(rule);
			dnsRules = [...dnsRules, rule];
			showRuleForm = false;
			hasChanges = true;
			unsavedChanges.markChanged('DNS', 'Created DNS rule');
			notifications.success($t('dns.ruleCreated'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleUpdateRule(rule: DnsRule) {
		if (editingRuleIndex === null) return;
		try {
			await api.updateDnsRule(editingRuleIndex, rule);
			dnsRules = dnsRules.map((r, i) => i === editingRuleIndex ? rule : r);
			showRuleForm = false;
			hasChanges = true;
			unsavedChanges.markChanged('DNS', `Updated DNS rule #${editingRuleIndex + 1}`);
			editingRuleIndex = null;
			notifications.success($t('dns.ruleUpdated'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleDeleteRule(index: number) {
		if (!confirm($t('dns.confirmDeleteRule'))) return;
		try {
			await api.deleteDnsRule(index);
			dnsRules = dnsRules.filter((_, i) => i !== index);
			hasChanges = true;
			unsavedChanges.markChanged('DNS', `Deleted DNS rule #${index + 1}`);
			notifications.success($t('dns.ruleDeleted'));
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function handleReorder(from: number, to: number) {
		try {
			await api.reorderDnsRules(from, to);
			const newRules = [...dnsRules];
			const [moved] = newRules.splice(from, 1);
			newRules.splice(to, 0, moved);
			dnsRules = newRules;
			hasChanges = true;
			unsavedChanges.markChanged('DNS', 'Reordered DNS rules');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function openEditRule(index: number) {
		editingRuleIndex = index;
		showRuleForm = true;
	}

	function openAddRule() {
		editingRuleIndex = null;
		showRuleForm = true;
	}

	// Settings handlers
	async function handleSettingsChange() {
		try {
			await api.updateDnsSettings(settings);
			hasChanges = true;
			unsavedChanges.markChanged('DNS', 'Updated DNS settings');
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	// Drag handlers
	function handleDragStart(e: DragEvent, index: number) {
		draggedIndex = index;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
		}
	}

	function handleDragOver(e: DragEvent, index: number) {
		e.preventDefault();
		if (draggedIndex !== null && draggedIndex !== index) {
			dropTargetIndex = index;
		}
	}

	function handleDrop(e: DragEvent, toIndex: number) {
		e.preventDefault();
		if (draggedIndex !== null && draggedIndex !== toIndex) {
			handleReorder(draggedIndex, toIndex);
		}
		draggedIndex = null;
		dropTargetIndex = null;
	}

	function handleDragEnd() {
		draggedIndex = null;
		dropTargetIndex = null;
	}

	function getRuleDescription(rule: DnsRule): string {
		const conditions: string[] = [];
		if (rule.domain?.length) conditions.push(`${rule.domain.length} domain(s)`);
		if (rule.domain_suffix?.length) conditions.push(`suffix: ${rule.domain_suffix.slice(0, 2).join(', ')}${rule.domain_suffix.length > 2 ? '...' : ''}`);
		if (rule.domain_keyword?.length) conditions.push(`keyword: ${rule.domain_keyword.join(', ')}`);
		if (rule.rule_set?.length) conditions.push(`ruleset: ${rule.rule_set.join(', ')}`);
		if (rule.query_type?.length) conditions.push(`type: ${rule.query_type.join(', ')}`);
		return conditions.join(' | ') || 'No conditions';
	}

	onMount(fetchData);
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('dns.title')}</h1>
		{#if hasChanges}
			<button
				onclick={applyChanges}
				disabled={applying}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
			>
				{#if applying}
					<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
					</svg>
				{/if}
				{$t('changes.applyChanges')}
			</button>
		{/if}
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else}
		<!-- DNS Settings -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 space-y-4">
			<h2 class="font-medium text-[var(--ctp-subtext1)]">{$t('dns.settings')}</h2>
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div>
					<label for="final" class="flex items-center gap-1 text-sm text-[var(--ctp-overlay1)] mb-1">
						{$t('dns.ruleServer')}
						<HelpTooltip text={$t('help.finalDnsServer')} />
					</label>
					<select
						id="final"
						bind:value={settings.final}
						onchange={handleSettingsChange}
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">{$t('common.none')}</option>
						{#each dnsServers as srv}
							<option value={srv.tag}>{srv.tag}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('dns.ruleServerHint')}</p>
				</div>
				<div>
					<label for="strategy" class="flex items-center gap-1 text-sm text-[var(--ctp-overlay1)] mb-1">
						{$t('dns.strategy')}
						<HelpTooltip text={$t('help.strategy')} />
					</label>
					<select
						id="strategy"
						bind:value={settings.strategy}
						onchange={handleSettingsChange}
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">{$t('common.default')}</option>
						<option value="prefer_ipv4">{$t('dns.strategies.preferIpv4')}</option>
						<option value="prefer_ipv6">{$t('dns.strategies.preferIpv6')}</option>
						<option value="ipv4_only">{$t('dns.strategies.ipv4Only')}</option>
						<option value="ipv6_only">{$t('dns.strategies.ipv6Only')}</option>
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('dns.strategyHint')}</p>
				</div>
			</div>

			<!-- Advanced DNS settings -->
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div>
					<label for="cache_capacity" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('dns.cacheCapacity')}</label>
					<input
						id="cache_capacity"
						type="number"
						bind:value={settings.cache_capacity}
						onchange={handleSettingsChange}
						placeholder={$t('dns.unlimited')}
						min="0"
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('dns.cacheCapacityHint')}</p>
				</div>
				<div>
					<label for="client_subnet" class="block text-sm text-[var(--ctp-overlay1)] mb-1">{$t('dns.clientSubnet')}</label>
					<input
						id="client_subnet"
						type="text"
						bind:value={settings.client_subnet}
						onchange={handleSettingsChange}
						placeholder="e.g., 1.2.3.0/24"
						class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('dns.clientSubnetHint')}</p>
				</div>
			</div>

			<div class="flex flex-wrap gap-4">
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={settings.disable_cache}
						onchange={handleSettingsChange}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					<span class="text-sm text-[var(--ctp-text)]">{$t('dns.disableCache')}</span>
				</label>
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={settings.disable_expire}
						onchange={handleSettingsChange}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					<span class="text-sm text-[var(--ctp-text)]">{$t('dns.disableExpire')}</span>
				</label>
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={settings.independent_cache}
						onchange={handleSettingsChange}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					<span class="text-sm text-[var(--ctp-text)]">{$t('dns.independentCache')}</span>
				</label>
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={settings.reverse_mapping}
						onchange={handleSettingsChange}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					<span class="text-sm text-[var(--ctp-text)]">{$t('dns.reverseMapping')}</span>
					<span class="text-xs text-[var(--ctp-overlay0)]">(IP→domain)</span>
				</label>
			</div>
		</div>

		<!-- DNS Servers Section -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
			<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<span class="font-medium text-[var(--ctp-subtext1)]">{$t('dns.servers')}</span>
				<button
					onclick={() => { editingServer = undefined; showServerForm = true; }}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					+ {$t('dns.addServer')}
				</button>
			</div>

			{#if dnsServers.length === 0}
				<div class="p-6 text-center text-[var(--ctp-overlay0)]">
					<p>{$t('dns.noServers')}</p>
					<p class="text-sm mt-1">{$t('dns.noServersHint')}</p>
				</div>
			{:else}
				<div class="divide-y divide-[var(--ctp-surface2)]">
					{#each dnsServers as server}
						<div class="px-4 py-3 flex items-center justify-between group">
							<div class="flex items-center gap-3 min-w-0">
								<span class="selection-chip flex-shrink-0">
									{server.type}
								</span>
								<span class="font-medium text-[var(--ctp-text)]">{server.tag}</span>
								{#if server.server}
									<span class="text-sm text-[var(--ctp-overlay0)] truncate">{server.server}</span>
								{/if}
								{#if server.detour}
									<span class="text-xs px-2 py-0.5 bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] rounded">
										via {server.detour}
									</span>
								{/if}
							</div>
							<div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
								<button
									onclick={() => openEditServer(server)}
									class="p-1.5 rounded-md hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] transition-colors"
									title={$t('common.edit')}
								>
									<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
									</svg>
								</button>
								<button
									onclick={() => handleDeleteServer(server.tag)}
									class="action-btn-danger"
									title={$t('common.delete')}
								>
									<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
									</svg>
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- DNS Rules Section -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
			<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<div>
					<span class="font-medium text-[var(--ctp-subtext1)]">{$t('dns.rules')}</span>
					<span class="ml-2 text-sm text-[var(--ctp-overlay0)]">({dnsRules.length} {$t('routes.rules').toLowerCase()})</span>
				</div>
				<button
					onclick={openAddRule}
					class="px-3 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					+ {$t('dns.addRule')}
				</button>
			</div>

			<div class="p-4">
				{#if dnsRules.length === 0}
					<div class="p-8 text-center text-[var(--ctp-overlay0)] bg-[var(--ctp-base)] rounded-lg border border-dashed border-[var(--ctp-surface2)]">
						<p>{$t('dns.noRules')}</p>
						<p class="text-sm mt-1">{$t('dns.noRulesHint')}</p>
					</div>
				{:else}
					<div class="space-y-2">
						{#each dnsRules as rule, index}
							<div
								role="listitem"
								draggable="true"
								ondragstart={(e) => handleDragStart(e, index)}
								ondragover={(e) => handleDragOver(e, index)}
								ondragleave={() => dropTargetIndex = null}
								ondrop={(e) => handleDrop(e, index)}
								ondragend={handleDragEnd}
								class="group relative flex items-center gap-3 p-3 bg-[var(--ctp-base)] rounded-lg border transition-all cursor-move
									{draggedIndex === index ? 'opacity-50 border-[var(--ctp-primary)]' : 'border-[var(--ctp-surface2)]'}
									{dropTargetIndex === index ? 'border-[var(--ctp-primary)] border-dashed' : ''}
									hover:border-[var(--ctp-overlay0)]"
							>
								<!-- Drag handle -->
								<div class="flex flex-col gap-0.5 text-[var(--ctp-overlay0)] group-hover:text-[var(--ctp-text)]">
									<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
										<path d="M7 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 2zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 7 14zm6-8a2 2 0 1 0-.001-4.001A2 2 0 0 0 13 6zm0 2a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 8zm0 6a2 2 0 1 0 .001 4.001A2 2 0 0 0 13 14z" />
									</svg>
								</div>

								<!-- Index -->
								<div class="w-8 h-8 flex-shrink-0 rounded-full bg-[var(--ctp-surface2)] flex items-center justify-center text-sm font-medium text-[var(--ctp-subtext0)]">
									{index + 1}
								</div>

								<!-- Rule info -->
								<div class="flex-1 min-w-0">
									<span class="text-sm text-[var(--ctp-text)] truncate">{getRuleDescription(rule)}</span>
								</div>

								<!-- Arrow and Action/Server -->
								<div class="flex items-center gap-2 flex-shrink-0">
									<svg class="w-4 h-4 text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3" />
									</svg>
									{#if rule.action === 'reject'}
										<span class="status-badge error">
											{rule.method === 'drop' ? 'drop' : 'reject'}
										</span>
									{:else if rule.action === 'predefined'}
										<span class="status-badge error">
											{rule.rcode ?? 'NXDOMAIN'}
										</span>
									{:else}
										<span class="status-badge">
											{rule.server}
										</span>
									{/if}
								</div>

								<!-- Actions -->
								<div class="flex items-center gap-1 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
									<button
										type="button"
										onclick={(e) => { e.stopPropagation(); openEditRule(index); }}
										class="p-1.5 rounded-md hover:bg-[var(--ctp-surface2)] text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] transition-colors"
										title={$t('common.edit')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
										</svg>
									</button>
									<button
										type="button"
										onclick={(e) => { e.stopPropagation(); handleDeleteRule(index); }}
										class="action-btn-danger"
										title={$t('common.delete')}
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
										</svg>
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			{#if dnsRules.length > 0}
				<div class="px-4 py-3 border-t border-[var(--ctp-surface2)] bg-[var(--ctp-surface1)]">
					<div class="flex items-center gap-2 text-sm text-[var(--ctp-overlay1)]">
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
						<span>{$t('dns.rulesOrderHint')}</span>
					</div>
				</div>
			{/if}
		</div>

		<!-- Final server indicator -->
		{#if dnsRules.length > 0 && settings.final}
			<div class="flex items-center justify-center gap-3 py-2">
				<div class="h-px flex-1 bg-[var(--ctp-surface2)]"></div>
				<div class="info-banner">
					<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.noMatch')}:</span>
					<span class="text-sm font-medium text-[var(--ctp-primary)]">{settings.final}</span>
				</div>
				<div class="h-px flex-1 bg-[var(--ctp-surface2)]"></div>
			</div>
		{/if}
	{/if}
</div>

<!-- Server Form Modal -->
{#if showServerForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">
					{editingServer ? `${$t('dns.editServer')}: ${editingServer.tag}` : $t('dns.addServer')}
				</h2>
				<button
					onclick={() => { showServerForm = false; editingServer = undefined; }}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label={$t('common.close')}
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<DnsServerForm
					server={editingServer}
					existingTags={dnsServers.filter(s => s.tag !== editingServer?.tag).map(s => s.tag)}
					existingServers={dnsServers}
					{outbounds}
					onSave={editingServer ? handleUpdateServer : handleCreateServer}
					onCancel={() => { showServerForm = false; editingServer = undefined; }}
				/>
			</div>
		</div>
	</div>
{/if}

<!-- Rule Form Modal -->
{#if showRuleForm}
	<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
		<div class="bg-[var(--ctp-base)] rounded-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
			<div class="px-4 py-3 border-b border-[var(--ctp-surface2)] flex items-center justify-between">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">
					{editingRuleIndex !== null ? `${$t('dns.editRule')} #${editingRuleIndex + 1}` : $t('dns.addRule')}
				</h2>
				<button
					onclick={() => { showRuleForm = false; editingRuleIndex = null; }}
					class="p-1 rounded-md hover:bg-[var(--ctp-surface1)] text-[var(--ctp-overlay1)]"
					aria-label={$t('common.close')}
				>
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="p-4">
				<DnsRuleForm
					rule={editingRuleIndex !== null ? dnsRules[editingRuleIndex] : undefined}
					{dnsServers}
					{ruleSets}
					{outbounds}
					onSave={editingRuleIndex !== null ? handleUpdateRule : handleCreateRule}
					onCancel={() => { showRuleForm = false; editingRuleIndex = null; }}
					onRuleSetCreated={(ruleSet) => {
						ruleSets = [...ruleSets, ruleSet];
						hasChanges = true;
						unsavedChanges.markChanged('DNS', `Created rule set "${ruleSet.tag}"`);
					}}
				/>
			</div>
		</div>
	</div>
{/if}

<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { DnsRule, DnsServer, RuleSet, Outbound } from '$lib/types';

	interface Props {
		rule: DnsRule;
		ruleIndex: number;
		dnsServers: DnsServer[];
		ruleSets: RuleSet[];
		outbounds: Outbound[];
		operating?: boolean;
		onSave: (index: number, rule: DnsRule) => void;
		onDelete: (index: number) => void;
		onClose: () => void;
	}

	let { rule, ruleIndex, dnsServers, ruleSets, outbounds, operating = false, onSave, onDelete, onClose }: Props = $props();

	// Local copy for editing
	let action = $state<'route' | 'reject' | 'predefined'>(rule.action ?? 'route');
	let server = $state(rule.server ?? '');
	let domain = $state(rule.domain?.join('\n') ?? '');
	let domainSuffix = $state(rule.domain_suffix?.join('\n') ?? '');
	let domainKeyword = $state(rule.domain_keyword?.join('\n') ?? '');
	let ipCidr = $state(rule.ip_cidr?.join('\n') ?? '');
	let queryType = $state(rule.query_type?.join(', ') ?? '');
	let selectedRuleSets = $state<string[]>(rule.rule_set ?? []);

	// Tabs
	type Tab = 'action' | 'conditions';
	let activeTab = $state<Tab>('action');

	function parseLines(text: string): string[] | undefined {
		const lines = text.split('\n').map(s => s.trim()).filter(s => s);
		return lines.length > 0 ? lines : undefined;
	}

	function parseComma(text: string): string[] | undefined {
		const items = text.split(',').map(s => s.trim().toUpperCase()).filter(s => s);
		return items.length > 0 ? items : undefined;
	}

	function handleSave() {
		const updatedRule: DnsRule = {
			action,
			server: action === 'route' ? server || undefined : undefined,
			domain: parseLines(domain),
			domain_suffix: parseLines(domainSuffix),
			domain_keyword: parseLines(domainKeyword),
			ip_cidr: parseLines(ipCidr),
			query_type: parseComma(queryType),
			rule_set: selectedRuleSets.length > 0 ? selectedRuleSets : undefined
		};
		onSave(ruleIndex, updatedRule);
	}

	function handleDelete() {
		if (confirm($t('dns.confirmDeleteRule'))) {
			onDelete(ruleIndex);
		}
	}

	function toggleRuleSet(tag: string) {
		if (selectedRuleSets.includes(tag)) {
			selectedRuleSets = selectedRuleSets.filter(t => t !== tag);
		} else {
			selectedRuleSets = [...selectedRuleSets, tag];
		}
	}

	// Reset when rule changes
	$effect(() => {
		action = rule.action ?? 'route';
		server = rule.server ?? '';
		domain = rule.domain?.join('\n') ?? '';
		domainSuffix = rule.domain_suffix?.join('\n') ?? '';
		domainKeyword = rule.domain_keyword?.join('\n') ?? '';
		ipCidr = rule.ip_cidr?.join('\n') ?? '';
		queryType = rule.query_type?.join(', ') ?? '';
		selectedRuleSets = rule.rule_set ?? [];
	});

	let needsServer = $derived(action === 'route');
</script>

<div class="edit-panel">
	<div class="panel-header">
		<h3 class="panel-title">{$t('dns.editRuleNumber', { values: { number: ruleIndex + 1 } })}</h3>
		<button class="close-btn" onclick={onClose} aria-label="Close">
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<div class="tabs">
		<button class="tab" class:active={activeTab === 'action'} onclick={() => activeTab = 'action'}>
			{$t('dns.action')}
		</button>
		<button class="tab" class:active={activeTab === 'conditions'} onclick={() => activeTab = 'conditions'}>
			{$t('routes.ruleConditions')}
		</button>
	</div>

	<div class="panel-content">
		{#if activeTab === 'action'}
			<div class="section">
				<label class="field-label">{$t('dns.action')}</label>
				<div class="action-buttons">
					<button
						class="action-btn"
						class:active={action === 'route'}
						onclick={() => action = 'route'}
					>
						Route
					</button>
					<button
						class="action-btn"
						class:active={action === 'reject'}
						onclick={() => action = 'reject'}
					>
						Reject
					</button>
				</div>
			</div>

			{#if needsServer}
				<div class="section">
					<label class="field-label">{$t('dns.ruleServer')}</label>
					<select class="field-select" bind:value={server}>
						<option value="">{$t('common.select')}</option>
						{#each dnsServers as srv}
							<option value={srv.tag}>{srv.tag} ({srv.type})</option>
						{/each}
					</select>
				</div>
			{/if}
		{:else}
			<div class="section">
				<label class="field-label">{$t('dns.domain')}</label>
				<textarea class="field-textarea" bind:value={domain} rows="3" placeholder="example.com"></textarea>
			</div>

			<div class="section">
				<label class="field-label">{$t('dns.domainSuffix')}</label>
				<textarea class="field-textarea" bind:value={domainSuffix} rows="3" placeholder=".google.com"></textarea>
			</div>

			<div class="section">
				<label class="field-label">{$t('dns.domainKeyword')}</label>
				<textarea class="field-textarea" bind:value={domainKeyword} rows="2" placeholder="google"></textarea>
			</div>

			<div class="section">
				<label class="field-label">{$t('routes.queryType')}</label>
				<input class="field-input" type="text" bind:value={queryType} placeholder="A, AAAA, HTTPS" />
			</div>

			{#if ruleSets.length > 0}
				<div class="section">
					<label class="field-label">{$t('routes.ruleSet')}</label>
					<div class="ruleset-list">
						{#each ruleSets as rs}
							<button
								class="ruleset-chip"
								class:selected={selectedRuleSets.includes(rs.tag)}
								onclick={() => toggleRuleSet(rs.tag)}
							>
								{rs.tag}
							</button>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	</div>

	<div class="panel-footer">
		<button class="btn-danger" onclick={handleDelete} disabled={operating}>
			{$t('common.delete')}
		</button>
		<div class="footer-right">
			<button class="btn-secondary" onclick={onClose} disabled={operating}>
				{$t('common.cancel')}
			</button>
			<button class="btn-primary" onclick={handleSave} disabled={operating}>
				{#if operating}
					<svg class="w-4 h-4 animate-spin inline mr-1" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
				{/if}
				{$t('common.save')}
			</button>
		</div>
	</div>
</div>

<style>
	.edit-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--ctp-mantle);
	}

	.panel-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem;
		border-bottom: 1px solid var(--ctp-surface0);
	}

	.panel-title {
		font-size: 1rem;
		font-weight: 600;
		color: var(--ctp-text);
	}

	.close-btn {
		padding: 0.25rem;
		color: var(--ctp-overlay1);
		border-radius: 0.25rem;
		transition: all 0.15s ease;
	}

	.close-btn:hover {
		color: var(--ctp-text);
		background: var(--ctp-surface0);
	}

	.tabs {
		display: flex;
		border-bottom: 1px solid var(--ctp-surface0);
	}

	.tab {
		flex: 1;
		padding: 0.75rem 1rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--ctp-overlay1);
		border-bottom: 2px solid transparent;
		transition: all 0.15s ease;
	}

	.tab:hover {
		color: var(--ctp-text);
	}

	.tab.active {
		color: var(--ctp-sapphire);
		border-bottom-color: var(--ctp-sapphire);
	}

	.panel-content {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
	}

	.section {
		margin-bottom: 1rem;
	}

	.field-label {
		display: block;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--ctp-subtext1);
		margin-bottom: 0.375rem;
	}

	.field-input,
	.field-select,
	.field-textarea {
		width: 100%;
		padding: 0.5rem;
		background: var(--ctp-base);
		border: 1px solid var(--ctp-surface1);
		border-radius: 0.375rem;
		color: var(--ctp-text);
		font-size: 0.875rem;
	}

	.field-textarea {
		resize: vertical;
		font-family: monospace;
		font-size: 0.75rem;
	}

	.field-input:focus,
	.field-select:focus,
	.field-textarea:focus {
		outline: none;
		border-color: var(--ctp-sapphire);
	}

	.action-buttons {
		display: flex;
		gap: 0.5rem;
	}

	.action-btn {
		flex: 1;
		padding: 0.5rem;
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface1);
		border-radius: 0.375rem;
		color: var(--ctp-text);
		font-size: 0.875rem;
		transition: all 0.15s ease;
	}

	.action-btn:hover {
		background: var(--ctp-surface1);
	}

	.action-btn.active {
		background: var(--ctp-sapphire);
		border-color: var(--ctp-sapphire);
		color: white;
	}

	.ruleset-list {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
	}

	.ruleset-chip {
		padding: 0.25rem 0.5rem;
		background: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface1);
		border-radius: 0.25rem;
		color: var(--ctp-subtext1);
		font-size: 0.75rem;
		transition: all 0.15s ease;
	}

	.ruleset-chip:hover {
		background: var(--ctp-surface1);
	}

	.ruleset-chip.selected {
		background: var(--ctp-sapphire);
		border-color: var(--ctp-sapphire);
		color: white;
	}

	.panel-footer {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem;
		border-top: 1px solid var(--ctp-surface0);
		background: var(--ctp-base);
	}

	.footer-right {
		display: flex;
		gap: 0.5rem;
	}

	.btn-primary {
		padding: 0.5rem 1rem;
		background: var(--ctp-sapphire);
		color: white;
		border-radius: 0.375rem;
		font-weight: 500;
		transition: filter 0.15s ease;
	}

	.btn-primary:hover:not(:disabled) {
		filter: brightness(1.1);
	}

	.btn-primary:disabled,
	.btn-secondary:disabled,
	.btn-danger:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-secondary {
		padding: 0.5rem 1rem;
		background: var(--ctp-surface0);
		color: var(--ctp-text);
		border-radius: 0.375rem;
		font-weight: 500;
		transition: background 0.15s ease;
	}

	.btn-secondary:hover {
		background: var(--ctp-surface1);
	}

	.btn-danger {
		padding: 0.5rem 1rem;
		background: transparent;
		color: var(--ctp-red);
		border: 1px solid var(--ctp-red);
		border-radius: 0.375rem;
		font-weight: 500;
		transition: all 0.15s ease;
	}

	.btn-danger:hover {
		background: var(--ctp-red);
		color: white;
	}
</style>

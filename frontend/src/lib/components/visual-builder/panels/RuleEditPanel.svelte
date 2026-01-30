<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RouteRule, RuleConditions, Outbound, RuleSet, Inbound } from '$lib/types';
	import { ConditionsForm, OutboundSelector, ActionSelector } from '$lib/components/config/rules';

	interface Props {
		rule: RouteRule;
		ruleIndex: number;
		outbounds: Outbound[];
		ruleSets: RuleSet[];
		inbounds: Inbound[];
		operating?: boolean;
		onSave: (index: number, rule: RouteRule) => void;
		onDelete: (index: number) => void;
		onClose: () => void;
	}

	let { rule, ruleIndex, outbounds, ruleSets, inbounds, operating = false, onSave, onDelete, onClose }: Props =
		$props();

	// Local copy for editing
	let editedRule = $state<RouteRule>({ ...rule });

	// Ensure outbound is always a string for the selector
	let outboundValue = $state(editedRule.outbound || '');

	// Extract conditions from rule
	let conditions = $state<RuleConditions>(extractConditions(rule));

	function extractConditions(r: RouteRule): RuleConditions {
		return {
			domain: r.domain,
			domain_suffix: r.domain_suffix,
			domain_keyword: r.domain_keyword,
			domain_regex: r.domain_regex,
			ip_cidr: r.ip_cidr,
			source_ip_cidr: r.source_ip_cidr,
			port: r.port,
			port_range: r.port_range,
			source_port: r.source_port,
			source_port_range: r.source_port_range,
			process_name: r.process_name,
			process_path: r.process_path,
			process_path_regex: r.process_path_regex,
			inbound: r.inbound,
			ip_is_private: r.ip_is_private,
			source_ip_is_private: r.source_ip_is_private,
			invert: r.invert,
			protocol: r.protocol,
			network: r.network,
			ip_version: r.ip_version,
			rule_set: r.rule_set,
			rule_set_ip_cidr_match_source: r.rule_set_ip_cidr_match_source,
			clash_mode: r.clash_mode,
			client: r.client,
			auth_user: r.auth_user,
			user: r.user,
			user_id: r.user_id
		};
	}

	function handleSave() {
		// Merge conditions back into rule
		const updatedRule: RouteRule = {
			...editedRule,
			...conditions,
			outbound: outboundValue || undefined
		};
		onSave(ruleIndex, updatedRule);
	}

	function handleDelete() {
		if (confirm($t('routes.confirmDeleteRule'))) {
			onDelete(ruleIndex);
		}
	}

	// Reset when rule changes
	$effect(() => {
		editedRule = { ...rule };
		conditions = extractConditions(rule);
		outboundValue = rule.outbound || '';
	});

	// Determine if action needs outbound
	let needsOutbound = $derived(
		editedRule.action === 'route' || editedRule.action === undefined || editedRule.action === null
	);
</script>

<div class="edit-panel">
	<div class="panel-header">
		<h3 class="panel-title">{$t('routes.editRuleNumber', { values: { number: ruleIndex + 1 } })}</h3>
		<button class="close-btn" onclick={onClose} aria-label="Close">
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<div class="panel-content">
		<div class="section">
			<ActionSelector bind:action={editedRule.action} />
		</div>

		{#if needsOutbound}
			<div class="section">
				<OutboundSelector bind:outbound={outboundValue} {outbounds} />
			</div>
		{/if}

		<div class="section">
			<h4 class="section-title">{$t('routes.ruleConditions')}</h4>
			<ConditionsForm bind:conditions {ruleSets} {inbounds} />
		</div>
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
		border-left: 1px solid var(--ctp-surface0);
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

	.panel-content {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
	}

	.section {
		margin-bottom: 1.5rem;
	}

	.section-title {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--ctp-subtext1);
		margin-bottom: 0.75rem;
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
		background: var(--ctp-primary);
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

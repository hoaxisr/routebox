<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { TestRouteResponse, RuleMatch, RouteRule } from '$lib/types';

	interface Props {
		rules: RouteRule[];
		finalOutbound: string;
	}

	let { rules, finalOutbound }: Props = $props();

	let input = $state('');
	let testing = $state(false);
	let result = $state<TestRouteResponse | null>(null);
	let error = $state('');
	let showAllRules = $state(false);

	// Example domains for quick testing
	const examples = [
		{ domain: 'google.com', label: 'Google' },
		{ domain: 'youtube.com', label: 'YouTube' },
		{ domain: 'facebook.com', label: 'Facebook' },
		{ domain: 'telegram.org', label: 'Telegram' },
		{ domain: '192.168.1.1', label: 'LAN IP' },
		{ domain: '8.8.8.8', label: 'Google DNS' }
	];

	async function testRoute() {
		if (!input.trim()) return;

		testing = true;
		error = '';
		result = null;
		showAllRules = false;

		try {
			result = await api.testRoute(input.trim());
		} catch (e) {
			error = `${e}`;
			notifications.error($t('routes.inspector.testFailed', { values: { error: String(e) } }));
		} finally {
			testing = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			testRoute();
		}
	}

	function quickTest(domain: string) {
		input = domain;
		testRoute();
	}

	function getActionColor(action: string): string {
		switch (action) {
			case 'route': return 'var(--ctp-green)';
			case 'reject': return 'var(--ctp-red)';
			case 'sniff': return 'var(--ctp-blue)';
			case 'hijack-dns': return 'var(--ctp-yellow)';
			default: return 'var(--ctp-overlay1)';
		}
	}

	// Get the matched rule from result
	let matchedRuleData = $derived.by(() => {
		if (!result || result.matched_rule < 0) return null;
		return result.matches.find(m => m.index === result.matched_rule);
	});
</script>

<div class="space-y-6">
	<!-- Input section -->
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 space-y-4">
		<div>
			<label for="test-input" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
				{$t('routes.inspector.inputLabel')}
			</label>
			<div class="flex gap-2">
				<input
					id="test-input"
					type="text"
					bind:value={input}
					onkeydown={handleKeydown}
					placeholder={$t('routes.inspector.inputPlaceholder')}
					class="min-w-0 flex-1 px-4 py-2.5 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono"
				/>
				<button
					onclick={testRoute}
					disabled={testing || !input.trim()}
					class="px-6 py-2.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2 flex-shrink-0 whitespace-nowrap"
				>
					{#if testing}
						<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
						</svg>
					{:else}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
						</svg>
					{/if}
					{$t('common.test')}
				</button>
			</div>
		</div>

		<!-- Quick examples -->
		<div class="flex flex-wrap gap-2">
			<span class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.inspector.quickTest')}:</span>
			{#each examples as ex}
				<button
					type="button"
					onclick={() => quickTest(ex.domain)}
					class="px-2 py-1 text-xs bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{ex.label}
				</button>
			{/each}
		</div>
	</div>

	<!-- Error -->
	{#if error}
		<div class="bg-[var(--ctp-red)] bg-opacity-10 border border-[var(--ctp-red)] rounded-lg p-4">
			<p class="text-[var(--ctp-red)]">{error}</p>
		</div>
	{/if}

	<!-- Results -->
	{#if result}
		<div class="space-y-4">
			<!-- Main Result Card -->
			<div class="bg-[var(--ctp-surface0)] rounded-xl p-5">
				<div class="flex items-center justify-between mb-4">
					<div class="flex items-center gap-3">
						<span class="text-3xl">
							{result.input_type === 'domain' ? '🌐' : '📍'}
						</span>
						<div>
							<div class="font-mono text-xl text-[var(--ctp-text)]">{result.input}</div>
							<div class="text-sm text-[var(--ctp-overlay1)]">{result.input_type}</div>
						</div>
					</div>
					<div class="flex items-center gap-3">
						<svg class="w-8 h-8 text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3" />
						</svg>
						<span class="text-2xl font-bold" style="color: {result.destination === 'REJECT' ? 'var(--ctp-red)' : 'var(--ctp-green)'}">
							{result.destination}
						</span>
					</div>
				</div>

				<!-- Matched Rule Details -->
				{#if result.matched_rule >= 0 && matchedRuleData}
					<div class="border-t border-[var(--ctp-surface2)] pt-4">
						<div class="flex items-start gap-3">
							<div class="w-8 h-8 rounded-full bg-[var(--ctp-green)] bg-opacity-20 flex items-center justify-center flex-shrink-0">
								<svg class="w-5 h-5 text-[var(--ctp-green)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							</div>
							<div class="flex-1">
								<div class="flex items-center gap-2 flex-wrap">
									<span class="font-medium text-[var(--ctp-text)]">{$t('routes.inspector.ruleNumber', { values: { number: result.matched_rule + 1 } })}</span>
									<span class="px-2 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, {getActionColor(matchedRuleData.action)} 20%, transparent); color: {getActionColor(matchedRuleData.action)}">
										{matchedRuleData.action.toUpperCase()}
									</span>
									{#if matchedRuleData.outbound}
										<span class="text-sm text-[var(--ctp-overlay1)]">→ {matchedRuleData.outbound}</span>
									{/if}
								</div>
								{#if matchedRuleData.reason}
									<div class="mt-2 text-sm text-[var(--ctp-green)]">
										{matchedRuleData.reason}
									</div>
								{/if}
								{#if matchedRuleData.conditions && matchedRuleData.conditions.length > 0}
									<div class="mt-1 text-xs text-[var(--ctp-overlay0)]">
										{$t('routes.ruleConditions')}: {matchedRuleData.conditions.join(', ')}
									</div>
								{/if}
							</div>
						</div>
					</div>
				{:else}
					<div class="border-t border-[var(--ctp-surface2)] pt-4">
						<div class="flex items-center gap-3 text-[var(--ctp-overlay1)]">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
							</svg>
							<span>{$t('routes.inspector.noRuleMatched')}: <strong class="text-[var(--ctp-text)]">{finalOutbound}</strong></span>
						</div>
					</div>
				{/if}
			</div>

			<!-- Show All Rules Toggle -->
			<div class="flex justify-center">
				<button
					type="button"
					onclick={() => showAllRules = !showAllRules}
					class="text-sm text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)] flex items-center gap-1"
				>
					{#if showAllRules}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7" />
						</svg>
						{$t('routes.inspector.hideRuleDetails')}
					{:else}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
						{$t('routes.inspector.showAllRules', { values: { count: result.matches.length } })}
					{/if}
				</button>
			</div>

			<!-- Rule-by-rule breakdown (collapsible) -->
			{#if showAllRules}
				<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
					<div class="px-4 py-3 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)]">
						<span class="font-medium text-[var(--ctp-subtext1)]">{$t('routes.inspector.ruleEvaluationOrder')}</span>
					</div>

					<div class="divide-y divide-[var(--ctp-surface2)] max-h-80 overflow-y-auto">
						{#each result.matches as match, i}
							<div class="px-4 py-2.5 flex items-center gap-3 {match.matched ? 'bg-[var(--ctp-green)] bg-opacity-5' : ''}">
								<!-- Match indicator -->
								<div class="flex-shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-xs {match.matched ? 'bg-[var(--ctp-green)] bg-opacity-20 text-[var(--ctp-green)]' : 'bg-[var(--ctp-surface1)] text-[var(--ctp-overlay0)]'}">
									{#if match.matched}
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
										</svg>
									{:else}
										{i + 1}
									{/if}
								</div>

								<div class="flex-1 min-w-0 flex items-center gap-2 flex-wrap">
									<span class="text-sm {match.matched ? 'font-medium text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)]'}">
										{$t('routes.inspector.ruleNumber', { values: { number: match.index + 1 } })}
									</span>
									<span class="px-1.5 py-0.5 text-xs rounded" style="background-color: color-mix(in srgb, {getActionColor(match.action)} 15%, transparent); color: {getActionColor(match.action)}">
										{match.action}
									</span>
									{#if match.outbound}
										<span class="text-xs text-[var(--ctp-overlay0)]">→ {match.outbound}</span>
									{/if}
								</div>

								<div class="text-xs text-right {match.matched ? 'text-[var(--ctp-green)]' : 'text-[var(--ctp-overlay0)]'} max-w-[200px] truncate" title={match.reason}>
									{match.reason || '—'}
								</div>
							</div>
							<!-- Show rule_keys for debugging -->
							{#if match.rule_keys && match.rule_keys.length > 0}
								<div class="px-4 py-1 text-[10px] text-[var(--ctp-overlay0)] bg-[var(--ctp-base)] border-t border-[var(--ctp-surface2)]">
									{$t('routes.inspector.keys')}: {match.rule_keys.join(', ')}
								</div>
							{/if}
						{/each}

						<!-- Final outbound -->
						<div class="px-4 py-2.5 flex items-center gap-3 bg-[var(--ctp-surface1)]">
							<div class="w-6 h-6 rounded-full bg-[var(--ctp-overlay0)] bg-opacity-20 flex items-center justify-center">
								<span class="text-xs text-[var(--ctp-overlay0)]">∞</span>
							</div>
							<span class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.inspector.finalDefault')}</span>
							<span class="text-sm font-medium text-[var(--ctp-text)]">→ {finalOutbound}</span>
						</div>
					</div>
				</div>
			{/if}

			<!-- Debug Info -->
			{#if result.debug}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4 text-xs">
					<div class="flex items-center gap-2 mb-2">
						<svg class="w-4 h-4 text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
						</svg>
						<span class="text-[var(--ctp-overlay1)] font-medium">{$t('routes.inspector.debugInfo')}</span>
					</div>
					<div class="grid grid-cols-2 gap-2 text-[var(--ctp-overlay0)]">
						<div>{$t('routes.rules')}: <span class="text-[var(--ctp-text)]">{result.debug.rule_count}</span></div>
						<div>{$t('routes.ruleSets')}: <span class="text-[var(--ctp-text)]">{result.debug.rule_set_count}</span></div>
						<div>{$t('routes.inspector.final')}: <span class="text-[var(--ctp-text)]">{result.debug.final_outbound}</span></div>
						<div class="col-span-2">
							{$t('routes.inspector.tags')}: <span class="text-[var(--ctp-text)]">{result.debug.rule_set_tags.join(', ') || $t('common.none')}</span>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{:else if !error}
		<!-- Empty state -->
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
			<div class="text-4xl mb-3">🔍</div>
			<h3 class="text-lg font-medium text-[var(--ctp-text)] mb-2">{$t('routes.inspector.title')}</h3>
			<p class="text-[var(--ctp-overlay1)] max-w-md mx-auto">
				{$t('routes.inspector.emptyStateDescription')}
			</p>
		</div>
	{/if}
</div>

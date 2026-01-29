<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RuleSet, Outbound } from '$lib/types';
	import { notifications, featureFlags } from '$lib/stores';

	interface Props {
		existingTags: string[];
		outbounds?: Outbound[];
		ruleSet?: RuleSet | null;
		onSave: (ruleSet: RuleSet) => void;
		onCancel: () => void;
	}

	let { existingTags, outbounds = [], ruleSet = null, onSave, onCancel }: Props = $props();

	const isEditing = !!ruleSet;

	// Popular presets
	const PRESETS = [
		{ tag: 'antizapret', name: 'Antizapret', url: 'https://github.com/savely-krasovsky/antizapret-sing-box/releases/latest/download/antizapret.srs' },
		{ tag: 'geosite-ru', name: 'GeoSite Russia', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ru.srs' },
		{ tag: 'geoip-ru', name: 'GeoIP Russia', url: 'https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-ru.srs' },
		{ tag: 'adblock', name: 'AdBlock', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ads-all.srs' }
	];

	// Form state - initialize from ruleSet if editing
	let tag = $state(ruleSet?.tag || '');
	let type = $state<'remote' | 'local' | 'inline'>(ruleSet?.type || 'remote');
	let format = $state<'binary' | 'source'>(ruleSet?.format || 'binary');
	let url = $state(ruleSet?.url || '');
	let path = $state(ruleSet?.path || '');
	// Remote-specific options
	let downloadDetour = $state(ruleSet?.download_detour || '');
	let updateInterval = $state(ruleSet?.update_interval || '24h');
	// Inline-specific options
	let inlineRulesJson = $state(ruleSet?.rules ? JSON.stringify(ruleSet.rules, null, 2) : '[\n  {\n    "domain_suffix": [".example.com"]\n  }\n]');

	let errors = $state<Record<string, string>>({});

	function applyPreset(preset: typeof PRESETS[0]) {
		tag = preset.tag;
		type = 'remote';
		format = 'binary';
		url = preset.url;
	}

	function validate(): boolean {
		errors = {};

		if (!tag.trim()) {
			errors['tag'] = $t('dns.validation.tagRequired');
		} else if (existingTags.includes(tag.trim())) {
			errors['tag'] = $t('dns.validation.tagExists');
		}

		if (type === 'remote' && !url.trim()) {
			errors['url'] = $t('dns.validation.serverRequired');
		}

		if (type === 'local' && !path.trim()) {
			errors['path'] = $t('errors.requiredField');
		}

		if (type === 'inline') {
			try {
				const parsed = JSON.parse(inlineRulesJson);
				if (!Array.isArray(parsed)) {
					errors['rules'] = $t('routes.inlineRulesMustBeArray');
				}
			} catch {
				errors['rules'] = $t('routes.inlineRulesInvalidJson');
			}
		}

		const errorKeys = Object.keys(errors);
		if (errorKeys.length > 0) {
			notifications.error(errors[errorKeys[0]]);
			return false;
		}

		return true;
	}

	function handleSubmit() {
		if (!validate()) return;

		const newRuleSet: RuleSet = {
			tag: tag.trim(),
			type,
			...(type !== 'inline' ? { format } : {})
		};

		if (type === 'remote') {
			newRuleSet.url = url.trim();
			if (downloadDetour) newRuleSet.download_detour = downloadDetour;
			if (updateInterval && updateInterval !== '24h') newRuleSet.update_interval = updateInterval;
		} else if (type === 'local') {
			newRuleSet.path = path.trim();
		} else if (type === 'inline') {
			newRuleSet.rules = JSON.parse(inlineRulesJson);
		}

		onSave(newRuleSet);
	}
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Presets (only when creating) -->
	{#if !isEditing}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('dns.quickAddPreset')}</label>
			<div class="flex flex-wrap gap-2">
				{#each PRESETS as preset}
					<button
						type="button"
						onclick={() => applyPreset(preset)}
						disabled={existingTags.includes(preset.tag)}
						class="preset-btn"
					>
						{preset.name}
					</button>
				{/each}
			</div>
		</div>

		<div class="border-t border-[var(--ctp-surface2)] pt-4">
			<p class="text-sm text-[var(--ctp-overlay0)] mb-4">{$t('dns.orConfigureManually')}</p>
		</div>
	{/if}

	<div>

		<!-- Tag -->
		<div class="mb-4">
			<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('common.tag')} *</label>
			<input
				id="tag"
				type="text"
				bind:value={tag}
				placeholder="my-ruleset"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['tag'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['tag']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['tag']}</p>
			{/if}
		</div>

		<!-- Type -->
		<div class="mb-4">
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('common.type')}</label>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={() => type = 'remote'}
					class="toggle-btn {type === 'remote' ? 'selected' : ''}"
				>
					{$t('routes.ruleSetTypes.remote')}
				</button>
				<button
					type="button"
					onclick={() => type = 'local'}
					class="toggle-btn {type === 'local' ? 'selected' : ''}"
				>
					{$t('routes.ruleSetTypes.local')}
				</button>
				{#if $featureFlags['inline_rule_sets']}
					<button
						type="button"
						onclick={() => type = 'inline'}
						class="toggle-btn {type === 'inline' ? 'selected' : ''}"
					>
						{$t('routes.ruleSetTypes.inline')}
					</button>
				{/if}
			</div>
		</div>

		<!-- Format (not for inline) -->
		{#if type !== 'inline'}
		<div class="mb-4">
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('common.format')}</label>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={() => format = 'binary'}
					class="toggle-btn {format === 'binary' ? 'selected' : ''}"
				>
					{$t('routes.ruleSetFormats.binary')}
				</button>
				<button
					type="button"
					onclick={() => format = 'source'}
					class="toggle-btn {format === 'source' ? 'selected' : ''}"
				>
					{$t('routes.ruleSetFormats.source')}
				</button>
			</div>
		</div>
		{/if}

		<!-- URL (for remote) -->
		{#if type === 'remote'}
			<div class="mb-4">
				<label for="url" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.ruleSetUrl')} *</label>
				<input
					id="url"
					type="url"
					bind:value={url}
					placeholder="https://example.com/ruleset.srs"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['url'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				{#if errors['url']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['url']}</p>
				{/if}
			</div>

			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
				<div>
					<label for="downloadDetour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.downloadDetour')}</label>
					<select
						id="downloadDetour"
						bind:value={downloadDetour}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">{$t('dns.noneDirect')}</option>
						{#each outbounds as ob}
							<option value={ob.tag}>{ob.tag}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.downloadDetourHint')}</p>
				</div>
				<div>
					<label for="updateInterval" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.updateInterval')}</label>
					<input
						id="updateInterval"
						type="text"
						bind:value={updateInterval}
						placeholder="24h"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.updateIntervalHint')}</p>
				</div>
			</div>
		{/if}

		<!-- Path (for local) -->
		{#if type === 'local'}
			<div class="mb-4">
				<label for="path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.ruleSetPath')} *</label>
				<input
					id="path"
					type="text"
					bind:value={path}
					placeholder="/etc/sing-box/rulesets/custom.srs"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['path'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				/>
				{#if errors['path']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['path']}</p>
				{/if}
			</div>
		{/if}

		<!-- Inline rules JSON editor -->
		{#if type === 'inline'}
			<div class="mb-4">
				<label for="inline-rules" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.inlineRules')} *</label>
				<textarea
					id="inline-rules"
					bind:value={inlineRulesJson}
					rows="10"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm {errors['rules'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
				></textarea>
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.inlineRulesHint')}</p>
				{#if errors['rules']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['rules']}</p>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('common.cancel')}
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{isEditing ? $t('common.saveChanges') : $t('routes.addRuleSet')}
		</button>
	</div>
</form>

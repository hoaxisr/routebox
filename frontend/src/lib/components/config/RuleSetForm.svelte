<script lang="ts">
	import type { RuleSet, Outbound } from '$lib/types';
	import { notifications } from '$lib/stores';

	interface Props {
		existingTags: string[];
		outbounds?: Outbound[];
		ruleSet?: RuleSet | null;
		currentOutbound?: string;
		currentAction?: string;
		onSave: (ruleSet: RuleSet, routeConfig?: { outbound?: string; action?: string }) => void;
		onCancel: () => void;
	}

	let { existingTags, outbounds = [], ruleSet = null, currentOutbound, currentAction, onSave, onCancel }: Props = $props();

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
	let type = $state<'remote' | 'local'>(ruleSet?.type || 'remote');
	let format = $state<'binary' | 'source'>(ruleSet?.format || 'binary');
	let url = $state(ruleSet?.url || '');
	let path = $state(ruleSet?.path || '');
	// Remote-specific options
	let downloadDetour = $state(ruleSet?.download_detour || '');
	let updateInterval = $state(ruleSet?.update_interval || '24h');

	// Route configuration
	let routeAction = $state<'none' | 'route' | 'reject'>(
		currentAction === 'reject' ? 'reject' : (currentOutbound ? 'route' : 'none')
	);
	let selectedOutbound = $state(currentOutbound || (outbounds[0]?.tag || ''));

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
			errors['tag'] = 'Tag is required';
		} else if (existingTags.includes(tag.trim())) {
			errors['tag'] = 'Tag already exists';
		}

		if (type === 'remote' && !url.trim()) {
			errors['url'] = 'URL is required for remote type';
		}

		if (type === 'local' && !path.trim()) {
			errors['path'] = 'Path is required for local type';
		}

		if (routeAction === 'route' && !selectedOutbound) {
			errors['outbound'] = 'Please select an outbound';
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
			format
		};

		if (type === 'remote') {
			newRuleSet.url = url.trim();
			if (downloadDetour) newRuleSet.download_detour = downloadDetour;
			if (updateInterval && updateInterval !== '24h') newRuleSet.update_interval = updateInterval;
		} else {
			newRuleSet.path = path.trim();
		}

		// Build route config
		let routeConfig: { outbound?: string; action?: string } | undefined;
		if (routeAction === 'reject') {
			routeConfig = { action: 'reject' };
		} else if (routeAction === 'route' && selectedOutbound) {
			routeConfig = { outbound: selectedOutbound };
		}

		onSave(newRuleSet, routeConfig);
	}
</script>

<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-6">
	<!-- Presets (only when creating) -->
	{#if !isEditing}
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Quick Add Preset</label>
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
			<p class="text-sm text-[var(--ctp-overlay0)] mb-4">Or configure manually:</p>
		</div>
	{/if}

	<div>

		<!-- Tag -->
		<div class="mb-4">
			<label for="tag" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Tag *</label>
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
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Type</label>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={() => type = 'remote'}
					class="toggle-btn {type === 'remote' ? 'selected' : ''}"
				>
					Remote
				</button>
				<button
					type="button"
					onclick={() => type = 'local'}
					class="toggle-btn {type === 'local' ? 'selected' : ''}"
				>
					Local
				</button>
			</div>
		</div>

		<!-- Format -->
		<div class="mb-4">
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Format</label>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={() => format = 'binary'}
					class="toggle-btn {format === 'binary' ? 'selected' : ''}"
				>
					Binary (.srs)
				</button>
				<button
					type="button"
					onclick={() => format = 'source'}
					class="toggle-btn {format === 'source' ? 'selected' : ''}"
				>
					Source (.json)
				</button>
			</div>
		</div>

		<!-- URL (for remote) -->
		{#if type === 'remote'}
			<div class="mb-4">
				<label for="url" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">URL *</label>
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

			<div class="grid grid-cols-2 gap-4 mb-4">
				<div>
					<label for="downloadDetour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Download Detour</label>
					<select
						id="downloadDetour"
						bind:value={downloadDetour}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">Default (direct)</option>
						{#each outbounds as ob}
							<option value={ob.tag}>{ob.tag}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">Outbound for downloading rule set</p>
				</div>
				<div>
					<label for="updateInterval" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Update Interval</label>
					<input
						id="updateInterval"
						type="text"
						bind:value={updateInterval}
						placeholder="24h"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">e.g., 24h, 7d</p>
				</div>
			</div>
		{/if}

		<!-- Path (for local) -->
		{#if type === 'local'}
			<div class="mb-4">
				<label for="path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Path *</label>
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
	</div>

	<!-- Route Configuration -->
	<div class="border-t border-[var(--ctp-surface2)] pt-4">
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">Route Action</label>
		<div class="flex gap-2 mb-3">
			<button
				type="button"
				onclick={() => routeAction = 'none'}
				class="toggle-btn {routeAction === 'none' ? 'selected' : ''}"
			>
				None
			</button>
			<button
				type="button"
				onclick={() => routeAction = 'route'}
				class="toggle-btn {routeAction === 'route' ? 'selected' : ''}"
			>
				Route to...
			</button>
			<button
				type="button"
				onclick={() => routeAction = 'reject'}
				class="toggle-btn {routeAction === 'reject' ? 'selected' : ''}"
			>
				Reject
			</button>
		</div>

		{#if routeAction === 'route'}
			<div>
				<label for="outbound" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Outbound</label>
				<select
					id="outbound"
					bind:value={selectedOutbound}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					{#each outbounds as ob}
						<option value={ob.tag}>{ob.tag}</option>
					{/each}
				</select>
				{#if errors['outbound']}
					<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['outbound']}</p>
				{/if}
			</div>
		{:else if routeAction === 'reject'}
			<p class="text-sm text-[var(--ctp-overlay0)]">
				Traffic matching this rule set will be rejected.
			</p>
		{:else}
			<p class="text-sm text-[var(--ctp-overlay0)]">
				No route rule will be created. You can add rules manually later.
			</p>
		{/if}
	</div>

	<!-- Actions -->
	<div class="flex justify-end gap-3 pt-4 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onCancel}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			Cancel
		</button>
		<button
			type="submit"
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
		>
			{isEditing ? 'Save Changes' : 'Add Rule Set'}
		</button>
	</div>
</form>

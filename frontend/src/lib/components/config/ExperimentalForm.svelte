<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ExperimentalSettings, Outbound } from '$lib/types';

	interface Props {
		settings: ExperimentalSettings;
		outbounds: Outbound[];
		onChange: (settings: ExperimentalSettings) => void;
	}

	let { settings, outbounds, onChange }: Props = $props();

	// Cache file state
	let cacheEnabled = $state(settings.cache_file?.enabled ?? false);
	let cachePath = $state(settings.cache_file?.path ?? 'cache.db');
	let cacheId = $state(settings.cache_file?.cache_id ?? '');
	let storeFakeip = $state(settings.cache_file?.store_fakeip ?? false);
	let storeRdrc = $state(settings.cache_file?.store_rdrc ?? false);

	// Clash API state
	let externalController = $state(settings.clash_api?.external_controller ?? '127.0.0.1:9090');
	let externalUi = $state(settings.clash_api?.external_ui ?? '');
	let externalUiDownloadUrl = $state(settings.clash_api?.external_ui_download_url ?? '');
	let externalUiDownloadDetour = $state(settings.clash_api?.external_ui_download_detour ?? '');
	let secret = $state(settings.clash_api?.secret ?? '');
	let defaultMode = $state<'rule' | 'global' | 'direct'>(settings.clash_api?.default_mode ?? 'rule');

	// Collapsed sections
	let cacheFileExpanded = $state(true);
	let clashApiExpanded = $state(true);

	function handleChange() {
		const newSettings: ExperimentalSettings = {};

		// Build cache_file if enabled or has path
		if (cacheEnabled || cachePath.trim()) {
			newSettings.cache_file = {
				enabled: cacheEnabled || undefined,
				path: cachePath.trim() || undefined,
				cache_id: cacheId.trim() || undefined,
				store_fakeip: storeFakeip || undefined,
				store_rdrc: storeRdrc || undefined
			};
		}

		// Build clash_api if has controller
		if (externalController.trim()) {
			newSettings.clash_api = {
				external_controller: externalController.trim() || undefined,
				external_ui: externalUi.trim() || undefined,
				external_ui_download_url: externalUiDownloadUrl.trim() || undefined,
				external_ui_download_detour: externalUiDownloadDetour.trim() || undefined,
				secret: secret || undefined,
				default_mode: defaultMode
			};
		}

		onChange(newSettings);
	}
</script>

<div class="space-y-6">
	<!-- Cache File Section -->
	<div class="border border-[var(--ctp-surface2)] rounded-lg overflow-hidden">
		<button
			type="button"
			onclick={() => cacheFileExpanded = !cacheFileExpanded}
			class="w-full px-4 py-3 flex items-center justify-between bg-[var(--ctp-surface0)] hover:bg-[var(--ctp-surface1)] transition-colors"
		>
			<div class="flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
				</svg>
				<span class="font-medium text-[var(--ctp-text)]">{$t('experimental.cacheFile')}</span>
				{#if cacheEnabled}
					<span class="status-badge">{$t('common.enabled')}</span>
				{/if}
			</div>
			<svg class="w-5 h-5 text-[var(--ctp-overlay1)] transition-transform {cacheFileExpanded ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
			</svg>
		</button>

		{#if cacheFileExpanded}
			<div class="p-4 space-y-4">
				<p class="text-sm text-[var(--ctp-overlay1)]">
					{$t('experimental.cacheFileDescription')}
				</p>

				<!-- Enable -->
				<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
					<input
						type="checkbox"
						bind:checked={cacheEnabled}
						onchange={handleChange}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
					/>
					{$t('experimental.cacheFileEnabled')}
				</label>

				<!-- Path -->
				<div>
					<label for="cache-path" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.cacheFilePath')}
					</label>
					<input
						id="cache-path"
						type="text"
						bind:value={cachePath}
						onchange={handleChange}
						placeholder="cache.db"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					/>
				</div>

				<!-- Cache ID -->
				<div>
					<label for="cache-id" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.cacheId')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
					</label>
					<input
						id="cache-id"
						type="text"
						bind:value={cacheId}
						onchange={handleChange}
						placeholder="default"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
						{$t('experimental.cacheIdHint')}
					</p>
				</div>

				<!-- Store options -->
				<div class="flex flex-col gap-2">
					<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
						<input
							type="checkbox"
							bind:checked={storeFakeip}
							onchange={handleChange}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						{$t('experimental.storeFakeip')}
					</label>
					<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
						<input
							type="checkbox"
							bind:checked={storeRdrc}
							onchange={handleChange}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						{$t('experimental.storeRdrc')}
					</label>
				</div>
			</div>
		{/if}
	</div>

	<!-- Clash API Section -->
	<div class="border border-[var(--ctp-surface2)] rounded-lg overflow-hidden">
		<button
			type="button"
			onclick={() => clashApiExpanded = !clashApiExpanded}
			class="w-full px-4 py-3 flex items-center justify-between bg-[var(--ctp-surface0)] hover:bg-[var(--ctp-surface1)] transition-colors"
		>
			<div class="flex items-center gap-2">
				<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
				<span class="font-medium text-[var(--ctp-text)]">{$t('experimental.clashApi')}</span>
				{#if externalController.trim()}
					<span class="status-badge">{externalController}</span>
				{/if}
			</div>
			<svg class="w-5 h-5 text-[var(--ctp-overlay1)] transition-transform {clashApiExpanded ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
			</svg>
		</button>

		{#if clashApiExpanded}
			<div class="p-4 space-y-4">
				<p class="text-sm text-[var(--ctp-overlay1)]">
					{$t('experimental.clashApiDescription')}
				</p>

				<!-- External Controller -->
				<div>
					<label for="external-controller" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.externalController')}
					</label>
					<input
						id="external-controller"
						type="text"
						bind:value={externalController}
						onchange={handleChange}
						placeholder="127.0.0.1:9090"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					/>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
						{$t('experimental.externalControllerHint')}
					</p>
				</div>

				<!-- External UI -->
				<div>
					<label for="external-ui" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.externalUi')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
					</label>
					<input
						id="external-ui"
						type="text"
						bind:value={externalUi}
						onchange={handleChange}
						placeholder="ui"
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					/>
				</div>

				<!-- External UI Download URL -->
				<div>
					<label for="external-ui-url" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.externalUiDownloadUrl')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
					</label>
					<input
						id="external-ui-url"
						type="text"
						bind:value={externalUiDownloadUrl}
						onchange={handleChange}
						placeholder="https://github.com/..."
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
					/>
				</div>

				<!-- Download Detour -->
				<div>
					<label for="download-detour" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.downloadDetour')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
					</label>
					<select
						id="download-detour"
						bind:value={externalUiDownloadDetour}
						onchange={handleChange}
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					>
						<option value="">{$t('common.none')} ({$t('outbounds.types.direct')})</option>
						{#each outbounds as ob}
							<option value={ob.tag}>{ob.tag}</option>
						{/each}
					</select>
					<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
						{$t('experimental.downloadDetourHint')}
					</p>
				</div>

				<!-- Secret -->
				<div>
					<label for="api-secret" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
						{$t('experimental.secret')}
						<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
					</label>
					<input
						id="api-secret"
						type="password"
						bind:value={secret}
						onchange={handleChange}
						placeholder="Enter secret..."
						class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
				</div>

				<!-- Default Mode -->
				<div>
					<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">{$t('experimental.defaultMode')}</label>
					<div class="flex gap-2">
						{#each (['rule', 'global', 'direct'] as const) as mode}
							<button
								type="button"
								onclick={() => { defaultMode = mode; handleChange(); }}
								class="toggle-btn {defaultMode === mode ? 'selected' : ''}"
							>
								{mode.charAt(0).toUpperCase() + mode.slice(1)}
							</button>
						{/each}
					</div>
				</div>
			</div>
		{/if}
	</div>
</div>

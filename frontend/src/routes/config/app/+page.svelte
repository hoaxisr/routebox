<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications, speedUnit } from '$lib/stores';
	import type { RouteBoxSettings, SettingsResponse } from '$lib/types';
	import { t } from 'svelte-i18n';
	import { setLocale } from '$lib/i18n';

	// State
	let loading = $state(true);
	let saving = $state(false);
	let settings = $state<RouteBoxSettings | null>(null);
	let settingsPath = $state('');
	let geoipLoaded = $state(false);

	// Track changes
	let originalSettings = '';
	let hasChanges = $derived(settings ? JSON.stringify(settings) !== originalSettings : false);

	onMount(async () => {
		await loadSettings();
	});

	async function loadSettings() {
		loading = true;
		try {
			const response: SettingsResponse = await api.getSettings();
			settings = response.settings;
			settingsPath = response.settings_path;
			geoipLoaded = response.geoip_loaded;
			originalSettings = JSON.stringify(response.settings);
			// Sync i18n locale with settings
			if (response.settings.ui.language) {
				setLocale(response.settings.ui.language);
			}
		} catch (err) {
			notifications.error(`Failed to load settings: ${err}`);
		} finally {
			loading = false;
		}
	}

	async function saveSettings() {
		if (!settings) return;
		saving = true;
		try {
			// Build flat updates from settings
			const updates: Record<string, unknown> = {};

			// GeoIP runtime settings
			updates['geoip.enabled'] = settings.geoip.enabled;
			updates['geoip.auto_reload'] = settings.geoip.auto_reload;

			// UI settings
			updates['ui.theme'] = settings.ui.theme;
			updates['ui.language'] = settings.ui.language;
			updates['ui.speed_unit'] = settings.ui.speed_unit;
			updates['ui.time_format'] = settings.ui.time_format;

			// Monitoring settings
			updates['monitoring.enrichment_enabled'] = settings.monitoring.enrichment_enabled;
			updates['monitoring.max_closed_connections'] = settings.monitoring.max_closed_connections;
			updates['monitoring.poll_interval_ms'] = settings.monitoring.poll_interval_ms;
			updates['monitoring.proxies_refresh_ms'] = settings.monitoring.proxies_refresh_ms;

			// Updates settings (backend ships the field; guard for older backends)
			if (settings.updates) {
				updates['updates.auto_check'] = settings.updates.auto_check;
			}

			await api.updateSettings(updates);
			originalSettings = JSON.stringify(settings);
			// Update i18n locale if language changed
			setLocale(settings.ui.language);
			// Sync speed unit store
			speedUnit.set(settings.ui.speed_unit);
			notifications.success($t('settings.settingsSaved'));
		} catch (err) {
			notifications.error(`Failed to save settings: ${err}`);
		} finally {
			saving = false;
		}
	}

	async function reloadSettings() {
		try {
			await api.reloadSettings();
			await loadSettings();
			notifications.success($t('settings.settingsReloaded'));
		} catch (err) {
			notifications.error(`Failed to reload: ${err}`);
		}
	}

	function resetChanges() {
		if (originalSettings) {
			settings = JSON.parse(originalSettings);
		}
	}
</script>

<svelte:head>
	<title>{$t('settings.title')} - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-4xl mx-auto">
	<!-- Header -->
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">App Settings</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
				Configure RouteBox application behavior
			</p>
		</div>
		<div class="flex gap-2">
			<button
				onclick={reloadSettings}
				disabled={loading || saving}
				class="px-3 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
				title="Reload from file"
			>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
				</svg>
			</button>
			{#if hasChanges}
				<button
					onclick={resetChanges}
					disabled={saving}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
				>
					Reset
				</button>
			{/if}
			<button
				onclick={saveSettings}
				disabled={!hasChanges || saving}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
			>
				{#if saving}
					<svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
					Saving...
				{:else}
					Save Changes
				{/if}
			</button>
		</div>
	</div>

	<!-- Settings path info -->
	{#if settingsPath}
		<div class="mb-6 px-4 py-3 bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] flex items-center gap-3">
			<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
			</svg>
			<code class="text-sm text-[var(--ctp-subtext1)]">{settingsPath}</code>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="animate-spin h-8 w-8 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else if settings}
		<div class="space-y-6">
			<!-- GeoIP Section -->
			<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
				<div class="flex items-center gap-3 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">GeoIP</h2>
					{#if geoipLoaded}
						<span class="px-2 py-0.5 text-xs rounded-full bg-[var(--ctp-green)]/20 text-[var(--ctp-green)]">
							Database Loaded
						</span>
					{:else}
						<span class="px-2 py-0.5 text-xs rounded-full bg-[var(--ctp-overlay0)]/20 text-[var(--ctp-overlay1)]">
							Not Configured
						</span>
					{/if}
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Database Path</label>
						<input
							type="text"
							value={settings.geoip.path || 'Not configured'}
							disabled
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] cursor-not-allowed"
						/>
						<p class="text-xs text-[var(--ctp-overlay0)] mt-1">Requires restart to change</p>
					</div>

					<div class="space-y-3">
						<label class="flex items-center gap-3 cursor-pointer">
							<input
								type="checkbox"
								bind:checked={settings.geoip.enabled}
								disabled={!geoipLoaded}
								class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)] disabled:opacity-50"
							/>
							<span class="text-sm text-[var(--ctp-text)]">Enable GeoIP enrichment</span>
						</label>

						<label class="flex items-center gap-3 cursor-pointer">
							<input
								type="checkbox"
								bind:checked={settings.geoip.auto_reload}
								disabled={!geoipLoaded}
								class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)] disabled:opacity-50"
							/>
							<span class="text-sm text-[var(--ctp-text)]">Auto-reload on file change</span>
						</label>
					</div>
				</div>
			</section>

			<!-- UI Section -->
			<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
				<div class="flex items-center gap-3 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">User Interface</h2>
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Default Theme</label>
						<select
							bind:value={settings.ui.theme}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							<option value="system">System</option>
							<option value="dark">Dark</option>
							<option value="light">Light</option>
						</select>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Language</label>
						<select
							bind:value={settings.ui.language}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							<option value="en">English</option>
						</select>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Speed Unit</label>
						<select
							bind:value={settings.ui.speed_unit}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							<option value="bytes">Bytes (MB/s)</option>
							<option value="bits">Bits (Mbps)</option>
						</select>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Time Format</label>
						<select
							bind:value={settings.ui.time_format}
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						>
							<option value="relative">Relative (5m ago)</option>
							<option value="absolute">Absolute (14:32:15)</option>
							<option value="both">Both</option>
						</select>
					</div>
				</div>
			</section>

			<!-- Monitoring Section -->
			<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
				<div class="flex items-center gap-3 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">Monitoring</h2>
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<div class="md:col-span-2">
						<label class="flex items-center gap-3 cursor-pointer">
							<input
								type="checkbox"
								bind:checked={settings.monitoring.enrichment_enabled}
								class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
							/>
							<span class="text-sm text-[var(--ctp-text)]">Enable connection enrichment (GeoIP, etc.)</span>
						</label>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Max Closed Connections History</label>
						<input
							type="number"
							bind:value={settings.monitoring.max_closed_connections}
							min="0"
							max="10000"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Poll Interval (ms)</label>
						<input
							type="number"
							bind:value={settings.monitoring.poll_interval_ms}
							min="500"
							max="30000"
							step="100"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
						<p class="text-xs text-[var(--ctp-overlay0)] mt-1">Fallback when WebSocket unavailable</p>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Proxies Refresh (ms)</label>
						<input
							type="number"
							bind:value={settings.monitoring.proxies_refresh_ms}
							min="0"
							max="60000"
							step="1000"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
						<p class="text-xs text-[var(--ctp-overlay0)] mt-1">0 to disable auto-refresh</p>
					</div>
				</div>
			</section>

			<!-- Updates Section -->
			{#if settings.updates}
				<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
					<div class="flex items-center gap-3 mb-4">
						<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
						</svg>
						<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('settings.updatesSection')}</h2>
					</div>

					<label class="flex items-center gap-3 cursor-pointer">
						<input
							type="checkbox"
							bind:checked={settings.updates.auto_check}
							class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
						/>
						<span class="text-sm text-[var(--ctp-text)]">{$t('settings.autoCheckUpdates')}</span>
					</label>
				</section>
			{/if}

			<!-- Read-only sections -->
			<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
				<div class="flex items-center gap-3 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">Restart Required</h2>
					<span class="text-xs text-[var(--ctp-overlay0)]">Edit routebox.toml to change</span>
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4 opacity-60">
					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Listen Address</label>
						<input
							type="text"
							value={settings.network.listen}
							disabled
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] cursor-not-allowed"
						/>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Log Level</label>
						<input
							type="text"
							value={settings.logging.level}
							disabled
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] cursor-not-allowed"
						/>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Sing-box Config</label>
						<input
							type="text"
							value={settings.singbox.config_path || 'Auto-detect'}
							disabled
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] cursor-not-allowed"
						/>
					</div>

					<div>
						<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">Binary Name</label>
						<input
							type="text"
							value={settings.singbox.binary_name}
							disabled
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] cursor-not-allowed"
						/>
					</div>
				</div>
			</section>

			<!-- About Section -->
			<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
				<div class="flex items-center gap-3 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">About</h2>
				</div>

				<div class="flex items-center justify-between">
					<div>
						<div class="text-[var(--ctp-text)] font-medium">RouteBox</div>
						<div class="text-sm text-[var(--ctp-overlay1)]">WebUI for amnezia-box router</div>
					</div>
					<div class="text-right">
						<div class="text-lg font-semibold text-[var(--ctp-primary)]">v{__APP_VERSION__}</div>
						<a
							href="https://github.com/hoaxisr/routebox"
							target="_blank"
							rel="noopener noreferrer"
							class="text-sm text-[var(--ctp-blue)] hover:underline"
						>
							GitHub
						</a>
					</div>
				</div>
			</section>
		</div>
	{/if}
</div>

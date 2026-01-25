<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { LogSettings, ExperimentalSettings, Outbound } from '$lib/types';
	import LogForm from '$lib/components/config/LogForm.svelte';
	import ExperimentalForm from '$lib/components/config/ExperimentalForm.svelte';

	// State
	let loading = $state(true);
	let applying = $state(false);
	let hasChanges = $state(false);

	let logSettings = $state<LogSettings>({ level: 'info' });
	let experimentalSettings = $state<ExperimentalSettings>({});
	let outbounds = $state<Outbound[]>([]);

	// Track original values for change detection
	let originalLog = '';
	let originalExperimental = '';

	onMount(async () => {
		await loadData();
	});

	async function loadData() {
		loading = true;
		try {
			const [log, exp, obs] = await Promise.all([
				api.getLogSettings(),
				api.getExperimental(),
				api.listOutbounds()
			]);

			logSettings = log;
			experimentalSettings = exp;
			outbounds = obs;

			originalLog = JSON.stringify(log);
			originalExperimental = JSON.stringify(exp);
			hasChanges = false;
		} catch (err) {
			notifications.error(`Failed to load settings: ${err}`);
		} finally {
			loading = false;
		}
	}

	function checkChanges() {
		const changed =
			JSON.stringify(logSettings) !== originalLog ||
			JSON.stringify(experimentalSettings) !== originalExperimental;

		if (changed && !hasChanges) {
			unsavedChanges.markChanged($t('settings.title'), $t('settings.description'));
		}
		hasChanges = changed;
	}

	function handleLogChange(settings: LogSettings) {
		logSettings = settings;
		checkChanges();
	}

	function handleExperimentalChange(settings: ExperimentalSettings) {
		experimentalSettings = settings;
		checkChanges();
	}

	async function applyChanges() {
		applying = true;
		try {
			// Save log settings
			await api.updateLogSettings(logSettings);
			// Save experimental settings
			await api.updateExperimental(experimentalSettings);
			// Apply to config file
			await api.applyConfig();

			originalLog = JSON.stringify(logSettings);
			originalExperimental = JSON.stringify(experimentalSettings);
			hasChanges = false;
			unsavedChanges.clearChanges();

			notifications.success($t('settings.settingsSaved'));
		} catch (err) {
			notifications.error(`Failed to apply: ${err}`);
		} finally {
			applying = false;
		}
	}

	async function resetChanges() {
		await loadData();
		notifications.info($t('changes.configDiscarded'));
	}
</script>

<svelte:head>
	<title>{$t('settings.title')} - {$t('app.name')}</title>
</svelte:head>

<div class="p-6 max-w-4xl mx-auto">
	<!-- Header -->
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">{$t('settings.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{$t('settings.description')}
			</p>
		</div>
		<div class="flex gap-2">
			{#if hasChanges}
				<button
					type="button"
					onclick={resetChanges}
					disabled={applying}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
				>
					{$t('common.reset')}
				</button>
			{/if}
			<button
				type="button"
				onclick={applyChanges}
				disabled={!hasChanges || applying}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 flex items-center gap-2"
			>
				{#if applying}
					<svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
					{$t('common.applying')}
				{:else}
					{$t('changes.applyChanges')}
				{/if}
			</button>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="animate-spin h-8 w-8 text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else}
		<div class="space-y-8">
			<!-- Log Settings -->
			<section>
				<div class="flex items-center gap-2 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('log.title')}</h2>
				</div>
				<div class="bg-[var(--ctp-mantle)] rounded-lg p-4 border border-[var(--ctp-surface0)]">
					<LogForm settings={logSettings} onChange={handleLogChange} />
				</div>
			</section>

			<!-- Experimental Settings -->
			<section>
				<div class="flex items-center gap-2 mb-4">
					<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
					</svg>
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('experimental.title')}</h2>
				</div>
				<div class="bg-[var(--ctp-mantle)] rounded-lg p-4 border border-[var(--ctp-surface0)]">
					<ExperimentalForm
						settings={experimentalSettings}
						{outbounds}
						onChange={handleExperimentalChange}
					/>
				</div>
			</section>
		</div>
	{/if}
</div>

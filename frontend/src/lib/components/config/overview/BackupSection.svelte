<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { SingboxConfig } from '$lib/types';

	let importing = $state(false);
	let applying = $state(false);
	let dragOver = $state(false);
	let importedConfig = $state<SingboxConfig | null>(null);
	let validationResult = $state<{ valid: boolean; errors: string[] } | null>(null);

	function handleExport() {
		api.exportConfig();
		notifications.success($t('backup.downloadStarted'));
	}

	function handleFileDrop(event: DragEvent) {
		event.preventDefault();
		dragOver = false;
		const file = event.dataTransfer?.files[0];
		if (file) processFile(file);
	}

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (file) processFile(file);
	}

	async function processFile(file: File) {
		if (!file.name.endsWith('.json')) {
			notifications.error($t('backup.jsonFileRequired'));
			return;
		}
		importing = true;
		try {
			const text = await file.text();
			const parsed = JSON.parse(text);
			const result = await api.importConfig(parsed);
			importedConfig = result.config;
			validationResult = { valid: result.valid, errors: result.errors };
			if (result.valid) {
				notifications.success($t('backup.configValidated'));
			} else {
				notifications.warning($t('backup.validationErrorCount', { values: { count: result.errors.length } }));
			}
		} catch (err) {
			notifications.error($t('backup.parseFailed', { values: { error: String(err) } }));
			importedConfig = null;
			validationResult = null;
		} finally {
			importing = false;
		}
	}

	async function applyImportedConfig() {
		if (!importedConfig || !validationResult?.valid) return;
		applying = true;
		try {
			await api.saveConfig(importedConfig!);
			notifications.success($t('backup.savedAsDraft'));
			unsavedChanges.markChanged('Config', 'Imported config from backup');
			unsavedChanges.refresh();
			importedConfig = null;
			validationResult = null;
		} catch (err) {
			notifications.error($t('backup.applyFailed', { values: { error: String(err) } }));
		} finally {
			applying = false;
		}
	}

	function clearImport() {
		importedConfig = null;
		validationResult = null;
	}
</script>

<div class="space-y-6">
	<!-- Export Section -->
	<section class="bg-[var(--ctp-surface0)] rounded-xl p-6">
		<div class="flex items-start gap-4">
			<div class="p-3 bg-[var(--ctp-surface1)] rounded-lg">
				<svg class="w-6 h-6 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
				</svg>
			</div>
			<div class="flex-1">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('backup.exportConfig')}</h2>
				<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('backup.exportDescription')}</p>
				<button
					onclick={handleExport}
					class="mt-4 px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity flex items-center gap-2"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
					</svg>
					{$t('backup.exportConfig')}
				</button>
			</div>
		</div>
	</section>

	<!-- Import Section -->
	<section class="bg-[var(--ctp-surface0)] rounded-xl p-6">
		<div class="flex items-start gap-4 mb-4">
			<div class="p-3 bg-[var(--ctp-surface1)] rounded-lg">
				<svg class="w-6 h-6 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
				</svg>
			</div>
			<div class="flex-1">
				<h2 class="text-lg font-medium text-[var(--ctp-text)]">{$t('backup.importConfig')}</h2>
				<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('backup.importDescription')}</p>
			</div>
		</div>

		<!-- Drop Zone -->
		<div
			role="button"
			tabindex="0"
			ondrop={handleFileDrop}
			ondragover={(e) => { e.preventDefault(); dragOver = true; }}
			ondragleave={() => dragOver = false}
			onclick={() => document.getElementById('backup-file-input')?.click()}
			onkeypress={(e) => { if (e.key === 'Enter') document.getElementById('backup-file-input')?.click(); }}
			class="dropzone {dragOver ? 'active' : ''}"
		>
			<input
				id="backup-file-input"
				type="file"
				accept=".json"
				onchange={handleFileSelect}
				class="hidden"
			/>
			{#if importing}
				<svg class="w-12 h-12 mx-auto text-[var(--ctp-primary)] animate-spin" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
				<p class="mt-2 text-[var(--ctp-text)]">{$t('common.validating')}</p>
			{:else}
				<svg class="w-12 h-12 mx-auto text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
				</svg>
				<p class="mt-2 text-[var(--ctp-text)]">{$t('backup.dropFile')}</p>
				<p class="text-sm text-[var(--ctp-overlay0)]">{$t('backup.selectFile')}</p>
			{/if}
		</div>

		<!-- Validation Results -->
		{#if validationResult}
			<div class="mt-4 result-card {validationResult.valid ? 'success' : 'error'}">
				<div class="flex items-center gap-2 mb-2">
					{#if validationResult.valid}
						<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
						</svg>
						<span class="font-medium text-[var(--ctp-primary)]">{$t('jsonEditor.configValid')}</span>
					{:else}
						<svg class="w-5 h-5 text-[var(--ctp-red)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
						</svg>
						<span class="font-medium text-[var(--ctp-red)]">{$t('backup.invalidConfig')}</span>
					{/if}
				</div>
				{#if !validationResult.valid && validationResult.errors.length > 0}
					<ul class="mt-2 space-y-1 text-sm text-[var(--ctp-red)]">
						{#each validationResult.errors as error}
							<li class="flex items-start gap-2">
								<span class="text-[var(--ctp-overlay0)]">•</span>
								{error}
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="mt-4 flex gap-3">
				<button
					onclick={clearImport}
					class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
				>
					{$t('common.discard')}
				</button>
				{#if validationResult.valid}
					<button
						onclick={applyImportedConfig}
						disabled={applying}
						class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center gap-2"
					>
						{#if applying}
							<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
							</svg>
							{$t('common.applying')}
						{:else}
							{$t('common.apply')}
						{/if}
					</button>
				{/if}
			</div>
		{/if}
	</section>

	<!-- Warning -->
	<div class="p-4 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg">
		<div class="flex items-start gap-3">
			<svg class="w-5 h-5 text-[var(--ctp-overlay1)] flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
			</svg>
			<div>
				<p class="text-sm font-medium text-[var(--ctp-text)]">{$t('common.warning')}</p>
				<p class="text-sm text-[var(--ctp-subtext1)] mt-1">{$t('backup.importWarning')}</p>
			</div>
		</div>
	</div>
</div>

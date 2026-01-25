<script lang="ts">
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';

	// State
	let importing = $state(false);
	let applying = $state(false);
	let dragOver = $state(false);

	let importedConfig = $state<object | null>(null);
	let validationResult = $state<{ valid: boolean; errors: string[] } | null>(null);

	function handleExport() {
		api.exportConfig();
		notifications.success('Config download started');
	}

	function handleFileDrop(event: DragEvent) {
		event.preventDefault();
		dragOver = false;

		const file = event.dataTransfer?.files[0];
		if (file) {
			processFile(file);
		}
	}

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (file) {
			processFile(file);
		}
	}

	async function processFile(file: File) {
		if (!file.name.endsWith('.json')) {
			notifications.error('Please select a JSON file');
			return;
		}

		importing = true;
		try {
			const text = await file.text();
			const config = JSON.parse(text);

			const result = await api.importConfig(config);
			importedConfig = result.config;
			validationResult = { valid: result.valid, errors: result.errors };

			if (result.valid) {
				notifications.success('Config validated successfully');
			} else {
				notifications.warning(`Config has ${result.errors.length} validation error(s)`);
			}
		} catch (err) {
			notifications.error(`Failed to parse config: ${err}`);
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
			await api.saveConfig(importedConfig as any);
			notifications.success('Config applied successfully');
			importedConfig = null;
			validationResult = null;
		} catch (err) {
			notifications.error(`Failed to apply config: ${err}`);
		} finally {
			applying = false;
		}
	}

	function clearImport() {
		importedConfig = null;
		validationResult = null;
	}
</script>

<svelte:head>
	<title>Backup & Restore - RouteBox</title>
</svelte:head>

<div class="p-6 max-w-4xl mx-auto">
	<!-- Header -->
	<div class="mb-6">
		<h1 class="text-2xl font-semibold text-[var(--ctp-text)]">Backup & Restore</h1>
		<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
			Export or import your sing-box configuration
		</p>
	</div>

	<div class="space-y-6">
		<!-- Export Section -->
		<section class="bg-[var(--ctp-mantle)] rounded-lg p-6 border border-[var(--ctp-surface0)]">
			<div class="flex items-start gap-4">
				<div class="p-3 bg-[var(--ctp-surface1)] rounded-lg">
					<svg class="w-6 h-6 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
					</svg>
				</div>
				<div class="flex-1">
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">Export Configuration</h2>
					<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
						Download your current configuration as a JSON file for backup or transfer.
					</p>
					<button
						onclick={handleExport}
						class="mt-4 px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity flex items-center gap-2"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
						</svg>
						Export Config
					</button>
				</div>
			</div>
		</section>

		<!-- Import Section -->
		<section class="bg-[var(--ctp-mantle)] rounded-lg p-6 border border-[var(--ctp-surface0)]">
			<div class="flex items-start gap-4 mb-4">
				<div class="p-3 bg-[var(--ctp-surface1)] rounded-lg">
					<svg class="w-6 h-6 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
					</svg>
				</div>
				<div class="flex-1">
					<h2 class="text-lg font-medium text-[var(--ctp-text)]">Import Configuration</h2>
					<p class="text-sm text-[var(--ctp-overlay1)] mt-1">
						Upload a JSON configuration file to validate and apply.
					</p>
				</div>
			</div>

			<!-- Drop Zone -->
			<div
				role="button"
				tabindex="0"
				ondrop={handleFileDrop}
				ondragover={(e) => { e.preventDefault(); dragOver = true; }}
				ondragleave={() => dragOver = false}
				onclick={() => document.getElementById('file-input')?.click()}
				onkeypress={(e) => { if (e.key === 'Enter') document.getElementById('file-input')?.click(); }}
				class="dropzone {dragOver ? 'active' : ''}"
			>
				<input
					id="file-input"
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
					<p class="mt-2 text-[var(--ctp-text)]">Processing...</p>
				{:else}
					<svg class="w-12 h-12 mx-auto text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
					</svg>
					<p class="mt-2 text-[var(--ctp-text)]">
						Drag & drop a JSON file here
					</p>
					<p class="text-sm text-[var(--ctp-overlay0)]">
						or click to browse
					</p>
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
							<span class="font-medium text-[var(--ctp-primary)]">Configuration is valid</span>
						{:else}
							<svg class="w-5 h-5 text-[var(--ctp-red)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
							</svg>
							<span class="font-medium text-[var(--ctp-red)]">Configuration has errors</span>
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

				<!-- Actions -->
				<div class="mt-4 flex gap-3">
					<button
						onclick={clearImport}
						class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
					>
						Clear
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
								Applying...
							{:else}
								Apply Configuration
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
					<p class="text-sm font-medium text-[var(--ctp-text)]">Important</p>
					<p class="text-sm text-[var(--ctp-subtext1)] mt-1">
						Importing a configuration will replace your current settings. Make sure to export a backup first if needed.
					</p>
				</div>
			</div>
		</div>
	</div>
</div>

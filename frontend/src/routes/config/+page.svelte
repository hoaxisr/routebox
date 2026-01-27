<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges } from '$lib/stores';
	import type { SingboxConfig } from '$lib/types';
	import SideBySideDiff from '$lib/components/shared/SideBySideDiff.svelte';

	let config: SingboxConfig | null = $state(null);
	let loading = $state(true);

	// Read initial tab from URL query param (?tab=diff)
	const validTabs = ['overview', 'json', 'diff', 'backup'] as const;
	type Tab = typeof validTabs[number];
	function getInitialTab(): Tab {
		if (typeof window !== 'undefined') {
			const param = new URLSearchParams(window.location.search).get('tab');
			if (param && validTabs.includes(param as Tab)) return param as Tab;
		}
		return 'overview';
	}
	let activeTab = $state<Tab>(getInitialTab());

	// Backup state
	let importing = $state(false);
	let applying = $state(false);
	let dragOver = $state(false);
	let importedConfig = $state<object | null>(null);
	let validationResult = $state<{ valid: boolean; errors: string[] } | null>(null);

	// For diff view
	let savedConfig: string = $state('');
	let currentConfig: string = $state('');
	let hasChanges = $derived(savedConfig !== currentConfig && savedConfig !== '');

	// Editor state
	let editing = $state(false);
	let editorContainer: HTMLDivElement | undefined = $state(undefined);
	let editorView: any = null;
	let editorModules: any = null;
	let saving = $state(false);
	let validating = $state(false);
	let validationErrors = $state<string[]>([]);
	let originalContent = '';

	async function fetchConfig() {
		loading = true;
		try {
			const [current, active] = await Promise.all([
				api.getConfig(),
				api.getActiveConfig()
			]);
			config = current;
			currentConfig = JSON.stringify(current, null, 2);
			savedConfig = JSON.stringify(active, null, 2);
			originalContent = currentConfig;
		} catch (e) {
			notifications.error(`Failed to load config: ${e}`);
		} finally {
			loading = false;
		}
	}

	function copyToClipboard() {
		navigator.clipboard.writeText(currentConfig);
		notifications.success('Config copied to clipboard');
	}

	function downloadConfig() {
		const blob = new Blob([currentConfig], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'config.json';
		a.click();
		URL.revokeObjectURL(url);
	}

	// Simple JSON syntax highlighting
	function highlightJson(json: string): string {
		return json
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?)/g, (match) => {
				let cls = 'json-string';
				if (match.endsWith(':')) {
					cls = 'json-key';
				}
				return `<span class="${cls}">${match}</span>`;
			})
			.replace(/\b(true|false)\b/g, '<span class="json-boolean">$1</span>')
			.replace(/\b(null)\b/g, '<span class="json-null">$1</span>')
			.replace(/\b(-?\d+\.?\d*)\b/g, '<span class="json-number">$1</span>');
	}

	// Lazy-load CodeMirror modules
	async function loadEditorModules() {
		if (editorModules) return editorModules;
		const [cm, cmState, cmJson, cmTheme] = await Promise.all([
			import('codemirror'),
			import('@codemirror/state'),
			import('@codemirror/lang-json'),
			import('@codemirror/theme-one-dark')
		]);
		editorModules = { ...cm, ...cmState, ...cmJson, ...cmTheme };
		return editorModules;
	}

	async function startEditing() {
		editing = true;
		validationErrors = [];

		// Wait for DOM to render the container
		await new Promise(r => requestAnimationFrame(r));
		await new Promise(r => requestAnimationFrame(r));

		if (!editorContainer) return;

		const mod = await loadEditorModules();
		const themeCompartment = new mod.Compartment();

		const state = mod.EditorState.create({
			doc: currentConfig,
			extensions: [
				mod.basicSetup,
				mod.json(),
				themeCompartment.of(mod.oneDark),
				mod.EditorView.lineWrapping,
				mod.EditorView.theme({
					'&': {
						height: '100%',
						fontSize: '13px'
					},
					'.cm-scroller': {
						overflow: 'auto'
					},
					'.cm-content': {
						fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
					}
				})
			]
		});

		editorView = new mod.EditorView({
			state,
			parent: editorContainer
		});
	}

	function stopEditing() {
		editorView?.destroy();
		editorView = null;
		editing = false;
		validationErrors = [];
	}

	async function handleSave() {
		if (!editorView) return;

		const content = editorView.state.doc.toString();

		let parsed: SingboxConfig;
		try {
			parsed = JSON.parse(content);
		} catch (err) {
			notifications.error(`Invalid JSON: ${err}`);
			return;
		}

		saving = true;
		try {
			await api.saveConfig(parsed);
			notifications.success($t('jsonEditor.saveToDraft'));
			unsavedChanges.markChanged('Config', 'Updated via JSON editor');
			unsavedChanges.refresh();
			originalContent = content;
			currentConfig = content;
			config = parsed;
			stopEditing();
		} catch (err) {
			notifications.error(`Failed to save: ${err}`);
		} finally {
			saving = false;
		}
	}

	async function handleValidate() {
		if (!editorView) return;

		const content = editorView.state.doc.toString();

		let parsed: SingboxConfig;
		try {
			parsed = JSON.parse(content);
		} catch (err) {
			validationErrors = [`JSON syntax error: ${err}`];
			return;
		}

		validating = true;
		try {
			const result = await api.validateConfig(parsed);
			if (result.valid) {
				validationErrors = [];
				notifications.success($t('jsonEditor.configValid'));
			} else {
				validationErrors = result.errors;
			}
		} catch (err) {
			validationErrors = [`Validation failed: ${err}`];
		} finally {
			validating = false;
		}
	}

	function handleFormat() {
		if (!editorView) return;

		const content = editorView.state.doc.toString();

		try {
			const formatted = JSON.stringify(JSON.parse(content), null, 2);
			editorView.dispatch({
				changes: {
					from: 0,
					to: editorView.state.doc.length,
					insert: formatted
				}
			});
		} catch (err) {
			notifications.error(`Cannot format: invalid JSON`);
		}
	}

	function handleReset() {
		if (!editorView || !originalContent) return;

		editorView.dispatch({
			changes: {
				from: 0,
				to: editorView.state.doc.length,
				insert: originalContent
			}
		});
		validationErrors = [];
	}

	// Backup functions
	function handleExport() {
		api.exportConfig();
		notifications.success('Config download started');
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
			notifications.error('Please select a JSON file');
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

	onMount(fetchConfig);

	onDestroy(() => {
		editorView?.destroy();
	});
</script>

<svelte:head>
	<title>{$t('overview.title')} - RouteBox</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('overview.title')}</h1>
		<div class="flex gap-2">
			{#if activeTab === 'json' && !editing}
				<button
					onclick={startEditing}
					class="px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
					</svg>
					{$t('common.edit')}
				</button>
				<button
					onclick={copyToClipboard}
					class="px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
					</svg>
					{$t('common.copy')}
				</button>
				<button
					onclick={downloadConfig}
					class="px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
					</svg>
					{$t('common.download')}
				</button>
			{/if}
			{#if !editing}
				<button
					onclick={fetchConfig}
					class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
					</svg>
					{$t('common.refresh')}
				</button>
			{/if}
		</div>
	</div>

	<!-- Tabs (hidden in edit mode) -->
	{#if !editing}
		<div class="flex gap-1 p-1 bg-[var(--ctp-surface0)] rounded-lg w-fit">
			<button
				onclick={() => activeTab = 'overview'}
				class="px-4 py-1.5 rounded-md text-sm transition-colors {activeTab === 'overview' ? 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
			>
				{$t('nav.overview')}
			</button>
			<button
				onclick={() => activeTab = 'json'}
				class="px-4 py-1.5 rounded-md text-sm transition-colors {activeTab === 'json' ? 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
			>
				JSON
			</button>
			<button
				onclick={() => activeTab = 'diff'}
				class="px-4 py-1.5 rounded-md text-sm transition-colors flex items-center gap-1.5 {activeTab === 'diff' ? 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
			>
				Diff
				{#if hasChanges}
					<span class="w-2 h-2 rounded-full bg-[var(--ctp-primary)]"></span>
				{/if}
			</button>
			<button
				onclick={() => activeTab = 'backup'}
				class="px-4 py-1.5 rounded-md text-sm transition-colors {activeTab === 'backup' ? 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
			>
				{$t('nav.backup')}
			</button>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="w-8 h-8 text-[var(--ctp-primary)] animate-spin" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else if !config}
		<div class="text-center py-12 text-[var(--ctp-red)]">
			{$t('errors.loadFailed')}
		</div>
	{:else}
		<!-- Overview Tab -->
		{#if activeTab === 'overview' && !editing}
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
				<a href="/config/endpoints" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.endpoints')}</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.endpoints?.length ?? 0}
					</div>
				</a>
				<a href="/config/outbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.outbounds')}</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.outbounds?.length ?? 0}
					</div>
				</a>
				<a href="/config/inbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.inbounds')}</div>
					<div class="text-2xl font-bold text-[var(--ctp-text)]">
						{config.inbounds?.length ?? 0}
					</div>
				</a>
				<a href="/config/routes" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.rules')}</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.route?.rules?.length ?? 0}
					</div>
				</a>
			</div>

			<!-- Quick stats -->
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
					<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">{$t('dns.title')}</h3>
					<div class="space-y-2 text-sm">
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">{$t('dns.servers')}</span>
							<span class="text-[var(--ctp-text)]">{config.dns?.servers?.length ?? 0}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">{$t('dns.rules')}</span>
							<span class="text-[var(--ctp-text)]">{config.dns?.rules?.length ?? 0}</span>
						</div>
					</div>
				</div>
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
					<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">{$t('routes.title')}</h3>
					<div class="space-y-2 text-sm">
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">{$t('routes.ruleSets')}</span>
							<span class="text-[var(--ctp-text)]">{config.route?.rule_set?.length ?? 0}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">{$t('routes.finalOutbound')}</span>
							<span class="text-[var(--ctp-text)]">{config.route?.final ?? 'direct'}</span>
						</div>
					</div>
				</div>
			</div>
		{/if}

		<!-- JSON Tab: View mode -->
		{#if activeTab === 'json' && !editing}
			<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
				<div class="px-4 py-2 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
					<span class="text-sm font-medium text-[var(--ctp-subtext1)]">config.json</span>
					<span class="text-xs text-[var(--ctp-overlay0)]">{currentConfig.split('\n').length} lines</span>
				</div>
				<div class="p-4 overflow-auto max-h-[calc(100vh-280px)] json-viewer">
					<pre class="text-sm font-mono leading-relaxed">{@html highlightJson(currentConfig)}</pre>
				</div>
			</div>
		{/if}

		<!-- JSON Tab: Edit mode -->
		{#if editing}
			<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
				<!-- Editor toolbar -->
				<div class="flex items-center justify-between px-4 py-2 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)]">
					<div class="flex items-center gap-2">
						<button
							type="button"
							onclick={handleFormat}
							class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
						>
							{$t('common.format')}
						</button>
						<button
							type="button"
							onclick={handleValidate}
							disabled={validating}
							class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
						>
							{validating ? $t('common.validate') + '...' : $t('common.validate')}
						</button>
						<button
							type="button"
							onclick={handleReset}
							class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
						>
							{$t('common.reset')}
						</button>
					</div>

					<div class="flex items-center gap-2">
						<button
							type="button"
							onclick={stopEditing}
							class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
						>
							{$t('common.cancel')}
						</button>
						<button
							type="button"
							onclick={handleSave}
							disabled={saving}
							class="px-4 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50"
						>
							{saving ? $t('common.saving') + '...' : $t('jsonEditor.saveToDraft')}
						</button>
					</div>
				</div>

				<!-- Validation errors -->
				{#if validationErrors.length > 0}
					<div class="p-3 bg-[var(--ctp-red)]/10 border-b border-[var(--ctp-red)]/30">
						<div class="text-sm text-[var(--ctp-red)] font-medium mb-1">{$t('jsonEditor.validationErrors')}:</div>
						<ul class="text-xs text-[var(--ctp-red)] space-y-1">
							{#each validationErrors as error}
								<li>{error}</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- CodeMirror container -->
				<div bind:this={editorContainer} class="editor-container overflow-auto max-h-[calc(100vh-320px)]"></div>
			</div>
		{/if}

		<!-- Backup Tab -->
		{#if activeTab === 'backup' && !editing}
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
		{/if}

		<!-- Diff Tab -->
		{#if activeTab === 'diff' && !editing}
			{#if !hasChanges}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
					<svg class="w-12 h-12 mx-auto text-[var(--ctp-overlay0)] mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<p class="text-[var(--ctp-subtext1)]">No changes detected</p>
					<p class="text-sm text-[var(--ctp-overlay0)] mt-1">
						The draft configuration matches the active configuration
					</p>
				</div>
			{:else}
				<SideBySideDiff oldText={savedConfig} newText={currentConfig} />
			{/if}
		{/if}
	{/if}
</div>

<style>
	/* JSON syntax highlighting */
	:global(.json-viewer .json-key) {
		color: var(--ctp-primary);
	}
	:global(.json-viewer .json-string) {
		color: #a6e3a1;
	}
	:global(.json-viewer .json-number) {
		color: #f9e2af;
	}
	:global(.json-viewer .json-boolean) {
		color: #89b4fa;
	}
	:global(.json-viewer .json-null) {
		color: var(--ctp-overlay0);
	}

	/* Light theme overrides */
	:global(:root.light .json-viewer .json-string) {
		color: #40a02b;
	}
	:global(:root.light .json-viewer .json-number) {
		color: #df8e1d;
	}
	:global(:root.light .json-viewer .json-boolean) {
		color: #1e66f5;
	}

	/* CodeMirror container */
	.editor-container :global(.cm-editor) {
		height: 100%;
	}
</style>

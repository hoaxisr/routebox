<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges, configReadOnly } from '$lib/stores';
	import type { SingboxConfig } from '$lib/types';
	import SideBySideDiff from '$lib/components/shared/SideBySideDiff.svelte';
	import StatsGrid from '$lib/components/config/overview/StatsGrid.svelte';
	import BackupSection from '$lib/components/config/overview/BackupSection.svelte';

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
					'&': { height: '100%', fontSize: '13px' },
					'.cm-scroller': { overflow: 'auto' },
					'.cm-content': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }
				})
			]
		});

		editorView = new mod.EditorView({ state, parent: editorContainer });
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
				changes: { from: 0, to: editorView.state.doc.length, insert: formatted }
			});
		} catch (err) {
			notifications.error(`Cannot format: invalid JSON`);
		}
	}

	function handleReset() {
		if (!editorView || !originalContent) return;

		editorView.dispatch({
			changes: { from: 0, to: editorView.state.doc.length, insert: originalContent }
		});
		validationErrors = [];
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

	<!-- Tabs -->
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
			<StatsGrid {config} />
		{/if}

		<!-- JSON Tab: View mode -->
		{#if activeTab === 'json' && !editing}
			<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
				<div class="px-4 py-2 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
					<span class="text-sm font-medium text-[var(--ctp-subtext1)]">config.json</span>
					<span class="text-xs text-[var(--ctp-overlay0)]">{currentConfig.split('\n').length} lines</span>
				</div>
				<div class="p-4 overflow-auto max-h-[calc(100vh-280px)] json-viewer">
					<pre class="text-sm leading-relaxed">{@html highlightJson(currentConfig)}</pre>
				</div>
			</div>
		{/if}

		<!-- JSON Tab: Edit mode -->
		{#if editing}
			<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
				<div class="flex items-center justify-between px-4 py-2 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)]">
					<div class="flex items-center gap-2">
						<button type="button" onclick={handleFormat} class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">
							{$t('common.format')}
						</button>
						<button type="button" onclick={handleValidate} disabled={validating} class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50">
							{validating ? $t('common.validate') + '...' : $t('common.validate')}
						</button>
						<button type="button" onclick={handleReset} class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">
							{$t('common.reset')}
						</button>
					</div>
					<div class="flex items-center gap-2">
						<button type="button" onclick={stopEditing} class="px-3 py-1.5 text-sm bg-[var(--ctp-surface0)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors">
							{$t('common.cancel')}
						</button>
						<button
							type="button"
							onclick={handleSave}
							disabled={saving || $configReadOnly}
							title={$configReadOnly ? $t('readOnly.saveBlocked') : ''}
							class="px-4 py-1.5 text-sm bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{saving ? $t('common.saving') + '...' : $t('jsonEditor.saveToDraft')}
						</button>
					</div>
				</div>

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

				<div bind:this={editorContainer} class="editor-container overflow-auto max-h-[calc(100vh-320px)]"></div>
			</div>
		{/if}

		<!-- Backup Tab -->
		{#if activeTab === 'backup' && !editing}
			<BackupSection />
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
	:global(.json-viewer .json-key) { color: var(--ctp-primary); }
	:global(.json-viewer .json-string) { color: #a6e3a1; }
	:global(.json-viewer .json-number) { color: #f9e2af; }
	:global(.json-viewer .json-boolean) { color: #89b4fa; }
	:global(.json-viewer .json-null) { color: var(--ctp-overlay0); }
	:global(:root.light .json-viewer .json-string) { color: #40a02b; }
	:global(:root.light .json-viewer .json-number) { color: #df8e1d; }
	:global(:root.light .json-viewer .json-boolean) { color: #1e66f5; }
	.editor-container :global(.cm-editor) { height: 100%; }
</style>

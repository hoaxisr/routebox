<script lang="ts">
	import { t } from 'svelte-i18n';
	import { onMount, onDestroy } from 'svelte';
	import { EditorView, basicSetup } from 'codemirror';
	import { EditorState, Compartment } from '@codemirror/state';
	import { json } from '@codemirror/lang-json';
	import { oneDark } from '@codemirror/theme-one-dark';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges, configReadOnly } from '$lib/stores';
	import type { SingboxConfig } from '$lib/types';

	let editorContainer: HTMLDivElement;
	let editorView: EditorView | null = null;
	let loading = $state(true);
	let saving = $state(false);
	let validating = $state(false);
	let validationErrors = $state<string[]>([]);
	let originalContent = '';

	const themeCompartment = new Compartment();

	async function loadConfig() {
		loading = true;
		try {
			const config = await api.getConfig();
			originalContent = JSON.stringify(config, null, 2);

			if (editorView) {
				editorView.dispatch({
					changes: {
						from: 0,
						to: editorView.state.doc.length,
						insert: originalContent
					}
				});
			}
		} catch (err) {
			notifications.error(`Failed to load config: ${err}`);
		} finally {
			loading = false;
		}
	}

	async function handleSave() {
		if (!editorView) return;

		const content = editorView.state.doc.toString();

		// Validate JSON syntax
		let config: SingboxConfig;
		try {
			config = JSON.parse(content);
		} catch (err) {
			notifications.error(`Invalid JSON: ${err}`);
			return;
		}

		saving = true;
		try {
			await api.saveConfig(config);
			notifications.success($t('jsonEditor.saveToDraft'));
			unsavedChanges.markChanged('Config', 'Updated via JSON editor');
			unsavedChanges.refresh();
			originalContent = content;
		} catch (err) {
			notifications.error(`Failed to save: ${err}`);
		} finally {
			saving = false;
		}
	}

	async function handleValidate() {
		if (!editorView) return;

		const content = editorView.state.doc.toString();

		// Validate JSON syntax first
		let config: SingboxConfig;
		try {
			config = JSON.parse(content);
		} catch (err) {
			validationErrors = [`JSON syntax error: ${err}`];
			return;
		}

		validating = true;
		try {
			const result = await api.validateConfig(config);
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

	onMount(() => {
		// Create CodeMirror editor
		const state = EditorState.create({
			doc: '',
			extensions: [
				basicSetup,
				json(),
				themeCompartment.of(oneDark),
				EditorView.lineWrapping,
				EditorView.theme({
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

		editorView = new EditorView({
			state,
			parent: editorContainer
		});

		loadConfig();

		return () => {
			editorView?.destroy();
		};
	});
</script>

<div class="h-full flex flex-col bg-[var(--ctp-base)]">
	<!-- Toolbar -->
	<div class="flex items-center justify-between p-3 bg-[var(--ctp-surface0)] border-b border-[var(--ctp-surface1)]">
		<div class="flex items-center gap-2">
			<button
				type="button"
				onclick={handleFormat}
				class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
			>
				{$t('common.format')}
			</button>
			<button
				type="button"
				onclick={handleValidate}
				disabled={validating}
				class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors disabled:opacity-50"
			>
				{validating ? $t('common.validate') + '...' : $t('common.validate')}
			</button>
			<button
				type="button"
				onclick={handleReset}
				class="px-3 py-1.5 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
			>
				{$t('common.reset')}
			</button>
		</div>

		<div class="flex items-center gap-2">
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

	<!-- Editor -->
	<div class="flex-1 relative">
		{#if loading}
			<div class="absolute inset-0 flex items-center justify-center bg-[var(--ctp-mantle)]">
				<span class="text-[var(--ctp-overlay1)]">{$t('jsonEditor.loadingConfig')}</span>
			</div>
		{/if}
		<div bind:this={editorContainer} class="h-full"></div>
	</div>
</div>

<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { SingboxConfig } from '$lib/types';

	let config: SingboxConfig | null = $state(null);
	let loading = $state(true);
	let activeTab = $state<'overview' | 'json' | 'diff'>('overview');

	// For diff view
	let savedConfig: string = $state('');
	let currentConfig: string = $state('');
	let hasChanges = $derived(savedConfig !== currentConfig && savedConfig !== '');

	async function fetchConfig() {
		loading = true;
		try {
			config = await api.getConfig();
			const jsonStr = JSON.stringify(config, null, 2);
			savedConfig = jsonStr;
			currentConfig = jsonStr;
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

	// Generate diff between saved and current
	function generateDiff(): { type: 'same' | 'added' | 'removed'; line: string; num?: number }[] {
		const savedLines = savedConfig.split('\n');
		const currentLines = currentConfig.split('\n');
		const result: { type: 'same' | 'added' | 'removed'; line: string; num?: number }[] = [];

		// Simple line-by-line diff (not optimal but works for JSON)
		const maxLen = Math.max(savedLines.length, currentLines.length);
		let lineNum = 1;

		for (let i = 0; i < maxLen; i++) {
			const saved = savedLines[i];
			const current = currentLines[i];

			if (saved === current) {
				if (saved !== undefined) {
					result.push({ type: 'same', line: saved, num: lineNum++ });
				}
			} else {
				if (saved !== undefined && current === undefined) {
					result.push({ type: 'removed', line: saved });
				} else if (saved === undefined && current !== undefined) {
					result.push({ type: 'added', line: current, num: lineNum++ });
				} else if (saved !== current) {
					result.push({ type: 'removed', line: saved });
					result.push({ type: 'added', line: current, num: lineNum++ });
				}
			}
		}

		return result;
	}

	onMount(fetchConfig);
</script>

<svelte:head>
	<title>Configuration - RouteBox</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-[var(--ctp-text)]">Configuration</h1>
		<div class="flex gap-2">
			{#if activeTab === 'json'}
				<button
					onclick={copyToClipboard}
					class="px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
					</svg>
					Copy
				</button>
				<button
					onclick={downloadConfig}
					class="px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-1.5"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
					</svg>
					Download
				</button>
			{/if}
			<button
				onclick={fetchConfig}
				class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm flex items-center gap-1.5"
			>
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
				</svg>
				Refresh
			</button>
		</div>
	</div>

	<!-- Tabs -->
	<div class="flex gap-1 p-1 bg-[var(--ctp-surface0)] rounded-lg w-fit">
		<button
			onclick={() => activeTab = 'overview'}
			class="px-4 py-1.5 rounded-md text-sm transition-colors {activeTab === 'overview' ? 'bg-[var(--ctp-surface1)] text-[var(--ctp-text)]' : 'text-[var(--ctp-overlay1)] hover:text-[var(--ctp-text)]'}"
		>
			Overview
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
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<svg class="w-8 h-8 text-[var(--ctp-primary)] animate-spin" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	{:else if !config}
		<div class="text-center py-12 text-[var(--ctp-red)]">
			Failed to load configuration
		</div>
	{:else}
		<!-- Overview Tab -->
		{#if activeTab === 'overview'}
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
				<a href="/config/endpoints" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">Endpoints</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.endpoints?.length ?? 0}
					</div>
				</a>
				<a href="/config/outbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">Outbounds</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.outbounds?.length ?? 0}
					</div>
				</a>
				<a href="/config/inbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">Inbounds</div>
					<div class="text-2xl font-bold text-[var(--ctp-text)]">
						{config.inbounds?.length ?? 0}
					</div>
				</a>
				<a href="/config/routes" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
					<div class="text-sm text-[var(--ctp-overlay1)]">Route Rules</div>
					<div class="text-2xl font-bold text-[var(--ctp-primary)]">
						{config.route?.rules?.length ?? 0}
					</div>
				</a>
			</div>

			<!-- Quick stats -->
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
					<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">DNS Configuration</h3>
					<div class="space-y-2 text-sm">
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">Servers</span>
							<span class="text-[var(--ctp-text)]">{config.dns?.servers?.length ?? 0}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">Rules</span>
							<span class="text-[var(--ctp-text)]">{config.dns?.rules?.length ?? 0}</span>
						</div>
					</div>
				</div>
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
					<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">Route Configuration</h3>
					<div class="space-y-2 text-sm">
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">Rule Sets</span>
							<span class="text-[var(--ctp-text)]">{config.route?.rule_set?.length ?? 0}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-[var(--ctp-subtext1)]">Final Outbound</span>
							<span class="text-[var(--ctp-text)]">{config.route?.final ?? 'direct'}</span>
						</div>
					</div>
				</div>
			</div>
		{/if}

		<!-- JSON Tab -->
		{#if activeTab === 'json'}
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

		<!-- Diff Tab -->
		{#if activeTab === 'diff'}
			{#if !hasChanges}
				<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center">
					<svg class="w-12 h-12 mx-auto text-[var(--ctp-overlay0)] mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					<p class="text-[var(--ctp-subtext1)]">No changes detected</p>
					<p class="text-sm text-[var(--ctp-overlay0)] mt-1">
						The configuration matches the saved version
					</p>
				</div>
			{:else}
				<div class="bg-[var(--ctp-surface0)] rounded-xl overflow-hidden">
					<div class="px-4 py-2 bg-[var(--ctp-surface1)] border-b border-[var(--ctp-surface2)] flex items-center justify-between">
						<span class="text-sm font-medium text-[var(--ctp-subtext1)]">Changes</span>
						<div class="flex items-center gap-3 text-xs">
							<span class="flex items-center gap-1">
								<span class="w-3 h-3 rounded bg-[#3d9970]/20 border border-[#3d9970]"></span>
								Added
							</span>
							<span class="flex items-center gap-1">
								<span class="w-3 h-3 rounded bg-[#d9534f]/20 border border-[#d9534f]"></span>
								Removed
							</span>
						</div>
					</div>
					<div class="p-4 overflow-auto max-h-[calc(100vh-280px)] font-mono text-sm">
						{#each generateDiff() as line}
							<div class="flex {line.type === 'added' ? 'bg-[#3d9970]/10' : line.type === 'removed' ? 'bg-[#d9534f]/10' : ''}">
								<span class="w-10 flex-shrink-0 text-right pr-3 text-[var(--ctp-overlay0)] select-none border-r border-[var(--ctp-surface2)] mr-3">
									{line.num ?? ''}
								</span>
								<span class="w-4 flex-shrink-0 {line.type === 'added' ? 'text-[#3d9970]' : line.type === 'removed' ? 'text-[#d9534f]' : 'text-[var(--ctp-overlay0)]'}">
									{line.type === 'added' ? '+' : line.type === 'removed' ? '-' : ' '}
								</span>
								<span class="flex-1 whitespace-pre {line.type === 'added' ? 'text-[#3d9970]' : line.type === 'removed' ? 'text-[#d9534f]' : 'text-[var(--ctp-text)]'}">
									{line.line}
								</span>
							</div>
						{/each}
					</div>
				</div>
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
</style>

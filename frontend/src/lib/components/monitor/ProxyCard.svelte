<script lang="ts">
	import type { ClashProxy } from '$lib/types';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';

	interface Props {
		proxy: ClashProxy;
		onUpdate?: () => void;
	}

	let { proxy, onUpdate }: Props = $props();

	let testing = $state(false);
	let delay = $state<number | null>(proxy.history?.[0]?.delay ?? null);

	function getDelayColor(ms: number | null): string {
		if (ms === null) return 'text-[var(--ctp-overlay0)]';
		if (ms === 0) return 'text-[var(--ctp-red)]'; // timeout/error
		if (ms < 300) return 'text-[var(--ctp-text)]';
		return 'text-[var(--ctp-overlay1)]'; // slow but working
	}

	function formatDelay(ms: number | null): string {
		if (ms === null) return '-';
		if (ms === 0) return 'timeout';
		return `${ms}ms`;
	}

	async function testLatency() {
		testing = true;
		try {
			const result = await api.testLatency(proxy.name);
			delay = result.delay;
		} catch (err) {
			delay = 0; // timeout
			notifications.error(`Test failed: ${err}`);
		} finally {
			testing = false;
		}
	}

	async function switchTo(target: string) {
		try {
			await api.switchProxy(proxy.name, target);
			notifications.success(`Switched to ${target}`);
			onUpdate?.();
		} catch (err) {
			notifications.error(`Failed to switch: ${err}`);
		}
	}

	const isSelector = $derived(proxy.type === 'Selector' || proxy.type === 'URLTest');
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
	<div class="flex items-center justify-between mb-2">
		<div>
			<h3 class="font-medium text-[var(--ctp-text)]">{proxy.name}</h3>
			<span class="text-xs px-2 py-0.5 rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)]">
				{proxy.type}
			</span>
		</div>
		<div class="flex items-center gap-2">
			<span class="text-lg font-mono {getDelayColor(delay)}">
				{formatDelay(delay)}
			</span>
			<button
				onclick={testLatency}
				disabled={testing}
				class="p-2 rounded-lg hover:bg-[var(--ctp-surface1)] transition-colors disabled:opacity-50"
				title="Test latency"
			>
				{#if testing}
					<svg class="w-4 h-4 animate-spin text-[var(--ctp-primary)]" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
				{:else}
					<svg class="w-4 h-4 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
				{/if}
			</button>
		</div>
	</div>

	{#if isSelector && proxy.all}
		<div class="mt-3 pt-3 border-t border-[var(--ctp-surface2)]">
			<div class="text-xs text-[var(--ctp-overlay0)] mb-2">
				Current: <span class="text-[var(--ctp-primary)]">{proxy.now || 'None'}</span>
			</div>
			<div class="flex flex-wrap gap-1">
				{#each proxy.all as option}
					<button
						onclick={() => switchTo(option)}
						class="px-2 py-1 text-xs rounded transition-colors {proxy.now === option
							? 'bg-[var(--ctp-primary)] text-white'
							: 'bg-[var(--ctp-surface1)] text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface2)]'}"
					>
						{option}
					</button>
				{/each}
			</div>
		</div>
	{/if}
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ClashProxy } from '$lib/types';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';

	interface Props {
		proxy: ClashProxy;
		allProxies?: Record<string, ClashProxy>;
		onUpdate?: () => void;
	}

	let { proxy, allProxies, onUpdate }: Props = $props();

	let testing = $state(false);
	let delay = $state<number | null>(proxy.history?.[proxy.history.length - 1]?.delay ?? null);

	const isGroup = $derived(proxy.type === 'Selector' || proxy.type === 'URLTest');

	const effectiveDelay = $derived.by((): number | null => {
		if (isGroup && proxy.now && allProxies) {
			const activeProxy = allProxies[proxy.now];
			const lastHistory = activeProxy?.history?.[activeProxy.history.length - 1];
			return lastHistory?.delay ?? null;
		}
		return delay;
	});

	function getStatusBg(ms: number | null): string {
		if (ms === 0) return 'color-mix(in srgb, var(--ctp-red) 15%, transparent)';
		return 'color-mix(in srgb, var(--ctp-green) 15%, transparent)';
	}

	function getStatusStroke(ms: number | null): string {
		if (ms === 0) return 'var(--ctp-red)';
		return 'var(--ctp-green)';
	}

	function getDelayColor(ms: number | null): string {
		if (ms === null) return 'text-[var(--ctp-overlay0)]';
		if (ms === 0) return 'text-[var(--ctp-red)]'; // timeout/error
		if (ms < 300) return 'text-[var(--ctp-text)]';
		return 'text-[var(--ctp-overlay1)]'; // slow but working
	}

	function formatDelay(ms: number | null): string {
		if (ms === null) return '-';
		if (ms === 0) return $t('proxies.timeout');
		return `${ms}ms`;
	}

	async function testLatency() {
		testing = true;
		try {
			const result = await api.testLatency(proxy.name);
			delay = result.delay;
		} catch (err) {
			delay = 0; // timeout
			notifications.error(`${$t('common.test')} ${$t('common.error').toLowerCase()}: ${err}`);
		} finally {
			testing = false;
		}
	}

	async function switchTo(target: string) {
		try {
			await api.switchProxy(proxy.name, target);
			notifications.success(`${$t('proxies.selected')}: ${target}`);
			onUpdate?.();
		} catch (err) {
			notifications.error(`${$t('errors.saveFailed')}: ${err}`);
		}
	}

</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 border border-[var(--ctp-surface2)]">
	<div class="flex items-center justify-between mb-2">
		<div class="flex items-center gap-3">
			<div class="icon-badge" style="background-color: {getStatusBg(effectiveDelay)}">
				<svg class="w-5 h-5" fill="none" stroke={getStatusStroke(effectiveDelay)} viewBox="0 0 24 24">
					{#if isGroup}
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
					{:else}
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
					{/if}
				</svg>
			</div>
			<div>
				<h3 class="font-medium text-[var(--ctp-text)]">{proxy.name}</h3>
				<span class="text-xs px-2 py-0.5 rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)]">
					{proxy.type}
				</span>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<span class="text-lg font-mono {getDelayColor(delay)}">
				{formatDelay(delay)}
			</span>
			<button
				onclick={testLatency}
				disabled={testing}
				class="p-2 rounded-lg hover:bg-[var(--ctp-surface1)] transition-colors disabled:opacity-50"
				title={$t('proxies.testLatency')}
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

	{#if isGroup && proxy.all}
		<div class="mt-3 pt-3 border-t border-[var(--ctp-surface2)]">
			<div class="text-xs text-[var(--ctp-overlay0)] mb-2">
				{$t('proxies.selected')}: <span class="text-[var(--ctp-primary)]">{proxy.now || $t('common.none')}</span>
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

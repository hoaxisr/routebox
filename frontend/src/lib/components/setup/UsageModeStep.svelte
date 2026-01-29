<script lang="ts">
	import { t } from 'svelte-i18n';
	import SelectableCard from '$lib/components/shared/SelectableCard.svelte';

	interface Props {
		stepNumber: number;
		usageMode: 'router' | 'proxy';
		proxyPort: number;
		machineIP: string;
		onModeChange: (mode: 'router' | 'proxy') => void;
		onPortChange: (port: number) => void;
	}

	let { stepNumber, usageMode = $bindable(), proxyPort = $bindable(), machineIP, onModeChange, onPortChange }: Props = $props();
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{stepNumber}. {$t('setup.mode.title')}</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.mode.description')}
		</p>
	</div>

	<div class="space-y-3">
		<SelectableCard
			selected={usageMode === 'router'}
			variant="radio"
			size="lg"
			onclick={() => { usageMode = 'router'; onModeChange('router'); }}
		>
			<div class="font-semibold text-[var(--ctp-text)]">{$t('setup.mode.router.title')}</div>
			<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{$t('setup.mode.router.description')}
			</div>
			<div class="mt-2 flex flex-wrap gap-2">
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.router.tunInterface')}</span>
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.router.splitTunneling')}</span>
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.router.ruleSets')}</span>
			</div>
		</SelectableCard>

		<SelectableCard
			selected={usageMode === 'proxy'}
			variant="radio"
			size="lg"
			onclick={() => { usageMode = 'proxy'; onModeChange('proxy'); }}
		>
			<div class="font-semibold text-[var(--ctp-text)]">{$t('setup.mode.proxy.title')}</div>
			<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{$t('setup.mode.proxy.description')}
			</div>
			<div class="mt-2 flex flex-wrap gap-2">
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.proxy.socks5')}</span>
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.proxy.httpProxy')}</span>
				<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">{$t('setup.mode.proxy.simpleSetup')}</span>
			</div>
		</SelectableCard>
	</div>

	{#if usageMode === 'proxy'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
			<label for="proxy-port" class="block text-sm text-[var(--ctp-overlay1)] mb-2">{$t('setup.mode.proxyPort')}</label>
			<input
				id="proxy-port"
				type="number"
				bind:value={proxyPort}
				oninput={() => onPortChange(proxyPort)}
				min="1024"
				max="65535"
				class="w-32 px-3 py-2 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
			<p class="text-xs text-[var(--ctp-overlay0)] mt-2">
				{$t('setup.mode.proxyAvailableAt')} <code class="bg-[var(--ctp-surface1)] px-1.5 py-0.5 rounded text-[var(--ctp-text)]">{machineIP}:{proxyPort}</code>
			</p>
		</div>
	{/if}
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import SelectableCard from '$lib/components/shared/SelectableCard.svelte';

	interface Props {
		stepNumber: number;
		routingMode: 'split' | 'all';
		onModeChange: (mode: 'split' | 'all') => void;
	}

	let { stepNumber, routingMode = $bindable(), onModeChange }: Props = $props();
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{stepNumber}. {$t('setup.routing.title')}</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.routing.description')}
		</p>
	</div>

	<div class="space-y-3">
		<SelectableCard
			selected={routingMode === 'split'}
			variant="radio"
			size="lg"
			onclick={() => { routingMode = 'split'; onModeChange('split'); }}
		>
			<div class="font-semibold text-[var(--ctp-text)]">{$t('setup.routing.split.title')}</div>
			<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{$t('setup.routing.split.description')}
			</div>
		</SelectableCard>

		<SelectableCard
			selected={routingMode === 'all'}
			variant="radio"
			size="lg"
			onclick={() => { routingMode = 'all'; onModeChange('all'); }}
		>
			<div class="font-semibold text-[var(--ctp-text)]">{$t('setup.routing.all.title')}</div>
			<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
				{$t('setup.routing.all.description')}
			</div>
		</SelectableCard>
	</div>
</div>

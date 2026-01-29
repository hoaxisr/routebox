<script lang="ts">
	import { t } from 'svelte-i18n';
	import SelectableCard from '$lib/components/shared/SelectableCard.svelte';

	interface RuleSetOption {
		id: string;
		name: string;
		desc: string;
		url: string;
	}

	interface Props {
		stepNumber: number;
		availableRuleSets: RuleSetOption[];
		selectedRuleSets: string[];
		onToggle: (id: string) => void;
	}

	let { stepNumber, availableRuleSets, selectedRuleSets = $bindable(), onToggle }: Props = $props();
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{stepNumber}. {$t('setup.ruleSets.title')}</h2>
		<p class="text-[var(--ctp-overlay1)] mt-1">
			{$t('setup.ruleSets.description')}
		</p>
	</div>

	<div class="space-y-3">
		{#each availableRuleSets as rs}
			<SelectableCard
				selected={selectedRuleSets.includes(rs.id)}
				variant="checkbox"
				onclick={() => onToggle(rs.id)}
			>
				<div class="font-medium text-[var(--ctp-text)]">{rs.name}</div>
				<div class="text-sm text-[var(--ctp-overlay1)]">{rs.desc}</div>
			</SelectableCard>
		{/each}
	</div>
</div>

<script lang="ts">
	interface Props {
		currentStep: number;
		totalSteps: number;
	}

	let { currentStep, totalSteps }: Props = $props();
</script>

<div class="flex items-center justify-center">
	{#each Array(totalSteps) as _, i}
		<div class="flex items-center">
			<div
				class="w-10 h-10 rounded-full flex items-center justify-center font-medium transition-colors"
				class:bg-[var(--ctp-primary)]={currentStep >= i}
				class:text-white={currentStep >= i}
				class:bg-[var(--ctp-surface1)]={currentStep < i}
				class:text-[var(--ctp-overlay0)]={currentStep < i}
			>
				{#if currentStep > i}
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
					</svg>
				{:else}
					{i + 1}
				{/if}
			</div>
			{#if i < totalSteps - 1}
				<div
					class="w-16 h-1 mx-2 rounded transition-colors"
					class:bg-[var(--ctp-primary)]={currentStep > i}
					class:bg-[var(--ctp-surface1)]={currentStep <= i}
				></div>
			{/if}
		</div>
	{/each}
</div>

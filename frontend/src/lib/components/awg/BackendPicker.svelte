<script lang="ts">
	import { t } from 'svelte-i18n';
	interface Props {
		value: 'kernel' | 'singbox';
		disabled?: boolean;
		onChange: (b: 'kernel' | 'singbox') => void;
	}
	let { value, disabled = false, onChange }: Props = $props();
	const options: Array<{ id: 'kernel' | 'singbox'; label: string; hint: string }> = [
		{ id: 'kernel', label: $t('awg.backendKernel'), hint: $t('awg.backendKernelHint') },
		{ id: 'singbox', label: $t('awg.backendSingbox'), hint: $t('awg.backendSingboxHint') }
	];
</script>

<div class="backend-picker">
	{#each options as o}
		<button
			type="button"
			class="type-btn"
			class:selected={value === o.id}
			{disabled}
			onclick={() => onChange(o.id)}
		>
			<span class="bp-label">{o.label}</span>
			<span class="bp-hint">{o.hint}</span>
		</button>
	{/each}
</div>

<style>
	.backend-picker { display: flex; gap: 0.6rem; margin-bottom: 1rem; }
	.type-btn { flex: 1; text-align: left; display: flex; flex-direction: column; gap: 2px; }
	.bp-label { font-weight: 600; }
	.bp-hint { font-size: 0.75rem; color: var(--ctp-overlay1); }
</style>

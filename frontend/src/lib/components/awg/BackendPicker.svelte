<script lang="ts">
	import { t } from 'svelte-i18n';
	interface Props {
		value: 'kernel' | 'singbox';
		disabled?: boolean;
		/** False hides the kernel option — pass the host's own capability, never
		 *  the install mode: what decides is whether the tools are there or
		 *  RouteBox can install them (#93), not router vs vps. */
		allowKernel?: boolean;
		/** Why the kernel backend is unavailable here, shown in its place. */
		kernelReason?: string;
		onChange: (b: 'kernel' | 'singbox') => void;
	}
	let { value, disabled = false, allowKernel = true, kernelReason = '', onChange }: Props = $props();
	const allOptions: Array<{ id: 'kernel' | 'singbox'; label: string; hint: string }> = [
		{ id: 'kernel', label: $t('awg.backendKernel'), hint: $t('awg.backendKernelHint') },
		{ id: 'singbox', label: $t('awg.backendSingbox'), hint: $t('awg.backendSingboxHint') }
	];
	const options = $derived(allowKernel ? allOptions : allOptions.filter((o) => o.id !== 'kernel'));
</script>

{#if !allowKernel && kernelReason}
	<p class="bp-reason">{$t('awg.backendKernelUnavailable')}: {kernelReason}</p>
{/if}

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
	.bp-reason { font-size: 0.8rem; color: var(--ctp-overlay1); margin-bottom: 0.5rem; }
</style>

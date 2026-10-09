<script lang="ts">
	import { t } from 'svelte-i18n';
	import { onMount } from 'svelte';
	import type { ApplyPhase } from '$lib/utils/applyTracker';

	// Shown while an apply runs: what the backend is doing now plus a running
	// clock, so a reload that takes the VPN down with it doesn't look frozen.
	let { phase }: { phase: ApplyPhase | null } = $props();

	let secs = $state(0);
	onMount(() => {
		const started = Date.now();
		const id = setInterval(() => (secs = Math.floor((Date.now() - started) / 1000)), 1000);
		return () => clearInterval(id);
	});

	const keys: Record<ApplyPhase, string> = {
		checking: 'changes.phaseChecking',
		reloading: 'changes.phaseReloading',
		reconnecting: 'changes.phaseReconnecting'
	};
</script>

{phase ? $t(keys[phase]) : $t('common.saving')}
<span class="tabular-nums opacity-70">{$t('changes.phaseSeconds', { values: { n: secs } })}</span>

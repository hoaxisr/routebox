<script lang="ts">
	import { t } from 'svelte-i18n';
	import { isEnableInFlight } from '$lib/utils/awgPhase';

	interface Props {
		/** status.phase — the orchestrator's step. */
		phase: string;
		/** status.module — the kernel module's install state. */
		module?: string;
		/** True while this page's own Enable request is still open. */
		enabling?: boolean;
	}

	let { phase, module = '', enabling = false }: Props = $props();

	// Driven by the phase, not only by our own request: on the kernel backend the
	// install can outlive the browser's patience for a single POST, and after that
	// the phase is the only thing still saying the work is going on.
	const active = $derived(enabling || isEnableInFlight(phase));

	const label = $derived(
		{
			validating: $t('awg.phaseValidating'),
			installing: $t('awg.phaseInstalling'),
			rendering: $t('awg.phaseRendering'),
			starting: $t('awg.phaseStarting'),
			'health-check': $t('awg.phaseHealthCheck')
		}[phase] ?? $t('awg.enabling')
	);
</script>

{#if active}
	<div class="enable-progress">
		<svg class="spin" width="14" height="14" viewBox="0 0 24 24" fill="none">
			<circle class="track" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
			<path fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
		</svg>
		<span>{label}</span>
		{#if phase === 'installing'}
			{#if module}<span class="status-badge info">{module}</span>{/if}
			<span class="hint">{$t('awg.phaseInstallingHint')}</span>
		{/if}
	</div>
{/if}

<style>
	.enable-progress {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-top: 0.6rem;
		font-size: 0.85rem;
		color: var(--ctp-subtext1);
	}
	.hint { font-size: 0.75rem; color: var(--ctp-overlay1); }
	.spin { color: var(--ctp-primary); animation: spin 1s linear infinite; }
	.track { opacity: 0.25; }
	@keyframes spin { to { transform: rotate(360deg); } }
</style>

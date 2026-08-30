<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { SpeedTestResult } from '$lib/types';

	interface Props {
		speed: SpeedTestResult;
	}

	let { speed }: Props = $props();

	// Decimal, the way the tool measures and the way links are sold.
	function formatBits(bps: number): string {
		if (bps >= 1e9) return `${(bps / 1e9).toFixed(2)} Gbps`;
		if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
		if (bps >= 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
		return `${bps} bps`;
	}
</script>

<!-- Accuracy comes from the binary, and a Low-accuracy figure is shown as such
     rather than dressed up as a measurement. -->
<div class="mt-3 pt-3 border-t border-[var(--ctp-surface2)] flex items-center gap-4 flex-wrap text-sm">
	<span class="text-[var(--ctp-overlay0)] text-xs uppercase tracking-wide">{$t('outbounds.speedTest')}</span>
	<span class="text-[var(--ctp-text)]" title="{$t('outbounds.speedAccuracy')}: {speed.download_accuracy}">
		↓ <span class="font-mono">{formatBits(speed.download_bps)}</span>
	</span>
	<span class="text-[var(--ctp-text)]" title="{$t('outbounds.speedAccuracy')}: {speed.upload_accuracy}">
		↑ <span class="font-mono">{formatBits(speed.upload_bps)}</span>
	</span>
	<span class="text-[var(--ctp-subtext1)]" title={$t('outbounds.speedLatencyHint')}>
		{$t('outbounds.speedLatency')} <span class="font-mono">{speed.idle_latency_ms} ms</span>
	</span>
	<span class="text-[var(--ctp-subtext1)]" title={$t('outbounds.speedRpmHint')}>
		{$t('outbounds.speedRpm')}
		<span class="font-mono">↓{speed.download_rpm} / ↑{speed.upload_rpm}</span>
		{$t('outbounds.speedRpmUnit')}
	</span>
</div>

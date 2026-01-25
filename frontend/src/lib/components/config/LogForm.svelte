<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { LogSettings } from '$lib/types';

	interface Props {
		settings: LogSettings;
		onChange: (settings: LogSettings) => void;
	}

	let { settings, onChange }: Props = $props();

	// Local state
	let level = $state(settings.level ?? 'info');
	let timestamp = $state(settings.timestamp ?? false);
	let output = $state(settings.output ?? '');

	const levels: LogSettings['level'][] = ['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'];

	function handleChange() {
		onChange({
			level,
			timestamp: timestamp || undefined,
			output: output.trim() || undefined
		});
	}
</script>

<div class="space-y-4">
	<!-- Log Level -->
	<div>
		<label for="log-level" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('log.level')}
		</label>
		<select
			id="log-level"
			bind:value={level}
			onchange={handleChange}
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		>
			{#each levels as lvl}
				<option value={lvl}>{$t(`log.levels.${lvl}`)}</option>
			{/each}
		</select>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{$t('log.levels.trace')} = most verbose, {$t('log.levels.panic')} = least verbose
		</p>
	</div>

	<!-- Timestamp -->
	<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
		<input
			type="checkbox"
			bind:checked={timestamp}
			onchange={handleChange}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
		/>
		{$t('log.timestamp')}
	</label>

	<!-- Output Path -->
	<div>
		<label for="log-output" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('log.output')}
			<span class="font-normal text-[var(--ctp-overlay0)]">({$t('common.optional')})</span>
		</label>
		<input
			id="log-output"
			type="text"
			bind:value={output}
			onchange={handleChange}
			placeholder="/var/log/sing-box.log"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		/>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{$t('common.empty')} = stdout
		</p>
	</div>
</div>

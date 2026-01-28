<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		server: string;
		serverPort: number;
		errors?: Record<string, string>;
	}

	let { server = $bindable(), serverPort = $bindable(), errors = {} }: Props = $props();
</script>

<div class="grid grid-cols-3 gap-4">
	<!-- Server -->
	<div class="col-span-2">
		<label for="server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('common.server')} *
		</label>
		<input
			id="server"
			type="text"
			bind:value={server}
			placeholder="example.com"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['server']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['server']}</p>
		{/if}
	</div>

	<!-- Port -->
	<div>
		<label for="serverPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('common.port')} *
		</label>
		<input
			id="serverPort"
			type="number"
			bind:value={serverPort}
			min="1"
			max="65535"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['serverPort']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['serverPort']}</p>
		{/if}
	</div>
</div>

<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		type: 'selector' | 'urltest';
		availableOutbounds: string[];
		selectedOutbounds: string[];
		defaultOutbound: string;
		interruptExistConnections: boolean;
		// URLTest specific
		testUrl?: string;
		testInterval?: string;
		tolerance?: number;
		idleTimeout?: string;
		errors?: Record<string, string>;
	}

	let {
		type,
		availableOutbounds,
		selectedOutbounds = $bindable(),
		defaultOutbound = $bindable(),
		interruptExistConnections = $bindable(),
		testUrl = $bindable(''),
		testInterval = $bindable('3m'),
		tolerance = $bindable(150),
		idleTimeout = $bindable(''),
		errors = {}
	}: Props = $props();

	function toggleOutbound(tag: string) {
		if (selectedOutbounds.includes(tag)) {
			selectedOutbounds = selectedOutbounds.filter((o) => o !== tag);
			if (defaultOutbound === tag) {
				defaultOutbound = selectedOutbounds[0] ?? '';
			}
		} else {
			selectedOutbounds = [...selectedOutbounds, tag];
			if (!defaultOutbound) {
				defaultOutbound = tag;
			}
		}
	}
</script>

<div class="space-y-4">
	<!-- Selected Outbounds -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
			{$t('outbounds.selectedOutbounds')} *
			<span class="font-normal text-[var(--ctp-overlay0)]">({selectedOutbounds.length} {$t('proxies.selected').toLowerCase()})</span>
		</label>
		{#if errors['outbounds']}
			<p class="mb-2 text-sm text-[var(--ctp-red)]">{errors['outbounds']}</p>
		{/if}
		<div class="max-h-48 overflow-y-auto bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)]">
			{#if availableOutbounds.length === 0}
				<div class="p-4 text-center text-[var(--ctp-overlay0)]">
					{$t('outbounds.noOutboundsAvailable')}
				</div>
			{:else}
				{#each availableOutbounds as tag}
					<button
						type="button"
						onclick={() => toggleOutbound(tag)}
						class="w-full px-4 py-2 flex items-center justify-between hover:bg-[var(--ctp-surface1)] transition-colors {selectedOutbounds.includes(tag) ? 'bg-[var(--ctp-surface1)]' : ''}"
					>
						<span class="text-[var(--ctp-text)]">{tag}</span>
						<span class="flex items-center gap-2">
							{#if type === 'selector' && defaultOutbound === tag}
								<span class="status-badge">{$t('common.default').toLowerCase()}</span>
							{/if}
							{#if selectedOutbounds.includes(tag)}
								<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							{/if}
						</span>
					</button>
				{/each}
			{/if}
		</div>
	</div>

	{#if selectedOutbounds.length > 0}
		{#if type === 'selector'}
			<!-- Default Outbound (selector only) -->
			<div>
				<label for="default" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.defaultOutbound')}
				</label>
				<select
					id="default"
					bind:value={defaultOutbound}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					{#each selectedOutbounds as tag}
						<option value={tag}>{tag}</option>
					{/each}
				</select>
			</div>
		{/if}

		<!-- Interrupt Existing -->
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={interruptExistConnections}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('outbounds.interruptExisting')}
		</label>
	{/if}

	<!-- URLTest specific settings -->
	{#if type === 'urltest'}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('outbounds.urlTestSettings')}</h3>
			<div>
				<label for="urltestUrl" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.testUrl')}</label>
				<input
					id="urltestUrl"
					type="url"
					bind:value={testUrl}
					placeholder="https://www.gstatic.com/generate_204"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
				/>
			</div>
			<div class="grid grid-cols-3 gap-4">
				<div>
					<label for="urltestInterval" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.testInterval')}</label>
					<input
						id="urltestInterval"
						type="text"
						bind:value={testInterval}
						placeholder="3m"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div>
					<label for="urltestTolerance" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.tolerance')} (ms)</label>
					<input
						id="urltestTolerance"
						type="number"
						bind:value={tolerance}
						min="0"
						placeholder="150"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
				<div>
					<label for="urltestIdleTimeout" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('outbounds.idleTimeout')}</label>
					<input
						id="urltestIdleTimeout"
						type="text"
						bind:value={idleTimeout}
						placeholder="30m"
						class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
					/>
				</div>
			</div>
		</div>
	{/if}
</div>

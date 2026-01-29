<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import type { ConnectTestResponse } from '$lib/types';

	let host = $state('');
	let port = $state(443);
	let timeout = $state(5);
	let testing = $state(false);
	let result = $state<ConnectTestResponse | null>(null);

	const presets = [
		{ label: 'Google', host: 'google.com', port: 443 },
		{ label: 'Cloudflare', host: '1.1.1.1', port: 443 },
		{ label: 'Telegram', host: 'telegram.org', port: 443 },
		{ label: 'YouTube', host: 'youtube.com', port: 443 },
		{ label: 'GitHub', host: 'github.com', port: 443 }
	];

	// Country code to flag emoji
	function getFlag(countryCode: string): string {
		if (!countryCode || countryCode.length !== 2) return '';
		const codePoints = countryCode
			.toUpperCase()
			.split('')
			.map((char) => 127397 + char.charCodeAt(0));
		return String.fromCodePoint(...codePoints);
	}

	async function runTest() {
		if (!host.trim()) return;
		testing = true;
		result = null;
		try {
			result = await api.connectionTest(host.trim(), port, timeout);
		} catch (e) {
			result = {
				success: false,
				error: `${e}`
			};
		} finally {
			testing = false;
		}
	}

	function quickTest(preset: (typeof presets)[0]) {
		host = preset.host;
		port = preset.port;
		runTest();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !testing) {
			runTest();
		}
	}
</script>

<div class="space-y-6">
	<!-- Input Section -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-6">
		<div class="space-y-4">
			<!-- Host input -->
			<div>
				<label for="test-host" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
					{$t('routes.inspector.inputLabel')}
				</label>
				<div class="flex gap-3">
					<input
						id="test-host"
						type="text"
						bind:value={host}
						onkeydown={handleKeydown}
						placeholder={$t('routes.inspector.inputPlaceholder')}
						class="flex-1 px-4 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder:text-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
					/>
					<input
						type="number"
						bind:value={port}
						min="1"
						max="65535"
						class="w-24 px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						title={$t('common.port')}
					/>
					<button
						onclick={runTest}
						disabled={testing || !host.trim()}
						class="px-6 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:bg-[var(--ctp-primary-hover)] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
					>
						{#if testing}
							<svg
								class="w-5 h-5 animate-spin"
								fill="none"
								viewBox="0 0 24 24"
							>
								<circle
									class="opacity-25"
									cx="12"
									cy="12"
									r="10"
									stroke="currentColor"
									stroke-width="4"
								></circle>
								<path
									class="opacity-75"
									fill="currentColor"
									d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
								></path>
							</svg>
						{:else}
							{$t('common.test')}
						{/if}
					</button>
				</div>
			</div>

			<!-- Quick test presets -->
			<div>
				<span class="text-sm text-[var(--ctp-overlay1)] mr-3">{$t('routes.inspector.quickTest')}:</span>
				<div class="inline-flex flex-wrap gap-2">
					{#each presets as preset}
						<button
							onclick={() => quickTest(preset)}
							disabled={testing}
							class="px-3 py-1 text-sm bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-md hover:bg-[var(--ctp-surface2)] disabled:opacity-50 transition-colors"
						>
							{preset.label}
						</button>
					{/each}
				</div>
			</div>
		</div>
	</div>

	<!-- Result Section -->
	{#if result}
		<div
			class="rounded-lg p-6 border {result.success
				? 'bg-[var(--ctp-green)] bg-opacity-10 border-[var(--ctp-green)]'
				: 'bg-[var(--ctp-red)] bg-opacity-10 border-[var(--ctp-red)]'}"
		>
			<!-- Status header -->
			<div class="flex items-center gap-3 mb-4">
				{#if result.success}
					<svg
						class="w-6 h-6 text-[var(--ctp-green)]"
						fill="none"
						stroke="currentColor"
						viewBox="0 0 24 24"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
						/>
					</svg>
					<span class="text-lg font-medium text-[var(--ctp-green)]"
						>{$t('status.connected')}</span
					>
				{:else}
					<svg
						class="w-6 h-6 text-[var(--ctp-red)]"
						fill="none"
						stroke="currentColor"
						viewBox="0 0 24 24"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"
						/>
					</svg>
					<span class="text-lg font-medium text-[var(--ctp-red)]"
						>{$t('common.error')}</span
					>
				{/if}
			</div>

			<!-- Details grid -->
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<!-- Latency -->
				{#if result.latency_ms !== undefined}
					<div class="bg-[var(--ctp-base)] rounded-lg p-4">
						<div class="text-sm text-[var(--ctp-overlay1)] mb-1">{$t('proxies.latency')}</div>
						<div class="text-xl font-semibold text-[var(--ctp-text)]">
							{result.latency_ms} ms
						</div>
					</div>
				{/if}

				<!-- Resolved IP -->
				{#if result.resolved_ip}
					<div class="bg-[var(--ctp-base)] rounded-lg p-4">
						<div class="text-sm text-[var(--ctp-overlay1)] mb-1">IP</div>
						<div class="text-lg font-mono text-[var(--ctp-text)]">
							{result.resolved_ip}
						</div>
					</div>
				{/if}

				<!-- Location -->
				{#if result.country}
					<div class="bg-[var(--ctp-base)] rounded-lg p-4">
						<div class="text-sm text-[var(--ctp-overlay1)] mb-1">{$t('connections.destinationIP')}</div>
						<div class="text-lg text-[var(--ctp-text)]">
							{#if result.country_code}
								<span class="mr-2">{getFlag(result.country_code)}</span>
							{/if}
							{result.country}
						</div>
					</div>
				{/if}

				<!-- Matched rule -->
				{#if result.matched_rule}
					<div class="bg-[var(--ctp-base)] rounded-lg p-4">
						<div class="text-sm text-[var(--ctp-overlay1)] mb-1">{$t('routes.matchedRule')}</div>
						<div class="text-lg text-[var(--ctp-text)]">
							{#if result.matched_rule.index >= 0}
								<span class="text-[var(--ctp-primary)]">#{result.matched_rule.index + 1}</span>
								<span class="mx-1">→</span>
							{/if}
							<span class="font-medium">{result.matched_rule.outbound || result.matched_rule.action}</span>
						</div>
						{#if result.matched_rule.reason}
							<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
								{result.matched_rule.reason}
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Error message -->
			{#if result.error}
				<div class="mt-4 p-3 bg-[var(--ctp-base)] rounded-lg">
					<div class="text-sm text-[var(--ctp-red)] font-mono">{result.error}</div>
				</div>
			{/if}
		</div>
	{:else}
		<!-- Empty state -->
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-12 text-center">
			<svg
				class="w-16 h-16 mx-auto text-[var(--ctp-overlay0)] mb-4"
				fill="none"
				stroke="currentColor"
				viewBox="0 0 24 24"
			>
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="1.5"
					d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0"
				/>
			</svg>
			<p class="text-[var(--ctp-overlay1)]">
				{$t('routes.inspector.emptyStateDescription')}
			</p>
		</div>
	{/if}
</div>

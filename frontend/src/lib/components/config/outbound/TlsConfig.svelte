<script lang="ts">
	import type { TLSConfig } from '$lib/types';
	import { t } from 'svelte-i18n';

	interface Props {
		tls: TLSConfig;
		showReality?: boolean;
		fingerprints?: string[];
	}

	let { tls = $bindable(), showReality = false, fingerprints = ['chrome', 'firefox', 'safari', 'edge', 'ios', 'android', 'random', 'randomized'] }: Props = $props();

	// Initialize nested objects if needed
	$effect(() => {
		if (!tls.utls) tls.utls = {};
		if (showReality && !tls.reality) tls.reality = {};
	});
</script>

<div class="space-y-4">
	<!-- Enable TLS -->
	<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
		<input
			type="checkbox"
			bind:checked={tls.enabled}
			class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
		/>
		{$t('outbounds.enableTls')}
	</label>

	{#if tls.enabled}
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<!-- SNI -->
			<div>
				<label for="tls-sni" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.sni')}
				</label>
				<input
					id="tls-sni"
					type="text"
					bind:value={tls.server_name}
					placeholder="example.com"
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>

			<!-- Fingerprint -->
			<div>
				<label for="tls-fp" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
					{$t('outbounds.fingerprint')}
				</label>
				<select
					id="tls-fp"
					bind:value={tls.utls!.fingerprint}
					class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				>
					<option value="">{$t('common.none')}</option>
					{#each fingerprints as fp}
						<option value={fp}>{fp}</option>
					{/each}
				</select>
			</div>
		</div>

		<!-- Insecure -->
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
			<input
				type="checkbox"
				bind:checked={tls.insecure}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
			/>
			{$t('outbounds.skipCertVerification')}
		</label>

		{#if showReality}
			<!-- Reality toggle -->
			<label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
				<input
					type="checkbox"
					bind:checked={tls.reality!.enabled}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]"
				/>
				{$t('outbounds.enableReality')}
			</label>

			{#if tls.reality?.enabled}
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pl-6">
					<!-- Reality Public Key -->
					<div>
						<label for="reality-pk" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
							{$t('outbounds.publicKey')} *
						</label>
						<input
							id="reality-pk"
							type="text"
							bind:value={tls.reality!.public_key}
							placeholder="Base64 public key"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>

					<!-- Reality Short ID -->
					<div>
						<label for="reality-sid" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
							{$t('outbounds.shortId')}
						</label>
						<input
							id="reality-sid"
							type="text"
							bind:value={tls.reality!.short_id}
							placeholder="0123456789abcdef"
							class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
						/>
					</div>
				</div>
			{/if}
		{/if}
	{/if}
</div>

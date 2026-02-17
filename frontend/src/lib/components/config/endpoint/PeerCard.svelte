<script lang="ts">
	import type { AWGPeer } from '$lib/types';
	import { t } from 'svelte-i18n';

	interface Props {
		peer: AWGPeer;
		index: number;
		showRemove: boolean;
		errors?: Record<string, string>;
		onRemove: () => void;
	}

	let { peer = $bindable(), index, showRemove, errors = {}, onRemove }: Props = $props();
</script>

<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('endpoints.peer')} {index + 1}</h3>
		{#if showRemove}
			<button
				type="button"
				onclick={onRemove}
				class="action-btn-danger"
			>
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
				</svg>
			</button>
		{/if}
	</div>

	<div class="grid grid-cols-3 gap-4">
		<div class="col-span-2">
			<label for="peer_{index}_address" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.serverAddress')} *</label>
			<input
				id="peer_{index}_address"
				type="text"
				bind:value={peer.address}
				placeholder="vpn.example.com"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors[`peer_${index}_address`] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
		</div>
		<div>
			<label for="peer_{index}_port" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('common.port')}</label>
			<input
				id="peer_{index}_port"
				type="number"
				bind:value={peer.port}
				min="1"
				max="65535"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
	</div>

	<div>
		<label for="peer_{index}_public_key" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.publicKey')} *</label>
		<input
			id="peer_{index}_public_key"
			type="text"
			bind:value={peer.public_key}
			placeholder={$t('endpoints.placeholders.publicKey')}
			class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm {errors[`peer_${index}_public_key`] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
	</div>

	<div>
		<label for="peer_{index}_psk" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.presharedKey')} ({$t('common.optional')})</label>
		<input
			id="peer_{index}_psk"
			type="password"
			bind:value={peer.preshared_key}
			placeholder={$t('endpoints.placeholders.psk')}
			class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		/>
	</div>

	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="peer_{index}_allowed_ips" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.allowedIPs')}</label>
			<input
				id="peer_{index}_allowed_ips"
				type="text"
				value={peer.allowed_ips.join(', ')}
				oninput={(e) => peer.allowed_ips = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean)}
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
			/>
		</div>
		<div>
			<label for="peer_{index}_keepalive" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.keepalive')}</label>
			<input
				id="peer_{index}_keepalive"
				type="number"
				bind:value={peer.persistent_keepalive_interval}
				min="0"
				placeholder="25"
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
			/>
		</div>
	</div>
</div>

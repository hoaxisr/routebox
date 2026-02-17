<script lang="ts">
	import type { AWGPeer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import PeerCard from './PeerCard.svelte';

	interface Props {
		peers: AWGPeer[];
		errors?: Record<string, string>;
	}

	let { peers = $bindable(), errors = {} }: Props = $props();

	function addPeer() {
		peers = [...peers, {
			address: '',
			port: 51820,
			public_key: '',
			preshared_key: undefined,
			allowed_ips: ['0.0.0.0/0', '::/0']
		}];
	}

	function removePeer(index: number) {
		peers = peers.filter((_, i) => i !== index);
	}
</script>

<div class="space-y-4">
	{#if errors['peers']}
		<p class="text-sm text-[var(--ctp-red)]">{errors['peers']}</p>
	{/if}

	{#each peers as _, i}
		<PeerCard
			bind:peer={peers[i]}
			index={i}
			showRemove={peers.length > 1}
			{errors}
			onRemove={() => removePeer(i)}
		/>
	{/each}

	<button
		type="button"
		onclick={addPeer}
		class="w-full py-2 border-2 border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-overlay1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors"
	>
		+ {$t('endpoints.addPeer')}
	</button>
</div>

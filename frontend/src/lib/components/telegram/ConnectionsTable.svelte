<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { MtprotoConnection } from '$lib/types';

	interface Props {
		connections: MtprotoConnection[];
	}

	let { connections }: Props = $props();

	// Only matched streams reach here — a connection that authenticated against
	// no secret is domain-fronted and belongs to no client, so the backend
	// leaves it out rather than showing a row nobody can act on.
	function since(iso: string): string {
		const started = Date.parse(iso);
		if (Number.isNaN(started)) return '—';

		const s = Math.max(0, Math.floor((Date.now() - started) / 1000));
		if (s < 60) return `${s}s`;
		if (s < 3600) return `${Math.floor(s / 60)}m`;
		return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
	}
</script>

{#if connections.length === 0}
	<div class="conn-empty">{$t('telegram.noConnections')}</div>
{:else}
	<div class="conn-table">
		<div class="conn-head">
			<span>{$t('telegram.connClient')}</span>
			<span>{$t('telegram.connIp')}</span>
			<span>{$t('telegram.connStarted')}</span>
		</div>
		{#each connections as c (c.stream_id)}
			<div class="conn-row">
				<span class="c-name">{c.client}</span>
				<span class="c-ip">{c.client_ip || '—'}</span>
				<span class="c-since">{since(c.started_at)}</span>
			</div>
		{/each}
	</div>
{/if}

<style>
	.conn-empty {
		color: var(--ctp-overlay1);
		font-size: 0.85rem;
		padding: 0.5rem 0;
	}
	.conn-table {
		display: flex;
		flex-direction: column;
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		overflow: hidden;
	}
	.conn-head,
	.conn-row {
		display: grid;
		grid-template-columns: 1fr 1fr 6rem;
		gap: 1rem;
		padding: 0.55rem 0.9rem;
		align-items: center;
	}
	.conn-head {
		background: var(--ctp-surface0);
		color: var(--ctp-overlay0);
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.conn-row {
		border-top: 1px solid var(--ctp-surface0);
		font-size: 0.8125rem;
	}
	.conn-row:first-of-type {
		border-top: none;
	}
	.c-name {
		font-weight: 600;
		color: var(--ctp-text);
	}
	.c-ip {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		color: var(--ctp-subtext0);
	}
	.c-since {
		color: var(--ctp-overlay1);
	}
	@media (max-width: 720px) {
		.conn-head {
			display: none;
		}
		.conn-row {
			grid-template-columns: 1fr auto;
		}
		.c-ip {
			grid-column: 1 / -1;
		}
	}
</style>

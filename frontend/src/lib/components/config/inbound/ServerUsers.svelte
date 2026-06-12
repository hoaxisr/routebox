<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { ServerInboundUser } from '$lib/types';
	import type { ServerInboundType } from '$lib/utils/serverInbound';

	interface Props {
		users: ServerInboundUser[];
		protocol: ServerInboundType;
		onShare?: (index: number) => void;
		canShare: boolean;
	}

	let { users = $bindable(), protocol, onShare, canShare }: Props = $props();

	async function addUser() {
		const u: ServerInboundUser = {};
		if (protocol === 'vless') {
			u.name = `user-${users.length + 1}`;
			try {
				u.uuid = (await api.generateUuid()).uuid;
			} catch {
				notifications.error($t('inbounds.server.genFailed'));
			}
			u.flow = 'xtls-rprx-vision';
		} else if (protocol === 'naive') {
			u.username = `user-${users.length + 1}`;
			try {
				u.password = (await api.generatePassword()).password;
			} catch {
				notifications.error($t('inbounds.server.genFailed'));
			}
		} else {
			u.name = `user-${users.length + 1}`;
			try {
				u.password = (await api.generatePassword()).password;
			} catch {
				notifications.error($t('inbounds.server.genFailed'));
			}
		}
		users = [...users, u];
	}

	function removeUser(i: number) {
		users = users.filter((_, idx) => idx !== i);
	}
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('inbounds.server.users')}</h3>
		<button type="button" onclick={addUser}
			class="text-sm px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
			+ {$t('inbounds.server.addUser')}
		</button>
	</div>

	{#if users.length === 0}
		<p class="text-sm text-[var(--ctp-overlay0)]">{$t('inbounds.server.noUsers')}</p>
	{/if}

	{#each users as user, i}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-3 space-y-2">
			<div class="flex items-center justify-between gap-2">
				{#if protocol === 'naive'}
					<input type="text" bind:value={user.username} placeholder={$t('inbounds.username')}
						class="flex-1 px-2 py-1.5 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded text-sm text-[var(--ctp-text)]" />
				{:else}
					<input type="text" bind:value={user.name} placeholder={$t('common.name')}
						class="flex-1 px-2 py-1.5 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded text-sm text-[var(--ctp-text)]" />
				{/if}
				<div class="flex items-center gap-1">
					{#if canShare && onShare}
						<button type="button" onclick={() => onShare?.(i)}
							class="text-xs px-2 py-1.5 bg-[var(--ctp-primary)] text-white rounded hover:opacity-90">
							{$t('inbounds.server.clientLink')}
						</button>
					{/if}
					<button type="button" onclick={() => removeUser(i)}
						class="action-btn-danger text-xs px-2 py-1.5 rounded">✕</button>
				</div>
			</div>

			{#if protocol === 'vless'}
				<input type="text" bind:value={user.uuid} placeholder="uuid"
					class="w-full px-2 py-1.5 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded text-sm text-[var(--ctp-text)] break-all" />
			{:else}
				<input type="text" bind:value={user.password} placeholder={$t('inbounds.server.password')}
					class="w-full px-2 py-1.5 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded text-sm text-[var(--ctp-text)] break-all" />
			{/if}
		</div>
	{/each}
</div>

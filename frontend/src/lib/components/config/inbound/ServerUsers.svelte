<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { ServerInboundUser, PanelUser } from '$lib/types';
	import type { ServerInboundType } from '$lib/utils/serverInbound';

	interface Props {
		users: ServerInboundUser[];
		protocol: ServerInboundType;
	}

	let { users = $bindable(), protocol }: Props = $props();

	let panelUsers = $state<PanelUser[]>([]);
	onMount(async () => {
		try {
			panelUsers = await api.getUsers();
		} catch {
			/* non-fatal: the existing-user picker just stays empty */
		}
	});

	// Build a ServerInboundUser for `name` with a freshly-generated credential for the
	// current protocol (only the name is parameterised — credentials are always fresh).
	async function genCredential(name: string): Promise<ServerInboundUser> {
		const u: ServerInboundUser = {};
		if (protocol === 'vless') {
			u.name = name;
			u.uuid = (await api.generateUuid()).uuid;
			u.flow = 'xtls-rprx-vision';
		} else if (protocol === 'naive') {
			u.username = name;
			u.password = (await api.generatePassword()).password;
		} else {
			u.name = name;
			u.password = (await api.generatePassword()).password;
		}
		return u;
	}

	async function addUser() {
		try {
			users = [...users, await genCredential(`user-${users.length + 1}`)];
		} catch {
			notifications.error($t('inbounds.server.genFailed'));
		}
	}

	// Attach an EXISTING panel user: reuse their name (+ a fresh credential) so
	// Reconcile groups by name into one panel user with one binding per inbound.
	async function addExisting(name: string) {
		if (!name) return;
		try {
			users = [...users, await genCredential(name)];
		} catch {
			notifications.error($t('inbounds.server.genFailed'));
		}
	}

	// Existing panel users not already on this inbound (naive identifies by username,
	// others by name); nameless users are excluded.
	const usedNames = $derived(new Set(users.map((u) => u.username ?? u.name ?? '')));
	const availableUsers = $derived(panelUsers.filter((p) => p.name && !usedNames.has(p.name)));

	function removeUser(i: number) {
		users = users.filter((_, idx) => idx !== i);
	}
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('inbounds.server.users')}</h3>
		<div class="flex items-center gap-2">
			{#if availableUsers.length > 0}
				<select
					onchange={(e) => {
						addExisting(e.currentTarget.value);
						e.currentTarget.value = '';
					}}
					class="text-sm px-2 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg border border-[var(--ctp-surface2)]">
					<option value="">+ {$t('inbounds.server.addExisting')}</option>
					{#each availableUsers as p (p.id)}
						<option value={p.name}>{p.name}</option>
					{/each}
				</select>
			{/if}
			<button type="button" onclick={addUser}
				class="text-sm px-3 py-1.5 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)]">
				+ {$t('inbounds.server.addUser')}
			</button>
		</div>
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

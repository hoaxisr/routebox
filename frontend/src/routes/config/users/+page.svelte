<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, unsavedChanges, formatBytes } from '$lib/stores';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ShareModal from '$lib/components/config/users/ShareModal.svelte';
	import Sparkline from '$lib/components/shared/Sparkline.svelte';
	import type { PanelUser, Inbound, UserTrafficResponse } from '$lib/types';

	let users = $state<PanelUser[]>([]);
	let serverInbounds = $state<{ tag: string; type: string }[]>([]);
	let loading = $state(true);

	let showAdd = $state(false);
	let addName = $state('');
	let addTag = $state('');
	let adding = $state(false);

	// Add-binding modal state.
	let bindUser = $state<PanelUser | null>(null);
	let bindTag = $state('');
	let binding = $state(false);

	// Share modal state.
	let shareUser = $state<PanelUser | null>(null);
	let publicHost = $state('');
	let publicPort = $state<number | undefined>(undefined);

	const serverTypes = ['vless', 'naive', 'hysteria2', 'mieru'];

	// Per-user traffic history, lazily fetched on expand. Keyed by user id.
	// 'loading' marks an in-flight fetch so the row can show a placeholder.
	let expanded = $state<Record<string, UserTrafficResponse | 'loading'>>({});

	async function toggleTraffic(id: string) {
		if (expanded[id]) {
			delete expanded[id];
			expanded = { ...expanded };
			return;
		}
		expanded = { ...expanded, [id]: 'loading' };
		try {
			expanded = { ...expanded, [id]: await api.getUserTraffic(id, '24h') };
		} catch (e) {
			delete expanded[id];
			expanded = { ...expanded };
			notifications.error(`${$t('users.trafficLoadFailed')}: ${e}`);
		}
	}

	async function load() {
		loading = true;
		try {
			users = await api.getUsers();
			const inbounds = await api.listInbounds();
			serverInbounds = (inbounds as Inbound[])
				.filter((i) => serverTypes.includes(i.type))
				.map((i) => ({ tag: i.tag, type: i.type }));
			if (!addTag && serverInbounds.length > 0) addTag = serverInbounds[0].tag;
			try {
				const s = await api.getSettings();
				publicHost = s.settings.server?.public_host ?? '';
				publicPort = s.settings.server?.public_port;
			} catch {
				/* public host optional */
			}
		} catch (e) {
			notifications.error(`${$t('users.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	function protocolOf(tag: string): string {
		return serverInbounds.find((i) => i.tag === tag)?.type ?? 'vless';
	}

	async function submitAdd() {
		if (!addName.trim() || !addTag) return;
		adding = true;
		try {
			const protocol = protocolOf(addTag);
			await api.createUser({ name: addName.trim(), protocol, inbound_tag: addTag });
			notifications.success($t('users.created', { values: { name: addName.trim() } }));
			unsavedChanges.markChanged('Users', `Added user "${addName.trim()}"`);
			showAdd = false;
			addName = '';
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			adding = false;
		}
	}

	async function remove(u: PanelUser) {
		if (!confirm($t('users.deleteConfirm', { values: { name: u.name } }))) return;
		try {
			await api.deleteUser(u.id);
			unsavedChanges.markChanged('Users', `Removed user "${u.name}"`);
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	type UserStatus = 'active' | 'disabled' | 'expired';
	function userStatus(u: PanelUser): UserStatus {
		if (!u.enabled) return 'disabled';
		if (u.expires_at && u.expires_at > 0 && u.expires_at <= Math.floor(Date.now() / 1000)) {
			return 'expired';
		}
		return 'active';
	}

	// The native date input is kept off-screen and driven by a styled button, the
	// same shape the AWG peer roster uses — a bare <input type="date"> rendered a
	// locale placeholder ("дд.мм.гггг") that read as an unlabelled stray field (#36).
	let dateEls = $state<Record<string, HTMLInputElement | null>>({});
	function openDatePicker(id: string) {
		const el = dateEls[id];
		if (el?.showPicker) el.showPicker();
		else el?.focus();
	}

	// <input type="date"> value (YYYY-MM-DD, local) <-> unix seconds.
	function expiryToDateInput(u: PanelUser): string {
		if (!u.expires_at || u.expires_at <= 0) return '';
		const d = new Date(u.expires_at * 1000);
		const off = d.getTimezoneOffset() * 60000;
		return new Date(d.getTime() - off).toISOString().slice(0, 10);
	}

	async function toggleEnabled(u: PanelUser) {
		try {
			await api.updateUser(u.id, { enabled: !u.enabled });
			notifications.success($t(u.enabled ? 'users.disabled' : 'users.enabled', { values: { name: u.name } }));
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	async function setExpiry(u: PanelUser, value: string) {
		// Empty => clear (0 = never). A date => seconds at LOCAL midnight.
		const expires_at = value ? Math.floor(new Date(value + 'T00:00:00').getTime() / 1000) : 0;
		try {
			await api.updateUser(u.id, { expires_at });
			notifications.success($t('users.expirySet', { values: { name: u.name } }));
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		}
	}

	function openShare(u: PanelUser) {
		if (u.pending) return;
		shareUser = u;
	}

	// Server inbounds the user is NOT already bound to.
	function availableInbounds(u: PanelUser): { tag: string; type: string }[] {
		const bound = new Set(u.bindings.map((b) => b.inbound_tag));
		return serverInbounds.filter((i) => !bound.has(i.tag));
	}

	function openAddBinding(u: PanelUser) {
		const avail = availableInbounds(u);
		if (avail.length === 0) return;
		bindUser = u;
		bindTag = avail[0].tag;
	}

	async function submitBinding() {
		if (!bindUser || !bindTag) return;
		binding = true;
		try {
			await api.addUserBinding(bindUser.id, { protocol: protocolOf(bindTag), inbound_tag: bindTag });
			notifications.success($t('users.bindingAdded', { values: { tag: bindTag } }));
			unsavedChanges.markChanged('Users', `Added binding "${bindTag}" to "${bindUser.name}"`);
			bindUser = null;
			await load();
		} catch (e) {
			notifications.error(`${e}`);
		} finally {
			binding = false;
		}
	}

	onMount(load);

	// The global pending-changes bar applies/discards the draft elsewhere. When it
	// does, the server reconciles the registry (pending users materialize, removed
	// ones vanish), so re-fetch on the hasChanges true→false transition — otherwise
	// the list keeps showing a stale "pending" entry after Apply.
	let prevHasChanges = false;
	$effect(() => {
		const has = $unsavedChanges.hasChanges;
		if (prevHasChanges && !has) {
			load();
		}
		prevHasChanges = has;
	});
</script>

<svelte:head><title>{$t('users.title')} - RouteBox</title></svelte:head>

<div class="space-y-4 max-w-4xl">
	<div class="flex items-center justify-between flex-wrap gap-3">
		<div>
			<h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('users.title')}</h1>
			<p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('users.description')}</p>
		</div>
		<button onclick={() => (showAdd = true)} disabled={serverInbounds.length === 0}
			class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
			{$t('users.add')}
		</button>
	</div>

	{#if loading}
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	{:else if serverInbounds.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('users.noServerInbounds')}</div>
	{:else if users.length === 0}
		<div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">{$t('users.empty')}</div>
	{:else}
		<div class="space-y-3">
			{#each users as u, i (u.id || `${u.name} ${u.bindings[0]?.credential ?? ''} ${i}`)}
				<section class="bg-[var(--ctp-mantle)] rounded-lg p-5 border border-[var(--ctp-surface0)]">
					<div class="flex items-start justify-between gap-4 flex-wrap">
						<div class="min-w-0">
							<div class="flex items-center gap-2">
								<h2 class="text-lg font-medium text-[var(--ctp-text)]">{u.name || '(unnamed)'}</h2>
								{#if u.pending}
									<span class="status-badge info">{$t('users.pending')}</span>
								{:else if userStatus(u) === 'active'}
									<span class="status-badge success">{$t('users.active')}</span>
								{:else if userStatus(u) === 'expired'}
									<span class="status-badge error">{$t('users.expired')}</span>
								{:else}
									<span class="status-badge info">{$t('users.disabledLabel')}</span>
								{/if}
							</div>
							<div class="mt-2 flex flex-wrap gap-1">
								{#each u.bindings as b}
									<span class="status-badge">{b.inbound_tag}</span>
								{/each}
							</div>
							{#if !u.pending && u.id}
								<div class="mt-3 flex items-center gap-3 flex-wrap">
									<button class="traffic-cell" onclick={() => u.id && toggleTraffic(u.id)} title={$t('users.usage')}>
										<span class="up">↑ {formatBytes(u.upload ?? 0)}</span>
										<span class="down">↓ {formatBytes(u.download ?? 0)}</span>
									</button>
									{#if expanded[u.id] === 'loading'}
										<span class="text-xs text-[var(--ctp-overlay0)]">{$t('common.loading')}</span>
									{:else if expanded[u.id]}
										<Sparkline values={(expanded[u.id] as UserTrafficResponse).history.map((p) => p.upload + p.download)} />
									{/if}
								</div>
							{/if}
						</div>
						<div class="flex items-center gap-2 flex-wrap">
							{#if u.pending}
								<span class="text-xs text-[var(--ctp-overlay0)]">{$t('users.pendingNoLink')}</span>
							{:else}
								<button onclick={() => openShare(u)}
									class="px-3 py-1.5 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm">
									{$t('users.share')}
								</button>
							{/if}
							{#if !u.pending && u.id}
								<button onclick={() => toggleEnabled(u)}
									class="px-3 py-1.5 text-[var(--ctp-text)] border border-[var(--ctp-surface2)] rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors text-sm">
									{u.enabled ? $t('users.disable') : $t('users.enable')}
								</button>
								{@const expiry = expiryToDateInput(u)}
								<div class="expiry-cell">
									<span class="expiry-label">{$t('users.expiresAt')}</span>
									<input type="date" class="expiry-native" tabindex="-1" aria-hidden="true"
										value={expiry} bind:this={dateEls[u.id]}
										onchange={(e) => setExpiry(u, (e.currentTarget as HTMLInputElement).value)} />
									<button type="button" class="expiry-btn" onclick={() => u.id && openDatePicker(u.id)}>
										<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
										{expiry || $t('users.setDate')}
									</button>
									{#if expiry}
										<button type="button" class="expiry-clear" onclick={() => setExpiry(u, '')}>{$t('users.never')}</button>
									{/if}
								</div>
							{/if}
							{#if u.id}
								<button onclick={() => openAddBinding(u)} disabled={availableInbounds(u).length === 0}
									title={availableInbounds(u).length === 0 ? $t('users.noAvailableInbounds') : ''}
									class="px-3 py-1.5 text-[var(--ctp-text)] border border-[var(--ctp-surface2)] rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors text-sm disabled:opacity-50">
									{$t('users.addBinding')}
								</button>
							{/if}
							{#if u.id}
								<button onclick={() => remove(u)}
									class="px-3 py-1.5 text-[var(--ctp-red)] border border-[var(--ctp-red)]/40 rounded-lg hover:bg-[var(--ctp-red)]/10 transition-colors text-sm">
									{$t('users.delete')}
								</button>
							{/if}
						</div>
					</div>
				</section>
			{/each}
		</div>
	{/if}
</div>

<Modal open={showAdd} title={$t('users.addTitle')} size="md" onClose={() => (showAdd = false)}>
	<div class="space-y-3">
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('users.name')}</span>
			<input bind:value={addName} type="text"
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</label>
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('users.inbound')}</span>
			<select bind:value={addTag}
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				{#each serverInbounds as i}
					<option value={i.tag}>{i.tag} ({i.type})</option>
				{/each}
			</select>
		</label>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button onclick={() => (showAdd = false)} class="px-4 py-2 text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors">
				{$t('common.cancel')}
			</button>
			<button onclick={submitAdd} disabled={adding || !addName.trim() || !addTag}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
				{adding ? $t('common.loading') : $t('users.add')}
			</button>
		</div>
	{/snippet}
</Modal>

<Modal open={!!bindUser} title={$t('users.addBindingTitle')} size="md" onClose={() => (bindUser = null)}>
	{#if bindUser}
		<label class="block">
			<span class="text-sm text-[var(--ctp-subtext1)]">{$t('users.inbound')}</span>
			<select bind:value={bindTag}
				class="mt-1 w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]">
				{#each availableInbounds(bindUser) as i}
					<option value={i.tag}>{i.tag} ({i.type})</option>
				{/each}
			</select>
		</label>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button onclick={() => (bindUser = null)} class="px-4 py-2 text-[var(--ctp-subtext1)] hover:bg-[var(--ctp-surface0)] rounded-lg transition-colors">
				{$t('common.cancel')}
			</button>
			<button onclick={submitBinding} disabled={binding || !bindTag}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50">
				{binding ? $t('common.loading') : $t('users.addBinding')}
			</button>
		</div>
	{/snippet}
</Modal>

{#if shareUser}
	<ShareModal user={shareUser} {publicHost} {publicPort}
		onClose={() => (shareUser = null)}
		onChanged={load} />
{/if}

<style>
	.traffic-cell {
		display: inline-flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.25rem 0.5rem;
		border-radius: 0.375rem;
		font-size: 0.8125rem;
		font-variant-numeric: tabular-nums;
		background: var(--ctp-surface0);
		transition: background-color 0.15s;
	}
	.traffic-cell:hover {
		background: var(--ctp-surface1);
	}
	.traffic-cell .up {
		color: var(--ctp-primary);
	}
	.traffic-cell .down {
		color: var(--ctp-overlay1);
	}

	.expiry-cell {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.expiry-label {
		font-size: 0.75rem;
		color: var(--ctp-overlay1);
	}
	.expiry-native {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
		pointer-events: none;
	}
	.expiry-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.375rem 0.75rem;
		border-radius: 0.5rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-mantle);
		color: var(--ctp-text);
		font-size: 0.875rem;
		cursor: pointer;
		transition: border-color 0.15s;
	}
	.expiry-btn:hover {
		border-color: var(--ctp-primary);
	}
	.expiry-clear {
		padding: 0.375rem 0.6rem;
		border-radius: 0.5rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface0);
		color: var(--ctp-subtext1);
		font-size: 0.8rem;
		cursor: pointer;
		transition:
			border-color 0.15s,
			color 0.15s;
	}
	.expiry-clear:hover {
		border-color: var(--ctp-primary);
		color: var(--ctp-primary);
	}
</style>

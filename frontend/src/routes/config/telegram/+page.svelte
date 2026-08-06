<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import type { MtprotoState, MtprotoClient, MtprotoConnection, MtprotoSettings } from '$lib/types';
	import ClientRoster from '$lib/components/telegram/ClientRoster.svelte';
	import ServerSettingsForm from '$lib/components/telegram/ServerSettingsForm.svelte';
	import ConnectionsTable from '$lib/components/telegram/ConnectionsTable.svelte';

	let proxy = $state<MtprotoState | null>(null);
	let clients = $state<MtprotoClient[]>([]);
	let connections = $state<MtprotoConnection[]>([]);
	let form = $state<MtprotoSettings | null>(null);

	let loading = $state(true);
	let saving = $state(false);
	let busy = $state(false);
	let settingsOpen = $state(false);

	const running = $derived(!!proxy?.status.running);
	const canShare = $derived(!!proxy?.can_issue_link);
	const readOnly = $derived(!!proxy?.read_only);
	const formDirty = $derived(
		!!form && !!proxy && JSON.stringify(form) !== JSON.stringify(proxy.settings)
	);

	// Enable is only meaningful once there is something to serve and somewhere to
	// pretend to be. Saying which is missing beats a disabled button with no
	// explanation, so the reason is rendered next to it.
	const blockedReason = $derived(
		clients.filter((c) => c.enabled).length === 0
			? $t('telegram.needsClient')
			: !proxy?.settings.masking_domain
				? $t('telegram.needsDomain')
				: ''
	);

	function cloneSettings(s: MtprotoSettings): MtprotoSettings {
		return { ...s };
	}

	async function loadAll() {
		loading = true;
		try {
			proxy = await api.mtprotoStatus();
			form = cloneSettings(proxy.settings);
			clients = await api.getMtprotoClients();
			connections = running ? await api.getMtprotoConnections() : [];
		} catch (e) {
			notifications.error(`${$t('telegram.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	// Refreshes everything the roster and strip show, without touching the
	// settings form — reloading that under the user would discard their edits.
	async function refresh() {
		try {
			const next = await api.mtprotoStatus();
			proxy = next;
			clients = await api.getMtprotoClients();
			connections = next.status.running ? await api.getMtprotoConnections() : [];
		} catch (e) {
			notifications.error(`${$t('telegram.loadFailed')}: ${e}`);
		}
	}

	let poll: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		loadAll();
		// The online dots and the connection list are only meaningful live.
		poll = setInterval(() => {
			if (!loading && !saving) refresh();
		}, 5000);
	});

	onDestroy(() => {
		if (poll) clearInterval(poll);
	});

	async function enable() {
		busy = true;
		try {
			await api.mtprotoEnable();
			notifications.success($t('telegram.running'));
			await refresh();
		} catch (e) {
			notifications.error(`${$t('telegram.startFailed')}: ${e}`);
		} finally {
			busy = false;
		}
	}

	async function disable() {
		busy = true;
		try {
			await api.mtprotoDisable();
			await refresh();
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		} finally {
			busy = false;
		}
	}

	async function saveSettings() {
		if (!form) return;
		saving = true;
		try {
			await api.updateMtprotoSettings({
				listen: form.listen,
				masking_domain: form.masking_domain.trim(),
				public_host: form.public_host.trim(),
				public_port: form.public_port,
				concurrency: form.concurrency,
				idle_timeout_sec: form.idle_timeout_sec,
				prefer_ip: form.prefer_ip,
				domain_fronting_port: form.domain_fronting_port,
				outbound: form.outbound,
				socks_port: form.socks_port
			});
			notifications.success($t('telegram.settingsSaved'));
			await refresh();
			if (proxy) form = cloneSettings(proxy.settings);
		} catch (e) {
			notifications.error(`${$t('telegram.saveFailed')}: ${e}`);
		} finally {
			saving = false;
		}
	}

	function resetForm() {
		if (proxy) form = cloneSettings(proxy.settings);
	}
</script>

<svelte:head><title>{$t('telegram.title')} - RouteBox</title></svelte:head>

{#if loading}
	<div class="mt-page">
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	</div>
{:else if !proxy || !form}
	<div class="mt-page">
		<div class="status-badge error">{$t('telegram.loadFailed')}</div>
	</div>
{:else}
	<div class="mt-page">
		<div class="page-head">
			<h1>{$t('telegram.title')}</h1>
			<p>{$t('telegram.description')}</p>
		</div>

		{#if readOnly}
			<div class="status-badge error err-line">{$t('telegram.readOnly')}</div>
		{/if}

		<!-- Status strip -->
		<div class="status-strip">
			<div class="strip-state">
				{#if running}
					<span class="live-dot"></span>
					<div>
						<div class="label">{$t('telegram.running')}</div>
						<div class="sub">{proxy.settings.masking_domain}</div>
					</div>
				{:else}
					<span class="dead-dot"></span>
					<div>
						<div class="label">{$t('telegram.stopped')}</div>
						<div class="sub">{proxy.settings.masking_domain || '—'}</div>
					</div>
				{/if}
			</div>

			<div class="strip-divider"></div>

			<div class="strip-metric">
				<span class="m-val mono">TCP {proxy.settings.listen.split(':').pop()}</span>
				<span class="m-key">{$t('telegram.listenPort')}</span>
			</div>

			<div class="strip-metric">
				<span class="m-val">{proxy.status.connected} / {proxy.status.clients}</span>
				<span class="m-key">{$t('telegram.connected')}</span>
			</div>

			{#if proxy.public_host}
				<div class="strip-metric">
					<span class="m-val mono">{proxy.public_host}:{proxy.public_port}</span>
					<span class="m-key">{$t('telegram.publicAddress')}</span>
				</div>
			{/if}

			<div class="strip-spacer"></div>

			{#if running}
				<button type="button" class="btn-toggle-power" onclick={disable} disabled={busy}>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0" /><line x1="12" y1="2" x2="12" y2="12" /></svg>
					{$t('telegram.disable')}
				</button>
			{:else}
				<button type="button" class="btn-enable" onclick={enable} disabled={busy || readOnly || !!blockedReason}>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0" /><line x1="12" y1="2" x2="12" y2="12" /></svg>
					{busy ? $t('telegram.enabling') : $t('telegram.enable')}
				</button>
			{/if}
		</div>

		{#if !running && blockedReason}
			<div class="note-line">{blockedReason}</div>
		{:else if !canShare}
			<div class="note-line">{$t('telegram.linkBlocked')}</div>
		{/if}

		<!-- Clients — centerpiece -->
		<div class="card clients-card">
			<div class="clients-head">
				<div class="title-wrap">
					<h2>{$t('telegram.clients')}</h2>
					<span class="count-pill">{clients.length}</span>
				</div>
			</div>
			<div class="clients-body">
				<ClientRoster {clients} {canShare} {readOnly} onChange={refresh} />
			</div>
		</div>

		<!-- Live connections -->
		{#if running}
			<div class="card conn-card">
				<div class="clients-head">
					<div class="title-wrap">
						<h2>{$t('telegram.connections')}</h2>
						<span class="count-pill">{connections.length}</span>
					</div>
				</div>
				<div class="clients-body">
					<ConnectionsTable {connections} />
				</div>
			</div>
		{/if}

		<!-- Server settings — progressive disclosure -->
		<div class="disclosure" class:open={settingsOpen}>
			<button type="button" class="disclosure-head" onclick={() => (settingsOpen = !settingsOpen)}>
				<span class="chev">
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
				</span>
				<span class="d-title">{$t('telegram.serverSettings')}</span>
				<span class="d-sub">{$t('telegram.serverSettingsSub')}</span>
			</button>
			{#if settingsOpen}
				<div class="disclosure-body">
					<ServerSettingsForm
						bind:form
						savedDomain={proxy.settings.masking_domain}
						outbounds={proxy.outbounds ?? []}
						{saving}
						dirty={formDirty}
						onSave={saveSettings}
						onReset={resetForm}
					/>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.mt-page {
		max-width: 980px;
		margin: 0 auto;
		padding: 0.5rem 0 3rem;
	}

	.page-head {
		margin-bottom: 1.25rem;
	}
	.page-head h1 {
		font-size: 1.5rem;
		font-weight: 600;
		margin: 0 0 0.25rem;
		letter-spacing: -0.01em;
		color: var(--ctp-text);
	}
	.page-head p {
		margin: 0;
		color: var(--ctp-overlay1);
		font-size: 0.875rem;
		max-width: 60ch;
	}

	/* ---- status strip ---- */
	.status-strip {
		display: flex;
		align-items: center;
		gap: 1.5rem;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		padding: 0.875rem 1.25rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}
	.strip-state {
		display: flex;
		align-items: center;
		gap: 0.625rem;
	}
	.live-dot {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--ctp-green);
		box-shadow: 0 0 0 0 color-mix(in srgb, var(--ctp-green) 70%, transparent);
		animation: mt-pulse 2s infinite;
	}
	.dead-dot {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--ctp-overlay0);
	}
	@keyframes mt-pulse {
		0% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--ctp-green) 50%, transparent);
		}
		70% {
			box-shadow: 0 0 0 7px color-mix(in srgb, var(--ctp-green) 0%, transparent);
		}
		100% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--ctp-green) 0%, transparent);
		}
	}
	.strip-state .label {
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--ctp-text);
	}
	.strip-state .sub {
		color: var(--ctp-overlay1);
		font-size: 0.75rem;
	}
	.strip-divider {
		width: 1px;
		align-self: stretch;
		background: var(--ctp-surface1);
	}
	.strip-metric {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.strip-metric .m-val {
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--ctp-text);
	}
	.strip-metric .m-val.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.875rem;
	}
	.strip-metric .m-key {
		color: var(--ctp-overlay0);
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.strip-spacer {
		flex: 1;
	}
	.btn-toggle-power {
		padding: 0.5rem 1rem;
		border-radius: 0.375rem;
		border: 1px solid var(--ctp-surface2);
		background: var(--ctp-surface0);
		color: var(--ctp-subtext1);
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s ease;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.btn-toggle-power:hover:not(:disabled) {
		border-color: var(--ctp-red);
		color: var(--ctp-red);
	}
	.btn-enable {
		padding: 0.5rem 1rem;
		border-radius: 0.375rem;
		border: none;
		background: var(--ctp-primary);
		color: #1a1a1a;
		font-weight: 600;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.btn-enable:hover:not(:disabled) {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-enable:disabled,
	.btn-toggle-power:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.note-line {
		color: var(--ctp-overlay1);
		font-size: 0.8125rem;
		margin: -0.25rem 0 1rem;
	}
	.err-line {
		display: block;
		margin-bottom: 1rem;
		word-break: break-word;
	}

	/* ---- cards ---- */
	.card {
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
	}
	.clients-card,
	.conn-card {
		margin-bottom: 1rem;
	}
	.clients-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1.25rem 1.25rem 0.75rem;
		gap: 1rem;
	}
	.title-wrap {
		display: flex;
		align-items: center;
		gap: 0.625rem;
	}
	.clients-head h2 {
		font-size: 1.0625rem;
		font-weight: 600;
		margin: 0;
		color: var(--ctp-text);
	}
	.count-pill {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--ctp-overlay1);
		background: var(--ctp-surface0);
		border-radius: 999px;
		padding: 0.125rem 0.55rem;
	}
	.clients-body {
		padding: 0 1.25rem 1.25rem;
	}

	/* ---- server settings disclosure ---- */
	.disclosure {
		margin-bottom: 1rem;
	}
	.disclosure-head {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		padding: 0.875rem 1.25rem;
		text-align: left;
		cursor: pointer;
		transition: border-color 0.15s ease;
	}
	.disclosure.open .disclosure-head {
		border-radius: 0.5rem 0.5rem 0 0;
		border-bottom-color: transparent;
	}
	.disclosure-head:hover {
		border-color: var(--ctp-surface2);
	}
	.disclosure-head .chev {
		color: var(--ctp-overlay1);
		transition: transform 0.15s ease;
		display: flex;
	}
	.disclosure.open .chev {
		transform: rotate(90deg);
	}
	.disclosure-head .d-title {
		font-weight: 600;
		color: var(--ctp-text);
	}
	.disclosure-head .d-sub {
		color: var(--ctp-overlay0);
		font-size: 0.8125rem;
	}
	.disclosure-body {
		border: 1px solid var(--ctp-surface0);
		border-top: none;
		border-radius: 0 0 0.5rem 0.5rem;
		padding: 1.25rem;
		background: var(--ctp-mantle);
	}
</style>

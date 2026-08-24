<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications, routerMode } from '$lib/stores';
	import { formatBytes } from '$lib/stores/settings';
	import type { AwgStatus, AwgPeer, AwgServerSettings } from '$lib/types';
	import ServerSettingsForm from '$lib/components/awg/ServerSettingsForm.svelte';
	import PeerRoster from '$lib/components/awg/PeerRoster.svelte';
	import BackendPicker from '$lib/components/awg/BackendPicker.svelte';

	// Where the "not installed" notice sends people: the upstream kernel module
	// project's own install instructions (per distro), not a RouteBox doc — the
	// module isn't RouteBox's to install outside the Debian/Ubuntu PPA path.
	const AMNEZIAWG_MODULE_REPO = 'https://github.com/amnezia-vpn/amneziawg-linux-kernel-module';

	let status = $state<AwgStatus | null>(null);
	let settings = $state<AwgServerSettings | null>(null);
	let form = $state<AwgServerSettings | null>(null);
	let peers = $state<AwgPeer[]>([]);

	let loading = $state(true);
	let saving = $state(false);
	let enabling = $state(false);
	let settingsOpen = $state(false);

	const mode = $derived(settings?.configured ? 'steady' : 'wizard');
	// Form differs from saved settings — drives the Save button state + unsaved marker.
	const formDirty = $derived(!!form && !!settings && JSON.stringify(form) !== JSON.stringify(settings));
	const isSingbox = $derived((status?.backend ?? settings?.backend) === 'singbox');
	const backendValue = $derived<'kernel' | 'singbox'>(isSingbox ? 'singbox' : 'kernel');
	// AWG3 controls (header protection, CPA/RAT) are available on singbox always,
	// and on the kernel backend only when the host's module + awg-quick/tools have
	// both confirmed awg3 capability (status.kernel_awg3_available).
	const awg3Available = $derived(isSingbox || !!status?.kernel_awg3_available);
	const awg31Available = $derived(isSingbox || !!status?.kernel_awg31_available);

	async function changeBackend(b: 'kernel' | 'singbox') {
		try {
			await api.updateSettings({ 'awg.backend': b });
			await loadAll();
		} catch (e) {
			notifications.error(`${$t('awg.saveFailed')}: ${e}`);
		}
	}

	function cloneSettings(s: AwgServerSettings): AwgServerSettings {
		return { ...s, dns: [...(s.dns ?? [])], obf: { ...s.obf } };
	}

	async function loadAll() {
		loading = true;
		try {
			status = await api.awgStatus();
			const resp = await api.getSettings();
			settings = resp.settings.awg ?? null;
			form = settings ? cloneSettings(settings) : null;
			// Router-friendly default: prefill the client-facing server address with
			// the host the admin browsed to (typically the router LAN/WAN IP).
			if (form && !form.server_host) form.server_host = window.location.hostname;
			peers = status?.enabled ? await api.getAwgPeers() : [];
		} catch (e) {
			notifications.error(`${$t('awg.loadFailed')}: ${e}`);
		} finally {
			loading = false;
		}
	}

	async function refreshStatus() {
		try {
			status = await api.awgStatus();
		} catch (e) {
			notifications.error(`${$t('awg.loadFailed')}: ${e}`);
		}
	}

	async function refreshPeers() {
		peers = status?.enabled ? await api.getAwgPeers() : [];
	}

	async function refreshLive() {
		await refreshStatus();
		await refreshPeers();
	}

	let poll: ReturnType<typeof setInterval> | null = null;

	// Persist the visible form, then refresh status (picks up config_dirty).
	async function save(): Promise<boolean> {
		if (!form) return false;
		saving = true;
		try {
			await api.updateSettings({
				'awg.server_host': form.server_host,
				'awg.subnet': form.subnet,
				'awg.listen_port': form.listen_port,
				'awg.mtu': form.mtu,
				'awg.client_keepalive': (form.client_keepalive ?? '').trim(),
				'awg.dns': form.dns,
				'awg.wan_iface': form.wan_iface,
				'awg.obf': form.obf,
				'awg.obf_preset': form.obf_preset,
				'awg.header_protection': form.header_protection,
				'awg.ipv6_broker': form.ipv6_broker
			});
			settings = cloneSettings(form);
			await refreshStatus();
			return true;
		} catch (e) {
			notifications.error(`${$t('awg.saveFailed')}: ${e}`);
			return false;
		} finally {
			saving = false;
		}
	}

	async function onSaveClick() {
		if (await save()) notifications.success($t('awg.settingsSaved'));
	}

	// One action for the settings form: while the server runs, saving must also apply
	// (re-render + restart) or the change stays inert until the drift banner is noticed.
	// Stopped server → plain save; the Turn-on/Enable step brings it up (enable() saves first).
	async function saveOrApply() {
		if (status?.enabled) await apply();
		else await onSaveClick();
	}

	function onResetClick() {
		if (settings) form = cloneSettings(settings);
	}

	// WYSIWYG: persist the visible form, then bring the interface up.
	async function enable() {
		enabling = true;
		try {
			if (!(await save())) return;
			status = await api.awgEnable();
			notifications.success($t('awg.enabled'));
			// Re-fetch settings so `configured` flips → page switches to steady.
			const resp = await api.getSettings();
			settings = resp.settings.awg ?? settings;
			form = settings ? cloneSettings(settings) : form;
			await refreshPeers();
		} catch (e) {
			notifications.error(`${$t('awg.enableFailed')}: ${e}`);
		} finally {
			enabling = false;
		}
	}

	// Apply = same as enable (save → re-render + restart interface).
	async function apply() {
		enabling = true;
		try {
			if (!(await save())) return;
			status = await api.awgEnable();
			notifications.success($t('awg.applied'));
			await refreshPeers();
		} catch (e) {
			notifications.error(`${$t('awg.applyFailed')}: ${e}`);
		} finally {
			enabling = false;
		}
	}

	async function disable() {
		if (!confirm($t('awg.disableConfirm'))) return;
		try {
			status = await api.awgDisable();
			peers = [];
			notifications.success($t('awg.disabled'));
		} catch (e) {
			notifications.error(`${$t('awg.disableFailed')}: ${e}`);
		}
	}

	onMount(() => {
		loadAll();
		// The online dots and traffic figures are only meaningful live — poll while
		// the tab is open, skipping a tick that would race a save/enable in flight.
		poll = setInterval(() => {
			if (!loading && !saving && !enabling) refreshLive();
		}, 5000);
	});

	onDestroy(() => {
		if (poll) clearInterval(poll);
	});
</script>

<svelte:head><title>{$t('awg.title')} - RouteBox</title></svelte:head>

{#if loading}
	<div class="awg-page">
		<div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
	</div>
{:else if !status || !form || !settings}
	<div class="awg-page">
		<div class="status-badge error">{$t('awg.loadFailed')}</div>
	</div>
{:else if mode === 'steady'}
	<!-- ================= STEADY (Concept B — Hand Out Access) ================= -->
	<div class="awg-page">
		<div class="page-head">
			<h1>{$t('awg.title')}</h1>
			<p>{$t('awg.description')}</p>
		</div>

		<BackendPicker value={backendValue} disabled={status.enabled} allowKernel={!$routerMode} onChange={changeBackend} />

		<!-- Status strip -->
		<div class="status-strip">
			<div class="strip-state">
				{#if status.enabled && (isSingbox || status.iface_up)}
					<span class="live-dot"></span>
					<div>
						<div class="label">{$t('awg.running')}</div>
						<div class="sub">{isSingbox ? 'sing-box' : settings.interface} · {status.peer_count} {$t('awg.peers')}</div>
					</div>
				{:else}
					<span class="dead-dot"></span>
					<div>
						<div class="label">{$t('awg.stopped')}</div>
						<div class="sub">{isSingbox ? 'sing-box' : settings.interface}</div>
					</div>
				{/if}
			</div>

			<div class="strip-divider"></div>

			<div class="strip-metric">
				<span class="m-val mono">UDP {status.listen_port || form.listen_port}</span>
				<span class="m-key">{$t('awg.listenPort')}</span>
			</div>

			<div class="strip-metric">
				<span class="m-val">{status.online} / {status.peer_count}</span>
				<span class="m-key">{$t('awg.connected')}</span>
			</div>

			<div class="strip-metric">
				<span class="m-val mono">↓ {formatBytes(status.rx)} &nbsp;↑ {formatBytes(status.tx)}</span>
				<span class="m-key">{$t('awg.traffic')}</span>
			</div>

				{#if status.public_host}
				<div class="strip-metric">
					<span class="m-val mono">{status.public_host}</span>
					<span class="m-key">WAN{!isSingbox && status.wan_iface ? ` · ${status.wan_iface}` : ''}</span>
				</div>
			{/if}

			<div class="strip-spacer"></div>

			{#if status.enabled}
				<button type="button" class="btn-toggle-power" onclick={disable}>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0" /><line x1="12" y1="2" x2="12" y2="12" /></svg>
					{$t('awg.disable')}
				</button>
			{:else}
				<button type="button" class="btn-enable" onclick={enable} disabled={enabling}>
					<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18.36 6.64a9 9 0 1 1-12.73 0" /><line x1="12" y1="2" x2="12" y2="12" /></svg>
					{enabling ? $t('awg.enabling') : $t('awg.enable')}
				</button>
			{/if}
		</div>

		{#if status.last_error}
			<div class="status-badge error err-line">{status.last_error}</div>
		{/if}
		{#if !isSingbox && status.enabled && status.nat_orphan}
			<div class="status-badge error err-line">{$t('awg.natOrphan')}</div>
		{/if}

		<!-- Apply banner on config drift -->
		{#if status.config_dirty && status.enabled}
			<div class="alert-box primary apply-banner">
				<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--ctp-primary)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /></svg>
				<div class="apply-text">
					<strong>{$t('awg.dirtyTitle')}</strong>
					<span>{$t('awg.dirtyWarn')}</span>
				</div>
				<button type="button" class="btn-apply" onclick={apply} disabled={enabling}>
					{enabling ? $t('awg.applying') : $t('awg.apply')}
				</button>
			</div>
		{/if}

		<!-- Clients — centerpiece -->
		<div class="card clients-card">
			<div class="clients-head">
				<div class="title-wrap">
					<h2>{$t('awg.clients')}</h2>
					<span class="count-pill">{peers.length}</span>
				</div>
			</div>
			<div class="clients-body">
				{#if status.enabled}
					<PeerRoster {peers} subnet={form.subnet} singbox={isSingbox} onChange={async () => { await refreshStatus(); await refreshPeers(); }} />
				{:else}
					<div class="locked-note">{$t('awg.shareLocked')}</div>
				{/if}
			</div>
		</div>

		<!-- Server settings — progressive disclosure -->
		<div class="disclosure" class:open={settingsOpen}>
			<button type="button" class="disclosure-head" onclick={() => (settingsOpen = !settingsOpen)}>
				<span class="chev">
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
				</span>
				<span class="d-title">{$t('awg.serverSettings')}</span>
				<span class="d-sub">{$t('awg.serverSettingsSub')}</span>
			</button>
			{#if settingsOpen}
				<div class="disclosure-body">
					<ServerSettingsForm bind:form saving={saving || enabling} {awg3Available} {awg31Available} {isSingbox} applied={status.enabled} dirty={formDirty} ipv6Active={status.ipv6_active} onSave={saveOrApply} onReset={onResetClick} />
				</div>
			{/if}
		</div>

		{#if !isSingbox}
			{#if status.kernel_module_version}
				<div class="dep-note">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><polyline points="20 6 9 17 4 12" /></svg>
					{$t('awg.depNoteVersion', { values: { version: status.kernel_module_version } })}
				</div>
			{:else}
				<div class="dep-note warn">
					<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /></svg>
					<span>{$t('awg.depNoteMissing')}</span>
					<a href={AMNEZIAWG_MODULE_REPO} target="_blank" rel="noopener noreferrer">{$t('awg.installInstructions')}</a>
				</div>
			{/if}
		{/if}
	</div>
{:else}
	<!-- ================= WIZARD (Concept C — Guided Lifecycle) ================= -->
	<div class="awg-page">
		<div class="page-head head-flex">
			<div>
				<h1>{$t('awg.title')}</h1>
				<p>{$t('awg.wizardIntro')}</p>
			</div>
			{#if status.enabled}
				<div class="live-pill">
					<span class="live-dot"></span>
					<div>
						<div class="lp-label">{$t('awg.running')}</div>
						<div class="lp-sub">{isSingbox ? 'sing-box' : settings.interface} · {status.peer_count}</div>
					</div>
				</div>
			{/if}
		</div>

		<BackendPicker value={backendValue} disabled={status.enabled} allowKernel={!$routerMode} onChange={changeBackend} />

		<div class="spine">
			<!-- STEP 1 — Setup / readiness (kernel backend only) -->
			{#if !isSingbox}
				<section class="step done">
					<div class="step-marker" aria-hidden="true">
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
					</div>
					<div class="card">
						<div class="step-head">
							<div>
								<div class="step-eyebrow">{$t('awg.stepSetup')}</div>
								<h3 class="step-title">{$t('awg.stepReadinessTitle')}</h3>
							</div>
							<span class="status-badge success">{$t('awg.ready')}</span>
						</div>
						<p class="step-desc">{$t('awg.stepReadinessDesc')}</p>
						<div class="ready-row">
							<div class="ready-chip">
								<span class="ok"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg></span>
								<span>{$t('awg.kernelModule')}</span>
								{#if status.kernel_module_version}
									<span class="status-badge success">v{status.kernel_module_version}</span>
								{:else}
									<span class="status-badge error">{$t('awg.notInstalled')}</span>
								{/if}
							</div>
						</div>
						{#if !status.kernel_module_version}
							<div class="dep-note warn">
								<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" width="14" height="14"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /></svg>
								<span>{$t('awg.depNoteMissing')}</span>
								<a href={AMNEZIAWG_MODULE_REPO} target="_blank" rel="noopener noreferrer">{$t('awg.installInstructions')}</a>
							</div>
						{/if}
					</div>
				</section>
			{/if}

			<!-- STEP 2 — Configure -->
			<section class="step" class:done={settings.configured} class:active={!status.enabled}>
				<div class="step-marker" aria-hidden="true">2</div>
				<div class="card">
					<div class="step-head">
						<div>
							<div class="step-eyebrow">{$t('awg.stepConfigure')}</div>
							<h3 class="step-title">{$t('awg.stepConfigureTitle')}</h3>
						</div>
					</div>
					<p class="step-desc">{$t('awg.stepConfigureDesc')}</p>
					<ServerSettingsForm bind:form saving={saving || enabling} {awg3Available} {awg31Available} {isSingbox} applied={status.enabled} dirty={formDirty} ipv6Active={status.ipv6_active} onSave={saveOrApply} onReset={onResetClick} />
				</div>
			</section>

			<!-- STEP 3 — Turn on -->
			<section class="step" class:active={!status.enabled} class:done={status.enabled}>
				<div class="step-marker" aria-hidden="true">
					{#if status.enabled}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
					{:else}
						3
					{/if}
				</div>
				<div class="card">
					<div class="step-head">
						<div>
							<div class="step-eyebrow">{$t('awg.stepTurnOn')}</div>
							<h3 class="step-title">{$t('awg.stepTurnOnTitle')}</h3>
						</div>
						{#if status.enabled}
							<span class="status-badge success">{$t('awg.up')}</span>
						{/if}
					</div>
					<p class="step-desc">{$t('awg.stepTurnOnDesc')}</p>

					{#if status.last_error}
						<div class="status-badge error err-line">{status.last_error}</div>
					{/if}

					{#if status.enabled}
						<div class="power-card on">
							<div class="power-icon">
								<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 2v8" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6.3 6.3a8 8 0 1011.4 0" /></svg>
							</div>
							<div>
								<div class="ptitle">{$t('awg.serverOn')}</div>
								<div class="psub">{$t('awg.serverOnSub')}</div>
							</div>
							<button type="button" class="btn-danger" onclick={disable}>{$t('awg.disable')}</button>
						</div>
					{:else}
						<button type="button" class="btn-turnon" onclick={enable} disabled={enabling}>
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 2v8" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6.3 6.3a8 8 0 1011.4 0" /></svg>
							{enabling ? $t('awg.enabling') : $t('awg.turnOnNow')}
						</button>
					{/if}
				</div>
			</section>

			<!-- STEP 4 — Share -->
			<section class="step" class:done={status.enabled}>
				<div class="step-marker" aria-hidden="true">
					{#if status.enabled}
						<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
					{:else}
						4
					{/if}
				</div>
				<div class="card" class:dimmed={!status.enabled}>
					<div class="step-head">
						<div>
							<div class="step-eyebrow">{$t('awg.stepShare')}</div>
							<h3 class="step-title">{$t('awg.stepShareTitle')}</h3>
						</div>
						{#if status.enabled}
							<span class="status-badge info">{peers.length} {$t('awg.peers')}</span>
						{/if}
					</div>
					<p class="step-desc">{$t('awg.stepShareDesc')}</p>

					{#if status.enabled}
						<PeerRoster {peers} subnet={form.subnet} singbox={isSingbox} onChange={async () => { await refreshStatus(); await refreshPeers(); }} />
					{:else}
						<div class="locked-note">{$t('awg.shareLocked')}</div>
					{/if}
				</div>
			</section>
		</div>
	</div>
{/if}

<style>
	.awg-page {
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
	.head-flex {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	/* ---- status strip (steady) ---- */
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
		animation: awg-pulse 2s infinite;
	}
	.dead-dot {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--ctp-overlay0);
	}
	@keyframes awg-pulse {
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
	.btn-toggle-power:hover {
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
	.btn-enable:hover {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-enable:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.err-line {
		display: block;
		margin-bottom: 1rem;
		word-break: break-word;
	}

	/* ---- alert / apply banner ---- */
	.alert-box {
		padding: 0.75rem 1rem;
		border-radius: 0.5rem;
		border: 1px solid;
		margin-bottom: 1.25rem;
	}
	.alert-box.primary {
		background-color: color-mix(in srgb, var(--ctp-primary) 10%, transparent);
		border-color: var(--ctp-primary);
	}
	.apply-banner {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.apply-text {
		flex: 1;
	}
	.apply-text strong {
		color: var(--ctp-text);
		font-weight: 600;
	}
	.apply-text span {
		display: block;
		color: var(--ctp-subtext0);
		font-size: 0.8125rem;
		margin-top: 1px;
	}
	.btn-apply {
		padding: 0.5rem 1.1rem;
		border-radius: 0.375rem;
		border: none;
		background: var(--ctp-primary);
		color: #1a1a1a;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
	}
	.btn-apply:hover {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-apply:disabled {
		opacity: 0.5;
		cursor: default;
	}

	/* ---- card ---- */
	.card {
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.5rem;
		padding: 1.25rem;
	}
	.clients-card {
		padding: 0;
		overflow: hidden;
		margin-bottom: 1.25rem;
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
	.locked-note {
		color: var(--ctp-overlay1);
		font-size: 0.85rem;
		padding: 1rem;
		text-align: center;
		border: 1px dashed var(--ctp-surface2);
		border-radius: 0.5rem;
	}

	/* ---- disclosure (steady settings) ---- */
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

	.dep-note {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
		padding: 0 0.25rem;
		color: var(--ctp-overlay0);
		font-size: 0.75rem;
	}
	.dep-note svg {
		color: var(--ctp-green);
		flex-shrink: 0;
	}
	.dep-note.warn {
		color: var(--ctp-red);
	}
	.dep-note.warn svg {
		color: var(--ctp-red);
	}
	.dep-note a {
		color: inherit;
		text-decoration: underline;
	}

	/* ---- wizard ---- */
	.live-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface0);
		padding: 0.5rem 0.85rem;
		border-radius: 999px;
		white-space: nowrap;
		flex-shrink: 0;
	}
	.live-pill .lp-label {
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--ctp-green);
	}
	.live-pill .lp-sub {
		font-size: 0.72rem;
		color: var(--ctp-overlay1);
	}

	.spine {
		position: relative;
		padding-left: 3.25rem;
	}
	.spine::before {
		content: '';
		position: absolute;
		left: 1.18rem;
		top: 1.4rem;
		bottom: 1.4rem;
		width: 2px;
		background: var(--ctp-surface1);
		z-index: 0;
	}
	.step {
		position: relative;
		margin-bottom: 1.5rem;
	}
	.step:last-child {
		margin-bottom: 0;
	}
	.step-marker {
		position: absolute;
		left: -3.25rem;
		top: 0;
		width: 2.4rem;
		height: 2.4rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: 600;
		font-size: 0.95rem;
		z-index: 1;
		border: 2px solid var(--ctp-surface1);
		background: var(--ctp-base);
		color: var(--ctp-overlay1);
	}
	.step.done .step-marker {
		border-color: var(--ctp-green);
		background: color-mix(in srgb, var(--ctp-green) 18%, var(--ctp-base));
		color: var(--ctp-green);
	}
	.step.active .step-marker {
		border-color: var(--ctp-primary);
		background: var(--ctp-primary);
		color: #fff;
		box-shadow: 0 0 0 5px color-mix(in srgb, var(--ctp-primary) 18%, transparent);
	}
	.step.active .card {
		border-color: color-mix(in srgb, var(--ctp-primary) 55%, var(--ctp-surface0));
		box-shadow:
			0 0 0 1px color-mix(in srgb, var(--ctp-primary) 25%, transparent),
			0 8px 24px -12px color-mix(in srgb, var(--ctp-primary) 40%, transparent);
	}
	.card.dimmed {
		opacity: 0.65;
	}
	.step-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.75rem;
		margin-bottom: 0.25rem;
	}
	.step-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--ctp-overlay0);
		font-weight: 600;
	}
	.step-title {
		font-size: 1.05rem;
		font-weight: 600;
		margin: 0.1rem 0 0;
		color: var(--ctp-text);
	}
	.step-desc {
		color: var(--ctp-overlay1);
		font-size: 0.85rem;
		margin: 0 0 1rem;
	}
	.ready-row {
		display: flex;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	.ready-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--ctp-base);
		border: 1px solid var(--ctp-surface0);
		border-radius: 0.45rem;
		padding: 0.55rem 0.8rem;
		font-size: 0.83rem;
		color: var(--ctp-subtext1);
	}
	.ready-chip .ok {
		color: var(--ctp-green);
		display: inline-flex;
	}

	.power-card {
		display: flex;
		flex-direction: column;
		justify-content: center;
		align-items: center;
		text-align: center;
		gap: 0.75rem;
		border-radius: 0.45rem;
		padding: 1.1rem;
	}
	.power-card.on {
		background: color-mix(in srgb, var(--ctp-green) 8%, var(--ctp-base));
		border: 1px solid color-mix(in srgb, var(--ctp-green) 30%, var(--ctp-surface0));
	}
	.power-icon {
		width: 3rem;
		height: 3rem;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		background: color-mix(in srgb, var(--ctp-green) 20%, transparent);
		color: var(--ctp-green);
	}
	.ptitle {
		font-weight: 600;
		color: var(--ctp-text);
	}
	.psub {
		font-size: 0.78rem;
		color: var(--ctp-overlay1);
	}
	.btn-danger {
		background: transparent;
		color: var(--ctp-red);
		border: 1px solid color-mix(in srgb, var(--ctp-red) 45%, transparent);
		border-radius: 0.4rem;
		padding: 0.5rem 1rem;
		font-size: 0.88rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s ease;
	}
	.btn-danger:hover {
		background: color-mix(in srgb, var(--ctp-red) 12%, transparent);
	}
	.btn-turnon {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--ctp-primary);
		color: #1a1a1a;
		border: none;
		border-radius: 0.4rem;
		padding: 0.65rem 1.4rem;
		font-size: 0.92rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.btn-turnon:hover {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-turnon:disabled {
		opacity: 0.5;
		cursor: default;
	}

	@media (max-width: 720px) {
		.spine {
			padding-left: 2.75rem;
		}
		.step-marker {
			left: -2.75rem;
			width: 2rem;
			height: 2rem;
		}
	}
</style>

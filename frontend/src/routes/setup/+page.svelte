<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import { parseConfig, toSingboxConfig, type ParsedConfig } from '$lib/utils/parsers';
	import type { SystemChecks } from '$lib/types';
	import ProgressIndicator from '$lib/components/shared/ProgressIndicator.svelte';
	import InstallStep from '$lib/components/setup/InstallStep.svelte';
	import VpnConfigStep from '$lib/components/setup/VpnConfigStep.svelte';
	import UsageModeStep from '$lib/components/setup/UsageModeStep.svelte';
	import RuleSetsStep from '$lib/components/setup/RuleSetsStep.svelte';
	import RoutingModeStep from '$lib/components/setup/RoutingModeStep.svelte';
	import ApplyStep from '$lib/components/setup/ApplyStep.svelte';

	// System state
	let loading = $state(true);
	let binaryInstalled = $state(true);
	let checkingInstall = $state(false);
	let systemChecks = $state<SystemChecks | null>(null);

	// Wizard state
	let currentStep = $state(1);
	let totalSteps = $derived.by(() => {
		const baseSteps = usageMode === 'router' ? 5 : 3;
		return binaryInstalled ? baseSteps : baseSteps + 1;
	});
	let effectiveStep = $derived(binaryInstalled ? currentStep : currentStep - 1);

	// Step 1: VPN Config
	let vpnInput = $state('');
	let parsedVpn = $state<ParsedConfig | null>(null);
	let vpnError = $state('');

	// Step 2: Usage Mode
	let usageMode = $state<'router' | 'proxy'>('router');
	let proxyPort = $state(1080);
	let machineIP = $state('');

	// Step 3: Rule Sets
	let selectedRuleSets = $state<string[]>(['antizapret']);
	const availableRuleSets = [
		{ id: 'antizapret', name: 'Antizapret', desc: 'Russian blocked sites', url: 'https://github.com/savely-krasovsky/antizapret-sing-box/releases/latest/download/antizapret.srs' },
		{ id: 'geosite-ru', name: 'GeoSite RU', desc: 'Russian domains', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ru.srs' },
		{ id: 'geoip-ru', name: 'GeoIP RU', desc: 'Russian IPs', url: 'https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-ru.srs' },
		{ id: 'adblock', name: 'AdBlock', desc: 'Block ads', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ads-all.srs' },
	];

	// Step 4: Routing Mode
	let routingMode = $state<'split' | 'all'>('split');

	// Step 5: Apply
	let applying = $state(false);
	let applied = $state(false);

	$effect(() => {
		if (typeof window !== 'undefined') {
			machineIP = window.location.hostname || 'localhost';
		}
	});

	onMount(async () => {
		try {
			const [setupStatus, processStatus] = await Promise.all([
				api.needsSetup(),
				api.getStatus()
			]);
			binaryInstalled = setupStatus.binary_installed;
			systemChecks = processStatus.system_checks || null;
			if (!binaryInstalled) {
				currentStep = 0;
			}
		} catch (e) {
			console.error('Failed to check setup status:', e);
		} finally {
			loading = false;
		}
	});

	async function checkInstallation() {
		checkingInstall = true;
		try {
			const status = await api.needsSetup();
			binaryInstalled = status.binary_installed;
			if (binaryInstalled) {
				currentStep = 1;
				notifications.success($t('setup.install.detected'));
			} else {
				notifications.error($t('setup.install.notFoundError'));
			}
		} catch (e) {
			notifications.error($t('setup.install.checkFailed'));
		} finally {
			checkingInstall = false;
		}
	}

	function parseVpnConfig() {
		vpnError = '';
		parsedVpn = null;

		if (!vpnInput.trim()) {
			vpnError = $t('setup.vpn.emptyError');
			return;
		}

		const result = parseConfig(vpnInput);

		if (result.success && result.config) {
			parsedVpn = result.config;
		} else {
			vpnError = result.error || 'Failed to parse configuration';
		}
	}

	function toggleRuleSet(id: string) {
		if (selectedRuleSets.includes(id)) {
			selectedRuleSets = selectedRuleSets.filter(r => r !== id);
		} else {
			selectedRuleSets = [...selectedRuleSets, id];
		}
	}

	function canProceed(): boolean {
		if (currentStep === 0) return binaryInstalled;

		if (usageMode === 'router') {
			switch (effectiveStep) {
				case 1: return parsedVpn !== null;
				case 2: return true;
				case 3: return true;
				case 4: return true;
				case 5: return applied;
				default: return false;
			}
		} else {
			switch (effectiveStep) {
				case 1: return parsedVpn !== null;
				case 2: return true;
				case 3: return applied;
				default: return false;
			}
		}
	}

	function nextStep() {
		if (currentStep < totalSteps && canProceed()) {
			currentStep++;
		}
	}

	function prevStep() {
		if (currentStep > 1) {
			currentStep--;
		}
	}

	async function applyConfiguration() {
		if (!parsedVpn) return;

		applying = true;
		try {
			const singbox = toSingboxConfig(parsedVpn);
			const vpnTag = singbox.outboundTag;

			async function safeCreate<T>(
				createFn: () => Promise<T>,
				resourceName: string
			): Promise<boolean> {
				try {
					await createFn();
					return true;
				} catch (err: unknown) {
					const msg = err instanceof Error ? err.message : String(err);
					if (msg.includes('already exists')) {
						console.log(`${resourceName} already exists, skipping`);
						return true;
					}
					throw err;
				}
			}

			if (singbox.endpoint) {
				await safeCreate(
					() => api.createEndpoint(singbox.endpoint!),
					'VPN endpoint'
				);
			}

			if (singbox.outbound) {
				await safeCreate(
					() => api.createOutbound(singbox.outbound!),
					'VPN outbound'
				);
			}

			if (usageMode === 'proxy') {
				await safeCreate(
					() => api.createInbound({
						type: 'mixed',
						tag: 'mixed-in',
						listen: '0.0.0.0',
						listen_port: proxyPort,
					}),
					'Mixed inbound'
				);

				await safeCreate(
					() => api.createOutbound({ type: 'direct', tag: 'direct' }),
					'direct outbound'
				);

				await safeCreate(
					() => api.createRule({
						inbound: ['mixed-in'],
						outbound: vpnTag
					}),
					'proxy route rule'
				);

				await api.updateRouteSettings({
					final: 'direct',
					auto_detect_interface: true
				});

			} else {
				await safeCreate(
					() => api.createInbound({
						type: 'tun',
						tag: 'tun-in',
						address: ['172.19.0.1/30', 'fdfe:dcba:9876::1/126'],
						auto_route: true,
						strict_route: true,
						stack: 'gvisor',
					}),
					'TUN inbound'
				);

				await safeCreate(
					() => api.createOutbound({ type: 'direct', tag: 'direct' }),
					'direct outbound'
				);
				await safeCreate(
					() => api.createOutbound({ type: 'block', tag: 'block' }),
					'block outbound'
				);

				await safeCreate(
					() => api.createDnsServer({
						tag: 'dns-direct',
						type: 'udp',
						server: '8.8.8.8',
					}),
					'direct DNS server'
				);
				await api.updateDnsSettings({ final: 'dns-direct', strategy: 'prefer_ipv4' });

				for (const rsId of selectedRuleSets) {
					const rs = availableRuleSets.find(r => r.id === rsId);
					if (rs) {
						await safeCreate(
							() => api.createRuleSet({
								tag: rs.id,
								type: 'remote',
								format: 'binary',
								url: rs.url,
							}),
							`rule set ${rs.id}`
						);
					}
				}

				await safeCreate(
					() => api.createRule({ action: 'sniff' }),
					'sniff rule'
				);
				await safeCreate(
					() => api.createRule({ action: 'hijack-dns', protocol: ['dns'] }),
					'DNS hijack rule'
				);
				await safeCreate(
					() => api.createRule({ ip_is_private: true, outbound: 'direct' }),
					'private IP rule'
				);

				if (routingMode === 'split') {
					for (const rsId of selectedRuleSets) {
						if (rsId === 'adblock') {
							await safeCreate(
								() => api.createRule({ rule_set: [rsId], outbound: 'block' }),
								'adblock rule'
							);
						} else {
							await safeCreate(
								() => api.createRule({ rule_set: [rsId], outbound: vpnTag }),
								`${rsId} rule`
							);
						}
					}
					await api.updateRouteSettings({ final: 'direct', auto_detect_interface: true });
				} else {
					if (selectedRuleSets.includes('adblock')) {
						await safeCreate(
							() => api.createRule({ rule_set: ['adblock'], outbound: 'block' }),
							'adblock rule'
						);
					}
					await api.updateRouteSettings({ final: vpnTag, auto_detect_interface: true });
				}
			}

			await api.updateExperimental({
				clash_api: {
					external_controller: '127.0.0.1:9090',
					default_mode: 'rule'
				}
			});

			await api.updateLogSettings({
				level: 'info',
				timestamp: true
			});

			await api.applyConfig();

			applied = true;
			notifications.success($t('setup.apply.configApplied'));
		} catch (err) {
			notifications.error($t('setup.apply.configError', { values: { error: String(err) } }));
		} finally {
			applying = false;
		}
	}

	function finish() {
		goto('/');
	}

	function getStepNumber(step: number): number {
		return binaryInstalled ? step : step + 1;
	}

	// Determine if we're on the final step
	let isOnFinalStep = $derived(
		(usageMode === 'router' && effectiveStep === 5) ||
		(usageMode === 'proxy' && effectiveStep === 3)
	);
</script>

<svelte:head>
	<title>{$t('setup.pageTitle')}</title>
</svelte:head>

{#if loading}
	<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center">
		<div class="text-center">
			<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)] mx-auto" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
			<p class="mt-2 text-[var(--ctp-overlay1)]">{$t('setup.checkingSystem')}</p>
		</div>
	</div>
{:else}
<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center p-6">
	<div class="w-full max-w-2xl">
		<!-- Header -->
		<div class="text-center mb-8">
			<h1 class="text-3xl font-bold text-[var(--ctp-text)]">{$t('setup.title')}</h1>
			<p class="text-[var(--ctp-overlay1)] mt-2">{$t('setup.subtitle')}</p>
		</div>

		<!-- System Requirements Warning -->
		{#if systemChecks && !systemChecks.all_checks_passed}
			<div class="bg-[var(--ctp-red)] rounded-xl p-4 mb-6">
				<div class="flex items-start gap-3">
					<svg class="w-6 h-6 text-white flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					<div class="flex-1">
						<h3 class="text-white font-semibold">{$t('setup.systemRequirementsNotMet')}</h3>
						<div class="text-white/90 text-sm mt-1 space-y-1">
							{#if !systemChecks.is_root}
								<p>{$t('setup.runWithSudo', { values: { command: 'sudo' } }).replace('{command}', '')}<code class="bg-white/20 px-1.5 py-0.5 rounded">sudo</code></p>
							{/if}
							{#if !systemChecks.ipv4_forward}
								<p>{$t('setup.enableIpForwarding', { values: { command: '' } }).replace('{command}', '')}<code class="bg-white/20 px-1.5 py-0.5 rounded">sysctl -w net.ipv4.ip_forward=1</code></p>
							{/if}
						</div>
					</div>
				</div>
			</div>
		{/if}

		<!-- Progress -->
		<div class="mb-8">
			<ProgressIndicator {currentStep} {totalSteps} />
		</div>

		<!-- Step Content -->
		<div class="bg-[var(--ctp-mantle)] rounded-xl p-8 border border-[var(--ctp-surface0)]">
			{#if currentStep === 0}
				<InstallStep checking={checkingInstall} onCheck={checkInstallation} />
			{:else if effectiveStep === 1}
				<VpnConfigStep
					stepNumber={getStepNumber(1)}
					bind:vpnInput
					{parsedVpn}
					bind:error={vpnError}
					onInput={parseVpnConfig}
				/>
			{:else if effectiveStep === 2}
				<UsageModeStep
					stepNumber={getStepNumber(2)}
					bind:usageMode
					bind:proxyPort
					{machineIP}
					onModeChange={() => {}}
					onPortChange={() => {}}
				/>
			{:else if effectiveStep === 3 && usageMode === 'router'}
				<RuleSetsStep
					stepNumber={getStepNumber(3)}
					{availableRuleSets}
					bind:selectedRuleSets
					onToggle={toggleRuleSet}
				/>
			{:else if effectiveStep === 4 && usageMode === 'router'}
				<RoutingModeStep
					stepNumber={getStepNumber(4)}
					bind:routingMode
					onModeChange={() => {}}
				/>
			{:else if isOnFinalStep}
				<ApplyStep
					stepNumber={getStepNumber(usageMode === 'router' ? 5 : 3)}
					{parsedVpn}
					{usageMode}
					{selectedRuleSets}
					{routingMode}
					{proxyPort}
					{machineIP}
					{applying}
					{applied}
					onApply={applyConfiguration}
				/>
			{/if}
		</div>

		<!-- Navigation -->
		<div class="flex justify-between mt-6">
			<button
				onclick={prevStep}
				disabled={currentStep === 0 || (currentStep === 1 && binaryInstalled)}
				class="px-6 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] disabled:opacity-50 transition-colors"
			>
				{$t('setup.navigation.back')}
			</button>

			{#if isOnFinalStep && applied}
				<button
					onclick={finish}
					class="px-6 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					{$t('setup.navigation.goToDashboard')}
				</button>
			{:else if currentStep > 0 && !isOnFinalStep}
				<button
					onclick={nextStep}
					disabled={!canProceed()}
					class="px-6 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity"
				>
					{$t('setup.navigation.next')}
				</button>
			{/if}
		</div>

		<!-- Skip link -->
		<div class="text-center mt-4">
			<a href="/" class="text-sm text-[var(--ctp-overlay0)] hover:text-[var(--ctp-text)] transition-colors">
				{$t('setup.navigation.skipSetup')}
			</a>
		</div>
	</div>
</div>
{/if}

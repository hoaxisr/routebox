<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';
	import { parseConfig, toSingboxConfig, type ParsedConfig } from '$lib/utils/parsers';
	import type { SystemChecks } from '$lib/types';

	// System state
	let loading = $state(true);
	let binaryInstalled = $state(true);
	let checkingInstall = $state(false);
	let systemChecks = $state<SystemChecks | null>(null);

	// Wizard state - step 0 is install step (only shown if binary not installed)
	// Router mode: VPN → Mode → RuleSets → Routing → Apply (5 steps, or 6 if install needed)
	// Proxy mode:  VPN → Mode → Apply (3 steps, or 4 if install needed)
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

	// Get machine IP from current location (browser only)
	let machineIP = $state('');
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
			// If binary not installed, start at step 0 (install step)
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
				notifications.success('amnezia-box detected! You can now continue setup.');
			} else {
				notifications.error('amnezia-box not found. Please install it first.');
			}
		} catch (e) {
			notifications.error('Failed to check installation status');
		} finally {
			checkingInstall = false;
		}
	}

	// Step 2: Rule Sets
	let selectedRuleSets = $state<string[]>(['antizapret']);
	const availableRuleSets = [
		{ id: 'antizapret', name: 'Antizapret', desc: 'Russian blocked sites', url: 'https://github.com/savely-krasovsky/antizapret-sing-box/releases/latest/download/antizapret.srs' },
		{ id: 'geosite-ru', name: 'GeoSite RU', desc: 'Russian domains', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ru.srs' },
		{ id: 'geoip-ru', name: 'GeoIP RU', desc: 'Russian IPs', url: 'https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-ru.srs' },
		{ id: 'adblock', name: 'AdBlock', desc: 'Block ads', url: 'https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ads-all.srs' },
	];

	// Step 3: Routing Mode
	let routingMode = $state<'split' | 'all'>('split');

	// Step 4: Apply
	let applying = $state(false);
	let applied = $state(false);

	function parseVpnConfig() {
		vpnError = '';
		parsedVpn = null;

		if (!vpnInput.trim()) {
			vpnError = 'Please enter a VPN configuration';
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
		// Step 0 is install step (only when binary not installed)
		if (currentStep === 0) return binaryInstalled;

		if (usageMode === 'router') {
			// Router: VPN(1) → Mode(2) → RuleSets(3) → Routing(4) → Apply(5)
			switch (effectiveStep) {
				case 1: return parsedVpn !== null;
				case 2: return true; // Mode selection always valid
				case 3: return true; // Rule sets are optional
				case 4: return true; // Routing mode always valid
				case 5: return applied;
				default: return false;
			}
		} else {
			// Proxy: VPN(1) → Mode(2) → Apply(3)
			switch (effectiveStep) {
				case 1: return parsedVpn !== null;
				case 2: return true; // Mode selection always valid
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

			// Helper to safely create resources (ignore "already exists" errors)
			async function safeCreate<T>(
				createFn: () => Promise<T>,
				resourceName: string
			): Promise<boolean> {
				try {
					await createFn();
					return true;
				} catch (err: any) {
					const msg = err?.message || String(err);
					if (msg.includes('already exists')) {
						console.log(`${resourceName} already exists, skipping`);
						return true;
					}
					throw err;
				}
			}

			// Create endpoint if present (AWG)
			if (singbox.endpoint) {
				await safeCreate(
					() => api.createEndpoint(singbox.endpoint as any),
					'VPN endpoint'
				);
			}

			// Create VPN outbound (only for VLESS/Hy2, not AWG)
			if (singbox.outbound) {
				await safeCreate(
					() => api.createOutbound(singbox.outbound as any),
					'VPN outbound'
				);
			}

			if (usageMode === 'proxy') {
				// ========== PROXY MODE ==========
				// Create mixed inbound (SOCKS5 + HTTP)
				await safeCreate(
					() => api.createInbound({
						type: 'mixed',
						tag: 'mixed-in',
						listen: '0.0.0.0',
						listen_port: proxyPort,
					}),
					'Mixed inbound'
				);

				// Create direct outbound
				await safeCreate(
					() => api.createOutbound({ type: 'direct', tag: 'direct' }),
					'direct outbound'
				);

				// Route rule: traffic from mixed-in inbound → VPN outbound
				await safeCreate(
					() => api.createRule({
						inbound: ['mixed-in'],
						outbound: vpnTag
					}),
					'proxy route rule'
				);

				// Fallback for any other traffic goes direct
				await api.updateRouteSettings({
					final: 'direct',
					auto_detect_interface: true
				});

			} else {
				// ========== ROUTER MODE ==========
				// Create TUN inbound
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

				// Create base outbounds
				await safeCreate(
					() => api.createOutbound({ type: 'direct', tag: 'direct' }),
					'direct outbound'
				);
				await safeCreate(
					() => api.createOutbound({ type: 'block', tag: 'block' }),
					'block outbound'
				);

				// Create DNS servers
				await safeCreate(
					() => api.createDnsServer({
						tag: 'dns-direct',
						type: 'udp',
						server: '8.8.8.8',
					}),
					'direct DNS server'
				);
				await api.updateDnsSettings({ final: 'dns-direct', strategy: 'prefer_ipv4' });

				// Create rule sets
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

				// Create routing rules
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

			// Configure Clash API (for monitoring)
			await api.updateExperimental({
				clash_api: {
					external_controller: '127.0.0.1:9090',
					default_mode: 'rule'
				}
			});

			// Configure log settings
			await api.updateLogSettings({
				level: 'info',
				timestamp: true
			});

			// Save config to disk
			await api.applyConfig();

			applied = true;
			notifications.success('Конфигурация применена!');
		} catch (err) {
			notifications.error(`Ошибка: ${err}`);
		} finally {
			applying = false;
		}
	}

	function finish() {
		goto('/');
	}

	function getTypeLabel(type: string): string {
		switch (type) {
			case 'vless': return 'VLESS';
			case 'hy2': return 'Hysteria2';
			case 'awg': return 'AmneziaWG';
			default: return type;
		}
	}

	// File upload handling
	let fileInput: HTMLInputElement;
	let isDragging = $state(false);

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files && input.files.length > 0) {
			readFile(input.files[0]);
		}
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		isDragging = false;
		if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
			readFile(event.dataTransfer.files[0]);
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		isDragging = true;
	}

	function handleDragLeave() {
		isDragging = false;
	}

	function readFile(file: File) {
		// Check file extension
		const validExtensions = ['.conf', '.txt', '.json'];
		const hasValidExt = validExtensions.some(ext => file.name.toLowerCase().endsWith(ext));

		if (!hasValidExt && file.size > 50000) {
			vpnError = 'Invalid file type. Please select a .conf, .txt, or .json file.';
			return;
		}

		const reader = new FileReader();
		reader.onload = (e) => {
			const content = e.target?.result as string;
			if (content) {
				vpnInput = content;
				parseVpnConfig();
			}
		};
		reader.onerror = () => {
			vpnError = 'Failed to read file';
		};
		reader.readAsText(file);
	}
</script>

<svelte:head>
	<title>Quick Setup - RouteBox</title>
</svelte:head>

{#if loading}
	<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center">
		<div class="text-center">
			<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)] mx-auto" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
			<p class="mt-2 text-[var(--ctp-overlay1)]">Checking system...</p>
		</div>
	</div>
{:else}
<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center p-6">
	<div class="w-full max-w-2xl">
		<!-- Header -->
		<div class="text-center mb-8">
			<h1 class="text-3xl font-bold text-[var(--ctp-text)]">Quick Setup</h1>
			<p class="text-[var(--ctp-overlay1)] mt-2">Configure your router in a few simple steps</p>
		</div>

		<!-- System Requirements Warning -->
		{#if systemChecks && !systemChecks.all_checks_passed}
			<div class="bg-[var(--ctp-red)] rounded-xl p-4 mb-6">
				<div class="flex items-start gap-3">
					<svg class="w-6 h-6 text-white flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					<div class="flex-1">
						<h3 class="text-white font-semibold">System Requirements Not Met</h3>
						<div class="text-white/90 text-sm mt-1 space-y-1">
							{#if !systemChecks.is_root}
								<p>Run with <code class="bg-white/20 px-1.5 py-0.5 rounded">sudo</code> to create TUN interface</p>
							{/if}
							{#if !systemChecks.ipv4_forward}
								<p>Enable IP forwarding: <code class="bg-white/20 px-1.5 py-0.5 rounded">sysctl -w net.ipv4.ip_forward=1</code></p>
							{/if}
						</div>
					</div>
				</div>
			</div>
		{/if}

		<!-- Progress -->
		<div class="flex items-center justify-center mb-8">
			{#each Array(totalSteps) as _, i}
				<div class="flex items-center">
					<div
						class="w-10 h-10 rounded-full flex items-center justify-center font-medium transition-colors"
						class:bg-[var(--ctp-primary)]={currentStep >= i}
						class:text-white={currentStep >= i}
						class:bg-[var(--ctp-surface1)]={currentStep < i}
						class:text-[var(--ctp-overlay0)]={currentStep < i}
					>
						{#if currentStep > i}
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
							</svg>
						{:else}
							{i + 1}
						{/if}
					</div>
					{#if i < totalSteps - 1}
						<div
							class="w-16 h-1 mx-2 rounded transition-colors"
							class:bg-[var(--ctp-primary)]={currentStep > i}
							class:bg-[var(--ctp-surface1)]={currentStep <= i}
						></div>
					{/if}
				</div>
			{/each}
		</div>

		<!-- Step Content -->
		<div class="bg-[var(--ctp-mantle)] rounded-xl p-8 border border-[var(--ctp-surface0)]">
			{#if currentStep === 0}
				<!-- Step 0: Install amnezia-box (only shown if not installed) -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">1. Install amnezia-box</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							amnezia-box is not installed on your system. Please install it first.
						</p>
					</div>

					<div class="alert-box warning">
						<div class="flex items-center gap-2">
							<svg class="w-5 h-5 text-[var(--ctp-yellow)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
							</svg>
							<span class="font-medium text-[var(--ctp-yellow)]">amnezia-box not found</span>
						</div>
					</div>

					<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
						<p class="text-sm text-[var(--ctp-text)]">Run this command to install amnezia-box:</p>

						<code class="block p-3 bg-[var(--ctp-crust)] rounded text-sm text-[var(--ctp-green)] font-mono break-all">
							curl -fsSL https://raw.githubusercontent.com/hoaxisr/amnezia-box/main/docs/installation/tools/install-amnezia-box.sh | sh
						</code>

						<p class="text-xs text-[var(--ctp-overlay1)]">
							After installation, click "Check Installation" to continue.
						</p>
					</div>

					<button
						onclick={checkInstallation}
						disabled={checkingInstall}
						class="w-full px-6 py-3 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center justify-center gap-2"
					>
						{#if checkingInstall}
							<svg class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
							</svg>
							Checking...
						{:else}
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
							</svg>
							Check Installation
						{/if}
					</button>
				</div>

			{:else if effectiveStep === 1}
				<!-- Step: VPN Config -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{binaryInstalled ? '1' : '2'}. Import VPN Configuration</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							Paste your VPN link, drop a .conf file, or select a file
						</p>
					</div>

					<!-- File drop zone -->
					<div
						class="relative border-2 border-dashed rounded-lg p-4 transition-colors {isDragging ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)]'}"
						ondrop={handleDrop}
						ondragover={handleDragOver}
						ondragleave={handleDragLeave}
						role="button"
						tabindex="0"
					>
						<textarea
							bind:value={vpnInput}
							oninput={parseVpnConfig}
							placeholder="vless://uuid@server:port?params#name
hy2://password@server:port#name
or paste/drop AmneziaWG .conf file..."
							rows="6"
							class="w-full px-4 py-3 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] font-mono text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] resize-none"
						></textarea>

						<!-- File upload button -->
						<div class="mt-3 flex items-center justify-center gap-3">
							<span class="text-sm text-[var(--ctp-overlay1)]">or</span>
							<button
								type="button"
								onclick={() => fileInput.click()}
								class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-2"
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
								</svg>
								Select .conf file
							</button>
							<input
								bind:this={fileInput}
								type="file"
								accept=".conf,.txt,.json"
								onchange={handleFileSelect}
								class="hidden"
							/>
						</div>
					</div>

					{#if vpnError}
						<div class="alert-box error">
							<p class="text-sm text-[var(--ctp-red)]">{vpnError}</p>
						</div>
					{/if}

					{#if parsedVpn}
						<div class="alert-box primary">
							<div class="flex items-center gap-2">
								<svg class="w-5 h-5 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								<span class="font-medium text-[var(--ctp-primary)]">Configuration parsed successfully</span>
							</div>
							<div class="mt-2 text-sm text-[var(--ctp-text)]">
								<span class="text-[var(--ctp-overlay1)]">Type:</span> {getTypeLabel(parsedVpn.type)} —
								<span class="text-[var(--ctp-overlay1)]">Name:</span> {parsedVpn.name}
							</div>
						</div>
					{/if}
				</div>

			{:else if effectiveStep === 2}
				<!-- Step: Usage Mode -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{binaryInstalled ? '2' : '3'}. Режим использования</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							Как вы планируете использовать RouteBox?
						</p>
					</div>

					<div class="space-y-3">
						<button
							onclick={() => usageMode = 'router'}
							class="selectable-card selectable-card-lg {usageMode === 'router' ? 'selected' : ''}"
						>
							<div class="flex items-start gap-4">
								<div
									class="w-6 h-6 rounded-full border-2 flex items-center justify-center mt-0.5"
									class:border-[var(--ctp-primary)]={usageMode === 'router'}
									class:border-[var(--ctp-overlay0)]={usageMode !== 'router'}
								>
									{#if usageMode === 'router'}
										<div class="w-3 h-3 rounded-full bg-[var(--ctp-primary)]"></div>
									{/if}
								</div>
								<div>
									<div class="font-semibold text-[var(--ctp-text)]">Роутер</div>
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										Маршрутизация трафика всей локальной сети через VPN.
										Требует настройки других устройств на использование этого роутера как шлюза.
									</div>
									<div class="mt-2 flex flex-wrap gap-2">
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">TUN интерфейс</span>
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">Split tunneling</span>
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">Rule sets</span>
									</div>
								</div>
							</div>
						</button>

						<button
							onclick={() => usageMode = 'proxy'}
							class="selectable-card selectable-card-lg {usageMode === 'proxy' ? 'selected' : ''}"
						>
							<div class="flex items-start gap-4">
								<div
									class="w-6 h-6 rounded-full border-2 flex items-center justify-center mt-0.5"
									class:border-[var(--ctp-primary)]={usageMode === 'proxy'}
									class:border-[var(--ctp-overlay0)]={usageMode !== 'proxy'}
								>
									{#if usageMode === 'proxy'}
										<div class="w-3 h-3 rounded-full bg-[var(--ctp-primary)]"></div>
									{/if}
								</div>
								<div>
									<div class="font-semibold text-[var(--ctp-text)]">Прокси</div>
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										Локальный SOCKS5/HTTP прокси. Только приложения, настроенные на прокси, будут использовать VPN.
									</div>
									<div class="mt-2 flex flex-wrap gap-2">
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">SOCKS5</span>
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">HTTP proxy</span>
										<span class="px-2 py-0.5 bg-[var(--ctp-surface1)] text-[var(--ctp-subtext0)] text-xs rounded">Простая настройка</span>
									</div>
								</div>
							</div>
						</button>
					</div>

					{#if usageMode === 'proxy'}
						<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
							<label for="proxy-port" class="block text-sm text-[var(--ctp-overlay1)] mb-2">Порт прокси</label>
							<input
								id="proxy-port"
								type="number"
								bind:value={proxyPort}
								min="1024"
								max="65535"
								class="w-32 px-3 py-2 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
							/>
							<p class="text-xs text-[var(--ctp-overlay0)] mt-2">
								Прокси будет доступен на <code class="bg-[var(--ctp-surface1)] px-1.5 py-0.5 rounded text-[var(--ctp-text)]">{machineIP}:{proxyPort}</code>
							</p>
						</div>
					{/if}
				</div>

			{:else if effectiveStep === 3 && usageMode === 'router'}
				<!-- Step: Rule Sets (Router only) -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{binaryInstalled ? '3' : '4'}. Списки правил</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							Какой трафик должен идти через VPN
						</p>
					</div>

					<div class="space-y-3">
						{#each availableRuleSets as rs}
							<button
								onclick={() => toggleRuleSet(rs.id)}
								class="selectable-card {selectedRuleSets.includes(rs.id) ? 'selected' : ''}"
							>
								<div class="flex items-center gap-3">
									<div
										class="w-5 h-5 rounded border-2 flex items-center justify-center transition-colors"
										class:border-[var(--ctp-primary)]={selectedRuleSets.includes(rs.id)}
										class:bg-[var(--ctp-primary)]={selectedRuleSets.includes(rs.id)}
										class:border-[var(--ctp-overlay0)]={!selectedRuleSets.includes(rs.id)}
									>
										{#if selectedRuleSets.includes(rs.id)}
											<svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
												<path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
											</svg>
										{/if}
									</div>
									<div>
										<div class="font-medium text-[var(--ctp-text)]">{rs.name}</div>
										<div class="text-sm text-[var(--ctp-overlay1)]">{rs.desc}</div>
									</div>
								</div>
							</button>
						{/each}
					</div>
				</div>

			{:else if effectiveStep === 4 && usageMode === 'router'}
				<!-- Step: Routing Mode (Router only) -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">{binaryInstalled ? '4' : '5'}. Режим маршрутизации</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							Какой трафик направлять через VPN
						</p>
					</div>

					<div class="space-y-3">
						<button
							onclick={() => routingMode = 'split'}
							class="selectable-card selectable-card-lg {routingMode === 'split' ? 'selected' : ''}"
						>
							<div class="flex items-start gap-4">
								<div
									class="w-6 h-6 rounded-full border-2 flex items-center justify-center mt-0.5"
									class:border-[var(--ctp-primary)]={routingMode === 'split'}
									class:border-[var(--ctp-overlay0)]={routingMode !== 'split'}
								>
									{#if routingMode === 'split'}
										<div class="w-3 h-3 rounded-full bg-[var(--ctp-primary)]"></div>
									{/if}
								</div>
								<div>
									<div class="font-semibold text-[var(--ctp-text)]">Split Tunneling (рекомендуется)</div>
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										Только выбранные списки идут через VPN. Остальной трафик напрямую.
									</div>
								</div>
							</div>
						</button>

						<button
							onclick={() => routingMode = 'all'}
							class="selectable-card selectable-card-lg {routingMode === 'all' ? 'selected' : ''}"
						>
							<div class="flex items-start gap-4">
								<div
									class="w-6 h-6 rounded-full border-2 flex items-center justify-center mt-0.5"
									class:border-[var(--ctp-primary)]={routingMode === 'all'}
									class:border-[var(--ctp-overlay0)]={routingMode !== 'all'}
								>
									{#if routingMode === 'all'}
										<div class="w-3 h-3 rounded-full bg-[var(--ctp-primary)]"></div>
									{/if}
								</div>
								<div>
									<div class="font-semibold text-[var(--ctp-text)]">Весь трафик</div>
									<div class="text-sm text-[var(--ctp-overlay1)] mt-1">
										Весь трафик через VPN. Максимальная приватность, но может снизить скорость.
									</div>
								</div>
							</div>
						</button>
					</div>
				</div>

			{:else if (effectiveStep === 5 && usageMode === 'router') || (effectiveStep === 3 && usageMode === 'proxy')}
				<!-- Step: Apply -->
				<div class="space-y-6">
					<div>
						<h2 class="text-xl font-semibold text-[var(--ctp-text)]">
							{#if usageMode === 'router'}
								{binaryInstalled ? '5' : '6'}. Применить конфигурацию
							{:else}
								{binaryInstalled ? '3' : '4'}. Применить конфигурацию
							{/if}
						</h2>
						<p class="text-[var(--ctp-overlay1)] mt-1">
							Проверьте настройки и примените конфигурацию
						</p>
					</div>

					{#if !applied}
						<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-3 text-sm">
							<div class="flex justify-between">
								<span class="text-[var(--ctp-overlay1)]">VPN:</span>
								<span class="text-[var(--ctp-text)]">{parsedVpn?.name} ({getTypeLabel(parsedVpn?.type || '')})</span>
							</div>
							<div class="flex justify-between">
								<span class="text-[var(--ctp-overlay1)]">Режим:</span>
								<span class="text-[var(--ctp-text)]">{usageMode === 'router' ? 'Роутер' : 'Прокси'}</span>
							</div>
							{#if usageMode === 'router'}
								<div class="flex justify-between">
									<span class="text-[var(--ctp-overlay1)]">Списки правил:</span>
									<span class="text-[var(--ctp-text)]">{selectedRuleSets.length > 0 ? selectedRuleSets.join(', ') : 'Нет'}</span>
								</div>
								<div class="flex justify-between">
									<span class="text-[var(--ctp-overlay1)]">Маршрутизация:</span>
									<span class="text-[var(--ctp-text)]">{routingMode === 'split' ? 'Split Tunneling' : 'Весь трафик'}</span>
								</div>
							{:else}
								<div class="flex justify-between">
									<span class="text-[var(--ctp-overlay1)]">Адрес:</span>
									<span class="text-[var(--ctp-text)]">{machineIP}:{proxyPort} (SOCKS5 + HTTP)</span>
								</div>
							{/if}
						</div>

						<button
							onclick={applyConfiguration}
							disabled={applying}
							class="w-full px-6 py-4 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity flex items-center justify-center gap-2 text-lg font-medium"
						>
							{#if applying}
								<svg class="w-6 h-6 animate-spin" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
								</svg>
								Applying...
							{:else}
								<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
								Apply Configuration
							{/if}
						</button>
					{:else}
						<div class="text-center py-8">
							<div class="w-20 h-20 mx-auto mb-4 rounded-full flex items-center justify-center" style="background-color: color-mix(in srgb, var(--ctp-primary) 20%, transparent);">
								<svg class="w-10 h-10 text-[var(--ctp-primary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
								</svg>
							</div>
							<h3 class="text-xl font-semibold text-[var(--ctp-text)]">Setup Complete!</h3>
							<p class="text-[var(--ctp-overlay1)] mt-2">
								Your router is now configured and ready to use.
							</p>
						</div>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Navigation -->
		<div class="flex justify-between mt-6">
			<button
				onclick={prevStep}
				disabled={currentStep === 0 || (currentStep === 1 && binaryInstalled)}
				class="px-6 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] disabled:opacity-50 transition-colors"
			>
				Back
			</button>

			{#if (usageMode === 'router' && effectiveStep === 5 && applied) || (usageMode === 'proxy' && effectiveStep === 3 && applied)}
				<button
					onclick={finish}
					class="px-6 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity"
				>
					Перейти в Dashboard
				</button>
			{:else if currentStep > 0 && ((usageMode === 'router' && effectiveStep < 5) || (usageMode === 'proxy' && effectiveStep < 3))}
				<button
					onclick={nextStep}
					disabled={!canProceed()}
					class="px-6 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity"
				>
					Далее
				</button>
			{/if}
		</div>

		<!-- Skip link -->
		<div class="text-center mt-4">
			<a href="/" class="text-sm text-[var(--ctp-overlay0)] hover:text-[var(--ctp-text)] transition-colors">
				Skip setup and go to dashboard
			</a>
		</div>
	</div>
</div>
{/if}

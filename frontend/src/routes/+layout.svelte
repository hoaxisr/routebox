<script lang="ts">
	import '../app.css';
	import { onMount, onDestroy, type Snippet } from 'svelte';
	import { page } from '$app/stores';
	import { theme, notifications, loadVersion, speedUnit, loadClientNames, routerMode, panelMode, loadMode } from '$lib/stores';
	import { api } from '$lib/api/client';
	import UnsavedChangesBar from '$lib/components/shared/UnsavedChangesBar.svelte';
	import { t, isLoading as i18nLoading } from 'svelte-i18n';
	import { initI18n } from '$lib/i18n';
	import { goto } from '$app/navigation';

	// Initialize i18n
	initI18n();

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	// Subscribe to theme for reactivity
	let currentTheme = $derived($theme);

	// Login route detection
	let isLogin = $derived($page.url.pathname.startsWith('/login'));

	// Auth state (for hiding logout when auth is disabled)
	let authEnabled = $state(false);

	// Mobile sidebar state
	let sidebarOpen = $state(false);
	let isMobile = $state(false);

	// Collapsible nav groups
	const networkPaths = ['/config/endpoints', '/config/outbounds', '/config/inbounds', '/config/dns'];
	const routingPaths = ['/config/rule-sets', '/config/domains', '/config/routes'];

	let networkExpanded = $state(false);
	let routingExpanded = $state(false);

	// Updates badge (dot when any update available)
	let updateAvailable = $state(false);

	// Auto-expand based on current path
	$effect(() => {
		const path = $page.url.pathname;
		if (networkPaths.some(p => path.startsWith(p))) {
			networkExpanded = true;
		}
		if (routingPaths.some(p => path.startsWith(p))) {
			routingExpanded = true;
		}
	});

	function checkMobile() {
		isMobile = window.innerWidth < 768;
		if (!isMobile) sidebarOpen = false;
	}

	function toggleSidebar() {
		sidebarOpen = !sidebarOpen;
	}

	function closeSidebar() {
		sidebarOpen = false;
	}

	// Close sidebar when navigating on mobile
	function handleNavClick() {
		if (isMobile) closeSidebar();
	}

	onMount(async () => {
		// Check mobile on mount and window resize
		checkMobile();
		window.addEventListener('resize', checkMobile);

		// Load version/feature flags in background
		loadVersion();
		loadClientNames();

		// Load operating mode for nav/redirect gating (fail-safe: stays router until an explicit vps read).
		loadMode();

		// Load speed unit preference
		api.getSettings().then(res => {
			speedUnit.set(res.settings.ui.speed_unit);
		}).catch(() => {});

		// Load updates badge in background (non-blocking)
		api.getUpdatesStatus().then((s) => {
			updateAvailable = s.targets.some((target) => target.update_available);
		}).catch(() => {});

		// Check whether auth is enabled (public endpoint, won't 401-loop)
		try { authEnabled = (await api.getSession()).auth_enabled; } catch { authEnabled = false; }
	});

	onDestroy(() => {
		if (typeof window !== 'undefined') {
			window.removeEventListener('resize', checkMobile);
		}
	});

	async function logout() {
		try {
			await api.logout();
		} catch {
			// ignore errors — navigate to login regardless
		}
		goto('/login');
	}
</script>

{#if $i18nLoading}
	<!-- Wait for i18n to load before rendering anything with translations -->
	<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center">
		<div class="text-center">
			<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)] mx-auto" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
		</div>
	</div>
{:else if isLogin}
	<!-- Login page: full-screen, no chrome -->
	{@render children()}
{:else}
	<!-- Normal layout with sidebar/header -->
	<div class="min-h-screen bg-[var(--ctp-base)] text-[var(--ctp-text)]">
		<!-- Header -->
		<header class="fixed top-0 left-0 right-0 h-14 bg-[var(--ctp-mantle)] border-b border-[var(--ctp-surface0)] z-50">
			<div class="h-full px-4 flex items-center justify-between">
				<div class="flex items-center gap-3">
					<!-- Mobile hamburger menu -->
					{#if isMobile}
						<button
							onclick={toggleSidebar}
							class="p-2.5 -ml-2.5 rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors"
							aria-label="Toggle menu"
						>
							<svg class="w-6 h-6 text-[var(--ctp-text)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								{#if sidebarOpen}
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								{:else}
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
								{/if}
							</svg>
						</button>
					{/if}
					<img src="/logo-v2.svg" alt="RouteBox" class="w-8 h-8" />
					<span class="text-xl font-semibold text-[var(--ctp-text)]">RouteBox</span>
				</div>

				<div class="flex items-center gap-4">
					<!-- App Settings -->
					<a
						href="/config/app"
						class="p-2 rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors"
						title={$t('nav.appSettings')}
					>
						<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
						</svg>
					</a>
					<!-- Theme toggle -->
					<button
						onclick={() => theme.toggle()}
						class="p-2 rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors"
						title={currentTheme === 'dark' ? $t('theme.light') : $t('theme.dark')}
					>
						{#if currentTheme === 'dark'}
							<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="currentColor" viewBox="0 0 20 20">
								<path fill-rule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clip-rule="evenodd" />
							</svg>
						{:else}
							<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="currentColor" viewBox="0 0 20 20">
								<path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
							</svg>
						{/if}
					</button>
					<!-- Logout (hidden when auth is disabled) -->
					{#if authEnabled}
						<button
							onclick={logout}
							class="p-2 rounded-lg hover:bg-[var(--ctp-surface0)] transition-colors"
							title={$t('auth.logout')}
						>
							<svg class="w-5 h-5 text-[var(--ctp-subtext1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
							</svg>
						</button>
					{/if}
				</div>
			</div>
		</header>

		<!-- Sidebar Overlay (mobile only) -->
		{#if isMobile && sidebarOpen}
			<button
				class="fixed inset-0 bg-black/50 z-30 sidebar-overlay"
				onclick={closeSidebar}
				aria-label="Close sidebar"
			></button>
		{/if}

		<!-- Sidebar -->
		<aside
			class="fixed top-14 left-0 bottom-0 w-56 bg-[var(--ctp-mantle)] border-r border-[var(--ctp-surface0)] z-40
				{isMobile ? 'sidebar-mobile shadow-xl' : ''}
				{isMobile && !sidebarOpen ? 'closed' : ''}"
		>
			<nav class="p-4 space-y-1 overflow-y-auto h-full">
				<!-- Dashboard -->
				<a href="/" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
					</svg>
					{$t('nav.dashboard')}
				</a>

				<!-- Config Section -->
				<div class="pt-4 pb-2">
					<span class="px-3 text-xs font-medium text-[var(--ctp-overlay1)] uppercase tracking-wider">{$t('nav.config')}</span>
				</div>

				<!-- Overview -->
				<a href="/config" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" />
					</svg>
					{$t('nav.overview')}
				</a>

				<!-- Network Group -->
				<div>
					<button
						onclick={() => networkExpanded = !networkExpanded}
						class="w-full flex items-center justify-between px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors"
					>
						<div class="flex items-center gap-3">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
							</svg>
							{$t('nav.network')}
						</div>
						<svg class="w-4 h-4 transition-transform {networkExpanded ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
						</svg>
					</button>
					{#if networkExpanded}
						<div class="ml-4 mt-1 space-y-1 border-l border-[var(--ctp-surface1)] pl-3">
							<a href="/config/endpoints" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.endpoints')}
							</a>
							<a href="/config/outbounds" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.outbounds')}
							</a>
							<a href="/config/inbounds" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.inbounds')}
							</a>
							<a href="/config/dns" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.dns')}
							</a>
						</div>
					{/if}
				</div>

				<!-- Routing Group -->
				<div>
					<button
						onclick={() => routingExpanded = !routingExpanded}
						class="w-full flex items-center justify-between px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors"
					>
						<div class="flex items-center gap-3">
							<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
							</svg>
							{$t('nav.routing')}
						</div>
						<svg class="w-4 h-4 transition-transform {routingExpanded ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
						</svg>
					</button>
					{#if routingExpanded}
						<div class="ml-4 mt-1 space-y-1 border-l border-[var(--ctp-surface1)] pl-3">
							<a href="/config/rule-sets" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.ruleSets')}
							</a>
							<a href="/config/domains" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.domains')}
							</a>
							<a href="/config/routes" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-1.5 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors text-sm">
								{$t('nav.routes')}
							</a>
						</div>
					{/if}
				</div>

				<!-- Experimental -->
				<a href="/config/settings" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
					</svg>
					{$t('nav.experimental')}
				</a>

				<!-- Clients (router-only) -->
				{#if $routerMode}
					<a href="/config/clients" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
						</svg>
						{$t('nav.clients')}
					</a>
				{/if}

				<!-- Users (panel-only) -->
				{#if $panelMode}
					<a href="/config/users" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-1.13a4 4 0 10-4-4 4 4 0 004 4z" />
						</svg>
						{$t('nav.users')}
					</a>
				{/if}

				<!-- Subscriptions (router-only) -->
				{#if $routerMode}
					<a href="/config/subscriptions" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v3m-4 0h8M5 8h14" />
						</svg>
						{$t('nav.subscriptions')}
					</a>
				{/if}

				<!-- Updates -->
				<a href="/config/updates" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
					</svg>
					<span class="flex items-center gap-2">
						{$t('nav.updates')}
						{#if updateAvailable}
							<span class="w-2 h-2 rounded-full bg-[var(--ctp-primary)]"></span>
						{/if}
					</span>
				</a>

				<!-- Monitor Section -->
				<div class="pt-4 pb-2">
					<span class="px-3 text-xs font-medium text-[var(--ctp-overlay1)] uppercase tracking-wider">{$t('nav.monitor')}</span>
				</div>

				{#if $routerMode}
					<a href="/monitor/traffic" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
						</svg>
						{$t('nav.traffic')}
					</a>
				{/if}

				<a href="/monitor/logs" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
					</svg>
					{$t('nav.logs')}
				</a>

				<a href="/monitor/connections" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
					</svg>
					{$t('nav.connections')}
				</a>

				{#if $routerMode}
					<a href="/monitor/breakdown" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 3.055A9.001 9.001 0 1020.945 13H11V3.055z" />
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.488 9H15V3.512A9.025 9.025 0 0120.488 9z" />
						</svg>
						{$t('nav.breakdown')}
					</a>

					<a href="/monitor/proxies" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
						</svg>
						{$t('nav.proxies')}
					</a>

					<a href="/monitor/route-inspector" onclick={handleNavClick} class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
						</svg>
						{$t('nav.routeInspector')}
					</a>
				{/if}
			</nav>
		</aside>

		<!-- Main content (with HTTP warning banner in normal flow) -->
		<main class="pt-14 {isMobile ? 'pl-0' : 'pl-56'}">
			<!-- HTTP warning banner (non-localhost HTTP, in normal flow to push content down) -->
			{#if typeof window !== 'undefined' && window.location.protocol === 'http:' && window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1'}
				<div class="bg-[var(--ctp-red)] text-white text-xs text-center py-1 px-2">
					{$t('auth.insecureWarning')}
				</div>
			{/if}
			<div class="p-4 md:p-6">
				{@render children()}
			</div>
		</main>
	</div>
{/if}

<!-- Unsaved changes bar -->
<UnsavedChangesBar />

<!-- Toast notifications (always visible, positioned above unsaved bar) -->
{#if $notifications.length > 0}
	<div class="fixed bottom-20 right-4 z-50 space-y-2">
		{#each $notifications as toast (toast.id)}
			<div
				class="toast px-4 py-3 rounded-lg shadow-lg max-w-sm flex items-start gap-3 text-white"
				class:bg-[var(--ctp-primary)]={toast.type === 'success' || toast.type === 'info'}
				class:bg-[var(--ctp-red)]={toast.type === 'error'}
				class:bg-[var(--ctp-surface2)]={toast.type === 'warning'}
			>
				<span class="flex-1">{toast.message}</span>
				<button onclick={() => notifications.remove(toast.id)} class="opacity-70 hover:opacity-100" aria-label="Close">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
		{/each}
	</div>
{/if}

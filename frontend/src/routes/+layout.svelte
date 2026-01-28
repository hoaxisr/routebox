<script lang="ts">
	import '../app.css';
	import { onMount, type Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { theme, notifications, loadVersion } from '$lib/stores';
	import { api } from '$lib/api/client';
	import UnsavedChangesBar from '$lib/components/shared/UnsavedChangesBar.svelte';
	import { t, isLoading as i18nLoading } from 'svelte-i18n';
	import { initI18n } from '$lib/i18n';

	// Initialize i18n
	initI18n();

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	// Subscribe to theme for reactivity
	let currentTheme = $derived($theme);

	// Check if we're on the setup page
	let isSetupPage = $derived($page.url.pathname === '/setup');

	// Check if setup is needed (skip check if already on /setup page)
	let setupChecked = $state(false);
	let needsSetup = $state(false);

	onMount(async () => {
		// Load version/feature flags in background
		loadVersion();

		// Skip check if already on setup page
		if ($page.url.pathname === '/setup') {
			setupChecked = true;
			return;
		}

		try {
			const result = await api.needsSetup();
			needsSetup = result.needs_setup;
			if (result.needs_setup) {
				goto('/setup');
			}
		} catch (e) {
			console.error('Failed to check setup status:', e);
		} finally {
			setupChecked = true;
		}
	});
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
{:else if isSetupPage}
	<!-- Setup page: minimal layout without sidebar/header -->
	<div class="min-h-screen bg-[var(--ctp-base)] text-[var(--ctp-text)]">
		{@render children()}
	</div>
{:else if !setupChecked}
	<!-- Loading state while checking setup -->
	<div class="min-h-screen bg-[var(--ctp-base)] flex items-center justify-center">
		<div class="text-center">
			<svg class="w-8 h-8 animate-spin text-[var(--ctp-primary)] mx-auto" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
			</svg>
			<p class="mt-2 text-[var(--ctp-overlay1)]">{$t('app.loading')}</p>
		</div>
	</div>
{:else}
	<!-- Normal layout with sidebar/header -->
	<div class="min-h-screen bg-[var(--ctp-base)] text-[var(--ctp-text)]">
		<!-- Header -->
		<header class="fixed top-0 left-0 right-0 h-14 bg-[var(--ctp-mantle)] border-b border-[var(--ctp-surface0)] z-50">
			<div class="h-full px-4 flex items-center justify-between">
				<div class="flex items-center gap-3">
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
				</div>
			</div>
		</header>

		<!-- Sidebar -->
		<aside class="fixed top-14 left-0 bottom-0 w-56 bg-[var(--ctp-mantle)] border-r border-[var(--ctp-surface0)]">
			<nav class="p-4 space-y-1">
				<a href="/" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
					</svg>
					{$t('nav.dashboard')}
				</a>

				<div class="pt-4 pb-2">
					<span class="px-3 text-xs font-medium text-[var(--ctp-overlay1)] uppercase tracking-wider">{$t('nav.config')}</span>
				</div>

				<a href="/config" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" />
					</svg>
					{$t('nav.overview')}
				</a>

				<a href="/config/endpoints" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
					</svg>
					{$t('nav.endpoints')}
				</a>

				<a href="/config/outbounds" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
					</svg>
					{$t('nav.outbounds')}
				</a>

				<a href="/config/inbounds" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 16l-4-4m0 0l4-4m-4 4h14m-5 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h7a3 3 0 013 3v1" />
					</svg>
					{$t('nav.inbounds')}
				</a>

				<a href="/config/rule-sets" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
					</svg>
					{$t('nav.ruleSets')}
				</a>

				<a href="/config/domains" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
					{$t('nav.domains')}
				</a>

				<a href="/config/routes" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
					</svg>
					{$t('nav.routes')}
				</a>

				<a href="/config/dns" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
					</svg>
					{$t('nav.dns')}
				</a>

				<a href="/config/settings" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
					</svg>
					{$t('nav.experimental')}
				</a>

				<div class="pt-4 pb-2">
					<span class="px-3 text-xs font-medium text-[var(--ctp-overlay1)] uppercase tracking-wider">{$t('nav.monitor')}</span>
				</div>

				<a href="/monitor/traffic" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
					</svg>
					{$t('nav.traffic')}
				</a>

				<a href="/monitor/logs" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
					</svg>
					{$t('nav.logs')}
				</a>

				<a href="/monitor/connections" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
					</svg>
					{$t('nav.connections')}
				</a>

				<a href="/monitor/proxies" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
					</svg>
					{$t('nav.proxies')}
				</a>

				<a href="/monitor/route-inspector" class="flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-[var(--ctp-surface0)] text-[var(--ctp-subtext1)] hover:text-[var(--ctp-text)] transition-colors">
					<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
					</svg>
					{$t('nav.routeInspector')}
				</a>
			</nav>
		</aside>

		<!-- Main content -->
		<main class="pt-14 pl-56">
			<div class="p-6">
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
				class="px-4 py-3 rounded-lg shadow-lg max-w-sm flex items-start gap-3 text-white"
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

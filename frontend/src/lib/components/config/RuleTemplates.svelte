<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { RouteRule, RuleSet, Outbound } from '$lib/types';

	interface Props {
		outbounds: Outbound[];
		existingRuleSetTags?: string[];
		onSelect: (ruleSet: RuleSet, rule: RouteRule) => void;
		onClose: () => void;
	}

	let { outbounds, existingRuleSetTags = [], onSelect, onClose }: Props = $props();

	// Selected outbound for templates that need one
	let selectedOutbound = $state(outbounds.find(o => o.type !== 'direct' && o.type !== 'block')?.tag ?? outbounds[0]?.tag ?? '');

	const GEOSITE_BASE = 'https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set';

	interface Template {
		id: string;
		nameKey: string;      // i18n key for name
		descKey: string;      // i18n key for description
		icon: string;
		category: 'services' | 'social' | 'streaming' | 'gaming' | 'regional' | 'ads';
		geosite: string;      // geosite category name
		action: 'route' | 'reject';
		customUrl?: string;   // custom URL for non-standard sources
	}

	const templates: Template[] = [
		// Services
		{ id: 'google', nameKey: 'templates.google', descKey: 'templates.googleDesc', icon: '🔍', category: 'services', geosite: 'google', action: 'route' },
		{ id: 'github', nameKey: 'templates.github', descKey: 'templates.githubDesc', icon: '🐙', category: 'services', geosite: 'github', action: 'route' },
		{ id: 'openai', nameKey: 'templates.openai', descKey: 'templates.openaiDesc', icon: '🤖', category: 'services', geosite: 'openai', action: 'route' },
		{ id: 'amazon', nameKey: 'templates.amazon', descKey: 'templates.amazonDesc', icon: '📦', category: 'services', geosite: 'amazon', action: 'route' },
		{ id: 'microsoft', nameKey: 'templates.microsoft', descKey: 'templates.microsoftDesc', icon: '🪟', category: 'services', geosite: 'microsoft', action: 'route' },
		{ id: 'apple', nameKey: 'templates.apple', descKey: 'templates.appleDesc', icon: '🍎', category: 'services', geosite: 'apple', action: 'route' },

		// Social
		{ id: 'telegram', nameKey: 'templates.telegram', descKey: 'templates.telegramDesc', icon: '✈️', category: 'social', geosite: 'telegram', action: 'route' },
		{ id: 'facebook', nameKey: 'templates.facebook', descKey: 'templates.facebookDesc', icon: '👤', category: 'social', geosite: 'facebook', action: 'route' },
		{ id: 'instagram', nameKey: 'templates.instagram', descKey: 'templates.instagramDesc', icon: '📷', category: 'social', geosite: 'instagram', action: 'route' },
		{ id: 'twitter', nameKey: 'templates.twitter', descKey: 'templates.twitterDesc', icon: '🐦', category: 'social', geosite: 'twitter', action: 'route' },
		{ id: 'tiktok', nameKey: 'templates.tiktok', descKey: 'templates.tiktokDesc', icon: '🎵', category: 'social', geosite: 'tiktok', action: 'route' },
		{ id: 'discord', nameKey: 'templates.discord', descKey: 'templates.discordDesc', icon: '💬', category: 'social', geosite: 'discord', action: 'route' },
		{ id: 'whatsapp', nameKey: 'templates.whatsapp', descKey: 'templates.whatsappDesc', icon: '📱', category: 'social', geosite: 'whatsapp', action: 'route' },
		{ id: 'linkedin', nameKey: 'templates.linkedin', descKey: 'templates.linkedinDesc', icon: '💼', category: 'social', geosite: 'linkedin', action: 'route' },

		// Streaming
		{ id: 'youtube', nameKey: 'templates.youtube', descKey: 'templates.youtubeDesc', icon: '▶️', category: 'streaming', geosite: 'youtube', action: 'route' },
		{ id: 'netflix', nameKey: 'templates.netflix', descKey: 'templates.netflixDesc', icon: '🎬', category: 'streaming', geosite: 'netflix', action: 'route' },
		{ id: 'spotify', nameKey: 'templates.spotify', descKey: 'templates.spotifyDesc', icon: '🎧', category: 'streaming', geosite: 'spotify', action: 'route' },
		{ id: 'twitch', nameKey: 'templates.twitch', descKey: 'templates.twitchDesc', icon: '📺', category: 'streaming', geosite: 'twitch', action: 'route' },
		{ id: 'disney', nameKey: 'templates.disney', descKey: 'templates.disneyDesc', icon: '🏰', category: 'streaming', geosite: 'disney', action: 'route' },
		{ id: 'primevideo', nameKey: 'templates.primevideo', descKey: 'templates.primevideoDesc', icon: '📽️', category: 'streaming', geosite: 'primevideo', action: 'route' },

		// Gaming
		{ id: 'steam', nameKey: 'templates.steam', descKey: 'templates.steamDesc', icon: '🎮', category: 'gaming', geosite: 'steam', action: 'route' },
		{ id: 'epicgames', nameKey: 'templates.epicgames', descKey: 'templates.epicgamesDesc', icon: '🎯', category: 'gaming', geosite: 'epicgames', action: 'route' },
		{ id: 'playstation', nameKey: 'templates.playstation', descKey: 'templates.playstationDesc', icon: '🕹️', category: 'gaming', geosite: 'playstation', action: 'route' },
		{ id: 'xbox', nameKey: 'templates.xbox', descKey: 'templates.xboxDesc', icon: '🎲', category: 'gaming', geosite: 'xbox', action: 'route' },
		{ id: 'nintendo', nameKey: 'templates.nintendo', descKey: 'templates.nintendoDesc', icon: '🍄', category: 'gaming', geosite: 'nintendo', action: 'route' },

		// Regional
		{ id: 'category-ru', nameKey: 'templates.categoryRu', descKey: 'templates.categoryRuDesc', icon: '🇷🇺', category: 'regional', geosite: 'category-ru', action: 'route' },
		{ id: 'category-ua', nameKey: 'templates.categoryUa', descKey: 'templates.categoryUaDesc', icon: '🇺🇦', category: 'regional', geosite: 'category-ua', action: 'route' },

		// Ads blocking
		{ id: 'ads', nameKey: 'templates.ads', descKey: 'templates.adsDesc', icon: '🚫', category: 'ads', geosite: 'category-ads-all', action: 'reject' }
	];

	const categories = [
		{ id: 'regional', labelKey: 'routes.categories.regional', descKey: 'routes.categories.regionalDesc' },
		{ id: 'services', labelKey: 'routes.categories.services', descKey: 'routes.categories.servicesDesc' },
		{ id: 'social', labelKey: 'routes.categories.social', descKey: 'routes.categories.socialDesc' },
		{ id: 'streaming', labelKey: 'routes.categories.streaming', descKey: 'routes.categories.streamingDesc' },
		{ id: 'gaming', labelKey: 'routes.categories.gaming', descKey: 'routes.categories.gamingDesc' },
		{ id: 'ads', labelKey: 'routes.categories.ads', descKey: 'routes.categories.adsDesc' }
	] as const;

	function isAlreadyAdded(template: Template): boolean {
		return existingRuleSetTags.includes(template.id);
	}

	function handleSelect(template: Template) {
		const ruleSet: RuleSet = {
			tag: template.id,
			type: 'remote',
			format: 'binary',
			url: `${GEOSITE_BASE}/geosite-${template.geosite}.srs`,
			download_detour: selectedOutbound,
			update_interval: '24h'
		};

		const rule: RouteRule = {
			rule_set: [template.id]
		};

		if (template.action === 'reject') {
			rule.action = 'reject';
		} else {
			rule.outbound = selectedOutbound;
		}

		onSelect(ruleSet, rule);
	}
</script>

<div class="space-y-4">
	<!-- Outbound selector -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<label for="template-outbound" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2">
			{$t('routes.routeThrough')}
		</label>
		<select
			id="template-outbound"
			bind:value={selectedOutbound}
			class="w-full px-3 py-2 bg-[var(--ctp-base)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		>
			{#each outbounds as ob}
				<option value={ob.tag}>{ob.tag} ({ob.type})</option>
			{/each}
		</select>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">
			{$t('routes.ruleSetsSource')} <a href="https://github.com/SagerNet/sing-geosite" target="_blank" rel="noopener" class="text-[var(--ctp-primary)] hover:underline">SagerNet/sing-geosite</a>
		</p>
	</div>

	<!-- Templates by category -->
	{#each categories as category}
		<div>
			<div class="flex items-center gap-2 mb-2">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t(category.labelKey)}</h3>
				<span class="text-xs text-[var(--ctp-overlay0)]">— {$t(category.descKey)}</span>
			</div>
			<div class="grid gap-2">
				{#each templates.filter(t => t.category === category.id) as template}
					{@const alreadyAdded = isAlreadyAdded(template)}
					<button
						type="button"
						onclick={() => handleSelect(template)}
						disabled={alreadyAdded}
						class="w-full p-3 bg-[var(--ctp-surface0)] hover:bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg text-left transition-colors group disabled:opacity-50 disabled:cursor-not-allowed"
					>
						<div class="flex items-start gap-3">
							<span class="text-xl">{template.icon}</span>
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2">
									<span class="font-medium text-[var(--ctp-text)]">{$t(template.nameKey)}</span>
									{#if alreadyAdded}
										<span class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.alreadyAdded')}</span>
									{:else if template.action === 'reject'}
										<span class="text-xs px-1.5 py-0.5 rounded bg-[var(--ctp-red)] bg-opacity-20 text-[var(--ctp-red)]">{$t('routes.block')}</span>
									{:else}
										<span class="text-xs text-[var(--ctp-overlay0)]">→ {selectedOutbound}</span>
									{/if}
								</div>
								<p class="text-sm text-[var(--ctp-overlay1)] mt-0.5">{$t(template.descKey)}</p>
							</div>
							{#if !alreadyAdded}
								<svg class="w-5 h-5 text-[var(--ctp-overlay0)] group-hover:text-[var(--ctp-primary)] transition-colors flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
								</svg>
							{/if}
						</div>
					</button>
				{/each}
			</div>
		</div>
	{/each}

	<!-- Close button -->
	<div class="flex justify-end pt-2 border-t border-[var(--ctp-surface2)]">
		<button
			type="button"
			onclick={onClose}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
		>
			{$t('common.close')}
		</button>
	</div>
</div>

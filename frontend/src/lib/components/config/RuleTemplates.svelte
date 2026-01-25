<script lang="ts">
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
		name: string;
		description: string;
		icon: string;
		category: 'services' | 'social' | 'streaming' | 'ads';
		geosite: string; // geosite category name
		action: 'route' | 'reject';
	}

	const templates: Template[] = [
		// Services
		{
			id: 'google',
			name: 'Google',
			description: 'Google Search, Maps, Gmail, Drive, etc.',
			icon: '🔍',
			category: 'services',
			geosite: 'google',
			action: 'route'
		},
		{
			id: 'github',
			name: 'GitHub',
			description: 'GitHub repositories, gists, actions',
			icon: '🐙',
			category: 'services',
			geosite: 'github',
			action: 'route'
		},
		{
			id: 'openai',
			name: 'OpenAI / ChatGPT',
			description: 'ChatGPT, OpenAI API, DALL-E',
			icon: '🤖',
			category: 'services',
			geosite: 'openai',
			action: 'route'
		},
		{
			id: 'amazon',
			name: 'Amazon',
			description: 'Amazon shopping, AWS services',
			icon: '📦',
			category: 'services',
			geosite: 'amazon',
			action: 'route'
		},
		// Social
		{
			id: 'telegram',
			name: 'Telegram',
			description: 'Telegram messenger',
			icon: '✈️',
			category: 'social',
			geosite: 'telegram',
			action: 'route'
		},
		{
			id: 'facebook',
			name: 'Facebook',
			description: 'Facebook social network',
			icon: '👤',
			category: 'social',
			geosite: 'facebook',
			action: 'route'
		},
		{
			id: 'instagram',
			name: 'Instagram',
			description: 'Instagram photos and stories',
			icon: '📷',
			category: 'social',
			geosite: 'instagram',
			action: 'route'
		},
		{
			id: 'twitter',
			name: 'Twitter / X',
			description: 'Twitter (X) social network',
			icon: '🐦',
			category: 'social',
			geosite: 'twitter',
			action: 'route'
		},
		{
			id: 'tiktok',
			name: 'TikTok',
			description: 'TikTok short videos',
			icon: '🎵',
			category: 'social',
			geosite: 'tiktok',
			action: 'route'
		},
		{
			id: 'discord',
			name: 'Discord',
			description: 'Discord voice and text chat',
			icon: '🎮',
			category: 'social',
			geosite: 'discord',
			action: 'route'
		},
		{
			id: 'whatsapp',
			name: 'WhatsApp',
			description: 'WhatsApp messenger',
			icon: '💬',
			category: 'social',
			geosite: 'whatsapp',
			action: 'route'
		},
		// Streaming
		{
			id: 'youtube',
			name: 'YouTube',
			description: 'YouTube videos and music',
			icon: '▶️',
			category: 'streaming',
			geosite: 'youtube',
			action: 'route'
		},
		{
			id: 'netflix',
			name: 'Netflix',
			description: 'Netflix streaming',
			icon: '🎬',
			category: 'streaming',
			geosite: 'netflix',
			action: 'route'
		},
		{
			id: 'spotify',
			name: 'Spotify',
			description: 'Spotify music streaming',
			icon: '🎧',
			category: 'streaming',
			geosite: 'spotify',
			action: 'route'
		},
		{
			id: 'twitch',
			name: 'Twitch',
			description: 'Twitch live streaming',
			icon: '📺',
			category: 'streaming',
			geosite: 'twitch',
			action: 'route'
		},
		{
			id: 'disney',
			name: 'Disney+',
			description: 'Disney+ streaming',
			icon: '🏰',
			category: 'streaming',
			geosite: 'disney',
			action: 'route'
		},
		// Ads blocking
		{
			id: 'ads',
			name: 'Block Ads',
			description: 'Block advertising domains',
			icon: '🚫',
			category: 'ads',
			geosite: 'category-ads-all',
			action: 'reject'
		}
	];

	const categories = [
		{ id: 'services', label: 'Services', description: 'Popular web services' },
		{ id: 'social', label: 'Social Media', description: 'Social networks and messengers' },
		{ id: 'streaming', label: 'Streaming', description: 'Video and music streaming' },
		{ id: 'ads', label: 'Ads Blocking', description: 'Block advertising' }
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
			Route through
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
			Rule sets from <a href="https://github.com/SagerNet/sing-geosite" target="_blank" rel="noopener" class="text-[var(--ctp-primary)] hover:underline">SagerNet/sing-geosite</a>
		</p>
	</div>

	<!-- Templates by category -->
	{#each categories as category}
		<div>
			<div class="flex items-center gap-2 mb-2">
				<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">{category.label}</h3>
				<span class="text-xs text-[var(--ctp-overlay0)]">— {category.description}</span>
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
									<span class="font-medium text-[var(--ctp-text)]">{template.name}</span>
									{#if alreadyAdded}
										<span class="text-xs text-[var(--ctp-overlay0)]">already added</span>
									{:else if template.action === 'reject'}
										<span class="text-xs px-1.5 py-0.5 rounded bg-[var(--ctp-red)] bg-opacity-20 text-[var(--ctp-red)]">BLOCK</span>
									{:else}
										<span class="text-xs text-[var(--ctp-overlay0)]">→ {selectedOutbound}</span>
									{/if}
								</div>
								<p class="text-sm text-[var(--ctp-overlay1)] mt-0.5">{template.description}</p>
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
			Close
		</button>
	</div>
</div>

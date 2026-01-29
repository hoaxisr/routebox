<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { SingboxConfig } from '$lib/types';

	interface Props {
		config: SingboxConfig;
	}

	let { config }: Props = $props();
</script>

<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
	<a href="/config/endpoints" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
		<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.endpoints')}</div>
		<div class="text-2xl font-bold text-[var(--ctp-primary)]">
			{config.endpoints?.length ?? 0}
		</div>
	</a>
	<a href="/config/outbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
		<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.outbounds')}</div>
		<div class="text-2xl font-bold text-[var(--ctp-primary)]">
			{config.outbounds?.length ?? 0}
		</div>
	</a>
	<a href="/config/inbounds" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
		<div class="text-sm text-[var(--ctp-overlay1)]">{$t('nav.inbounds')}</div>
		<div class="text-2xl font-bold text-[var(--ctp-text)]">
			{config.inbounds?.length ?? 0}
		</div>
	</a>
	<a href="/config/routes" class="bg-[var(--ctp-surface0)] rounded-xl p-4 hover:bg-[var(--ctp-surface1)] transition-colors">
		<div class="text-sm text-[var(--ctp-overlay1)]">{$t('routes.rules')}</div>
		<div class="text-2xl font-bold text-[var(--ctp-primary)]">
			{config.route?.rules?.length ?? 0}
		</div>
	</a>
</div>

<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">{$t('dns.title')}</h3>
		<div class="space-y-2 text-sm">
			<div class="flex justify-between">
				<span class="text-[var(--ctp-subtext1)]">{$t('dns.servers')}</span>
				<span class="text-[var(--ctp-text)]">{config.dns?.servers?.length ?? 0}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-[var(--ctp-subtext1)]">{$t('dns.rules')}</span>
				<span class="text-[var(--ctp-text)]">{config.dns?.rules?.length ?? 0}</span>
			</div>
		</div>
	</div>
	<div class="bg-[var(--ctp-surface0)] rounded-xl p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-overlay1)] mb-3">{$t('routes.title')}</h3>
		<div class="space-y-2 text-sm">
			<div class="flex justify-between">
				<span class="text-[var(--ctp-subtext1)]">{$t('routes.ruleSets')}</span>
				<span class="text-[var(--ctp-text)]">{config.route?.rule_set?.length ?? 0}</span>
			</div>
			<div class="flex justify-between">
				<span class="text-[var(--ctp-subtext1)]">{$t('routes.finalOutbound')}</span>
				<span class="text-[var(--ctp-text)]">{config.route?.final ?? 'direct'}</span>
			</div>
		</div>
	</div>
</div>

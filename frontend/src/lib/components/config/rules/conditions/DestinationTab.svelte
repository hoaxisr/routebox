<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		ipIsPrivate: boolean;
		invert: boolean;
		domainSuffix: string;
		ipCidr: string;
		ports: string;
		network: 'tcp' | 'udp' | 'icmp' | '';
		ipVersion: number | undefined;
		clashMode: string;
	}

	let {
		ipIsPrivate = $bindable(),
		invert = $bindable(),
		domainSuffix = $bindable(),
		ipCidr = $bindable(),
		ports = $bindable(),
		network = $bindable(),
		ipVersion = $bindable(),
		clashMode = $bindable()
	}: Props = $props();
</script>

<div class="space-y-4">
	<!-- Toggle options row -->
	<div class="flex flex-wrap gap-4">
		<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
			<input type="checkbox" bind:checked={ipIsPrivate}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
			<div>
				<span class="text-sm text-[var(--ctp-text)]">{$t('routes.privateIp')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.privateIpHint')}</p>
			</div>
		</label>
		<label class="flex items-center gap-2 p-2 bg-[var(--ctp-surface0)] rounded-lg cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors">
			<input type="checkbox" bind:checked={invert}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
			<div>
				<span class="text-sm text-[var(--ctp-text)]">{$t('routes.invert')}</span>
				<p class="text-xs text-[var(--ctp-overlay0)]">{$t('routes.invertHint')}</p>
			</div>
		</label>
	</div>

	<!-- Domain Suffix (most common) -->
	<div>
		<label for="domain-suffix" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.domainSuffix')}
			<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.mostCommon')})</span>
		</label>
		<textarea id="domain-suffix" bind:value={domainSuffix} rows={3}
			placeholder="google.com&#10;youtube.com&#10;facebook.com"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		></textarea>
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.domainSuffixHint')}</p>
	</div>

	<!-- IP CIDR -->
	<div>
		<label for="ip-cidr" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('routes.ipCidr')}
			<span class="font-normal text-[var(--ctp-overlay0)]">({$t('routes.onePerLine')})</span>
		</label>
		<textarea id="ip-cidr" bind:value={ipCidr} rows={2}
			placeholder="192.168.1.0/24&#10;10.0.0.0/8&#10;8.8.8.8"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm"
		></textarea>
	</div>

	<!-- Ports + Network -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.port')}</label>
			<input id="ports" type="text" bind:value={ports} placeholder="80, 443, 8080"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.network')}</label>
			<div class="flex gap-2">
				<button type="button" onclick={() => network = ''}
					class="flex-1 toggle-btn {network === '' ? 'selected' : ''}">{$t('routes.networkAny')}</button>
				<button type="button" onclick={() => network = 'tcp'}
					class="flex-1 toggle-btn {network === 'tcp' ? 'selected' : ''}">TCP</button>
				<button type="button" onclick={() => network = 'udp'}
					class="flex-1 toggle-btn {network === 'udp' ? 'selected' : ''}">UDP</button>
			</div>
		</div>
	</div>

	<!-- IP Version -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.ipVersion')}</label>
		<div class="flex gap-2">
			<button type="button" onclick={() => ipVersion = undefined}
				class="flex-1 toggle-btn {ipVersion === undefined ? 'selected' : ''}">{$t('routes.networkAny')}</button>
			<button type="button" onclick={() => ipVersion = 4}
				class="flex-1 toggle-btn {ipVersion === 4 ? 'selected' : ''}">IPv4</button>
			<button type="button" onclick={() => ipVersion = 6}
				class="flex-1 toggle-btn {ipVersion === 6 ? 'selected' : ''}">IPv6</button>
		</div>
	</div>

	<!-- Clash Mode -->
	<div>
		<label for="clash-mode" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('routes.clashMode')}</label>
		<input id="clash-mode" type="text" bind:value={clashMode} placeholder="direct, global, rule"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('routes.clashModeHint')}</p>
	</div>
</div>

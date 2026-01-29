<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ClashConnection } from '$lib/types';

	interface Props {
		connections: ClashConnection[];
		onClose: (id: string) => void;
	}

	let { connections, onClose }: Props = $props();

	// Sorting
	type SortKey = 'host' | 'network' | 'upload' | 'download' | 'start';
	let sortKey = $state<SortKey>('start');
	let sortAsc = $state(false);

	// Filter
	let filter = $state('');

	// Expanded rows
	let expandedIds = $state<Set<string>>(new Set());

	function toggleExpand(id: string) {
		const newSet = new Set(expandedIds);
		if (newSet.has(id)) {
			newSet.delete(id);
		} else {
			newSet.add(id);
		}
		expandedIds = newSet;
	}

	function setSort(key: SortKey) {
		if (sortKey === key) {
			sortAsc = !sortAsc;
		} else {
			sortKey = key;
			sortAsc = false;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
	}

	function timeAgo(dateStr: string): string {
		const date = new Date(dateStr);
		const now = new Date();
		const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

		if (seconds < 60) return `${seconds}s`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
		return `${Math.floor(seconds / 86400)}d`;
	}

	function getHost(conn: ClashConnection): string {
		return conn.metadata.host || conn.metadata.destinationIP || '-';
	}

	// Convert country code to emoji flag (e.g., "US" -> "🇺🇸")
	function countryCodeToFlag(code: string): string {
		if (!code || code.length !== 2) return '';
		const codePoints = code
			.toUpperCase()
			.split('')
			.map(char => 127397 + char.charCodeAt(0));
		return String.fromCodePoint(...codePoints);
	}

	// Build GeoIP tooltip text
	function getGeoTooltip(conn: ClashConnection): string {
		const geo = conn.geoip;
		if (!geo) return '';
		const parts: string[] = [];
		if (geo.country) parts.push(geo.country);
		if (geo.as_name) parts.push(geo.as_name);
		if (geo.asn) parts.push(geo.asn);
		return parts.join(' • ');
	}

	const filteredConnections = $derived(() => {
		let result = connections;

		// Filter
		if (filter.trim()) {
			const lower = filter.toLowerCase();
			result = result.filter(conn =>
				getHost(conn).toLowerCase().includes(lower) ||
				conn.chains.some(c => c.toLowerCase().includes(lower)) ||
				conn.rule.toLowerCase().includes(lower) ||
				conn.geoip?.country?.toLowerCase().includes(lower) ||
				conn.geoip?.country_code?.toLowerCase().includes(lower) ||
				conn.geoip?.as_name?.toLowerCase().includes(lower) ||
				conn.geoip?.asn?.toLowerCase().includes(lower)
			);
		}

		// Sort
		result = [...result].sort((a, b) => {
			let cmp = 0;
			switch (sortKey) {
				case 'host':
					cmp = getHost(a).localeCompare(getHost(b));
					break;
				case 'network':
					cmp = a.metadata.network.localeCompare(b.metadata.network);
					break;
				case 'upload':
					cmp = a.upload - b.upload;
					break;
				case 'download':
					cmp = a.download - b.download;
					break;
				case 'start':
					cmp = new Date(a.start).getTime() - new Date(b.start).getTime();
					break;
			}
			return sortAsc ? cmp : -cmp;
		});

		return result;
	});

	function getSortIcon(key: SortKey): string {
		if (sortKey !== key) return '↕';
		return sortAsc ? '↑' : '↓';
	}

	// Check if any connection has GeoIP data (for attribution footer)
	const hasGeoIPData = $derived(connections.some(c => c.geoip?.country_code));
</script>

<div class="space-y-4">
	<!-- Filter -->
	<div class="relative">
		<input
			type="text"
			bind:value={filter}
			placeholder={$t('connections.search')}
			class="w-full px-4 py-2 pl-10 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
		/>
		<svg class="absolute left-3 top-2.5 w-5 h-5 text-[var(--ctp-overlay0)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
		</svg>
	</div>

	<!-- Table -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] overflow-hidden">
		<div class="overflow-x-auto">
		<table class="w-full min-w-[700px]">
			<thead>
				<tr class="bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] text-sm">
					<th class="px-4 py-3 text-left font-medium">
						<button onclick={() => setSort('host')} class="flex items-center gap-1 hover:text-[var(--ctp-text)]">
							{$t('connections.host')} <span class="opacity-50">{getSortIcon('host')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-left font-medium">
						<button onclick={() => setSort('network')} class="flex items-center gap-1 hover:text-[var(--ctp-text)]">
							{$t('connections.network')} <span class="opacity-50">{getSortIcon('network')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-right font-medium">
						<button onclick={() => setSort('upload')} class="flex items-center gap-1 justify-end hover:text-[var(--ctp-text)]">
							{$t('connections.upload')} <span class="opacity-50">{getSortIcon('upload')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-right font-medium">
						<button onclick={() => setSort('download')} class="flex items-center gap-1 justify-end hover:text-[var(--ctp-text)]">
							{$t('connections.download')} <span class="opacity-50">{getSortIcon('download')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-left font-medium">{$t('connections.chain')}</th>
					<th class="px-4 py-3 text-right font-medium">
						<button onclick={() => setSort('start')} class="flex items-center gap-1 justify-end hover:text-[var(--ctp-text)]">
							{$t('connections.time')} <span class="opacity-50">{getSortIcon('start')}</span>
						</button>
					</th>
					<th class="px-4 py-3 w-12"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-[var(--ctp-surface2)]">
				{#each filteredConnections() as conn (conn.id)}
					<tr class="hover:bg-[var(--ctp-surface1)] transition-colors">
						<td class="px-4 py-3">
							<button
								onclick={() => toggleExpand(conn.id)}
								class="text-left hover:text-[var(--ctp-primary)] transition-colors"
							>
								<div class="flex items-center gap-1.5">
									{#if conn.geoip?.country_code}
										<span
											class="text-base cursor-help"
											title={getGeoTooltip(conn)}
										>{countryCodeToFlag(conn.geoip.country_code)}</span>
									{/if}
									<div class="font-medium text-[var(--ctp-text)] truncate max-w-[180px]" title={getHost(conn)}>
										{getHost(conn)}
									</div>
								</div>
								<div class="text-xs text-[var(--ctp-overlay0)]" class:ml-6={conn.geoip?.country_code}>
									:{conn.metadata.destinationPort}
								</div>
							</button>
						</td>
						<td class="px-4 py-3">
							<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)]">
								{conn.metadata.network.toUpperCase()}
							</span>
						</td>
						<td class="px-4 py-3 text-right font-mono text-sm text-[var(--ctp-subtext1)]">
							{formatBytes(conn.upload)}
						</td>
						<td class="px-4 py-3 text-right font-mono text-sm text-[var(--ctp-subtext1)]">
							{formatBytes(conn.download)}
						</td>
						<td class="px-4 py-3">
							<div class="flex flex-wrap gap-1">
								{#each conn.chains as chain}
									<span class="selection-chip">
										{chain}
									</span>
								{/each}
							</div>
						</td>
						<td class="px-4 py-3 text-right text-sm text-[var(--ctp-overlay1)]">
							{timeAgo(conn.start)}
						</td>
						<td class="px-4 py-3">
							<button
								onclick={() => onClose(conn.id)}
								class="p-1 text-[var(--ctp-overlay0)] hover:text-[var(--ctp-red)] transition-colors"
								title={$t('connections.closeConnection')}
							>
								<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</td>
					</tr>
					{#if expandedIds.has(conn.id)}
						<tr class="bg-[var(--ctp-mantle)]">
							<td colspan="7" class="px-4 py-3">
								<div class="grid grid-cols-2 gap-4 text-sm">
									<div>
										<span class="text-[var(--ctp-overlay0)]">{$t('connections.sourceIP')}:</span>
										<span class="ml-2 font-mono text-[var(--ctp-text)]">
											{conn.metadata.sourceIP}:{conn.metadata.sourcePort}
										</span>
									</div>
									<div>
										<span class="text-[var(--ctp-overlay0)]">{$t('connections.destinationIP')}:</span>
										<span class="ml-2 font-mono text-[var(--ctp-text)]">
											{conn.metadata.destinationIP}:{conn.metadata.destinationPort}
										</span>
									</div>
									<div>
										<span class="text-[var(--ctp-overlay0)]">{$t('connections.rule')}:</span>
										<span class="ml-2 text-[var(--ctp-text)]">{conn.rule}</span>
										{#if conn.rulePayload}
											<span class="ml-1 text-[var(--ctp-overlay1)]">({conn.rulePayload})</span>
										{/if}
									</div>
									<div>
										<span class="text-[var(--ctp-overlay0)]">{$t('connections.type')}:</span>
										<span class="ml-2 text-[var(--ctp-text)]">{conn.metadata.type}</span>
									</div>
									{#if conn.metadata.processPath}
										<div class="col-span-2">
											<span class="text-[var(--ctp-overlay0)]">{$t('routes.processPath')}:</span>
											<span class="ml-2 font-mono text-[var(--ctp-text)] text-xs">
												{conn.metadata.processPath}
											</span>
										</div>
									{/if}
									{#if conn.geoip}
										<div class="col-span-2 pt-2 border-t border-[var(--ctp-surface2)]">
											<span class="text-[var(--ctp-overlay0)]">{$t('routes.geoip')}:</span>
											<span class="ml-2 text-[var(--ctp-text)]">
												{#if conn.geoip.country_code}
													{countryCodeToFlag(conn.geoip.country_code)}
												{/if}
												{conn.geoip.country || ''}
												{#if conn.geoip.as_name}
													<span class="text-[var(--ctp-overlay1)]">• {conn.geoip.as_name}</span>
												{/if}
												{#if conn.geoip.asn}
													<span class="text-[var(--ctp-overlay0)]">({conn.geoip.asn})</span>
												{/if}
											</span>
										</div>
									{/if}
								</div>
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center text-[var(--ctp-overlay0)]">
							{filter ? $t('proxies.noProxiesMatchFilter') : $t('connections.noConnections')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
		</div>
	</div>

	{#if hasGeoIPData}
		<div class="mt-3 text-xs text-[var(--ctp-overlay0)] text-right">
			{$t('settings.geoip')}: <a href="https://iplocate.io" target="_blank" rel="noopener noreferrer" class="text-[var(--ctp-primary)] hover:underline">IPLocate.io</a>
		</div>
	{/if}
</div>

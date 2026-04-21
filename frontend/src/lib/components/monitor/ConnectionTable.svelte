<script lang="ts">
	import { browser } from '$app/environment';
	import { t } from 'svelte-i18n';
	import { formatBytes } from '$lib/stores';
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

	// Grouping — persisted, mutually exclusive
	let groupBySource = $state(
		browser ? localStorage.getItem('connections.groupBySource') !== 'false' : true
	);
	let groupByChain = $state(
		browser ? localStorage.getItem('connections.groupByChain') === 'true' : false
	);
	let expandedGroups = $state<Set<string>>(new Set());

	$effect(() => {
		if (browser) localStorage.setItem('connections.groupBySource', String(groupBySource));
	});
	$effect(() => {
		if (browser) localStorage.setItem('connections.groupByChain', String(groupByChain));
	});

	function toggleGroupBySource(checked: boolean) {
		groupBySource = checked;
		if (checked) groupByChain = false;
		expandedGroups = new Set();
	}

	function toggleGroupByChain(checked: boolean) {
		groupByChain = checked;
		if (checked) groupBySource = false;
		expandedGroups = new Set();
	}

	function getChainKey(conn: ClashConnection): string {
		if (!conn.chains || conn.chains.length === 0) return '-';
		return conn.chains.join(' → ');
	}

	function toggleGroup(ip: string) {
		const newSet = new Set(expandedGroups);
		if (newSet.has(ip)) {
			newSet.delete(ip);
		} else {
			newSet.add(ip);
		}
		expandedGroups = newSet;
	}

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

	function sortConnections(conns: ClashConnection[]): ClashConnection[] {
		return [...conns].sort((a, b) => {
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
	}

	const filteredConnections = $derived.by(() => {
		let result = connections;

		// Filter
		if (filter.trim()) {
			const lower = filter.toLowerCase();
			result = result.filter(conn =>
				getHost(conn).toLowerCase().includes(lower) ||
				conn.chains.some(c => c.toLowerCase().includes(lower)) ||
				conn.rule.toLowerCase().includes(lower) ||
				conn.metadata.sourceIP?.toLowerCase().includes(lower) ||
				conn.geoip?.country?.toLowerCase().includes(lower) ||
				conn.geoip?.country_code?.toLowerCase().includes(lower) ||
				conn.geoip?.as_name?.toLowerCase().includes(lower) ||
				conn.geoip?.asn?.toLowerCase().includes(lower)
			);
		}

		return sortConnections(result);
	});

	// Grouped connections by sourceIP
	interface SourceGroup {
		sourceIP: string;
		connections: ClashConnection[];
		totalUpload: number;
		totalDownload: number;
	}

	const groupedConnections = $derived.by((): SourceGroup[] => {
		const conns = filteredConnections;
		const groups = new Map<string, ClashConnection[]>();

		for (const conn of conns) {
			const ip = conn.metadata.sourceIP || 'unknown';
			if (!groups.has(ip)) {
				groups.set(ip, []);
			}
			groups.get(ip)!.push(conn);
		}

		return Array.from(groups.entries())
			.map(([ip, conns]) => ({
				sourceIP: ip,
				connections: conns,
				totalUpload: conns.reduce((s, c) => s + c.upload, 0),
				totalDownload: conns.reduce((s, c) => s + c.download, 0)
			}))
			.sort((a, b) => (b.totalDownload + b.totalUpload) - (a.totalDownload + a.totalUpload));
	});

	interface ChainGroup {
		chain: string;
		connections: ClashConnection[];
		totalUpload: number;
		totalDownload: number;
	}

	const chainGroupedConnections = $derived.by((): ChainGroup[] => {
		const conns = filteredConnections;
		const groups = new Map<string, ClashConnection[]>();

		for (const conn of conns) {
			const key = getChainKey(conn);
			if (!groups.has(key)) {
				groups.set(key, []);
			}
			groups.get(key)!.push(conn);
		}

		return Array.from(groups.entries())
			.map(([chain, conns]) => ({
				chain,
				connections: conns,
				totalUpload: conns.reduce((s, c) => s + c.upload, 0),
				totalDownload: conns.reduce((s, c) => s + c.download, 0)
			}))
			.sort((a, b) => (b.totalDownload + b.totalUpload) - (a.totalDownload + a.totalUpload));
	});

	function getSortIcon(key: SortKey): string {
		if (sortKey !== key) return '↕';
		return sortAsc ? '↑' : '↓';
	}

	// Check if any connection has GeoIP data (for attribution footer)
	const hasGeoIPData = $derived(connections.some(c => c.geoip?.country_code));
</script>

<div class="space-y-4">
	<!-- Filter + Group toggle -->
	<div class="flex items-center gap-3">
		<div class="relative flex-1">
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
		<label class="flex items-center gap-2 cursor-pointer flex-shrink-0 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" checked={groupBySource}
				onchange={(e) => toggleGroupBySource(e.currentTarget.checked)}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
			{$t('connections.groupBySource')}
		</label>
		<label class="flex items-center gap-2 cursor-pointer flex-shrink-0 text-sm text-[var(--ctp-subtext1)]">
			<input type="checkbox" checked={groupByChain}
				onchange={(e) => toggleGroupByChain(e.currentTarget.checked)}
				class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
			{$t('connections.groupByChain')}
		</label>
	</div>

	<!-- Table -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] overflow-hidden">
		<div class="overflow-x-auto">
		<table class="w-full min-w-[700px] table-fixed">
			<colgroup>
				<col class="w-[35%]" /><!-- Host -->
				<col class="w-[8%]" /><!-- Network -->
				<col class="w-[12%]" /><!-- Upload -->
				<col class="w-[12%]" /><!-- Download -->
				<col class="w-[15%]" /><!-- Chain -->
				<col class="w-[8%]" /><!-- Time -->
				<col class="w-[40px]" /><!-- Close -->
			</colgroup>
			<thead>
				<tr class="bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] text-sm">
					<th class="px-4 py-3 text-left font-medium">
						<button onclick={() => setSort('host')} class="w-full flex items-center gap-1 hover:text-[var(--ctp-text)]">
							{$t('connections.host')} <span class="opacity-50">{getSortIcon('host')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-center font-medium">
						<button onclick={() => setSort('network')} class="w-full flex items-center gap-1 justify-center hover:text-[var(--ctp-text)]">
							{$t('connections.network')} <span class="opacity-50">{getSortIcon('network')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-right font-medium">
						<button onclick={() => setSort('upload')} class="w-full flex items-center gap-1 justify-end hover:text-[var(--ctp-text)]">
							{$t('connections.upload')} <span class="opacity-50">{getSortIcon('upload')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-right font-medium">
						<button onclick={() => setSort('download')} class="w-full flex items-center gap-1 justify-end hover:text-[var(--ctp-text)]">
							{$t('connections.download')} <span class="opacity-50">{getSortIcon('download')}</span>
						</button>
					</th>
					<th class="px-4 py-3 text-center font-medium">{$t('connections.chain')}</th>
					<th class="px-4 py-3 text-center font-medium">
						<button onclick={() => setSort('start')} class="w-full flex items-center gap-1 justify-center hover:text-[var(--ctp-text)]">
							{$t('connections.time')} <span class="opacity-50">{getSortIcon('start')}</span>
						</button>
					</th>
					<th class="px-4 py-3"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-[var(--ctp-surface2)]">
				{#if groupBySource}
					{#each groupedConnections as group}
						<!-- Group header -->
						<tr class="bg-[var(--ctp-mantle)] cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors"
							onclick={() => toggleGroup(group.sourceIP)}>
							<td class="px-4 py-2" colspan="2">
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4 transition-transform text-[var(--ctp-overlay1)] {expandedGroups.has(group.sourceIP) ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
									</svg>
									<span class="font-mono font-medium text-[var(--ctp-text)]">{group.sourceIP}</span>
									<span class="text-xs text-[var(--ctp-overlay0)]">
										({group.connections.length})
									</span>
								</div>
							</td>
							<td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
								{formatBytes(group.totalUpload)}
							</td>
							<td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
								{formatBytes(group.totalDownload)}
							</td>
							<td colspan="3"></td>
						</tr>
						{#if expandedGroups.has(group.sourceIP)}
							{#each group.connections as conn (conn.id)}
								{@render connectionRow(conn)}
								{@render expandedRow(conn)}
							{/each}
						{/if}
					{/each}
					{#if groupedConnections.length === 0}
						<tr>
							<td colspan="7" class="px-4 py-8 text-center text-[var(--ctp-overlay0)]">
								{filter ? $t('proxies.noProxiesMatchFilter') : $t('connections.noConnections')}
							</td>
						</tr>
					{/if}
				{:else if groupByChain}
					{#each chainGroupedConnections as group}
						<tr class="bg-[var(--ctp-mantle)] cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors"
							onclick={() => toggleGroup(group.chain)}>
							<td class="px-4 py-2" colspan="2">
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4 transition-transform text-[var(--ctp-overlay1)] {expandedGroups.has(group.chain) ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
									</svg>
									<span class="selection-chip">{group.chain}</span>
									<span class="text-xs text-[var(--ctp-overlay0)]">
										({group.connections.length})
									</span>
								</div>
							</td>
							<td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
								{formatBytes(group.totalUpload)}
							</td>
							<td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
								{formatBytes(group.totalDownload)}
							</td>
							<td colspan="3"></td>
						</tr>
						{#if expandedGroups.has(group.chain)}
							{#each group.connections as conn (conn.id)}
								{@render connectionRow(conn)}
								{@render expandedRow(conn)}
							{/each}
						{/if}
					{/each}
					{#if chainGroupedConnections.length === 0}
						<tr>
							<td colspan="7" class="px-4 py-8 text-center text-[var(--ctp-overlay0)]">
								{filter ? $t('proxies.noProxiesMatchFilter') : $t('connections.noConnections')}
							</td>
						</tr>
					{/if}
				{:else}
					{#each filteredConnections as conn (conn.id)}
						{@render connectionRow(conn)}
						{@render expandedRow(conn)}
					{:else}
						<tr>
							<td colspan="7" class="px-4 py-8 text-center text-[var(--ctp-overlay0)]">
								{filter ? $t('proxies.noProxiesMatchFilter') : $t('connections.noConnections')}
							</td>
						</tr>
					{/each}
				{/if}
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

{#snippet connectionRow(conn: ClashConnection)}
	<tr class="hover:bg-[var(--ctp-surface1)] transition-colors">
		<td class="px-4 py-3 max-w-0">
			<button
				onclick={() => toggleExpand(conn.id)}
				class="text-left hover:text-[var(--ctp-primary)] transition-colors w-full"
			>
				<div class="flex items-center gap-1.5">
					{#if conn.geoip?.country_code}
						<span
							class="text-base cursor-help"
							title={getGeoTooltip(conn)}
						>{countryCodeToFlag(conn.geoip.country_code)}</span>
					{/if}
					<div class="font-medium text-[var(--ctp-text)] truncate" title={getHost(conn)}>
						{getHost(conn)}
					</div>
				</div>
				<div class="text-xs text-[var(--ctp-overlay0)]" class:ml-6={conn.geoip?.country_code}>
					:{conn.metadata.destinationPort}
				</div>
			</button>
		</td>
		<td class="px-4 py-3 text-center">
			<span class="px-2 py-0.5 text-xs rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)]">
				{conn.metadata.network.toUpperCase()}
			</span>
		</td>
		<td class="px-4 py-3 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
			{formatBytes(conn.upload)}
		</td>
		<td class="px-4 py-3 text-right font-mono text-sm text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap">
			{formatBytes(conn.download)}
		</td>
		<td class="px-4 py-3 text-center">
			<div class="flex flex-wrap gap-1 justify-center">
				{#each conn.chains as chain}
					<span class="selection-chip">
						{chain}
					</span>
				{/each}
			</div>
		</td>
		<td class="px-4 py-3 text-center text-sm text-[var(--ctp-overlay1)] whitespace-nowrap">
			{timeAgo(conn.start)}
		</td>
		<td class="px-4 py-3 text-center">
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
{/snippet}

{#snippet expandedRow(conn: ClashConnection)}
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
{/snippet}

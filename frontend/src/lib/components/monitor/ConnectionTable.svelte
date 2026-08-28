<script lang="ts">
	import { browser } from '$app/environment';
	import { t } from 'svelte-i18n';
	import { formatBytes, clientNames, behindFront } from '$lib/stores';
	import type { ClashConnection } from '$lib/types';
	import { isGroupOnlyChain } from '$lib/utils/chainLabel';

	interface Props {
		connections: ClashConnection[];
		onClose: (id: string) => void;
		/** Tags of selector/urltest outbounds, for spotting a chain that names a
		 *  group and nothing else (#79). Empty = claim nothing. */
		groups?: Set<string>;
	}

	let { connections, onClose, groups = new Set() }: Props = $props();

	// A chain that is only a group says the traffic went through it but not which
	// member carried it — the connection was opened before the group had picked
	// one, and the chain is never revised afterwards. Marked rather than silently
	// shown as if it were a complete path.
	const groupOnly = (chain: string[] | string) => isGroupOnlyChain(chain, groups);

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

	// Behind the front every connection reports the loopback as its source, so
	// the address is not data — it is the absence of data. One derived flag, so
	// the table, the cards and the details cannot disagree about it. GeoIP is
	// unaffected: it is looked up on the destination, which is real either way.
	const showSource = $derived(!$behindFront);
	// Grouping by client needs client addresses; without them every connection
	// falls into one group named after the loopback.
	const groupingBySource = $derived(groupBySource && showSource);

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
				(showSource && conn.metadata.sourceIP?.toLowerCase().includes(lower)) ||
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
	<div class="flex flex-col sm:flex-row sm:items-center gap-3">
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
		<div class="flex items-center gap-4 sm:gap-3 sm:contents">
			{#if showSource}
				<label class="flex items-center gap-2 cursor-pointer flex-shrink-0 text-sm text-[var(--ctp-subtext1)]">
					<input type="checkbox" checked={groupBySource}
						onchange={(e) => toggleGroupBySource(e.currentTarget.checked)}
						class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
					{$t('connections.groupBySource')}
				</label>
			{/if}
			<label class="flex items-center gap-2 cursor-pointer flex-shrink-0 text-sm text-[var(--ctp-subtext1)]">
				<input type="checkbox" checked={groupByChain}
					onchange={(e) => toggleGroupByChain(e.currentTarget.checked)}
					class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)] focus:ring-[var(--ctp-primary)]" />
				{$t('connections.groupByChain')}
			</label>
		</div>
	</div>

	{#if !showSource}
		<div class="flex items-start gap-2 px-3 py-2 rounded-lg bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] text-sm">
			<svg class="w-4 h-4 mt-0.5 flex-shrink-0 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
			</svg>
			<span>
				<span class="text-[var(--ctp-text)]">{$t('connections.clientAddressUnavailable')}</span>
				<span class="text-[var(--ctp-overlay1)]"> — {$t('connections.clientAddressUnavailableHint')}</span>
			</span>
		</div>
	{/if}

	<!-- Table -->
	<div class="hidden md:block bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] overflow-hidden">
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
					<th class="px-4 py-3 text-center font-medium">
						{groupByChain && showSource ? $t('connections.source') : $t('connections.chain')}
					</th>
					<th class="px-4 py-3 text-center font-medium">
						<button onclick={() => setSort('start')} class="w-full flex items-center gap-1 justify-center hover:text-[var(--ctp-text)]">
							{$t('connections.time')} <span class="opacity-50">{getSortIcon('start')}</span>
						</button>
					</th>
					<th class="px-4 py-3"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-[var(--ctp-surface2)]">
				{#if groupingBySource}
					{#each groupedConnections as group}
						{@const groupName = $clientNames.get(group.sourceIP)}
						<!-- Group header -->
						<tr class="bg-[var(--ctp-mantle)] cursor-pointer hover:bg-[var(--ctp-surface1)] transition-colors"
							onclick={() => toggleGroup(group.sourceIP)}>
							<td class="px-4 py-2" colspan="2">
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4 transition-transform text-[var(--ctp-overlay1)] {expandedGroups.has(group.sourceIP) ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
									</svg>
									<span class="font-medium text-[var(--ctp-text)]">
										{#if groupName}{groupName} <span class="font-mono text-xs text-[var(--ctp-overlay0)]">({group.sourceIP})</span>
										{:else}<span class="font-mono">{group.sourceIP}</span>
										{/if}
									</span>
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
								{filter ? $t('connections.noConnectionsMatchFilter') : $t('connections.noConnections')}
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
								{filter ? $t('connections.noConnectionsMatchFilter') : $t('connections.noConnections')}
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
								{filter ? $t('connections.noConnectionsMatchFilter') : $t('connections.noConnections')}
							</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
		</div>
	</div>

	<!-- Mobile card list -->
	<div class="md:hidden space-y-2">
		{#if groupingBySource}
			{#each groupedConnections as group}
				{@render groupHeaderCard(group.sourceIP, $clientNames.get(group.sourceIP), group.connections.length, group.totalUpload, group.totalDownload)}
				{#if expandedGroups.has(group.sourceIP)}
					{#each group.connections as conn (conn.id)}
						{@render connectionCard(conn)}
					{/each}
				{/if}
			{:else}
				{@render emptyCard()}
			{/each}
		{:else if groupByChain}
			{#each chainGroupedConnections as group}
				{@render groupHeaderCard(group.chain, undefined, group.connections.length, group.totalUpload, group.totalDownload)}
				{#if expandedGroups.has(group.chain)}
					{#each group.connections as conn (conn.id)}
						{@render connectionCard(conn)}
					{/each}
				{/if}
			{:else}
				{@render emptyCard()}
			{/each}
		{:else}
			{#each filteredConnections as conn (conn.id)}
				{@render connectionCard(conn)}
			{:else}
				{@render emptyCard()}
			{/each}
		{/if}
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
			{#if groupByChain && showSource}
				{@const name = $clientNames.get(conn.metadata.sourceIP || '')}
				<span class="text-sm text-[var(--ctp-subtext1)] whitespace-nowrap" title={conn.metadata.sourceIP}>
					{#if name}{name}
					{:else}<span class="font-mono">{conn.metadata.sourceIP || '-'}</span>{/if}
				</span>
			{:else}
				<div class="flex flex-wrap gap-1 justify-center">
					{#each conn.chains as chain}
						<span class="selection-chip">
							{chain}
						</span>
					{/each}
					{#if groupOnly(conn.chains)}
						<span class="chain-partial" title={$t('connections.chainGroupOnlyHint')}>?</span>
					{/if}
				</div>
			{/if}
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
						{#if showSource}
							<span class="ml-2 font-mono text-[var(--ctp-text)]">
								{conn.metadata.sourceIP}:{conn.metadata.sourcePort}
							</span>
						{:else}
							<span class="ml-2 text-[var(--ctp-overlay1)]" title={$t('connections.clientAddressUnavailableHint')}>
								{$t('connections.clientAddressUnavailableShort')}
							</span>
						{/if}
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

{#snippet groupHeaderCard(key: string, name: string | undefined, count: number, up: number, down: number)}
	<button
		onclick={() => toggleGroup(key)}
		aria-expanded={expandedGroups.has(key)}
		class="w-full flex items-center gap-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg px-3 py-2.5 text-left"
	>
		<svg class="w-4 h-4 flex-shrink-0 transition-transform text-[var(--ctp-overlay1)] {expandedGroups.has(key) ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
		</svg>
		<span class="font-medium text-sm text-[var(--ctp-text)] truncate">
			{#if groupByChain}<span class="selection-chip">{key}</span>{#if groupOnly(key)}<span class="chain-partial" title={$t('connections.chainGroupOnlyHint')}>?</span>{/if}
			{:else if name}{name}
			{:else}<span class="font-mono">{key}</span>{/if}
		</span>
		<span class="text-xs text-[var(--ctp-overlay0)] flex-shrink-0">({count})</span>
		<span class="ml-auto font-mono text-xs text-[var(--ctp-subtext1)] tabular-nums whitespace-nowrap flex-shrink-0">
			↑ {formatBytes(up)} ↓ {formatBytes(down)}
		</span>
	</button>
{/snippet}

{#snippet connectionCard(conn: ClashConnection)}
	<div class="bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg p-3">
		<div class="flex items-start justify-between gap-2">
			<button onclick={() => toggleExpand(conn.id)} aria-expanded={expandedIds.has(conn.id)} class="text-left min-w-0 flex-1">
				<div class="flex items-center gap-1.5">
					{#if conn.geoip?.country_code}
						<span class="text-base" title={getGeoTooltip(conn)}>{countryCodeToFlag(conn.geoip.country_code)}</span>
					{/if}
					<span class="font-medium text-[var(--ctp-text)] truncate" title={getHost(conn)}>{getHost(conn)}</span>
					<span class="text-xs text-[var(--ctp-overlay0)] flex-shrink-0">:{conn.metadata.destinationPort}</span>
				</div>
			</button>
			<button
				onclick={() => onClose(conn.id)}
				class="p-2 -m-1 flex-shrink-0 text-[var(--ctp-overlay0)] hover:text-[var(--ctp-red)] transition-colors"
				title={$t('connections.closeConnection')}
			>
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>
		<div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 mt-2 text-xs text-[var(--ctp-overlay1)]">
			<span class="px-2 py-0.5 rounded bg-[var(--ctp-surface2)] text-[var(--ctp-subtext1)]">{conn.metadata.network.toUpperCase()}</span>
			{#if groupByChain && showSource}
				{@const name = $clientNames.get(conn.metadata.sourceIP || '')}
				<span class="text-[var(--ctp-subtext1)] whitespace-nowrap" title={conn.metadata.sourceIP}>
					{#if name}{name}
					{:else}<span class="font-mono">{conn.metadata.sourceIP || '-'}</span>{/if}
				</span>
			{:else}
				{#each conn.chains as chain}
					<span class="selection-chip">{chain}</span>
				{/each}
				{#if groupOnly(conn.chains)}
					<span class="chain-partial" title={$t('connections.chainGroupOnlyHint')}>?</span>
				{/if}
			{/if}
			<span class="font-mono tabular-nums">↑ {formatBytes(conn.upload)}</span>
			<span class="font-mono tabular-nums">↓ {formatBytes(conn.download)}</span>
			<span>{timeAgo(conn.start)}</span>
		</div>
		{#if expandedIds.has(conn.id)}
			<div class="mt-2 pt-2 border-t border-[var(--ctp-surface2)] text-xs space-y-1 text-[var(--ctp-subtext1)]">
				<div>
					<span class="text-[var(--ctp-overlay0)]">{$t('connections.sourceIP')}:</span>
					{#if showSource}
						<span class="font-mono">{conn.metadata.sourceIP}:{conn.metadata.sourcePort}</span>
					{:else}
						<span class="text-[var(--ctp-overlay1)]">{$t('connections.clientAddressUnavailableShort')}</span>
					{/if}
				</div>
				<div>
					<span class="text-[var(--ctp-overlay0)]">{$t('connections.rule')}:</span>
					{conn.rule}{#if conn.rulePayload} <span class="text-[var(--ctp-overlay1)]">({conn.rulePayload})</span>{/if}
				</div>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet emptyCard()}
	<div class="bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg px-4 py-8 text-center text-[var(--ctp-overlay0)]">
		{filter ? $t('connections.noConnectionsMatchFilter') : $t('connections.noConnections')}
	</div>
{/snippet}

<style>
	/* Deliberately quiet: the row is still real data, only its path is partial. */
	.chain-partial {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1rem;
		height: 1rem;
		border-radius: 999px;
		font-size: 0.65rem;
		font-weight: 600;
		cursor: help;
		background: color-mix(in srgb, var(--ctp-overlay1) 20%, transparent);
		color: var(--ctp-overlay1);
	}
</style>

<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { t } from 'svelte-i18n';
    import { api, createConnectionsStream } from '$lib/api/client';
    import { notifications, applyClients, formatBytes } from '$lib/stores';
    import type { ClientEntry, ClashConnection } from '$lib/types';

    let entries = $state<ClientEntry[]>([]);
    let loading = $state(true);
    let search = $state('');
    let showOffline = $state(true);
    let hideUnnamed = $state(false);

    let liveConnections = $state<ClashConnection[]>([]);
    let stream: { close: () => void } | null = null;

    let traffic24h = $state<Map<string, number>>(new Map());

    async function fetchClients() {
        try {
            const list = await api.listClients();
            entries = list;
            applyClients(list);
        } catch (e) {
            notifications.error(`${$t('errors.loadFailed')}: ${e}`);
        } finally {
            loading = false;
        }
    }

    async function fetchTraffic24h() {
        try {
            const data = await api.getTrafficHistory('24h');
            const map = new Map<string, number>();
            for (const b of data.buckets) {
                map.set(b.source, (map.get(b.source) ?? 0) + b.upload + b.download);
            }
            traffic24h = map;
        } catch {
            /* ignore — backend may not have any data yet */
        }
    }

    function startStream() {
        stream = createConnectionsStream((data) => {
            liveConnections = data.connections ?? [];
        });
    }

    function connCount(ip: string): number {
        return liveConnections.filter(c => c.metadata.sourceIP === ip).length;
    }

    function formatLastSeen(entry: ClientEntry): string {
        if (entry.online) return $t('clients.live');
        const seconds = Math.floor(Date.now() / 1000) - entry.last_seen;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
        if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
        return `${Math.floor(seconds / 86400)}d ago`;
    }

    let filtered = $derived.by(() => {
        const lower = search.trim().toLowerCase();
        return entries.filter(e => {
            if (!showOffline && !e.online) return false;
            if (hideUnnamed && !e.name) return false;
            if (lower) {
                if (!e.ip.toLowerCase().includes(lower)
                    && !(e.name?.toLowerCase().includes(lower))
                    && !(e.note?.toLowerCase().includes(lower))) return false;
            }
            return true;
        });
    });

    async function saveEntry(ip: string, name: string, note: string) {
        try {
            const updated = await api.updateClient(ip, { name, note });
            entries = entries.map(e => e.ip === ip ? { ...e, name: updated.name, note: updated.note } : e);
            applyClients(entries);
        } catch (e) {
            notifications.error(`${e}`);
        }
    }

    async function forget(ip: string) {
        if (!confirm($t('clients.forgetConfirm', { values: { ip } }))) return;
        try {
            await api.deleteClient(ip);
            entries = entries.filter(e => e.ip !== ip);
            applyClients(entries);
        } catch (e) {
            notifications.error(`${e}`);
        }
    }

    onMount(() => {
        fetchClients();
        fetchTraffic24h();
        startStream();
        const refresh = setInterval(fetchClients, 30_000);
        return () => clearInterval(refresh);
    });
    onDestroy(() => {
        stream?.close();
    });
</script>

<svelte:head><title>{$t('clients.title')} - RouteBox</title></svelte:head>

<div class="space-y-4">
    <div>
        <h1 class="text-2xl font-bold text-[var(--ctp-text)]">{$t('clients.title')}</h1>
        <p class="text-sm text-[var(--ctp-overlay1)] mt-1">{$t('clients.description')}</p>
    </div>

    <!-- Toolbar -->
    <div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] p-3 flex items-center gap-3 flex-wrap">
        <input
            type="text"
            bind:value={search}
            placeholder={$t('clients.search')}
            class="flex-1 min-w-[200px] px-3 py-1.5 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded text-[var(--ctp-text)] text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
        />
        <label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
            <input type="checkbox" bind:checked={showOffline} class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)]"/>
            {$t('clients.showOffline')}
        </label>
        <label class="flex items-center gap-2 text-sm text-[var(--ctp-subtext1)]">
            <input type="checkbox" bind:checked={hideUnnamed} class="w-4 h-4 rounded border-[var(--ctp-surface2)] text-[var(--ctp-primary)]"/>
            {$t('clients.hideUnnamed')}
        </label>
    </div>

    <!-- Table -->
    {#if loading}
        <div class="text-[var(--ctp-overlay0)]">{$t('common.loading')}</div>
    {:else if filtered.length === 0}
        <div class="bg-[var(--ctp-surface0)] rounded-xl p-8 text-center text-[var(--ctp-overlay1)]">
            {$t('clients.noClients')}
        </div>
    {:else}
        <div class="bg-[var(--ctp-surface0)] rounded-lg border border-[var(--ctp-surface2)] overflow-hidden">
            <table class="w-full">
                <thead>
                    <tr class="bg-[var(--ctp-mantle)] text-[var(--ctp-subtext1)] text-xs uppercase tracking-wide">
                        <th class="px-4 py-2 text-left">{$t('clients.ip')}</th>
                        <th class="px-4 py-2 text-left">{$t('clients.name')}</th>
                        <th class="px-4 py-2 text-left">{$t('clients.note')}</th>
                        <th class="px-4 py-2 text-right">{$t('clients.connections')}</th>
                        <th class="px-4 py-2 text-right">{$t('clients.traffic24h')}</th>
                        <th class="px-4 py-2 text-center">{$t('clients.lastSeen')}</th>
                        <th class="px-4 py-2"></th>
                    </tr>
                </thead>
                <tbody class="divide-y divide-[var(--ctp-surface2)]">
                    {#each filtered as e (e.ip)}
                        {@const traffic = traffic24h.get(e.ip)}
                        <tr class="hover:bg-[var(--ctp-surface1)] transition-colors">
                            <td class="px-4 py-2 font-mono text-sm flex items-center gap-2">
                                <span class="w-2 h-2 rounded-full {e.online ? 'bg-[var(--ctp-green)]' : 'bg-[var(--ctp-overlay0)]'}"></span>
                                {e.ip}
                            </td>
                            <td class="px-4 py-2">
                                <input
                                    type="text"
                                    value={e.name}
                                    placeholder={$t('clients.namePlaceholder')}
                                    onblur={(ev) => saveEntry(e.ip, ev.currentTarget.value, e.note)}
                                    class="w-full px-2 py-1 bg-transparent border border-transparent rounded text-sm text-[var(--ctp-text)] hover:border-[var(--ctp-surface2)] hover:bg-[var(--ctp-mantle)] focus:bg-[var(--ctp-mantle)] focus:border-[var(--ctp-primary)] focus:outline-none"
                                />
                            </td>
                            <td class="px-4 py-2">
                                <input
                                    type="text"
                                    value={e.note}
                                    placeholder={$t('clients.notePlaceholder')}
                                    onblur={(ev) => saveEntry(e.ip, e.name, ev.currentTarget.value)}
                                    class="w-full px-2 py-1 bg-transparent border border-transparent rounded text-xs text-[var(--ctp-overlay1)] hover:border-[var(--ctp-surface2)] hover:bg-[var(--ctp-mantle)] focus:bg-[var(--ctp-mantle)] focus:border-[var(--ctp-primary)] focus:outline-none"
                                />
                            </td>
                            <td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)]">{connCount(e.ip)}</td>
                            <td class="px-4 py-2 text-right font-mono text-sm text-[var(--ctp-subtext1)]">{traffic !== undefined ? formatBytes(traffic) : '—'}</td>
                            <td class="px-4 py-2 text-center text-xs text-[var(--ctp-overlay1)]">{formatLastSeen(e)}</td>
                            <td class="px-4 py-2 text-center">
                                <button
                                    onclick={() => forget(e.ip)}
                                    class="p-1 text-[var(--ctp-overlay0)] hover:text-[var(--ctp-red)] transition-colors"
                                    title={$t('clients.forget')}
                                >
                                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                                    </svg>
                                </button>
                            </td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
    {/if}
</div>

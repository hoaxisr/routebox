<script lang="ts">
	import type { DnsServer } from '$lib/types';
	import { t } from 'svelte-i18n';
	import DomainResolverField from './DomainResolverField.svelte';

	interface Props {
		server: string;
		serverPort: number;
		ports: string;
		transport: 'TCP' | 'UDP';
		username: string;
		password: string;
		multiplexing: string;
		trafficPattern: string;
		domainResolver: string;
		dnsServers: DnsServer[];
		hasDefaultResolver: boolean;
		/** Mark server-determined params (transport, traffic pattern) read-only on the client. */
		serverLocked?: boolean;
		errors?: Record<string, string>;
		onImport?: () => void;
	}

	let {
		server = $bindable(),
		serverPort = $bindable(),
		ports = $bindable(),
		transport = $bindable(),
		username = $bindable(),
		password = $bindable(),
		multiplexing = $bindable(),
		trafficPattern = $bindable(),
		domainResolver = $bindable(''),
		dnsServers = [],
		hasDefaultResolver = false,
		serverLocked = false,
		errors = {},
		onImport
	}: Props = $props();

	const transports: ('TCP' | 'UDP')[] = ['TCP', 'UDP'];

	// '' = unset; the MULTIPLEXING_* labels are intentionally raw enum identifiers.
	const muxOptions = [
		'',
		'MULTIPLEXING_DEFAULT',
		'MULTIPLEXING_OFF',
		'MULTIPLEXING_LOW',
		'MULTIPLEXING_MIDDLE',
		'MULTIPLEXING_HIGH'
	];
</script>

<div class="space-y-4">
	{#if onImport}
		<button
			type="button"
			onclick={onImport}
			class="w-full px-4 py-2 bg-[var(--ctp-surface0)] border border-dashed border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-subtext1)] hover:border-[var(--ctp-primary)] hover:text-[var(--ctp-primary)] transition-colors flex items-center justify-center gap-2"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
			</svg>
			{$t('outbounds.importFromMieru')}
		</button>
	{/if}

	<!-- Server & Port (inline: mieru allows port 0 = ranges only) -->
	<div class="grid grid-cols-3 gap-4">
		<div class="col-span-2">
			<label for="mieru-server" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('common.server')} *
			</label>
			<input
				id="mieru-server"
				type="text"
				bind:value={server}
				placeholder="example.com"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['server'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['server']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['server']}</p>
			{/if}
		</div>
		<div>
			<label for="mieru-port" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('common.port')}
			</label>
			<input
				id="mieru-port"
				type="number"
				bind:value={serverPort}
				min="0"
				max="65535"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['serverPort'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['serverPort']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['serverPort']}</p>
			{:else}
				<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.mieruForm.serverPortHint')}</p>
			{/if}
		</div>
	</div>

	<!-- Domain Resolver -->
	<DomainResolverField
		bind:value={domainResolver}
		serverAddress={server}
		{dnsServers}
		{hasDefaultResolver}
	/>

	<!-- Extra port ranges -->
	<div>
		<label for="mieru-ports" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('outbounds.mieruForm.ports')}
		</label>
		<input
			id="mieru-ports"
			type="text"
			bind:value={ports}
			placeholder="9000-9010, 8443"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm {errors['serverPorts'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
		/>
		{#if errors['serverPorts']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['serverPorts']}</p>
		{:else}
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.mieruForm.portsHint')}</p>
		{/if}
	</div>

	<!-- Transport (buttons, not radio) -->
	<div>
		<label class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-2 flex items-center gap-2">
			{$t('outbounds.mieruForm.transport')} *
			{#if serverLocked}<span class="text-[0.65rem] font-normal text-[var(--ctp-overlay0)] bg-[var(--ctp-surface1)] rounded px-1.5 py-0.5">{$t('common.serverOnly')}</span>{/if}
		</label>
		<div class="flex flex-wrap gap-2">
			{#each transports as tr}
				<button
					type="button"
					disabled={serverLocked}
					onclick={() => (transport = tr)}
					class="toggle-btn {transport === tr ? 'selected' : ''} {serverLocked ? 'opacity-70 cursor-not-allowed' : ''}"
				>
					{tr}
				</button>
			{/each}
		</div>
	</div>

	<!-- Credentials -->
	<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
		<div>
			<label for="mieru-username" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.username')} *
			</label>
			<input
				id="mieru-username"
				type="text"
				bind:value={username}
				placeholder="user"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['username'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['username']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['username']}</p>
			{/if}
		</div>
		<div>
			<label for="mieru-password" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
				{$t('outbounds.password')} *
			</label>
			<input
				id="mieru-password"
				type="password"
				bind:value={password}
				placeholder="Password"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['password'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}"
			/>
			{#if errors['password']}
				<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['password']}</p>
			{/if}
		</div>
	</div>

	<!-- Advanced -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)]">
			{$t('common.advanced')} ({$t('common.optional').toLowerCase()})
		</h3>

		<div>
			<label for="mieru-mux" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
				{$t('outbounds.mieruForm.multiplexing')}
			</label>
			<select
				id="mieru-mux"
				bind:value={multiplexing}
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm"
			>
				{#each muxOptions as opt}
					<option value={opt}>{opt || $t('outbounds.mieruForm.multiplexingDefault')}</option>
				{/each}
			</select>
		</div>

		<div>
			<label for="mieru-traffic-pattern" class="block text-xs text-[var(--ctp-overlay0)] mb-1 flex items-center gap-2">
				{$t('outbounds.mieruForm.trafficPattern')}
				{#if serverLocked}<span class="text-[0.65rem] font-normal text-[var(--ctp-overlay0)] bg-[var(--ctp-surface1)] rounded px-1.5 py-0.5">{$t('common.serverOnly')}</span>{/if}
			</label>
			<textarea
				id="mieru-traffic-pattern"
				bind:value={trafficPattern}
				rows="2"
				placeholder="base64"
				readonly={serverLocked}
				class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-code text-xs resize-y read-only:opacity-70 read-only:cursor-not-allowed"
			></textarea>
			<p class="mt-1 text-xs text-[var(--ctp-overlay0)]">{$t('outbounds.mieruForm.trafficPatternHint')}</p>
		</div>
	</div>
</div>

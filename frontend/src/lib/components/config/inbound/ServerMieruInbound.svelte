<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerUsers from './ServerUsers.svelte';
	import type { ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
		errors?: Record<string, string>;
	}
	let { state = $bindable(), errors = {} }: Props = $props();
</script>

<div class="space-y-4">
	<div class="grid grid-cols-3 gap-4">
		<div class="col-span-2">
			<label for="listen" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenAddress')} *</label>
			<input id="listen" type="text" bind:value={state.listen} placeholder="::"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="listenPort" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.listenPort')}</label>
			<input id="listenPort" type="number" bind:value={state.listenPort} min="0" max="65535"
				class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['port'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
			{#if errors['port']}<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['port']}</p>{/if}
		</div>
	</div>

	<div>
		<label for="mieruListenPorts" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">
			{$t('inbounds.server.mieruListenPorts')}
		</label>
		<input id="mieruListenPorts" type="text" bind:value={state.mieruListenPorts}
			placeholder="25010-25012, 26000-26100"
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] {errors['mieruListenPorts'] ? 'border-[var(--ctp-red)]' : 'border-[var(--ctp-surface2)]'}" />
		{#if errors['mieruListenPorts']}
			<p class="mt-1 text-sm text-[var(--ctp-red)]">{errors['mieruListenPorts']}</p>
		{:else}
			<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.mieruListenPortsHint')}</p>
		{/if}
	</div>

	<div>
		<span class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.mieruTransport')} *</span>
		<div class="flex gap-2" role="group" aria-label={$t('inbounds.server.mieruTransport')}>
			<button type="button" class="toggle-btn {state.mieruTransport === 'TCP' ? 'selected' : ''}"
				onclick={() => (state.mieruTransport = 'TCP')}>TCP</button>
			<button type="button" class="toggle-btn {state.mieruTransport === 'UDP' ? 'selected' : ''}"
				onclick={() => (state.mieruTransport = 'UDP')}>UDP</button>
		</div>
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.mieruTransportHint')}</p>
	</div>

	<ServerUsers bind:users={state.users} protocol="mieru" error={errors['userCred'] ?? errors['users']} />

	<div>
		<label for="mieruTP" class="block text-sm font-medium text-[var(--ctp-subtext1)] mb-1">{$t('inbounds.server.trafficPattern')}</label>
		<input id="mieruTP" type="text" bind:value={state.trafficPattern}
			class="w-full px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.trafficPatternHint')}</p>
	</div>

	<div>
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
			<input type="checkbox" bind:checked={state.userHintIsMandatory} />
			{$t('inbounds.server.userHintMandatory')}
		</label>
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.userHintMandatoryHint')}</p>
	</div>
</div>

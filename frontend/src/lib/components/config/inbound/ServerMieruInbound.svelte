<script lang="ts">
	import { t } from 'svelte-i18n';
	import ServerUsers from './ServerUsers.svelte';
	import ListenAddressFields from './ListenAddressFields.svelte';
	import MieruPatternEditor from '../MieruPatternEditor.svelte';
	import type { ServerFormState } from '$lib/utils/serverInbound';

	interface Props {
		state: ServerFormState;
		errors?: Record<string, string>;
		publicHost?: string;
	}
	let { state = $bindable(), errors = {}, publicHost = '' }: Props = $props();
</script>

<div class="space-y-4">
	<ListenAddressFields bind:listen={state.listen} bind:listenPort={state.listenPort} {errors} {publicHost} portRequired={false} />

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

	<MieruPatternEditor bind:value={state.trafficPattern} />

	<div>
		<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
			<input type="checkbox" bind:checked={state.userHintIsMandatory} />
			{$t('inbounds.server.userHintMandatory')}
		</label>
		<p class="mt-1 text-xs text-[var(--ctp-subtext0)]">{$t('inbounds.server.userHintMandatoryHint')}</p>
	</div>
</div>

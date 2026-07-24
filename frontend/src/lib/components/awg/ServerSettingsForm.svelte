<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { AwgServerSettings } from '$lib/types';
	import ObfuscationControl from './ObfuscationControl.svelte';

	interface Props {
		form: AwgServerSettings;
		pubkey?: string;
		saving?: boolean;
		/** Active backend is sing-box: shows awg3-only controls (header protection, CPA/RAT). */
		isSingbox?: boolean;
		/** Server is running: the primary button saves AND applies (restarts the interface). */
		applied?: boolean;
		/** Form differs from the saved settings — enables the buttons and shows the marker. */
		dirty?: boolean;
		onSave: () => void;
		onReset: () => void;
	}

	let { form = $bindable(), pubkey = '', saving = false, isSingbox = false, applied = false, dirty = true, onSave, onReset }: Props = $props();

	// DNS is stored as string[]; edit it as a comma-separated field and write back.
	let dnsText = $state((form.dns ?? []).join(', '));

	function syncDns() {
		form.dns = dnsText
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}
</script>

<div class="field-grid">
	<div class="field">
		<label for="awg-port">{$t('awg.listenPort')}</label>
		<input id="awg-port" type="number" bind:value={form.listen_port} />
		<span class="hint">{$t('awg.listenPortHint')}</span>
	</div>
	<div class="field">
		<label for="awg-host">{$t('awg.serverHost')}</label>
		<input id="awg-host" type="text" bind:value={form.server_host} />
		<span class="hint">{$t('awg.serverHostHint')}</span>
	</div>
	<div class="field">
		<label for="awg-subnet">{$t('awg.subnet')}</label>
		<input id="awg-subnet" type="text" bind:value={form.subnet} />
		<span class="hint">{$t('awg.subnetHint')}</span>
	</div>
	<div class="field">
		<label for="awg-mtu">{$t('awg.mtu')}</label>
		<input id="awg-mtu" type="number" bind:value={form.mtu} />
		<span class="hint">{$t('awg.mtuHint')}</span>
	</div>
	<div class="field">
		<label for="awg-dns">{$t('awg.dns')}</label>
		<input id="awg-dns" type="text" bind:value={dnsText} oninput={syncDns} onblur={syncDns} />
		<span class="hint">{$t('awg.dnsHint')}</span>
	</div>
	<div class="field">
		<label for="awg-wan">{$t('awg.wanIface')}</label>
		<input id="awg-wan" type="text" bind:value={form.wan_iface} placeholder={$t('awg.wanAuto')} />
		<span class="hint">{$t('awg.wanIfaceHint')}</span>
	</div>
	{#if pubkey}
		<div class="field">
			<label for="awg-pub">{$t('awg.module')}</label>
			<input id="awg-pub" type="text" value={pubkey} readonly class="mono-readonly" />
		</div>
	{/if}
</div>

<div class="settings-divider"></div>

<ObfuscationControl bind:obf={form.obf} bind:preset={form.obf_preset} {isSingbox} />

{#if isSingbox}
	<div class="hp-row">
		<button
			type="button"
			class="toggle-btn"
			class:selected={form.header_protection}
			onclick={() => (form.header_protection = !form.header_protection)}
		>
			{$t('awg.headerProtection')}
		</button>
		<span class="hint">{$t('awg.headerProtectionHint')}</span>
	</div>
{/if}

<div class="save-row">
	<span class="save-status" class:dirty>
		{#if dirty}
			<span class="dot"></span>{$t('awg.unsavedChanges')}
		{:else}
			{$t('awg.upToDate')}
		{/if}
	</span>
	<button type="button" class="btn-ghost-sm" onclick={onReset} disabled={saving || !dirty}>{$t('awg.reset')}</button>
	<button type="button" class="btn-save" onclick={onSave} disabled={saving || !dirty}>
		{saving ? $t('common.saving') : applied ? $t('awg.saveAndApply') : $t('awg.save')}
	</button>
</div>

<style>
	.field-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}
	.field label {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--ctp-subtext1);
	}
	.field .hint {
		font-size: 0.75rem;
		color: var(--ctp-overlay0);
	}
	.field input {
		background: var(--ctp-base);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.375rem;
		padding: 0.5rem 0.7rem;
		color: var(--ctp-text);
		font-size: 0.875rem;
		transition: border-color 0.15s ease;
	}
	.field input:focus {
		outline: none;
		border-color: var(--ctp-primary);
	}
	.field input::placeholder {
		color: var(--ctp-overlay0);
	}
	.mono-readonly {
		color: var(--ctp-overlay1);
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
	.settings-divider {
		height: 1px;
		background: var(--ctp-surface0);
		margin: 1.5rem 0;
	}
	.hp-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
		flex-wrap: wrap;
	}
	.hp-row .hint {
		font-size: 0.75rem;
		color: var(--ctp-overlay0);
		max-width: 46ch;
	}
	.save-row {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1.5rem;
		padding-top: 1.25rem;
		border-top: 1px solid var(--ctp-surface0);
	}
	.save-status {
		margin-right: auto;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.78rem;
		color: var(--ctp-overlay0);
	}
	.save-status.dirty {
		color: var(--ctp-primary);
		font-weight: 500;
	}
	.save-status .dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--ctp-primary);
	}
	.btn-ghost-sm:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.btn-save {
		padding: 0.5rem 1.25rem;
		border-radius: 0.375rem;
		border: none;
		background: var(--ctp-primary);
		color: #1a1a1a;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-save:hover {
		background: var(--ctp-primary-hover, #e08a6c);
	}
	.btn-save:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.btn-ghost-sm {
		padding: 0.5rem 0.75rem;
		border-radius: 0.375rem;
		border: 1px solid transparent;
		background: transparent;
		color: var(--ctp-subtext0);
		font-weight: 500;
		cursor: pointer;
	}
	.btn-ghost-sm:hover {
		color: var(--ctp-text);
	}
	@media (max-width: 720px) {
		.field-grid {
			grid-template-columns: 1fr;
		}
	}
</style>

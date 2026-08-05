<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { MtprotoSettings, RoutableTag } from '$lib/types';
	import { configReadOnly } from '$lib/stores';

	interface Props {
		form: MtprotoSettings;
		/** The masking domain as last saved, so the invalidation warning shows
		 *  only when it is actually being changed. */
		savedDomain: string;
		/** Outbounds and endpoints Telegram traffic can be routed through. */
		outbounds?: RoutableTag[];
		saving?: boolean;
		/** Form differs from the saved settings — enables the buttons. */
		dirty?: boolean;
		onSave: () => void;
		onReset: () => void;
	}

	let {
		form = $bindable(),
		savedDomain,
		outbounds = [],
		saving = false,
		dirty = true,
		onSave,
		onReset
	}: Props = $props();

	// Only warn when the domain is genuinely being changed, and only when there
	// was one to invalidate — on first setup there are no links out yet.
	const domainChanged = $derived(savedDomain !== '' && form.masking_domain.trim() !== savedDomain);

	// The chosen tag may have been renamed or deleted elsewhere in the config
	// since it was saved. Keeping it as an option rather than silently snapping
	// the select back to Direct is the difference between "your exit is gone" and
	// "your exit quietly became your own IP".
	const missingOutbound = $derived(
		form.outbound !== '' && !outbounds.some((o) => o.tag === form.outbound)
	);

	// The loopback port only exists to carry traffic to that exit, so it has no
	// meaning — and no reason to be editable — when Telegram goes out directly.
	const routed = $derived(form.outbound !== '');
</script>

<div class="field-grid">
	<div class="field">
		<label for="mt-listen">{$t('telegram.listen')}</label>
		<input id="mt-listen" type="text" bind:value={form.listen} placeholder="0.0.0.0:9443" />
		<span class="hint">{$t('telegram.listenHint')}</span>
	</div>
	<div class="field">
		<label for="mt-domain">{$t('telegram.maskingDomain')}</label>
		<input id="mt-domain" type="text" bind:value={form.masking_domain} placeholder="storage.googleapis.com" />
		<span class="hint">{$t('telegram.maskingDomainHint')}</span>
	</div>
	<div class="field">
		<label for="mt-host">{$t('telegram.publicHost')}</label>
		<input id="mt-host" type="text" bind:value={form.public_host} />
		<span class="hint">{$t('telegram.publicHostHint')}</span>
	</div>
	<div class="field">
		<label for="mt-port">{$t('telegram.publicPort')}</label>
		<input id="mt-port" type="number" bind:value={form.public_port} />
		<span class="hint">{$t('telegram.publicPortHint')}</span>
	</div>
	<div class="field">
		<label for="mt-conc">{$t('telegram.concurrency')}</label>
		<input id="mt-conc" type="number" bind:value={form.concurrency} />
	</div>
	<div class="field">
		<label for="mt-idle">{$t('telegram.idleTimeout')}</label>
		<input id="mt-idle" type="number" bind:value={form.idle_timeout_sec} />
	</div>
	<div class="field">
		<label for="mt-preferip">{$t('telegram.preferIp')}</label>
		<select id="mt-preferip" bind:value={form.prefer_ip}>
			<option value="">prefer-ipv6</option>
			<option value="prefer-ipv4">prefer-ipv4</option>
			<option value="prefer-ipv6">prefer-ipv6</option>
			<option value="only-ipv4">only-ipv4</option>
			<option value="only-ipv6">only-ipv6</option>
		</select>
	</div>
	<div class="field">
		<label for="mt-outbound">{$t('telegram.outbound')}</label>
		<select id="mt-outbound" bind:value={form.outbound}>
			<option value="">{$t('telegram.outboundDirect')}</option>
			{#if missingOutbound}
				<option value={form.outbound}>{form.outbound} — {$t('telegram.outboundMissing')}</option>
			{/if}
			{#each outbounds as o (o.tag)}
				<option value={o.tag}>{o.tag} · {o.type}</option>
			{/each}
		</select>
		<span class="hint">{$t('telegram.outboundHint')}</span>
	</div>
	{#if routed}
		<div class="field">
			<label for="mt-socksport">{$t('telegram.socksPort')}</label>
			<input id="mt-socksport" type="number" bind:value={form.socks_port} placeholder="1080" />
			<span class="hint">{$t('telegram.socksPortHint')}</span>
		</div>
	{/if}
</div>

{#if missingOutbound}
	<div class="domain-warn">
		<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /></svg>
		<span>{$t('telegram.outboundMissingWarning')}</span>
	</div>
{/if}

{#if outbounds.length === 0 && !missingOutbound}
	<p class="no-outbounds">{$t('telegram.outboundNone')}</p>
{/if}

{#if domainChanged}
	<div class="domain-warn">
		<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4" /><path d="M12 17h.01" /><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /></svg>
		<span>{$t('telegram.maskingDomainWarning')}</span>
	</div>
{/if}

<div class="save-row">
	<button type="button" class="btn-ghost-sm" onclick={onReset} disabled={saving || !dirty}>{$t('telegram.cancel')}</button>
	<button
		type="button"
		class="btn-save"
		onclick={onSave}
		disabled={saving || !dirty || $configReadOnly}
		title={$configReadOnly ? $t('readOnly.saveBlocked') : ''}
	>
		{saving ? $t('common.saving') : $t('telegram.save')}
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
	.field input,
	.field select {
		background: var(--ctp-base);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.375rem;
		padding: 0.5rem 0.7rem;
		color: var(--ctp-text);
		font-size: 0.875rem;
		transition: border-color 0.15s ease;
	}
	.field input:focus,
	.field select:focus {
		outline: none;
		border-color: var(--ctp-primary);
	}
	.field input::placeholder {
		color: var(--ctp-overlay0);
	}
	.domain-warn {
		display: flex;
		align-items: flex-start;
		gap: 0.55rem;
		margin-top: 1.25rem;
		padding: 0.7rem 0.9rem;
		border-radius: 0.5rem;
		border: 1px solid color-mix(in srgb, var(--ctp-yellow, #e5c890) 45%, transparent);
		background: color-mix(in srgb, var(--ctp-yellow, #e5c890) 10%, transparent);
		color: var(--ctp-subtext1);
		font-size: 0.8125rem;
		line-height: 1.4;
	}
	.domain-warn svg {
		flex: none;
		margin-top: 1px;
		color: var(--ctp-yellow, #e5c890);
	}
	.no-outbounds {
		margin: 1rem 0 0;
		font-size: 0.8125rem;
		color: var(--ctp-overlay0);
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
	.btn-save {
		padding: 0.5rem 1.25rem;
		border-radius: 0.375rem;
		border: none;
		background: var(--ctp-primary);
		color: #1a1a1a;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-save:hover:not(:disabled) {
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
	.btn-ghost-sm:hover:not(:disabled) {
		color: var(--ctp-text);
	}
	.btn-ghost-sm:disabled {
		opacity: 0.4;
		cursor: default;
	}
	@media (max-width: 720px) {
		.field-grid {
			grid-template-columns: 1fr;
		}
	}
</style>

<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { AwgObf } from '$lib/types';
	import { PRESETS, OBF_J, OBF_S, OBF_STR } from './obf';
	import Modal from '$lib/components/shared/Modal.svelte';

	interface Props {
		obf: AwgObf;
		preset: string;
		/** Shows the awg3-only fields (CPA/RAT): true on sing-box always, true on the
		 * kernel backend only when the host confirms awg3 capability. */
		awg3Available?: boolean;
		/** Shows the AWG 3.1 device flags. A 3.0 module ignores them without an
		 * error, so gating on awg3Available alone would leave the server looking
		 * configured while running as before. */
		awg31Available?: boolean;
	}

	// Bindable so the parent's form.obf / form.obf_preset stay in sync.
	let { obf = $bindable(), preset = $bindable(), awg3Available = false, awg31Available = false }: Props = $props();

	let advOpen = $state(false);
	// Turning random trailers ON is confirmed explicitly: it drops every client
	// below 3.1 with no error on their side, and every issued config has to be
	// reissued. Turning it OFF needs no confirmation — that only restores
	// compatibility.
	let confirmTrailers = $state(false);

	function onTrailersToggle(next: boolean) {
		if (next) {
			confirmTrailers = true;
			return;
		}
		obf.random_trailers = false;
		markCustom();
	}

	function acceptTrailers() {
		obf.random_trailers = true;
		confirmTrailers = false;
		markCustom();
	}

	function pick(name: string) {
		// Presets now carry the awg3 fields too (off clears them), so apply verbatim.
		obf = PRESETS[name]();
		preset = name;
	}

	// Editing any advanced field detaches from a named preset.
	function markCustom() {
		preset = 'custom';
	}

	const active = $derived(preset !== 'off');
</script>

<div class="obf-head">
	<div>
		<div class="obf-title">{$t('awg.obfuscation')}</div>
		<div class="obf-desc">{$t('awg.obfuscationDesc')}</div>
	</div>
	<span class="status-badge {active ? 'success' : 'info'}">
		{active ? $t('awg.obfActive') : $t('awg.obfInactive')}
	</span>
</div>

<div class="preset-row">
	{#each [['off', 'awg.obfOff'], ['dns', 'awg.obfDns'], ['web', 'awg.obfWeb'], ['stealth', 'awg.obfStealth']] as [key, label] (key)}
		<button type="button" class="preset-btn {preset === key ? 'selected' : ''}" onclick={() => pick(key)}>{$t(label)}</button>
	{/each}
	{#if preset === 'custom'}<span class="status-badge info">{$t('awg.obfCustom')}</span>{/if}
</div>

<div class="adv" class:open={advOpen}>
	<button type="button" class="adv-head" onclick={() => (advOpen = !advOpen)}>
		<span class="chev">
			<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
		</span>
		{$t('awg.advanced')}
		<span class="a-spacer"></span>
		<span class="adv-keys">Jc · Jmin · Jmax · S1–S4 · H1–H4{awg3Available ? ' · CPA · RAT' : ''}</span>
	</button>
	{#if advOpen}
		<div class="adv-body">
			<p class="adv-note">{$t('awg.advancedNote')}</p>
			<div class="adv-grid">
				{#each OBF_J as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="number" bind:value={obf[k]} oninput={markCustom} />
					</div>
				{/each}
			</div>
			<div class="adv-grid">
				{#each OBF_S as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="number" bind:value={obf[k]} oninput={markCustom} />
					</div>
				{/each}
			</div>
			<div class="adv-grid">
				{#each OBF_STR as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="text" bind:value={obf[k]} oninput={markCustom} />
					</div>
				{/each}
			</div>
			{#if awg3Available}
			<div class="adv-grid two">
				<div class="mini-field">
					<label for="obf-cpa">{$t('awg.contentPadding')}</label>
					<input
						id="obf-cpa"
						type="text"
						placeholder="0-64"
						value={obf.content_padding_addition ?? ''}
						oninput={(e) => {
							obf.content_padding_addition = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.contentPaddingHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rat">{$t('awg.rekeyAfterTime')}</label>
					<input
						id="obf-rat"
						type="text"
						placeholder="120-180"
						value={obf.rekey_after_time ?? ''}
						oninput={(e) => {
							obf.rekey_after_time = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.rekeyAfterTimeHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rkt">{$t('awg.rekeyTimeout')}</label>
					<input
						id="obf-rkt"
						type="text"
						placeholder="5"
						value={obf.rekey_timeout ?? ''}
						oninput={(e) => {
							obf.rekey_timeout = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.rekeyTimeoutHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rjt">{$t('awg.rejectAfterTime')}</label>
					<input
						id="obf-rjt"
						type="text"
						placeholder="180"
						value={obf.reject_after_time ?? ''}
						oninput={(e) => {
							obf.reject_after_time = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.rejectAfterTimeHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-kat">{$t('awg.keepaliveTimeout')}</label>
					<input
						id="obf-kat"
						type="text"
						placeholder="25"
						value={obf.keepalive_timeout ?? ''}
						oninput={(e) => {
							obf.keepalive_timeout = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.keepaliveTimeoutHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-mha">{$t('awg.maxHandshakeAttempts')}</label>
					<input
						id="obf-mha"
						type="text"
						placeholder="18"
						value={obf.max_handshake_attempts ?? ''}
						oninput={(e) => {
							obf.max_handshake_attempts = e.currentTarget.value;
							markCustom();
						}}
					/>
					<span class="mini-hint">{$t('awg.maxHandshakeAttemptsHint')}</span>
				</div>
			</div>
			{/if}
			{#if awg31Available}
			<div class="flag-list">
				<label class="flag">
					<input
						type="checkbox"
						checked={obf.random_trailers ?? false}
						onchange={(e) => onTrailersToggle(e.currentTarget.checked)}
					/>
					<span>
						<span class="flag-name">{$t('awg.randomTrailers')}</span>
						<span class="flag-hint">{$t('awg.randomTrailersHint')}</span>
						<span class="flag-warn">{$t('awg.randomTrailersWarn')}</span>
					</span>
				</label>
				<label class="flag">
					<input
						type="checkbox"
						checked={obf.disable_cookies ?? false}
						onchange={(e) => {
							obf.disable_cookies = e.currentTarget.checked;
							markCustom();
						}}
					/>
					<span>
						<span class="flag-name">{$t('awg.disableCookies')}</span>
						<span class="flag-hint">{$t('awg.disableCookiesHint')}</span>
					</span>
				</label>
			</div>
			{/if}
		</div>
	{/if}
</div>

<Modal open={confirmTrailers} title={$t('awg.randomTrailersConfirmTitle')} size="sm" onClose={() => (confirmTrailers = false)}>
	<p class="confirm-text">{$t('awg.randomTrailersConfirmBody')}</p>
	<p class="confirm-text">{$t('awg.randomTrailersConfirmReissue')}</p>
	{#snippet footer()}
		<button type="button" class="preset-btn" onclick={() => (confirmTrailers = false)}>{$t('common.cancel')}</button>
		<button type="button" class="preset-btn selected" onclick={acceptTrailers}>{$t('awg.randomTrailersConfirmAccept')}</button>
	{/snippet}
</Modal>

<style>
	.flag-list {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		margin-top: 1rem;
	}

	.flag {
		display: flex;
		align-items: flex-start;
		gap: 0.625rem;
		cursor: pointer;
	}

	.flag input {
		margin-top: 0.2rem;
		flex: none;
	}

	.flag-name,
	.flag-hint,
	.flag-warn {
		display: block;
	}

	.flag-name {
		font-weight: 600;
	}

	.flag-hint,
	.flag-warn {
		font-size: 0.8125rem;
		line-height: 1.45;
	}

	.flag-hint {
		opacity: 0.75;
	}

	.flag-warn {
		margin-top: 0.25rem;
		color: var(--warn, #b45309);
	}

	.confirm-text {
		margin: 0 0 0.75rem;
		line-height: 1.5;
	}

	.obf-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.75rem;
	}
	.obf-title {
		font-weight: 600;
	}
	.obf-desc {
		color: var(--ctp-overlay1);
		font-size: 0.8125rem;
		margin-top: 1px;
	}
	.preset-row {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}
	.adv {
		border: 1px dashed var(--ctp-surface2);
		border-radius: 0.5rem;
		overflow: hidden;
	}
	.adv-head {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 1rem;
		background: var(--ctp-base);
		border: none;
		text-align: left;
		color: var(--ctp-subtext1);
		font-weight: 500;
		font-size: 0.875rem;
	}
	.adv-head:hover {
		color: var(--ctp-text);
	}
	.adv-head .chev {
		color: var(--ctp-overlay1);
		transition: transform 0.15s;
		display: flex;
	}
	.adv.open .chev {
		transform: rotate(90deg);
	}
	.a-spacer {
		flex: 1;
	}
	.adv-keys {
		color: var(--ctp-overlay0);
		font-size: 0.75rem;
		font-weight: 400;
	}
	.adv-body {
		padding: 1rem;
		border-top: 1px dashed var(--ctp-surface2);
	}
	.adv-note {
		color: var(--ctp-overlay0);
		font-size: 0.75rem;
		margin: 0 0 1rem;
	}
	.adv-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.75rem;
	}
	.adv-grid + .adv-grid {
		margin-top: 0.75rem;
	}
	.adv-grid.two {
		grid-template-columns: repeat(2, 1fr);
	}
	.mini-hint {
		font-size: 0.6875rem;
		color: var(--ctp-overlay0);
	}
	.mini-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0; /* let the grid track shrink; else long mono values overflow-clip */
	}
	.mini-field label {
		font-size: 0.6875rem;
		font-weight: 600;
		color: var(--ctp-overlay1);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.mini-field input {
		width: 100%;
		min-width: 0;
		box-sizing: border-box;
		background: var(--ctp-mantle);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.375rem;
		padding: 0.4rem 0.5rem;
		color: var(--ctp-text);
		font-family: inherit;
		font-size: 0.8125rem;
	}
	.mini-field input:focus {
		outline: none;
		border-color: var(--ctp-primary);
	}
	@media (max-width: 720px) {
		.adv-grid {
			grid-template-columns: repeat(2, 1fr);
		}
	}
	/* Narrow phones: the CPA/RAT/timer grid carries long hints — stack it. */
	@media (max-width: 480px) {
		.adv-grid.two {
			grid-template-columns: 1fr;
		}
	}
</style>

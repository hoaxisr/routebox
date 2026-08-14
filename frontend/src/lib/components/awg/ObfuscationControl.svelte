<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { AwgObf } from '$lib/types';
	import { PRESETS, OBF_J, OBF_S, OBF_STR, AWG_VERSIONS, applyVersion, awgVersion, type AwgVersion } from './obf';
	import Modal from '$lib/components/shared/Modal.svelte';

	interface Props {
		obf: AwgObf;
		preset: string;
		/** AWG 3.0 header protection — the switch that decides which protocol
		 * version the client's config actually is, so the version row owns it. */
		headerProtection: boolean;
		/** Shows the awg3-only fields (CPA/RAT): true on sing-box always, true on the
		 * kernel backend only when the host confirms awg3 capability. */
		awg3Available?: boolean;
		/** Shows the AWG 3.1 device flags. A 3.0 module ignores them without an
		 * error, so gating on awg3Available alone would leave the server looking
		 * configured while running as before. */
		awg31Available?: boolean;
	}

	// Bindable so the parent's form.obf / form.obf_preset / form.header_protection stay in sync.
	let { obf = $bindable(), preset = $bindable(), headerProtection = $bindable(false), awg3Available = false, awg31Available = false }: Props = $props();

	let advOpen = $state(false);
	// Moving up to 3.1 is confirmed explicitly: random trailers drop every client
	// below 3.1 with no error on their side, and every issued config has to be
	// reissued. Moving back down needs no confirmation — that only restores
	// compatibility.
	let confirmTrailers = $state(false);

	const version = $derived(awgVersion(obf, headerProtection));
	const versions = $derived(AWG_VERSIONS.filter((v) => v !== '3.1' || awg31Available));

	function pick(name: string) {
		// The preset name is not just a UI label: the backend keys the client's CPS
		// mimicry (I1-I5) off it, so it survives hand-editing the fields below and
		// only ever changes here. Editing used to rewrite it to "custom", which
		// silently emptied the mimicry set on every export (#76).
		const v = name === 'off' ? '2.0' : version;
		obf = applyVersion(PRESETS[name](), v, name);
		preset = name;
		headerProtection = v !== '2.0';
	}

	function setVersion(v: AwgVersion) {
		if (v === '3.1' && !obf.random_trailers) {
			confirmTrailers = true;
			return;
		}
		obf = applyVersion(obf, v, preset);
		headerProtection = v !== '2.0';
	}

	function acceptTrailers() {
		obf = applyVersion(obf, '3.1', preset);
		headerProtection = true;
		confirmTrailers = false;
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
</div>

{#if awg3Available}
	<div class="ver-row">
		<span class="ver-label">{$t('awg.protocolVersion')}</span>
		{#each versions as v (v)}
			<button type="button" class="toggle-btn" class:selected={version === v} onclick={() => setVersion(v)}>AWG {v}</button>
		{/each}
	</div>
	<p class="ver-hint">{$t('awg.protocolVersionHint')}</p>
{/if}

<div class="adv" class:open={advOpen}>
	<button type="button" class="adv-head" onclick={() => (advOpen = !advOpen)}>
		<span class="chev">
			<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
		</span>
		{$t('awg.advanced')}
		<span class="a-spacer"></span>
		<span class="adv-keys">Jc · Jmin · Jmax · S1–S4 · H1–H4{awg3Available && version !== '2.0' ? ' · CPA · RAT' : ''}</span>
	</button>
	{#if advOpen}
		<div class="adv-body">
			<p class="adv-note">{$t('awg.advancedNote')}</p>
			<div class="adv-grid">
				{#each OBF_J as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="number" bind:value={obf[k]} />
					</div>
				{/each}
			</div>
			<div class="adv-grid">
				{#each OBF_S as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="number" bind:value={obf[k]} />
					</div>
				{/each}
			</div>
			<div class="adv-grid">
				{#each OBF_STR as k (k)}
					<div class="mini-field">
						<label for="obf-{k}">{k}</label>
						<input id="obf-{k}" type="text" bind:value={obf[k]} />
					</div>
				{/each}
			</div>
			<!-- Version-gated: at AWG 2.0 these six are cleared and never reach a
			     client, so showing them filled is the mismatch #76 reported. -->
			{#if awg3Available && version !== '2.0'}
			<div class="adv-grid two">
				<div class="mini-field">
					<label for="obf-cpa">{$t('awg.contentPadding')}</label>
					<input id="obf-cpa" type="text" placeholder="0-64" bind:value={obf.content_padding_addition} />
					<span class="mini-hint">{$t('awg.contentPaddingHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rat">{$t('awg.rekeyAfterTime')}</label>
					<input id="obf-rat" type="text" placeholder="120-180" bind:value={obf.rekey_after_time} />
					<span class="mini-hint">{$t('awg.rekeyAfterTimeHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rkt">{$t('awg.rekeyTimeout')}</label>
					<input id="obf-rkt" type="text" placeholder="5" bind:value={obf.rekey_timeout} />
					<span class="mini-hint">{$t('awg.rekeyTimeoutHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-rjt">{$t('awg.rejectAfterTime')}</label>
					<input id="obf-rjt" type="text" placeholder="180" bind:value={obf.reject_after_time} />
					<span class="mini-hint">{$t('awg.rejectAfterTimeHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-kat">{$t('awg.keepaliveTimeout')}</label>
					<input id="obf-kat" type="text" placeholder="25" bind:value={obf.keepalive_timeout} />
					<span class="mini-hint">{$t('awg.keepaliveTimeoutHint')}</span>
				</div>
				<div class="mini-field">
					<label for="obf-mha">{$t('awg.maxHandshakeAttempts')}</label>
					<input id="obf-mha" type="text" placeholder="18" bind:value={obf.max_handshake_attempts} />
					<span class="mini-hint">{$t('awg.maxHandshakeAttemptsHint')}</span>
				</div>
			</div>
			{/if}
			<!-- RandomTrailers is what AWG 3.1 IS here, so the version row owns it;
			     DisableCookies is responder-side policy and stays a free choice. -->
			{#if version === '3.1'}
			<div class="flag-list">
				<label class="flag">
					<input type="checkbox" checked={obf.disable_cookies ?? false} onchange={(e) => (obf.disable_cookies = e.currentTarget.checked)} />
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
	.flag-hint {
		display: block;
	}

	.flag-name {
		font-weight: 600;
	}

	.flag-hint {
		font-size: 0.8125rem;
		line-height: 1.45;
	}

	.flag-hint {
		opacity: 0.75;
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
	.ver-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	/* .toggle-btn is flex:1 by default — three of them would each eat a third of a
	   desktop row. These are short labels, so size them to their text. */
	.ver-row .toggle-btn {
		flex: 0 0 auto;
		white-space: nowrap;
	}
	.ver-label {
		font-size: 0.8125rem;
		color: var(--ctp-subtext1);
		margin-right: 0.25rem;
	}
	/* Phone: the label and three buttons do not share a 480px row — give the label
	   its own line so the versions stay together. */
	@media (max-width: 560px) {
		.ver-label {
			flex: 0 0 100%;
			margin-right: 0;
		}
	}
	.ver-hint {
		margin: 0.4rem 0 1rem;
		font-size: 0.75rem;
		color: var(--ctp-overlay0);
		max-width: 60ch;
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

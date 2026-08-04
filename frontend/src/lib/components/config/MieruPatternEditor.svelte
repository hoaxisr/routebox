<script lang="ts">
	import { t } from 'svelte-i18n';
	import {
		decodeTrafficPattern,
		encodeTrafficPattern,
		rotationParts,
		rotationValue,
		MAX_SEED,
		MAX_SLEEP_MS,
		MAX_NONCE_LEN,
		MAX_PADDING_LEN,
		MAX_ROTATION_STEPS,
		type TrafficPattern
	} from '$lib/utils/mieruPattern';

	interface Props {
		/** base64 `traffic_pattern` as sing-box stores it; "" = feature off. */
		value: string;
		readonly?: boolean;
	}
	let { value = $bindable(''), readonly = false }: Props = $props();

	let decodedOnce = decodeTrafficPattern(value);
	// A blob we cannot parse is shown raw rather than silently rewritten.
	let raw = $state(decodedOnce === null ? value : '');
	let p = $state<TrafficPattern>(decodedOnce ?? {});
	let advanced = $state(hasAdvanced(p));
	let lastEmitted = value;

	function hasAdvanced(x: TrafficPattern): boolean {
		return !!(x.tcpFragment || x.nonce || x.padding || x.lowEntropy);
	}

	// Pick up changes made by the parent (link import, protocol switch).
	$effect(() => {
		if (value === lastEmitted) return;
		lastEmitted = value;
		const decoded = decodeTrafficPattern(value);
		raw = decoded === null ? value : '';
		p = decoded ?? {};
		advanced = hasAdvanced(p);
	});

	function emit() {
		const encoded = encodeTrafficPattern(p);
		lastEmitted = encoded;
		value = encoded;
	}

	const on = $derived(raw !== '' || Object.keys(p).length > 0);

	function randomSeed(): number {
		return Math.floor(Math.random() * (MAX_SEED + 1));
	}

	function turnOn() {
		// A pattern of nothing encodes to "" (= off), so start from a stable seed.
		p = { seed: randomSeed() };
		raw = '';
		emit();
	}

	function turnOff() {
		p = {};
		raw = '';
		advanced = false;
		emit();
	}

	/** Reads a number input: blank means "let mieru generate it". */
	function num(e: Event): number | undefined {
		const s = (e.currentTarget as HTMLInputElement).value.trim();
		if (s === '') return undefined;
		const n = Number(s);
		return Number.isFinite(n) ? Math.round(n) : undefined;
	}

	/** Sets a field on a submessage, creating/dropping the submessage as needed. */
	function set<K extends keyof TrafficPattern>(key: K, patch: Partial<NonNullable<TrafficPattern[K]>>) {
		const merged = { ...(p[key] as object ?? {}), ...patch } as Record<string, unknown>;
		for (const k of Object.keys(merged)) if (merged[k] === undefined) delete merged[k];
		if (Object.keys(merged).length === 0) delete p[key];
		else (p[key] as unknown) = merged;
		emit();
	}

	const nonceTypes = [
		{ v: undefined, label: 'auto' },
		{ v: 0, label: 'random' },
		{ v: 1, label: 'printable' },
		{ v: 2, label: 'printableSubset' },
		{ v: 3, label: 'fixed' }
	];
	const lowEntropyModes = [
		{ v: undefined, label: 'auto' },
		{ v: 0, label: 'off' },
		{ v: 1, label: '32' },
		{ v: 2, label: '40' },
		{ v: 3, label: '48' },
		{ v: 4, label: '56' }
	];

	const rot = $derived(rotationParts(p.lowEntropy?.maskRotation ?? 0));
	const hexList = $derived((p.nonce?.customHexStrings ?? []).join(', '));

	const inputCls =
		'w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] text-sm disabled:opacity-70 disabled:cursor-not-allowed';
	const hintCls = 'mt-1 text-xs text-[var(--ctp-overlay0)]';
</script>

<div class="space-y-3">
	<div class="flex flex-wrap items-center gap-2">
		<span class="text-sm font-medium text-[var(--ctp-subtext1)]">{$t('mieruPattern.title')}</span>
		{#if readonly}
			<span class="text-[0.65rem] text-[var(--ctp-overlay0)] bg-[var(--ctp-surface1)] rounded px-1.5 py-0.5">
				{$t('common.serverOnly')}
			</span>
		{/if}
		<div class="ml-auto flex gap-2">
			<button type="button" disabled={readonly} class="toggle-btn {on ? '' : 'selected'}" onclick={turnOff}>
				{$t('mieruPattern.off')}
			</button>
			<button type="button" disabled={readonly} class="toggle-btn {on ? 'selected' : ''}" onclick={turnOn}>
				{$t('mieruPattern.on')}
			</button>
		</div>
	</div>
	<p class={hintCls}>{$t('mieruPattern.intro')}</p>

	{#if raw}
		<div class="rounded-lg border border-[var(--ctp-red)] p-3 space-y-2">
			<p class="text-xs text-[var(--ctp-red)]">{$t('mieruPattern.unparsed')}</p>
			<input
				type="text"
				disabled={readonly}
				value={raw}
				oninput={(e) => {
					raw = e.currentTarget.value;
					lastEmitted = raw;
					value = raw;
				}}
				class="{inputCls} font-code text-xs"
			/>
		</div>
	{:else if on}
		<div class="bg-[var(--ctp-surface0)] rounded-lg p-4 space-y-4">
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				<div>
					<label for="tp-seed" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('mieruPattern.seed')}</label>
					<div class="flex gap-2">
						<input
							id="tp-seed"
							type="number"
							min="0"
							max={MAX_SEED}
							disabled={readonly}
							value={p.seed ?? ''}
							oninput={(e) => {
								p.seed = num(e);
								emit();
							}}
							placeholder={$t('mieruPattern.autoPlaceholder')}
							class={inputCls}
						/>
						<button
							type="button"
							disabled={readonly}
							class="toggle-btn shrink-0"
							onclick={() => {
								p.seed = randomSeed();
								emit();
							}}
						>
							{$t('mieruPattern.regenerate')}
						</button>
					</div>
					<p class={hintCls}>{$t('mieruPattern.seedHint')}</p>
				</div>
				<div>
					<span class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('mieruPattern.unlockAll')}</span>
					<label class="flex items-center gap-2 text-sm text-[var(--ctp-text)]">
						<input
							type="checkbox"
							disabled={readonly}
							checked={p.unlockAll ?? false}
							onchange={(e) => {
								p.unlockAll = e.currentTarget.checked ? true : undefined;
								emit();
							}}
						/>
						{$t('mieruPattern.unlockAllLabel')}
					</label>
					<p class={hintCls}>{$t('mieruPattern.unlockAllHint')}</p>
				</div>
			</div>

			<button
				type="button"
				class="text-xs text-[var(--ctp-primary)] hover:underline"
				onclick={() => (advanced = !advanced)}
			>
				{advanced ? $t('mieruPattern.hideAdvanced') : $t('mieruPattern.showAdvanced')}
			</button>

			{#if advanced}
				<div class="space-y-4 border-t border-[var(--ctp-surface2)] pt-4">
					<!-- TCP fragmentation -->
					<div>
						<span class="block text-xs font-medium text-[var(--ctp-subtext1)] mb-1">{$t('mieruPattern.tcpFragment')}</span>
						<div class="flex flex-wrap gap-2">
							{#each [{ v: undefined, label: 'auto' }, { v: true, label: 'onValue' }, { v: false, label: 'offValue' }] as opt}
								<button
									type="button"
									disabled={readonly}
									class="toggle-btn {(p.tcpFragment?.enable ?? undefined) === opt.v ? 'selected' : ''}"
									onclick={() => set('tcpFragment', { enable: opt.v })}
								>
									{$t(`mieruPattern.${opt.label}`)}
								</button>
							{/each}
						</div>
						{#if p.tcpFragment?.enable}
							<div class="mt-2">
								<label for="tp-sleep" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
									{$t('mieruPattern.maxSleepMs')}
								</label>
								<input
									id="tp-sleep"
									type="number"
									min="0"
									max={MAX_SLEEP_MS}
									disabled={readonly}
									value={p.tcpFragment?.maxSleepMs ?? ''}
									oninput={(e) => set('tcpFragment', { maxSleepMs: num(e) })}
									placeholder={$t('mieruPattern.autoPlaceholder')}
									class={inputCls}
								/>
							</div>
						{/if}
						<p class={hintCls}>{$t('mieruPattern.tcpFragmentHint')}</p>
					</div>

					<!-- Nonce -->
					<div>
						<span class="block text-xs font-medium text-[var(--ctp-subtext1)] mb-1">{$t('mieruPattern.nonce')}</span>
						<div class="flex flex-wrap gap-2">
							{#each nonceTypes as opt}
								<button
									type="button"
									disabled={readonly}
									class="toggle-btn {p.nonce?.type === opt.v ? 'selected' : ''}"
									onclick={() => set('nonce', { type: opt.v })}
								>
									{$t(`mieruPattern.nonceType.${opt.label}`)}
								</button>
							{/each}
						</div>
						<p class={hintCls}>{$t('mieruPattern.nonceHint')}</p>

						{#if p.nonce?.type === 3}
							<div class="mt-2">
								<label for="tp-hex" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
									{$t('mieruPattern.customHex')}
								</label>
								<input
									id="tp-hex"
									type="text"
									disabled={readonly}
									value={hexList}
									oninput={(e) => {
										const list = e.currentTarget.value
											.split(',')
											.map((s) => s.trim())
											.filter(Boolean);
										set('nonce', { customHexStrings: list.length ? list : undefined });
									}}
									placeholder="00010203, 04050607"
									class="{inputCls} font-code"
								/>
								<p class={hintCls}>{$t('mieruPattern.customHexHint')}</p>
							</div>
						{:else if p.nonce?.type !== undefined && p.nonce.type !== 0}
							<div class="mt-2 grid grid-cols-2 gap-4">
								<div>
									<label for="tp-minlen" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
										{$t('mieruPattern.minLen')}
									</label>
									<input
										id="tp-minlen"
										type="number"
										min="0"
										max={MAX_NONCE_LEN}
										disabled={readonly}
										value={p.nonce?.minLen ?? ''}
										oninput={(e) => set('nonce', { minLen: num(e) })}
										placeholder={$t('mieruPattern.autoPlaceholder')}
										class={inputCls}
									/>
								</div>
								<div>
									<label for="tp-maxlen" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
										{$t('mieruPattern.maxLen')}
									</label>
									<input
										id="tp-maxlen"
										type="number"
										min="0"
										max={MAX_NONCE_LEN}
										disabled={readonly}
										value={p.nonce?.maxLen ?? ''}
										oninput={(e) => set('nonce', { maxLen: num(e) })}
										placeholder={$t('mieruPattern.autoPlaceholder')}
										class={inputCls}
									/>
								</div>
							</div>
						{/if}

						{#if p.nonce?.type !== undefined}
							<label class="mt-2 flex items-center gap-2 text-sm text-[var(--ctp-text)]">
								<input
									type="checkbox"
									disabled={readonly}
									checked={p.nonce?.applyToAllUDPPacket ?? false}
									onchange={(e) => set('nonce', { applyToAllUDPPacket: e.currentTarget.checked ? true : undefined })}
								/>
								{$t('mieruPattern.applyToAllUdp')}
							</label>
						{/if}
					</div>

					<!-- Padding -->
					<div>
						<span class="block text-xs font-medium text-[var(--ctp-subtext1)] mb-1">{$t('mieruPattern.padding')}</span>
						<div class="grid grid-cols-2 gap-4">
							<div>
								<label for="tp-midpad" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
									{$t('mieruPattern.middlePadding')}
								</label>
								<input
									id="tp-midpad"
									type="number"
									min="0"
									max={MAX_PADDING_LEN}
									disabled={readonly}
									value={p.padding?.maxMiddlePaddingLen ?? ''}
									oninput={(e) => set('padding', { maxMiddlePaddingLen: num(e) })}
									placeholder={$t('mieruPattern.autoPlaceholder')}
									class={inputCls}
								/>
							</div>
							<div>
								<label for="tp-endpad" class="block text-xs text-[var(--ctp-overlay0)] mb-1">
									{$t('mieruPattern.endPadding')}
								</label>
								<input
									id="tp-endpad"
									type="number"
									min="0"
									max={MAX_PADDING_LEN}
									disabled={readonly}
									value={p.padding?.maxEndPaddingLen ?? ''}
									oninput={(e) => set('padding', { maxEndPaddingLen: num(e) })}
									placeholder={$t('mieruPattern.autoPlaceholder')}
									class={inputCls}
								/>
							</div>
						</div>
						<p class={hintCls}>{$t('mieruPattern.paddingHint')}</p>
					</div>

					<!-- Low entropy (mieru 3.35+) -->
					<div>
						<span class="block text-xs font-medium text-[var(--ctp-subtext1)] mb-1">{$t('mieruPattern.lowEntropy')}</span>
						<div class="flex flex-wrap gap-2">
							{#each lowEntropyModes as opt}
								<button
									type="button"
									disabled={readonly}
									class="toggle-btn {p.lowEntropy?.mode === opt.v ? 'selected' : ''}"
									onclick={() => set('lowEntropy', { mode: opt.v })}
								>
									{$t(`mieruPattern.lowEntropyMode.${opt.label}`)}
								</button>
							{/each}
						</div>
						<p class={hintCls}>{$t('mieruPattern.lowEntropyHint')}</p>

						{#if p.lowEntropy?.mode}
							<div class="mt-2">
								<span class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('mieruPattern.maskRotation')}</span>
								<div class="flex flex-wrap items-center gap-2">
									{#each ['none', 'right', 'left'] as dir}
										<button
											type="button"
											disabled={readonly}
											class="toggle-btn {rot.direction === dir ? 'selected' : ''}"
											onclick={() =>
												set('lowEntropy', {
													maskRotation: rotationValue(dir as 'none' | 'right' | 'left', rot.steps)
												})}
										>
											{$t(`mieruPattern.rotation.${dir}`)}
										</button>
									{/each}
									{#if rot.direction !== 'none'}
										<input
											type="number"
											min="1"
											max={MAX_ROTATION_STEPS}
											aria-label={$t('mieruPattern.rotationSteps')}
											disabled={readonly}
											value={rot.steps}
											oninput={(e) => {
												const n = num(e);
												if (n !== undefined) set('lowEntropy', { maskRotation: rotationValue(rot.direction, n) });
											}}
											class="{inputCls} w-24"
										/>
										<span class="text-xs text-[var(--ctp-overlay0)]">{$t('mieruPattern.rotationSteps')}</span>
									{/if}
								</div>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>

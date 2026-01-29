<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		jc: number;
		jmin: number;
		jmax: number;
		s1: number;
		s2: number;
		s3: number;
		s4: number;
		h1: string;
		h2: string;
		h3: string;
		h4: string;
		i1: string;
		i2: string;
		i3: string;
		i4: string;
		i5: string;
	}

	let {
		jc = $bindable(),
		jmin = $bindable(),
		jmax = $bindable(),
		s1 = $bindable(),
		s2 = $bindable(),
		s3 = $bindable(),
		s4 = $bindable(),
		h1 = $bindable(),
		h2 = $bindable(),
		h3 = $bindable(),
		h4 = $bindable(),
		i1 = $bindable(),
		i2 = $bindable(),
		i3 = $bindable(),
		i4 = $bindable(),
		i5 = $bindable()
	}: Props = $props();
</script>

<div class="space-y-4">
	<!-- Junk Packets -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.junkPackets')}</h3>
		<div class="grid grid-cols-3 gap-4">
			<div>
				<label for="jc" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jc')}</label>
				<input
					id="jc"
					type="number"
					bind:value={jc}
					min="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
			<div>
				<label for="jmin" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jmin')}</label>
				<input
					id="jmin"
					type="number"
					bind:value={jmin}
					min="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
			<div>
				<label for="jmax" class="block text-xs text-[var(--ctp-overlay0)] mb-1">{$t('endpoints.jmax')}</label>
				<input
					id="jmax"
					type="number"
					bind:value={jmax}
					min="0"
					class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]"
				/>
			</div>
		</div>
	</div>

	<!-- Init Packet Params (S1-S4) -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.initPacketParams')}</h3>
		<div class="grid grid-cols-4 gap-4">
			<div>
				<label for="s1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S1</label>
				<input id="s1" type="number" bind:value={s1} min="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="s2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S2</label>
				<input id="s2" type="number" bind:value={s2} min="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="s3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S3</label>
				<input id="s3" type="number" bind:value={s3} min="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
			<div>
				<label for="s4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">S4</label>
				<input id="s4" type="number" bind:value={s4} min="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
			</div>
		</div>
	</div>

	<!-- Header Params (H1-H4) -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.headerParams')}</h3>
		<div class="grid grid-cols-4 gap-4">
			<div>
				<label for="h1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H1</label>
				<input id="h1" type="text" bind:value={h1} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="h2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H2</label>
				<input id="h2" type="text" bind:value={h2} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="h3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H3</label>
				<input id="h3" type="text" bind:value={h3} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="h4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">H4</label>
				<input id="h4" type="text" bind:value={h4} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
		</div>
	</div>

	<!-- Init Params (I1-I5) -->
	<div class="bg-[var(--ctp-surface0)] rounded-lg p-4">
		<h3 class="text-sm font-medium text-[var(--ctp-subtext1)] mb-3">{$t('endpoints.obfuscation.initParams')}</h3>
		<div class="grid grid-cols-5 gap-3">
			<div>
				<label for="i1" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I1</label>
				<input id="i1" type="text" bind:value={i1} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="i2" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I2</label>
				<input id="i2" type="text" bind:value={i2} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="i3" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I3</label>
				<input id="i3" type="text" bind:value={i3} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="i4" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I4</label>
				<input id="i4" type="text" bind:value={i4} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
			<div>
				<label for="i5" class="block text-xs text-[var(--ctp-overlay0)] mb-1">I5</label>
				<input id="i5" type="text" bind:value={i5} placeholder="0" class="w-full px-3 py-2 bg-[var(--ctp-mantle)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-mono text-sm" />
			</div>
		</div>
	</div>

	<p class="text-sm text-[var(--ctp-overlay0)]">
		{$t('endpoints.obfuscation.hint')}
	</p>
</div>

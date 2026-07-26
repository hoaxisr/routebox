<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		onImport: (text: string) => void;
		onClose: () => void;
	}

	let { onImport, onClose }: Props = $props();

	let importText = $state('');
	let fileInput: HTMLInputElement;

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		const reader = new FileReader();
		reader.onload = (event) => {
			importText = event.target?.result as string || '';
		};
		reader.readAsText(file);
	}

	function handleImport() {
		if (importText.trim()) {
			onImport(importText);
		}
	}
</script>

<div
	class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
	onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
	role="dialog"
	aria-modal="true"
>
	<div class="bg-[var(--ctp-base)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
		<div class="flex items-center justify-between px-6 py-4 border-b border-[var(--ctp-surface2)]">
			<h2 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('endpoints.importDialogTitle')}</h2>
			<button
				onclick={onClose}
				class="p-1 hover:bg-[var(--ctp-surface1)] rounded-lg transition-colors"
				aria-label={$t('common.close')}
			>
				<svg class="w-5 h-5 text-[var(--ctp-overlay1)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>

		<div class="flex-1 overflow-y-auto p-6 space-y-4">
			<p class="text-sm text-[var(--ctp-overlay1)]">
				{$t('endpoints.importDialogHint')}
			</p>

			<!-- File upload -->
			<input
				type="file"
				accept=".conf,.txt"
				bind:this={fileInput}
				onchange={handleFileSelect}
				class="hidden"
			/>
			<button
				type="button"
				onclick={() => fileInput.click()}
				class="import-btn"
			>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 13h6m-3-3v6m5 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
				</svg>
				{$t('endpoints.selectConfFile')}
			</button>

			<div class="relative flex items-center">
				<div class="flex-1 border-t border-[var(--ctp-surface2)]"></div>
				<span class="px-3 text-sm text-[var(--ctp-overlay0)]">{$t('endpoints.orPasteConfig')}</span>
				<div class="flex-1 border-t border-[var(--ctp-surface2)]"></div>
			</div>

			<textarea
				bind:value={importText}
				placeholder="[Interface]
PrivateKey = ...
Address = 10.0.0.2/32
MTU = 1280
Jc = 4
Jmin = 50
Jmax = 1000
...

[Peer]
PublicKey = ...
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0"
				class="w-full h-48 px-3 py-2 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] font-code text-sm resize-none"
			></textarea>
		</div>

		<div class="px-6 py-4 border-t border-[var(--ctp-surface2)] flex justify-end gap-3">
			<button
				onclick={onClose}
				class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors"
			>
				{$t('common.cancel')}
			</button>
			<button
				onclick={handleImport}
				disabled={!importText.trim()}
				class="px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
			>
				{$t('common.import')}
			</button>
		</div>
	</div>
</div>

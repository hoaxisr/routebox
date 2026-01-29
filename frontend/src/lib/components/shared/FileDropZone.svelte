<script lang="ts">
	import { t } from 'svelte-i18n';

	interface Props {
		value: string;
		placeholder?: string;
		rows?: number;
		accept?: string;
		error?: string;
		onInput?: (value: string) => void;
		onFileRead?: (content: string, filename: string) => void;
		onError?: (error: string) => void;
	}

	let {
		value = $bindable(),
		placeholder = '',
		rows = 6,
		accept = '.conf,.txt,.json',
		error = '',
		onInput,
		onFileRead,
		onError
	}: Props = $props();

	let fileInput: HTMLInputElement;
	let isDragging = $state(false);

	function handleInput(e: Event) {
		const target = e.target as HTMLTextAreaElement;
		value = target.value;
		onInput?.(value);
	}

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files && input.files.length > 0) {
			readFile(input.files[0]);
		}
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		isDragging = false;
		if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
			readFile(event.dataTransfer.files[0]);
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		isDragging = true;
	}

	function handleDragLeave() {
		isDragging = false;
	}

	function readFile(file: File) {
		const validExtensions = accept.split(',').map(ext => ext.trim());
		const hasValidExt = validExtensions.some(ext => file.name.toLowerCase().endsWith(ext));

		if (!hasValidExt && file.size > 50000) {
			onError?.($t('common.invalidFileType'));
			return;
		}

		const reader = new FileReader();
		reader.onload = (e) => {
			const content = e.target?.result as string;
			if (content) {
				value = content;
				onFileRead?.(content, file.name);
			}
		};
		reader.onerror = () => {
			onError?.($t('common.fileReadError'));
		};
		reader.readAsText(file);
	}
</script>

<div
	class="relative border-2 border-dashed rounded-lg p-4 transition-colors {isDragging ? 'border-[var(--ctp-primary)] bg-[var(--ctp-surface0)]' : 'border-[var(--ctp-surface2)]'}"
	ondrop={handleDrop}
	ondragover={handleDragOver}
	ondragleave={handleDragLeave}
	role="button"
	tabindex="0"
>
	<textarea
		{value}
		oninput={handleInput}
		{placeholder}
		{rows}
		class="w-full px-4 py-3 bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] placeholder-[var(--ctp-overlay0)] font-mono text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)] resize-none"
	></textarea>

	<div class="mt-3 flex items-center justify-center gap-3">
		<span class="text-sm text-[var(--ctp-overlay1)]">{$t('common.or')}</span>
		<button
			type="button"
			onclick={() => fileInput.click()}
			class="px-4 py-2 bg-[var(--ctp-surface1)] text-[var(--ctp-text)] rounded-lg hover:bg-[var(--ctp-surface2)] transition-colors text-sm flex items-center gap-2"
		>
			<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
			</svg>
			{$t('common.selectFile')}
		</button>
		<input
			bind:this={fileInput}
			type="file"
			{accept}
			onchange={handleFileSelect}
			class="hidden"
		/>
	</div>
</div>

{#if error}
	<div class="mt-2 alert-box error">
		<p class="text-sm text-[var(--ctp-red)]">{error}</p>
	</div>
{/if}

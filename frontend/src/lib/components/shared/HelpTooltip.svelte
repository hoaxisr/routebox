<script lang="ts">
	interface Props {
		text: string;
	}

	let { text }: Props = $props();
	let show = $state(false);
</script>

<span class="help-tooltip">
	<button
		type="button"
		class="help-tooltip-btn"
		onmouseenter={() => (show = true)}
		onmouseleave={() => (show = false)}
		onfocus={() => (show = true)}
		onblur={() => (show = false)}
	>
		?
	</button>

	{#if show}
		<div class="help-tooltip-content">
			{text}
			<div class="help-tooltip-arrow"></div>
		</div>
	{/if}
</span>

<style>
	.help-tooltip {
		position: relative;
		display: inline-flex;
		align-items: center;
	}

	.help-tooltip-btn {
		width: 1rem;
		height: 1rem;
		border-radius: 9999px;
		background-color: var(--ctp-surface1);
		color: var(--ctp-overlay0);
		font-size: 0.625rem;
		font-weight: 600;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: help;
		transition: all 0.15s ease;
		border: none;
		padding: 0;
	}

	.help-tooltip-btn:hover,
	.help-tooltip-btn:focus {
		background-color: var(--ctp-surface2);
		color: var(--ctp-text);
		outline: none;
	}

	.help-tooltip-content {
		position: absolute;
		z-index: 50;
		bottom: 100%;
		left: 50%;
		transform: translateX(-50%);
		margin-bottom: 0.5rem;
		padding: 0.5rem 0.75rem;
		background-color: var(--ctp-surface0);
		border: 1px solid var(--ctp-surface2);
		border-radius: 0.5rem;
		box-shadow:
			0 4px 6px -1px rgb(0 0 0 / 0.1),
			0 2px 4px -2px rgb(0 0 0 / 0.1);
		font-size: 0.8125rem;
		line-height: 1.4;
		color: var(--ctp-text);
		white-space: normal;
		max-width: 280px;
		min-width: 200px;
		text-align: left;
		font-weight: 400;
	}

	.help-tooltip-arrow {
		position: absolute;
		top: 100%;
		left: 50%;
		transform: translateX(-50%);
		width: 0;
		height: 0;
		border-left: 6px solid transparent;
		border-right: 6px solid transparent;
		border-top: 6px solid var(--ctp-surface2);
	}

	.help-tooltip-arrow::after {
		content: '';
		position: absolute;
		top: -7px;
		left: -5px;
		width: 0;
		height: 0;
		border-left: 5px solid transparent;
		border-right: 5px solid transparent;
		border-top: 5px solid var(--ctp-surface0);
	}
</style>

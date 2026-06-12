<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api/client';
	import { notifications } from '$lib/stores';

	let username = $state('');
	let password = $state('');
	let busy = $state(false);

	async function submit() {
		busy = true;
		try {
			await api.login(username, password);
			window.location.href = '/';
		} catch {
			notifications.error($t('auth.invalidCredentials'));
		} finally {
			busy = false;
		}
	}
</script>

<div class="min-h-screen flex items-center justify-center bg-[var(--ctp-base)] p-4">
	<form onsubmit={(e) => { e.preventDefault(); submit(); }}
		class="w-full max-w-sm bg-[var(--ctp-surface0)] border border-[var(--ctp-surface2)] rounded-xl p-6 space-y-4">
		<h1 class="text-lg font-semibold text-[var(--ctp-text)]">{$t('auth.signIn')}</h1>
		<div>
			<label for="u" class="block text-sm text-[var(--ctp-subtext1)] mb-1">{$t('auth.username')}</label>
			<input id="u" type="text" bind:value={username} autocomplete="username" required
				class="w-full px-3 py-2 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<div>
			<label for="p" class="block text-sm text-[var(--ctp-subtext1)] mb-1">{$t('auth.password')}</label>
			<input id="p" type="password" bind:value={password} autocomplete="current-password" required
				class="w-full px-3 py-2 bg-[var(--ctp-surface1)] border border-[var(--ctp-surface2)] rounded-lg text-[var(--ctp-text)] focus:outline-none focus:ring-2 focus:ring-[var(--ctp-primary)]" />
		</div>
		<button type="submit" disabled={busy}
			class="w-full px-4 py-2 bg-[var(--ctp-primary)] text-white rounded-lg hover:opacity-90 disabled:opacity-50">
			{busy ? $t('common.loading') : $t('auth.signIn')}
		</button>
	</form>
</div>

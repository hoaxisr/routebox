import { browser } from '$app/environment';
import { init, register, locale } from 'svelte-i18n';

// Register locales
register('en', () => import('./locales/en.json'));

// Initialize i18n
export function initI18n(initialLocale?: string) {
	init({
		fallbackLocale: 'en',
		initialLocale: initialLocale || 'en'
	});
}

// Set locale and persist to localStorage
export function setLocale(newLocale: string) {
	if (newLocale === 'en') {
		locale.set(newLocale);
		if (browser) {
			localStorage.setItem('routebox-language', newLocale);
		}
	}
}

// Export locale store for reactive access
export { locale } from 'svelte-i18n';

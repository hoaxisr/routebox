import { browser } from '$app/environment';
import { init, register, locale } from 'svelte-i18n';

// Register locales
register('en', () => import('./locales/en.json'));
register('ru', () => import('./locales/ru.json'));

// Initialize i18n
export function initI18n(initialLocale?: string) {
	init({
		fallbackLocale: 'en',
		initialLocale: initialLocale || 'en'
	});
}

// Set locale and persist to localStorage
export function setLocale(newLocale: string) {
	const allowed = ['en', 'ru'];
	const l = allowed.includes(newLocale) ? newLocale : 'en';
	locale.set(l);
	if (browser) {
		localStorage.setItem('routebox-language', l);
	}
}

// Export locale store for reactive access
export { locale } from 'svelte-i18n';

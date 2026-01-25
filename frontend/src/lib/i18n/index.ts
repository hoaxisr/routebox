import { browser } from '$app/environment';
import { init, register, getLocaleFromNavigator, locale } from 'svelte-i18n';

// Register locales
register('en', () => import('./locales/en.json'));
register('ru', () => import('./locales/ru.json'));

// Initialize i18n
export function initI18n(initialLocale?: string) {
	init({
		fallbackLocale: 'en',
		initialLocale: initialLocale || getInitialLocale()
	});
}

// Get initial locale from localStorage or browser
function getInitialLocale(): string {
	if (browser) {
		// Check localStorage first (user preference)
		const stored = localStorage.getItem('routebox-language');
		if (stored && ['en', 'ru'].includes(stored)) {
			return stored;
		}

		// Fall back to browser language
		const browserLocale = getLocaleFromNavigator();
		if (browserLocale?.startsWith('ru')) {
			return 'ru';
		}
	}
	return 'en';
}

// Set locale and persist to localStorage
export function setLocale(newLocale: string) {
	if (['en', 'ru'].includes(newLocale)) {
		locale.set(newLocale);
		if (browser) {
			localStorage.setItem('routebox-language', newLocale);
		}
	}
}

// Export locale store for reactive access
export { locale } from 'svelte-i18n';

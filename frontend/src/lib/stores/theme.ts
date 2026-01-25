import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'light' | 'dark';

function createThemeStore() {
	const storedTheme = browser ? (localStorage.getItem('theme') as Theme) : null;
	const prefersDark = browser ? window.matchMedia('(prefers-color-scheme: dark)').matches : true;
	const initialTheme = storedTheme || (prefersDark ? 'dark' : 'light');

	const { subscribe, set, update } = writable<Theme>(initialTheme);

	if (browser) {
		document.documentElement.classList.toggle('light', initialTheme === 'light');
		document.documentElement.classList.toggle('dark', initialTheme === 'dark');
	}

	return {
		subscribe,
		toggle() {
			update((current) => {
				const newTheme = current === 'dark' ? 'light' : 'dark';
				if (browser) {
					localStorage.setItem('theme', newTheme);
					document.documentElement.classList.toggle('light', newTheme === 'light');
					document.documentElement.classList.toggle('dark', newTheme === 'dark');
				}
				return newTheme;
			});
		},
		set(theme: Theme) {
			if (browser) {
				localStorage.setItem('theme', theme);
				document.documentElement.classList.toggle('light', theme === 'light');
				document.documentElement.classList.toggle('dark', theme === 'dark');
			}
			set(theme);
		}
	};
}

export const theme = createThemeStore();

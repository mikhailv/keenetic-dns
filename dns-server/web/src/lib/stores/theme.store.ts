import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Theme = 'light' | 'dark' | 'system';

type BSTheme = 'light' | 'dark';

const STORAGE_KEY = 'theme';

function getStoredTheme(): Theme {
	if (browser) {
		return (localStorage.getItem(STORAGE_KEY) as Theme) ?? 'system';
	}
	return 'system';
}

function resolveBootstrapTheme(theme: Theme): BSTheme {
	if (theme === 'system') {
		if (browser) {
			return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
		}
		return 'dark';
	}
	return theme;
}

function applyTheme(theme: Theme) {
	if (browser) {
		document.documentElement.setAttribute('data-bs-theme', resolveBootstrapTheme(theme));
		localStorage.setItem(STORAGE_KEY, theme);
	}
}

export const theme = writable<Theme>(getStoredTheme());

if (browser) {
	// Apply on store change
	theme.subscribe(applyTheme);

	// React to OS preference changes when in system mode
	window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
		theme.update((t) => {
			if (t === 'system') {
				applyTheme(t);
			}
			return t;
		});
	});
}

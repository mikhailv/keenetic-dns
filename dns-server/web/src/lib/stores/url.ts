import { browser } from '$app/environment';

export const baseURL = resolveBaseURL();

function resolveBaseURL(): URL {
	if (browser) {
		if (localStorage['API_BASE_URL']) {
			return new URL(localStorage['API_BASE_URL']);
		}
		return new URL('/', currentURL());
	}
	return currentURL();
}

export function queryParams(): URLSearchParams {
	return new URLSearchParams(currentURL().search);
}

export function currentURL(): URL {
	return browser ? new URL(location.href) : new URL('http://localhost');
}

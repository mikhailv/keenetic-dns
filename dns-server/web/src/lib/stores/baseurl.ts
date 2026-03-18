import { browser } from '$app/environment';

export const baseURL = new URL(resolveBaseURL());

function resolveBaseURL(): string | URL {
	if (!browser) {
		return 'http://localhost';
	}
	return localStorage['API_BASE_URL'] || new URL('/', new URL(location.href));
}

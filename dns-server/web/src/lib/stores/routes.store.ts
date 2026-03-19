import { derived, type Readable, writable } from 'svelte/store';
import type { IPRoute } from '$lib/types';
import { api } from '$lib/services';
import { storeBuilder } from '$lib/stores/util';

export interface RouteStore extends Readable<Readonly<RouteStoreState>> {
	reload(): Promise<void>;
	filter: string;
}

export interface RouteStoreState {
	items: IPRoute[];
	total: number;
	error?: string;
}

export function createRouteStore(): RouteStore {
	const routes = writable<RouteStoreState>({ items: [], total: 0 });
	const filter = writable('');

	const filtered = derived([routes, filter], ([$routes, $filter]) => {
		return {
			items: filterRoutes($routes.items, $filter),
			total: $routes.items.length,
			error: $routes.error
		};
	});

	async function reload(): Promise<void> {
		const { items, error } = await api.getRoutes();
		routes.update(() => ({ items, total: items.length, error }));
	}

	function filterRoutes(routes: IPRoute[], filter: string): IPRoute[] {
		filter = filter.trim().toLowerCase();
		if (filter === '') {
			return routes;
		}
		return routes.filter(
			(route) =>
				route.addr.toLowerCase().includes(filter) ||
				route.iface.toLowerCase().includes(filter) ||
				route.dns_records?.some(
					(rec) =>
						rec.domain.toLowerCase().includes(filter) || rec.ip.toLowerCase().includes(filter)
				)
		);
	}

	return storeBuilder({
		subscribe: filtered.subscribe,
		reload
	}).property('filter', filter).store;
}

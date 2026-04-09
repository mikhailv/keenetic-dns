import { type Readable, writable } from 'svelte/store';
import type { HostInfo } from '$lib/types';
import { api } from '$lib/services';

const DEFAULT_RELOAD_INTERVAL = 30_000;

export interface HostStore extends Readable<Readonly<HostStoreState>> {
	autoreload(interval?: number): () => void;
	reload(): Promise<void>;
}

export interface HostStoreState {
	items: HostInfo[];
	byIP: Record<string, HostInfo>;
	total: number;
	error?: string;
}

export function createHostStore(): HostStore {
	const hosts = writable<HostStoreState>({ items: [], byIP: {}, total: 0 });

	function autoreload(interval: number = DEFAULT_RELOAD_INTERVAL) {
		reload().then();
		const intervalId = setInterval(reload, interval);
		return () => clearInterval(intervalId);
	}

	async function reload(): Promise<void> {
		const { items, error } = await api.getHosts();
		hosts.update(() => ({
			items,
			byIP: items.reduce<Record<string, HostInfo>>((r, v) => ((r[v.ip!] = v), r), {}),
			total: items.length,
			error
		}));
	}

	return {
		subscribe: hosts.subscribe,
		autoreload,
		reload
	};
}

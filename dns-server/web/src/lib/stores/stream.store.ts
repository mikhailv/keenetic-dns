import { type Writable, writable } from 'svelte/store';

export interface StreamStore<T> extends Writable<StreamStoreState<T>> {
	start(): void;
	stop(): void;
}

interface StreamStoreState<T> {
	items: T[];
	error: string | null;
}

export function createWebSocketStreamStore<T extends { cursor: string }>(
	url: URL,
	limit: number,
	parser: (data: typeof MessageEvent.prototype.data) => T[]
): StreamStore<T> {
	const { subscribe, set, update } = writable<StreamStoreState<T>>({
		items: [],
		error: null
	});

	const RECONNECT_TIMEOUT = 2000;

	let stopped = true;
	let ws: WebSocket | null = null;
	let cursor: string = '';

	function connect() {
		stopped = false;

		url.searchParams.set('preload_count', String(limit));
		url.searchParams.set('cursor', cursor);

		ws = new WebSocket(url);
		ws.onopen = () => {
			update((v) => {
				v.error = null;
				return v;
			});
		};
		ws.onerror = () => {
			update((v) => {
				v.error = 'WebSocket connection failed';
				return v;
			});
		};
		ws.onclose = () => {
			if (!stopped) {
				setTimeout(connect, RECONNECT_TIMEOUT);
			}
		};
		ws.onmessage = (ev) => {
			const data: T[] = parser(ev.data);
			if (data.length) {
				cursor = data[data.length - 1].cursor;
			}
			update((v) => {
				// Add new entries to the beginning, maintaining max items limit
				v.items = [...data.reverse(), ...v.items].slice(0, limit);
				return v;
			});
		};
	}

	function disconnect() {
		stopped = true;
		cursor = '';
		ws?.close();
		ws = null;
	}

	return {
		subscribe,
		set,
		update,
		start: () => {
			disconnect();
			connect();
		},
		stop: () => {
			disconnect();
		}
	};
}

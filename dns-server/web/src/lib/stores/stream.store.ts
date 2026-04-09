import { type Readable, writable } from 'svelte/store';
import { mutator } from '$lib/stores/util';

export interface StreamStore<T> extends Readable<Readonly<StreamStoreState<T>>> {
	start(): () => void;
	stop(): void;
}

interface StreamStoreState<T> {
	items: T[];
	error?: string;
}

export function createWebSocketStreamStore<T extends { cursor: string }>(
	url: URL,
	limit: number,
	parser: (data: typeof MessageEvent.prototype.data) => T[]
): StreamStore<T> {
	const { subscribe, update } = writable<StreamStoreState<T>>({
		items: []
	});
	const mutate = mutator(update);

	const RECONNECT_TIMEOUT = 2000;

	let connected = false;
	let ws: WebSocket | null = null;
	let cursor: string = '';

	function connect() {
		connected = true;

		url.searchParams.set('preload_count', String(limit));
		url.searchParams.set('cursor', cursor);

		ws = new WebSocket(url);
		ws.onopen = () => {
			mutate((v) => (v.error = undefined));
		};
		ws.onerror = () => {
			mutate((v) => (v.error = 'WebSocket connection failed'));
		};
		ws.onclose = () => {
			if (connected) {
				setTimeout(connect, RECONNECT_TIMEOUT);
			}
		};
		ws.onmessage = (ev) => {
			try {
				const data: T[] = parser(ev.data);
				if (data.length) {
					cursor = data[data.length - 1].cursor;
				}
				mutate((v) => {
					// Add new entries to the beginning, maintaining max items limit
					v.items = [...data.reverse(), ...v.items].slice(0, limit);
				});
			} catch (e) {
				mutate((v) => (v.error = `WebSocket data parse error: ${e}`));
			}
		};
	}

	function disconnect() {
		connected = false;
		cursor = '';
		ws?.close();
		ws = null;
	}

	function start() {
		disconnect();
		connect();
		return stop;
	}

	function stop() {
		disconnect();
	}

	return {
		subscribe,
		start,
		stop
	};
}

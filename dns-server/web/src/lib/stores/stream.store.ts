import { type Readable, writable } from 'svelte/store';
import { mutator } from '$lib/stores/util';

export interface StreamStore<T> extends Readable<Readonly<StreamStoreState<T>>> {
	start(): () => void;
	stop(): void;
	setParam(name: string, value: string): void;
}

interface StreamStoreState<T> {
	items: T[];
	error?: string;
}

export function createWebSocketStreamStore<T extends { cursor?: string }>(
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

		const socket = new WebSocket(url);
		ws = socket;
		socket.onopen = () => {
			if (ws === socket) {
				mutate((v) => (v.error = undefined));
			}
		};
		socket.onerror = () => {
			if (ws === socket) {
				mutate((v) => (v.error = 'WebSocket connection failed'));
			}
		};
		socket.onclose = () => {
			if (ws === socket && connected) {
				setTimeout(() => {
					if (ws === socket) {
						connect();
					}
				}, RECONNECT_TIMEOUT);
			}
		};
		socket.onmessage = (ev) => {
			if (ws !== socket) {
				return;
			}
			try {
				const data: T[] = parser(ev.data);
				if (data.length) {
					cursor = data[data.length - 1].cursor ?? '';
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

	function setParam(name: string, value: string) {
		if (url.searchParams.get(name) === value || (!value && !url.searchParams.has(name))) {
			return;
		}
		if (value) {
			url.searchParams.set(name, value);
		} else {
			url.searchParams.delete(name);
		}
		const reconnect = connected;
		if (reconnect) {
			disconnect();
		}
		mutate((v) => (v.items = []));
		if (reconnect) {
			connect();
		}
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
		stop,
		setParam
	};
}

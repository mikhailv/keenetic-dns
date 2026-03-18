import type { DNSQuery, IPRoute, LogEntry } from '$lib/types';
import { createWebSocketStreamStore, type StreamStore } from '$lib/stores';
import { baseURL } from '$lib/stores';

interface ListResponse<T> {
	items: T[];
	first_cursor: string;
	last_cursor: string;
	has_more: boolean;
	prev_page_url: string;
	next_page_url: string;
}

type StreamResponse<T> = T[];

export class APIService {
	private readonly baseUrl: string;

	constructor(baseUrl: string = '') {
		this.baseUrl = baseUrl.replace(/\/$/, '');
	}

	async getRoutes(): Promise<IPRoute[]> {
		const res = await fetch(`${this.baseUrl}/api/routes`);
		if (!res.ok) {
			throw new Error(`Failed to fetch routes: ${res.status}`);
		}
		const data: IPRoute[] = await res.json();
		data.forEach((route) => {
			route.dns_records?.forEach((r) => (r.expires = new Date(r.expires)));
		});
		return data;
	}

	async getDNSQueries(backward: boolean, count: number): Promise<DNSQuery[]> {
		const res = await fetch(
			`${this.baseUrl}/api/dns-queries?backward=${backward ? 1 : 0}&count=${count}`
		);
		if (!res.ok) {
			throw new Error(`Failed to fetch DNS queries: ${res.status}`);
		}
		const data: ListResponse<DNSQuery> = await res.json();
		data.items.forEach((it) => (it.time = new Date(it.time)));
		return data.items;
	}

	createDNSQueryStreamStore(limit: number): StreamStore<DNSQuery> {
		return createWebSocketStreamStore(
			new URL(`${this.baseUrl}/api/dns-queries/ws`),
			limit,
			(data) => {
				const res: StreamResponse<DNSQuery> = JSON.parse(data);
				res.forEach((it) => (it.time = new Date(it.time)));
				return res;
			}
		);
	}

	createLogStreamStore(limit: number): StreamStore<LogEntry> {
		return createWebSocketStreamStore(new URL(`${this.baseUrl}/api/logs/ws`), limit, (data) => {
			const res: StreamResponse<LogEntry> = JSON.parse(data);
			res.forEach((it) => (it.time = new Date(it.time)));
			return res;
		});
	}
}

export const api = new APIService(baseURL.href);

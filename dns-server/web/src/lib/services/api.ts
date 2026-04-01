import type { DNSQuery, HostInfo, IPRoute, LogEntry } from '$lib/types';
import { baseURL, createWebSocketStreamStore, type StreamStore } from '$lib/stores';

interface ListResponse<T> {
	items: T[];
	first_cursor: string;
	last_cursor: string;
	has_more: boolean;
	prev_page_url: string;
	next_page_url: string;
}

export type ListResult<T> = {
	items: T[];
	error?: string;
};

export type DataResult<T> = {
	data?: T;
	error?: string;
};

type StreamResponse<T> = T[];

export class APIService {
	private readonly baseUrl: string;

	constructor(baseUrl: string = '') {
		this.baseUrl = baseUrl.replace(/\/$/, '');
	}

	async getRoutes(): Promise<ListResult<IPRoute>> {
		const { data, error } = await requestData<IPRoute[]>(`${this.baseUrl}/api/routes`);
		return {
			items: data?.map(parseIPRoute) ?? [],
			error
		};
	}

	async getHosts(): Promise<ListResult<HostInfo>> {
		const { data, error } = await requestData<HostInfo[]>(`${this.baseUrl}/api/hosts`);
		return {
			items: data ?? [],
			error
		};
	}

	async getDNSQueries(backward: boolean, count: number): Promise<ListResult<DNSQuery>> {
		const { data, error } = await requestData<ListResponse<DNSQuery>>(
			`${this.baseUrl}/api/dns-queries?backward=${backward ? 1 : 0}&count=${count}`
		);
		return {
			items: data?.items?.map(parseDNSQuery) ?? [],
			error
		};
	}

	createDNSQueryStreamStore(limit: number): StreamStore<DNSQuery> {
		return createWebSocketStreamStore(
			new URL(`${this.baseUrl}/api/dns-queries/ws`),
			limit,
			(data) => (JSON.parse(data) as StreamResponse<DNSQuery>).map(parseDNSQuery)
		);
	}

	createLogStreamStore(limit: number): StreamStore<LogEntry> {
		return createWebSocketStreamStore(new URL(`${this.baseUrl}/api/logs/ws`), limit, (data) =>
			(JSON.parse(data) as StreamResponse<LogEntry>).map(parseLogEntry)
		);
	}
}

export const api = new APIService(baseURL.href);

function parseIPRoute(it: IPRoute): IPRoute {
	it.added_at = new Date(it.added_at);
	it.dns_records?.forEach((it) => {
		it.resolved = new Date(it.resolved);
		it.expires = new Date(it.expires);
	});
	return it;
}

function parseDNSQuery(it: DNSQuery): DNSQuery {
	it.time = new Date(it.time);
	it.client_ip = it.client_addr.split(':')[0];
	return it;
}

function parseLogEntry(it: LogEntry): LogEntry {
	it.time = new Date(it.time);
	return it;
}

async function requestData<T>(url: URL | string, opts?: RequestInit): Promise<DataResult<T>> {
	try {
		const resp = await fetch(url, opts);
		if (!resp.ok) {
			return { error: `unexpected status code: ${resp.status}` };
		}
		return { data: (await resp.json()) as T };
	} catch (e) {
		return { error: `Failed to load data: ${e}` };
	}
}

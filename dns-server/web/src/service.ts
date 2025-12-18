import { DNSQuery, IPRoute } from './types';
import { Stream, websocketStream } from './stream';

interface ListResponse<T> {
  items: T[];
  first_cursor: string;
  last_cursor: string;
  has_more: boolean;
  prev_page_url: string;
  next_page_url: string;
}

type StreamResponse<T> = T[];

export class Service {
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async routes(): Promise<IPRoute[]> {
    const res: IPRoute[] = await (await fetch(this.baseUrl + '/api/routes')).json();
    res.forEach(it => it.dns_records?.forEach(r => r.expires = new Date(r.expires)));
    return res;
  }

  async dnsQueries(backward: boolean, count: number): Promise<DNSQuery[]> {
    const res: ListResponse<DNSQuery> = await (await fetch(this.baseUrl + `/api/dns-queries?backward=${backward ? 1 : 0}&count=${count}`)).json();
    res.items.forEach(it => it.time = new Date(it.time));
    return res.items;
  }

  streamDNSQueries(preloadCount: number = 1, cursor: string = ''): Stream<DNSQuery[]> {
    return websocketStream<DNSQuery[]>(
      () => new WebSocket(this.baseUrl + `/api/dns-queries/ws?preload_count=${preloadCount}&cursor=${cursor}`),
      data => {
        const res: StreamResponse<DNSQuery> = JSON.parse(data);
        res.forEach(it => it.time = new Date(it.time));
        return res;
      },
    );
  }
}

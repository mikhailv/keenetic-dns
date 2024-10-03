import { DNSQuery, IPRoute } from './types';
import { Stream, websocketStream } from './stream';

export class Service {
  private readonly baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async routes(): Promise<IPRoute[]> {
    const res: IPRoute[] = await (await fetch(this.baseUrl + '/api/routes')).json();
    res.forEach(it => it.dnsRecords?.forEach(r => r.expires = new Date(r.expires)));
    return res;
  }

  streamDomainResolve(): Stream<DNSQuery[]> {
    return websocketStream<DNSQuery[]>(
      () => new WebSocket(this.baseUrl + '/api/dns-queries/ws'),
      data => {
        const res = JSON.parse(data) as DNSQuery[];
        res.forEach(it => it.time = new Date(it.time));
        return res;
      },
    );
  }
}

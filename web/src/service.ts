import { DomainResolve, IPRoute } from './types';
import { Stream, websocketStream } from './stream';

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

  streamDomainResolve(): Stream<DomainResolve[]> {
    return websocketStream<DomainResolve[]>(
      () => new WebSocket(this.baseUrl + '/api/dns-resolve/ws'),
      data => {
        const res = JSON.parse(data) as DomainResolve[];
        res.forEach(it => it.time = new Date(it.time));
        return res;
      },
    );
  }
}

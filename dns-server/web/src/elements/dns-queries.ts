import { html, LitElement } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { consume } from '@lit/context';
import { serviceContext } from '../context';
import { Service } from '../service';
import { DNSQuery } from '../types';
import { listenStream, Stream } from '../stream';

const maxItems = 200;

@customElement('x-dns-queries')
export class DNSRequestsElement extends LitElement {
  @consume({ context: serviceContext })
  @state()
  private _service?: Service;

  @state()
  private _stream?: Stream<DNSQuery[]>;

  @state()
  private _items: DNSQuery[] = [];

  override createRenderRoot() {
    return this;
  }

  override async connectedCallback() {
    super.connectedCallback();
    this._items = [];
    this._stream = this._service!.streamDNSQueries(maxItems);
    listenStream(this._stream!, async res => {
      if (res !== 'cancelled' && res.value) {
        this._items.unshift(...[...res.value].reverse());
        this._items = this._items.slice(0, maxItems);
      }
    });
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    this._stream?.cancel();
  }

  override render() {
    return html`
      <h1>DNS Queries</h1>
      ${this._renderTable()}
    `;
  }

  private _renderTable() {
    return html`
      <table class="table table-sm table-hover table-sticky-header">
        <thead>
        <tr>
          <th scope="col">Time</th>
          <th scope="col">Client</th>
          <th scope="col">Domain</th>
          <th scope="col">TTL</th>
          <th scope="col">IP</th>
          <th scope="col">Routed</th>
        </tr>
        </thead>
        <tbody class="table-group-divider">
        ${repeat(this._items, it => it.cursor, it => html`
          <tr class="animate-new-row">
            <td title=${it.time.toLocaleString()}>${formatTime(it.time)}</td>
            <td>${it.client_addr.split(':')[0]}</td>
            <td>${it.domain}</td>
            <td>${it.ttl}</td>
            <td class="fw-light" style="font-size: 0.9rem">
              ${it.ips.map(v => html`<div>${v.ip}</div>`)}
            </td>
            <td class="fw-light" style="font-size: 0.9rem">
              ${it.ips.map(v => html`<div>${v.route_iface ? `${v.route_iface} (${v.route_reason})` : '-'}</div>`)}
            </td>
          </tr>
        `)}
        </tbody>
      </table>
    `;
  }
}

function formatTime(d: Date): string {
  return d.toTimeString().split(' ')[0];
}

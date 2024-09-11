import { html, LitElement } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { consume } from '@lit/context';
import { serviceContext } from '../context';
import { Service } from '../service';
import { IPRoute } from '../types';
import { Stream, tickerStream } from '../stream';
import { stream } from '../stream-directive';

@customElement('x-routes')
export class RoutesElement extends LitElement {
  @consume({ context: serviceContext })
  @state()
  private _service?: Service;

  @state()
  private _stream?: Stream<IPRoute[]>;

  @state()
  private _filter = '';

  private _updateInterval?: ReturnType<typeof setInterval>;

  override createRenderRoot() {
    return this;
  }

  override connectedCallback() {
    super.connectedCallback();
    this._refresh();
    this._updateInterval = setInterval(() => this.requestUpdate(), 1000);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    this._stream?.cancel();
    clearInterval(this._updateInterval);
  }

  override render() {
    return html`
      <h1>Routes</h1>
      <div class="hstack gap-3">
        <button type="button" class="btn btn-outline-primary" @click="${this._refresh}">Refresh</button>
        <input class="form-control me-auto" type="text" placeholder="Filter..." aria-label="Filter..."
               .value=${this._filter} @keyup=${e => this._updateFilter(e)} @change=${e => this._updateFilter(e)}>
      </div>
      ${stream(this._stream, {
        initial: () => html`<p class="text-body pt-1">Loading data...</p>`,
        render: routes => this._renderTable(filterRoutes(routes, this._filter)),
        error: error => html`<p class="text-danger">Something went wrong: ${error}</p>`,
      }, [this._filter])}
    `;
  }

  private _refresh() {
    this._stream?.cancel();
    if (this._service) {
      this._stream = tickerStream(5000, () => this._service!.routes());
    }
  }

  private _renderTable(routes: IPRoute[]) {
    return html`
      <table class="table table-sm table-hover caption-top">
        <caption class="text-end pb-0">Routes: ${routes.length}</caption>
        <thead>
        <tr>
          <th scope="col" style="width: 1%">#</th>
          <th scope="col" style="width: 15%">Address</th>
          <th scope="col" style="width: 15%">Interface</th>
          <th scope="col" class="ps-2">DNS Records</th>
        </tr>
        </thead>
        <tbody class="table-group-divider">
        ${repeat(
            routes,
            route => `${route.addr}\t${route.iface}`,
            (route, i) => html`
              <tr>
                <th scope="row">${i + 1}</th>
                <td>${route.addr}</td>
                <td style="font-size: 0.9rem">${route.iface}</td>
                <td class="ps-2">
                  ${repeat(
                      route.dns_records ?? [],
                      rec => rec.domain,
                      rec => html`
                        <div class="row" style="font-size: 0.9rem">
                          <div class="col fw-light">${rec.domain}</div>
                          <div class="col d-none d-lg-block">${expired(rec.expires)}</div>
                        </div>
                      `,
                  )}
                </td>
              </tr>
            `,
        )}
        </tbody>
      </table>
    `;
  }

  private _updateFilter(e: Event) {
    this._filter = (e.target as HTMLInputElement).value;
  }
}

function expired(time: Date) {
  const seconds = Math.floor((time.valueOf() - Date.now()) / 1000);
  if (seconds >= 0) {
    return `${seconds} sec`;
  }
  return html`<span class="fw-light text-secondary">expired ${-seconds} sec ago</span>`;
}

function filterRoutes(routes: IPRoute[], filter: string): IPRoute[] {
  filter = filter.trim();
  if (filter === '') {
    return routes;
  }
  return routes.filter(route => route.addr.includes(filter)
      || route.iface.includes('filter')
      || route.dns_records?.some(rec => rec.domain.includes(filter) || rec.ip.includes(filter)));
}

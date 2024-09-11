import { html, LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';
import { Router } from '@lit-labs/router';

import { Service } from '../service';
import { provide } from '@lit/context';
import { serviceContext } from '../context';

import './routes';
import './dns-requests';

@customElement('x-app')
export class AppElement extends LitElement {
  private readonly _router = new Router(this, [
    { path: '/', enter: () => this._router.goto('/routes').then(() => false) },
    { path: '/routes', render: () => html`<x-routes></x-routes>` },
    { path: '/dns-requests', render: () => html`<x-dns-requests></x-dns-requests>` },
    { path: '/logs', render: () => html`<h1>Logs</h1>` },
  ]);

  @provide({ context: serviceContext })
  private readonly _service: Service;

  constructor() {
    super();
    this._service = new Service('SERVICE_BASE_URL');
  }

  override createRenderRoot() {
    return this;
  }

  override render() {
    return this._router.outlet();
  }
}

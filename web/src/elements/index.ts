import { AppElement } from './app';
import { RoutesElement } from './routes';
import { DNSRequestsElement } from './dns-requests';

declare global {
  interface HTMLElementTagNameMap {
    'x-app': AppElement;
    'x-routes': RoutesElement;
    'x-dns-requests': DNSRequestsElement;
  }
}

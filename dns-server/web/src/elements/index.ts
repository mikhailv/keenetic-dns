import { AppElement } from './app';
import { RoutesElement } from './routes';
import { DNSRequestsElement } from './dns-queries';

declare global {
  interface HTMLElementTagNameMap {
    'x-app': AppElement;
    'x-routes': RoutesElement;
    'x-dns-queries': DNSRequestsElement;
  }
}

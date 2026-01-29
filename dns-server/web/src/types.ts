export interface IPRoute {
  addr: string;
  iface: string;
  dns_records?: DNSRecord[];
}

export interface DNSRecord {
  ip: string;
  domain: string;
  expires: Date;
}

export interface DNSQuery {
  cursor: string;
  time: Date;
  client_addr: string;
  domain: string;
  ttl: number;
  ips: RoutedIP[];
}

export interface RoutedIP {
  ip: string;
  route_added?: boolean;
  route_iface?: string;
  route_reason?: string;
}

export interface LogEntry {
  cursor: string;
  time: Date;
  level: string;
  msg: string;
  attrs: Record<string, string>;
}

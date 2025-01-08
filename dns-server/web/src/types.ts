export interface IPRoute {
  addr: string;
  iface: string;
  dnsRecords?: DNSRecord[];
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
  ips: string[];
  routed?: string[];
}

export interface LogEntry {
  cursor: string;
  time: Date;
  level: string;
  msg: string;
  attrs: Record<string, string>;
}

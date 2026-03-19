export interface IPRoute {
	table: number;
	iface: string;
	addr: string;
	reason: string;
	added_at: Date;
	dns_records?: DNSRecord[];
}

export interface DNSRecord {
	ip: string;
	domain: string;
	resolved: Date;
	expires: Date;
}

export interface DNSQuery {
	cursor: string;
	time: Date;
	client_addr: string;
	duration: number;
	resolved_by: {
		resolver: string;
		duration: number;
	};
	domain: string;
	cnames?: DomainEntry<string>[];
	ips: DomainIP[];
	routed_ips?: Record<string, RoutedIP>;
}

export interface DomainEntry<T> {
	name: T;
	ttl: number;
}

export interface DomainIP {
	ip: string;
	ttl: number;
	ptr?: DomainEntry<string>[];
	soa?: DomainEntry<string>[];
}

export interface RoutedIP {
	ip: string;
	static: boolean;
	iface: string;
	reason: string;
	added: boolean;
}

export interface LogEntry {
	cursor: string;
	time: Date;
	level: string;
	msg: string;
	attrs?: Record<string, string>;
}

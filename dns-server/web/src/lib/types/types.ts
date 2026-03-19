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
	client_ip: string;
	client_addr: string;
	duration: number;
	resolver: ResolverInfo;
	domain: string;
	cnames?: DomainEntry<string>[];
	ips: DomainIP[];
	routed_ips?: Record<string, RoutedIP>;
}

export interface ResolverInfo {
	name: string;
	duration: number;
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
	ptr_resolver: ResolverInfo;
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

export interface DNSRawQuery {
	cursor: string;
	time: Date;
	client_addr: string;
	response?: boolean;
	msg: string;
	error?: string;
}

export interface HostInfo {
	mac?: string;
	via?: string;
	ip?: string;
	hostname?: string;
	name?: string;
	registered?: boolean;
	access?: string;
	priority?: number;
	active?: boolean;
	rx_bytes?: number;
	tx_bytes?: number;
	link?: string;
	uptime?: number;
	first_seen?: number;
	last_seen?: number;
	auto_negotiation?: boolean;
	speed?: number;
	duplex?: boolean;
	port?: number;
	system_mode?: string;
	http_port?: number;
	http_host?: string;
	region?: string;
	description?: string;
	firmware?: string;
	interface?: HostInterface;
	dhcp?: HostDHCP;
	mws?: HostMWS;
}

export interface HostInterface {
	id?: string;
	name?: string;
	description?: string;
}

export interface HostDHCP {
	expires?: number;
}

export interface HostMWS {
	cid?: string;
	ap?: string;
	psm?: boolean;
	mld?: boolean;
	authenticated?: boolean;
	tx_rate?: number;
	uptime?: number;
	rssi?: number;
	mcs?: number;
	security?: string;
}

export interface ListHostsResponse {
	hosts?: HostInfo[];
}

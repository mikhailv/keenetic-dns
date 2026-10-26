<script lang="ts">
	import type { IPGeoInfo } from '$lib/types';
	import { type HostStore, hostName } from '$lib/stores';
	import { primaryDomain } from './util';

	let {
		ip,
		hosts,
		ips,
		names = {}
	}: {
		ip: string;
		hosts: HostStore;
		ips: Record<string, IPGeoInfo>;
		names?: Record<string, string[]>;
	} = $props();

	const info = $derived(ips[ip]);
	const host = $derived(hostName($hosts.byIP[ip]));
	const domains = $derived(names[ip] ?? []);
	const asn = $derived(
		info
			? `${info.asn ? `AS${info.asn} ${info.org ?? ''}` : info.network}${info.country ? ` · ${info.country}` : ''}`
			: undefined
	);
	const label = $derived([host ?? primaryDomain(domains), asn].filter(Boolean).join(' · ') || '?');
	const title = $derived(
		[
			...domains,
			info?.asn ? `AS${info.asn} ${info.org ?? ''}` : undefined,
			info ? [info.network, info.city, info.country].filter(Boolean).join(' · ') : undefined
		]
			.filter(Boolean)
			.join('\n') || undefined
	);
</script>

<span {title}>
	{ip}
	<span class="text-base-content/60 font-light text-sm2">/ {label}</span>
</span>

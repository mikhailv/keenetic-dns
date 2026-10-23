<script lang="ts">
	import type { IPGeoInfo } from '$lib/types';
	import { type HostStore, hostName } from '$lib/stores';

	let {
		ip,
		hosts,
		ips
	}: {
		ip: string;
		hosts: HostStore;
		ips: Record<string, IPGeoInfo>;
	} = $props();

	const info = $derived(ips[ip]);
	const host = $derived(hostName($hosts.byIP[ip]));
	const title = $derived(info ? [info.network, info.city, info.country].filter(Boolean).join(' · ') : undefined);
</script>

{ip}
{#if info}
	<span class="text-base-content/60 font-light text-sm2" {title}>
		/ {info.asn ? `AS${info.asn} ${info.org ?? ''}` : info.network}{info.country ? ` · ${info.country}` : ''}
	</span>
{:else}
	<span class="text-base-content/60 font-light text-sm2">/ {host ?? '?'}</span>
{/if}

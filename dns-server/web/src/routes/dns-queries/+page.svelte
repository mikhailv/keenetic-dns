<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/services/api';
	import type { DNSQuery, DomainIP } from '$lib/types';
	import { type StreamStore, createHostStore } from '$lib/stores';

	const MAX_ITEMS = 200;

	const stream: StreamStore<DNSQuery> = api.createDNSQueryStreamStore(MAX_ITEMS);
	const hosts = createHostStore();

	onMount(() => hosts.autoreload());
	onMount(() => stream.start());

	function formatTime(date: Date): string {
		return date.toTimeString().split(' ')[0];
	}

	function formatMilli(seconds: number): string {
		return (seconds * 1000).toFixed(2);
	}

	function ipTitle(ip: DomainIP): string {
		const ptrInfo = ip.ptr?.map((p) => `PTR: ${p.name} (${p.ttl})`).join('\n') ?? '';
		const soaInfo = ip.soa?.map((s) => `SOA: ${s.name} (${s.ttl})`).join('\n') ?? '';
		return [
			`IP: ${ip.ip}`,
			`TTL: ${ip.ttl}`,
			ptrInfo,
			soaInfo,
			`Resolver: ${ip.ptr_resolver.name}`,
			`Duration: ${formatMilli(ip.ptr_resolver.duration)} ms`
		]
			.filter((s) => s !== '')
			.join('\n');
	}

	function shortenDomain(domain: string, segmentMaxSize = 24): string {
		return domain
			.split('.')
			.map((s) => (s.length > segmentMaxSize ? s.substring(0, 6) + '…' + s.substring(s.length - 6) : s))
			.join('.');
	}
</script>

<svelte:head>
	<title>DNS Queries - Keenetic DNS</title>
</svelte:head>

{#if $stream.error}
	<div class="alert alert-error" role="alert">{$stream.error}</div>
{/if}

<div class="overflow-x-auto">
	<table class="table">
		<thead>
			<tr>
				<th>Time</th>
				<th>Client</th>
				<th>Domain</th>
				<th>TTL</th>
				<th>IP</th>
				<th>Routed</th>
				<th>Duration</th>
			</tr>
		</thead>
		<tbody class="border-t border-base-300">
			{#each $stream.items as query (query.cursor)}
				{@const host = $hosts.byIP[query.client_ip]}
				<tr class="hover animate-new-row">
					<td title={query.time.toLocaleString()} class="font-light text-sm1">{formatTime(query.time)}</td>
					<td class="text-sm1">
						{#if host}
							{host.name}
							<div class="font-light text-sm2">{query.client_ip}</div>
						{:else}
							{query.client_ip}
						{/if}
					</td>
					<td>
						<div title={query.domain}>
							{shortenDomain(query.domain)}
						</div>
						<div class="font-light text-sm2" title="resolver">
							{query.resolver.name} / {formatMilli(query.resolver.duration)} ms
							{#if query.reused_from}
								<span class="badge badge-sm ml-2" title="from query {query.reused_from}">reused</span>
							{/if}
						</div>
					</td>
					<td class="font-light text-sm1">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ttl}</div>
						{/each}
					</td>
					<td class="font-light text-sm1">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ip}</div>
						{/each}
					</td>
					<td class="font-light text-sm1 whitespace-nowrap">
						{#each query.ips as it (it.ip)}
							{@const route = query.ip_routings?.[it.ip]}
							<div>
								{#if route}
									{#if route.action === 'ignored'}
										<span class="text-base-content/60 italic">- ({route.reason})</span>
									{:else}
										{route.iface} ({route.reason}){route.added ? ' (added)' : ''}
									{/if}
								{:else}
									-
								{/if}
							</div>
						{/each}
					</td>
					<td class="font-light text-sm3">{formatMilli(query.duration)} ms</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

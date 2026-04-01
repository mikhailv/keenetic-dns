<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { DNSQuery, DomainIP } from '$lib/types';
	import { type StreamStore, createHostStore } from '$lib/stores';

	const MAX_ITEMS = 200;
	const HOSTS_RELOAD_INTERVAL = 30_000;

	const stream: StreamStore<DNSQuery> = api.createDNSQueryStreamStore(MAX_ITEMS);
	const hosts = createHostStore();
	let hostsReloadInterval: ReturnType<typeof setInterval>;

	onMount(() => {
		stream.start();
		hosts.reload();
		hostsReloadInterval = setInterval(() => hosts.reload(), HOSTS_RELOAD_INTERVAL);
	});

	onDestroy(() => {
		stream.stop();
		clearInterval(hostsReloadInterval);
	});

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

<h1>DNS Queries</h1>

{#if $stream.error}
	<div class="alert alert-danger" role="alert">{$stream.error}</div>
{/if}

<div class="table-responsive">
	<table class="table table-sm table-hover table-sticky-header">
		<thead>
			<tr>
				<th scope="col">Time</th>
				<th scope="col">Client</th>
				<th scope="col">Domain</th>
				<th scope="col">TTL</th>
				<th scope="col">IP</th>
				<th scope="col">Routed</th>
				<th scope="col">Duration</th>
			</tr>
		</thead>
		<tbody class="table-group-divider">
			{#each $stream.items as query (query.cursor)}
				{@const host = $hosts.byIP[query.client_ip]}
				<tr class="animate-new-row">
					<td title={query.time.toLocaleString()} class="fw-light text-sm1">{formatTime(query.time)}</td>
					<td class="text-sm1">
						{#if host}
							{host.name}
							<div class="fw-light text-sm2">{query.client_ip}</div>
						{:else}
							{query.client_ip}
						{/if}
					</td>
					<td>
						<div title={query.domain}>
							{shortenDomain(query.domain)}
						</div>
						<div class="fw-light text-sm2" title="resolver">
							{query.resolver.name} / {formatMilli(query.resolver.duration)} ms
						</div>
					</td>
					<td class="fw-light text-sm1">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ttl}</div>
						{/each}
					</td>
					<td class="fw-light text-sm1">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ip}</div>
						{/each}
					</td>
					<td class="fw-light text-sm1 text-nowrap">
						{#each query.ips as it (it.ip)}
							{@const route = query.ip_routings?.[it.ip]}
							<div>
								{#if route}
									{#if route.action === 'ignored'}
										<span class="text-secondary italic">- ({route.reason})</span>
									{:else}
										{route.iface} ({route.reason}){route.added ? ' (added)' : ''}
									{/if}
								{:else}
									-
								{/if}
							</div>
						{/each}
					</td>
					<td class="fw-light text-sm3">{formatMilli(query.duration)} ms</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

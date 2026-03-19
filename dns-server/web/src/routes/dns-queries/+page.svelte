<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { DNSQuery, DomainIP } from '$lib/types';
	import type { StreamStore } from '$lib/stores';

	const MAX_ITEMS = 200;

	let stream: StreamStore<DNSQuery> = api.createDNSQueryStreamStore(MAX_ITEMS);

	onMount(() => {
		stream.start();
	});

	onDestroy(() => {
		stream.stop();
	});

	function formatTime(date: Date): string {
		return date.toTimeString().split(' ')[0];
	}

	function ipTitle(ip: DomainIP): string {
		const ptrInfo = ip.ptr?.map((p) => `PTR: ${p.name} (${p.ttl})`).join('\n') ?? '';
		const soaInfo = ip.soa?.map((s) => `SOA: ${s.name} (${s.ttl})`).join('\n') ?? '';
		return `IP: ${ip.ip}\nTTL: ${ip.ttl}\n${ptrInfo}${soaInfo}`;
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
				<tr class="animate-new-row">
					<td title={query.time.toLocaleString()} class="fw-light text-sm1"
						>{formatTime(query.time)}</td
					>
					<td class="text-sm1">{query.client_addr.split(':')[0]}</td>
					<td>
						{query.domain}
						<div class="fw-light text-sm2" title="resolved by">
							{query.resolved_by.resolver} / {(query.resolved_by.duration * 1000).toFixed(2)} ms
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
					<td class="fw-light text-sm1">
						{#each query.ips as ip (ip.ip)}
							{@const route = query.routed_ips?.[ip.ip]}
							<div>
								{route ? `${route.iface} (${route.reason})${route.added ? ' (added)' : ''}` : '-'}
							</div>
						{/each}
					</td>
					<td class="fw-light text-sm3">{(query.duration * 1000).toFixed(2)} ms</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

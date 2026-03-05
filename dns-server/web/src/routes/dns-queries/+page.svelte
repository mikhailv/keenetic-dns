<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { DNSQuery, DomainIP } from '$lib/types';
	import { listenStream, type Stream } from '$lib/stream';

	const MAX_ITEMS = 200;
	const USE_WS = false;

	let queries = $state<DNSQuery[]>([]);
	let error = $state<string | null>(null);

	let ws: WebSocket | null = null;
	let stream: Stream<DNSQuery[]> | null = null;

	onMount(() => {
		if (USE_WS) {
			connectWebSocket();
		} else {
			stream = api.streamDNSQueries(MAX_ITEMS);
			listenStream(stream, async (res) => {
				if (res !== 'cancelled' && res.value) {
					// Add new queries to the beginning, maintaining max items limit
					queries = [...[...res.value].reverse(), ...queries].slice(0, MAX_ITEMS);
				}
			});
		}
	});

	onDestroy(() => {
		stream?.cancel();
		ws?.close();
	});

	function connectWebSocket() {
		ws = api.createDNSQueriesWebSocket(MAX_ITEMS);

		ws.onmessage = (event) => {
			const data: DNSQuery[] = JSON.parse(event.data);
			data.forEach((q) => (q.time = new Date(q.time)));
			// Add new queries to the beginning, maintaining max items limit
			queries = [...data.reverse(), ...queries].slice(0, MAX_ITEMS);
		};

		ws.onerror = () => {
			error = 'WebSocket connection failed';
		};

		ws.onclose = () => {
			// Auto-reconnect after 2 seconds
			setTimeout(connectWebSocket, 2000);
		};
	}

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

{#if error}
	<div class="alert alert-danger" role="alert">{error}</div>
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
			{#each queries as query (query.cursor)}
				<tr class="animate-new-row">
					<td title={query.time.toLocaleString()}>{formatTime(query.time)}</td>
					<td>{query.client_addr.split(':')[0]}</td>
					<td>{query.domain}</td>
					<td class="fw-light" style="font-size: 0.9rem">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ttl}</div>
						{/each}
					</td>
					<td class="fw-light" style="font-size: 0.9rem">
						{#each query.ips as ip (ip.ip)}
							<div title={ipTitle(ip)}>{ip.ip}</div>
						{/each}
					</td>
					<td class="fw-light" style="font-size: 0.9rem">
						{#each query.ips as ip (ip.ip)}
							<div>
								{ip.route_iface
									? `${ip.route_iface} (${ip.route_reason})${ip.route_added ? ' (added)' : ''}`
									: '-'}
							</div>
						{/each}
					</td>
					<td class="fw-light" style="font-size: 0.9rem">{query.duration}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

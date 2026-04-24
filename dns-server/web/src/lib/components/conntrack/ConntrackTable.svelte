<script lang="ts">
	import type { ConntrackBucket, ConntrackEntry } from '$lib/types';
	import { formatBytes } from './util';
	import { SvelteMap } from 'svelte/reactivity';
	import { type HostStore } from '$lib/stores';
	import { formatNumber } from '$lib/util';

	let {
		buckets,
		hosts
	}: {
		buckets: ConntrackBucket[];
		hosts: HostStore;
	} = $props();

	interface GroupRow extends Omit<ConntrackEntry, 'dst_ip' | 'dst_port' | 'protocol'> {
		items: GroupItemRow[];
		conn_count: number;
	}

	interface GroupItemRow extends Omit<ConntrackEntry, 'src_ip'> {
		key: string;
		conn_count: number;
	}

	function detailKey(e: ConntrackEntry): string {
		return `${e.protocol}|${e.dst_ip}|${e.dst_port}`;
	}

	const groups = $derived.by(() => {
		const groupMap = new SvelteMap<string, GroupRow>();
		const itemsMap = new SvelteMap<string, Map<string, GroupItemRow>>();
		for (const b of buckets) {
			for (const e of b.entries) {
				let g = groupMap.get(e.src_ip);
				if (!g) {
					g = {
						src_ip: e.src_ip,
						bytes_orig: 0,
						bytes_reply: 0,
						packets_orig: 0,
						packets_reply: 0,
						conn_count: 0,
						items: []
					};
					groupMap.set(e.src_ip, g);
					itemsMap.set(e.src_ip, new SvelteMap());
				}
				g.bytes_orig += e.bytes_orig;
				g.bytes_reply += e.bytes_reply;
				g.packets_orig += e.packets_orig;
				g.packets_reply += e.packets_reply;
				g.conn_count += e.conn_count;

				const dmap = itemsMap.get(e.src_ip)!;
				const dk = detailKey(e);
				let d = dmap.get(dk);
				if (!d) {
					d = {
						key: dk,
						protocol: e.protocol,
						dst_ip: e.dst_ip,
						dst_port: e.dst_port,
						bytes_orig: 0,
						bytes_reply: 0,
						packets_orig: 0,
						packets_reply: 0,
						conn_count: 0
					};
					dmap.set(dk, d);
				}
				d.bytes_orig += e.bytes_orig;
				d.bytes_reply += e.bytes_reply;
				d.packets_orig += e.packets_orig;
				d.packets_reply += e.packets_reply;
				d.conn_count += e.conn_count;
			}
		}
		const out: GroupRow[] = [];
		for (const [src, g] of groupMap) {
			g.items = [...itemsMap.get(src)!.values()].sort(
				(a, b) => b.bytes_orig + b.bytes_reply - (a.bytes_orig + a.bytes_reply)
			);
			out.push(g);
		}
		out.sort((a, b) => b.bytes_orig + b.bytes_reply - (a.bytes_orig + a.bytes_reply));
		return out;
	});

	const expanded: Record<string, boolean> = $state({});

	function toggle(src: string) {
		expanded[src] = !expanded[src];
	}
</script>

<table class="table">
	<thead>
		<tr>
			<th style="width: 1px"></th>
			<th>Source IP</th>
			<th class="text-right">Dsts</th>
			<th class="text-right">Bytes ↑</th>
			<th class="text-right">Bytes ↓</th>
			<th class="text-right">Pkts ↑</th>
			<th class="text-right">Pkts ↓</th>
			<th class="text-right">Conns</th>
		</tr>
	</thead>
	<tbody>
		{#each groups as g (g.src_ip)}
			<tr class="hover cursor-pointer" onclick={() => toggle(g.src_ip)}>
				<td>{expanded[g.src_ip] ? '▼' : '▶'}</td>
				<td>
					{g.src_ip}
					<span class="text-base-content/60 font-light text-sm2">/ {$hosts.byIP[g.src_ip]?.name ?? '?'}</span>
				</td>
				<td class="text-right">{g.items.length}</td>
				<td class="text-right">{formatBytes(g.bytes_orig)}</td>
				<td class="text-right">{formatBytes(g.bytes_reply)}</td>
				<td class="text-right">{formatNumber(g.packets_orig)}</td>
				<td class="text-right">{formatNumber(g.packets_reply)}</td>
				<td class="text-right">{formatNumber(g.conn_count)}</td>
			</tr>
			{#if expanded[g.src_ip]}
				<tr>
					<td></td>
					<td colspan="7" style="padding: 0">
						<table class="table mb-0">
							<thead>
								<tr>
									<th>Proto</th>
									<th>Dst IP</th>
									<th>Dst Port</th>
									<th class="text-right">Bytes ↑</th>
									<th class="text-right">Bytes ↓</th>
									<th class="text-right">Pkts ↑</th>
									<th class="text-right">Pkts ↓</th>
									<th class="text-right">Conns</th>
								</tr>
							</thead>
							<tbody>
								{#each g.items as d (d.key)}
									<tr>
										<td>{d.protocol}</td>
										<td>
											{d.dst_ip}
											<span class="text-base-content/60 font-light text-sm2"
												>/ {$hosts.byIP[d.dst_ip]?.name ?? '?'}</span>
										</td>
										<td>{d.dst_port}</td>
										<td class="text-right">{formatBytes(d.bytes_orig)}</td>
										<td class="text-right">{formatBytes(d.bytes_reply)}</td>
										<td class="text-right">{formatNumber(d.packets_orig)}</td>
										<td class="text-right">{formatNumber(d.packets_reply)}</td>
										<td class="text-right">{formatNumber(d.conn_count)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</td>
				</tr>
			{/if}
		{/each}
	</tbody>
</table>

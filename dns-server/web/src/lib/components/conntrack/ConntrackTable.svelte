<script lang="ts">
	import type { ConntrackBucket, ConntrackEntry } from '$lib/types';
	import { formatBytes } from './util';
	import { SvelteMap } from 'svelte/reactivity';
	import { createHostStore } from '$lib/stores';
	import { onMount } from 'svelte';

	let { buckets }: { buckets: ConntrackBucket[] } = $props();

	const hosts = createHostStore();

	onMount(() => hosts.autoreload());

	interface GroupRow extends Omit<ConntrackEntry, 'dst_ip' | 'dst_port' | 'protocol' | 'src_ports'> {
		items: GroupItemRow[];
		src_ports: Set<number>;
	}

	interface GroupItemRow extends Omit<ConntrackEntry, 'src_ip' | 'src_ports'> {
		key: string;
		src_ports: Set<number>;
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
						src_ports: new Set<number>(),
						items: []
					};
					groupMap.set(e.src_ip, g);
					itemsMap.set(e.src_ip, new SvelteMap());
				}
				g.bytes_orig += e.bytes_orig;
				g.bytes_reply += e.bytes_reply;
				g.packets_orig += e.packets_orig;
				g.packets_reply += e.packets_reply;
				e.src_ports.forEach((p) => g.src_ports.add(p));

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
						src_ports: new Set<number>()
					};
					dmap.set(dk, d);
				}
				d.bytes_orig += e.bytes_orig;
				d.bytes_reply += e.bytes_reply;
				d.packets_orig += e.packets_orig;
				d.packets_reply += e.packets_reply;
				e.src_ports.forEach((p) => d.src_ports.add(p));
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

<table class="table table-sm align-middle">
	<thead>
		<tr>
			<th></th>
			<th>Source IP</th>
			<th class="text-end">Dsts</th>
			<th class="text-end">Bytes ↑</th>
			<th class="text-end">Bytes ↓</th>
			<th class="text-end">Pkts ↑</th>
			<th class="text-end">Pkts ↓</th>
			<th class="text-end">Conns</th>
		</tr>
	</thead>
	<tbody>
		{#each groups as g (g.src_ip)}
			<tr style="cursor:pointer" onclick={() => toggle(g.src_ip)}>
				<td>{expanded[g.src_ip] ? '▼' : '▶'}</td>
				<td>
					{g.src_ip}
					<span class="text-muted fw-light text-sm2">/ {$hosts.byIP[g.src_ip]?.name ?? '?'}</span>
				</td>
				<td class="text-end">{g.items.length}</td>
				<td class="text-end">{formatBytes(g.bytes_orig)}</td>
				<td class="text-end">{formatBytes(g.bytes_reply)}</td>
				<td class="text-end">{g.packets_orig}</td>
				<td class="text-end">{g.packets_reply}</td>
				<td class="text-end">{g.src_ports.size}</td>
			</tr>
			{#if expanded[g.src_ip]}
				<tr>
					<td></td>
					<td colspan="7">
						<table class="table table-sm mb-0">
							<thead>
								<tr>
									<th>Proto</th>
									<th>Dst IP</th>
									<th>Dst Port</th>
									<th class="text-end">Bytes ↑</th>
									<th class="text-end">Bytes ↓</th>
									<th class="text-end">Pkts ↑</th>
									<th class="text-end">Pkts ↓</th>
									<th class="text-end">Conns</th>
								</tr>
							</thead>
							<tbody>
								{#each g.items as d (d.key)}
									<tr>
										<td>{d.protocol}</td>
										<td>
											{d.dst_ip}
											<span class="text-muted fw-light text-sm2">/ {$hosts.byIP[d.dst_ip]?.name ?? '?'}</span>
										</td>
										<td>{d.dst_port}</td>
										<td class="text-end">{formatBytes(d.bytes_orig)}</td>
										<td class="text-end">{formatBytes(d.bytes_reply)}</td>
										<td class="text-end">{d.packets_orig}</td>
										<td class="text-end">{d.packets_reply}</td>
										<td class="text-end">{d.src_ports.size}</td>
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

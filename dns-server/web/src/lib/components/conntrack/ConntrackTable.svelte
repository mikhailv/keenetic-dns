<script lang="ts">
	import type { ConntrackBucket, ConntrackEntry, ConntrackGroupBy, IPGeoInfo } from '$lib/types';
	import { formatBytes, primaryDomain } from './util';
	import { SvelteMap } from 'svelte/reactivity';
	import { type HostStore } from '$lib/stores';
	import { formatNumber } from '$lib/util';
	import IPLabel from './IPLabel.svelte';

	let {
		buckets,
		hosts,
		ips = {},
		names = {},
		groupBy = 'host'
	}: {
		buckets: ConntrackBucket[];
		hosts: HostStore;
		ips?: Record<string, IPGeoInfo>;
		names?: Record<string, string[]>;
		groupBy?: ConntrackGroupBy;
	} = $props();

	interface Stats {
		bytes_orig: number;
		bytes_reply: number;
		packets_orig: number;
		packets_reply: number;
		conn_count: number;
	}

	interface GroupRow extends Stats {
		key: string;
		ip?: string;
		label?: string;
		asns?: number[];
		items: ItemRow[];
	}

	interface ItemRow extends Stats {
		key: string;
		protocol: string;
		ip: string;
		local_ip?: string;
		dst_port: number;
	}

	function emptyStats(): Stats {
		return { bytes_orig: 0, bytes_reply: 0, packets_orig: 0, packets_reply: 0, conn_count: 0 };
	}

	function addStats(s: Stats, e: ConntrackEntry) {
		s.bytes_orig += e.bytes_orig;
		s.bytes_reply += e.bytes_reply;
		s.packets_orig += e.packets_orig;
		s.packets_reply += e.packets_reply;
		s.conn_count += e.conn_count;
	}

	function byBytes(a: Stats, b: Stats): number {
		return b.bytes_orig + b.bytes_reply - (a.bytes_orig + a.bytes_reply);
	}

	function hostGroup(e: ConntrackEntry): Omit<GroupRow, keyof Stats | 'items'> {
		return { key: e.src_ip, ip: e.src_ip };
	}

	function hostItem(e: ConntrackEntry): Omit<ItemRow, keyof Stats> {
		return { key: `${e.protocol}|${e.dst_ip}|${e.dst_port}`, protocol: e.protocol, ip: e.dst_ip, dst_port: e.dst_port };
	}

	function remoteSide(e: ConntrackEntry): { remote: string; local: string } {
		if (!ips[e.dst_ip] && ips[e.src_ip]) {
			return { remote: e.src_ip, local: e.dst_ip };
		}
		return { remote: e.dst_ip, local: e.src_ip };
	}

	function asnGroup(e: ConntrackEntry): Omit<GroupRow, keyof Stats | 'items'> {
		const info = ips[remoteSide(e).remote];
		if (!info?.asn) {
			return { key: 'unknown', label: 'Local / unresolved' };
		}
		return { key: `AS${info.asn}`, label: `AS${info.asn} ${info.org ?? ''}` };
	}

	function orgGroup(e: ConntrackEntry): Omit<GroupRow, keyof Stats | 'items'> {
		const info = ips[remoteSide(e).remote];
		const org = info?.org?.trim();
		if (!info?.asn || !org) {
			return asnGroup(e);
		}
		return { key: `org:${org.toLowerCase()}`, label: org, asns: [info.asn] };
	}

	function domainGroup(e: ConntrackEntry): Omit<GroupRow, keyof Stats | 'items'> {
		const name = primaryDomain(names[remoteSide(e).remote]);
		if (!name) {
			return { key: 'nodns', label: 'No DNS name' };
		}
		return { key: `dns:${name}`, label: name };
	}

	function domain2Group(e: ConntrackEntry): Omit<GroupRow, keyof Stats | 'items'> {
		const name = primaryDomain(names[remoteSide(e).remote]);
		if (!name) {
			return { key: 'nodns', label: 'No DNS name' };
		}
		const base = name.split('.').slice(-2).join('.');
		return { key: `dns2:${base}`, label: base };
	}

	function groupLabel(g: GroupRow): string {
		if (!g.asns) {
			return g.label ?? '';
		}
		const asns = [...g.asns].sort((a, b) => a - b).map((a) => `AS${a}`);
		return `${g.label} (${asns.join(', ')})`;
	}

	function asnItem(e: ConntrackEntry): Omit<ItemRow, keyof Stats> {
		const { remote, local } = remoteSide(e);
		return {
			key: `${e.protocol}|${remote}|${local}|${e.dst_port}`,
			protocol: e.protocol,
			ip: remote,
			local_ip: local,
			dst_port: e.dst_port
		};
	}

	const groups = $derived.by(() => {
		const groupOf = { host: hostGroup, asn: asnGroup, org: orgGroup, domain: domainGroup, domain2: domain2Group }[
			groupBy
		];
		const itemOf = groupBy === 'host' ? hostItem : asnItem;
		const groupMap = new SvelteMap<string, GroupRow>();
		const itemsMap = new SvelteMap<string, Map<string, ItemRow>>();
		for (const b of buckets) {
			for (const e of b.entries) {
				const gk = groupOf(e);
				let g = groupMap.get(gk.key);
				if (!g) {
					g = { ...gk, ...emptyStats(), items: [] };
					groupMap.set(gk.key, g);
					itemsMap.set(gk.key, new SvelteMap());
				} else if (g.asns && gk.asns && !g.asns.includes(gk.asns[0])) {
					g.asns.push(gk.asns[0]);
				}
				addStats(g, e);

				const ik = itemOf(e);
				const dmap = itemsMap.get(gk.key)!;
				let d = dmap.get(ik.key);
				if (!d) {
					d = { ...ik, ...emptyStats() };
					dmap.set(ik.key, d);
				}
				addStats(d, e);
			}
		}
		const out: GroupRow[] = [];
		for (const [key, g] of groupMap) {
			g.items = [...itemsMap.get(key)!.values()].sort(byBytes);
			out.push(g);
		}
		return out.sort(byBytes);
	});

	const expanded: Record<string, boolean> = $state({});

	function toggle(key: string) {
		expanded[key] = !expanded[key];
	}
</script>

<table class="table">
	<thead>
		<tr>
			<th style="width: 1px"></th>
			<th
				>{{ host: 'Source IP', asn: 'ASN', org: 'Organization', domain: 'Domain', domain2: 'Base domain' }[
					groupBy
				]}</th>
			<th class="text-right">{groupBy === 'host' ? 'Dsts' : 'Flows'}</th>
			<th class="text-right">Bytes ↑</th>
			<th class="text-right">Bytes ↓</th>
			<th class="text-right">Pkts ↑</th>
			<th class="text-right">Pkts ↓</th>
			<th class="text-right">Conns</th>
		</tr>
	</thead>
	<tbody>
		{#each groups as g (g.key)}
			<tr class="hover cursor-pointer" onclick={() => toggle(g.key)}>
				<td>{expanded[g.key] ? '▼' : '▶'}</td>
				<td>
					{#if g.ip}
						<IPLabel ip={g.ip} {hosts} {ips} {names} />
					{:else}
						{groupLabel(g)}
					{/if}
				</td>
				<td class="text-right">{g.items.length}</td>
				<td class="text-right">{formatBytes(g.bytes_orig)}</td>
				<td class="text-right">{formatBytes(g.bytes_reply)}</td>
				<td class="text-right">{formatNumber(g.packets_orig)}</td>
				<td class="text-right">{formatNumber(g.packets_reply)}</td>
				<td class="text-right">{formatNumber(g.conn_count)}</td>
			</tr>
			{#if expanded[g.key]}
				<tr>
					<td></td>
					<td colspan="7" style="padding: 0">
						<table class="table mb-0">
							<thead>
								<tr>
									<th>Proto</th>
									<th>{groupBy === 'host' ? 'Dst IP' : 'Remote IP'}</th>
									{#if groupBy !== 'host'}<th>Local IP</th>{/if}
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
										<td><IPLabel ip={d.ip} {hosts} {ips} {names} /></td>
										{#if d.local_ip}<td><IPLabel ip={d.local_ip} {hosts} {ips} {names} /></td>{/if}
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

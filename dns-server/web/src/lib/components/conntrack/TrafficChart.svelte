<script lang="ts">
	import type { ConntrackBucket, ConntrackEntry } from '$lib/types';
	import { formatBytes } from './util';

	let { buckets }: { buckets: ConntrackBucket[] } = $props();

	const TOP_N = 10;
	const WIDTH = 800;
	const HEIGHT = 240;
	const PAD = { top: 10, right: 10, bottom: 24, left: 60 };

	function entryKey(e: ConntrackEntry): string {
		return `${e.protocol}|${e.src_ip}|${e.dst_ip}|${e.dst_port}`;
	}

	function entryBytes(e: ConntrackEntry): number {
		return e.bytes_orig + e.bytes_reply;
	}

	const totals = $derived.by(() => {
		const m: Record<string, { key: string; label: string; bytes: number }> = {};
		for (const b of buckets) {
			for (const e of b.entries) {
				const k = entryKey(e);
				const cur = m[k];
				const v = entryBytes(e);
				if (cur) cur.bytes += v;
				else m[k] = { key: k, label: `${e.src_ip}→${e.dst_ip}:${e.dst_port}`, bytes: v };
			}
		}
		return Object.values(m).sort((a, b) => b.bytes - a.bytes);
	});

	const topKeys = $derived(new Set(totals.slice(0, TOP_N).map((t) => t.key)));
	const legend = $derived([
		...totals.slice(0, TOP_N),
		...(totals.length > TOP_N ? [{ key: '__other', label: 'other', bytes: 0 }] : [])
	]);

	const stacked = $derived.by(() => {
		return buckets.map((b) => {
			const slices: Record<string, number> = {};
			let total = 0;
			for (const e of b.entries) {
				const k = entryKey(e);
				const bucketKey = topKeys.has(k) ? k : '__other';
				const v = entryBytes(e);
				slices[bucketKey] = (slices[bucketKey] ?? 0) + v;
				total += v;
			}
			return { time: b.time_range.start, slices, total };
		});
	});

	const maxTotal = $derived(stacked.reduce((m, s) => (s.total > m ? s.total : m), 1));
	const barWidth = $derived(stacked.length > 0 ? (WIDTH - PAD.left - PAD.right) / stacked.length : 0);

	const palette = [
		'#4e79a7',
		'#f28e2c',
		'#e15759',
		'#76b7b2',
		'#59a14f',
		'#edc949',
		'#af7aa1',
		'#ff9da7',
		'#9c755f',
		'#bab0ab',
		'#cccccc'
	];

	function colorFor(key: string): string {
		const idx = legend.findIndex((l) => l.key === key);
		return palette[idx >= 0 ? idx % palette.length : palette.length - 1];
	}

	function fmtTime(unix: number): string {
		return new Date(unix * 1000).toLocaleTimeString();
	}
</script>

{#if buckets.length === 0}
	<div class="text-muted">No data</div>
{:else}
	<svg viewBox="0 0 {WIDTH} {HEIGHT}" class="w-100" style="max-height:240px;">
		<line
			x1={PAD.left}
			y1={HEIGHT - PAD.bottom}
			x2={WIDTH - PAD.right}
			y2={HEIGHT - PAD.bottom}
			stroke="currentColor"
			stroke-opacity="0.3" />
		<text x={PAD.left - 4} y={PAD.top + 8} text-anchor="end" font-size="10" fill="currentColor">
			{formatBytes(maxTotal)}
		</text>
		<text x={PAD.left - 4} y={HEIGHT - PAD.bottom} text-anchor="end" font-size="10" fill="currentColor"> 0 </text>
		{#each stacked as bar, i (bar.time)}
			{@const x = PAD.left + i * barWidth}
			{@const innerW = Math.max(barWidth - 1, 1)}
			{@const segments = (() => {
				let acc = 0;
				const out: { y: number; h: number; key: string }[] = [];
				for (const l of legend) {
					const v = bar.slices[l.key] ?? 0;
					if (v <= 0) continue;
					const h = ((HEIGHT - PAD.top - PAD.bottom) * v) / maxTotal;
					acc += h;
					out.push({ y: HEIGHT - PAD.bottom - acc, h, key: l.key });
				}
				return out;
			})()}
			{#each segments as seg (seg.key)}
				<rect {x} y={seg.y} width={innerW} height={seg.h} fill={colorFor(seg.key)}>
					<title>{fmtTime(bar.time)} — {formatBytes(bar.total)}</title>
				</rect>
			{/each}
		{/each}
	</svg>

	<div class="d-flex flex-wrap gap-2 mt-2 small">
		{#each legend as l (l.key)}
			<span class="d-inline-flex align-items-center gap-1">
				<span style="display:inline-block;width:10px;height:10px;background:{colorFor(l.key)}"></span>
				{l.label}
			</span>
		{/each}
	</div>
{/if}

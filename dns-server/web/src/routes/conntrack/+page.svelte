<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { api } from '$lib/services/api';
	import type { ConntrackBucket, ConntrackBucketsResponse } from '$lib/types';
	import RangeSelector from '$lib/components/conntrack/RangeSelector.svelte';
	import IntervalSelector from '$lib/components/conntrack/IntervalSelector.svelte';
	import TrafficChart from '$lib/components/conntrack/TrafficChart.svelte';
	import ConntrackTable from '$lib/components/conntrack/ConntrackTable.svelte';
	import { autoBumpInterval } from '$lib/components/conntrack/util';
	import { createHostStore, type StreamStore } from '$lib/stores';

	const AUTO_REFRESH_INTERVAL = 10_000;

	const now0 = Math.floor(Date.now() / 1000);
	let from = $state(now0 - 3600);
	let to = $state(now0);
	let anchored = $state(true);
	let autoRefresh = $state(false);
	let interval = $state(60);

	let resp = $state<ConntrackBucketsResponse | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(false);

	let filterIP = $state<string | null>(null);
	let filterBucketIdx = $state<number | null>(null);

	const rangeSeconds = $derived(Math.max(1, to - from));
	const effectiveInterval = $derived(autoBumpInterval(interval, rangeSeconds));
	const appliedHint = $derived(resp?.interval ?? null);

	const tableBuckets = $derived.by(() => {
		if (!resp) return [];
		let b = resp.buckets;
		if (filterBucketIdx !== null && filterBucketIdx >= 0 && filterBucketIdx < b.length) {
			b = [b[filterBucketIdx]];
		}
		if (filterIP) {
			const ip = filterIP;
			b = b.map((bucket) => ({
				...bucket,
				entries: bucket.entries.filter((e) => e.src_ip === ip)
			}));
		}
		return b;
	});

	const hosts = createHostStore();
	onMount(() => hosts.autoreload());

	const stream: StreamStore<ConntrackBucket> = api.createConntrackStreamStore(0);
	onMount(() => stream.start());

	async function load() {
		loading = true;
		const res = await api.getConntrackBuckets(from, to, effectiveInterval);
		resp = res.data ?? null;
		error = res.error ?? null;
		loading = false;
	}

	function updateAnchor() {
		const span = to - from;
		to = Math.floor(Date.now() / 1000);
		from = to - span;
	}

	let timer: ReturnType<typeof setInterval> | null = null;
	$effect(() => {
		if (timer) clearInterval(timer);
		if (autoRefresh && anchored) {
			timer = setInterval(updateAnchor, AUTO_REFRESH_INTERVAL);
		}
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	$effect(() => {
		void from;
		void to;
		void interval;
		void effectiveInterval;
		load();
	});
</script>

<div class="flex flex-wrap gap-3 items-center mb-3">
	<RangeSelector bind:from bind:to bind:anchored bind:autoRefresh />
	<IntervalSelector bind:interval {rangeSeconds} {appliedHint} />
	{#if loading}<small class="text-base-content/60">loading…</small>{/if}
</div>

{#if error}
	<div class="alert alert-error">{error}</div>
{/if}

{#if resp}
	<TrafficChart
		buckets={resp.buckets}
		{hosts}
		{filterIP}
		{filterBucketIdx}
		onfilterip={(ip: string) => (filterIP = filterIP === ip ? null : ip)}
		onselectbar={(idx: number) => (filterBucketIdx = filterBucketIdx === idx ? null : idx)} />
	{#if filterIP || filterBucketIdx !== null}
		<div class="flex items-center gap-2 mt-2 text-sm text-base-content/70">
			<span>Filtered by:</span>
			{#if filterIP}
				<button class="badge badge-outline gap-1" onclick={() => (filterIP = null)}>
					IP: {filterIP} ✕
				</button>
			{/if}
			{#if filterBucketIdx !== null}
				<button class="badge badge-outline gap-1" onclick={() => (filterBucketIdx = null)}>
					Bar #{filterBucketIdx + 1} ✕
				</button>
			{/if}
			<button
				class="link link-hover text-xs"
				onclick={() => {
					filterIP = null;
					filterBucketIdx = null;
				}}>
				Clear all
			</button>
		</div>
	{/if}
	<div class="mt-3">
		<ConntrackTable buckets={tableBuckets} {hosts} />
	</div>
{/if}

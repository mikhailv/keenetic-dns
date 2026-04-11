<script lang="ts">
	import { onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { ConntrackBucketsResponse } from '$lib/types';
	import RangeSelector from '$lib/components/conntrack/RangeSelector.svelte';
	import IntervalSelector from '$lib/components/conntrack/IntervalSelector.svelte';
	import TrafficChart from '$lib/components/conntrack/TrafficChart.svelte';
	import ConntrackTable from '$lib/components/conntrack/ConntrackTable.svelte';
	import { autoBumpInterval } from '$lib/components/conntrack/util';

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

	const rangeSeconds = $derived(Math.max(1, to - from));
	const effectiveInterval = $derived(autoBumpInterval(interval, rangeSeconds));
	const appliedHint = $derived(resp?.interval ?? null);

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
	<TrafficChart buckets={resp.buckets} />
	<div class="mt-3">
		<ConntrackTable buckets={resp.buckets} />
	</div>
{/if}

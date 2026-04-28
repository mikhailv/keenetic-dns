<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/services/api';
	import type { ConntrackBucketsResponse } from '$lib/types';
	import RangeSelector from '$lib/components/conntrack/RangeSelector.svelte';
	import IntervalSelector from '$lib/components/conntrack/IntervalSelector.svelte';
	import TrafficChart from '$lib/components/conntrack/TrafficChart.svelte';
	import ConntrackTable from '$lib/components/conntrack/ConntrackTable.svelte';
	import { autoBumpInterval } from '$lib/components/conntrack/util';
	import { createHostStore, currentURL, queryParams } from '$lib/stores';
	import { toTimestamp } from '$lib/util';
	import { SvelteURLSearchParams } from 'svelte/reactivity';

	const AUTO_REFRESH_INTERVAL = 10_000;

	// Read initial state from URL query params
	function initFromURL() {
		const params = queryParams();
		const now = toTimestamp(Date.now());

		let from: number;
		let to: number;
		let span: number | null = null;
		if (params.has('from') && params.has('to')) {
			from = parseInt(params.get('from') || '') || now - 3600;
			to = parseInt(params.get('to') || '') || now;
		} else {
			span = parseInt(params.get('span') || '') || 3600;
			from = now - span;
			to = now;
		}

		return {
			from,
			to,
			span,
			autoRefresh: params.get('auto-refresh') === '1',
			interval: parseInt(params.get('interval') || '') || 60,
			filterIP: params.get('ip') || null,
			filterBucketIdx: params.has('bar') ? parseInt(params.get('bar')!) : null
		};
	}

	const init = initFromURL();
	let from = $state(init.from);
	let to = $state(init.to);
	let span = $state<number | null>(init.span);
	let autoRefresh = $state(init.autoRefresh);
	let interval = $state(init.interval);

	let resp = $state<ConntrackBucketsResponse | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(false);

	let filterIP = $state<string | null>(init.filterIP);
	let filterBucketIdx = $state<number | null>(init.filterBucketIdx);

	// Sync state to URL query params
	$effect(() => {
		const params = new SvelteURLSearchParams();
		if (span) {
			params.set('span', String(span));
		} else {
			params.set('from', String(from));
			params.set('to', String(to));
		}
		params.set('interval', String(interval));
		if (autoRefresh) params.set('auto-refresh', '1');
		if (filterIP) params.set('ip', filterIP);
		if (filterBucketIdx !== null) params.set('bar', String(filterBucketIdx));

		const newUrl = `${currentURL().pathname}${params.size ? `?${params}` : ''}`;
		tick().then(() => replaceState(newUrl, {}));
	});

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

	async function load() {
		loading = true;
		const res = await api.getConntrackBuckets(from, to, effectiveInterval);
		resp = res.data ?? null;
		error = res.error ?? null;
		loading = false;
	}

	function updateAnchor() {
		const span = to - from;
		to = toTimestamp(Date.now());
		from = to - span;
	}

	let timer: ReturnType<typeof setInterval> | null = null;
	$effect(() => {
		if (timer) clearInterval(timer);
		if (autoRefresh && span) {
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
	<RangeSelector bind:from bind:to bind:span bind:autoRefresh />
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
		onfilterip={(ip) => (filterIP = filterIP === ip ? null : ip)}
		onselectbar={(idx) => (filterBucketIdx = filterBucketIdx === idx ? null : idx)} />
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

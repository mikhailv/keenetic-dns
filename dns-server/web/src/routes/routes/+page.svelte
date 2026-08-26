<script lang="ts">
	import { onMount } from 'svelte';
	import { createRouteStore } from '$lib/stores';
	import { formatDecimal, formatNumber, toTimestamp } from '$lib/util';

	const ROUTES_RELOAD_INTERVAL = 5000;

	const routes = createRouteStore();
	let loading = $state(false);

	onMount(() => {
		reload();
		const interval = setInterval(reload, ROUTES_RELOAD_INTERVAL);
		return () => clearInterval(interval);
	});

	async function reload() {
		loading = true;
		await routes.reload();
		loading = false;
	}

	function formatDuration(expires: Date): string {
		const seconds = toTimestamp(expires.getTime() - Date.now());
		const duration = durationString(Math.abs(seconds));
		if (seconds >= 0) {
			return duration;
		}
		return `${duration} ago`;
	}

	function durationString(seconds: number) {
		if (seconds < 60) {
			return `${seconds} sec`;
		}
		return `${Math.floor(seconds / 60)} min`;
	}
</script>

<svelte:head>
	<title>Routes - Keenetic DNS</title>
</svelte:head>

<div class="flex items-center gap-3 mb-3">
	<button class="btn btn-sm btn-outline btn-primary btn-refresh" onclick={reload} disabled={loading}>
		{#if loading}
			<span class="loading loading-spinner loading-sm" aria-hidden="true"></span>
		{/if}
		Refresh
	</button>
	<input
		class="input input-sm me-auto focus:outline-none"
		type="text"
		placeholder="Filter..."
		aria-label="Filter"
		bind:value={routes.filter} />
</div>

{#if $routes.error}
	<div class="alert alert-error" role="alert">{$routes.error}</div>
{:else if loading && $routes.total === 0}
	<p class="text-base-content/50 pt-1">Loading data...</p>
{:else}
	<table class="table caption-top table-md">
		<caption class="text-right pb-0">
			Routes: {formatNumber($routes.total)} | IPs: {formatNumber($routes.totalIPs)} ({formatDecimal(
				($routes.totalIPs * 100) / Math.pow(2, 32),
				4
			)}%)
		</caption>
		<thead>
			<tr>
				<th style="width: 1%">#</th>
				<th style="width: 10%">Address</th>
				<th style="width: 10%">Interface</th>
				<th>DNS Records</th>
				<th style="width: 15%" class="hidden lg:table-cell">TTL</th>
				<th style="width: 20%" class="hidden lg:table-cell">Info</th>
			</tr>
		</thead>
		<tbody class="border-t border-base-300">
			{#each $routes.items as route, i (route.addr + route.iface)}
				<tr class="hover">
					<td class="text-base-content/50">{i + 1}</td>
					<td>{route.addr}</td>
					<td>{route.iface}</td>
					<td class="font-light text-sm1">
						{#each route.dns_records ?? [] as rec (rec.domain)}
							<div>{rec.domain}</div>
						{/each}
					</td>
					<td class="hidden lg:table-cell text-sm1">
						{#each route.dns_records ?? [] as rec (rec.domain)}
							<div>
								{#if rec.expires > new Date()}
									{formatDuration(rec.expires)}
								{:else}
									<span class="font-light text-base-content/50">
										expired {formatDuration(rec.expires)}
									</span>
								{/if}
							</div>
						{/each}
					</td>
					<td class="hidden lg:table-cell font-light text-base-content/75 text-sm1">
						<div>added {formatDuration(route.added_at)}</div>
						<div>reason: <strong>{route.reason}</strong></div>
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}

<style>
	.btn-refresh {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 100px;
		justify-content: center;
	}

	.btn-refresh .loading {
		flex-shrink: 0;
	}
</style>

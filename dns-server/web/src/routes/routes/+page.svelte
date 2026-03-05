<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { IPRoute } from '$lib/types';

	let routes = $state<IPRoute[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let filter = $state('');
	let refreshInterval: ReturnType<typeof setInterval>;

	onMount(() => {
		loadRoutes();
		refreshInterval = setInterval(loadRoutes, 5000);
	});

	onDestroy(() => {
		clearInterval(refreshInterval);
	});

	async function loadRoutes() {
		loading = true;
		error = null;
		try {
			routes = await api.getRoutes();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load routes';
		} finally {
			loading = false;
		}
	}

	function filteredRoutes(): IPRoute[] {
		const f = filter.trim().toLowerCase();
		if (!f) return routes;

		return routes.filter(
			(route) =>
				route.addr.toLowerCase().includes(f) ||
				route.iface.toLowerCase().includes(f) ||
				route.dns_records?.some(
					(rec) => rec.domain.toLowerCase().includes(f) || rec.ip.toLowerCase().includes(f)
				)
		);
	}

	function formatExpires(expires: Date): string {
		const seconds = Math.floor((expires.getTime() - Date.now()) / 1000);
		const duration = durationString(Math.abs(seconds));
		if (seconds >= 0) {
			return duration;
		}
		return `expired ${duration} ago`;
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

<h1>Routes</h1>

<div class="hstack gap-3 mb-3">
	<button class="btn btn-outline-primary btn-refresh" onclick={loadRoutes} disabled={loading}>
		{#if loading}
			<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>
		{/if}
		Refresh
	</button>
	<input
		class="form-control me-auto"
		type="text"
		placeholder="Filter..."
		aria-label="Filter"
		bind:value={filter}
	/>
</div>

{#if error}
	<div class="alert alert-danger" role="alert">{error}</div>
{:else if loading && routes.length === 0}
	<p class="text-body-secondary pt-1">Loading data...</p>
{:else}
	<table class="table table-sm table-hover caption-top">
		<caption class="text-end pb-0">Routes: {filteredRoutes().length}</caption>
		<thead>
			<tr>
				<th scope="col" style="width: 1%">#</th>
				<th scope="col" style="width: 15%">Address</th>
				<th scope="col" style="width: 15%">Interface</th>
				<th scope="col" class="ps-2">DNS Records</th>
			</tr>
		</thead>
		<tbody class="table-group-divider">
			{#each filteredRoutes() as route, i (route.addr + route.iface)}
				<tr>
					<th scope="row">{i + 1}</th>
					<td>{route.addr}</td>
					<td style="font-size: 0.9rem">{route.iface}</td>
					<td class="ps-2">
						{#each route.dns_records ?? [] as rec (rec.domain)}
							<div class="row" style="font-size: 0.9rem">
								<div class="col fw-light">{rec.domain}</div>
								<div class="col d-none d-lg-block">
									{#if rec.expires > new Date()}
										{formatExpires(rec.expires)}
									{:else}
										<span class="fw-light text-body-secondary">
											{formatExpires(rec.expires)}
										</span>
									{/if}
								</div>
							</div>
						{/each}
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

	.btn-refresh .spinner-border {
		flex-shrink: 0;
	}
</style>

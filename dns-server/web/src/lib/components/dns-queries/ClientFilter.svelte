<script lang="ts">
	import type { HostInfo } from '$lib/types';

	let { hosts, selected = $bindable<string>() }: { hosts: HostInfo[]; selected: string } = $props();

	let search = $state('');

	const UNASSIGNED_IP = '0.0.0.0';

	const clients = $derived(collectClients(hosts));

	function collectClients(hosts: HostInfo[]): { ip: string; name: string }[] {
		const byIP: Record<string, string> = {};
		for (const host of hosts) {
			if (!host.ip || host.ip === UNASSIGNED_IP) {
				continue;
			}
			byIP[host.ip] ||= host.name || host.hostname || '';
		}
		return Object.entries(byIP)
			.map(([ip, name]) => ({ ip, name }))
			.sort((a, b) => (a.name || a.ip).localeCompare(b.name || b.ip));
	}

	const visible = $derived(
		clients.filter((it) => {
			const text = search.trim().toLowerCase();
			return text === '' || it.ip.includes(text) || it.name.toLowerCase().includes(text);
		})
	);

	const label = $derived.by(() => {
		if (!selected) {
			return 'All clients';
		}
		const client = clients.find((it) => it.ip === selected);
		return client?.name || selected;
	});

	function select(ip: string) {
		selected = ip;
		search = '';
		(document.activeElement as HTMLElement | null)?.blur();
	}
</script>

<div class="dropdown">
	<div tabindex="0" role="button" class="btn btn-sm" aria-haspopup="true">
		{label}
		<span class="text-xs">▾</span>
	</div>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<ul
		tabindex="0"
		class="dropdown-content menu bg-base-200 rounded-box z-10 w-64 p-2 shadow-sm flex-nowrap max-h-96 overflow-y-auto">
		<li class="menu-title p-1">
			<input
				class="input input-sm w-full focus:outline-none"
				type="text"
				placeholder="Find client..."
				aria-label="Find client"
				bind:value={search} />
		</li>
		<li>
			<button type="button" class:menu-active={!selected} onclick={() => select('')}>All clients</button>
		</li>
		{#each visible as client (client.ip)}
			<li>
				<button type="button" class:menu-active={selected === client.ip} onclick={() => select(client.ip)}>
					<span class="flex flex-col items-start">
						{#if client.name}
							<span>{client.name}</span>
							<span class="font-light text-sm2">{client.ip}</span>
						{:else}
							<span>{client.ip}</span>
						{/if}
					</span>
				</button>
			</li>
		{/each}
		{#if visible.length === 0}
			<li class="p-2 text-base-content/50">No clients found</li>
		{/if}
	</ul>
</div>

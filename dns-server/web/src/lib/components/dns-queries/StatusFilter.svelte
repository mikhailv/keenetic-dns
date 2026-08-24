<script lang="ts">
	import { QUERY_STATUSES, type QueryStatus } from './util';

	let { selected = $bindable<QueryStatus[]>() }: { selected: QueryStatus[] } = $props();

	const label = $derived(
		selected.length === 0
			? 'All queries'
			: QUERY_STATUSES.filter((it) => selected.includes(it.value))
					.map((it) => it.label)
					.join(', ')
	);

	function toggle(status: QueryStatus) {
		selected = selected.includes(status) ? selected.filter((it) => it !== status) : [...selected, status];
	}
</script>

<div class="dropdown">
	<div tabindex="0" role="button" class="btn btn-sm" aria-haspopup="true">
		{label}
		<span class="text-xs">▾</span>
	</div>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<ul tabindex="0" class="dropdown-content menu bg-base-200 rounded-box z-10 w-52 p-2 shadow-sm">
		{#each QUERY_STATUSES as status (status.value)}
			<li>
				<label class="label cursor-pointer justify-start gap-2">
					<input
						type="checkbox"
						class="checkbox checkbox-sm"
						checked={selected.includes(status.value)}
						onchange={() => toggle(status.value)} />
					<span class="label-text">{status.label}</span>
				</label>
			</li>
		{/each}
		{#if selected.length > 0}
			<li>
				<button type="button" class="btn btn-ghost btn-xs mt-1" onclick={() => (selected = [])}> Clear </button>
			</li>
		{/if}
	</ul>
</div>

<script lang="ts">
	import { RANGE_OPTIONS } from './util';
	import { fromLocalInput, fromTimestamp, toLocalInput, toTimestamp } from '$lib/util';
	import { onMount } from 'svelte';

	let {
		from = $bindable<number>(),
		to = $bindable<number>(),
		span = $bindable<number | null>(),
		autoRefresh = $bindable()
	}: {
		from: number;
		to: number;
		span: number | null;
		autoRefresh: boolean;
	} = $props();

	let customFrom = $state<string>('');
	let customTo = $state<string>('');
	let mounted = $state<boolean>(false);

	onMount(() => (mounted = true));

	updateCustom();

	$effect(() => {
		if (span) {
			const now = toTimestamp(Date.now());
			from = now - span;
			to = now;
			updateCustom();
		}
	});

	function updateCustom() {
		customFrom = toLocalInput(fromTimestamp(from));
		customTo = toLocalInput(fromTimestamp(to));
	}

	function applyCustom() {
		from = toTimestamp(fromLocalInput(customFrom));
		to = toTimestamp(fromLocalInput(customTo));
	}
</script>

<div class="flex flex-wrap items-center gap-2">
	<div class="join" role="group" aria-label="Range">
		{#each RANGE_OPTIONS as opt (opt.value)}
			<button
				type="button"
				class="btn btn-sm join-item"
				class:btn-active={span && span === opt.value}
				onclick={() => (span = opt.value)}>
				{opt.label}
			</button>
		{/each}
		<button type="button" class="btn btn-sm join-item" class:btn-active={!span} onclick={() => (span = null)}>
			Custom…
		</button>
	</div>

	{#if mounted && !span}
		<input type="datetime-local" class="input input-sm w-auto focus:outline-none" bind:value={customFrom} />
		<input type="datetime-local" class="input input-sm w-auto focus:outline-none" bind:value={customTo} />
		<button type="button" class="btn btn-sm btn-primary" onclick={applyCustom}>Apply</button>
	{/if}

	<label class="flex items-center gap-2 ms-2 mb-0 cursor-pointer">
		<input type="checkbox" class="toggle toggle-sm" bind:checked={autoRefresh} disabled={!span} />
		<span class="text-sm2">Auto-refresh</span>
	</label>
</div>

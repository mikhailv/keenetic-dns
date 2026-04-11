<script lang="ts">
	import { INTERVAL_OPTIONS, autoBumpInterval, intervalLabel } from './util';

	let {
		interval = $bindable(60),
		rangeSeconds,
		appliedHint
	}: {
		interval: number;
		rangeSeconds: number;
		appliedHint: number | null;
	} = $props();

	const effective = $derived(autoBumpInterval(interval, rangeSeconds));
	const display = $derived(appliedHint ?? effective);
	const showHint = $derived(display !== interval);
</script>

<div class="flex items-center gap-2">
	<select class="select select-bordered select-sm w-auto" bind:value={interval}>
		{#each INTERVAL_OPTIONS as opt (opt.value)}
			<option value={opt.value}>{opt.label}</option>
		{/each}
	</select>
	{#if showHint}
		<small class="text-base-content/60">auto-adjusted to {intervalLabel(display)}</small>
	{/if}
</div>

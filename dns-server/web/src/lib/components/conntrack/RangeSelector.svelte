<script lang="ts">
	import { RANGE_OPTIONS } from './util';

	type Mode = 'preset' | 'custom';

	let {
		from = $bindable(),
		to = $bindable(),
		anchored = $bindable(true),
		autoRefresh = $bindable(false)
	}: {
		from: number;
		to: number;
		anchored: boolean;
		autoRefresh: boolean;
	} = $props();

	let mode: Mode = $state('preset');
	let presetSeconds = $state(3600);
	let customFrom = $state(toLocalInput(from));
	let customTo = $state(toLocalInput(to));

	function toLocalInput(unix: number): string {
		const d = new Date(unix * 1000);
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	function fromLocalInput(s: string): number {
		return Math.floor(new Date(s).getTime() / 1000);
	}

	function applyPreset(seconds: number) {
		presetSeconds = seconds;
		const now = Math.floor(Date.now() / 1000);
		from = now - seconds;
		to = now;
		anchored = true;
	}

	function applyCustom() {
		from = fromLocalInput(customFrom);
		to = fromLocalInput(customTo);
		anchored = false;
	}
</script>

<div class="flex flex-wrap items-center gap-2">
	<div class="join" role="group" aria-label="Range">
		{#each RANGE_OPTIONS as opt (opt.value)}
			<button
				type="button"
				class="btn btn-sm join-item"
				class:btn-active={mode === 'preset' && presetSeconds === opt.value}
				onclick={() => {
					mode = 'preset';
					applyPreset(opt.value);
				}}>
				{opt.label}
			</button>
		{/each}
		<button
			type="button"
			class="btn btn-sm join-item"
			class:btn-active={mode === 'custom'}
			onclick={() => (mode = 'custom')}>
			Custom…
		</button>
	</div>

	{#if mode === 'custom'}
		<input type="datetime-local" class="input input-sm w-auto focus:outline-none" bind:value={customFrom} />
		<input type="datetime-local" class="input input-sm w-auto focus:outline-none" bind:value={customTo} />
		<button type="button" class="btn btn-sm btn-primary" onclick={applyCustom}>Apply</button>
	{/if}

	<label class="flex items-center gap-2 ms-2 mb-0 cursor-pointer">
		<input type="checkbox" class="toggle toggle-sm" bind:checked={autoRefresh} disabled={!anchored} />
		<span class="text-sm2">Auto-refresh</span>
	</label>
</div>

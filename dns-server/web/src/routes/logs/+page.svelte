<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/services/api';
	import type { LogEntry } from '$lib/types';
	import type { StreamStore } from '$lib/stores/stream.store';

	const MAX_ITEMS = 1000;

	const stream: StreamStore<LogEntry> = api.createLogStreamStore(MAX_ITEMS);

	onMount(() => stream.start());

	function formatTime(date: Date): string {
		return date.toTimeString().split(' ')[0];
	}

	function getLevelClass(level: string): string {
		switch (level.toLowerCase()) {
			case 'error':
				return 'text-error font-semibold';
			case 'warn':
			case 'warning':
				return 'text-warning font-semibold';
			case 'info':
				return 'text-info font-semibold';
			case 'debug':
				return 'text-base-content/60';
			default:
				return '';
		}
	}

	function formatAttrs(attrs?: Record<string, string>): string {
		if (!attrs) {
			return '';
		}
		return Object.entries(attrs)
			.map(([key, val]) => `${key}= ${val}`)
			.join('\n');
	}
</script>

<svelte:head>
	<title>Logs - Keenetic DNS</title>
</svelte:head>

{#if $stream.error}
	<div class="alert alert-error" role="alert">{$stream.error}</div>
{/if}

<div class="overflow-x-auto">
	<table class="table">
		<thead>
			<tr>
				<th>Time</th>
				<th>Level</th>
				<th>Message</th>
				<th>Attributes</th>
			</tr>
		</thead>
		<tbody class="border-t border-base-300">
			{#each $stream.items as entry (entry.cursor)}
				<tr class="hover animate-new-row">
					<td title={entry.time.toLocaleString()} class="font-light text-sm1">{formatTime(entry.time)}</td>
					<td class="{getLevelClass(entry.level)} text-sm2">{entry.level}</td>
					<td class="text-sm1">{entry.msg}</td>
					<td class="font-light text-sm2" style="white-space: pre-wrap">{formatAttrs(entry.attrs)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

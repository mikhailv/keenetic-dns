<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/services/api';
	import type { LogEntry } from '$lib/types';
	import type { StreamStore } from '$lib/stores/stream.store';

	const MAX_ITEMS = 200;

	let stream: StreamStore<LogEntry> = api.createLogStreamStore(MAX_ITEMS);

	onMount(() => {
		stream.start();
	});

	onDestroy(() => {
		stream.stop();
	});

	function formatTime(date: Date): string {
		return date.toTimeString().split(' ')[0];
	}

	function getLevelClass(level: string): string {
		switch (level.toLowerCase()) {
			case 'error':
				return 'text-danger fw-semibold';
			case 'warn':
			case 'warning':
				return 'text-warning fw-semibold';
			case 'info':
				return 'text-info fw-semibold';
			case 'debug':
				return 'text-muted';
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

<h1>Logs</h1>

{#if $stream.error}
	<div class="alert alert-danger" role="alert">{$stream.error}</div>
{/if}

<div class="table-responsive">
	<table class="table table-sm table-hover table-sticky-header">
		<thead>
			<tr>
				<th scope="col">Time</th>
				<th scope="col">Level</th>
				<th scope="col">Message</th>
				<th scope="col">Attributes</th>
			</tr>
		</thead>
		<tbody class="table-group-divider">
			{#each $stream.items as entry (entry.cursor)}
				<tr class="animate-new-row">
					<td title={entry.time.toLocaleString()} class="fw-light text-sm1"
						>{formatTime(entry.time)}</td
					>
					<td class="{getLevelClass(entry.level)} text-sm2">{entry.level}</td>
					<td class="text-sm1">{entry.msg}</td>
					<td class="fw-light text-sm1" style="white-space: pre-wrap">{formatAttrs(entry.attrs)}</td
					>
				</tr>
			{/each}
		</tbody>
	</table>
</div>

<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { theme, type Theme } from '$lib/stores/theme.store';

	let { children } = $props();

	const themes: { value: Theme; label: string }[] = [
		{ value: 'system', label: 'Auto' },
		{ value: 'light', label: 'Light' },
		{ value: 'dark', label: 'Dark' }
	];
</script>

<nav class="d-flex align-items-center mb-4">
	<div class="nav nav-pills me-auto">
		<a class="nav-link" class:active={page.url.pathname === '/routes'} href="/routes">Routes</a>
		<a class="nav-link" class:active={page.url.pathname === '/dns-queries'} href="/dns-queries">DNS Queries</a>
		<a class="nav-link" class:active={page.url.pathname === '/logs'} href="/logs">Logs</a>
		<a class="nav-link" class:active={page.url.pathname === '/conntrack'} href="/conntrack">Conntrack</a>
	</div>
	<div class="btn-group btn-group-sm" role="group" aria-label="Theme">
		{#each themes as t (t.value)}
			<button
				type="button"
				class="btn btn-outline-secondary"
				class:active={$theme === t.value}
				onclick={() => ($theme = t.value)}>
				{t.label}
			</button>
		{/each}
	</div>
</nav>

{@render children()}

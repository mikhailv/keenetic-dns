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

<nav class="flex items-center mb-2">
	<div class="tabs tabs-lift me-auto">
		<a class="tab" class:tab-active={page.url.pathname === '/routes'} href="/routes">Routes</a>
		<a class="tab" class:tab-active={page.url.pathname === '/dns-queries'} href="/dns-queries">DNS Queries</a>
		<a class="tab" class:tab-active={page.url.pathname === '/logs'} href="/logs">Logs</a>
		<a class="tab" class:tab-active={page.url.pathname === '/conntrack'} href="/conntrack">Conntrack</a>
	</div>
	<div class="join" role="group" aria-label="Theme">
		{#each themes as t (t.value)}
			<button
				type="button"
				class="btn btn-xs join-item"
				class:btn-active={$theme === t.value}
				onclick={() => ($theme = t.value)}>
				{t.label}
			</button>
		{/each}
	</div>
</nav>

{@render children()}

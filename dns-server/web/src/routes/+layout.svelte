<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';

	let { children } = $props();
</script>

<nav class="nav nav-pills mb-4">
	<a class="nav-link" class:active={page.url.pathname === '/routes'} href="/routes"> Routes </a>
	<a class="nav-link" class:active={page.url.pathname === '/dns-queries'} href="/dns-queries">
		DNS Queries
	</a>
	<a class="nav-link" class:active={page.url.pathname === '/logs'} href="/logs"> Logs </a>
</nav>

{@render children()}

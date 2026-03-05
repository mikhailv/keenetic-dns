<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	onMount(() => {
		goto('/routes', { replaceState: true });
	});
</script>

<div class="d-flex justify-content-center align-items-center" style="height: 50vh">
	<div class="spinner-border text-primary" role="status">
		<span class="visually-hidden">Loading...</span>
	</div>
</div>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	onMount(() => {
		goto('/routes', { replaceState: true });
	});
</script>

<div class="flex justify-center items-center" style="height: 50vh">
	<span class="loading loading-spinner text-primary"></span>
</div>

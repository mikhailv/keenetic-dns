import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		appDir: 'static',
		adapter: adapter({
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		output: {
			bundleStrategy: 'single'
		}
	}
};

export default config;

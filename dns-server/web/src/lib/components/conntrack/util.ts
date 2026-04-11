export const INTERVAL_OPTIONS = [
	{ value: 60, label: '1m' },
	{ value: 120, label: '2m' },
	{ value: 180, label: '3m' },
	{ value: 300, label: '5m' },
	{ value: 600, label: '10m' },
	{ value: 900, label: '15m' },
	{ value: 1200, label: '20m' },
	{ value: 1800, label: '30m' },
	{ value: 3600, label: '1h' },
	{ value: 7200, label: '2h' },
	{ value: 10800, label: '3h' },
	{ value: 21600, label: '6h' },
	{ value: 43200, label: '12h' },
	{ value: 86400, label: '1d' }
];

export const RANGE_OPTIONS = [
	{ value: 300, label: '5m' },
	{ value: 600, label: '10m' },
	{ value: 1800, label: '30m' },
	{ value: 3600, label: '1h' },
	{ value: 7200, label: '2h' },
	{ value: 10800, label: '3h' },
	{ value: 21600, label: '6h' },
	{ value: 43200, label: '12h' },
	{ value: 86400, label: '1d' },
	{ value: 259200, label: '3d' },
	{ value: 604800, label: '7d' }
];

export const MAX_BUCKETS = 180;

/**
 * Returns the smallest INTERVAL_OPTIONS value ≥ requested that satisfies
 * (rangeSeconds / interval) ≤ MAX_BUCKETS. Falls back to the largest option.
 */
export function autoBumpInterval(requested: number, rangeSeconds: number): number {
	if (rangeSeconds / requested <= MAX_BUCKETS) return requested;
	for (const opt of INTERVAL_OPTIONS) {
		if (opt.value < requested) continue;
		if (rangeSeconds / opt.value <= MAX_BUCKETS) return opt.value;
	}
	return INTERVAL_OPTIONS[INTERVAL_OPTIONS.length - 1].value;
}

export function intervalLabel(seconds: number): string {
	const found = INTERVAL_OPTIONS.find((o) => o.value === seconds);
	if (found) return found.label;
	if (seconds % 86400 === 0) return `${seconds / 86400}d`;
	if (seconds % 3600 === 0) return `${seconds / 3600}h`;
	if (seconds % 60 === 0) return `${seconds / 60}m`;
	return `${seconds}s`;
}

export function formatBytes(n: number): string {
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let i = 0;
	let v = n;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

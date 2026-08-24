export type QueryStatus = 'direct' | 'routed' | 'excluded' | 'blocked' | 'reused';

export const QUERY_STATUSES: { value: QueryStatus; label: string }[] = [
	{ value: 'direct', label: 'Direct' },
	{ value: 'routed', label: 'Routed' },
	{ value: 'excluded', label: 'Excluded' },
	{ value: 'blocked', label: 'Blocked' },
	{ value: 'reused', label: 'Reused' }
];

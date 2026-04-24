export function defined<T>(v: T | undefined): v is T {
	return typeof v !== 'undefined';
}

export function toTimestamp(d: Date | number): number {
	if (d instanceof Date) {
		d = d.getTime();
	}
	return Math.floor(d / 1000);
}

export function fromTimestamp(v: number): Date {
	return new Date(v * 1000);
}

export function toLocalInput(d: Date): string {
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocalInput(s: string): Date {
	return new Date(s);
}

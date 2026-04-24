import { defined } from '$lib/util/typings';
import { fromTimestamp } from '$lib/util/date';

// TODO: make it configurable?
const locale = 'ru-RU';

const numFormat = new Intl.NumberFormat(locale, {
	minimumFractionDigits: 0,
	maximumFractionDigits: 2
});

const dateFormat = new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: undefined });
const dateTimeFormat = new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'medium' });
const timeFormat = new Intl.DateTimeFormat(locale, { dateStyle: undefined, timeStyle: 'medium' });

export function formatNumber(n: number): string {
	return numFormat.format(n);
}

export function formatDecimal(
	n: number,
	maxFractionDigits: number = 2,
	minFractionDigits?: number
): string {
	if (maxFractionDigits === 2 && !defined(minFractionDigits)) {
		return numFormat.format(n);
	}
	return new Intl.NumberFormat(locale, {
		minimumFractionDigits: minFractionDigits,
		maximumFractionDigits: maxFractionDigits
	}).format(n);
}

export function formatDate(d: Date | number): string {
	return dateFormat.format(toDate(d));
}

export function formatDateTime(d: Date | number): string {
	return dateTimeFormat.format(toDate(d));
}

export function formatTime(d: Date | number): string {
	return timeFormat.format(toDate(d));
}

function toDate(d: Date | number): Date {
	return d instanceof Date ? d : fromTimestamp(d);
}

import { fromStore, type Updater, type Writable } from 'svelte/store';

export type Mutator<T> = (value: T) => unknown;

export function mutator<T>(update: (fn: Updater<T>) => void): (fn: Mutator<T>) => void {
	return (fn: Mutator<T>) => {
		update((value) => {
			fn(value);
			return value;
		});
	};
}

class Builder<T> {
	store: T;

	constructor(value: T) {
		this.store = value;
	}

	property<N extends string, U>(name: N, store: Writable<U>): Builder<T & { [K in N]: U }> {
		Object.defineProperty(this.store, name, {
			enumerable: true,
			get: () => fromStore(store).current,
			set: (v) => store.set(v)
		});
		return this as Builder<T & { [K in N]: U }>;
	}
}

export function storeBuilder<T>(value: T): Builder<T> {
	return new Builder<T>(value);
}

import { notEqual } from '@lit/reactive-element';

export type Result<T> = { value: T, error?: undefined } | { value?: undefined, error: Error | string };

export function shallowArrayEquals<T extends ReadonlyArray<unknown>>(oldVal: T, newVal: T): boolean {
  return oldVal === newVal || (oldVal.length === newVal.length && oldVal.every((v, i) => !notEqual(v, newVal[i])));
}

export type Opt<T> = T | null | undefined;

export function defined<T>(val: Opt<T>): val is T {
  return val !== null && val !== undefined;
}

import { Result } from './utils';

export const ErrCancelledStream = 'cancelled';

export interface Stream<T> {
  next(): Promise<T>;
  cancel(): void;
}

export function listenStream<T>(stream: Stream<T>, listener: (res: Result<T> | 'cancelled') => void): void {
  setTimeout(async() => {
    for (;;) {
      try {
        listener({ value: await stream.next() });
      } catch (e) {
        if (e === ErrCancelledStream) {
          listener(ErrCancelledStream);
          break;
        }
        listener({ error: e as (Error | string) });
      }
    }
  })
}

export function tickerStream<T>(interval: number, provider: () => Promise<T>): Stream<T> {
  return newStream(async (ctx) => {
    if (!ctx.initial) {
      await new Promise(resolve => setTimeout(resolve, interval));
    }
    ctx.notCancelled();
    return provider();
  });
}

export function websocketStream<T>(provider: () => WebSocket, mapper?: (data: string) => T): Stream<T> {
  mapper ??= data => JSON.parse(data) as T;
  let ws: WebSocket | null;
  connect();
  const [streamPush, streamProvider] = pushStreamProvider<T>();
  return newStream(streamProvider, () => {
    ws?.close();
    ws = null;
  });

  function connect() {
    ws = provider();
    ws.addEventListener('message', msg => streamPush(mapper!(msg.data)));
    ws.addEventListener('close', (e) => {
      console.log(e);
      if (ws) {
        connect();
      }
    });
  }
}

export interface ProviderContext {
  get initial(): boolean;
  get cancelled(): boolean;
  notCancelled(): void;
}

export type StreamProvider<T> = (ctx: ProviderContext) => Promise<T>;

export function newStream<T>(provider: StreamProvider<T>, oncancel?: () => void): Stream<T> {
  let calls = 0;
  let cancelled = false;
  let promise: Promise<T>;
  const rejectCallbacks = new Set<(reason: unknown) => void>();

  const providerCtx: ProviderContext = {
    get initial(): boolean { return calls === 0 },
    get cancelled(): boolean { return cancelled },
    notCancelled(): void {
      if (cancelled) {
        throw ErrCancelledStream;
      }
    }
  };

  scheduleNext();

  function scheduleNext() {
    if (!cancelled) {
      promise = provider(providerCtx);
      calls++;
      promise.then(scheduleNext, scheduleNext).catch(() => {});
    }
  }

  return {
    next(): Promise<T> {
      if (cancelled) {
        return promise;
      }
      return new Promise<T>((resolve, reject) => {
        rejectCallbacks.add(reject);
        promise.then(resolve, reject).catch(() => {}).then(() => rejectCallbacks.delete(reject));
      });
    },
    cancel() {
      if (!cancelled) {
        cancelled = true;
        promise = Promise.reject(ErrCancelledStream);
        rejectCallbacks.forEach(f => f(ErrCancelledStream))
        rejectCallbacks.clear();
        oncancel?.()
      }
    }
  }
}

export function pushStreamProvider<T>(): [(value: T) => void, StreamProvider<T>] {
  let resolveCallback: (value: T) => void;

  function push(value: T) {
    resolveCallback?.(value);
  }

  return [push, (): Promise<T> => {
    return new Promise(resolve => {
      resolveCallback = resolve;
    });
  }];
}

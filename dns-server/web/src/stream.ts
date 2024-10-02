import { Result } from './utils';

export const ErrCancelledStream = 'cancelled';

export interface Stream<T> {
  next(): Promise<T>;
  cancel(): void;
}

export type StreamSource<T> = () => Promise<T>;

export interface StreamSink<T> {
  push(value: T): void;
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
  return newStream({
    async next(ctx) {
      if (!ctx.initial) {
        await new Promise(resolve => setTimeout(resolve, interval));
      }
      ctx.notCancelled();
      return provider();
    }
  });
}

export function websocketStream<T>(provider: () => WebSocket, mapper?: (data: string) => T): Stream<T> {
  mapper ??= data => JSON.parse(data) as T;
  let ws: WebSocket | null;
  const pushProvider = newPushStreamDataProvider<T>();
  connect();
  return newStream(pushProvider, () => {
    // stream close
    ws?.close();
    ws = null;
  });

  function connect() {
    ws = provider();
    ws.addEventListener('message', msg => pushProvider.push(mapper!(msg.data)));
    ws.addEventListener('close', () => {
      if (ws) {
        // reconnect
        connect();
      }
    });
  }
}

export function mapStream<T, R>(stream: Stream<T>, mapper: (value: Result<T>, sink: (value: R) => void) => void): Stream<R> {
  const provider = newPushStreamDataProvider<R>();
  const result = newStream<R>(provider);
  listenStream(stream, res => {
    if (res === 'cancelled') {
      result.cancel();
    } else {
      mapper(res, provider.push);
    }
  });
  return result;
}

export interface ProviderContext {
  get initial(): boolean;
  get cancelled(): boolean;
  notCancelled(): void;
}

export interface StreamDataProvider<T> {
  next(ctx: ProviderContext): Promise<T>;
}

export function newStream<T>(provider: StreamDataProvider<T>, oncancel?: () => void): Stream<T> {
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
      promise = provider.next(providerCtx);
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

export interface PushStreamDataProvider<T> extends StreamDataProvider<T> {
  push(value: T): void;
}

export function newPushStreamDataProvider<T>(): PushStreamDataProvider<T> {
  let resolveCallback: (value: T) => void;
  return {
    next(): Promise<T> {
      return new Promise(resolve => resolveCallback = resolve);
    },
    push(value: T) {
      resolveCallback?.(value);
    },
  };
}

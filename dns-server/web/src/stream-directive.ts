import { noChange } from 'lit';
import { AsyncDirective, directive } from 'lit/async-directive.js';
import { Result, shallowArrayEquals } from './utils';
import { Opt } from './typings';
import { listenStream, Stream } from './stream';

export type StreamRenderer<T> = {
  initial?: () => unknown,
  render?: (value: T) => unknown,
  error?: (error: unknown) => unknown,
}

class StreamDirective<T, A extends ReadonlyArray<unknown>> extends AsyncDirective {
  private _stream?: Stream<T>;
  private _renderer: StreamRenderer<T> = {};
  private _prevArgs?: A;
  private _result?: Result<T>;
  private _renderedResult?: unknown;

  render(stream: Opt<Stream<T>>, renderer: StreamRenderer<T>, args: A) {
    if (this._stream === stream) {
      if (shallowArrayEquals(this._prevArgs ?? [], args)) {
        return this._renderedResult;
      }
      this._prevArgs = args;
      return this._renderResult();
    }
    this._stream?.cancel();
    this._stream = stream ?? undefined;
    this._renderer = renderer;
    this._prevArgs = args;
    if (this.isConnected) {
      this._listenStream();
    }
    return renderer.initial ? renderer.initial() : noChange;
  }

  private _renderResult() {
    if (!this._result) {
      return undefined;
    }
    const { value, error } = this._result;
    if (value) {
      this._renderedResult = this._renderer.render ? this._renderer.render(value) : value;
    } else {
      this._renderedResult = this._renderer.error ? this._renderer.error(error) : error;
    }
    return this._renderedResult;
  }

  private _listenStream() {
    if (this._stream) {
      listenStream(this._stream, res => {
        if (res !== 'cancelled') {
          this._result = res;
          this.setValue(this._renderResult());
        }
      });
    }
  }

  protected override disconnected() {
    super.disconnected();
    this._stream?.cancel();
  }

  protected override reconnected() {
    super.reconnected();
    this._listenStream();
  }
}

export const stream = directive(StreamDirective) as (<T>(stream: Opt<Stream<T>>, renderer: StreamRenderer<T>, watch: unknown) => unknown);

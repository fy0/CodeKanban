import { EventEmitter } from 'node:events';

import { CodeKanbanConfigError, CodeKanbanError, CodeKanbanValidationError } from './errors.js';
import {
  buildWebSessionHeartbeatFrame,
  decodeWebSessionSocketMessage,
  isWebSessionHeartbeatFrame,
  normalizeWebSessionFrame,
  shouldEmitWebSessionFrame,
} from './web-session-shared.js';
import { ensureOptionalString, ensureString } from './utils.js';

const SOCKET_OPEN = 1;
const DEFAULT_OPEN_TIMEOUT_MS = 8000;

function nowEventBase() {
  const timestampMs = Date.now();
  return {
    timestampMs,
    timestamp: new Date(timestampMs).toISOString(),
  };
}

export class WebSessionEventStream {
  constructor({ url, sessionId, WebSocketImpl, webSocketOptions } = {}) {
    const resolvedUrl = ensureString(url, 'url');
    const Socket = WebSocketImpl || globalThis.WebSocket;
    if (!Socket) {
      throw new CodeKanbanConfigError('WebSocket implementation is unavailable');
    }

    this.url = resolvedUrl;
    this.sessionId = ensureOptionalString(sessionId) || null;
    this.socket = webSocketOptions
      ? new Socket(resolvedUrl, webSocketOptions)
      : new Socket(resolvedUrl);
    this._emitter = new EventEmitter();
    this._queue = [];
    this._nextResolvers = [];
    this._closed = false;
    this._openPromise = new Promise((resolve, reject) => {
      this._resolveOpen = resolve;
      this._rejectOpen = reject;
    });

    this.socket.addEventListener('open', () => {
      if (this.sessionId && this.socket.readyState === SOCKET_OPEN) {
        this.socket.send(JSON.stringify(buildWebSessionHeartbeatFrame('focus', this.sessionId)));
      }
      const payload = {
        type: 'open',
        url: this.url,
        sessionId: this.sessionId,
        ...nowEventBase(),
      };
      this._resolveOpen?.();
      this._publish(payload);
    });

    this.socket.addEventListener('message', event => {
      this._handleMessage(event.data);
    });

    this.socket.addEventListener('error', event => {
      const error = new CodeKanbanError('web session event websocket error', { event });
      const payload = {
        type: 'error',
        errorType: 'socket',
        message: error.message,
        error,
        raw: event,
        sessionId: this.sessionId,
        ...nowEventBase(),
      };
      this._rejectOpen?.(error);
      this._publish(payload);
    });

    this.socket.addEventListener('close', event => {
      this._closed = true;
      const payload = {
        type: 'close',
        url: this.url,
        sessionId: this.sessionId,
        code: typeof event?.code === 'number' ? event.code : null,
        reason: typeof event?.reason === 'string' ? event.reason : '',
        wasClean: event?.wasClean === true,
        ...nowEventBase(),
      };
      this._rejectOpen?.(new CodeKanbanError('web session event websocket closed', { event }));
      this._publish(payload);
      this._flushIteratorDone();
    });
  }

  on(type, handler) {
    this._emitter.on(type, handler);
    return () => this.off(type, handler);
  }

  off(type, handler) {
    this._emitter.off(type, handler);
  }

  async waitForOpen(timeoutMs = DEFAULT_OPEN_TIMEOUT_MS) {
    if (this.socket.readyState === SOCKET_OPEN) {
      return;
    }
    await Promise.race([
      this._openPromise,
      new Promise((_, reject) => {
        setTimeout(
          () => reject(new CodeKanbanValidationError(`web session event stream did not open within ${timeoutMs}ms`)),
          timeoutMs,
        );
      }),
    ]);
  }

  async waitFor(predicate, options = {}) {
    const timeoutMs = Number.isFinite(options.timeoutMs) ? Math.max(1, Math.trunc(options.timeoutMs)) : 15000;

    return await new Promise((resolve, reject) => {
      const cleanup = this.on('__event__', payload => {
        try {
          if (!predicate(payload)) {
            return;
          }
          cleanup();
          clearTimeout(timer);
          resolve(payload);
        } catch (error) {
          cleanup();
          clearTimeout(timer);
          reject(error);
        }
      });

      const timer = setTimeout(() => {
        cleanup();
        reject(new CodeKanbanValidationError(`web session event stream waitFor timed out after ${timeoutMs}ms`));
      }, timeoutMs);
    });
  }

  close() {
    this._closed = true;
    this.socket.close();
  }

  [Symbol.asyncIterator]() {
    return {
      next: () => {
        if (this._queue.length > 0) {
          return Promise.resolve({ value: this._queue.shift(), done: false });
        }
        if (this._closed) {
          return Promise.resolve({ value: undefined, done: true });
        }
        return new Promise(resolve => {
          this._nextResolvers.push(resolve);
        });
      },
    };
  }

  _handleMessage(data) {
    let rawFrame;
    let frame;
    try {
      rawFrame = decodeWebSessionSocketMessage(data);
      if (isWebSessionHeartbeatFrame(rawFrame)) {
        if (rawFrame?.op === 'ping' && this.socket.readyState === SOCKET_OPEN) {
          this.socket.send(JSON.stringify(buildWebSessionHeartbeatFrame('pong')));
        }
        return;
      }
      frame = normalizeWebSessionFrame(rawFrame);
    } catch (error) {
      const payload = {
        type: 'error',
        errorType: 'decode',
        message: error instanceof Error ? error.message : String(error),
        error,
        raw: data,
        sessionId: this.sessionId,
        ...nowEventBase(),
      };
      this._publish(payload);
      return;
    }

    if (!shouldEmitWebSessionFrame(frame, this.sessionId)) {
      return;
    }

    this._publish(frame.type === 'error' ? { ...frame, errorType: 'frame' } : frame, true);
  }

  _publish(payload, emitFrame = false) {
    if (emitFrame) {
      this._emitter.emit('frame', payload);
    }
    this._emitter.emit(payload.type, payload);
    this._emitter.emit('__event__', payload);

    if (this._nextResolvers.length > 0) {
      const resolve = this._nextResolvers.shift();
      resolve?.({ value: payload, done: false });
      return;
    }
    this._queue.push(payload);
  }

  _flushIteratorDone() {
    while (this._nextResolvers.length > 0) {
      const resolve = this._nextResolvers.shift();
      resolve?.({ value: undefined, done: true });
    }
  }
}

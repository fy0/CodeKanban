export const WEB_SESSION_WIRE_VERSION = 1 as const;

export type WebSessionWireFrameKind = 'ack' | 'evt' | 'err' | 'hb';
export type WebSessionWireHeartbeatOperation = 'ping' | 'pong' | 'focus';

export type WebSessionWireFrame<
  TSession = unknown,
  THistoryItem = unknown,
  TSubAgent = unknown,
  TPendingInput = unknown,
  TScheduledInput = unknown,
> = {
  v: typeof WEB_SESSION_WIRE_VERSION;
  k: WebSessionWireFrameKind;
  rid?: string;
  sid?: string;
  ts: number;
  rev?: string;
  op?: string;
  p?: unknown;
  ok?: number;
  s?: TSession;
  h?: {
    its: THistoryItem[];
    hm: boolean;
    bc?: string;
    tot: number;
  };
  i?: THistoryItem;
  ag?: TSubAgent;
  pi?: TPendingInput[];
  si?: TScheduledInput[];
  code?: string;
  msg?: string;
  retry?: boolean;
};

export function buildWebSessionCommandFrame(
  requestId: string,
  operation: string,
  sessionId: string,
  payload: Record<string, unknown>
) {
  return {
    v: WEB_SESSION_WIRE_VERSION,
    k: 'cmd' as const,
    rid: requestId,
    sid: sessionId || undefined,
    op: operation,
    p: payload,
  };
}

export function buildWebSessionHeartbeatFrame(
  operation: WebSessionWireHeartbeatOperation,
  sessionId = ''
) {
  return {
    v: WEB_SESSION_WIRE_VERSION,
    k: 'hb' as const,
    ts: Date.now(),
    op: operation,
    sid: sessionId || undefined,
  };
}

export function parseWebSessionWireFrame<
  TSession = unknown,
  THistoryItem = unknown,
  TSubAgent = unknown,
  TPendingInput = unknown,
  TScheduledInput = unknown,
>(
  raw: unknown
): WebSessionWireFrame<TSession, THistoryItem, TSubAgent, TPendingInput, TScheduledInput> {
  const value = typeof raw === 'string' ? JSON.parse(raw) : raw;
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('web session websocket frame must be an object');
  }
  const frame = value as Record<string, unknown>;
  if (frame.v !== WEB_SESSION_WIRE_VERSION) {
    throw new Error(`unsupported web session websocket protocol version: ${String(frame.v)}`);
  }
  if (!['ack', 'evt', 'err', 'hb'].includes(String(frame.k ?? ''))) {
    throw new Error(`unsupported web session websocket frame kind: ${String(frame.k)}`);
  }
  if (typeof frame.ts !== 'number' || !Number.isFinite(frame.ts)) {
    throw new Error('web session websocket frame is missing a valid timestamp');
  }
  return frame as WebSessionWireFrame<
    TSession,
    THistoryItem,
    TSubAgent,
    TPendingInput,
    TScheduledInput
  >;
}

export class WebSessionCommandError extends Error {
  readonly code: string;
  readonly operation: string;
  readonly sessionId: string;

  constructor({
    code,
    message,
    operation,
    sessionId,
  }: {
    code: string;
    message: string;
    operation: string;
    sessionId: string;
  }) {
    super(message);
    this.name = 'WebSessionCommandError';
    this.code = code;
    this.operation = operation;
    this.sessionId = sessionId;
  }
}

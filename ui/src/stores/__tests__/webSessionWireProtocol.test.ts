import { describe, expect, it } from 'vitest';

import {
  buildWebSessionCommandFrame,
  buildWebSessionHeartbeatFrame,
  parseWebSessionWireFrame,
  WEB_SESSION_WIRE_VERSION,
} from '@/stores/webSessionWireProtocol';

describe('webSession wire protocol', () => {
  it('uses one versioned envelope for commands and heartbeats', () => {
    expect(buildWebSessionCommandFrame('request-1', 'send', 'session-1', { txt: 'hello' })).toEqual(
      {
        v: WEB_SESSION_WIRE_VERSION,
        k: 'cmd',
        rid: 'request-1',
        sid: 'session-1',
        op: 'send',
        p: { txt: 'hello' },
      }
    );
    expect(buildWebSessionHeartbeatFrame('focus', 'session-1')).toMatchObject({
      v: WEB_SESSION_WIRE_VERSION,
      k: 'hb',
      sid: 'session-1',
      op: 'focus',
    });
  });

  it('rejects unversioned and snapshot websocket frames', () => {
    expect(() => parseWebSessionWireFrame({ k: 'evt', ts: 1 })).toThrow(
      'unsupported web session websocket protocol version'
    );
    expect(() =>
      parseWebSessionWireFrame({ v: WEB_SESSION_WIRE_VERSION, k: 'snap', ts: 1 })
    ).toThrow('unsupported web session websocket frame kind');
  });

  it('accepts the lightweight resync event contract', () => {
    expect(
      parseWebSessionWireFrame(
        JSON.stringify({
          v: WEB_SESSION_WIRE_VERSION,
          k: 'evt',
          sid: 'session-1',
          rev: '15',
          ts: 1,
          op: 'resync_required',
          p: { reason: 'history_reconciled' },
        })
      )
    ).toMatchObject({
      k: 'evt',
      sid: 'session-1',
      rev: '15',
      op: 'resync_required',
    });
  });
});

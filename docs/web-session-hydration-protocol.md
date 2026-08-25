# WebSession Hydration Protocol

WebSession uses one authoritative full-state transport and two incremental
WebSocket channels. A logical mutation must never return the same full history
through more than one channel.

## Transport ownership

| Transport                     | Responsibility                                 | Full history allowed |
| ----------------------------- | ---------------------------------------------- | -------------------- |
| `GET .../snapshot`            | Conditional full hydration                     | Yes                  |
| `POST /web-sessions/reconcile` | Conditional summary recovery                   | No                   |
| Other HTTP endpoints          | Mutation result and hydration target           | No                   |
| Command WebSocket             | Command ack or command-specific compact result | No                   |
| Event WebSocket               | Session summary and incremental state          | No                   |

The conditional snapshot response is either:

- a complete snapshot at `revision`; or
- `{ revision, unchanged: true }` when `knownRevision` is current.

Mutation HTTP responses that require hydration return a target containing
`session` and `revision`. The client then uses the conditional snapshot endpoint.

## Resume reconciliation

When a page becomes visible, receives window focus, is restored from page cache,
or reconnects its event stream, the browser conditionally reconciles session
summaries before hydrating the focused conversation. The request contains at
most 256 `{ id, revision }` targets: every locally non-terminal session, the
focused session, and up to 48 other sessions active within the last six hours.

The server reads only those IDs and returns:

- complete summaries whose revision changed; and
- `missingIds` tombstones for sessions deleted while the client was away.

Unchanged targets produce no response item. This keeps resume traffic bounded
when a user has hundreds of sessions and avoids loading any conversation history
until the focused session actually requires hydration.

## WebSocket envelope

All frames use protocol version `1` and the compact envelope:

```json
{
  "v": 1,
  "k": "ack | evt | err | hb",
  "sid": "session-id",
  "rev": "42",
  "ts": 1787664000000,
  "op": "operation",
  "p": {}
}
```

`snap` is not a valid frame kind. The server rejects unsupported protocol
versions, and the browser rejects unknown versions and frame kinds.

Command acknowledgements contain `rid`, `sid`, and the committed `rev` when the
session still exists. `goal_get` additionally returns the compact goal value in
`p.goal`, including `null` when no goal exists.

The event channel may send:

- `session`: compact session summary;
- `hist_item` and `hist_page`: incremental history data;
- `pending` and `scheduled`: transient input state;
- `sub_agent`: one sub-agent registry update;
- `resync_required`: a lightweight hydration notice.

A resync notice contains only the envelope and an optional reason:

```json
{
  "v": 1,
  "k": "evt",
  "sid": "session-id",
  "rev": "42",
  "ts": 1787664000000,
  "op": "resync_required",
  "p": { "reason": "history_reconciled" }
}
```

## Revision rules

`snapshot_revision` identifies recoverable session state.

1. A durable business mutation advances the revision in its existing database
   write or transaction.
2. Broadcasting a summary or resync notice reuses that committed revision and
   never performs another SQLite update.
3. Pending and scheduled state can live outside `web_sessions`. Publishing a
   changed transient snapshot advances the durable revision exactly once so a
   reconnect can detect that conditional hydration is required.
4. Repairing stale history counters does not advance the revision because it
   fixes metadata for already committed history.
5. Read-only refreshes, including `goal_get`, do not write or advance the
   revision when the fetched state is unchanged.

## Client revision clocks

The browser tracks three revisions per session:

- `observed`: highest revision seen on any server response;
- `applied`: highest revision incorporated through a snapshot or incremental event;
- `hydrated`: highest revision loaded from the full HTTP snapshot endpoint.

Only `hydrated` may be sent as `knownRevision`. Incremental events can advance
`observed` and `applied`, but they do not prove that the complete baseline is
current.

## Resync scheduling

For each session, the client hydration queue:

1. skips a notice already covered by `hydrated`;
2. coalesces duplicate notices into one in-flight conditional request;
3. retains only the highest requested revision;
4. runs one trailing request when a higher revision arrives during hydration;
5. does not automatically loop on a failed request, but allows a later notice
   for the same revision to retry.

This keeps recovery idempotent while preventing command ack, HTTP fallback, and
event reconciliation from starting parallel full hydrations.

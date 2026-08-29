# WebSession Hydration Protocol

WebSession uses one authoritative full-state transport, one cursor-based
catch-up transport, and two WebSocket channels. A logical mutation must never
return the same full history through more than one channel.

## Transport ownership

| Transport                      | Responsibility                                 | Full history allowed |
| ------------------------------ | ---------------------------------------------- | -------------------- |
| `GET .../snapshot`             | Conditional full hydration                     | Yes                  |
| `GET .../catch-up`             | Event-cursor delta for one focused session     | No                   |
| `POST /web-sessions/reconcile` | Conditional summary recovery                   | No                   |
| Other HTTP endpoints           | Mutation result and hydration target           | No                   |
| Command WebSocket              | Command ack or command-specific compact result | No                   |
| Event WebSocket                | Session summary and focused incremental state  | No                   |

The conditional snapshot response is either:

- a complete snapshot at `revision`; or
- `{ revision, historyEpoch, eventCursor, pendingEpoch, pendingVersion,
pendingInputs, unchanged: true }` when `knownRevision` is current.

Snapshot is a pure database read. It does not clear unread state, repair
counters, or contact the App Server. Source reconciliation remains an explicit
`sync` operation. This ensures that switching to a cached session cannot turn a
read into an expensive source reload.

Pending state is always included because it has an independent in-memory clock
and can change without advancing `revision`.

Mutation HTTP responses that require hydration return a target containing
`session` and `revision`. The client normally catches up from its hydrated event
cursor. An explicit `resync_required` notice always forces a full snapshot.

## Event cursor and history epoch

A full snapshot establishes `(historyEpoch, eventCursor)`. The cursor has the
form `eventSequence:orderIndex`; a snapshot baseline ends at
`eventSequence:9223372036854775807`.

`GET .../catch-up` receives the last hydrated cursor and epoch. Its first page
also fixes a target cursor, which subsequent pages reuse. This gives one stable
catch-up window while new events continue to arrive. History items carry their
last event sequence so an older HTTP response cannot overwrite a newer
WebSocket update for the same item.

Operations that replace, delete, compact, or reorder cached history increment
`historyEpoch`. A mismatched epoch returns `resetRequired: true`; the browser
keeps the cached timeline visible while replacing it from snapshot. Ordinary
append and in-place event projections retain the epoch and are recoverable by
cursor.

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
until the focused session actually requires hydration. Background sessions
receive summaries only. Detailed history, scheduled-input, and sub-agent frames
are sent only for the session named by the connection's focus heartbeat.

## WebSocket envelope

All frames use protocol version `1` and the compact envelope:

```json
{
  "v": 1,
  "k": "ack | evt | err | hb",
  "sid": "session-id",
  "rev": "42",
  "pe": "process-epoch",
  "pv": 7,
  "pi": [],
  "ts": 1787664000000,
  "op": "operation",
  "p": {}
}
```

`snap` is not a valid frame kind. The server rejects unsupported protocol
versions, and the browser rejects unknown versions and frame kinds.

Acknowledgement fields depend on the operation:

- durable mutations and a `send` that starts a new turn contain the committed
  `rev`;
- `pending_del`, `pending_update`, `pending_reorder`, `pending_clear`, and a
  queued `send` contain `pe`, `pv`, and the complete `pi` snapshot, with no
  durable `rev` read or write;
- read commands return their compact result in `p`. For example, `goal_get`
  returns `p.goal`, including `null` when no goal exists.
- `mark_read` carries the last observed attention revision in `p.ar`. Its ack
  returns the authoritative unread state and attention revision without changing
  the content revision.

An empty pending queue is encoded as `"pi": []`, not by omitting `pi`.

The event channel may send:

- `session`: compact session summary;
- `hist_item` and `hist_page`: incremental history data;
- `pending` and `scheduled`: transient input state;
- `sub_agent`: one sub-agent registry update;
- `resync_required`: a lightweight hydration notice.

After opening an event stream for one session, a client sends a
`{ "k": "hb", "op": "focus", "sid": "..." }` heartbeat. The server immediately
returns an authoritative `pending` event for that session, including an empty
queue. The browser repeats this heartbeat when focus changes.

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
3. Scheduled state remains recoverable through the durable revision. Pending
   state does not update or read `snapshot_revision`; it uses the independent
   pending clock described below.
4. Snapshot never repairs stale history counters. Repair belongs to an explicit
   maintenance or synchronization path.
5. Read-only refreshes, including `goal_get`, do not write or advance the
   revision when the fetched state is unchanged.

## Client revision clocks

The browser tracks three revisions per session:

- `observed`: highest revision seen on any server response;
- `applied`: highest revision incorporated through a snapshot or incremental event;
- `hydrated`: highest revision loaded from the full HTTP snapshot endpoint.

Only `hydrated` may be sent as `knownRevision`. Incremental events can advance
`observed` and `applied`, but they do not prove that the complete baseline is
current. Separately, the client stores the hydrated history epoch and event
cursor. Focused history frames advance that cursor only to the individual item
that was applied; a catch-up response advances it to the response page boundary.

## Attention clock

Unread state is independent from content hydration. Each background attention
event sets `hasUnread` and increments `attentionRevision`. After catch-up has
settled and the updated timeline has rendered, the browser sends `mark_read`
asynchronously with the revision it actually observed.

The database clears unread state only when both `hasUnread` and
`attentionRevision` still match. A newer background event therefore wins over a
stale mark-read request. Clearing unread advances the attention revision but not
`snapshot_revision`, so viewing a session never creates another content
hydration cycle.

## Pending clock

Pending state uses `(pendingEpoch, pendingVersion)` (`pe`, `pv`) independently
of the durable revision:

1. `pendingEpoch` is generated once per server process.
2. `pendingVersion` increases monotonically per session whenever its in-memory
   pending snapshot changes.
3. A different epoch replaces the client's pending baseline. Within one epoch,
   only a higher version is applied; duplicate or lower versions are ignored.
4. Every pending event and pending-only acknowledgement carries the complete
   `pi` snapshot. Conditional HTTP snapshots carry the same fields, including
   when the durable snapshot is unchanged.

This lets pending broadcast, focus recovery, and command acknowledgement avoid
SQLite revision contention while remaining deterministic for clients.

## Redirect undo window

The five-second redirect undo window belongs to the browser, not the server or
database. A new redirect is staged in per-tab `sessionStorage`; during the
window the browser does not open the command WebSocket or send the command.
Queue input is sent immediately.

Editing, resuming, or promoting an existing server item first leaves the
authoritative item paused, then starts the same local window. Switching sessions
does not stop its timer. Restoring a tab gives active staged items a fresh five
seconds. When dispatch fails, the item remains paused with `failed` state and is
not retried automatically. Stable pending IDs make a resend after a lost ACK
idempotent within the server process.

## Resync scheduling

For each session, the client hydration queue:

1. skips a notice already covered by `hydrated`;
2. coalesces duplicate notices into one in-flight hydration request;
3. retains only the highest requested revision;
4. runs one trailing request when a higher revision arrives during hydration;
5. does not automatically loop on a failed request, but allows a later notice
   for the same revision to retry.

Ordinary revision recovery uses event-cursor catch-up. A changed history epoch
or explicit resync reason uses snapshot. This keeps recovery idempotent while
preventing command ack, HTTP fallback, and event reconciliation from starting
parallel full hydrations.

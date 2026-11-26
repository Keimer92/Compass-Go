# Sync protocol (v1)

The HTTP contract between a client (Go `syncengine`, Dart client, or the
Worker's own tests) and a project's Cloudflare Worker. Every byte below is
captured from a real session against the worker (`wrangler dev --local`,
simulated D1/KV) and replayed by `TestReplayDelContratoWire` in School
Tracker — see `protocol/fixtures/wire-session.json`.

## Transport

- One endpoint per operation: `POST <workerUrl>/<path>` with body
  `application/json` (always a JSON object, even `{}`).
- Every request carries the header `X-School-Key: <key>`. The worker routes
  the key through the `SCHOOL_ROUTING` KV to the project's D1 database
  (`database_id` binding `DB`). One key = one namespace inside one D1.
- Responses are JSON objects with `ok: true`. Errors come either as HTTP
  4xx/5xx or as HTTP 200 with `ok: false`; in both cases the human-readable
  message is in `error`. A client must treat **both** forms as failure.
- Timestamps on rows are `YYYY-MM-DD HH:MM:SS` (SQLite style, UTC-ish local
  text); `/hello.serverTime` is ISO-8601 with milliseconds.

## Endpoints

### `POST /hello` — liveness + routing probe

```json
// request
{}
// response
{"ok":true,"databaseId":"primary","serverTime":"2026-11-01T05:46:28.031Z"}
```

`databaseId` is the KV routing target (which D1 the key resolved to).
Used by the app's "test connection" UI.

### `POST /stats` — remote row counts

```json
// request
{}
// response
{"ok":true,"total":0,"tombstones":0,"porTabla":{"ano_lectivo":0, ...}}
```

Counts per syncable table (names come from the project manifest), plus the
global `total` and the pending `tombstones`. Diagnostic only.

### `POST /ids` — reserve id blocks

```json
// request
{"deviceId":"dev-fixture-A","blocks":{"estudiante":10,"dia_sin_clases":5}}
// response
{"ok":true,"blocks":{"estudiante":{"start":21,"end":30},"dia_sin_clases":{"start":11,"end":15}}}
```

Offline-safe id pre-allocation: the client asks for `n` ids per table and the
server returns an inclusive range from a per-D1 counter, so ids never collide
between devices even when created while offline. The worker itself never
assigns ids to synced rows — the client's id travels in the payload — which
is why blocks must be reserved before writing.

### `POST /push` — send local changes

```json
// request
{
  "since": "1970-01-01 00:00:00",
  "deviceId": "dev-fixture-A",
  "data": {
    "asistencia": [
      {"id":101,"estudiante_id":1,"fecha":"2026-10-20","estado":"P",
       "observacion":null,"updated_at":"2026-10-20 08:00:00",
       "estudiante_codigo":"COD-A"}
    ],
    "estudiante": [ ... ], "nota": [ ... ]
  },
  "deletions": [
    {"tabla":"estudiante","pk":["COD-B"]},
    {"tabla":"asistencia","pk":["COD-A","2026-10-21"]}
  ]
}
// response
{"ok":true,"applied":{"asistencia":3,"estudiante":2, ...},"deletionsApplied":2}
```

- `data` maps table → array of row objects with the **stored columns** from
  the manifest (`wire_cols`) **plus** computed protocol columns
  (e.g. `estudiante_codigo`), which the client resolves locally via a parent
  join before sending. The worker does not store computed columns.
- Only rows with `updated_at > since` may be sent; `since` is the client's
  own push cursor (see "Cursor model").
- `deletions[].pk` is the **canonical PK tuple in manifest `pk` order**. PK
  components may be business codes (`"COD-B"`) instead of local ids — the
  manifest's `pk` says which. Values keep their JSON type (string/number).
- The worker upserts rows `ON CONFLICT(canonical pk)` into the project's D1,
  sealing `updated_at` to the server clock, and records each deletion as a
  tombstone row `(tabla, pk, updated_at, device_id)`.

### `POST /pull` — receive everything newer

```json
// request
{"deviceId":"dev-fixture-A","since":"1970-01-01 00:00:00"}
// response
{
  "ok": true,
  "rows": {
    "asistencia": [
      {"id":101,"estudiante_id":1,"fecha":"2026-10-20","estado":"J",
       "observacion":null,"updated_at":"2026-11-01 05:46:28",
       "estudiante_codigo":"COD-A"}
    ], ...
  },
  "deleted": [
    {"tabla":"estudiante","pk":["COD-B"],
     "updated_at":"2026-11-01 05:46:28","device_id":"dev-fixture-A"}
  ],
  "serverTime": "2026-11-01 05:46:28"
}
```

- `rows[table]` holds stored columns **plus** computed columns, resolved by
  the worker on the fly (`LEFT JOIN` on the parent table). This is where the
  cloud's canonical `estudiante_id` gets re-attached to `estudiante_codigo`
  so each device rewrites it to its own local id.
- Rows whose `device_id` equals the requester's `deviceId` are **excluded**
  (the device just pushed them): first half of the anti-echo mechanism.
- `serverTime` is the new pull cursor; a client must persist it only on
  success.

### `POST /reset` — empty the namespace

```json
// request
{}
// response
{"ok":true,"borradas":32}
```

Deletes every row of every syncable table and all tombstones for the routed
database. Used by "wipe remote" flows. `borradas` counts tombstones removed.

### `POST /usage` — quota report (not in fixtures)

Requests `{}`; the response reports rows written today vs the free-tier
quota. It requires the Cloudflare Analytics API binding, which does not
exist under `wrangler dev --local`, so the deterministic capture session
deliberately omits it. Treat as optional/diagnostic.

### Non-JSON routes (optional modules, not protocol)

`/ws` (WebSocket change-hub Durable Object), `/conectar` (key-provisioning
web page + deep-link redirect), `/release.json` + updater artifacts (KV
`RELEASES`). A client only needs the six endpoints above.

## Cursor model (client requirement)

Two independent clocks — this is the engine's core invariant:

- **Pull cursor** — server time. Stored per config; starts at
  `"1970-01-01 00:00:00"`; replaced by `serverTime` after a *successful*
  pull.
- **Push cursor** — local clock (`last_push_sync`), advanced only after a
  *real, successful* push. A failed push never advances it. Recovery paths
  that legitimately null it: `wipe_remote`, `reset_remote`,
  `restore_from_cloud`, or an explicit `full=true` push.

A row is pushed when `updated_at > push cursor`. Because the worker seals
`updated_at`, a pulled-then-untouched row is not newer locally, which (plus
the `device_id` exclusion above and the `sync_recent_pulls` anti-echo table
with TTL 86 400 s) is what stops push↔pull echo loops.

## Retry brake (client requirement)

Consecutive failed pushes arm an exponential backoff of 5 → 240 minutes
(`push_fail_count`, `push_retry_after` in the client's config table). While
armed, sync still **pulls** but does not push; `force=true` bypasses it only
for explicit user buttons. Rationale: the D1 free tier quota (≈100k
rows/day, a full push ≈8k rows).

## Canonical identity rules

- A table's cloud identity is its manifest `pk` (ordered list). Tombstones
  and the anti-echo key use it verbatim.
- PK components may be **computed columns** (business codes) when local ids
  diverge per device. `CloudPK` = `pk` with computed columns replaced by
  their stored `fk` — used when tombstoning remote rows during a wipe.
- Anti-echo key: `table + "|" + pkPart + "|" + updated_at`, where `pkPart`
  is each PK value JSON-encoded (`"0035"` for strings, `35` for numbers,
  `null` for nulls) joined with `|` **and a trailing `|`**, e.g.
  `asistencia|"0035"|"2026-10-21"||2026-10-21 08:00:00`.

## Reference session

`protocol/fixtures/wire-session.json` — one deterministic multi-device
session (reset → hello → stats → ids → push/pull ×3 devices → restore →
wipe → reset) captured with a pinned clock and pinned device ids. Use it to
validate a new client implementation byte-for-byte (the Go engine's replay
test fails on any drift in request bytes).

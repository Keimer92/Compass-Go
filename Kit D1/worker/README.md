# Worker template

The kit's Cloudflare Worker: a generic D1 sync server driven entirely by the
project's manifest (`sync.schema.json`), generated from the Go registry.
Never hand-edit `worker/instances/<project>/src/**` — it is a copy of
`template/src` and `scripts/build-instance.mjs` overwrites it.

## Layout

```
template/src/
  index.ts      router: mounts modules by which bindings exist
  engine.ts     /hello /push /pull /ids /stats /reset (manifest-driven SQL)
  hub.ts        optional: /ws realtime broadcast (Durable Object)
  releases.ts   optional: /release.json + /release/<file> + /updates/latest
  auth.ts       optional: Google + email auth, recovery codes, admin, /nube/*
  provision.ts  optional: /ping and /conectar deep-link redirect
  types.ts      manifest types + KitConfig + Env
instances/<project>/
  sync.schema.json   the contract (go run ./example/.../cmd/gen-manifest -o …)
  sync.config.json   namespace, deepLinkScheme, auth branding (optional)
  instance.ts        legacy migrations for pre-kit D1s (optional)
  wrangler.toml      project bindings (own worker name, own D1, own KVs)
  package.json       pinned wrangler/workers-types
  sync-project.json  ops config for provision/generate-key/admin: keyPrefix,
                     routingKv, kvNamespaces, schemaFiles, tenantsFile,
                     keysFile, workerUrl
  schema.sql         the D1 DDL provision.mjs applies
  tenants.json       tenant list (slug -> database id in the routing KV)
  migrations/        one-off D1 migrations already baked into schema.sql (archive)
  sync-keys.json     issued keys — a credential store, gitignored
```

## Module mounting (by binding presence)

| Module     | Requires binding            | Provides |
|------------|-----------------------------|----------|
| engine     | `DB` + `SCHOOL_ROUTING`     | sync endpoints, `/usage` only with USERS |
| provision  | (any)                       | `/ping`, `/conectar` when `deepLinkScheme` is set |
| releases   | `RELEASES` KV               | release feed + binary download + `/updates/latest` |
| auth       | `USERS` KV                  | `/auth/*`, `/nube/*`, `/admin/*` |
| hub        | `SYNC_HUB` DO               | `/ws` broadcast; pushes notify viewers |

A project that only needs sync declares `DB` + `SCHOOL_ROUTING` and gets a
minimal worker; everything else is opt-in by wrangler.toml.

## Update feed (`/updates/latest`)

With a `RELEASES` KV the worker exposes a generic updater endpoint:

- `GET /updates/latest?version=<client>&platform=<os>` reads the KV key
  `updates/latest` and returns the manifest as `200`, or `204` when the
  published `version` is not newer than the client's. `404` means nothing has
  been published yet (the app treats it as a failed check, not "up to date").
- `platform` selects `manifest.platforms.<platform>` when present, so one
  manifest can describe several targets.
- Artifacts live under `file:<name>` in the same namespace and download from
  `/release/<name>`.

Manifest shape:

```json
{
  "version": "1.2.0",
  "notes": "Release notes shown in the app dialog.",
  "url": "https://<worker>/release/GymPro-1.2.0-Setup.exe",
  "sha256": "<sha256 of the artifact>",
  "mandatory": false
}
```

Publish with `node scripts/publish-update.mjs --dir worker/instances/<p>
--manifest update.json --file <artifact>`: it uploads the artifact, fills
`url`/`sha256` when missing, and writes `updates/latest`.

## How the manifest drives the engine

Each table in `sync.schema.json` carries the derived behavior (see
`docs/protocol.md` and `go/syncengine/manifest.go`):

- `conflict` — the `ON CONFLICT (...)` target (business key when there is one).
- `set_exclude_id` — the UPDATE never re-writes the stored `id` (also forced
  for `server_assigns_id`: the INSERT omits the id, so `excluded.id` is NULL
  and keeping it in SET would hand the row a brand-new rowid on every push).
- `server_assigns_id` — INSERT drops the id; the business key is the identity.
- `computed[].canonize` — the push re-writes the FK to the cloud's canonical
  parent id via a `COALESCE((SELECT …), …)` lookup on the business code.
- Tombstone deletes resolve computed components through the parent subquery,
  with a legacy numeric-pk fallback.

`ensureSchema` adds the protocol columns (`sk`, `device_id`), the `idx_<t>_sk`
index and, for `set_exclude_id` tables, the `idx_<t>_conflict` UNIQUE index
the `ON CONFLICT` target needs — so a project's D1 can be a plain app schema.

## New project checklist

1. Write the Go registry for the project; `go run …/cmd/gen-manifest -o
   worker/instances/<p>/sync.schema.json`.
2. Copy an existing instance dir for `wrangler.toml`/`package.json` shapes;
   use your own worker name, D1 and KV ids (each app owns its deployment).
3. Add `sync.config.json` (namespace, deep link scheme, auth branding).
4. `node scripts/build-instance.mjs <p>` then `npx tsc --noEmit` inside it.
5. `node scripts/provision.mjs --dir worker/instances/<p>` (reads that
   instance's `sync-project.json`) to create D1/KV and register the routing
   key; `npx wrangler dev --local` to test.
6. Issue connection files with `node scripts/generate-key.mjs --dir …` and run
   operator commands with `node scripts/admin.mjs --dir …` (needs
   `SYNC_ADMIN_ID_TOKEN`, a Google ID token of an active admin). Nothing about
   a project's cloud side should live in the app's own repo — `worker/instances/school-tracker`
   is the reference layout.

## Parity testing an existing project's worker

`scripts/parity-check.mjs` diffs a fresh capture against the golden wire
fixture (the recorded contract of the project's original worker):

```
FIXTURE_WORKER_URL=http://127.0.0.1:<port> go test ./internal/wiretest -run Capturar
node scripts/parity-check.mjs <golden.json> <new.json> worker/instances/<p>/sync.schema.json
```

It compares paths/statuses strictly, requests byte-first (array-order-only
differences accepted), and responses as timestamp-normalized,
order-insensitive JSON; `server_assigns_id` row ids and `/ids` counter values
are normalized because they depend on the local DB's history, not the
contract. The school-tracker instance passed all 14 exchanges of
`protocol/fixtures/wire-session.json` (evidence in
`instances/school-tracker/parity/`).

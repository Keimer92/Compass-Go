# sync-kit

Reusable Cloudflare D1 sync stack extracted from School Tracker: a
registry-driven Go engine, a byte-exact wire-contract fixture set, protocol
specs, and per-project provisioning scripts.

Model: **each project runs its own Worker deployment and its own D1
database**, built from the same tested code with only its schema manifest
and `wrangler.toml` names differing. A `SCHOOL_ROUTING`-style KV maps sync
keys → databases *within* a project (multi-tenant); projects never share a
worker.

## Layout

```
go/syncengine/            generic engine (module github.com/Keimer92/kit-d1/go/syncengine)
  client.go               HTTP POST + X-Sync-Key-style header transport
  registry.go             TableSpec / Registry / strategies / validation
  collect.go              push payload builder (driven by the Registry)
  sync.go                 SyncAll, two-cursor model, retry brake, anti-echo,
                          reset / ids / hello / stats, restore, wipe
  example/school/         School Tracker's 22-table manifest + its DDL
                          (reference for a new project's manifest)
protocol/
  schema.spec.md          manifest format + minimum local schema contract
  fixtures/wire-session.json   deterministic capture of the whole protocol
docs/
  protocol.md             HTTP contract (endpoints, payloads, cursors)
scripts/
  provision.mjs           D1 + KV + first key, parameterized by sync-project.json
  generate-key.mjs        issue/rotate .key connection files
  build-instance.mjs      assemble worker/instances/<p>/src from the template
  parity-check.mjs        diff a fresh capture against the golden fixture
  admin.mjs               operator CLI for a deployed worker's /admin/*
  publish-update.mjs      publish an app update manifest + artifact to RELEASES
worker/
  template/src/           manifest-driven Worker (engine + optional modules)
  instances/school-tracker/  first instance (parity-verified + live 2026-11-01)
    sync.schema.json        contract, generated from the Go registry
    sync.config.json        project glue (namespace, deep-link scheme, auth)
    sync-project.json       ops config: keyPrefix, KV bindings, schemaFiles,
                            tenantsFile, keysFile, workerUrl
    schema.sql              D1 DDL applied by provision.mjs
    tenants.json            tenant list (slug -> routing KV)
    migrations/             historical one-off D1 migrations (archive)
    sync-keys.json          OUTPUT, a credential store — gitignored, never commit
    parity/                 parity captures against protocol/fixtures
  instances/school-tracker-flutter/  isolated Worker + D1 for the Flutter app
  README.md               module mounting, new-project checklist, parity flow
```

## Using it from a Go app

1. Copy `go/syncengine/example/school/` as a starting point and declare your
   project's tables (see `protocol/schema.spec.md` for the strategies and the
   build-time validation rules).
2. Depend on the published module (no local paths):
   ```
   require github.com/Keimer92/kit-d1/go/syncengine v0.1.0
   ```
   To edit the kit live without publishing, add a `go.work` (not committed)
   with `replace github.com/Keimer92/kit-d1/go/syncengine => "<kit>/go/syncengine"`.
3. Inject the registry into the engine calls (`syncengine` takes a
   `*Registry` everywhere it used to switch on table names) and keep only
   your UI/glue around `d1_config`.

Requirements on your local SQLite: every synced table has `updated_at` and a
deletion tombstone path; the engine manages `d1_config`,
`sync_recent_pulls`, `sync_pending_deletions` (+ dedicated per-table ones)
and `d1_id_blocks`.

## Provisioning a project's cloud side

On Windows, run `cloudflare.bat` from the kit directory for the School Tracker
worker menu (assemble, typecheck, local dev, guarded deploy, provisioning
preview, account listing and key export).

```bash
cd <project>/cloudflare          # worker dir with wrangler.toml + schema
node <kit>/scripts/provision.mjs --dry-run
node <kit>/scripts/provision.mjs --deploy
node <kit>/scripts/generate-key.mjs --dir .
node <kit>/scripts/admin.mjs --dir . list   # needs SYNC_ADMIN_ID_TOKEN
```

Config lives in `sync-project.json` (key prefix, KV bindings, schema files,
tenant list, `workerUrl`). Never commit `sync-keys.json` or `*.key` — they are
credentials. `worker/instances/school-tracker/` is the worked example: it is
provisioned, keyed, deployed and parity-checked entirely from this kit, with
nothing left in the app's repo (School Tracker keeps only a legacy copy of the
pre-kit worker for rollback).

## Testing a client or worker against the contract

`protocol/fixtures/wire-session.json` records a full multi-device session
(reset → hello → stats → ids → push/pull on three devices → restore → wipe →
reset) with a pinned clock against the real worker under `wrangler dev
--local`. A conforming implementation must produce identical request bytes
and accept the recorded responses. School Tracker replays this in
`internal/wiretest` (`TestReplayDelContratoWire`).

## Version control

`origin` is `https://github.com/Keimer92/Kit-D1` (local folder: `Kit D1`).
Consumers require the module by version (`v0.1.0` and up) and pin the tag
`go/syncengine/vX.Y.Z`; the only local `replace` left is the optional
`go.work` used while editing the kit.

`.gitignore` keeps credentials out: `*.key`, `sync-keys.json`,
`school-keys.json`, `.dev.vars`, `.wrangler/`, `*.db`/`*.sqlite`. Note that
`worker/instances/<project>/wrangler.toml` holds deployment *identifiers*
(account id, KV/D1 ids, Google client id, admin emails) — not secrets (the API
token lives in `wrangler secret`), which is why the repo is private.

## Status

- Engine + School Tracker manifest + tests: working, consumed by School
  Tracker via `replace` (branch `refactor/sync-kit`).
- Worker template: extracted, parity-verified and DEPLOYED (2026-11-01) in
  place of the original School Tracker worker — same worker name, so URL,
  secrets and bindings carried over. The school-tracker instance replays all
  14 exchanges of the golden wire session (`scripts/parity-check.mjs`). See
  `worker/README.md`.
- Flutter's separate `school-tracker-flutter` Worker/D1: deployed and health-
  checked; the app's local connection file is generated and gitignored. The
  Flutter UI still needs to import that file and call the existing Go D1
  configuration/restore commands.
- Dart/Flutter client: consumes the shared Go engine; UI integration remains
  in progress.

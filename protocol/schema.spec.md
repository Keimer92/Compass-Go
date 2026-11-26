# Schema manifest spec (v1)

Each project declares its syncable schema in one manifest. The Go engine
consumes it as `[]syncengine.TableSpec` (code is a valid place to declare
it — see `go/syncengine/example/school/schema.go`); the worker template
(Phase 2) will consume it as JSON. Both must describe the **same** contract,
and the local SQLite schema must satisfy the minimum contract below.

## Minimum local schema contract

Every syncable table must:

1. exist in the client's local database with the same name as in the cloud
   D1 (and in the worker's `TABLES`);
2. have an `updated_at TEXT` column maintained on every write — it is the
   only ordering the protocol knows;
3. be deletable with a tombstone recordable **per canonical PK**: deletes
   write `(tabla, pk)` into a pending-deletions table in the same
   transaction as the `DELETE`;
4. have the local infrastructure tables (the engine creates/uses them):
   `d1_config`, `sync_recent_pulls`, `sync_pending_deletions` (+ dedicated
   per-table ones), `d1_id_blocks`.

## TableSpec fields

| Field | Type | Meaning |
|---|---|---|
| `name` | ident | Table name, shared by local SQLite and cloud D1. |
| `pk` | ident[] | **Canonical (cloud) identity**, in tombstone order. Components may be computed columns. |
| `strategy` | enum | Apply path for pulled rows (default `replace`). |
| `business_key` | ident[] | Columns reconciled instead of the id (required by `catalog`). |
| `computed` | ComputedFK[] | Protocol-only columns resolved through parent joins. |
| `upsert` | Upsert | Conflict target/config for `key-upsert`. |
| `tombstone_table` | ident | Dedicated pending-deletions table (else generic `sync_pending_deletions`). |
| `orphan` | OrphanPurge | Local-only purge of rows whose parent is gone (never tombstoned). |
| `restored` | bool | Wiped by an authoritative restore-from-cloud. |
| `wire_cols` | ident[] | Columns the worker stores for this table — mirrors the worker's registry, validated at build time. |

### ComputedFK

```
column     wire column carrying the business value (e.g. "estudiante_codigo")
parent     table owning the business key
parent_key parent's key column (default "id")
fk         this table's column referencing parent (rewritten on apply)
target     parent's business-key column (stable across devices)
code_col   column of the dedicated tombstone holding the code (default: target)
```

Computed columns are **not stored by the worker**: push attaches them (local
join), pull resolves them (`LEFT JOIN`) and the client rewrites `fk` with
the cloud's canonical id. They exist because local rowids diverge between
devices — an id captured offline on the phone is another row (or none) on
the PC.

### Upsert

```
conflict    local conflict-target columns
set         columns updated on conflict (empty = every inserted column except the target)
include_id  insert the payload's id instead of letting local autoincrement assign
```

## Strategies

| Strategy | Semantics | Requires |
|---|---|---|
| `replace` | `INSERT OR REPLACE` by stored columns (default). | — |
| `catalog` | Reconcile by `business_key`, preserving the local id (local FKs point at it). | `business_key` |
| `fk-rewrite` | Resolve each computed FK to this device's local parent id, then `INSERT OR REPLACE`; rows whose parent is missing locally are dropped. | `computed` |
| `key-upsert` | Rewrite computed FKs (when any), then `INSERT … ON CONFLICT(conflict) DO UPDATE SET …` so the same business identity keeps one local row despite divergent ids. | `upsert.conflict` |
| `validate-only` | Cannot be canonicalized: apply only when the attached code resolves to the same local row, else drop the row. | `computed` |

Escape hatch (not in the manifest): the engine accepts an injectable
`UpsertHook` per app for one-off logic; anything two projects need becomes a
strategy in the kit instead.

## Build-time validation (enforced by `syncengine.NewRegistry`)

- `name`, `pk`, `wire_cols`, `business_key`, `upsert.*`, `tombstone_table`
  and every computed identifier must match `^[A-Za-z_][A-Za-z0-9_]*$`
  (they reach SQL through string interpolation — the allowlist is the
  injection guard; names come from code, never user input).
- No empty name, no duplicate tables, no empty `pk`.
- Every `pk` component is either a computed column or appears in
  `wire_cols`.
- Strategy/requirement matrix above; unknown strategy names rejected.
- `orphan` needs `fk` and `parent` (`parent_key` defaults to `id`).

Ordering: the spec list **is** the payload/apply order. Parents must precede
tables reconciled by their business key (e.g. `estudiante` before
`asistencia`).

## Example (School Tracker, trimmed)

```go
{Name: "estudiante", PK: []string{"codigo"}, Strategy: d1.StrategyCatalog,
 BusinessKey: []string{"codigo"}, Restored: true,
 TombstoneTable: "sync_pending_deletions_estudiante",
 WireCols: []string{"id","nombre","codigo","sexo","ano_lectivo_id",
   "grado_id","seccion_id","colegio_id","foto","updated_at"}},

{Name: "asistencia", PK: []string{"estudiante_codigo","fecha"},
 Strategy: d1.StrategyKeyUpsert, Restored: true,
 Computed: []d1.ComputedFK{{Column:"estudiante_codigo", Parent:"estudiante",
   ParentKey:"id", FK:"estudiante_id", Target:"codigo"}},
 Upsert: d1.Upsert{Conflict: []string{"estudiante_id","fecha"},
   Set: []string{"estado","observacion","updated_at"}},
 TombstoneTable: "sync_pending_deletions_asistencia",
 Orphan: &d1.OrphanPurge{FK:"estudiante_id", Parent:"estudiante", ParentKey:"id"},
 WireCols: []string{"id","estudiante_id","fecha","estado","observacion","updated_at"}},

{Name: "nota", PK: []string{"estudiante_id","criterio_id",
   "corte_evaluativo_id","asignatura_id"},
 Strategy: d1.StrategyValidateOnly, Restored: true, …}
```

The full 22-table manifest lives in
`go/syncengine/example/school/schema.go` (plus its DDL in `ddl.go`) and is
contract-tested from the app side in School Tracker's
`internal/syncschema/schema_test.go`.

## JSON projection (for the worker template, Phase 2)

```json
{
  "version": 1,
  "namespace": "<project>",
  "tables": {
    "asistencia": {
      "pk": ["estudiante_codigo", "fecha"],
      "strategy": "key-upsert",
      "computed": [{ "column": "estudiante_codigo", "parent": "estudiante",
                      "parent_key": "id", "fk": "estudiante_id", "target": "codigo" }],
      "upsert": { "conflict": ["estudiante_id", "fecha"],
                   "set": ["estado", "observacion", "updated_at"] },
      "cols": ["id", "estudiante_id", "fecha", "estado", "observacion", "updated_at"]
    }
  }
}
```

Field names map 1:1 to the TableSpec table above (`cols` = `wire_cols`).
The worker only needs `pk`, `computed`, `cols` and `business_key` to build
its upserts/canonicalization; apply strategies are client-side concerns.

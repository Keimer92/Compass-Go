/**
 * sync-kit worker template — shared types.
 *
 * A project instantiates this template in `instances/<project>/`: the
 * generated `sync.schema.json` (from the Go registry, the single source of
 * truth) plus a `wrangler.toml`. The modules below are brand-free; anything
 * project-specific arrives through the manifest, the bindings present in
 * wrangler.toml (a module mounts only when its bindings exist) or
 * `sync.config.json`.
 */

/** One computed (protocol-only) column, as exported by the Go registry. */
export interface ManifestComputed {
  column: string;
  parent: string;
  parent_key: string;
  fk: string;
  target: string;
  /** Push re-writes `fk` with the parent row found by `target`. */
  canonize: boolean;
}

/** One syncable table, in worker terms. */
export interface ManifestTable {
  name: string;
  /** Canonical identity (tombstone order); may name computed columns. */
  pk: string[];
  /** Stored identity (computed columns replaced by their fk). */
  upsert_pk: string[];
  /** Stored columns, in response key order. */
  cols: string[];
  /** ON CONFLICT target: business key, declared upsert conflict, upsert_pk. */
  conflict: string[];
  /** Conflict is a business key: never re-write the stored id on update. */
  set_exclude_id: boolean;
  /** INSERT drops the id too: the server assigns it (business-key catalogs). */
  server_assigns_id: boolean;
  computed?: ManifestComputed[];
}

export interface Manifest {
  version: number;
  namespace: string;
  tables: ManifestTable[];
}

/** Optional, branding-ish settings (`instances/<p>/sync.config.json`). */
export interface KitConfig {
  /** Project namespace (auth email subject/brand fallback). */
  namespace?: string;
  /** Scheme for the QR deep link redirect (empty disables /conectar). */
  deepLinkScheme?: string;
  /** Prefix of human-dictated recovery codes (e.g. "ST"). */
  recoveryPrefix?: string;
  /** Cloud-auth branding (only read when the auth module is mounted). */
  auth?: { brand?: string; resendFrom?: string; recoveryPrefix?: string };
}

/** Everything a bound D1 row of a syncable table carries beyond `cols`. */
export interface Env {
  CF_ACCOUNT_ID?: string;
  CF_API_TOKEN?: string;
  SCHOOL_ROUTING: KVNamespace;
  DB: D1Database;
  // Optional modules mount only when their bindings are declared.
  USERS?: KVNamespace;
  RELEASES?: KVNamespace;
  SYNC_HUB?: DurableObjectNamespace;
  GOOGLE_CLIENT_ID?: string;
  ADMIN_EMAILS?: string;
  ALLOWED_EMAILS?: string;
  RESEND_API_KEY?: string;
}

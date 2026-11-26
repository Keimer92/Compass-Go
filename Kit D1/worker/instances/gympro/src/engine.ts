/**
 * sync-kit worker engine — manifest-driven push/pull and namespace ops.
 *
 * The table contract comes from `sync.schema.json` (generated from the
 * project's Go registry; NEVER hand-written). This module knows no table
 * names: upserts, canonicalization, pulls, tombstones, resets, stats and id
 * blocks all follow the manifest.
 *
 * Invariants preserved from the original School Tracker worker:
 *   - rows are namespaced by `sk` = sha-256(schoolKey)[0..8] in hex;
 *   - the SERVER seals `updated_at` on every write;
 *   - the worker never assigns ids (the client's id travels) except on
 *     `server_assigns_id` tables, whose id is dropped from the INSERT;
 *   - rows written by the requesting device are excluded from its pull
 *     (first half of the anti-echo);
 *   - a computed FK canonizes through a parent lookup, with a numeric
 *     fallback that keeps legacy tombstones (id-based) working.
 */

import type { Env, KitConfig, Manifest, ManifestTable } from './types';

// The instance directory is the worker root at build time (wrangler `main`
// sits next to the generated schema).
import MANIFEST from '../sync.schema.json';

export const manifest = MANIFEST as unknown as Manifest;

// ── small shared helpers ─────────────────────────────────────────────────
export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

// CORS for WebView fetches to /auth and /admin/* (the sync itself is
// client-side HTTP and does not need it).
export const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, X-School-Key, X-Device-Id, X-Auth-Token',
  'Access-Control-Max-Age': '86400',
};

export function jsonCors(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...CORS_HEADERS } });
}

export function nowSqlite(): string {
  return new Date().toISOString().slice(0, 19).replace('T', ' ');
}

/** Short hash of the school key → the row namespace (`sk`). */
export async function skOf(schoolKey: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(schoolKey));
  return Array.from(new Uint8Array(buf)).slice(0, 8).map((b) => b.toString(16).padStart(2, '0')).join('');
}

export async function queryRows(db: D1Database, sql: string, params: unknown[] = []): Promise<any[]> {
  const r = await db.prepare(sql).bind(...params).all();
  if (!r.success) throw new Error(`D1 error: ${r.error || 'unknown'}`);
  return r.results as any[];
}

/** Max statements per batch call (the real D1 limit is 100). */
export const BATCH_STMT = 90;

export async function batchExec(
  db: D1Database, statements: { sql: string; params: any[] }[],
): Promise<{ results: any[]; changes: number; rowsRead: number }[]> {
  if (statements.length === 0) return [];
  const stmts = statements.map((s) => db.prepare(s.sql).bind(...s.params));
  try {
    const res = await db.batch(stmts);
    for (const r of res) {
      if (!r.success) throw new Error(`D1 batch error: ${r.error || 'unknown'}`);
    }
    return res.map((r) => ({
      results: r.results as any[],
      changes: r.meta?.changes ?? 0,
      rowsRead: r.meta?.rows_read ?? 0,
    }));
  } catch (e: any) {
    throw new Error(`D1 batch error: ${e?.message || String(e)}`);
  }
}

/** Validate a name before it reaches interpolated SQL (manifest-derived). */
const IDENT = /^[A-Za-z_][A-Za-z0-9_]*$/;
function ident(what: string, name: string): string {
  if (!IDENT.test(name)) throw new Error(`manifest: unsafe ${what} ${JSON.stringify(name)}`);
  return name;
}

export function tableByName(name: string): ManifestTable | undefined {
  return manifest.tables.find((t) => t.name === name);
}

// ── routing ──────────────────────────────────────────────────────────────
export async function routeToDb(env: Env, req: Request): Promise<string> {
  const schoolKey = req.headers.get('X-School-Key');
  if (!schoolKey) throw new Error('Missing X-School-Key');
  const dbId = await env.SCHOOL_ROUTING.get(schoolKey);
  if (!dbId) throw new Error('Unknown or suspended school key');
  return dbId;
}

// ── schema bootstrap (generic part) ──────────────────────────────────────
let schemaReady = false;

/**
 * Cold-start schema guarantees the protocol needs, derived from the
 * manifest: infrastructure tables, the `sk`/`device_id` columns + index on
 * every syncable table, and the UNIQUE indexes that business-key conflicts
 * require. Project legacy migrations live in the instance and run first.
 */
export async function ensureSchema(env: Env, legacy?: (env: Env) => Promise<void>): Promise<void> {
  if (schemaReady) return;

  if (legacy) await legacy(env);

  await env.DB.prepare(
    'CREATE TABLE IF NOT EXISTS sync_metadata (key TEXT PRIMARY KEY, value INTEGER NOT NULL DEFAULT 0)',
  ).run();
  await env.DB.prepare(`CREATE TABLE IF NOT EXISTS sync_tombstone (
    tabla TEXT NOT NULL, pk TEXT NOT NULL, updated_at TEXT NOT NULL,
    device_id TEXT, sk TEXT DEFAULT '',
    PRIMARY KEY (tabla, pk, sk))`).run();

  for (const t of manifest.tables) {
    const table = ident('table name', t.name);
    try { await env.DB.prepare(`ALTER TABLE ${table} ADD COLUMN sk TEXT`).run(); } catch { /* exists */ }
    try { await env.DB.prepare(`ALTER TABLE ${table} ADD COLUMN device_id TEXT`).run(); } catch { /* exists */ }
    try { await env.DB.prepare(`CREATE INDEX IF NOT EXISTS idx_${table}_sk ON ${table}(sk, updated_at)`).run(); } catch { /* exists */ }
    // A business-key upsert needs a UNIQUE index on the conflict columns.
    if (t.set_exclude_id) {
      const conflict = t.conflict.map((c) => ident('column', c));
      try {
        await env.DB.prepare(
          `CREATE UNIQUE INDEX IF NOT EXISTS idx_${table}_conflict ON ${table}(${conflict.join(', ')})`,
        ).run();
      } catch { /* duplicate values: business-key upsert will fall through */ }
    }
  }

  schemaReady = true;
}

/** Reset the cold-start flag (tests). */
export function resetSchemaFlagForTest(): void { schemaReady = false; }

// ── upsert / pull SQL builders ───────────────────────────────────────────
function canonizing(t: ManifestTable): NonNullable<ManifestTable['computed']> {
  return (t.computed ?? []).filter((c) => c.canonize);
}

/** LWW upsert with the row namespace (`sk`); the server seals `updated_at`. */
export function buildUpsert(
  t: ManifestTable, rows: any[], deviceId: string, serverNow: string, sk: string,
): { sql: string; params: any[] } {
  const table = ident('table name', t.name);
  const extras = canonizing(t);
  const withMeta = [...t.cols, 'device_id', 'sk'];
  // server_assigns_id: the INSERT drops the id (the business key is the
  // identity); set_exclude_id: the UPDATE never re-writes the stored id.
  const insertCols = t.server_assigns_id ? withMeta.filter((c) => c !== 'id') : withMeta;
  const setCols = t.set_exclude_id ? withMeta.filter((c) => c !== 'id') : withMeta;
  insertCols.forEach((c) => ident('column', c));
  setCols.forEach((c) => ident('column', c));
  const conflictTarget = t.conflict.map((c) => ident('column', c)).join(', ');

  const valueTuples: string[] = [];
  const params: any[] = [];
  let n = 0;
  for (const row of rows) {
    const cells: string[] = [];
    for (const c of insertCols) {
      const comp = extras.find((e) => e.fk === c);
      if (comp) {
        // Canonical FK: the sender's local id diverges between devices, so
        // the stored value is the parent's id IN THIS namespace found by the
        // business code. A numeric code (legacy tombstone/id) keeps the id.
        cells.push(
          `COALESCE((SELECT p.id FROM ${ident('parent', comp.parent)} p WHERE p.sk = ?${n + 1} AND p.${ident('target', comp.target)} = ?${n + 2}), ?${n + 3})`,
        );
        let id: any = row[comp.fk] ?? null;
        if (typeof id === 'boolean') id = id ? 1 : 0;
        params.push(sk, row[comp.column] ?? null, id);
        n += 3;
        continue;
      }
      if (c === 'device_id') { cells.push(`?${n + 1}`); params.push(deviceId); n += 1; continue; }
      if (c === 'updated_at') { cells.push(`?${n + 1}`); params.push(serverNow); n += 1; continue; }
      if (c === 'sk') { cells.push(`?${n + 1}`); params.push(sk); n += 1; continue; }
      let v: any = row[c] ?? null;
      if (typeof v === 'boolean') v = v ? 1 : 0;
      cells.push(`?${n + 1}`); params.push(v); n += 1;
    }
    valueTuples.push(`(${cells.join(', ')})`);
  }

  const setClause = setCols.map((c) => `${c} = excluded.${c}`).join(', ');
  const changedCols = setCols.filter((c) => c !== 'updated_at' && c !== 'device_id' && c !== 'sk');
  const changedClause = changedCols.length
    ? changedCols.map((c) => `${table}.${c} IS NOT excluded.${c}`).join(' OR ')
    : '0';
  return {
    sql: `INSERT INTO ${table} (${insertCols.join(', ')}) VALUES ${valueTuples.join(', ')}
ON CONFLICT(${conflictTarget}) DO UPDATE SET ${setClause}
WHERE ${changedClause}`,
    params,
  };
}

/** `/pull` SELECT: stored columns (+ computed columns resolved by joins). */
export function pullSelect(t: ManifestTable): string {
  const table = ident('table name', t.name);
  const computed = t.computed ?? [];
  if (computed.length === 0) {
    return `SELECT * FROM ${table} WHERE sk = ?1 AND updated_at > ?2 AND IFNULL(device_id, '') != ?3`;
  }
  const joins = computed.map((c, i) => {
    const p = ident('parent', c.parent);
    return `LEFT JOIN ${p} p${i} ON p${i}.sk = ${table}.sk AND p${i}.${ident('parent key', c.parent_key)} = ${table}.${ident('fk', c.fk)}`;
  });
  const extra = computed.map((c, i) => `p${i}.${ident('target', c.target)} AS ${ident('column', c.column)}`);
  return `SELECT ${table}.*, ${extra.join(', ')} FROM ${table} ${joins.join(' ')}
WHERE ${table}.sk = ?1 AND ${table}.updated_at > ?2 AND IFNULL(${table}.device_id, '') != ?3`;
}

/** WHERE for a tombstone delete: canonical pk, computed columns resolved. */
export function deleteStatements(t: ManifestTable, pk: any[], sk: string): { sql: string; params: any[] } {
  const table = ident('table name', t.name);
  if (t.pk.length !== pk.length) {
    throw new Error(`${table}: deletion pk has ${pk.length} parts, manifest says ${t.pk.length}`);
  }
  const parts: string[] = [];
  const params: any[] = [];
  let n = 0;
  for (let i = 0; i < pk.length; i++) {
    const col = ident('pk column', t.pk[i]);
    const comp = (t.computed ?? []).find((c) => c.column === col);
    if (comp && typeof pk[i] !== 'number') {
      // Code-keyed deletion: match the fk through the parent in this
      // namespace (the same canonicalization the insert does).
      const fk = ident('fk', comp.fk);
      const target = ident('target', comp.target);
      const parent = ident('parent', comp.parent);
      parts.push(`${fk} IN (SELECT p.id FROM ${parent} p WHERE p.sk = ?${pk.length + 1} AND p.${target} = ?${n + 1})`);
      params.push(pk[i]); n += 1;
      continue;
    }
    if (comp) {
      // Legacy numeric tombstone: the value is already an id of SOME device;
      // match the stored fk (original behavior).
      parts.push(`${ident('fk', comp.fk)} = ?${n + 1}`);
      params.push(pk[i]); n += 1;
      continue;
    }
    parts.push(`${col} = ?${n + 1}`);
    params.push(pk[i]); n += 1;
  }
  return {
    sql: `DELETE FROM ${table} WHERE ${parts.join(' AND ')} AND sk = ?${n + 1}`,
    params: [...params, sk],
  };
}

// ── endpoints ────────────────────────────────────────────────────────────
export async function handleEngine(
  env: Env, req: Request, path: string, config: KitConfig,
): Promise<Response | null> {
  switch (path) {
    case '/hello': {
      const dbId = await routeToDb(env, req);
      const rows = await queryRows(env.DB, 'SELECT 1 AS ok');
      return json({ ok: rows.length > 0, databaseId: dbId, serverTime: new Date().toISOString() });
    }

    case '/reset': {
      // Total wipe of the caller's namespace (same auth as /push|/pull).
      const schoolKey = req.headers.get('X-School-Key') || '';
      if (!schoolKey) return json({ ok: false, error: 'Missing X-School-Key' }, 403);
      await routeToDb(env, req);
      const sk = await skOf(schoolKey);
      const stmts = [
        ...manifest.tables.map((t) => ({ sql: `DELETE FROM ${ident('table name', t.name)} WHERE sk = ?1`, params: [sk] })),
        { sql: 'DELETE FROM sync_tombstone WHERE sk = ?1', params: [sk] },
      ];
      let borradas = 0;
      for (let i = 0; i < stmts.length; i += BATCH_STMT) {
        const res = await batchExec(env.DB, stmts.slice(i, i + BATCH_STMT));
        borradas += res.reduce((a, b) => a + b.changes, 0);
      }
      return json({ ok: true, borradas });
    }

    case '/stats': {
      const schoolKey = req.headers.get('X-School-Key') || '';
      if (!schoolKey) return json({ ok: false, error: 'Missing X-School-Key' }, 403);
      await routeToDb(env, req);
      const sk = await skOf(schoolKey);
      const stmts = [
        ...manifest.tables.map((t) => ({ sql: `SELECT COUNT(*) AS n FROM ${ident('table name', t.name)} WHERE sk = ?1`, params: [sk] })),
        { sql: 'SELECT COUNT(*) AS n FROM sync_tombstone WHERE sk = ?1', params: [sk] },
      ];
      const results = await batchExec(env.DB, stmts);
      const porTabla: Record<string, number> = {};
      let total = 0;
      manifest.tables.forEach((t, i) => {
        const n = Number(results[i].results?.[0]?.n) || 0;
        porTabla[t.name] = n;
        total += n;
      });
      const tombstones = Number(results[manifest.tables.length].results?.[0]?.n) || 0;
      return json({ ok: true, total, tombstones, porTabla });
    }

    case '/push': {
      const schoolKey = req.headers.get('X-School-Key') || '';
      await routeToDb(env, req);
      const sk = await skOf(schoolKey);
      const body: any = await req.json();
      const deviceId = body.deviceId || (req.headers.get('X-Device-Id') ?? '');
      if (!deviceId) throw new Error('Missing deviceId');
      const data: Record<string, any[]> = body.data || {};
      const deletions: { tabla: string; pk: any[] }[] = body.deletions || [];

      const statements: { sql: string; params: any[] }[] = [];
      const stats: Record<string, number> = {};

      const serverNow = nowSqlite();
      for (const [name, rows] of Object.entries(data)) {
        const t = tableByName(name);
        if (!t || !Array.isArray(rows) || rows.length === 0) continue;
        const extraParams = canonizing(t).length > 0 ? 2 : 0;
        const chunkSize = Math.max(1, Math.floor(90 / (t.cols.length + 3 + extraParams)));
        for (let i = 0; i < rows.length; i += chunkSize) {
          statements.push(buildUpsert(t, rows.slice(i, i + chunkSize), deviceId, serverNow, sk));
        }
        stats[name] = rows.length;
      }

      let delCount = 0;
      for (const del of deletions) {
        const t = tableByName(del.tabla);
        if (!t) continue;
        statements.push({
          sql: `INSERT INTO sync_tombstone (tabla, pk, updated_at, device_id, sk)
VALUES (?1, ?2, ?3, ?4, ?5)
ON CONFLICT(tabla, pk, sk) DO UPDATE SET updated_at = excluded.updated_at, device_id = excluded.device_id`,
          params: [del.tabla, JSON.stringify(del.pk), serverNow, deviceId, sk],
        });
        statements.push(deleteStatements(t, del.pk, sk));
        delCount++;
      }

      let realChanges = 0;
      for (let i = 0; i < statements.length; i += BATCH_STMT) {
        const res = await batchExec(env.DB, statements.slice(i, i + BATCH_STMT));
        realChanges += res.reduce((a, b) => a + b.changes, 0);
      }

      if ((realChanges > 0 || delCount > 0) && schoolKey && env.SYNC_HUB) {
        try {
          const stub = env.SYNC_HUB.get(env.SYNC_HUB.idFromName(schoolKey));
          await stub.fetch('https://sync-hub/broadcast', {
            method: 'POST', body: JSON.stringify({ from: deviceId }),
          });
        } catch (e) { console.error('[WS] broadcast failed:', e); }
      }

      return json({ ok: true, applied: stats, deletionsApplied: delCount });
    }

    case '/pull': {
      const schoolKey = req.headers.get('X-School-Key') || '';
      await routeToDb(env, req);
      const sk = await skOf(schoolKey);
      const body: any = await req.json();
      const since = body.since || '1970-01-01 00:00:00';
      const deviceId: string = body.deviceId || '';

      const statements = manifest.tables.map((t) => ({ sql: pullSelect(t), params: [sk, since, deviceId] }));
      statements.push({
        sql: `SELECT tabla, pk, updated_at, device_id FROM sync_tombstone WHERE sk = ?1 AND updated_at > ?2 AND IFNULL(device_id, '') != ?3`,
        params: [sk, since, deviceId],
      });
      const results = await batchExec(env.DB, statements);

      const rows: Record<string, any[]> = {};
      manifest.tables.forEach((t, i) => {
        const extra = (t.computed ?? []).map((c) => c.column);
        rows[t.name] = (results[i].results || []).map((r: any) => {
          const o: Record<string, any> = {};
          for (const c of [...t.cols, ...extra]) o[c] = r[c];
          return o;
        });
      });
      const deleted = (results[manifest.tables.length].results || []).map((ts: any) => ({
        tabla: ts.tabla, pk: JSON.parse(ts.pk), updated_at: ts.updated_at, device_id: ts.device_id,
      }));

      return json({ ok: true, rows, deleted, serverTime: nowSqlite() });
    }

    case '/ids': {
      await routeToDb(env, req);
      const body: any = await req.json();
      const blocks: Record<string, { start: number; end: number }> = {};
      const needed = Object.entries(body.blocks || {}).filter(([t]) => !!tableByName(t));
      const statements = needed.map(([table, count]) => ({
        sql: `INSERT INTO sync_metadata (key, value) VALUES (?1, ?2)
ON CONFLICT(key) DO UPDATE SET value = value + ?2
RETURNING value`,
        params: [`id:${ident('table name', table)}`, Math.max(1, Math.floor(Number(count) || 1))],
      }));
      const results = await batchExec(env.DB, statements);
      needed.forEach(([table, count], i) => {
        const n = Math.max(1, Math.floor(Number(count) || 1));
        const newVal = Number(results[i].results[0]?.value) || n;
        blocks[table] = { start: newVal - n + 1, end: newVal };
      });
      return json({ ok: true, blocks });
    }

    case '/usage': {
      // D1 quota via the account Analytics API. Bypasses ensureSchema so it
      // still answers when the read quota is exhausted.
      if (path === '/usage' && req.method !== 'POST') return null;
      await routeToDb(env, req);
      if (!env.CF_API_TOKEN || !env.CF_ACCOUNT_ID) {
        throw new Error('Cloudflare Analytics API is not configured');
      }
      if (!env.USERS) throw new Error('USERS KV (usage cache) is not bound');
      const dayUtc = new Date().toISOString().slice(0, 10);
      const cacheKey = `d1-usage-v2:${env.CF_ACCOUNT_ID}:${dayUtc}`;
      const cached = await env.USERS.get(cacheKey);
      if (cached) {
        return new Response(cached, { headers: { 'Content-Type': 'application/json', 'X-Usage-Cache': 'HIT' } });
      }
      const query = `
        query D1DailyUsage($accountTag: String!, $start: Date, $end: Date) {
          viewer {
            accounts(filter: { accountTag: $accountTag }) {
              d1AnalyticsAdaptiveGroups(limit: 10000, filter: { date_geq: $start, date_leq: $end }) {
                sum { rowsRead rowsWritten }
              }
            }
          }
        }`;
      const ares = await fetch('https://api.cloudflare.com/client/v4/graphql', {
        method: 'POST',
        headers: { Authorization: `Bearer ${env.CF_API_TOKEN}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ query, variables: { accountTag: env.CF_ACCOUNT_ID, start: dayUtc, end: dayUtc } }),
        signal: AbortSignal.timeout(10000),
      });
      if (!ares.ok) {
        if (ares.status === 429) {
          return new Response(JSON.stringify({ ok: false, error: 'Cloudflare Analytics is rate-limiting usage checks; please retry later.' }),
            { status: 429, headers: { 'Content-Type': 'application/json', 'Retry-After': ares.headers.get('Retry-After') || '60' } });
        }
        throw new Error(`Cloudflare Analytics API returned HTTP ${ares.status}`);
      }
      const analytics: any = await ares.json();
      if (analytics.errors?.length) {
        const messages = analytics.errors.map((e: any) => e?.message).filter(Boolean).join('; ');
        if (/not authorized.*account|unauthorized.*account|account.*not authorized/i.test(messages)) {
          return json({ ok: false, error: 'The Worker CF_API_TOKEN cannot access the configured Cloudflare account. Grant Account Analytics:Read and verify the account scope matches CF_ACCOUNT_ID.' }, 502);
        }
        throw new Error(messages || 'Cloudflare Analytics query failed');
      }
      const groups = analytics.data?.viewer?.accounts?.[0]?.d1AnalyticsAdaptiveGroups;
      if (!Array.isArray(groups)) throw new Error('Cloudflare Analytics returned no account data');
      const rowsRead = groups.reduce((t: number, g: any) => t + (Number(g.sum?.rowsRead) || 0), 0);
      const rowsWritten = groups.reduce((t: number, g: any) => t + (Number(g.sum?.rowsWritten) || 0), 0);
      const dailyReadLimit = 5_000_000;
      const dailyWriteLimit = 100_000;
      const usage = {
        ok: true, scope: 'account', dayUtc, rowsRead, rowsWritten,
        dailyReadLimit, dailyWriteLimit,
        readPercentage: (rowsRead / dailyReadLimit) * 100,
        writePercentage: (rowsWritten / dailyWriteLimit) * 100,
        updatedAt: new Date().toISOString(),
      };
      await env.USERS.put(cacheKey, JSON.stringify(usage), { expirationTtl: 60 });
      return json(usage);
    }

    default:
      return null;
  }
}

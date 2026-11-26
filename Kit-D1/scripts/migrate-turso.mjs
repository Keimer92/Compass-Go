#!/usr/bin/env node
/**
 * migrate-turso.mjs — copy a project's Turso (libSQL) database into its D1
 * database through the deployed sync Worker.
 *
 * Why through the Worker and not `wrangler d1 execute`:
 *   the Worker is the only place that knows the row namespace (`sk`), seals
 *   `updated_at` with the server clock and writes the protocol's tombstones.
 *   Writing raw SQL to D1 would produce rows no client could pull.
 *
 * The table/column contract comes from the instance manifest
 * (`<dir>/sync.schema.json`), the same file the Worker uses — so the script
 * cannot invent columns the Worker will not store.
 *
 * Usage:
 *   node migrate-turso.mjs --dir worker/instances/gympro \
 *     --turso-url libsql://... --turso-token ey... \
 *     --worker-url https://gympro-d1-sync.<account>.workers.dev \
 *     --key gym_xxxxxxxxxxxxxxxxx
 *
 *   node migrate-turso.mjs ... --dry-run          # read Turso, print counts
 *   node migrate-turso.mjs ... --tables=members,payments --limit=50
 *   node migrate-turso.mjs ... --key-file ../gympro.key   # {workerUrl, schoolKey}
 *
 * Credentials may also come from the environment:
 *   TURSO_URL / TURSO_TOKEN, WORKER_URL / SYNC_KEY
 *
 * Read-only against Turso; idempotent against D1 (the Worker upserts by id).
 */

import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

// ── CLI ────────────────────────────────────────────────────────────────────
function arg(name) {
  const argv = process.argv.slice(2);
  const eq = argv.find((a) => a.startsWith(`--${name}=`));
  if (eq) return eq.slice(name.length + 3);
  const i = argv.indexOf(`--${name}`);
  if (i !== -1 && argv[i + 1] && !argv[i + 1].startsWith('--')) return argv[i + 1];
  return null;
}
const flag = (name) => process.argv.includes(`--${name}`);

const DIR = resolve(arg('dir') || process.cwd());
const TURSO_URL = (arg('turso-url') || process.env.TURSO_URL || '').trim();
const TURSO_TOKEN = (arg('turso-token') || process.env.TURSO_TOKEN || '').trim();

// A `.key` file written by generate-key.mjs is {workerUrl, schoolKey}; accept
// it directly so the operator does not retype two credentials.
const KEY_FILE = arg('key-file');
let fileCreds = {};
if (KEY_FILE) {
  const p = resolve(KEY_FILE);
  if (!existsSync(p)) throw new Error(`key file not found: ${p}`);
  fileCreds = JSON.parse(readFileSync(p, 'utf8'));
}

const WORKER_URL = (arg('worker-url') || process.env.WORKER_URL || fileCreds.workerUrl || '').trim().replace(/\/+$/, '');
const SYNC_KEY = (arg('key') || process.env.SYNC_KEY || fileCreds.schoolKey || '').trim();
const DRY = flag('dry-run');
const BATCH = Math.max(1, Number(arg('batch') || 100));
const LIMIT = Number(arg('limit') || 0);
const ONLY = (arg('tables') || '').split(',').map((s) => s.trim()).filter(Boolean);

const MANIFEST_PATH = join(DIR, 'sync.schema.json');

// ── Manifest ───────────────────────────────────────────────────────────────
function loadManifest() {
  if (!existsSync(MANIFEST_PATH)) {
    throw new Error(`manifest not found: ${MANIFEST_PATH}\n  generate it with: go run ./cmd/gen-manifest -o ${MANIFEST_PATH}`);
  }
  return JSON.parse(readFileSync(MANIFEST_PATH, 'utf8'));
}

// Turso columns that never existed in the legacy schema get a value derived
// from the nearest historical timestamp. Same expressions as
// internal/db/syncd1.go (syncUpdatedAt): a NULL updated_at would leave the row
// out of every future pull, because `updated_at > cursor` is NULL against all.
const UPDATED_AT_FALLBACK = {
  payments: 'COALESCE(created_at, payment_date, CURRENT_TIMESTAMP)',
  attendance: 'COALESCE(check_in, CURRENT_TIMESTAMP)',
  sales: 'COALESCE(date, CURRENT_TIMESTAMP)',
  sale_items: 'COALESCE((SELECT s.date FROM sales s WHERE s.id = sale_items.sale_id), CURRENT_TIMESTAMP)',
  audit_logs: 'COALESCE(created_at, CURRENT_TIMESTAMP)',
  categories: 'COALESCE(created_at, CURRENT_TIMESTAMP)',
  roles: 'COALESCE(created_at, CURRENT_TIMESTAMP)',
  pricing_plans: 'CURRENT_TIMESTAMP',
};

// ── Turso (libSQL HTTP pipeline, read-only) ────────────────────────────────
function tursoEndpoint() {
  return `${TURSO_URL.replace(/^libsql:\/\//, 'https://').replace(/\/+$/, '')}/v2/pipeline`;
}

async function turso(sql) {
  const resp = await fetch(tursoEndpoint(), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${TURSO_TOKEN}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      requests: [{ type: 'execute', stmt: { sql, args: [] } }, { type: 'close' }],
    }),
  });
  const body = await resp.json().catch(() => null);
  const item = body?.results?.[0];
  if (item?.type === 'error') {
    throw new Error(`Turso: ${item.error?.message || JSON.stringify(item)}`);
  }
  if (!resp.ok) throw new Error(`Turso HTTP ${resp.status}: ${JSON.stringify(body)}`);
  return item?.response?.result ?? { cols: [], rows: [] };
}

function cell(v) {
  if (!v || v.type === 'null') return null;
  switch (v.type) {
    case 'text':
      return v.value;
    case 'integer':
    case 'real':
    case 'float':
      return Number(v.value);
    case 'blob':
      return v.base64 ?? v.value ?? null;
    default:
      return v.value ?? null;
  }
}

function rowToObject(cols, row) {
  const out = {};
  cols.forEach((c, i) => { out[c.name] = cell(row[i]); });
  return out;
}

async function tursoColumns(table) {
  const r = await turso(`PRAGMA table_info("${table}")`);
  return new Set(r.rows.map((row) => cell(row[1])));
}

async function readTable(table, cols, tursoCols) {
  const exprs = cols.map((c) => {
    if (tursoCols.has(c)) return `"${c}"`;
    if (c === 'updated_at' && UPDATED_AT_FALLBACK[table]) {
      return `${UPDATED_AT_FALLBACK[table]} AS updated_at`;
    }
    throw new Error(`Turso table "${table}" has no column "${c}" required by the manifest`);
  });

  const out = [];
  const page = 500;
  for (let offset = 0; ; offset += page) {
    const limit = LIMIT ? Math.min(page, LIMIT - out.length) : page;
    if (limit <= 0) break;
    const r = await turso(
      `SELECT ${exprs.join(', ')} FROM "${table}" ORDER BY id LIMIT ${limit} OFFSET ${offset}`,
    );
    const colsMeta = r.cols?.length ? r.cols : cols.map((name) => ({ name }));
    for (const row of r.rows) out.push(rowToObject(colsMeta, row));
    if (r.rows.length < limit) break;
  }
  return out;
}

// ── Worker push ────────────────────────────────────────────────────────────
async function push(table, rows) {
  const resp = await fetch(`${WORKER_URL}/push`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-School-Key': SYNC_KEY,
    },
    body: JSON.stringify({
      since: '1970-01-01 00:00:00',
      deviceId: 'turso-migration',
      data: { [table]: rows },
      deletions: [],
    }),
  });
  const body = await resp.json().catch(() => null);
  if (!resp.ok || body?.ok === false) {
    throw new Error(`${table}: worker /push failed (HTTP ${resp.status}): ${body?.error || JSON.stringify(body)}`);
  }
  return body?.applied?.[table] ?? rows.length;
}

// ── Main ───────────────────────────────────────────────────────────────────
async function main() {
  if (!TURSO_URL || !TURSO_TOKEN) throw new Error('missing Turso URL/token (--turso-url/--turso-token or TURSO_URL/TURSO_TOKEN)');
  if (!DRY && (!WORKER_URL || !SYNC_KEY)) throw new Error('missing worker URL/key (--worker-url/--key or WORKER_URL/SYNC_KEY)');

  const manifest = loadManifest();
  const tables = manifest.tables.filter((t) => ONLY.length === 0 || ONLY.includes(t.name));

  console.log(`manifest : ${MANIFEST_PATH}`);
  console.log(`turso    : ${TURSO_URL.replace(/^libsql:\/\//, 'https://')}`);
  console.log(`worker   : ${DRY ? '(dry run)' : WORKER_URL}`);
  console.log(`tables   : ${tables.map((t) => t.name).join(', ')}\n`);

  let totalRows = 0;
  const perTable = {};

  for (const t of tables) {
    const cols = t.cols;
    const tursoCols = await tursoColumns(t.name);
    const rows = await readTable(t.name, cols, tursoCols);

    if (DRY) {
      perTable[t.name] = rows.length;
      totalRows += rows.length;
      console.log(`  ${t.name.padEnd(16)} ${String(rows.length).padStart(6)} rows  (dry run)`);
      continue;
    }

    let applied = 0;
    for (let i = 0; i < rows.length; i += BATCH) {
      applied += await push(t.name, rows.slice(i, i + BATCH));
    }
    perTable[t.name] = applied;
    totalRows += applied;
    console.log(`  ${t.name.padEnd(16)} ${String(applied).padStart(6)} rows pushed`);
  }

  console.log(`\nTotal: ${totalRows} rows ${DRY ? 'read' : 'pushed'}.`);
  if (!DRY) {
    try {
      const resp = await fetch(`${WORKER_URL}/stats`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-School-Key': SYNC_KEY },
        body: '{}',
      });
      const stats = await resp.json();
      if (stats?.ok) {
        console.log('\nRemote /stats:');
        for (const [name, n] of Object.entries(stats.porTabla ?? {})) {
          const local = perTable[name] ?? 0;
          const mark = Number(n) === Number(local) ? 'ok' : `MISMATCH (local ${local})`;
          console.log(`  ${name.padEnd(16)} ${String(n).padStart(6)}  ${mark}`);
        }
        console.log(`  tombstones      ${String(stats.tombstones ?? 0).padStart(6)}`);
      }
    } catch (e) {
      console.log(`\n(stats unavailable: ${e.message})`);
    }
  }
}

main().catch((e) => {
  console.error(`\nERROR: ${e.message}`);
  process.exit(1);
});

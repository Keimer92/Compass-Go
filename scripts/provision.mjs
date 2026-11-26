#!/usr/bin/env node
/**
 * provision.mjs — provision the Cloudflare infrastructure (D1 + KV + first
 * sync key) for ONE project using this kit.
 *
 * Ported from School Tracker's `cloudflare-sync/scripts/d1-provision.mjs` and
 * de-branded: everything project-specific comes from a small JSON config so
 * each app can run the same script against its own worker directory.
 *
 * Steps (all idempotent, safe to re-run):
 *   1. Resolve the D1 database named in the worker's wrangler.toml
 *      ([[d1_databases]]). If missing in the account, optionally create it
 *      and write its database_id back into the TOML.
 *   2. Apply the schema files (CREATE TABLE IF NOT EXISTS style) remotely.
 *   3. Verify/create the KV namespaces listed in the config and write their
 *      ids into wrangler.toml.
 *   4. Reuse (or mint) the tenant's sync key and register it in the routing
 *      KV → database binding target.
 *   5. With --deploy: `wrangler deploy`.
 *
 * Usage:
 *   node provision.mjs --dir ../myapp/cloudflare           # plan + confirm
 *   node provision.mjs --dir <workerDir> --dry-run         # show plan only
 *   node provision.mjs --yes                               # no prompts (CI)
 *   node provision.mjs --no-create --no-kv --skip-schema   # keys/deploy only
 *   node provision.mjs --deploy
 *
 * Config (default `<workerDir>/sync-project.json`, override with --config):
 *   {
 *     "namespace":  "myapp",              // used in output file names
 *     "keyPrefix":  "app_",               // sync key prefix
 *     "d1Binding":  "DB",                 // informational (TOML is the truth)
 *     "routingKv":  "SCHOOL_ROUTING",     // binding that maps key -> database
 *     "kvNamespaces": ["SCHOOL_ROUTING", "USERS"],   // created in order
 *     "schemaFiles": ["schema-base.sql"], // applied in order, remotely
 *     "tenantsFile": "tenants.json",      // { "tenants": [{ "slug": ... }] }
 *     "defaultTenant": "default",
 *     "keysFile": "sync-keys.json"        // OUTPUT — a credential store, gitignore it
 *   }
 *
 * Requires: node >= 18 and an authenticated wrangler (`npx wrangler login`).
 */

import { execSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import crypto from 'node:crypto';
import readline from 'node:readline';

const FLAGS = {
  dryRun: process.argv.includes('--dry-run'),
  yes: process.argv.includes('--yes'),
  noCreate: process.argv.includes('--no-create'),
  noKv: process.argv.includes('--no-kv'),
  skipSchema: process.argv.includes('--skip-schema'),
  deploy: process.argv.includes('--deploy'),
};

function arg(name, alias = null) {
  const argv = process.argv.slice(2);
  for (const key of [name, alias]) {
    if (!key) continue;
    const i = argv.indexOf(`--${key}`);
    if (i !== -1 && argv[i + 1] && !argv[i + 1].startsWith('--')) return argv[i + 1];
  }
  return null;
}

const ROOT = resolve(arg('dir', 'worker-dir') || process.cwd());
const CONFIG_PATH = resolve(arg('config') || join(ROOT, 'sync-project.json'));

const DEFAULTS = {
  namespace: 'sync',
  keyPrefix: 'sync_',
  routingKv: 'SCHOOL_ROUTING',
  kvNamespaces: ['SCHOOL_ROUTING'],
  schemaFiles: ['schema.sql'],
  tenantsFile: 'tenants.json',
  defaultTenant: 'default',
  keysFile: 'sync-keys.json',
};

function loadConfig() {
  if (!existsSync(CONFIG_PATH)) return { ...DEFAULTS };
  const raw = JSON.parse(readFileSync(CONFIG_PATH, 'utf8'));
  return { ...DEFAULTS, ...raw };
}

// ---------------------------------------------------------------------------
// Wrangler helpers
// ---------------------------------------------------------------------------
let WRANGLER_CMD = 'npx --yes wrangler';

// Everything below reaches wrangler through a shell string, so each
// interpolated value must pass an allowlist first (shell-injection guard).
const SAFE = {
  name: /^[A-Za-z0-9_.\-]+$/,          // D1 names, KV bindings, file names
  id: /^[0-9a-fA-F-]{8,64}$/,          // database uuid / 32-hex KV id
  key: /^[A-Za-z0-9_]+$/,              // sync keys (prefix + hex)
};
function guard(what, value, kind) {
  if (typeof value !== 'string' || !SAFE[kind].test(value)) {
    throw new Error(`refusing to run wrangler with unsafe ${what}: ${JSON.stringify(value)}`);
  }
  return value;
}

function detectarWrangler() {
  try {
    execSync('wrangler --version', { stdio: 'ignore', shell: true });
    return 'wrangler';
  } catch {
    return 'npx --yes wrangler';
  }
}

function wrangler(args, { allowFail = false } = {}) {
  const cmd = `${WRANGLER_CMD} ${args}`;
  console.log(`\n$ ${cmd}`);
  try {
    return execSync(cmd, {
      cwd: ROOT, encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'], shell: true,
    }).trim();
  } catch (e) {
    if (allowFail) return '';
    const salida = [e.stdout, e.stderr].filter(Boolean).join('\n').trim();
    throw new Error(`wrangler ${args} failed:\n${salida || e.message}`);
  }
}

function escribir(archivo, contenido) {
  writeFileSync(archivo, contenido, 'utf8'); // UTF-8, no BOM
}

// ---------------------------------------------------------------------------
// Minimal TOML block access (line-based; keeps comments and formatting)
// ---------------------------------------------------------------------------
function leerToml() {
  const p = join(ROOT, 'wrangler.toml');
  if (!existsSync(p)) throw new Error(`no wrangler.toml in ${ROOT}`);
  return readFileSync(p, 'utf8');
}

/** Text of the `indice`-th `[[tabla]]` block, or null. */
function bloqueTabla(toml, tabla, indice = 0) {
  const lineas = toml.split(/\r?\n/);
  let dentro = false, n = 0, buf = [];
  for (const linea of lineas) {
    const t = linea.trim();
    if (t.startsWith('[')) {
      if (dentro) {
        if (n === indice) return buf.join('\n');
        n++; dentro = false; buf = [];
      }
      if (t === `[[${tabla}]]` || t === `[${tabla}]`) { dentro = true; buf = []; }
      continue;
    }
    if (dentro) buf.push(linea);
  }
  if (dentro && n === indice) return buf.join('\n');
  return null;
}

/** All `[[kv_namespaces]]` blocks keyed by their `binding` value. */
function kvBloquesPorBinding(toml) {
  const out = {};
  const lineas = toml.split(/\r?\n/);
  let dentro = false, buf = [];
  const flush = () => {
    if (!dentro) return;
    const bloque = buf.join('\n');
    const binding = bloque.match(/binding\s*=\s*"([^"]+)"/)?.[1];
    if (binding) out[binding] = bloque;
  };
  for (const linea of lineas) {
    const t = linea.trim();
    if (t.startsWith('[')) {
      flush();
      dentro = t === '[[kv_namespaces]]';
      buf = [];
      continue;
    }
    if (dentro) buf.push(linea);
  }
  flush();
  return out;
}

function valorEnBloque(toml, tabla, clave, indice = 0) {
  const bloque = bloqueTabla(toml, tabla, indice);
  if (bloque === null) return null;
  return bloque.match(new RegExp(`${clave}\\s*=\\s*"([^"]*)"`))?.[1] ?? null;
}

function reemplazarEnBloque(toml, tabla, clave, valor, indice = 0) {
  const bloque = bloqueTabla(toml, tabla, indice);
  if (bloque === null) throw new Error(`no [[${tabla}]] block in wrangler.toml`);
  const nuevoBloque = bloque.replace(
    new RegExp(`(${clave}\\s*=\\s*)"[^"]*"`), `$1"${valor}"`,
  );
  if (nuevoBloque === bloque) throw new Error(`no key ${clave} inside [[${tabla}]]`);
  const salto = toml.includes('\r\n') ? '\r\n' : '\n';
  return toml.replace(
    bloque.replace(/\n/g, salto),
    nuevoBloque.replace(/\n/g, salto),
  );
}

/** Set `binding`'s KV id, appending a new [[kv_namespaces]] block if absent. */
function ponerKvId(toml, binding, id) {
  const bloques = kvBloquesPorBinding(toml);
  const bloque = bloques[binding];
  if (bloque !== undefined) {
    if (/id\s*=\s*"[^"]+"/.test(bloque)) {
      return toml.replace(bloque, bloque.replace(/id\s*=\s*"[^"]*"/, `id = "${id}"`));
    }
    return toml.replace(bloque, bloque + `\nid = "${id}"\n`);
  }
  return toml.replace(/\s*$/, '\n\n[[kv_namespaces]]\n') +
    `binding = "${binding}"\nid = "${id}"\n`;
}

function tieneKvConId(toml, binding) {
  const bloque = kvBloquesPorBinding(toml)[binding];
  return bloque !== undefined && /id\s*=\s*"[^"]+"/.test(bloque);
}

// ---------------------------------------------------------------------------
// Confirmation + wrangler object operations
// ---------------------------------------------------------------------------
function confirmar(pregunta) {
  if (FLAGS.yes) return Promise.resolve(true);
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
  return new Promise((done) => {
    rl.question(`${pregunta} [y/N] `, (r) => { rl.close(); done(/^y(es)?$/i.test(r.trim())); });
  });
}

function listarBases() {
  const out = wrangler('d1 list --json', { allowFail: true });
  if (!out) return [];
  try {
    const json = JSON.parse(out);
    const arr = Array.isArray(json) ? json : (json.result ?? []);
    return arr.map((d) => ({ uuid: d.uuid ?? d.database_id ?? d.id, name: d.name }));
  } catch { return []; }
}

function crearBase(nombre) {
  const out = wrangler(`d1 create ${guard('database name', nombre, 'name')}`);
  const m =
    out.match(/database_id\s*=\s*"([^"]+)"/) ||
    out.match(/"database_id"\s*:\s*"([^"]+)"/) ||
    out.match(/uuid\s*=\s*"([^"]+)"/i) ||
    out.match(/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/i);
  if (!m) throw new Error(`could not extract database_id from:\n${out}`);
  return m[1];
}

function crearNamespace(binding) {
  const out = wrangler(`kv namespace create ${guard('KV binding', binding, 'name')}`);
  const m =
    out.match(/id\s*=\s*"([^"]+)"/) ||
    out.match(/"id"\s*:\s*"([^"]+)"/) ||
    out.match(/([0-9a-f]{32})/i);
  if (!m) throw new Error(`could not extract KV id for ${binding} from:\n${out}`);
  return m[1];
}

function nuevaKey(prefix) {
  return `${prefix}${crypto.randomBytes(9).toString('hex')}`;
}

function leerJson(p, fallback) {
  if (!existsSync(p)) return fallback;
  try { return JSON.parse(readFileSync(p, 'utf8')); }
  catch (e) { throw new Error(`${p} unreadable: ${e.message}`); }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
async function main() {
  WRANGLER_CMD = detectarWrangler();
  const cfg = loadConfig();
  const toml = leerToml();

  const dbName = valorEnBloque(toml, 'd1_databases', 'database_name');
  let dbId = valorEnBloque(toml, 'd1_databases', 'database_id');
  if (!dbName) throw new Error('no [[d1_databases]] with database_name in wrangler.toml');

  const tenants = leerJson(join(ROOT, cfg.tenantsFile), {});
  const slug = (tenants.tenants?.[0]?.slug) ?? cfg.defaultTenant;
  const keysPath = join(ROOT, cfg.keysFile);
  const saved = leerJson(keysPath, {});

  console.log(`worker dir : ${ROOT}`);
  console.log(`config     : ${existsSync(CONFIG_PATH) ? CONFIG_PATH : '(defaults)'}`);
  console.log('\n=== Plan ===');
  console.log(`  D1 database (${cfg.d1Binding ?? 'binding DB'}): ${dbName}`);
  console.log(`  database_id in TOML : ${dbId || '(empty)'}`);
  for (const b of cfg.kvNamespaces) {
    console.log(`  KV ${b} : ${tieneKvConId(toml, b) ? 'id present' : 'MISSING (will create)'}`);
  }
  console.log(`  schema files        : ${FLAGS.skipSchema ? 'SKIPPED' : cfg.schemaFiles.join(', ')}`);
  console.log(`  tenant key          : "${slug}" (prefix "${cfg.keyPrefix}") -> ${cfg.keysFile}`);

  if (FLAGS.dryRun) { console.log('\n--dry-run: nothing touched.'); return; }
  if (!(await confirmar('Continue?'))) { console.log('Cancelled.'); return; }

  // --- 1) D1 ---------------------------------------------------------------
  const bases = listarBases();
  let base = bases.find((b) => b.name === dbName);
  let tomlNuevo = toml;

  if (!base) {
    if (FLAGS.noCreate) {
      throw new Error(`database "${dbName}" not in account and --no-create given.\n  Create it: ${WRANGLER_CMD} d1 create ${dbName}`);
    }
    console.log(`\nDatabase "${dbName}" does not exist in this account.`);
    if (!(await confirmar(`Create D1 "${dbName}"?`))) {
      throw new Error('database missing and creation cancelled.');
    }
    dbId = crearBase(dbName);
    console.log(`Created: ${dbName} -> ${dbId}`);
  } else {
    dbId = base.uuid;
    console.log(`\nExisting database: ${dbName} -> ${dbId}`);
  }

  if (dbId !== valorEnBloque(tomlNuevo, 'd1_databases', 'database_id')) {
    tomlNuevo = reemplazarEnBloque(tomlNuevo, 'd1_databases', 'database_id', dbId);
    escribir(join(ROOT, 'wrangler.toml'), tomlNuevo);
    console.log('wrangler.toml updated with database_id.');
  }

  // --- 2) Schema -----------------------------------------------------------
  if (!FLAGS.skipSchema) {
    for (const f of cfg.schemaFiles) {
      console.log(`\nApplying ${f} (idempotent)...`);
      wrangler(`d1 execute ${guard('database name', dbName, 'name')} --file=./${guard('schema file', f, 'name')} --remote --yes`);
    }
    console.log('Schema applied.');
  }

  // --- 3) KV namespaces ----------------------------------------------------
  if (!FLAGS.noKv) {
    for (const binding of cfg.kvNamespaces) {
      if (tieneKvConId(leerToml(), binding)) {
        console.log(`\nKV ${binding}: keeping existing id (check it belongs to this account).`);
        continue;
      }
      console.log(`\nKV ${binding}: no id in wrangler.toml.`);
      if (!(await confirmar(`Create KV namespace "${binding}"?`))) {
        console.log(`   Left without id: deploy will fail until you set it.`);
        continue;
      }
      const id = crearNamespace(binding);
      escribir(join(ROOT, 'wrangler.toml'), ponerKvId(leerToml(), binding, id));
      console.log(`KV ${binding} created: ${id} (written to wrangler.toml)`);
    }
  }

  // --- 4) Sync key ---------------------------------------------------------
  let key = saved[slug]?.syncKey ?? saved[slug]?.schoolKey;
  if (!key) {
    key = nuevaKey(cfg.keyPrefix);
    console.log(`\nNew sync key for "${slug}": ${key}`);
  } else {
    console.log(`\nExisting key for "${slug}" (kept): ${key}`);
  }

  const routingId = kvBloquesPorBinding(leerToml())[cfg.routingKv]?.match(/id\s*=\s*"([^"]+)"/)?.[1];
  if (routingId) {
    wrangler(`kv key put --namespace-id=${guard('routing KV id', routingId, 'id')} "${guard('sync key', key, 'key')}" ${guard('database id', dbId, 'id')} --remote`);
    console.log(`Registered in KV ${cfg.routingKv}.`);
  } else {
    console.log(`WARNING: no id for KV ${cfg.routingKv}: key NOT registered.`);
  }

  saved[slug] = { ...(saved[slug] || {}), name: slug, schoolKey: key, databaseId: dbId };
  escribir(keysPath, JSON.stringify(saved, null, 2));
  console.log(`${cfg.keysFile} updated (credentials: keep it out of git).`);

  // --- 5) Optional deploy --------------------------------------------------
  if (FLAGS.deploy) {
    console.log('\nDeploying worker...');
    wrangler('deploy');
    console.log('Worker deployed.');
  }

  console.log('\n===================================================');
  console.log('Summary');
  console.log('===================================================');
  console.log(`  D1        : ${dbName} (${dbId})`);
  console.log(`  sync key  : ${key}`);
  console.log(`  next step : node generate-key.mjs --dir ${ROOT}  (issue the .key file)`);
}

const invokedDirectly = process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1]);
if (invokedDirectly) {
  main().catch((e) => { console.error(`\nERROR: ${e.message}`); process.exit(1); });
}

// Exported so the TOML helpers can be unit-tested.
export { bloqueTabla, valorEnBloque, reemplazarEnBloque, kvBloquesPorBinding, ponerKvId };

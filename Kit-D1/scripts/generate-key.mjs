#!/usr/bin/env node
/**
 * generate-key.mjs — issue a `.key` connection file for a project using this
 * kit, from the keys registered by provision.mjs.
 *
 * The `.key` file is a one-object JSON the app imports (the client stores
 * exactly these two values, never more):
 *
 *   { "workerUrl": "https://<worker>.<subdomain>.workers.dev", "schoolKey": "app_xxxx" }
 *
 * Normal mode: export an already-provisioned tenant's key (re-issue the file
 * for another device). `--nueva`: mint a new key, register it in the routing
 * KV pointing at the SAME database and update the keys file. Rotating does
 * NOT revoke the old key — devices holding the old `.key` keep syncing.
 *
 * Usage (from anywhere; the worker directory is required):
 *   node generate-key.mjs --dir ../myapp/cloudflare
 *   node generate-key.mjs --dir <workerDir> --slug otherTenant
 *   node generate-key.mjs --dir <workerDir> --out ../share.key
 *   node generate-key.mjs --dir <workerDir> --url https://x.example.com  # force URL
 *   node generate-key.mjs --dir <workerDir> --key app_abc123             # explicit, no KV
 *   node generate-key.mjs --dir <workerDir> --nueva                      # mint + register
 *   node generate-key.mjs --dir <workerDir> --solo-json                  # stdout, no file
 *
 * Config: same `sync-project.json` as provision.mjs (keyPrefix, routingKv,
 * keysFile, namespace). URL resolution order: --url → config.workerUrl →
 * `wrangler deployments list`. Requires wrangler logged in only for --nueva
 * or when the URL must be discovered.
 */

import { execSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import crypto from 'node:crypto';

const __dirname = dirname(fileURLToPath(import.meta.url));

function arg(name, alias = null) {
  const argv = process.argv.slice(2);
  for (const key of [name, alias]) {
    if (!key) continue;
    const i = argv.indexOf(`--${key}`);
    if (i !== -1 && argv[i + 1] && !argv[i + 1].startsWith('--')) return argv[i + 1];
  }
  return null;
}
const flag = (name) => process.argv.includes(`--${name}`);

const ROOT = resolve(arg('dir', 'worker-dir') || process.cwd());
const CONFIG_PATH = resolve(arg('config') || join(ROOT, 'sync-project.json'));
const slugArg = arg('slug');
const outArg = arg('out');
const urlArg = arg('url');
const keyArg = arg('key');
const NUEVA = flag('nueva');
const SOLO_JSON = flag('solo-json');

// Allowlist: these reach a shell string in registrarEnKV / discoverUrl.
const SAFE_KEY = /^[A-Za-z0-9_]+$/;
const SAFE_ID = /^[0-9a-fA-F-]{8,64}$/;

function loadConfig() {
  const defaults = {
    namespace: 'sync', keyPrefix: 'sync_', routingKv: 'SCHOOL_ROUTING',
    keysFile: 'sync-keys.json', workerUrl: null,
  };
  if (!existsSync(CONFIG_PATH)) return defaults;
  return { ...defaults, ...JSON.parse(readFileSync(CONFIG_PATH, 'utf8')) };
}

function leerKeys(file) {
  const p = join(ROOT, file);
  if (!existsSync(p)) return {};
  try { return JSON.parse(readFileSync(p, 'utf8')); }
  catch (e) { throw new Error(`${file} unreadable: ${e.message}`); }
}

function nombreWorker() {
  try {
    const raw = readFileSync(join(ROOT, 'wrangler.toml'), 'utf8');
    return raw.match(/^\s*name\s*=\s*"([^"]+)"/m)?.[1] || null;
  } catch { return null; }
}

function kvRoutingId(cfg) {
  try {
    const raw = readFileSync(join(ROOT, 'wrangler.toml'), 'utf8');
    const re = new RegExp(`\\[\\[kv_namespaces\\]\\]([^\\[]*)(?:\\[|$)`, 'g');
    let m;
    while ((m = re.exec(raw)) !== null) {
      if (m[1].includes(`binding = "${cfg.routingKv}"`) || m[1].includes(`binding = '${cfg.routingKv}'`)) {
        return m[1].match(/id\s*=\s*"([^"]+)"/)?.[1] || null;
      }
    }
    return null;
  } catch { return null; }
}

function normalizarUrl(u) {
  let s = String(u).trim();
  if (!s) throw new Error('empty workerUrl');
  if (!/^https?:\/\//i.test(s)) s = `https://${s}`;
  return s.replace(/\/+$/, '');
}

function resolverWorkerUrl(cfg) {
  if (urlArg) return normalizarUrl(urlArg);
  if (cfg.workerUrl) return normalizarUrl(cfg.workerUrl);

  const nombre = nombreWorker();
  if (nombre) {
    try {
      const out = execSync('npx --yes wrangler deployments list --json', {
        cwd: ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
      });
      const m = out.match(new RegExp(`https://${nombre.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}[^"\\s]*workers\\.dev`));
      if (m?.[0]) return normalizarUrl(m[0]);
    } catch { /* no wrangler session */ }
  }

  throw new Error(
    'could not determine the worker URL.\n' +
    '  Pass it: --url https://<worker>.<subdomain>.workers.dev\n' +
    '  or set "workerUrl" in sync-project.json.'
  );
}

function nuevaKey(prefix) {
  return `${prefix}${crypto.randomBytes(9).toString('hex')}`;
}

function registrarEnKV(cfg, key, databaseId) {
  const kvId = kvRoutingId(cfg);
  if (!kvId) throw new Error(`no id for KV ${cfg.routingKv} in wrangler.toml`);
  if (!SAFE_KEY.test(key)) throw new Error('refusing to register an unsafe key');
  if (!SAFE_ID.test(kvId) || !SAFE_ID.test(String(databaseId))) {
    throw new Error('refusing to run wrangler with unsafe KV/database id');
  }
  execSync(
    `npx --yes wrangler kv key put --namespace-id=${kvId} "${key}" ${databaseId} --remote`,
    { cwd: ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] },
  );
}

function main() {
  const cfg = loadConfig();
  const guardado = leerKeys(cfg.keysFile);
  const slugs = Object.keys(guardado);

  let slug = slugArg;
  if (!slug) {
    if (slugs.length === 1) slug = slugs[0];
    else if (slugs.length === 0) slug = cfg.defaultTenant || 'default';
    else {
      console.error(`Several tenants in ${cfg.keysFile}. Pick one:`);
      for (const s of slugs) console.error(`  --slug ${s}`);
      process.exit(1);
    }
  }

  const registro = guardado[slug];
  if (!registro && !keyArg && !NUEVA) {
    throw new Error(
      `"${slug}" not in ${cfg.keysFile}.\n` +
      '  Provision first: node provision.mjs --dir ' + ROOT +
      '\n  or pass an explicit key: --key ' + cfg.keyPrefix + 'xxxx'
    );
  }

  let schoolKey = keyArg || registro?.schoolKey || null;
  const databaseId = registro?.databaseId || null;

  if (NUEVA) {
    schoolKey = nuevaKey(cfg.keyPrefix);
    if (!databaseId) {
      throw new Error(
        `"${slug}" has no databaseId in ${cfg.keysFile}: nowhere to point the new key.\n` +
        '  Run provision.mjs first.'
      );
    }
    console.log(`New key for "${slug}": ${schoolKey}`);
    console.log(`Registering in KV ${cfg.routingKv}...`);
    registrarEnKV(cfg, schoolKey, databaseId);
    guardado[slug] = { ...(registro || {}), name: registro?.name || slug, schoolKey, databaseId };
    writeFileSync(join(ROOT, cfg.keysFile), JSON.stringify(guardado, null, 2), 'utf8');
    console.log(`${cfg.keysFile} updated (credentials: keep it out of git).`);
  }

  if (!schoolKey) throw new Error('could not determine the sync key');

  const workerUrl = resolverWorkerUrl(cfg);
  const contenido = JSON.stringify({ workerUrl, schoolKey }, null, 2) + '\n';

  if (SOLO_JSON) { process.stdout.write(contenido); return; }

  const destino = resolve(outArg || join(ROOT, '..', `${cfg.namespace}-${slug}.key`));
  writeFileSync(destino, contenido, 'utf8');

  console.log('\n===================================================');
  console.log('Connection file generated');
  console.log('===================================================');
  console.log(`  tenant : ${slug}`);
  console.log(`  worker : ${workerUrl}`);
  console.log(`  key    : ${schoolKey}`);
  console.log(`  file   : ${destino}`);
  console.log('\nImport it in the app (worker URL + key fields, or the .key picker).');
  console.log('WARNING: the .key is a credential — never commit it or send it over chat.');
}

try {
  main();
} catch (e) {
  console.error(`\nERROR: ${e.message}`);
  process.exit(1);
}

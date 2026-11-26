#!/usr/bin/env node
/**
 * publish-update.mjs — publish a GymPro update to a project's RELEASES KV.
 *
 *   node scripts/publish-update.mjs --dir worker/instances/gympro \
 *     --manifest update.json --file path/to/GymPro-Setup.exe
 *
 * The manifest is uploaded as `updates/latest` (served by /updates/latest)
 * and the artifact as `file:<name>` (served by /release/<name>). When the
 * manifest omits `url`/`sha256` they are filled from `--file` and the
 * instance's `workerUrl`.
 *
 * Requires a logged-in wrangler (`npx wrangler whoami`).
 */
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, statSync, writeFileSync, mkdtempSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const KIT = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function arg(name, fallback = '') {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
}

const projectDir = resolve(KIT, arg('dir', 'worker/instances/gympro'));
const manifestPath = arg('manifest');
const artifactPath = arg('file');

if (!manifestPath) {
  console.error('usage: node scripts/publish-update.mjs --dir <instance> --manifest <file> [--file <artifact>]');
  process.exit(1);
}

const projectFile = join(projectDir, 'sync-project.json');
const wranglerFile = join(projectDir, 'wrangler.toml');
for (const f of [projectFile, wranglerFile, manifestPath]) {
  if (!existsSync(f)) {
    console.error('missing file: ' + f);
    process.exit(1);
  }
}

const project = JSON.parse(readFileSync(projectFile, 'utf8'));
const workerUrl = String(project.workerUrl || '').replace(/\/+$/, '');

const toml = readFileSync(wranglerFile, 'utf8');
const releasesBlock = toml.match(/binding\s*=\s*"RELEASES"[\s\S]*?id\s*=\s*"([^"]+)"/);
if (!releasesBlock) {
  console.error('wrangler.toml has no RELEASES KV binding');
  process.exit(1);
}
const namespaceId = releasesBlock[1];

const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
if (!manifest.version) {
  console.error('manifest.version is required');
  process.exit(1);
}

const wranglerBin = join(projectDir, 'node_modules', 'wrangler', 'bin', 'wrangler.js');
if (!existsSync(wranglerBin)) {
  console.error('wrangler is not installed in ' + projectDir + ' (run npm install there)');
  process.exit(1);
}
function kvPut(key, path, metadata) {
  const args = [wranglerBin, 'kv', 'key', 'put', key, '--namespace-id', namespaceId, '--path', path, '--remote'];
  if (metadata) args.push('--metadata', JSON.stringify(metadata));
  execFileSync(process.execPath, args, { cwd: projectDir, stdio: 'inherit' });
}

if (artifactPath) {
  const artifact = resolve(artifactPath);
  if (!existsSync(artifact)) {
    console.error('missing artifact: ' + artifact);
    process.exit(1);
  }
  const name = basename(artifact);
  const bytes = readFileSync(artifact);
  const sha256 = createHash('sha256').update(bytes).digest('hex');
  const size = statSync(artifact).size;

  kvPut('file:' + name, artifact, { size });
  manifest.url = manifest.url || (workerUrl ? workerUrl + '/release/' + name : '');
  manifest.sha256 = manifest.sha256 || sha256;
  console.log('artifact uploaded: ' + name + ' (' + size + ' bytes, sha256 ' + sha256 + ')');
}

if (!manifest.url) {
  console.error('manifest.url is empty and no --file was supplied');
  process.exit(1);
}

const dir = mkdtempSync(join(tmpdir(), 'gympro-update-'));
const out = join(dir, 'updates-latest.json');
writeFileSync(out, JSON.stringify(manifest, null, 2));
kvPut('updates/latest', out);
console.log('manifest published: v' + manifest.version + ' -> ' + manifest.url);

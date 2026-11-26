#!/usr/bin/env node
/**
 * admin.mjs — operator CLI for a deployed worker of this kit.
 *
 * Talks to the worker's own `/admin/*` endpoints (defined by the template's
 * auth module), so it works against any project instance. The caller must
 * present a Google ID token of an active administrator; the token travels in
 * the `X-Auth-Token` header and is only ever read from the environment (never
 * from argv, where it would show up in process listings and shell history).
 *
 * Usage:
 *   node admin.mjs --dir <workerDir> list
 *   node admin.mjs --dir <workerDir> suspend <email>
 *   node admin.mjs --dir <workerDir> restore <email>
 *   node admin.mjs --dir <workerDir> delete-data <email>
 *   node admin.mjs --dir <workerDir> delete-account <email>
 *
 * Worker URL resolution order: --worker → $SYNC_WORKER_URL →
 * `workerUrl` in <workerDir>/sync-project.json.
 * Credentials: $SYNC_ADMIN_ID_TOKEN (required for every command but --help).
 *
 * `delete-data` and `delete-account` are irreversible and ask for a typed
 * confirmation of the target email.
 */

import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { createInterface } from 'node:readline/promises';
import { stdin, stdout } from 'node:process';

const argv = process.argv.slice(2);

function flag(name) {
  const i = argv.indexOf(`--${name}`);
  return i !== -1 && argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[i + 1] : null;
}

const positional = argv.filter((a, i) => !a.startsWith('--') && argv[i - 1] !== '--dir' && argv[i - 1] !== '--worker');
const [command, email] = positional;

const USAGE = `Usage:
  node admin.mjs --dir <workerDir> list
  node admin.mjs --dir <workerDir> suspend <email>
  node admin.mjs --dir <workerDir> restore <email>
  node admin.mjs --dir <workerDir> delete-data <email>
  node admin.mjs --dir <workerDir> delete-account <email>

  --dir <workerDir>   instance/worker directory holding sync-project.json
  --worker <url>      override the worker URL from the project config

Set SYNC_ADMIN_ID_TOKEN to a Google ID token for an active administrator.
Optionally set SYNC_WORKER_URL instead of --dir/--worker.`;

function workerUrlFrom(dir) {
  const configPath = join(dir, 'sync-project.json');
  if (!existsSync(configPath)) return null;
  try {
    return JSON.parse(readFileSync(configPath, 'utf8')).workerUrl || null;
  } catch (error) {
    throw new Error(`cannot read ${configPath}: ${error.message}`);
  }
}

function normalizeUrl(raw) {
  const url = String(raw).replace(/\/+$/, '');
  // Rejected before it reaches fetch: a bare string from config/env could
  // otherwise carry credentials or a scheme-less host.
  if (!/^https:\/\/[A-Za-z0-9._-]+(?::\d+)?$/.test(url)) {
    throw new Error(`worker URL must be https://<host>[:port] without a path, got: ${raw}`);
  }
  return url;
}

const dirArg = flag('dir');
const workerDir = dirArg ? resolve(dirArg) : process.cwd();
const token = process.env.SYNC_ADMIN_ID_TOKEN;

async function request(workerUrl, path, body = {}) {
  const response = await fetch(`${workerUrl}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Auth-Token': token },
    body: JSON.stringify(body),
  });
  const result = await response.json().catch(() => ({}));
  if (!response.ok || result.ok === false) {
    throw new Error(result.error || `worker request failed (${response.status})`);
  }
  return result;
}

async function confirmEmail(target, action) {
  const readline = createInterface({ input: stdin, output: stdout });
  try {
    const entered = (await readline.question(`${action} ${target}. Type the email to confirm: `))
      .trim().toLowerCase();
    if (entered !== target.toLowerCase()) {
      throw new Error('confirmation did not match the target email; nothing was changed');
    }
  } finally {
    readline.close();
  }
}

async function main() {
  if (!command || command === '--help' || command === '-h') {
    console.log(USAGE);
    process.exitCode = command ? 0 : 1;
    return;
  }

  const rawUrl = flag('worker') || process.env.SYNC_WORKER_URL || workerUrlFrom(workerDir);
  if (!rawUrl) {
    throw new Error(
      'no worker URL: pass --worker, set SYNC_WORKER_URL, or point --dir at a ' +
      'project directory whose sync-project.json defines workerUrl'
    );
  }
  const workerUrl = normalizeUrl(rawUrl);

  if (!token) throw new Error('set SYNC_ADMIN_ID_TOKEN before running admin commands');
  console.log(`worker: ${workerUrl}`);

  if (command === 'list' && !email) {
    const { users = [] } = await request(workerUrl, '/admin/users');
    if (users.length === 0) {
      console.log('No registered accounts found.');
    } else {
      console.table(users.map(({ email: userEmail, nombre, rol, creado, suspendido }) => ({
        email: userEmail,
        name: nombre,
        role: rol,
        created: creado,
        suspended: suspendido,
      })));
    }
    return;
  }

  if (['suspend', 'restore', 'delete-data', 'delete-account'].includes(command) && email) {
    const target = email.trim().toLowerCase();
    if (!target.includes('@')) throw new Error('provide a valid email address');

    if (command === 'suspend' || command === 'restore') {
      const result = await request(workerUrl, '/admin/suspend', {
        email: target,
        suspend: command === 'suspend',
      });
      console.log(`Account ${target} ${command === 'suspend' ? 'suspended' : 'restored'}.`);
      console.log(JSON.stringify(result));
      return;
    }

    const action = command === 'delete-account'
      ? 'Permanently delete the account and all its data for'
      : 'Delete all D1 data for';
    await confirmEmail(target, action);
    const result = command === 'delete-account'
      ? await request(workerUrl, '/admin/delete-account', { email: target, confirmEmail: target })
      : await request(workerUrl, '/admin/delete-data', { email: target });
    console.log(JSON.stringify(result, null, 2));
    return;
  }

  console.log(USAGE);
  process.exitCode = 1;
}

main().catch((error) => {
  console.error(`admin command failed: ${error.message}`);
  process.exitCode = 1;
});

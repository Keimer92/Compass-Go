/**
 * sync-kit worker — router.
 *
 * Mounts the engine always and each optional module only when its bindings
 * are declared in wrangler.toml:
 *   USERS       -> auth + /nube/* + /usage caching
 *   RELEASES    -> /release.json + /release/<file>
 *   SYNC_HUB    -> /ws (Durable Object hub)
 *   deepLinkScheme (sync.config.json) -> /conectar
 *
 * The instance directory supplies `sync.schema.json` (generated from the
 * project's Go registry), an optional `sync.config.json` and an optional
 * `instance.ts` with the project's legacy D1 migrations.
 */
import type { Env, KitConfig } from './types';
import { CORS_HEADERS, ensureSchema, handleEngine } from './engine';
import { handleHubFetch } from './hub';
import { handleReleases } from './releases';
import { handleAuth } from './auth';
import { handleProvision } from './provision';

// Instance glue (present in every project instantiated from this template).
import CONFIG from '../sync.config.json';
import { legacyMigrations } from '../instance';

const config = CONFIG as unknown as KitConfig;

async function route(req: Request, env: Env): Promise<Response> {
  const url = new URL(req.url);
  const path = url.pathname;

  // ── public routes (no school key) ──
  const provisionRes = await handleProvision(env, req, url, config);
  if (provisionRes) return provisionRes;

  const releasesRes = await handleReleases(path, req.method, env);
  if (releasesRes) return releasesRes;

  // Auth/admin/nube routes answer before ensureSchema: they never touch D1
  // except the legacy claim, and the WebView may call them on a cold DB.
  if (env.USERS) {
    const authRes = await handleAuth(env, req, url, config, () => ensureSchema(env, legacyMigrations));
    if (authRes) return authRes;
  }

  // Usage bypasses ensureSchema (works while the D1 read quota is exhausted).
  if (path === '/usage' && env.USERS) {
    return (await handleEngine(env, req, path, config)) ?? notFound();
  }

  await ensureSchema(env, legacyMigrations);

  // ── WebSocket upgrade ──
  if (env.SYNC_HUB) {
    const wsRes = await handleHubFetch(env, req, url);
    if (wsRes) return wsRes;
  }

  const engineRes = await handleEngine(env, req, path, config);
  if (engineRes) return engineRes;

  return notFound();
}

function notFound(): Response {
  return new Response(JSON.stringify({ error: 'Not found' }), {
    status: 404, headers: { 'Content-Type': 'application/json' },
  });
}

export default {
  async fetch(req: Request, env: Env, _ctx: ExecutionContext): Promise<Response> {
    if (req.method === 'OPTIONS') {
      return new Response(null, { status: 204, headers: CORS_HEADERS });
    }
    let res: Response;
    try {
      res = await route(req, env);
    } catch (e: any) {
      res = new Response(JSON.stringify({ ok: false, error: e?.message || String(e) }), {
        status: 400, headers: { 'Content-Type': 'application/json' },
      });
    }
    // CORS headers on every response (WebView fetches).
    const out = new Response(res.body, res);
    for (const [k, v] of Object.entries(CORS_HEADERS)) out.headers.set(k, v);
    return out;
  },
};

export { SyncHub } from './hub';

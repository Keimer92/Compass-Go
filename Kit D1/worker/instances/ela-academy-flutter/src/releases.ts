/**
 * Optional updater module: release manifest + artifacts served from the KV
 * namespace bound as RELEASES (uploaded by the project's publish script).
 *
 * Two feeds share the same namespace:
 *   - /release.json + /release/<file>  legacy feed for School Tracker.
 *   - /updates/latest                  generic update manifest for app updaters
 *                                      (GymPro): 200 + JSON when the published
 *                                      version is newer, 204 when the client is
 *                                      already up to date, 404 when nothing has
 *                                      been published yet.
 *
 * Rules inherited from the original: ship the raw executable, never the
 * installer, and KV values must stay under 25 MB.
 */
import type { Env } from './types';
import { json } from './engine';

interface UpdateManifest {
  version?: string;
  notes?: string;
  url?: string;
  sha256?: string;
  mandatory?: boolean;
  platforms?: Record<string, Partial<UpdateManifest>>;
}

export async function handleReleases(path: string, method: string, env: Env, url?: URL): Promise<Response | null> {
  if (!env.RELEASES || method !== 'GET') return null;

  if (path === '/release.json') {
    const raw = await env.RELEASES.get('manifest');
    if (!raw) return json({ error: 'No release published yet' }, 404);
    return new Response(raw, {
      headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store' },
    });
  }

  if (path === '/updates/latest') {
    const raw = await env.RELEASES.get('updates/latest');
    if (!raw) return json({ error: 'No update manifest published' }, 404);

    let manifest: UpdateManifest;
    try {
      manifest = JSON.parse(raw) as UpdateManifest;
    } catch {
      return json({ error: 'Invalid update manifest' }, 500);
    }

    // Optional per-platform artifacts: `platforms.windows` overrides the
    // top-level fields, so one manifest can describe every target.
    const platform = url?.searchParams.get('platform') ?? '';
    const override = platform ? manifest.platforms?.[platform] : undefined;
    if (override) manifest = { ...manifest, ...override };
    delete manifest.platforms;

    const current = url?.searchParams.get('version') ?? '';
    if (current && compareVersions(String(manifest.version ?? ''), current) <= 0) {
      return new Response(null, { status: 204 });
    }

    return new Response(JSON.stringify(manifest), {
      headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store' },
    });
  }

  if (path.startsWith('/release/')) {
    const nombre = decodeURIComponent(path.slice('/release/'.length));
    // Plain file names only: the route cannot be walked.
    if (!/^[A-Za-z0-9._-]+$/.test(nombre)) return json({ error: 'invalid file name' }, 400);
    const objeto = await env.RELEASES.getWithMetadata('file:' + nombre, 'stream');
    if (!objeto.value) return json({ error: 'not found' }, 404);
    // `metadata` is null when the key was uploaded without metadata: then
    // there is no Content-Length (chunked). Updaters use the MANIFEST size.
    const meta = (objeto.metadata ?? {}) as { size?: number };
    const headers: Record<string, string> = {
      'Content-Type': 'application/octet-stream',
      'Content-Disposition': 'attachment; filename="' + nombre + '"',
      'Cache-Control': 'public, max-age=3600',
    };
    if (typeof meta.size === 'number' && meta.size > 0) headers['Content-Length'] = String(meta.size);
    return new Response(objeto.value, { headers });
  }

  return null;
}

/** Simple semver comparison (1.2.3, v1.2.3, 1.2.3-beta). >0 when a is newer. */
function compareVersions(a: string, b: string): number {
  const pa = parseVersion(a);
  const pb = parseVersion(b);
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] > pb[i] ? 1 : -1;
  }
  return 0;
}

function parseVersion(v: string): [number, number, number] {
  const out: [number, number, number] = [0, 0, 0];
  const clean = v.trim().replace(/^v/i, '').split(/[-+]/)[0];
  clean.split('.').slice(0, 3).forEach((part, i) => {
    const n = Number.parseInt(part, 10);
    out[i] = Number.isFinite(n) ? n : 0;
  });
  return out;
}

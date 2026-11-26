/**
 * Optional updater module: release manifest + artifacts served from the KV
 * namespace bound as RELEASES (uploaded by the project's publish script).
 *
 * Rules inherited from the original: ship the raw executable, never the
 * installer, and KV values must stay under 25 MB.
 */
import type { Env } from './types';
import { json } from './engine';

export async function handleReleases(path: string, method: string, env: Env): Promise<Response | null> {
  if (!env.RELEASES || method !== 'GET') return null;

  if (path === '/release.json') {
    const raw = await env.RELEASES.get('manifest');
    if (!raw) return json({ error: 'No release published yet' }, 404);
    return new Response(raw, {
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

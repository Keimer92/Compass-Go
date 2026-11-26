/**
 * Optional provisioning/entry-point helpers:
 *   - `/ping`  — public health check (no key).
 *   - `/conectar` — QR landing: phone cameras cannot open custom schemes, so
 *     the QR points at https://<worker>/conectar?key=… and this route 302s
 *     to `<deepLinkScheme>://conectar?key=…&worker=…`.
 */
import type { Env, KitConfig } from './types';
import { jsonCors } from './engine';

export async function handleProvision(
  _env: Env, req: Request, url: URL, config: KitConfig,
): Promise<Response | null> {
  const path = url.pathname;

  if (req.method === 'GET' && path === '/ping') {
    return jsonCors({ ok: true, serverTime: new Date().toISOString() });
  }

  if (req.method === 'GET' && path === '/conectar') {
    if (!config.deepLinkScheme) return null;
    const key = (url.searchParams.get('key') || '').trim();
    // Only key-shaped strings: the route cannot be used to redirect anywhere.
    if (!/^[A-Za-z0-9_-]{8,64}$/.test(key)) {
      return new Response('Invalid key', { status: 400 });
    }
    const destino = `${config.deepLinkScheme}://conectar?key=` +
      encodeURIComponent(key) + '&worker=' + encodeURIComponent(url.origin);
    return new Response(null, { status: 302, headers: { Location: destino, 'Cache-Control': 'no-store' } });
  }

  return null;
}

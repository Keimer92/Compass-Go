/**
 * Optional WebSocket change-hub: one Durable Object per school key. It only
 * relays "there were changes" hints; clients still pull over HTTP.
 *
 * Mounted when the wrangler.toml declares a durable_namespaces binding
 * named SYNC_HUB.
 */
import type { Env } from './types';

export class SyncHub {
  private state: DurableObjectState;

  constructor(state: DurableObjectState, _env: Env) {
    this.state = state;
    try {
      this.state.setWebSocketAutoResponse(new WebSocketRequestResponsePair('ping', 'pong'));
    } catch { /* unsupported runtime */ }
  }

  async fetch(req: Request): Promise<Response> {
    const url = new URL(req.url);

    if (url.pathname === '/connect') {
      const pair = new WebSocketPair();
      this.state.acceptWebSocket(pair[1]);
      return new Response(null, { status: 101, webSocket: pair[0] });
    }

    if (url.pathname === '/broadcast') {
      const body: any = await req.json().catch(() => ({}));
      const msg = JSON.stringify({ type: 'changes', from: body.from || '', ts: Date.now() });
      let notified = 0;
      for (const ws of this.state.getWebSockets()) {
        try { ws.send(msg); notified++; } catch { /* socket already dead */ }
      }
      return new Response(JSON.stringify({ ok: true, notified }), {
        headers: { 'Content-Type': 'application/json' },
      });
    }

    if (url.pathname === '/stats') {
      return new Response(JSON.stringify({ ok: true, clients: this.state.getWebSockets().length }), {
        headers: { 'Content-Type': 'application/json' },
      });
    }

    return new Response(JSON.stringify({ error: 'Not found' }), { status: 404 });
  }
}

/** `/ws` handler for the main router; returns null when the hub is unbound. */
export async function handleHubFetch(env: Env, req: Request, url: URL): Promise<Response | null> {
  if (!env.SYNC_HUB || url.pathname !== '/ws') return null;
  const key = url.searchParams.get('key') || '';
  if (!key || !(await env.SCHOOL_ROUTING.get(key))) {
    return new Response(JSON.stringify({ ok: false, error: 'Invalid school key' }), {
      status: 403, headers: { 'Content-Type': 'application/json' },
    });
  }
  const stub = env.SYNC_HUB.get(env.SYNC_HUB.idFromName(key));
  const hubUrl = new URL(url.toString());
  hubUrl.pathname = '/connect';
  return stub.fetch(new Request(hubUrl.toString(), req));
}

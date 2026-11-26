/**
 * Optional cloud-auth module (the most brand-coupled piece): Google ID-token
 * login, email/password accounts, recovery codes, email OTP (Resend) and the
 * admin panel. Mounted only when the USERS KV binding exists.
 *
 * Project-specific strings arrive through `sync.config.json`:
 *   auth: { brand, resendFrom, recoveryPrefix }
 *
 * Invariants kept from the original:
 *   - the school key IS the credential; recovery codes are the backup and
 *     only their SHA-256 ever touches KV (single use);
 *   - the OTP/login/reset routes are per-IP rate limited;
 *   - the first admin can claim pre-namespaced rows exactly once
 *     (legacy_claimed marker).
 */
import type { Env, KitConfig } from './types';
import {
  BATCH_STMT, batchExec, json, jsonCors, nowSqlite, skOf,
} from './engine';
import { manifest } from './engine';

export interface AuthConfig {
  brand: string;
  resendFrom: string;
  recoveryPrefix: string;
}

export function authConfig(config: KitConfig): AuthConfig {
  const a = (config as any).auth || {};
  return {
    brand: a.brand || config.namespace || 'app',
    resendFrom: a.resendFrom || 'no-reply@example.com',
    recoveryPrefix: a.recoveryPrefix || 'RC',
  };
}

interface GoogleClaims {
  sub: string; email: string; email_verified?: boolean;
  name?: string; picture?: string; aud: string; iss: string; exp: number;
}

interface UserRecord {
  sub: string; email: string; nombre: string; foto: string;
  schoolKey: string; rol: 'admin' | 'user';
  creado: string; suspendido?: boolean; lastLoginAt?: string; passwordChangedAt?: string;
}

interface UserPasswordCred {
  email: string; hash: string; salt: string; schoolKey: string; creado: string;
}

function b64urlToBytes(s: string): Uint8Array {
  s = s.replace(/-/g, '+').replace(/_/g, '/');
  while (s.length % 4) s += '=';
  const bin = atob(s);
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

async function verifyGoogleIdToken(env: Env, idToken: string): Promise<GoogleClaims> {
  const parts = idToken.split('.');
  if (parts.length !== 3) throw new Error('Malformed ID token');
  const header = JSON.parse(new TextDecoder().decode(b64urlToBytes(parts[0])));
  if (header.alg !== 'RS256') throw new Error('Unsupported algorithm');
  const kid: string = header.kid;

  let jwks: any = (globalThis as any).__googleJwks;
  if (!jwks) {
    const res = await fetch('https://www.googleapis.com/oauth2/v3/certs');
    if (!res.ok) throw new Error('Could not fetch Google JWKS');
    jwks = await res.json();
    (globalThis as any).__googleJwks = jwks;
  }
  const jwk = jwks.keys.find((k: any) => k.kid === kid);
  if (!jwk) throw new Error('Unknown kid');

  const key = await crypto.subtle.importKey(
    'jwk', jwk, { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' }, false, ['verify'],
  );
  const signedPart = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  const valid = await crypto.subtle.verify('RSASSA-PKCS1-v1_5', key, b64urlToBytes(parts[2]), signedPart);
  if (!valid) throw new Error('Invalid ID token signature');

  const claims: GoogleClaims = JSON.parse(new TextDecoder().decode(b64urlToBytes(parts[1])));
  const now = Math.floor(Date.now() / 1000);
  if (claims.exp < now - 60) throw new Error('Expired ID token');
  if (!['accounts.google.com', 'https://accounts.google.com'].includes(claims.iss)) throw new Error('Invalid issuer');
  if (claims.aud !== env.GOOGLE_CLIENT_ID) throw new Error('Invalid audience');
  if (!claims.email || claims.email_verified === false) throw new Error('Email not verified');
  return claims;
}

function isAdminEmail(env: Env, email: string): boolean {
  return (env.ADMIN_EMAILS || '').split(',').map((e) => e.trim().toLowerCase()).includes(email.toLowerCase());
}

/** ALLOWED_EMAILS empty => any Google account may log in (each gets its key). */
function isAllowedEmail(env: Env, email: string): boolean {
  const list = (env.ALLOWED_EMAILS || '').split(',').map((e) => e.trim().toLowerCase()).filter(Boolean);
  if (list.length === 0) return true;
  return list.includes(email.toLowerCase());
}

function randomSchoolKey(): string {
  const bytes = new Uint8Array(18);
  crypto.getRandomValues(bytes);
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
}

// ── recovery codes ───────────────────────────────────────────────────────
// Alphabet excludes 0/O and 1/I so codes can be dictated over the phone.
const RECOVERY_ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';

function nuevoCodigoRecuperacion(prefix: string): string {
  const bytes = new Uint8Array(12);
  crypto.getRandomValues(bytes);
  const raw = Array.from(bytes).map((b) => RECOVERY_ALPHABET[b % RECOVERY_ALPHABET.length]).join('');
  return `${prefix}-${raw.slice(0, 4)}-${raw.slice(4, 8)}-${raw.slice(8, 12)}`;
}

function normalizarCodigo(codigo: string): string {
  return (codigo || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
}

async function hashCodigo(codigo: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(normalizarCodigo(codigo)));
  return Array.from(new Uint8Array(buf)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

/** Emits a fresh batch, deleting the previous one (regenerating revokes). */
async function emitirCodigos(env: Env, schoolKey: string, prefix: string, cantidad = 6): Promise<string[]> {
  const prev = await env.USERS!.get(`recoverymeta:${schoolKey}`);
  if (prev) {
    try {
      const hashes: string[] = JSON.parse(prev).hashes || [];
      for (const h of hashes) await env.USERS!.delete(`recovery:${h}`);
    } catch { /* corrupt meta: ignored */ }
  }
  const codigos: string[] = [];
  const hashes: string[] = [];
  for (let i = 0; i < cantidad; i++) {
    const codigo = nuevoCodigoRecuperacion(prefix);
    const hash = await hashCodigo(codigo);
    codigos.push(codigo);
    hashes.push(hash);
    await env.USERS!.put(`recovery:${hash}`, schoolKey);
  }
  await env.USERS!.put(`recoverymeta:${schoolKey}`, JSON.stringify({ hashes, creado: nowSqlite() }));
  return codigos;
}

async function consumeCodigo(env: Env, hash: string): Promise<void> {
  await env.USERS!.delete(`recovery:${hash}`);
}

async function hashPassword(password: string, saltHex: string): Promise<string> {
  const enc = new TextEncoder();
  const saltBytes = new Uint8Array(saltHex.match(/.{1,2}/g)?.map((b) => parseInt(b, 16)) || []);
  const keyMaterial = await crypto.subtle.importKey('raw', enc.encode(password), { name: 'PBKDF2' }, false, ['deriveBits']);
  const derived = await crypto.subtle.deriveBits(
    { name: 'PBKDF2', salt: saltBytes, iterations: 100000, hash: 'SHA-256' }, keyMaterial, 256,
  );
  return Array.from(new Uint8Array(derived)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

async function loadUserByEmail(env: Env, email: string): Promise<UserRecord | null> {
  const raw = await env.USERS!.get(`user:${email.toLowerCase()}`);
  return raw ? (JSON.parse(raw) as UserRecord) : null;
}

async function saveUser(env: Env, u: UserRecord): Promise<void> {
  await env.USERS!.put(`user:${u.email.toLowerCase()}`, JSON.stringify(u));
}

async function requireAdmin(env: Env, req: Request): Promise<UserRecord> {
  const token = req.headers.get('X-Auth-Token') || '';
  if (!token) throw new Error('Missing authentication');
  const claims = await verifyGoogleIdToken(env, token);
  const user = await loadUserByEmail(env, claims.email);
  if (!user || user.rol !== 'admin') throw new Error('Admins only');
  if (user.suspendido) throw new Error('Suspended account');
  return user;
}

async function purgeNamespace(env: Env, schoolKey: string): Promise<number> {
  const sk = await skOf(schoolKey);
  const statements = [
    ...manifest.tables.map((t) => ({ sql: `DELETE FROM ${t.name} WHERE sk = ?1`, params: [sk] })),
    { sql: 'DELETE FROM sync_tombstone WHERE sk = ?1', params: [sk] },
  ];
  let deletedRows = 0;
  for (let i = 0; i < statements.length; i += BATCH_STMT) {
    const results = await batchExec(env.DB, statements.slice(i, i + BATCH_STMT));
    deletedRows += results.reduce((total, r) => total + r.changes, 0);
  }
  return deletedRows;
}

async function sendResendOtpEmail(apiKey: string, brand: string, from: string, toEmail: string, otp: string): Promise<void> {
  const html = [
    `<div style="font-family:-apple-system,sans-serif;max-width:480px;margin:0 auto;padding:24px;border:1px solid #e5e5e5;border-radius:12px;background:#fff">`,
    `<h2 style="color:#111827;margin-top:0;font-size:20px">Account recovery - ${brand}</h2>`,
    `<p style="color:#4b5563;font-size:14px;line-height:1.5">You requested a code to reset your password.</p>`,
    `<div style="margin:24px 0;padding:16px;background:#f3f4f6;border-radius:8px;text-align:center">`,
    `<span style="font-size:32px;font-weight:700;letter-spacing:6px;color:#0a84ff;font-family:monospace">${otp}</span>`,
    `</div>`,
    `<p style="color:#6b7280;font-size:12px">Valid for 10 minutes. If you did not request it, ignore this message.</p>`,
    `</div>`,
  ].join('');

  const res = await fetch('https://api.resend.com/emails', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + apiKey, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      from: `${brand} <${from}>`,
      to: [toEmail],
      subject: `${brand} recovery code: ${otp}`,
      html,
    }),
  });
  if (!res.ok) {
    const errText = await res.text();
    console.warn('[Resend] error:', errText);
    throw new Error('Could not send the email: ' + errText);
  }
}

/**
 * Handles every auth/admin route. Returns null when the path is not an
 * auth path. `env.USERS` must be bound (the router only calls this then).
 */
export async function handleAuth(
  env: Env, req: Request, url: URL, config: KitConfig,
  ensureActivitySchema?: () => Promise<void>,
): Promise<Response | null> {
  const path = url.pathname;
  const method = req.method;
  const ac = authConfig(config);

  // ── Account-less cloud (code signup/recovery) ──
  if (method === 'POST' && path === '/nube/nueva') {
    const schoolKey = randomSchoolKey();
    await env.SCHOOL_ROUTING.put(schoolKey, 'primary');
    await env.USERS!.put(`anon:${schoolKey}`, JSON.stringify({ schoolKey, rol: 'user', creado: nowSqlite() }));
    const codigos = await emitirCodigos(env, schoolKey, ac.recoveryPrefix);
    console.log(`[nube] anonymous signup: ${await skOf(schoolKey)}`);
    return jsonCors({ ok: true, schoolKey, worker: url.origin, codigos });
  }

  if (method === 'POST' && path === '/nube/recuperar') {
    const body: any = await req.json();
    const codigo = String(body.codigo || '');
    if (normalizarCodigo(codigo).length < 8) return jsonCors({ ok: false, error: 'Invalid code' }, 400);
    const ip = req.headers.get('CF-Connecting-IP') || 'anon';
    const rlKey = `rl:recuperar:${ip}`;
    const intentos = Number(await env.USERS!.get(rlKey) || '0');
    if (intentos >= 20) return jsonCors({ ok: false, error: 'Too many attempts. Wait a few minutes.' }, 429);
    await env.USERS!.put(rlKey, String(intentos + 1), { expirationTtl: 600 });

    const hash = await hashCodigo(codigo);
    const schoolKey = await env.USERS!.get(`recovery:${hash}`);
    if (!schoolKey) return jsonCors({ ok: false, error: 'Invalid or already used code' }, 404);
    await env.USERS!.delete(`recovery:${hash}`); // single use
    const metaRaw = await env.USERS!.get(`recoverymeta:${schoolKey}`);
    if (metaRaw) {
      try {
        const meta = JSON.parse(metaRaw);
        meta.hashes = (meta.hashes || []).filter((h: string) => h !== hash);
        await env.USERS!.put(`recoverymeta:${schoolKey}`, JSON.stringify(meta));
      } catch { /* corrupt meta: ignored */ }
    }
    console.log(`[nube] recovery: ${await skOf(schoolKey)}`);
    return jsonCors({ ok: true, schoolKey, worker: url.origin });
  }

  if (method === 'POST' && path === '/nube/codigos') {
    const schoolKey = (req.headers.get('X-School-Key') || '').trim();
    if (!schoolKey) return jsonCors({ ok: false, error: 'Missing key' }, 401);
    if (!(await env.SCHOOL_ROUTING.get(schoolKey))) return jsonCors({ ok: false, error: 'Invalid key' }, 403);
    const codigos = await emitirCodigos(env, schoolKey, ac.recoveryPrefix);
    return jsonCors({ ok: true, codigos });
  }

  // ── Email/password register & login ──
  if (method === 'POST' && path === '/auth/register') {
    const body: any = await req.json().catch(() => ({}));
    const email = String(body.email || '').trim().toLowerCase();
    const nombre = String(body.nombre || '').trim();
    const password = String(body.password || '');
    if (!email || !email.includes('@') || email.length < 5) return jsonCors({ ok: false, error: 'Invalid email' }, 400);
    if (!nombre || nombre.length > 100) return jsonCors({ ok: false, error: 'Name must contain between 1 and 100 characters' }, 400);
    if (password.length < 8) return jsonCors({ ok: false, error: 'The password must be at least 8 characters' }, 400);
    if (await env.USERS!.get('user_cred:' + email)) return jsonCors({ ok: false, error: 'An account with this email already exists' }, 409);

    const saltBytes = new Uint8Array(16);
    crypto.getRandomValues(saltBytes);
    const saltHex = Array.from(saltBytes).map((b) => b.toString(16).padStart(2, '0')).join('');
    const hash = await hashPassword(password, saltHex);
    const existingProfile = await loadUserByEmail(env, email);
    const schoolKey = existingProfile?.schoolKey || randomSchoolKey();
    const now = nowSqlite();
    const cred: UserPasswordCred = { email, hash, salt: saltHex, schoolKey, creado: now };
    await env.USERS!.put('user_cred:' + email, JSON.stringify(cred));
    await env.SCHOOL_ROUTING.put(schoolKey, 'primary');
    await saveUser(env, existingProfile
      ? { ...existingProfile, nombre, lastLoginAt: now, passwordChangedAt: now }
      : {
          sub: 'email:' + email, email, nombre, foto: '',
          schoolKey, rol: isAdminEmail(env, email) ? 'admin' : 'user',
          creado: now, lastLoginAt: now, passwordChangedAt: now,
        });
    console.log('[auth] email register: ' + email);
    return jsonCors({ ok: true, schoolKey, email });
  }

  if (method === 'POST' && path === '/auth/login') {
    const body: any = await req.json().catch(() => ({}));
    const email = String(body.email || '').trim().toLowerCase();
    const password = String(body.password || '');
    if (!email || !password) return jsonCors({ ok: false, error: 'Incomplete credentials' }, 400);
    const ip = req.headers.get('CF-Connecting-IP') || 'anon';
    const rlKey = 'rl:login:' + ip;
    const attempts = Number(await env.USERS!.get(rlKey) || '0');
    if (attempts >= 25) return jsonCors({ ok: false, error: 'Too many failed attempts. Wait a few minutes.' }, 429);
    const raw = await env.USERS!.get('user_cred:' + email);
    if (!raw) {
      await env.USERS!.put(rlKey, String(attempts + 1), { expirationTtl: 300 });
      return jsonCors({ ok: false, error: 'Wrong email or password' }, 401);
    }
    let cred: UserPasswordCred;
    try { cred = JSON.parse(raw); } catch { return jsonCors({ ok: false, error: 'Error reading credentials' }, 500); }
    const hash = await hashPassword(password, cred.salt);
    if (hash !== cred.hash) {
      await env.USERS!.put(rlKey, String(attempts + 1), { expirationTtl: 300 });
      return jsonCors({ ok: false, error: 'Wrong email or password' }, 401);
    }
    await env.USERS!.delete(rlKey);
    const user = await loadUserByEmail(env, email);
    if (user) {
      user.lastLoginAt = nowSqlite();
      await saveUser(env, user);
    }
    console.log('[auth] email login: ' + email);
    return jsonCors({ ok: true, schoolKey: cred.schoolKey, email: cred.email });
  }

  // ── Send OTP email (6 digits, 10 min TTL) ──
  if (method === 'POST' && path === '/auth/send-otp') {
    const body: any = await req.json().catch(() => ({}));
    const email = String(body.email || '').trim().toLowerCase();
    if (!email || !email.includes('@')) return jsonCors({ ok: false, error: 'Invalid email' }, 400);
    const user = await loadUserByEmail(env, email);
    if (!user) return jsonCors({ ok: false, error: 'No account with this email' }, 404);
    const ip = req.headers.get('CF-Connecting-IP') || 'anon';
    const rlKey = 'rl:otp:' + ip;
    if (Number(await env.USERS!.get(rlKey) || '0') >= 5) {
      return jsonCors({ ok: false, error: 'Too many requests. Wait a few minutes.' }, 429);
    }
    await env.USERS!.put(rlKey, '1', { expirationTtl: 300 });
    const randBytes = new Uint8Array(4);
    crypto.getRandomValues(randBytes);
    const randNum = ((randBytes[0] << 24) | (randBytes[1] << 16) | (randBytes[2] << 8) | randBytes[3]) >>> 0;
    const otp = String((randNum % 900000) + 100000);
    const apiKey = env.RESEND_API_KEY || '';
    if (!apiKey) return jsonCors({ ok: false, error: 'Email service not configured' }, 503);
    await sendResendOtpEmail(apiKey, ac.brand, ac.resendFrom, email, otp);
    await env.USERS!.put('otp:' + email, otp, { expirationTtl: 600 });
    console.log('[auth] OTP sent: ' + email);
    return jsonCors({ ok: true, email });
  }

  if (method === 'POST' && path === '/auth/reset-with-otp') {
    const body: any = await req.json().catch(() => ({}));
    const email = String(body.email || '').trim().toLowerCase();
    const otp = String(body.otp || '').trim();
    const newPassword = String(body.newPassword || '');
    if (!email || !otp || !newPassword) return jsonCors({ ok: false, error: 'All fields are required' }, 400);
    if (newPassword.length < 8) return jsonCors({ ok: false, error: 'The new password must be at least 8 characters' }, 400);
    const user = await loadUserByEmail(env, email);
    if (!user) return jsonCors({ ok: false, error: 'No account with this email' }, 404);
    const ip = req.headers.get('CF-Connecting-IP') || 'anon';
    const rlKey = 'rl:otp_verify:' + ip;
    const intentos = Number(await env.USERS!.get(rlKey) || '0');
    if (intentos >= 10) return jsonCors({ ok: false, error: 'Too many attempts. Wait a few minutes.' }, 429);
    const storedOtp = await env.USERS!.get('otp:' + email);
    if (!storedOtp || storedOtp !== otp) {
      await env.USERS!.put(rlKey, String(intentos + 1), { expirationTtl: 300 });
      return jsonCors({ ok: false, error: 'Invalid or expired code' }, 401);
    }
    await env.USERS!.delete('otp:' + email);
    await env.USERS!.delete(rlKey);
    const saltBytes = new Uint8Array(16);
    crypto.getRandomValues(saltBytes);
    const saltHex = Array.from(saltBytes).map((b) => b.toString(16).padStart(2, '0')).join('');
    const now = nowSqlite();
    const cred: UserPasswordCred = { email, hash: await hashPassword(newPassword, saltHex), salt: saltHex, schoolKey: user.schoolKey, creado: now };
    await env.USERS!.put('user_cred:' + email, JSON.stringify(cred));
    user.passwordChangedAt = now;
    user.lastLoginAt = now;
    await saveUser(env, user);
    console.log('[auth] password reset via OTP: ' + email);
    return jsonCors({ ok: true, schoolKey: user.schoolKey, email });
  }

  // ── Password reset with an emergency recovery code ──
  if (method === 'POST' && path === '/auth/reset-password') {
    const body: any = await req.json().catch(() => ({}));
    const email = String(body.email || '').trim().toLowerCase();
    const codigo = String(body.codigo || '').trim();
    const newPassword = String(body.newPassword || '');
    if (!email || !codigo || !newPassword) return jsonCors({ ok: false, error: 'All fields are required' }, 400);
    if (newPassword.length < 8) return jsonCors({ ok: false, error: 'The new password must be at least 8 characters' }, 400);
    const user = await loadUserByEmail(env, email);
    if (!user || !user.schoolKey) return jsonCors({ ok: false, error: 'No account with this email' }, 404);
    const ip = req.headers.get('CF-Connecting-IP') || 'anon';
    const rlKey = 'rl:reset:' + ip;
    const intentos = Number(await env.USERS!.get(rlKey) || '0');
    if (intentos >= 15) return jsonCors({ ok: false, error: 'Too many attempts. Wait a few minutes.' }, 429);
    const hash = await hashCodigo(codigo);
    const codeSchoolKey = await env.USERS!.get('recovery:' + hash);
    if (!codeSchoolKey || codeSchoolKey !== user.schoolKey) {
      await env.USERS!.put(rlKey, String(intentos + 1), { expirationTtl: 600 });
      return jsonCors({ ok: false, error: 'Recovery code invalid or for another account' }, 401);
    }
    await consumeCodigo(env, hash);
    const metaRaw = await env.USERS!.get(`recoverymeta:${user.schoolKey}`);
    if (metaRaw) {
      try {
        const meta = JSON.parse(metaRaw);
        meta.hashes = (meta.hashes || []).filter((h: string) => h !== hash);
        await env.USERS!.put(`recoverymeta:${user.schoolKey}`, JSON.stringify(meta));
      } catch { /* corrupt meta */ }
    }
    const saltBytes = new Uint8Array(16);
    crypto.getRandomValues(saltBytes);
    const saltHex = Array.from(saltBytes).map((b) => b.toString(16).padStart(2, '0')).join('');
    const now = nowSqlite();
    const cred: UserPasswordCred = { email, hash: await hashPassword(newPassword, saltHex), salt: saltHex, schoolKey: user.schoolKey, creado: now };
    await env.USERS!.put('user_cred:' + email, JSON.stringify(cred));
    user.passwordChangedAt = now;
    user.lastLoginAt = now;
    await saveUser(env, user);
    await env.USERS!.delete(rlKey);
    console.log('[auth] password reset via recovery code: ' + email);
    return jsonCors({ ok: true, schoolKey: user.schoolKey, email });
  }

  // ── Google login/registration ──
  if (path === '/auth') {
    const body: any = await req.json();
    if (!body.idToken) throw new Error('Missing idToken');
    const claims = await verifyGoogleIdToken(env, body.idToken);
    const email = claims.email.toLowerCase();
    if (!isAllowedEmail(env, email)) {
      return json({ ok: false, error: 'This Google account is not authorized for the cloud.' }, 403);
    }
    const admin = isAdminEmail(env, email);

    let user = await loadUserByEmail(env, email);
    let creado = false;
    const now = nowSqlite();
    if (!user) {
      const schoolKey = randomSchoolKey();
      user = {
        sub: claims.sub, email, nombre: claims.name || email,
        foto: claims.picture || '', schoolKey,
        rol: admin ? 'admin' : 'user', creado: now, lastLoginAt: now,
      };
      await saveUser(env, user);
      await env.SCHOOL_ROUTING.put(schoolKey, 'primary');
      creado = true;
      console.log(`[auth] registered: ${email} (${user.rol})`);
    } else {
      if (user.suspendido) return json({ ok: false, error: 'Account suspended. Contact the administrator.' }, 403);
      user.nombre = claims.name || user.nombre;
      user.foto = claims.picture || user.foto;
      user.lastLoginAt = now;
      await saveUser(env, user);
    }

    // Legacy claim: the FIRST admin claims pre-namespaced rows once.
    let reclamadas = 0;
    if (user.rol === 'admin') {
      const flag = await env.USERS!.get('legacy_claimed');
      if (!flag) {
        const sk = await skOf(user.schoolKey);
        const others = await env.USERS!.list({ prefix: 'user:' });
        const soloAdmin = others.keys.length <= 1;
        const stmts = manifest.tables.map((t) => (soloAdmin
          ? { sql: `UPDATE ${t.name} SET sk = ?1`, params: [sk] }
          : { sql: `UPDATE ${t.name} SET sk = ?1 WHERE sk IS NULL OR sk = ''`, params: [sk] }));
        stmts.push(soloAdmin
          ? { sql: `UPDATE sync_tombstone SET sk = ?1`, params: [sk] }
          : { sql: `UPDATE sync_tombstone SET sk = ?1 WHERE sk IS NULL OR sk = ''`, params: [sk] });
        for (let i = 0; i < stmts.length; i += BATCH_STMT) {
          const res = await batchExec(env.DB, stmts.slice(i, i + BATCH_STMT));
          reclamadas += res.reduce((a, b) => a + b.changes, 0);
        }
        await env.USERS!.put('legacy_claimed', `${user.email}:${nowSqlite()}`);
        console.log(`[auth] legacy claimed by ${email}: ${reclamadas} rows (soloAdmin=${soloAdmin})`);
      }
    }

    if (!(await env.SCHOOL_ROUTING.get(user.schoolKey))) {
      await env.SCHOOL_ROUTING.put(user.schoolKey, 'primary');
    }

    return json({
      ok: true, creado,
      schoolKey: user.schoolKey,
      perfil: { nombre: user.nombre, foto: user.foto, email: user.email, rol: user.rol },
      legacyReclamadas: reclamadas,
    });
  }

  // ── Admin panel (X-Auth-Token of an admin) ──
  if (path.startsWith('/admin/')) {
    const admin = await requireAdmin(env, req);

    if (path === '/admin/users' && method === 'POST') {
      const list = await env.USERS!.list({ prefix: 'user:' });
      const users: any[] = [];
      for (const k of list.keys) {
        const raw = await env.USERS!.get(k.name);
        if (!raw) continue;
        const u = JSON.parse(raw);
        users.push({
          email: u.email, nombre: u.nombre, foto: u.foto, rol: u.rol,
          creado: u.creado, suspendido: !!u.suspendido,
          lastLoginAt: u.lastLoginAt || null,
        });
      }
      return json({ ok: true, users });
    }

    if (path === '/admin/user-activity' && method === 'POST') {
      const body: any = await req.json().catch(() => ({}));
      const target = String(body.email || '').trim().toLowerCase();
      const user = await loadUserByEmail(env, target);
      if (!user) throw new Error('User not found');
      if (ensureActivitySchema) await ensureActivitySchema();

      const sk = await skOf(user.schoolKey);
      const safeTables = manifest.tables.map((table) => {
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(table.name)) {
          throw new Error(`manifest: unsafe table ${JSON.stringify(table.name)}`);
        }
        return table.name;
      });
      const statements = safeTables.map((table) => ({
        sql: `SELECT COUNT(*) AS rows, MAX(updated_at) AS latest FROM ${table} WHERE sk = ?1`,
        params: [sk],
      }));
      statements.push({
        sql: 'SELECT COUNT(*) AS rows, MAX(updated_at) AS latest FROM sync_tombstone WHERE sk = ?1',
        params: [sk],
      });
      const results = await batchExec(env.DB, statements);
      const tables = safeTables.map((table, index) => ({
        name: table,
        rows: Number(results[index].results?.[0]?.rows) || 0,
        lastModifiedAt: results[index].results?.[0]?.latest || null,
      }));
      const deletions = results[safeTables.length]?.results?.[0] || {};
      const latestValues = [
        ...tables.map((table) => table.lastModifiedAt).filter((value): value is string => !!value),
        ...(deletions.latest ? [String(deletions.latest)] : []),
      ];
      return json({
        ok: true,
        email: user.email,
        passwordChangedAt: user.passwordChangedAt || null,
        lastActivityAt: latestValues.sort().at(-1) || null,
        totalRows: tables.reduce((total, table) => total + table.rows, 0),
        deletedRows: Number(deletions.rows) || 0,
        tables: tables.filter((table) => table.rows > 0 || table.lastModifiedAt)
          .sort((a, b) => (b.lastModifiedAt || '').localeCompare(a.lastModifiedAt || '')),
      });
    }

    if (path === '/admin/set-password' && method === 'POST') {
      const body: any = await req.json().catch(() => ({}));
      const target = String(body.email || '').trim().toLowerCase();
      const password = String(body.password || '');
      if (!target || !password) return json({ ok: false, error: 'Email and password are required' }, 400);
      if (password.length < 8 || password.length > 128) {
        return json({ ok: false, error: 'Password must contain between 8 and 128 characters' }, 400);
      }
      const user = await loadUserByEmail(env, target);
      if (!user) throw new Error('User not found');
      if (user.rol === 'admin' || admin.email.toLowerCase() === target) {
        return json({ ok: false, error: 'Administrator passwords cannot be changed here' }, 403);
      }

      const saltBytes = new Uint8Array(16);
      crypto.getRandomValues(saltBytes);
      const salt = Array.from(saltBytes).map((b) => b.toString(16).padStart(2, '0')).join('');
      const now = nowSqlite();
      const cred: UserPasswordCred = {
        email: target, hash: await hashPassword(password, salt), salt,
        schoolKey: user.schoolKey, creado: now,
      };
      await env.USERS!.put('user_cred:' + target, JSON.stringify(cred));
      user.passwordChangedAt = now;
      await saveUser(env, user);
      console.log(`[admin] password reset for ${target}`);
      return json({ ok: true });
    }

    if (path === '/admin/suspend' && method === 'POST') {
      const body: any = await req.json();
      const target = String(body.email || '').toLowerCase();
      const suspend = body.suspend !== false;
      const u = await loadUserByEmail(env, target);
      if (!u) throw new Error('User not found');
      if (u.rol === 'admin') throw new Error('Administrators cannot be suspended');
      u.suspendido = suspend;
      await saveUser(env, u);
      if (suspend) await env.SCHOOL_ROUTING.delete(u.schoolKey);
      else await env.SCHOOL_ROUTING.put(u.schoolKey, 'primary');
      return json({ ok: true });
    }

    if (path === '/admin/delete-data' && method === 'POST') {
      const body: any = await req.json();
      const target = String(body.email || '').toLowerCase();
      const u = await loadUserByEmail(env, target);
      if (!u) throw new Error('User not found');
      const deletedRows = await purgeNamespace(env, u.schoolKey);
      return json({ ok: true, borradas: deletedRows });
    }

    if (path === '/admin/delete-account' && method === 'POST') {
      const body: any = await req.json();
      const target = String(body.email || '').trim().toLowerCase();
      const confirmation = String(body.confirmEmail || '').trim().toLowerCase();
      if (!target || confirmation !== target) {
        return json({ ok: false, error: 'Repeat the target email in confirmEmail to confirm deletion' }, 400);
      }
      const user = await loadUserByEmail(env, target);
      if (!user) throw new Error('User not found');
      if (user.rol === 'admin' || admin.email.toLowerCase() === target) {
        return json({ ok: false, error: 'Administrator accounts cannot be deleted' }, 403);
      }
      const recoveryMetaKey = `recoverymeta:${user.schoolKey}`;
      const recoveryMetaRaw = await env.USERS!.get(recoveryMetaKey);
      let recoveryHashes: string[] = [];
      if (recoveryMetaRaw) {
        const meta = JSON.parse(recoveryMetaRaw) as { hashes?: string[] };
        if (meta.hashes && !Array.isArray(meta.hashes)) throw new Error('Invalid recovery metadata');
        recoveryHashes = meta.hashes || [];
        for (const h of recoveryHashes) {
          if (!/^[a-f0-9]{64}$/i.test(h)) throw new Error('Invalid recovery hash');
        }
      }
      // Purge D1 first; if that fails the credentials stay to retry.
      const deletedRows = await purgeNamespace(env, user.schoolKey);
      for (const h of recoveryHashes) await env.USERS!.delete(`recovery:${h}`);
      await env.USERS!.delete(recoveryMetaKey);
      await env.SCHOOL_ROUTING.delete(user.schoolKey);
      await env.USERS!.delete(`anon:${user.schoolKey}`);
      await env.USERS!.delete(`otp:${target}`);
      await env.USERS!.delete(`user_cred:${target}`);
      await env.USERS!.delete(`user:${target}`);
      console.log(`[admin] deleted account ${target}: ${deletedRows} rows`);
      return json({ ok: true, email: target, deletedRows });
    }

    return json({ ok: false, error: 'Unknown admin route' }, 404);
  }

  return null;
}

#!/usr/bin/env node
/**
 * parity-check.mjs — Compare a fresh wire capture against the golden Phase 0
 * fixture (the recorded contract of the original School Tracker worker).
 *
 * Comparison criteria (agreed for Phase 2c):
 *   - paths and HTTP statuses must match exactly; REQUEST bytes should match
 *     after normalizing timestamps — if they differ only by the ORDER of
 *     rows/tombstones inside arrays (SQLite result order is not contractual
 *     and neither is the client's), the exchange is accepted with a note;
 *   - RESPONSES must be deep-equal, order-insensitive (arrays of objects are
 *     compared as sets), after normalizing:
 *       * timestamps → <TS> (server clock differs between runs);
 *       * ids of `server_assigns_id` tables → <RID> (the server assigns a
 *         rowid; its value depends on the local DB's insertion history);
 *       * /ids block numbers → <S>/<E> after validating the reservation
 *         formula (end - start + 1 == requested count); the absolute values
 *         depend on the persistent counter's history.
 *
 * Usage:
 *   node scripts/parity-check.mjs <golden.json> <new.json> [manifest.json]
 */

import { readFileSync } from 'node:fs';

const [goldenPath, newPath, manifestPath] = process.argv.slice(2);
if (!goldenPath || !newPath) {
  console.error('usage: node parity-check.mjs <golden.json> <new.json> [manifest.json]');
  process.exit(2);
}

const golden = JSON.parse(readFileSync(goldenPath, 'utf8'));
const fresh = JSON.parse(readFileSync(newPath, 'utf8'));

const serverAssigned = new Set(
  manifestPath
    ? JSON.parse(readFileSync(manifestPath, 'utf8')).tables.filter((t) => t.server_assigns_id).map((t) => t.name)
    : ['instrumento_plantilla'],
);

const TS = /\d{4}-\d{2}-\d{2}([T ])\d{2}:\d{2}:\d{2}(\.\d+)?Z?/g;
const normText = (s) => s.replace(TS, '<TS>');

/** Mutate a parsed response, replacing run-history-dependent values. */
function normalizeJson(path, obj) {
  if (path === '/ids' && obj && obj.blocks) {
    for (const v of Object.values(obj.blocks)) {
      delete v.start; delete v.end;
      v.range = '<S>-<E>';
    }
  }
  if (obj && obj.rows) {
    for (const [table, rows] of Object.entries(obj.rows)) {
      if (!serverAssigned.has(table) || !Array.isArray(rows)) continue;
      for (const row of rows) if (row && 'id' in row) row.id = '<RID>';
    }
  }
  return obj;
}

const sorted = (v) => {
  if (Array.isArray(v)) {
    const items = v.map(sorted);
    // Arrays of objects are unordered sets (rows, tombstones). Arrays of
    // primitives are kept as-is: pk component lists ARE ordered.
    if (items.every((x) => x && typeof x === 'object')) {
      items.sort((x, y) => (JSON.stringify(x) < JSON.stringify(y) ? -1 : 1));
    }
    return items;
  }
  if (v && typeof v === 'object') {
    const o = {};
    for (const k of Object.keys(v).sort()) o[k] = sorted(v[k]);
    return o;
  }
  return v;
};

const E = golden.exchanges, F = fresh.exchanges;
let fails = 0;
const say = (i, msg) => { console.log(`EXCHANGE[${i}] ${E[i]?.path}: ${msg}`); fails++; };

if (!Array.isArray(E) || !Array.isArray(F) || E.length !== F.length) {
  console.error(`exchange count differs: golden ${E?.length} vs fresh ${F?.length}`);
  process.exit(1);
}

for (let i = 0; i < E.length; i++) {
  const a = E[i], b = F[i];
  const probs = [];
  if (a.path !== b.path) probs.push(`path ${a.path} vs ${b.path}`);
  if (a.status !== b.status) probs.push(`status ${a.status} vs ${b.status}`);
  if (normText(a.request) !== normText(b.request)) {
    let same = false;
    try {
      same = JSON.stringify(sorted(normalizeJson(a.path, JSON.parse(normText(a.request)))))
           === JSON.stringify(sorted(normalizeJson(b.path, JSON.parse(normText(b.request)))));
    } catch { /* not JSON: stays a failure */ }
    if (same) console.log(`note [${i}] ${a.path}: request bytes differ only by array order (accepted)`);
    else probs.push('REQUEST differs (after TS + set-order normalization)');
  }
  try {
    const ja = sorted(normalizeJson(a.path, JSON.parse(normText(a.response))));
    const jb = sorted(normalizeJson(b.path, JSON.parse(normText(b.response))));
    if (JSON.stringify(ja) !== JSON.stringify(jb)) probs.push('RESPONSE deep-differs');
  } catch (e) { probs.push('response unparseable: ' + e.message); }
  // /ids: validate the reservation formula on both sides.
  if (a.path === '/ids') {
    for (const [side, txt] of [['golden', a], ['fresh', b]]) {
      const req = JSON.parse(txt.request).blocks || {};
      const res = JSON.parse(txt.response).blocks || {};
      for (const [t, n] of Object.entries(req)) {
        const r = res[t];
        if (!r || r.end - r.start + 1 !== Math.max(1, Math.floor(n))) {
          probs.push(`${side} /ids formula broken for ${t}`);
        }
      }
    }
  }
  if (probs.length) say(i, probs.join('; '));
}

if (fails) { console.error(`\n❌ ${fails} of ${E.length} exchanges FAILED`); process.exit(1); }
console.log(`\n✅ PARITY OK — all ${E.length} exchanges match the golden contract`);
console.log('   (timestamps, server-assigned ids and /ids counter history normalized)');

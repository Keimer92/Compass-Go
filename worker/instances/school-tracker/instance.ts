/**
 * School Tracker instance: one-time D1 migrations carried by the worker's
 * cold start, for databases created before the current `schema-base-nueva`
 * shape. A fresh local/prod D1 provisioned from that schema no-ops through
 * all of these.
 *
 * These are LEGACY shapes, not protocol: the generic part of ensureSchema
 * (infrastructure tables, sk/device_id columns, conflict indexes) lives in
 * the template engine and never mentions table names.
 */
import type { Env } from './src/types';
import { queryRows } from './src/engine';

async function markerDone(env: Env, key: string): Promise<boolean> {
  const row = await env.DB.prepare(`SELECT value FROM sync_metadata WHERE key = ?1`).bind(key).first();
  return !!row;
}

async function setMarker(env: Env, key: string): Promise<void> {
  try {
    await env.DB.prepare(`INSERT OR REPLACE INTO sync_metadata (key, value) VALUES (?1, 1)`).bind(key).run();
  } catch { /* best effort */ }
}

/** Rebuild tables that were created with GLOBAL UNIQUEs (multi-tenant D1). */
const DEUNIQUE_TABLES: Record<string, string> = {
  ano_lectivo: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, activo INTEGER DEFAULT 0, cualitativo INTEGER DEFAULT 0, updated_at TEXT, device_id TEXT, sk TEXT)`,
  corte_evaluativo: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, activo INTEGER DEFAULT 0, cualitativo INTEGER DEFAULT 0, updated_at TEXT, device_id TEXT, sk TEXT)`,
  colegio: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, activo INTEGER DEFAULT 0, cualitativo INTEGER DEFAULT 0, orden INTEGER, tipo TEXT DEFAULT 'Primaria', updated_at TEXT, device_id TEXT, sk TEXT)`,
  asignatura: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, cualitativo INTEGER DEFAULT 0, updated_at TEXT, device_id TEXT, sk TEXT)`,
  grado: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, cualitativo INTEGER DEFAULT 0, updated_at TEXT, device_id TEXT, sk TEXT)`,
  seccion: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, cualitativo INTEGER DEFAULT 0, updated_at TEXT, device_id TEXT, sk TEXT)`,
  estudiante: `(id INTEGER PRIMARY KEY AUTOINCREMENT, nombre TEXT, codigo TEXT, sexo TEXT, ano_lectivo_id INTEGER, grado_id INTEGER, seccion_id INTEGER, colegio_id INTEGER, foto TEXT, updated_at TEXT, device_id TEXT, sk TEXT)`,
  estudiante_retirado: `(estudiante_id INTEGER PRIMARY KEY, fecha_retiro TEXT, motivo TEXT, updated_at TEXT, device_id TEXT, sk TEXT)`,
};

export async function legacyMigrations(env: Env): Promise<void> {
  await env.DB.prepare(
    'CREATE TABLE IF NOT EXISTS sync_metadata (key TEXT PRIMARY KEY, value INTEGER NOT NULL DEFAULT 0)',
  ).run();

  // Reshape of the old dia_sin_clases without grado_id.
  try {
    const dscCols = await queryRows(env.DB, "SELECT name FROM pragma_table_info('dia_sin_clases')");
    if (dscCols.length > 0 && !dscCols.some((c: any) => c.name === 'grado_id')) {
      await env.DB.prepare('DROP TABLE dia_sin_clases').run();
    }
  } catch { /* table not there yet */ }

  // Old sync_tombstone (PK without sk): copy preserving rows.
  try {
    const tsCols = await queryRows(env.DB, "SELECT name FROM pragma_table_info('sync_tombstone')");
    if (tsCols.length > 0 && !tsCols.some((c: any) => c.name === 'sk')) {
      await env.DB.prepare('ALTER TABLE sync_tombstone RENAME TO sync_tombstone_old').run();
      await env.DB.exec(`CREATE TABLE sync_tombstone (
        tabla TEXT NOT NULL, pk TEXT NOT NULL, updated_at TEXT NOT NULL,
        device_id TEXT, sk TEXT DEFAULT '',
        PRIMARY KEY (tabla, pk, sk))`);
      await env.DB.exec(`INSERT OR IGNORE INTO sync_tombstone (tabla, pk, updated_at, device_id, sk)
        SELECT tabla, pk, updated_at, device_id, '' FROM sync_tombstone_old`);
      await env.DB.prepare('DROP TABLE sync_tombstone_old').run();
    }
  } catch { /* best effort */ }

  for (const [sql, note] of [
    ["ALTER TABLE indicador ADD COLUMN unidad_numero INTEGER", 'indicador.unidad_numero'],
    ["ALTER TABLE colegio ADD COLUMN tipo TEXT DEFAULT 'Primaria'", 'colegio.tipo'],
    ["ALTER TABLE indicador ADD COLUMN ano_lectivo_id INTEGER DEFAULT 0", 'indicador.ano_lectivo_id'],
    ["ALTER TABLE indicador ADD COLUMN plantilla_id INTEGER", 'indicador.plantilla_id'],
    ['ALTER TABLE dia_sin_clases ADD COLUMN device_id TEXT', 'dia_sin_clases.device_id'],
  ] as const) {
    try { await env.DB.prepare(sql).run(); console.log(`[migration] ${note}`); } catch { /* exists */ }
  }

  // criterio: UNIQUE(indicador_id, orden) with a dedup pass (reads nota, so
  // gate it durably; never treat a read error as "not done").
  if (!(await markerDone(env, 'criterio_unique_v1'))) {
    const indexes = await queryRows(env.DB, "PRAGMA index_list('criterio')");
    let hasUniquePair = false;
    let deduplicated = false;
    for (const index of indexes) {
      if (Number(index.unique) !== 1) continue;
      const indexName = String(index.name).replace(/'/g, "''");
      const columns = await queryRows(env.DB, `PRAGMA index_info('${indexName}')`);
      if (columns.map((c: any) => c.name).join('|') === 'indicador_id|orden') { hasUniquePair = true; break; }
    }
    if (!hasUniquePair) {
      const hasDuplicates = await env.DB.prepare(
        'SELECT 1 FROM criterio GROUP BY indicador_id, orden HAVING COUNT(*) > 1 LIMIT 1',
      ).first();
      if (hasDuplicates) {
        await env.DB.prepare(`
          DELETE FROM criterio WHERE id NOT IN (
            SELECT id FROM (
              SELECT id, ROW_NUMBER() OVER (
                PARTITION BY indicador_id, orden
                ORDER BY (SELECT COUNT(*) FROM nota n WHERE n.criterio_id = criterio.id) DESC, id ASC
              ) AS rn
              FROM criterio
            ) WHERE rn = 1
          )
        `).run();
        deduplicated = true;
      }
    }
    await env.DB.prepare(
      'CREATE UNIQUE INDEX IF NOT EXISTS idx_criterio_indicador_orden ON criterio(indicador_id, orden)',
    ).run();
    await setMarker(env, 'criterio_unique_v1');
    console.log(`[migration] criterio unique (dedup=${deduplicated})`);
  }

  // Drop GLOBAL UNIQUEs once (multi-tenant namespaces need per-sk uniqueness).
  if (!(await markerDone(env, 'deunique_v2'))) {
    for (const [table, def] of Object.entries(DEUNIQUE_TABLES)) {
      let renamed = false;
      try {
        const info = await queryRows(env.DB, `SELECT sql FROM sqlite_master WHERE type='table' AND name='${table}'`);
        if (!info.length || !String(info[0].sql || '').toUpperCase().includes('UNIQUE')) continue;
        const oldCols = await queryRows(env.DB, `SELECT name FROM pragma_table_info('${table}')`);
        const colList = oldCols.map((c: any) => c.name).join(', ');
        await env.DB.prepare(`ALTER TABLE ${table} RENAME TO ${table}_old_dq`).run();
        renamed = true;
        await env.DB.exec(`CREATE TABLE ${table} ${def}`);
        await env.DB.exec(`INSERT INTO ${table} (${colList}, device_id, sk) SELECT ${colList}, '', '' FROM ${table}_old_dq`);
        await env.DB.prepare(`DROP TABLE ${table}_old_dq`).run();
      } catch (e: any) {
        console.error(`[migration] ${table}: ${e?.message}`);
        if (renamed) { try { await env.DB.prepare(`ALTER TABLE ${table}_old_dq RENAME TO ${table}`).run(); } catch { /* give up */ } }
      }
    }
    await setMarker(env, 'deunique_v2');
  }

  if (!(await markerDone(env, 'corte_evaluativo_namespace_v1'))) {
    const columns = new Set(
      (await queryRows(env.DB, "SELECT name FROM pragma_table_info('corte_evaluativo')"))
        .map((column: any) => String(column.name)),
    );
    if (!columns.has('id') || !columns.has('nombre')) {
      throw new Error('Cannot migrate corte_evaluativo: required id/nombre columns are missing');
    }

    const value = (column: string, fallback: string) => columns.has(column) ? column : fallback;
    await env.DB.batch([
      env.DB.prepare('DROP TABLE IF EXISTS corte_evaluativo_namespace_new'),
      env.DB.prepare(`CREATE TABLE corte_evaluativo_namespace_new (
        id INTEGER NOT NULL,
        nombre TEXT NOT NULL,
        activo INTEGER NOT NULL DEFAULT 0,
        cualitativo INTEGER NOT NULL DEFAULT 0,
        updated_at TEXT,
        device_id TEXT,
        sk TEXT NOT NULL DEFAULT '',
        PRIMARY KEY (id, sk)
      )`),
      env.DB.prepare(`INSERT INTO corte_evaluativo_namespace_new
        (id, nombre, activo, cualitativo, updated_at, device_id, sk)
        SELECT id, nombre, COALESCE(${value('activo', '0')}, 0),
          COALESCE(${value('cualitativo', '0')}, 0),
          ${value('updated_at', "datetime('now')")},
          ${value('device_id', "''")}, COALESCE(${value('sk', "''")}, '')
        FROM corte_evaluativo`),
      env.DB.prepare('DROP TABLE corte_evaluativo'),
      env.DB.prepare('ALTER TABLE corte_evaluativo_namespace_new RENAME TO corte_evaluativo'),
    ]);
    await env.DB.prepare(
      "INSERT OR REPLACE INTO sync_metadata (key, value) VALUES ('corte_evaluativo_namespace_v1', 1)",
    ).run();
    console.log('[migration] corte_evaluativo namespace-scoped identity');
  }

  // Templates are owned per user: the identity is (propietario, nombre), so the
  // old UNIQUE(nombre) index must go or a second teacher could not reuse a name.
  // The column is added here too so a deploy without the SQL file still works.
  try {
    await env.DB.prepare(
      "ALTER TABLE instrumento_plantilla ADD COLUMN propietario TEXT NOT NULL DEFAULT ''",
    ).run();
  } catch { /* column already present */ }

  try {
    await env.DB.prepare('DROP INDEX IF EXISTS idx_instrumento_plantilla_nombre').run();
  } catch { /* already gone */ }

  try {
    await env.DB.prepare(
      'CREATE UNIQUE INDEX IF NOT EXISTS idx_instrumento_plantilla_conflict ON instrumento_plantilla(propietario, nombre)',
    ).run();
  } catch { /* duplicate names */ }

  try {
    await env.DB.prepare(
      'CREATE UNIQUE INDEX IF NOT EXISTS idx_dia_sin_clases_unica ON dia_sin_clases(colegio_id, IFNULL(grado_id, 0), IFNULL(seccion_id, 0), fecha)',
    ).run();
  } catch { /* duplicates */ }
}

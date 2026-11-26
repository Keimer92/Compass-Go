package schoolschema

// SchemaSQL es el DDL SQLite de ejemplo (la base de School Tracker, la app
// whose manifest ships as this example). It doubles as the test bootstrap:
// the kit never creates app tables in production, only in its own tests.

const SchemaSQL = `
-- ── Catálogos globales ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ano_lectivo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS corte_evaluativo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS colegio (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    orden INTEGER DEFAULT 0,
    tipo TEXT DEFAULT 'Primaria',
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS asignatura (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS grado (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS seccion (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS app_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS d1_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    worker_url TEXT NOT NULL DEFAULT '',
    school_key TEXT NOT NULL DEFAULT '',
    device_id TEXT NOT NULL DEFAULT '',
    last_sync TEXT,
    -- Freno de reintentos del PUSH: si un push falla, NO se reintenta hasta
    -- esta hora (ver GOTCHA #1 del AGENTS.md: la cuota diaria de D1).
    push_fail_count INTEGER DEFAULT 0,
    push_retry_after TEXT,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS d1_id_blocks (
    tabla TEXT PRIMARY KEY,
    next_id INTEGER NOT NULL DEFAULT 0,
    end_id INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now'))
);

-- ── Estudiantes ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS estudiante (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL,
    codigo TEXT NOT NULL UNIQUE,
    codigo_base TEXT NOT NULL DEFAULT '',
    sexo TEXT CHECK(sexo IN ('M', 'F')) NOT NULL,
    ano_lectivo_id INTEGER NOT NULL,
    grado_id INTEGER NOT NULL,
    seccion_id INTEGER NOT NULL,
    colegio_id INTEGER NOT NULL,
    foto TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(ano_lectivo_id) REFERENCES ano_lectivo(id),
    FOREIGN KEY(grado_id) REFERENCES grado(id),
    FOREIGN KEY(seccion_id) REFERENCES seccion(id),
    FOREIGN KEY(colegio_id) REFERENCES colegio(id)
);

CREATE TABLE IF NOT EXISTS respaldo_meta (
    clave TEXT PRIMARY KEY,
    valor TEXT NOT NULL
);

-- Baja lógica: la fila del estudiante NO se borra, queda marcada acá.
CREATE TABLE IF NOT EXISTS estudiante_retirado (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    estudiante_id INTEGER NOT NULL UNIQUE,
    fecha_retiro TEXT NOT NULL,
    motivo TEXT NOT NULL DEFAULT '',
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(estudiante_id) REFERENCES estudiante(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_estudiante_retirado_est
    ON estudiante_retirado(estudiante_id);

-- Vista con los estudiantes que siguen activos (todas las lecturas la usan).
DROP VIEW IF EXISTS estudiante_activo;
CREATE VIEW estudiante_activo AS
    SELECT e.* FROM estudiante e
    WHERE NOT EXISTS (
        SELECT 1 FROM estudiante_retirado r WHERE r.estudiante_id = e.id
    );

-- ── Relaciones muchos a muchos ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS grado_asignatura (
    grado_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (grado_id, asignatura_id),
    FOREIGN KEY(grado_id) REFERENCES grado(id) ON DELETE CASCADE,
    FOREIGN KEY(asignatura_id) REFERENCES asignatura(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS colegio_asignatura (
    colegio_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (colegio_id, asignatura_id),
    FOREIGN KEY(colegio_id) REFERENCES colegio(id) ON DELETE CASCADE,
    FOREIGN KEY(asignatura_id) REFERENCES asignatura(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS colegio_grado (
    colegio_id INTEGER NOT NULL,
    grado_id INTEGER NOT NULL,
    activo INTEGER DEFAULT 1,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (colegio_id, grado_id),
    FOREIGN KEY(colegio_id) REFERENCES colegio(id) ON DELETE CASCADE,
    FOREIGN KEY(grado_id) REFERENCES grado(id) ON DELETE CASCADE
);

-- ── Evaluaciones ──────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS indicador (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    grado_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    corte_evaluativo_id INTEGER NOT NULL,
    ano_lectivo_id INTEGER NOT NULL DEFAULT 0,
    orden INTEGER NOT NULL,
    descripcion TEXT DEFAULT '',
    activo INTEGER DEFAULT 1,
    instrumento TEXT NOT NULL DEFAULT 'general',
    plantilla_id INTEGER,
    unidad_numero INTEGER,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(grado_id) REFERENCES grado(id),
    FOREIGN KEY(asignatura_id) REFERENCES asignatura(id),
    FOREIGN KEY(corte_evaluativo_id) REFERENCES corte_evaluativo(id),
    FOREIGN KEY(ano_lectivo_id) REFERENCES ano_lectivo(id)
);

CREATE TABLE IF NOT EXISTS criterio (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indicador_id INTEGER NOT NULL,
    orden INTEGER NOT NULL,
    descripcion TEXT DEFAULT '',
    puntaje_maximo INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(indicador_id) REFERENCES indicador(id) ON DELETE CASCADE,
    UNIQUE(indicador_id, orden)
);

CREATE TABLE IF NOT EXISTS indicador_colegio_seccion_activo (
    colegio_id INTEGER NOT NULL,
    seccion_id INTEGER NOT NULL,
    indicador_id INTEGER NOT NULL,
    activo INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (colegio_id, seccion_id, indicador_id),
    FOREIGN KEY(indicador_id) REFERENCES indicador(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ind_colegio_seccion
    ON indicador_colegio_seccion_activo(colegio_id, seccion_id, indicador_id);

-- ── Asistencia ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS asistencia (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    estudiante_id INTEGER NOT NULL,
    fecha TEXT NOT NULL,
    estado TEXT CHECK(estado IN ('P', 'A', 'J')) NOT NULL,
    observacion TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    UNIQUE(estudiante_id, fecha),
    FOREIGN KEY(estudiante_id) REFERENCES estudiante(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS colegio_dias_asistencia (
    colegio_id INTEGER PRIMARY KEY,
    lunes INTEGER DEFAULT 1,
    martes INTEGER DEFAULT 1,
    miercoles INTEGER DEFAULT 1,
    jueves INTEGER DEFAULT 1,
    viernes INTEGER DEFAULT 1,
    sabado INTEGER DEFAULT 0,
    domingo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(colegio_id) REFERENCES colegio(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS dia_sin_clases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    colegio_id INTEGER NOT NULL,
    grado_id INTEGER,
    seccion_id INTEGER,
    fecha TEXT NOT NULL,
    descripcion TEXT NOT NULL DEFAULT '',
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY(colegio_id) REFERENCES colegio(id) ON DELETE CASCADE,
    FOREIGN KEY(grado_id) REFERENCES grado(id) ON DELETE CASCADE,
    FOREIGN KEY(seccion_id) REFERENCES seccion(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dia_sin_clases_unica
    ON dia_sin_clases(colegio_id, IFNULL(grado_id, 0), IFNULL(seccion_id, 0), fecha);

CREATE TABLE IF NOT EXISTS instrumento_plantilla (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    propietario TEXT NOT NULL DEFAULT '',
    nombre TEXT NOT NULL,
    tipo TEXT NOT NULL DEFAULT 'escala',
    definicion TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT DEFAULT (datetime('now'))
);

-- ── Notas ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS nota (
    estudiante_id INTEGER NOT NULL,
    criterio_id INTEGER NOT NULL,
    valor REAL NOT NULL,
    corte_evaluativo_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (estudiante_id, criterio_id, corte_evaluativo_id, asignatura_id),
    FOREIGN KEY(estudiante_id) REFERENCES estudiante(id) ON DELETE CASCADE,
    FOREIGN KEY(criterio_id) REFERENCES criterio(id) ON DELETE CASCADE,
    FOREIGN KEY(corte_evaluativo_id) REFERENCES corte_evaluativo(id),
    FOREIGN KEY(asignatura_id) REFERENCES asignatura(id)
);

-- ── Configuración de calificaciones ───────────────────────────────────────
CREATE TABLE IF NOT EXISTS configuracion_calificacion (
    id TEXT PRIMARY KEY,
    nombre TEXT NOT NULL,
    color TEXT NOT NULL,
    minimo INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now'))
);

-- ── Sincronización (pendientes de borrado) ────────────────────────────────
CREATE TABLE IF NOT EXISTS sync_pending_deletions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tabla TEXT NOT NULL,
    registro_id INTEGER NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- El borrado de un ESTUDIANTE viaja por su codigo, no por su id local.
--
-- Los ids locales divergen entre dispositivos (cada base tiene los suyos), pero
-- el codigo es la clave de negocio del estudiante: es el mismo en todos los
-- equipos. Un tombstone por id no borraba nada en la nube (la fila remota
-- tenia el id de otro dispositivo) y el alumno "reaparecia" en el otro equipo.
--
-- Por eso los estudiantes tienen su propia tabla de pendientes (la generica
-- guarda un registro_id INTEGER y no puede llevar el codigo).
CREATE TABLE IF NOT EXISTS sync_pending_deletions_estudiante (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    codigo TEXT NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Attendance travels by the student codigo too: its identity in the cloud is
-- (codigo, fecha), not (estudiante_id, fecha). Local ids diverge between
-- devices, so a tombstone by id deleted the wrong row (or none), and every new
-- push of that tombstone re-stamped it and wiped the attendance another device
-- had just taken. See the note on sync_pending_deletions_estudiante.
CREATE TABLE IF NOT EXISTS sync_pending_deletions_asistencia (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    estudiante_id INTEGER NOT NULL,
    codigo TEXT NOT NULL DEFAULT '',
    fecha TEXT NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_pending_deletions_nota (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    estudiante_id INTEGER NOT NULL,
    criterio_id INTEGER NOT NULL,
    corte_evaluativo_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS multiplicador_calificacion (
    id TEXT PRIMARY KEY,
    multiplicador REAL NOT NULL DEFAULT 1.0,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sync_pending_deletions_grado_asignatura (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    grado_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_pending_deletions_colegio_asignatura (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    colegio_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_pending_deletions_colegio_grado (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    colegio_id INTEGER NOT NULL,
    grado_id INTEGER NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_pending_deletions_instrumento_plantilla (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    propietario TEXT NOT NULL DEFAULT '',
    nombre TEXT NOT NULL,
    fecha_intento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ── Configuración de la app (key-value, sincronizable) ────────────────────
CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT DEFAULT (datetime('now'))
);
`

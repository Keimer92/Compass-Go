-- Esquema extraido de la base que YA FUNCIONA (schooltracker-nube).
-- Se replica en la base nueva: el worker hace deUniqueTables() al arrancar y
-- QUITA los UNIQUE globales sin crear el indice unico por (clave, sk),
-- con lo que el ON CONFLICT del upsert se queda sin constraint.
-- Aqui se deja el esquema con los UNIQUE y se marca 'deunique_v2'
-- para que el worker no lo desmonte.

CREATE TABLE "ano_lectivo" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '{}', updated_at TEXT, device_id TEXT, sk TEXT);

CREATE TABLE "asignatura" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE "asistencia" (
    estudiante_id INTEGER NOT NULL,
    fecha TEXT NOT NULL,
    id INTEGER,
    estado TEXT CHECK(estado IN ('P', 'A', 'J')) NOT NULL,
    observacion TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT, sk TEXT,
    PRIMARY KEY (estudiante_id, fecha)
);

CREATE TABLE "colegio" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    orden INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT, tipo TEXT DEFAULT 'Primaria');

CREATE TABLE colegio_asignatura (
    colegio_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT, sk TEXT,
    PRIMARY KEY (colegio_id, asignatura_id)
);

CREATE TABLE colegio_dias_asistencia (
    colegio_id INTEGER PRIMARY KEY,
    lunes INTEGER DEFAULT 1,
    martes INTEGER DEFAULT 1,
    miercoles INTEGER DEFAULT 1,
    jueves INTEGER DEFAULT 1,
    viernes INTEGER DEFAULT 1,
    sabado INTEGER DEFAULT 1,
    domingo INTEGER DEFAULT 1,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE colegio_grado (colegio_id INTEGER NOT NULL, grado_id INTEGER NOT NULL, activo INTEGER DEFAULT 1, cualitativo INTEGER DEFAULT 0, updated_at TEXT, sk TEXT, device_id TEXT, PRIMARY KEY (colegio_id, grado_id));

CREATE TABLE configuracion_calificacion (
    id TEXT PRIMARY KEY,
    nombre TEXT NOT NULL,
    color TEXT NOT NULL,
    minimo INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE "corte_evaluativo" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    activo INTEGER DEFAULT 0,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE criterio (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    indicador_id INTEGER NOT NULL,
    orden INTEGER NOT NULL,
    descripcion TEXT DEFAULT '',
    puntaje_maximo INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE dia_sin_clases (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      colegio_id INTEGER NOT NULL,
      grado_id INTEGER,
      seccion_id INTEGER,
      fecha TEXT NOT NULL,
      descripcion TEXT NOT NULL DEFAULT '',
      updated_at TEXT
    , device_id TEXT, sk TEXT);

CREATE TABLE "estudiante" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL,
    codigo TEXT NOT NULL UNIQUE,
    sexo TEXT CHECK(sexo IN ('M', 'F')) NOT NULL,
    ano_lectivo_id INTEGER NOT NULL,
    grado_id INTEGER NOT NULL,
    seccion_id INTEGER NOT NULL,
    colegio_id INTEGER NOT NULL,
    telefono TEXT,
    direccion TEXT,
    email TEXT,
    foto TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE estudiante_retirado (estudiante_id INTEGER PRIMARY KEY, fecha_retiro TEXT, motivo TEXT, updated_at TEXT, sk TEXT, device_id TEXT);

CREATE TABLE "grado" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE grado_asignatura (
    grado_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT, sk TEXT,
    PRIMARY KEY (grado_id, asignatura_id)
);

CREATE TABLE indicador (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    grado_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    corte_evaluativo_id INTEGER NOT NULL,
    orden INTEGER NOT NULL,
    descripcion TEXT DEFAULT '',
    activo INTEGER DEFAULT 1,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, instrumento TEXT NOT NULL DEFAULT 'general', unidad_numero INTEGER, sk TEXT, ano_lectivo_id INTEGER DEFAULT 0, plantilla_id INTEGER);

CREATE TABLE indicador_colegio_seccion_activo (colegio_id INTEGER NOT NULL, seccion_id INTEGER NOT NULL, indicador_id INTEGER NOT NULL, activo INTEGER NOT NULL DEFAULT 0, updated_at TEXT, sk TEXT, device_id TEXT, PRIMARY KEY (colegio_id, seccion_id, indicador_id));

CREATE TABLE instrumento_plantilla (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      nombre TEXT,
      tipo TEXT DEFAULT 'escala',
      definicion TEXT NOT NULL DEFAULT '{}',
      updated_at TEXT,
      device_id TEXT
    , sk TEXT);

CREATE TABLE multiplicador_calificacion (
    id TEXT PRIMARY KEY,
    multiplicador REAL NOT NULL DEFAULT 1.0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE nota (
    estudiante_id INTEGER NOT NULL,
    criterio_id INTEGER NOT NULL,
    valor REAL NOT NULL,
    corte_evaluativo_id INTEGER NOT NULL,
    asignatura_id INTEGER NOT NULL,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT, sk TEXT,
    PRIMARY KEY (estudiante_id, criterio_id, corte_evaluativo_id, asignatura_id)
);

CREATE TABLE "seccion" (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre TEXT NOT NULL UNIQUE,
    cualitativo INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    device_id TEXT
, sk TEXT);

CREATE TABLE sync_metadata (
    key TEXT PRIMARY KEY,
    value INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE sync_tombstone (
      tabla TEXT NOT NULL,
      pk TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      device_id TEXT,
      sk TEXT DEFAULT '',
      PRIMARY KEY (tabla, pk, sk)
    );

CREATE TABLE "sync_tombstone_old" (
    tabla TEXT NOT NULL,
    pk TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    device_id TEXT,
    PRIMARY KEY (tabla, pk)
);

CREATE INDEX idx_ano_lectivo_sk ON "ano_lectivo"(sk, updated_at);

CREATE INDEX idx_ano_lectivo_updated ON "ano_lectivo"(updated_at);

CREATE INDEX idx_app_settings_sk ON app_settings(sk, updated_at);

CREATE INDEX idx_asignatura_sk ON "asignatura"(sk, updated_at);

CREATE INDEX idx_asignatura_updated ON "asignatura"(updated_at);

CREATE INDEX idx_asistencia_sk ON asistencia(sk, updated_at);

CREATE INDEX idx_asistencia_updated ON asistencia(updated_at);

CREATE INDEX idx_colegio_asignatura_sk ON colegio_asignatura(sk, updated_at);

CREATE INDEX idx_colegio_asignatura_updated ON colegio_asignatura(updated_at);

CREATE INDEX idx_colegio_dias_asistencia_sk ON colegio_dias_asistencia(sk, updated_at);

CREATE INDEX idx_colegio_dias_updated ON colegio_dias_asistencia(updated_at);

CREATE INDEX idx_colegio_grado_sk ON colegio_grado(sk, updated_at);

CREATE INDEX idx_colegio_sk ON "colegio"(sk, updated_at);

CREATE INDEX idx_colegio_updated ON "colegio"(updated_at);

CREATE INDEX idx_config_calif_updated ON configuracion_calificacion(updated_at);

CREATE INDEX idx_configuracion_calificacion_sk ON configuracion_calificacion(sk, updated_at);

CREATE INDEX idx_corte_evaluativo_sk ON "corte_evaluativo"(sk, updated_at);

CREATE INDEX idx_corte_updated ON "corte_evaluativo"(updated_at);

CREATE UNIQUE INDEX idx_criterio_indicador_orden ON criterio(indicador_id, orden);

CREATE INDEX idx_criterio_sk ON criterio(sk, updated_at);

CREATE INDEX idx_criterio_updated ON criterio(updated_at);

CREATE INDEX idx_dia_sin_clases_sk ON dia_sin_clases(sk, updated_at);

CREATE UNIQUE INDEX idx_dia_sin_clases_unica ON dia_sin_clases(colegio_id, IFNULL(grado_id, 0), IFNULL(seccion_id, 0), fecha);

CREATE INDEX idx_estudiante_retirado_sk ON estudiante_retirado(sk, updated_at);

CREATE INDEX idx_estudiante_sk ON "estudiante"(sk, updated_at);

CREATE INDEX idx_estudiante_updated ON "estudiante"(updated_at);

CREATE INDEX idx_grado_asignatura_sk ON grado_asignatura(sk, updated_at);

CREATE INDEX idx_grado_asignatura_updated ON grado_asignatura(updated_at);

CREATE INDEX idx_grado_sk ON "grado"(sk, updated_at);

CREATE INDEX idx_grado_updated ON "grado"(updated_at);

CREATE INDEX idx_indicador_colegio_seccion_activo_sk ON indicador_colegio_seccion_activo(sk, updated_at);

CREATE INDEX idx_indicador_sk ON indicador(sk, updated_at);

CREATE INDEX idx_indicador_updated ON indicador(updated_at);

CREATE UNIQUE INDEX idx_instrumento_plantilla_nombre ON instrumento_plantilla(nombre);

CREATE INDEX idx_instrumento_plantilla_sk ON instrumento_plantilla(sk, updated_at);

CREATE INDEX idx_mult_calif_updated ON multiplicador_calificacion(updated_at);

CREATE INDEX idx_multiplicador_calificacion_sk ON multiplicador_calificacion(sk, updated_at);

CREATE INDEX idx_nota_sk ON nota(sk, updated_at);

CREATE INDEX idx_nota_updated ON nota(updated_at);

CREATE INDEX idx_seccion_sk ON "seccion"(sk, updated_at);

CREATE INDEX idx_seccion_updated ON "seccion"(updated_at);

CREATE INDEX idx_tombstone_updated ON "sync_tombstone_old"(updated_at);

-- Marca de que el 'de-unique' ya se aplicó (el worker lo respeta y no toca las tablas).
INSERT OR REPLACE INTO sync_metadata (key, value) VALUES ('deunique_v2', 1);

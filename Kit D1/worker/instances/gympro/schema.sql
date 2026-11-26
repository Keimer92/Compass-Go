-- GymPro · esquema D1 para el worker del kit de sincronización.
--
-- Es la proyección en la nube de internal/db/schema.go, recortada a las
-- columnas del manifiesto (sync.schema.json): el motor sólo escribe las
-- columnas que declara, así que una columna de más en D1 queda muerta y una
-- de menos rompe el INSERT. Generar el manifiesto tras tocar este archivo:
--   go run ./cmd/gen-manifest -namespace gympro -o <kit>/worker/instances/gympro/sync.schema.json
--
-- No van `sync_status` (libro mayor por dispositivo) ni los BLOB biométricos
-- (user_fingerprints / fingerprint_templates) ni `settings` — ver la lista
-- `excluidas` de internal/syncschema.
--
-- Los UNIQUE de negocio se omiten a propósito: el worker arranca con
-- deUniqueTables() y el ON CONFLICT del upsert va sobre (id, sk), que es la
-- PK más la columna de inquilino que añade ensureSchema.
--
-- Idempotente: provision.mjs lo aplica con CREATE TABLE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS pricing_plans (
    id            TEXT PRIMARY KEY NOT NULL,
    name          TEXT NOT NULL,
    price         REAL NOT NULL,
    duration_days INTEGER NOT NULL,
    updated_at    TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS roles (
    id             TEXT PRIMARY KEY NOT NULL,
    name           TEXT NOT NULL,
    description    TEXT,
    permissions    TEXT NOT NULL,
    created_at     TEXT,
    is_system_role INTEGER DEFAULT 0,
    updated_at     TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS categories (
    id         TEXT PRIMARY KEY NOT NULL,
    name       TEXT NOT NULL,
    description TEXT,
    created_at TEXT,
    deleted_at TEXT,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY NOT NULL,
    username      TEXT NOT NULL,
    password_hash TEXT,
    full_name     TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'staff',
    created_at    TEXT,
    updated_at    TEXT DEFAULT (datetime('now')),
    is_active     INTEGER DEFAULT 1,
    photo_url     TEXT,
    deleted_at    TEXT
);

CREATE TABLE IF NOT EXISTS members (
    id                          TEXT PRIMARY KEY NOT NULL,
    full_name                   TEXT NOT NULL,
    email                       TEXT,
    phone                       TEXT,
    sex                         TEXT,
    fingerprint_id              TEXT,
    registration_date           TEXT,
    created_at                  TEXT,
    updated_at                  TEXT DEFAULT (datetime('now')),
    is_active                   INTEGER DEFAULT 1,
    deleted_at                  TEXT,
    last_payment_expiry_date    TEXT
);

CREATE TABLE IF NOT EXISTS payments (
    id             TEXT PRIMARY KEY NOT NULL,
    member_id      TEXT NOT NULL,
    user_id        TEXT,
    amount         REAL NOT NULL,
    discount       REAL DEFAULT 0,
    plan_name      TEXT,
    payment_date   TEXT NOT NULL,
    payment_method TEXT NOT NULL,
    expiry_date    TEXT NOT NULL,
    created_at     TEXT,
    deleted_at     TEXT,
    weight         REAL,
    updated_at     TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS attendance (
    id         TEXT PRIMARY KEY NOT NULL,
    member_id  TEXT NOT NULL,
    check_in   TEXT,
    deleted_at TEXT,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS products (
    id          TEXT PRIMARY KEY NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    price       REAL NOT NULL,
    cost        REAL DEFAULT 0,
    stock       INTEGER NOT NULL DEFAULT 0,
    min_stock   INTEGER DEFAULT 5,
    barcode     TEXT,
    category    TEXT,
    image_url   TEXT,
    created_at  TEXT,
    updated_at  TEXT DEFAULT (datetime('now')),
    deleted_at  TEXT
);

CREATE TABLE IF NOT EXISTS sales (
    id             TEXT PRIMARY KEY NOT NULL,
    user_id        TEXT,
    member_id      TEXT,
    total_amount   REAL NOT NULL,
    discount       REAL DEFAULT 0,
    payment_method TEXT DEFAULT 'cash',
    date           TEXT,
    due_date       TEXT,
    status         TEXT DEFAULT 'completed',
    deleted_at     TEXT,
    updated_at     TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sale_items (
    id         TEXT PRIMARY KEY NOT NULL,
    sale_id    TEXT NOT NULL,
    product_id TEXT NOT NULL,
    quantity   INTEGER NOT NULL,
    unit_price REAL NOT NULL,
    subtotal   REAL NOT NULL,
    deleted_at TEXT,
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id          TEXT PRIMARY KEY NOT NULL,
    user_id     TEXT,
    username    TEXT NOT NULL,
    action      TEXT NOT NULL,
    details     TEXT,
    entity_type TEXT,
    entity_id   TEXT,
    created_at  TEXT,
    updated_at  TEXT DEFAULT (datetime('now'))
);

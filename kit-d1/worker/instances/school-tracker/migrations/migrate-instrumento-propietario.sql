-- Templates are owned per user: identity goes from `nombre` to
-- (propietario, nombre). Existing rows become shared (propietario = '').
--
-- Run this BEFORE deploying the new worker:
--   npx wrangler d1 execute <db> --remote --file=./migrations/migrate-instrumento-propietario.sql --yes
--
-- Every statement is idempotent enough for a retry: the ALTER fails if the
-- column already exists (safe to ignore), the DROP uses IF EXISTS and the
-- CREATE uses IF NOT EXISTS.

ALTER TABLE instrumento_plantilla ADD COLUMN propietario TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_instrumento_plantilla_nombre;

-- The previously deployed worker created the conflict index on `nombre` alone.
-- `CREATE INDEX IF NOT EXISTS` would silently keep that definition, so it has
-- to be dropped first or the new key never applies.
DROP INDEX IF EXISTS idx_instrumento_plantilla_conflict;

CREATE UNIQUE INDEX IF NOT EXISTS idx_instrumento_plantilla_conflict
  ON instrumento_plantilla(propietario, nombre);

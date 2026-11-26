-- One row per ENROLLMENT (student + school year).
--
-- Promoting a student no longer deletes the origin row: the new enrollment is
-- inserted with `codigo = <codigo_base>@<year>` (its sync identity) and
-- `codigo_base` holds the code the user sees, which repeats across years.
-- Existing rows keep their `codigo` untouched (so the cloud does not see them
-- as brand-new students) and just get `codigo_base = codigo`.
--
-- Run this BEFORE deploying the new worker:
--   npx wrangler d1 execute <db> --remote --file=./migrations/migrate-estudiante-codigo-base.sql --yes
--
-- The ALTER fails if the column already exists (safe to ignore on a retry);
-- the UPDATE is idempotent.

ALTER TABLE estudiante ADD COLUMN codigo_base TEXT NOT NULL DEFAULT '';

UPDATE estudiante SET codigo_base = codigo WHERE codigo_base = '' OR codigo_base IS NULL;

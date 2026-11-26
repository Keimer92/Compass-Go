package syncengine_test

// Test bootstrap for the kit: opens a bare SQLite database, applies the
// example schema's DDL (syncable tables + the local infrastructure tables
// the engine assumes exist) and runs with the same PRAGMAs the apps use.
//
// The kit deliberately does NOT own app table creation in production (each
// app migrates its own database); this DDL only makes the engine's behavior
// testable end-to-end without depending on any app.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	schoolschema "github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine/example/school"
)

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

func abrirBase(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("abriendo la base: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Una sola conexión: los PRAGMAs se aplican una vez (igual que las apps).
	conn.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			t.Fatalf("no se pudo aplicar %q: %v", pragma, err)
		}
	}
	if _, err := conn.Exec(schoolschema.SchemaSQL); err != nil {
		t.Fatalf("aplicando el DDL de ejemplo: %v", err)
	}
	// El DDL base de la app es histórico; las columnas de infra que el motor
	// usa hoy (cursor de push + freno) llegaron después en las migraciones de
	// la app. En el kit forman parte del bootstrap de tests.
	for _, alter := range []string{
		"ALTER TABLE d1_config ADD COLUMN last_push_sync TEXT",
		"ALTER TABLE d1_config ADD COLUMN push_fail_count INTEGER",
		"ALTER TABLE d1_config ADD COLUMN push_retry_after TEXT",
	} {
		if _, err := conn.Exec(alter); err != nil && !isDuplicateColumn(err) {
			t.Fatalf("aplicando %q: %v", alter, err)
		}
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("encendiendo las FKs: %v", err)
	}
	return conn
}

// toString convierte un valor de JSON a string (los números vienen como
// float64/json.Number desde el scan).
func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

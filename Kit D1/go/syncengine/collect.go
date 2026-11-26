package syncengine

// Push payload: local rows modified since the cursor + pending deletions.
//
// Everything here is driven by the injected Registry: the engine knows no
// table names. The wire contract is defined by the worker
// (`cloudflare-sync/src/index.ts`): POST JSON with header `X-School-Key`.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Row is one table row as a column → JSON value map.
type Row map[string]any

func (r Row) clone() Row {
	out := make(Row, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

// Deletion is a tombstone: `{"tabla": ..., "pk": [...]}`.
//
// The `pk` values travel IN ORDER (the order of the table's canonical PK):
// that is how the worker builds the `DELETE` and the `sync_tombstone` row.
type Deletion struct {
	Tabla string `json:"tabla"`
	PK    []any  `json:"pk"`
}

// CollectLocalDataSince builds the push `data`: every row of every syncable
// table with `updated_at > since`, EXCLUDING rows a previous pull just
// downloaded (anti-echo).
//
// `recent` holds the "table|pk|updated_at" keys that already came from the
// cloud: those rows are not re-pushed until something changes locally.
func CollectLocalDataSince(reg *Registry, conn *sql.DB, since string, recent map[string]bool) (map[string][]Row, error) {
	out := map[string][]Row{}
	for _, spec := range reg.specs() {
		// No device_id filter (not every local table has one). The re-relay
		// loop is already cut upstream: the worker re-stamps `updated_at` with
		// its own clock and the two cursors (pull = server, push = local) keep
		// a pulled row out of the next push.
		//
		// Identifiers come from the validated Registry, never from user input;
		// the cursor itself always travels as a bound parameter.
		var (
			rows *sql.Rows
			err  error
		)
		if len(spec.Computed) > 0 {
			rows, err = conn.Query(selectWithComputed(spec), since)
		} else {
			rows, err = conn.Query(
				`SELECT * FROM "`+spec.Name+`" WHERE updated_at > ?`, since)
		}
		if err != nil {
			return nil, fmt.Errorf("Error query %s: %w", spec.Name, err)
		}
		list, err := scanRows(rows)
		if err != nil {
			return nil, fmt.Errorf("Error query %s: %w", spec.Name, err)
		}
		kept := make([]Row, 0, len(list))
		for _, r := range list {
			if recent[reg.RowRecentKey(spec.Name, r)] {
				continue
			}
			kept = append(kept, r)
		}
		out[spec.Name] = kept
	}
	return out, nil
}

// selectWithComputed reads a table attaching its computed protocol columns
// through a LEFT JOIN on the parent: the parent's business key is the row's
// cloud identity, because local ids diverge between devices.
func selectWithComputed(spec *TableSpec) string {
	projections := make([]string, 0, len(spec.Computed))
	joins := make([]string, 0, len(spec.Computed))
	for i, c := range spec.Computed {
		alias := "p" + strconv.Itoa(i)
		projections = append(projections,
			fmt.Sprintf(`%s."%s" AS "%s"`, alias, c.Target, c.Column))
		joins = append(joins,
			fmt.Sprintf(`LEFT JOIN "%s" %s ON %s."%s" = t."%s"`,
				c.Parent, alias, alias, c.ParentKey, c.FK))
	}
	return fmt.Sprintf(`SELECT t.*, %s FROM "%s" t %s WHERE t.updated_at > ?`,
		strings.Join(projections, ", "), spec.Name, strings.Join(joins, " "))
}

// scanRows converts *sql.Rows into `Row` values (JSON-compatible).
func scanRows(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := Row{}
		for i, c := range cols {
			m[c] = sqlValueToJSON(vals[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CollectPendingDeletions builds the push `deletions` list from the local
// pending-deletion tables.
//
// Each tombstone family lives in ITS OWN table: composite keys cannot share
// the generic `(tabla, registro_id)` one. Tables without a dedicated table
// travel through the generic one, by id.
func CollectPendingDeletions(reg *Registry, conn *sql.DB) ([]Deletion, error) {
	deletions := []Deletion{}

	rows, err := conn.Query("SELECT tabla, registro_id FROM sync_pending_deletions")
	if err != nil {
		return nil, err
	}
	generic, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	for _, r := range generic {
		tabla, _ := r["tabla"].(string)
		if spec, ok := reg.Spec(tabla); ok && spec.TombstoneTable != "" {
			// A table with a dedicated tombstone table must never travel here:
			// an id-based tombstone does not match the remote row, which holds
			// the id a different device gave it.
			continue
		}
		deletions = append(deletions, Deletion{Tabla: tabla, PK: []any{r["registro_id"]}})
	}

	for _, spec := range reg.specs() {
		if spec.TombstoneTable == "" {
			continue
		}
		list, err := queryColumns(conn, spec.TombstoneTable, tombstoneColumns(spec))
		if err != nil {
			return nil, err
		}
		for _, r := range list {
			pk := tombstonePK(spec, r)
			if emptyPK(pk) {
				continue
			}
			deletions = append(deletions, Deletion{Tabla: spec.Name, PK: pk})
		}
	}

	return deletions, nil
}

// tombstoneColumns lists the dedicated-table columns needed to rebuild the
// canonical PK: each stored PK column, plus for every computed PK component
// both the business code and the local FK (rows recorded before the code
// existed carry only the id, which is exact for what this device wrote).
func tombstoneColumns(spec *TableSpec) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, pk := range spec.PK {
		if c, ok := computedColumn(spec, pk); ok {
			add(c.FK)
			add(c.CodeCol)
			continue
		}
		add(pk)
	}
	return out
}

// tombstonePK rebuilds one row's canonical PK in Registry order.
func tombstonePK(spec *TableSpec, row Row) []any {
	pk := make([]any, 0, len(spec.PK))
	for _, col := range spec.PK {
		if c, ok := computedColumn(spec, col); ok {
			if codigo, _ := row[c.CodeCol].(string); codigo != "" {
				pk = append(pk, codigo)
				continue
			}
			pk = append(pk, row[c.FK])
			continue
		}
		pk = append(pk, row[col])
	}
	return pk
}

// emptyPK reports a tombstone row with no identity at all (an empty business
// key): it would delete nothing on the receiver, only echo a bogus tombstone.
func emptyPK(pk []any) bool {
	for _, v := range pk {
		switch x := v.(type) {
		case nil:
		case string:
			if x != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func queryColumns(conn *sql.DB, table string, cols []string) ([]Row, error) {
	if len(cols) == 0 {
		return nil, fmt.Errorf("tombstone table %q: no columns to read", table)
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
	}
	rows, err := conn.Query(`SELECT ` + strings.Join(quoted, ", ") + ` FROM "` + table + `"`)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

// ClearAllPendingDeletions drops the pending tombstones.
//
// ⚠️ Only called when the push was NOT skipped: if the retry brake skipped it,
// those deletions never reached the cloud and clearing them would lose them
// forever.
func ClearAllPendingDeletions(reg *Registry, exec execer) error {
	for _, t := range reg.TombstoneTables() {
		if _, err := exec.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	return nil
}

// execer is the minimum needed by writers (works with a connection or a
// transaction).
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// queryer is the minimum needed by readers that may run inside a transaction.
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// ---------------------------------------------------------------------------
// Value conversion
// ---------------------------------------------------------------------------

// sqlValueToJSON converts a SQLite value to its JSON equivalent.
func sqlValueToJSON(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int64:
		return json.Number(strconv.FormatInt(x, 10))
	case float64:
		return json.Number(strconv.FormatFloat(x, 'g', -1, 64))
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case []byte:
		// The Rust original turned blobs into null.
		return nil
	case string:
		return x
	case time.Time:
		return x.UTC().Format("2006-01-02 15:04:05")
	default:
		return nil
	}
}

// jsonValueString renders a JSON value as text (it keys the anti-echo set).
// Mirror of Rust's `Value::to_string()`: strings keep their JSON quotes.
func jsonValueString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		raw, err := json.Marshal(x)
		if err != nil {
			return x
		}
		return string(raw)
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(raw)
	}
}

// sqliteValue converts a JSON value into something SQLite accepts:
// null → NULL, bool → 0/1, integer number → INTEGER, real → REAL,
// text → TEXT, anything else → NULL.
func sqliteValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return nil
	case float64:
		if i := int64(x); float64(i) == x {
			return i
		}
		return x
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		return x
	default:
		return nil
	}
}

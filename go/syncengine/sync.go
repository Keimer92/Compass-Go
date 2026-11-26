package syncengine

// Sync engine: cursors, retry brake, push, pull and apply.
//
// The engine is schema-agnostic: every table it touches comes from the
// injected Registry (see registry.go). Quoting a name inside SQL is safe only
// because NewRegistry validated every identifier as ^[A-Za-z_][A-Za-z0-9_]*$.
//
// ⚠️ THE D1 QUOTA IS THE SCARCE RESOURCE: the free plan writes 100,000
// rows/day and a full push is ~8,000. Because of that:
//
//   - `last_push_sync` (the LOCAL push cursor) is NEVER nulled without a
//     clear reason (identity / reconciliation / real recovery);
//   - if the push fails, the cursor does NOT advance (if it did, local
//     changes that never reached the cloud would be marked as pushed →
//     silent data loss);
//   - pending deletions are NOT cleared if the push was skipped.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config is the D1 configuration (table `d1_config`, a single row).
//
// The JSON names are the Rust ones with `#[serde(rename_all = "camelCase")]`.
type Config struct {
	WorkerURL string `json:"workerUrl"`
	SchoolKey string `json:"schoolKey"`
	DeviceID  string `json:"deviceId"`
	// LastSync is the PULL cursor: SERVER time (authoritative).
	LastSync *string `json:"lastSync"`
	// LastPushSync is the PUSH cursor: LOCAL time.
	LastPushSync *string `json:"lastPushSync"`
}

// ReadConfig reads the configuration; nil when there is no row.
func ReadConfig(conn *sql.DB) (*Config, error) {
	var (
		cfg       Config
		lastSync  sql.NullString
		lastPush  sql.NullString
		workerURL string
		schoolKey string
		deviceID  string
	)
	err := conn.QueryRow(
		"SELECT worker_url, school_key, device_id, last_sync, last_push_sync FROM d1_config WHERE id = 1",
	).Scan(&workerURL, &schoolKey, &deviceID, &lastSync, &lastPush)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	cfg.WorkerURL = workerURL
	cfg.SchoolKey = schoolKey
	cfg.DeviceID = deviceID
	if lastSync.Valid {
		v := lastSync.String
		cfg.LastSync = &v
	}
	if lastPush.Valid {
		v := lastPush.String
		cfg.LastPushSync = &v
	}
	return &cfg, nil
}

// WriteConfig saves the configuration (upsert of the row id = 1).
func WriteConfig(conn *sql.DB, cfg *Config) error {
	var lastSync, lastPush any
	if cfg.LastSync != nil {
		lastSync = *cfg.LastSync
	}
	if cfg.LastPushSync != nil {
		lastPush = *cfg.LastPushSync
	}
	_, err := conn.Exec(
		"INSERT INTO d1_config (id, worker_url, school_key, device_id, last_sync, last_push_sync, updated_at) "+
			"VALUES (1, ?1, ?2, ?3, ?4, ?5, datetime('now')) "+
			"ON CONFLICT(id) DO UPDATE SET worker_url = ?1, school_key = ?2, device_id = ?3, "+
			"last_sync = ?4, last_push_sync = ?5, updated_at = datetime('now')",
		cfg.WorkerURL, cfg.SchoolKey, cfg.DeviceID, lastSync, lastPush,
	)
	return err
}

// GenerateDeviceID returns a fresh device identifier.
//
// A variable (not a function) so wire-fixture tests can pin the clock/random
// and replay the session byte-for-byte. Production behavior is unchanged.
var GenerateDeviceID = func() string {
	now := time.Now()
	return fmt.Sprintf("dev-%d-%x", now.UnixMilli(), now.UnixNano())
}

// NowSQLite is the local time in SQLite format ("YYYY-MM-DD HH:MM:SS").
//
// Pinnable for the same reason as `GenerateDeviceID`.
var NowSQLite = func() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

// PushBackoffMin are the minutes to wait between push retries.
//
// The last value is the cap: from there on every attempt waits 4 h.
var PushBackoffMin = []int64{5, 15, 30, 60, 120, 240}

// PushEnEspera reports whether the retry brake is armed and how long is left.
//
// Returns ("", false) when there is no brake.
func PushEnEspera(conn *sql.DB) (string, bool) {
	var hasta sql.NullString
	if err := conn.QueryRow("SELECT push_retry_after FROM d1_config WHERE id = 1").Scan(&hasta); err != nil {
		return "", false
	}
	if !hasta.Valid || hasta.String == "" {
		return "", false
	}
	var minutos int64
	if err := conn.QueryRow(
		"SELECT CAST((julianday(?1) - julianday('now')) * 24 * 60 AS INTEGER)", hasta.String,
	).Scan(&minutos); err != nil {
		return "", false
	}
	if minutos > 0 {
		return fmt.Sprintf("%d min", minutos), true
	}
	return "", false
}

// RegistrarPushFallido raises the failure counter and re-arms the brake.
func RegistrarPushFallido(conn *sql.DB, motivo string) error {
	var prev int64
	_ = conn.QueryRow("SELECT COALESCE(push_fail_count, 0) FROM d1_config WHERE id = 1").Scan(&prev)
	n := prev + 1
	idx := n - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= int64(len(PushBackoffMin)) {
		idx = int64(len(PushBackoffMin)) - 1
	}
	minutos := PushBackoffMin[idx]
	_, err := conn.Exec(
		"UPDATE d1_config SET push_fail_count = ?1, push_retry_after = datetime('now', ?2), "+
			"updated_at = datetime('now') WHERE id = 1",
		n, fmt.Sprintf("+%d minutes", minutos),
	)
	return err
}

// LimpiarPushFallidos releases the brake (a push or a sync went well).
func LimpiarPushFallidos(exec execer) {
	_, _ = exec.Exec("UPDATE d1_config SET push_fail_count = 0, push_retry_after = NULL WHERE id = 1")
}

// ---------------------------------------------------------------------------
// The sync
// ---------------------------------------------------------------------------

// Emitter sends an event to the frontend (e.g. sync progress).
type Emitter func(name string, data any)

// Options are the options of `d1_sync_all`.
type Options struct {
	// Source is where the sync was triggered from (diagnostics).
	Source string
	// Full ignores the cursors and pushes the whole database (account
	// change, .db import). The ONLY justified reason for a full push.
	Full bool
	// Force ignores the retry brake: manual user buttons ONLY. Never pass
	// it from an automatic trigger.
	Force bool
}

// Result is the outcome of `d1_sync_all` (same fields as the Rust).
type Result struct {
	OK             bool           `json:"ok"`
	Pushed         map[string]int `json:"pushed"`
	PushOmitido    bool           `json:"pushOmitido"`
	Pulled         map[string]int `json:"pulled"`
	DeletedApplied int64          `json:"deletedApplied"`
	LastSync       string         `json:"lastSync"`
}

// SyncAll: bilateral sync (push local + pull cloud + apply).
var syncAllMu sync.Mutex

func SyncAll(reg *Registry, conn *sql.DB, client *Client, opts Options, emit Emitter) (*Result, error) {
	syncAllMu.Lock()
	defer syncAllMu.Unlock()

	nowEpoch := time.Now().Unix()

	// ── Local phase ────────────────────────────────────────────────────────
	cfg, err := ReadConfig(conn)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("D1 no está configurado")
	}

	// Anti re-relay: remember rows freshly PULLED so they are not pushed
	// again until they change locally. Elastic cut: without this memory,
	// rows stamped "in the future" (crooked clocks) were re-sent by every
	// device and reverted other people's changes.
	if _, err := conn.Exec(
		"CREATE TABLE IF NOT EXISTS sync_recent_pulls (" +
			"tabla TEXT NOT NULL, pk TEXT NOT NULL, updated_at TEXT NOT NULL, ts INTEGER NOT NULL, " +
			"PRIMARY KEY (tabla, pk))",
	); err != nil {
		return nil, err
	}
	if _, err := conn.Exec("DELETE FROM sync_recent_pulls WHERE ts < ?", nowEpoch-86400); err != nil {
		return nil, err
	}

	recent := map[string]bool{}
	if !opts.Full {
		rows, err := conn.Query("SELECT tabla, pk, updated_at FROM sync_recent_pulls")
		if err != nil {
			return nil, err
		}
		lista, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		for _, r := range lista {
			tabla, _ := r["tabla"].(string)
			pk, _ := r["pk"].(string)
			ua, _ := r["updated_at"].(string)
			recent[tabla+"|"+pk+"|"+ua] = true
		}
	}

	// TWO independent cursors:
	//  - pushSince = LOCAL clock (comparable with the updated_at this device
	//    stamps on its own rows) → never eats its own edits.
	//  - since (pull) = SERVER clock (authoritative across devices).
	pushSince := "1970-01-01 00:00:00"
	if !opts.Full && cfg.LastPushSync != nil {
		pushSince = *cfg.LastPushSync
	}
	since := "1970-01-01 00:00:00"
	if cfg.LastSync != nil {
		since = *cfg.LastSync
	}

	// ── 0. Orphan purge: rows whose parent is gone on THIS device ──────────
	nHuerfanas, err := purgeOrphans(reg, conn)
	if err != nil {
		return nil, err
	}
	if nHuerfanas > 0 {
		log.Printf("[d1] purga de huérfanos: %d filas borradas localmente (sin tombstone)", nHuerfanas)
	}

	// ── 1. PUSH (local cursor) ─────────────────────────────────────────────
	data, err := CollectLocalDataSince(reg, conn, pushSince, recent)
	if err != nil {
		return nil, err
	}
	deletions, err := CollectPendingDeletions(reg, conn)
	if err != nil {
		return nil, err
	}
	totalPush := 0
	for _, filas := range data {
		totalPush += len(filas)
	}

	espera, frenado := PushEnEspera(conn)
	if frenado && !opts.Force {
		// The push is skipped: pending changes are NOT lost (the push cursor
		// does not advance), they are just deferred until the wait expires.
		log.Printf("[D1] push OMITIDO por el freno de reintentos (faltan %s). "+
			"El pull sigue adelante; los %d cambios pendientes se reintentan al vencer la espera.",
			espera, totalPush)
	} else if frenado {
		log.Printf("[D1] push FORZADO por el usuario (freno activo, quedaban %s).", espera)
	}

	// The push cursor acknowledges at least the newest `updated_at` included
	// in this push: otherwise future-dated rows would be re-pushed on every
	// start until the local clock catches up.
	pushCursor := NowSQLite()
	for _, filas := range data {
		for _, fila := range filas {
			ua, ok := fila["updated_at"].(string)
			if ok && ua > pushCursor {
				pushCursor = ua
			}
		}
	}

	// ── Retry brake ────────────────────────────────────────────────────────
	// If the previous push failed and we are still inside the wait window,
	// the push is SKIPPED and only the pull runs (it reads, does not write,
	// so it does not consume the write quota). Pending changes are NOT lost:
	// `last_push_sync` does not advance.
	pushOmitido := frenado && !opts.Force

	var pushResp map[string]any
	switch {
	case pushOmitido:
		pushResp = map[string]any{"ok": true, "applied": map[string]any{}, "omitidoPorFreno": true}
	case totalPush == 0 && len(deletions) == 0:
		pushResp = map[string]any{"ok": true, "applied": map[string]any{}}
	default:
		emitProgress(emit, "Subiendo cambios a la nube…")
		pushResp, err = client.Post("/push", map[string]any{
			"since":     pushSince,
			"deviceId":  cfg.DeviceID,
			"data":      data,
			"deletions": deletions,
		})
		if err != nil {
			// The failure is recorded BEFORE propagating. The sync aborts
			// here (no pull) on purpose: the pull could overwrite local
			// changes that have not been pushed yet.
			_ = RegistrarPushFallido(conn, err.Error())
			return nil, err
		}
	}

	// ── 1b. PUSH cursor saved RIGHT AWAY, before touching the cloud ────────
	// The push above finished cleanly: those rows ARE in D1. Previously the
	// cursor was only saved inside the apply transaction, so if the PULL
	// failed the UPDATE never ran and the NEXT trigger pushed exactly the
	// same rows again (~8,000 per push: the daily quota drained by itself).
	//
	// If the push was skipped by the brake the cursor is NOT touched (those
	// changes are still missing from the cloud and must be retried).
	if !pushOmitido {
		if _, err := conn.Exec(
			"UPDATE d1_config SET last_push_sync = ?1, updated_at = datetime('now') WHERE id = 1",
			pushCursor,
		); err != nil {
			return nil, err
		}
	}

	// ── 2. PULL ────────────────────────────────────────────────────────────
	emitProgress(emit, "Descargando cambios de la nube…")
	pullResp, err := client.Post("/pull", map[string]any{
		"since":    since,
		"deviceId": cfg.DeviceID,
	})
	if err != nil {
		// A failed pull brakes the sync too: otherwise every trigger would
		// repeat the push+pull cycle without pause.
		motivo := "Error en pull: " + err.Error()
		_ = RegistrarPushFallido(conn, motivo)
		return nil, errors.New(motivo)
	}

	// ── 3. Apply ───────────────────────────────────────────────────────────
	emitProgress(emit, "Guardando datos localmente…")
	pulled, deletedApplied, newSync, err := applyPull(reg, conn, pullResp, cfg.DeviceID, deletions, pushOmitido, pushCursor, nowEpoch)
	if err != nil {
		return nil, err
	}

	return &Result{
		OK:             true,
		Pushed:         appliedCounts(pushResp),
		PushOmitido:    pushOmitido,
		Pulled:         pulled,
		DeletedApplied: deletedApplied,
		LastSync:       newSync,
	}, nil
}

// purgeOrphans deletes LOCALLY the rows whose referenced parent no longer
// exists on this device, returning how many were removed.
//
// They generate NO tombstone. An orphan row arrived from the cloud with ANOTHER
// device's id: this device cannot tell which business row it belongs to, and
// tombstoning it propagated a deletion that killed the good row on EVERY
// device (the cycle repeated each time somebody marked it again). Real
// deletions are tombstoned when they happen, by business key.
func purgeOrphans(reg *Registry, exec execer) (int64, error) {
	var total int64
	for _, spec := range reg.specs() {
		o := spec.Orphan
		if o == nil {
			continue
		}
		res, err := exec.Exec(fmt.Sprintf(
			`DELETE FROM "%s" WHERE "%s" NOT IN (SELECT "%s" FROM "%s")`,
			spec.Name, o.FK, o.ParentKey, o.Parent))
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// applyPull applies the pull response in ONE transaction with
// `foreign_keys = OFF` (like the Rust).
func applyPull(
	reg *Registry,
	conn *sql.DB,
	pullResp map[string]any,
	deviceID string,
	deletions []Deletion,
	pushOmitido bool,
	pushCursor string,
	nowEpoch int64,
) (map[string]int, int64, string, error) {
	// The PRAGMA has no effect inside a transaction: it goes before BEGIN.
	if _, err := conn.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return nil, 0, "", err
	}
	tx, err := conn.Begin()
	if err != nil {
		_, _ = conn.Exec("PRAGMA foreign_keys = ON")
		return nil, 0, "", err
	}
	defer func() {
		_ = tx.Rollback()
		_, _ = conn.Exec("PRAGMA foreign_keys = ON")
	}()

	pulled := map[string]int{}
	var omitidas int
	// Resolves a business code to its LOCAL parent id. Lazy: the first
	// query happens when the first dependent row arrives, once the parents
	// of THIS same pass are already applied.
	refs := newLocalRefs(tx)
	if filasObj, ok := pullResp["rows"].(map[string]any); ok {
		// The Registry ORDER matters: a parent is declared before the rows
		// reconciled by its business key. Iterating the response map (random
		// order) would let a child arrive before its parent and be dropped.
		for _, table := range reg.Tables() {
			arr, ok := filasObj[table].([]any)
			if !ok {
				continue
			}
			n := 0
			for _, entry := range arr {
				obj, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				row := Row(obj)
				aplicada, err := upsertRow(reg, tx, table, row, refs)
				if err != nil {
					return nil, 0, "", err
				}
				if !aplicada {
					omitidas++
					continue
				}
				// Record in the anti-echo set: this row (pk + stamp) must not
				// be pushed again without a local modification.
				ua, _ := row["updated_at"].(string)
				if _, err := tx.Exec(
					"INSERT INTO sync_recent_pulls (tabla, pk, updated_at, ts) VALUES (?1, ?2, ?3, ?4) "+
						"ON CONFLICT(tabla, pk) DO UPDATE SET updated_at = excluded.updated_at, ts = excluded.ts",
					table, reg.PKPart(table, row), ua, nowEpoch,
				); err != nil {
					return nil, 0, "", err
				}
				n++
			}
			pulled[table] = n
		}
	}
	if omitidas > 0 {
		log.Printf("[d1] pull: %d filas se descartaron porque su referencia no corresponde a este dispositivo",
			omitidas)
	}

	var deletedApplied int64
	if deletedArr, ok := pullResp["deleted"].([]any); ok {
		for _, d := range deletedArr {
			del, ok := d.(map[string]any)
			if !ok {
				continue
			}
			tabla, _ := del["tabla"].(string)
			pk := del["pk"]
			// Ignore tombstones created by this same device (already applied
			// locally).
			dev, _ := del["device_id"].(string)
			if dev == deviceID {
				continue
			}
			if err := deleteByPk(reg, tx, tabla, pk); err != nil {
				return nil, 0, "", err
			}
			deletedApplied++
		}
	}

	// Confirm: the local pending deletions were applied on the worker.
	// ⚠️ Only if the push was NOT skipped.
	if !pushOmitido {
		if err := ClearAllPendingDeletions(reg, tx); err != nil {
			return nil, 0, "", err
		}
	}

	// Authoritative SERVER cursor (Cloudflare's single clock).
	newSync := NowSQLite()
	if st, ok := pullResp["serverTime"].(string); ok && st != "" {
		newSync = st
	}

	if pushOmitido {
		// ⚠️ The push was skipped by the brake: `last_push_sync` MUST NOT
		// advance. If it did, local changes that never reached the cloud
		// would be marked "already pushed" and lost forever.
		if _, err := tx.Exec(
			"UPDATE d1_config SET last_sync = ?1, updated_at = datetime('now') WHERE id = 1",
			newSync,
		); err != nil {
			return nil, 0, "", err
		}
	} else {
		if _, err := tx.Exec(
			"UPDATE d1_config SET last_sync = ?1, last_push_sync = ?2, updated_at = datetime('now') WHERE id = 1",
			newSync, pushCursor,
		); err != nil {
			return nil, 0, "", err
		}
		// The push arrived: if it came after an earlier failure, release the
		// brake.
		LimpiarPushFallidos(tx)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, "", err
	}
	return pulled, deletedApplied, newSync, nil
}

// appliedCounts extracts "applied" from the push response (table → rows).
func appliedCounts(pushResp map[string]any) map[string]int {
	out := map[string]int{}
	applied, ok := pushResp["applied"].(map[string]any)
	if !ok {
		return out
	}
	for tabla, n := range applied {
		switch v := n.(type) {
		case json.Number:
			if i, err := v.Int64(); err == nil {
				out[tabla] = int(i)
			}
		case float64:
			out[tabla] = int(v)
		}
	}
	return out
}

// emitProgress sends the sync-progress event.
func emitProgress(emit Emitter, mensaje string) {
	if emit == nil {
		return
	}
	emit("d1-sync-progress", map[string]any{"message": mensaje})
}

// ---------------------------------------------------------------------------
// Upserts (the pull apply)
// ---------------------------------------------------------------------------

// upsertRow inserts or updates one row pulled from the cloud, following the
// strategy declared in the Registry for its table.
//
// It returns `false` when the row was dropped because the parent it references
// does not exist on THIS device: storing it with another device's id would
// orphan the row (and the purge would turn it into a deletion for every other
// device) or, worse, attach it to a different parent that happens to share
// that id locally.
func upsertRow(reg *Registry, exec execer, table string, row Row, refs *localRefs) (bool, error) {
	spec, ok := reg.Spec(table)
	if !ok {
		return false, fmt.Errorf("unknown syncable table %q in pull", table)
	}
	switch spec.Strategy {
	case StrategyKeyUpsert:
		rewritten, ok, err := rewriteComputed(spec, row, refs)
		if err != nil || !ok {
			return ok, err
		}
		return true, keyUpsert(exec, spec, rewritten)
	case StrategyFKRewrite:
		rewritten, ok, err := rewriteComputed(spec, row, refs)
		if err != nil || !ok {
			return ok, err
		}
		return true, insertOrReplace(exec, spec, rewritten)
	case StrategyValidateOnly:
		matches, err := computedMatches(spec, row, refs)
		if err != nil || !matches {
			return false, err
		}
		return true, insertOrReplace(exec, spec, row)
	case StrategyCatalog:
		return true, upsertCatalog(exec, spec, row)
	default:
		return true, insertOrReplace(exec, spec, row)
	}
}

// rewriteComputed copies the row re-writing each computed FK to the LOCAL
// parent id resolved through the business code. `ok=false` means the parent
// does not exist on this device (drop the row). Without a code on the wire
// (old worker or legacy row) the received id is kept, as before.
func rewriteComputed(spec *TableSpec, row Row, refs *localRefs) (Row, bool, error) {
	if len(spec.Computed) == 0 {
		return row, true, nil
	}
	out := row.clone()
	for i := range spec.Computed {
		c := &spec.Computed[i]
		code, _ := row[c.Column].(string)
		if code == "" {
			continue
		}
		localID, ok, err := refs.keyFor(c, code)
		if err != nil || !ok {
			return nil, ok, err
		}
		out[c.FK] = localID
	}
	return out, true, nil
}

// computedMatches validates a row that cannot be re-written: it is applied
// ONLY when its computed FK equals the local id of the parent carried by the
// business code. A row that would point at a different parent here is dropped
// and stays in the cloud until both devices agree.
func computedMatches(spec *TableSpec, row Row, refs *localRefs) (bool, error) {
	for i := range spec.Computed {
		c := &spec.Computed[i]
		code, _ := row[c.Column].(string)
		if code == "" {
			// Without a code there is no way to know which parent the row is
			// for. Keep the old behavior only if that id IS a parent here;
			// otherwise drop it (an orphan row is invisible to the app).
			exists, err := refs.hasKey(c, row[c.FK])
			if err != nil || !exists {
				return false, err
			}
			continue
		}
		localID, ok, err := refs.keyFor(c, code)
		if err != nil || !ok {
			return false, err
		}
		if jsonValueString(localID) != jsonValueString(row[c.FK]) {
			return false, nil
		}
	}
	return true, nil
}

// localRefs resolves the parent references of computed columns.
//
// The business code is the same on every device while local ids diverge. Each
// parent is loaded once per pass, AFTER the parents of the same pull were
// applied (hence the Registry's declared order).
type localRefs struct {
	query   queryer
	parents map[string]*parentIndex
}

func newLocalRefs(query queryer) *localRefs {
	return &localRefs{query: query, parents: map[string]*parentIndex{}}
}

type parentIndex struct {
	byTarget map[string]any
	keys     map[string]bool
	cargada  bool
}

// index returns (lazily loading) the parent index for one computed column.
func (r *localRefs) index(c *ComputedFK) (*parentIndex, error) {
	cacheKey := c.Parent + "|" + c.ParentKey + "|" + c.Target
	if idx, ok := r.parents[cacheKey]; ok && idx.cargada {
		return idx, nil
	}
	rows, err := r.query.Query(fmt.Sprintf(`SELECT "%s", "%s" FROM "%s"`,
		c.Target, c.ParentKey, c.Parent))
	if err != nil {
		return nil, err
	}
	list, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	idx := &parentIndex{byTarget: map[string]any{}, keys: map[string]bool{}}
	for _, row := range list {
		target, _ := row[c.Target].(string)
		key := row[c.ParentKey]
		idx.byTarget[target] = key
		idx.keys[jsonValueString(key)] = true
	}
	idx.cargada = true
	r.parents[cacheKey] = idx
	return idx, nil
}

// keyFor returns the local parent key for a business code.
func (r *localRefs) keyFor(c *ComputedFK, code string) (any, bool, error) {
	idx, err := r.index(c)
	if err != nil {
		return nil, false, err
	}
	key, ok := idx.byTarget[code]
	return key, ok, nil
}

// hasKey reports whether this database contains a parent with that key id.
func (r *localRefs) hasKey(c *ComputedFK, key any) (bool, error) {
	idx, err := r.index(c)
	if err != nil {
		return false, err
	}
	return idx.keys[jsonValueString(key)], nil
}

// insertOrReplace writes the row with INSERT OR REPLACE using the JSON
// columns, id included. Computed protocol columns are skipped: they are not
// columns of the local table.
func insertOrReplace(exec execer, spec *TableSpec, row Row) error {
	cols := payloadColumns(spec, row, true)
	if len(cols) == 0 {
		return nil
	}
	placeholders := make([]string, len(cols))
	vals := make([]any, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		vals[i] = sqliteValue(row[c])
	}
	sql := fmt.Sprintf(`INSERT OR REPLACE INTO "%s" (%s) VALUES (%s)`,
		spec.Name, strings.Join(quoteCols(cols), ", "), strings.Join(placeholders, ", "))
	_, err := exec.Exec(sql, vals...)
	return err
}

// payloadColumns lists the table columns carried by a row: every key except
// the computed protocol ones, and except `id` when keepID is false.
func payloadColumns(spec *TableSpec, row Row, keepID bool) []string {
	computed := make(map[string]bool, len(spec.Computed))
	for i := range spec.Computed {
		computed[spec.Computed[i].Column] = true
	}
	cols := make([]string, 0, len(row))
	for c := range row {
		if computed[c] {
			continue
		}
		if c == "id" && !keepID {
			continue
		}
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}

func quoteCols(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = `"` + c + `"`
	}
	return out
}

// upsertCatalog updates the existing local row (keeping its id) or inserts a
// new one (local autoincrement) when the business key is unknown.
//
// It avoids colliding with UNIQUE(name/code) when local ids diverge from the
// remote ones after reseeds/restores.
func upsertCatalog(exec execer, spec *TableSpec, row Row) error {
	cols := payloadColumns(spec, row, false)
	if len(cols) == 0 {
		return nil
	}
	setClause := make([]string, len(cols))
	vals := make([]any, 0, len(cols)+len(spec.BusinessKey))
	for i, c := range cols {
		setClause[i] = `"` + c + `" = ?`
		vals = append(vals, sqliteValue(row[c]))
	}
	where := make([]string, len(spec.BusinessKey))
	for i, bk := range spec.BusinessKey {
		where[i] = `"` + bk + `" = ?`
		vals = append(vals, sqliteValue(row[bk]))
	}
	updateSQL := fmt.Sprintf(`UPDATE "%s" SET %s WHERE %s`,
		spec.Name, strings.Join(setClause, ", "), strings.Join(where, " AND "))
	res, err := exec.Exec(updateSQL, vals...)
	if err != nil {
		return err
	}
	updated, _ := res.RowsAffected()
	if updated > 0 {
		return nil
	}
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		spec.Name, strings.Join(quoteCols(cols), ", "), strings.Join(placeholders, ", "))
	_, err = exec.Exec(insertSQL, vals[:len(cols)]...)
	return err
}

// keyUpsert upserts by the table's local conflict target (its business
// identity after FK rewriting), updating only the listed columns and keeping
// the local row for the same identity instead of colliding on divergent ids.
func keyUpsert(exec execer, spec *TableSpec, row Row) error {
	cols := payloadColumns(spec, row, spec.Upsert.IncludeID)
	if len(cols) == 0 {
		return nil
	}
	set := spec.Upsert.Set
	if len(set) == 0 {
		conflict := inSet(spec.Upsert.Conflict)
		for _, c := range cols {
			if !conflict[c] {
				set = append(set, c)
			}
		}
	}
	placeholders := make([]string, len(cols))
	vals := make([]any, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		vals[i] = sqliteValue(row[c])
	}
	setClause := make([]string, len(set))
	for i, c := range set {
		setClause[i] = `"` + c + `" = excluded."` + c + `"`
	}
	sql := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s`,
		spec.Name, strings.Join(quoteCols(cols), ", "), strings.Join(placeholders, ", "),
		strings.Join(quoteCols(spec.Upsert.Conflict), ", "), strings.Join(setClause, ", "))
	_, err := exec.Exec(sql, vals...)
	return err
}

func inSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, x := range list {
		out[x] = true
	}
	return out
}

// deleteByPk deletes locally by canonical PK (values in Registry order).
//
// A computed PK component arrives as the parent's business code (string): the
// delete resolves it through the parent table, because the local FK id of the
// deleting device means nothing here. A numeric value is a legacy tombstone:
// exact for what this device wrote, so it deletes by id.
func deleteByPk(reg *Registry, exec execer, table string, pk any) error {
	spec, ok := reg.Spec(table)
	if !ok {
		return nil
	}
	arr, _ := pk.([]any)
	where := make([]string, 0, len(spec.PK))
	vals := make([]any, 0, len(spec.PK))
	for i, col := range spec.PK {
		var v any
		if i < len(arr) {
			v = arr[i]
		}
		if c, isComputed := computedColumn(spec, col); isComputed {
			if _, esCodigo := v.(string); esCodigo {
				where = append(where, fmt.Sprintf(`"%s" IN (SELECT "%s" FROM "%s" WHERE "%s" = ?)`,
					c.FK, c.ParentKey, c.Parent, c.Target))
			} else {
				where = append(where, `"`+c.FK+`" = ?`)
			}
			vals = append(vals, sqliteValue(v))
			continue
		}
		where = append(where, `"`+col+`" = ?`)
		vals = append(vals, sqliteValue(v))
	}
	if len(where) == 0 {
		return nil
	}
	_, err := exec.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE %s`,
		spec.Name, strings.Join(where, " AND ")), vals...)
	return err
}

// ---------------------------------------------------------------------------
// Cloud commands
// ---------------------------------------------------------------------------

// RemoteStats: how many rows the cloud holds for this school key.
func RemoteStats(client *Client) (map[string]any, error) {
	return client.Post("/stats", map[string]any{})
}

// DailyUsage: daily row usage (Cloudflare Analytics).
func DailyUsage(client *Client) (map[string]any, error) {
	return client.Post("/usage", map[string]any{})
}

// Status: connection state (tested against `/hello`).
func Status(client *Client) (map[string]any, error) {
	return client.Post("/hello", map[string]any{})
}

// WipeRemote purges the cloud completely for this profile (school key)
// without touching the local configuration.
//
// Strategy: a full /pull with a "probe" deviceId (the worker excludes the
// rows written by its own device_id from /pull; a different id returns
// EVERYTHING), then a /push per batch with ALL the PKs as tombstones. The
// cursors end NULL so the next sync is a full push+pull.
func WipeRemote(reg *Registry, conn *sql.DB, client *Client) (map[string]any, error) {
	cfg, err := ReadConfig(conn)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("D1 no está configurado")
	}

	// ── 1. Full pull (probe deviceId → own rows are not excluded) ──
	resp, err := client.Post("/pull", map[string]any{
		"since":    "1970-01-01 00:00:00",
		"deviceId": "wipe-probe",
	})
	if err != nil {
		return nil, err
	}

	// ── 2. Build [{tabla, pk}] with the cloud-stored PKs in order ──
	deletions := []Deletion{}
	if filasObj, ok := resp["rows"].(map[string]any); ok {
		for _, table := range reg.Tables() {
			arr, ok := filasObj[table].([]any)
			if !ok {
				continue
			}
			cols := reg.CloudPK(table)
			for _, entry := range arr {
				obj, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				row := Row(obj)
				pk := make([]any, 0, len(cols))
				// The wipe operates on rows of THE CLOUD: their exact key is
				// what is stored there (the FK id), not the protocol code.
				for _, c := range cols {
					pk = append(pk, row[c])
				}
				deletions = append(deletions, Deletion{Tabla: table, PK: pk})
			}
		}
	}
	total := len(deletions)

	// ── 3. Push the tombstones in batches (the worker builds 2 statements
	// per row) ──
	for i := 0; i < len(deletions); i += 100 {
		fin := i + 100
		if fin > len(deletions) {
			fin = len(deletions)
		}
		if _, err := client.Post("/push", map[string]any{
			"deviceId":  cfg.DeviceID,
			"data":      map[string]any{},
			"deletions": deletions[i:fin],
		}); err != nil {
			return nil, err
		}
	}

	// ── 4. Cursors NULL → the next sync is a full push+pull ──
	if _, err := conn.Exec(
		"UPDATE d1_config SET last_sync = NULL, last_push_sync = NULL, updated_at = datetime('now') WHERE id = 1",
	); err != nil {
		return nil, err
	}

	return map[string]any{"ok": true, "deleted": total}, nil
}

// ResetRemote deletes EVERY row and EVERY tombstone in the cloud.
//
// Used BEFORE importing a backup: residual tombstones (newer than the
// restored data) would delete the freshly pushed rows on the next pull.
func ResetRemote(conn *sql.DB, client *Client) (map[string]any, error) {
	cfg, err := ReadConfig(conn)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("D1 no está configurado")
	}
	resp, err := client.Post("/reset", map[string]any{})
	if err != nil {
		return nil, err
	}
	// Cursors NULL → the next sync is a full push+pull from epoch.
	if _, err := conn.Exec(
		"UPDATE d1_config SET last_sync = NULL, last_push_sync = NULL, updated_at = datetime('now') WHERE id = 1",
	); err != nil {
		return nil, err
	}
	return resp, nil
}

// RestoreFromCloud: authoritative restore from the cloud.
//
// Differences with SyncAll:
//   - Regenerates the device_id BEFORE the pull: the worker excludes from
//     /pull the rows written by the same device_id, so reusing the old id
//     would return an EMPTY pull.
//   - DELETES every restored local table first (the pull is the source of
//     truth and cannot collide on UNIQUE columns with different ids).
//   - Does NOT push: this is a cloud→local restoration.
func RestoreFromCloud(reg *Registry, conn *sql.DB, client *Client, emit Emitter) (map[string]any, error) {
	nowEpoch := time.Now().Unix()

	// 1) Config: regenerate device_id and null the cursors for a full pull.
	cfg, err := ReadConfig(conn)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("D1 no está configurado")
	}
	cfg.DeviceID = GenerateDeviceID()
	cfg.LastSync = nil
	cfg.LastPushSync = nil
	if err := WriteConfig(conn, cfg); err != nil {
		return nil, err
	}

	// 2) FULL PULL first. If the pull fails, the local DB stays intact.
	emitProgress(emit, "Descargando datos de la nube…")
	pullResp, err := client.Post("/pull", map[string]any{
		"since":    "1970-01-01 00:00:00",
		"deviceId": cfg.DeviceID,
	})
	if err != nil {
		return nil, err
	}

	// 3) Apply in ONE transaction as an authoritative restore.
	emitProgress(emit, "Guardando datos localmente…")
	if _, err := conn.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return nil, err
	}
	tx, err := conn.Begin()
	if err != nil {
		_, _ = conn.Exec("PRAGMA foreign_keys = ON")
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
		_, _ = conn.Exec("PRAGMA foreign_keys = ON")
	}()

	for _, table := range reg.RestoredTables() {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return nil, fmt.Errorf("Error limpiando %s: %w", table, err)
		}
	}

	if _, err := tx.Exec(
		"CREATE TABLE IF NOT EXISTS sync_recent_pulls (" +
			"tabla TEXT NOT NULL, pk TEXT NOT NULL, updated_at TEXT NOT NULL, ts INTEGER NOT NULL, " +
			"PRIMARY KEY (tabla, pk))",
	); err != nil {
		return nil, err
	}
	// Clear only the anti-echo memory (not the data tables): every pull
	// regenerates the device_id, so pulled rows must not be re-sent.
	if _, err := tx.Exec("DELETE FROM sync_recent_pulls"); err != nil {
		return nil, err
	}

	pulled := map[string]int{}
	var omitidas int
	refs := newLocalRefs(tx)
	if filasObj, ok := pullResp["rows"].(map[string]any); ok {
		// Declared order (not the map's): parents must be applied before the
		// rows reconciled by their business code.
		for _, table := range reg.Tables() {
			arr, ok := filasObj[table].([]any)
			if !ok {
				continue
			}
			n := 0
			for _, entry := range arr {
				obj, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				row := Row(obj)
				aplicada, err := insertRestoreRow(reg, tx, table, row, refs)
				if err != nil {
					return nil, err
				}
				if !aplicada {
					omitidas++
					continue
				}
				ua, _ := row["updated_at"].(string)
				if _, err := tx.Exec(
					"INSERT INTO sync_recent_pulls (tabla, pk, updated_at, ts) VALUES (?1, ?2, ?3, ?4) "+
						"ON CONFLICT(tabla, pk) DO UPDATE SET updated_at = excluded.updated_at, ts = excluded.ts",
					table, reg.PKPart(table, row), ua, nowEpoch,
				); err != nil {
					return nil, err
				}
				n++
			}
			pulled[table] = n
		}
	}
	if omitidas > 0 {
		log.Printf("[d1] restore: %d filas se descartaron porque su referencia no corresponde a este dispositivo",
			omitidas)
	}

	var deletedApplied int64
	if deletedArr, ok := pullResp["deleted"].([]any); ok {
		for _, d := range deletedArr {
			del, ok := d.(map[string]any)
			if !ok {
				continue
			}
			tabla, _ := del["tabla"].(string)
			dev, _ := del["device_id"].(string)
			if dev == cfg.DeviceID {
				continue
			}
			if err := deleteByPk(reg, tx, tabla, del["pk"]); err != nil {
				return nil, err
			}
			deletedApplied++
		}
	}

	newSync := NowSQLite()
	if st, ok := pullResp["serverTime"].(string); ok && st != "" {
		newSync = st
	}
	if _, err := tx.Exec(
		"UPDATE d1_config SET last_sync = ?1, last_push_sync = ?2, updated_at = datetime('now') WHERE id = 1",
		newSync, NowSQLite(),
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if emit != nil {
		emit("schooltracker-sync", map[string]any{"source": "restore"})
		emit("data-updated", map[string]any{"source": "restore"})
	}

	return map[string]any{
		"ok":             true,
		"pulled":         pulled,
		"deletedApplied": deletedApplied,
		"lastSync":       newSync,
	}, nil
}

// insertRestoreRow inserts one row of the authoritative restore keeping the
// REMOTE ids.
//
// The local DB was wiped first, so there is nothing to reconcile by name and
// composite-key relationships keep their original references — except the
// parent reference: the FK carried in a row is the id of the device that
// wrote it, so rows with computed columns are re-written to this base's id
// through the business code, and validate-only rows are checked instead.
func insertRestoreRow(reg *Registry, exec execer, table string, row Row, refs *localRefs) (bool, error) {
	spec, ok := reg.Spec(table)
	if !ok {
		return false, fmt.Errorf("unknown syncable table %q in pull", table)
	}
	if spec.Strategy == StrategyValidateOnly {
		matches, err := computedMatches(spec, row, refs)
		if err != nil || !matches {
			return false, err
		}
		return true, insertOrReplace(exec, spec, row)
	}
	if len(spec.Computed) > 0 {
		rewritten, ok, err := rewriteComputed(spec, row, refs)
		if err != nil || !ok {
			return false, err
		}
		return true, insertOrReplace(exec, spec, rewritten)
	}
	return true, insertOrReplace(exec, spec, row)
}

// ReserveIDs reserves id blocks and stores them in `d1_id_blocks`.
func ReserveIDs(conn *sql.DB, client *Client, blocks map[string]int64) (map[string]any, error) {
	cfg, err := ReadConfig(conn)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("D1 no está configurado")
	}

	body := map[string]any{"deviceId": cfg.DeviceID}
	if blocks != nil {
		body["blocks"] = blocks
	} else {
		body["blocks"] = map[string]int64{}
	}
	resp, err := client.Post("/ids", body)
	if err != nil {
		return nil, err
	}

	if b, ok := resp["blocks"].(map[string]any); ok {
		for tabla, rango := range b {
			r, ok := rango.(map[string]any)
			if !ok {
				continue
			}
			start := jsonInt(r["start"])
			end := jsonInt(r["end"])
			if _, err := conn.Exec(
				"INSERT INTO d1_id_blocks (tabla, next_id, end_id, updated_at) "+
					"VALUES (?1, ?2, ?3, datetime('now')) "+
					"ON CONFLICT(tabla) DO UPDATE SET next_id = ?2, end_id = ?3, updated_at = datetime('now')",
				tabla, start, end,
			); err != nil {
				return nil, err
			}
		}
	}
	return resp, nil
}

// jsonInt extracts an integer out of a JSON value.
func jsonInt(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			f, ferr := x.Float64()
			if ferr != nil {
				return 0
			}
			return int64(f)
		}
		return i
	case float64:
		return int64(x)
	case int64:
		return x
	case string:
		i, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

package syncengine_test

// Engine behavior tests: the whole SyncAll/restore cycle against a stub
// worker, exercising the registry-driven strategies (fk rewrite, key upsert,
// validate-only, catalog, tombstones).
//
// These are the ported guards of the "codigo" production bugs: a row whose
// student reference is another device's local id must never be inserted
// orphaned (the purge would tombstone it and kill good rows everywhere), and
// a nota that would land on a different child is dropped.
import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"github.com/Keimer92/kit-d1/go/syncengine"
	"testing"

	"github.com/Keimer92/kit-d1/go/syncengine/example/school"
)

// stubWorker habla el contrato HTTP del worker real (`/push`, `/pull`).
type stubWorker struct {
	srv        *httptest.Server
	mu         sync.Mutex
	pushBodies []map[string]any
	// pullRows / pullDeleted se programan antes de cada sync.
	pullRows    map[string][]any
	pullDeleted []any
}

func newStubWorker(t *testing.T) *stubWorker {
	t.Helper()
	w := &stubWorker{}
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/push":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("stub /push: %v", err)
			}
			w.mu.Lock()
			w.pushBodies = append(w.pushBodies, body)
			w.mu.Unlock()
			writeJSON(t, rw, map[string]any{"ok": true, "applied": map[string]any{}})
		case "/pull":
			rows := w.pullRows
			if rows == nil {
				rows = map[string][]any{}
			}
			deleted := w.pullDeleted
			if deleted == nil {
				deleted = []any{}
			}
			writeJSON(t, rw, map[string]any{
				"ok":         true,
				"rows":       rows,
				"deleted":    deleted,
				"serverTime": "2026-10-30 12:00:00",
			})
		default:
			writeJSON(t, rw, map[string]any{"ok": false, "error": "ruta no prevista: " + r.URL.Path})
		}
	}))
	t.Cleanup(w.srv.Close)
	return w
}
func writeJSON(t *testing.T, rw http.ResponseWriter, v any) {
	t.Helper()
	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(v); err != nil {
		t.Errorf("stub: escribiendo respuesta: %v", err)
	}
}

// nuevaBase abre la base de ejemplo (DDL del manifiesto shipped con el kit)
// y configura el sync hacia el stub.
func nuevaBase(t *testing.T) (*sql.DB, *stubWorker, *syncengine.Registry) {
	t.Helper()
	conn := abrirBase(t)
	for _, q := range []string{
		"INSERT INTO ano_lectivo (id, nombre) VALUES (1, '2026')",
		"INSERT INTO grado (id, nombre) VALUES (1, '1er grado')",
		"INSERT INTO seccion (id, nombre) VALUES (1, 'A')",
		"INSERT INTO colegio (id, nombre) VALUES (1, 'Colegio')",
	} {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	w := newStubWorker(t)
	reg := schoolschema.Must()
	if err := syncengine.WriteConfig(conn, &syncengine.Config{
		WorkerURL: w.srv.URL, SchoolKey: "st_test", DeviceID: "dev-test",
	}); err != nil {
		t.Fatalf("escribiendo d1_config: %v", err)
	}
	return conn, w, reg
}
func sincronizar(t *testing.T, reg *syncengine.Registry, conn *sql.DB, w *stubWorker) *syncengine.Result {
	t.Helper()
	res, err := syncengine.SyncAll(reg, conn, syncengine.NewClient(w.srv.URL, "st_test"),
		syncengine.Options{Source: "test"}, nil)
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	return res
}
func nuevoEstudiante(t *testing.T, conn *sql.DB, id int, codigo string) {
	t.Helper()
	if _, err := conn.Exec(
		"INSERT INTO estudiante (id, nombre, codigo, sexo, ano_lectivo_id, grado_id, seccion_id, colegio_id, updated_at) "+
			"VALUES (?1, ?2, ?3, 'F', 1, 1, 1, 1, datetime('now'))",
		id, "Alumno "+codigo, codigo,
	); err != nil {
		t.Fatalf("insertando el estudiante %s: %v", codigo, err)
	}
}
func nuevaAsistencia(t *testing.T, conn *sql.DB, estudianteID int, fecha, estado string) {
	t.Helper()
	if _, err := conn.Exec(
		"INSERT INTO asistencia (estudiante_id, fecha, estado, updated_at) VALUES (?1, ?2, ?3, datetime('now'))",
		estudianteID, fecha, estado,
	); err != nil {
		t.Fatalf("insertando la asistencia de %d el %s: %v", estudianteID, fecha, err)
	}
}
func asistenciasDe(t *testing.T, q interface {
	Query(query string, args ...any) (*sql.Rows, error)
}, fecha string) map[int]string {
	t.Helper()
	rows, err := q.Query("SELECT estudiante_id, estado FROM asistencia WHERE fecha = ?", fecha)
	if err != nil {
		t.Fatalf("leyendo las asistencias: %v", err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var estado string
		if err := rows.Scan(&id, &estado); err != nil {
			t.Fatalf("escaneando una asistencia: %v", err)
		}
		out[id] = estado
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("leyendo las asistencias: %v", err)
	}
	return out
}
func selectIDs(t *testing.T, conn *sql.DB, query string) []int64 {
	t.Helper()
	rows, err := conn.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("escaneando: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	return out
}
func filaEstudiante(id string, codigo string) map[string]any {
	return map[string]any{
		"id": json.Number(id), "nombre": "Alumna " + codigo, "codigo": codigo,
		"sexo": "F", "ano_lectivo_id": json.Number("1"), "grado_id": json.Number("1"),
		"seccion_id": json.Number("1"), "colegio_id": json.Number("1"), "foto": nil,
		"updated_at": "2026-10-20 08:00:00",
	}
}
func filaAsistencia(id, estudianteID string, codigo, fecha, estado string) map[string]any {
	row := map[string]any{
		"id": json.Number(id), "estudiante_id": json.Number(estudianteID),
		"fecha": fecha, "estado": estado, "observacion": nil,
		"updated_at": "2026-10-21 17:07:00",
	}
	if codigo != "" {
		row["estudiante_codigo"] = codigo
	}
	return row
}
func filaRetirado(estudianteID, codigo, fecha string) map[string]any {
	row := map[string]any{
		"estudiante_id": json.Number(estudianteID),
		"fecha_retiro":  fecha, "motivo": "cambio de colegio",
		"updated_at": "2026-10-05 08:00:00",
	}
	if codigo != "" {
		row["estudiante_codigo"] = codigo
	}
	return row
}
func filaNota(estudianteID, codigo, valor string) map[string]any {
	row := map[string]any{
		"estudiante_id": json.Number(estudianteID),
		"criterio_id":   json.Number("3"), "corte_evaluativo_id": json.Number("1"),
		"asignatura_id": json.Number("2"), "valor": json.Number(valor),
		"updated_at": "2026-10-05 08:00:00",
	}
	if codigo != "" {
		row["estudiante_codigo"] = codigo
	}
	return row
}

// --- asistencia -----------------------------------------------------------
// TestApplyPullRemapaLaAsistenciaAlAlumnoLocal: la fila que llega con el id
// del OTRO dispositivo se guarda apuntando al alumno local de su `codigo`; la
// que apunta a un alumno inexistente se descarta.
func TestApplyPullRemapaLaAsistenciaAlAlumnoLocal(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	w.pullRows = map[string][]any{"asistencia": []any{
		filaAsistencia("77", "47", "0035", "2026-10-21", "P"),
		filaAsistencia("78", "48", "9999", "2026-10-21", "A"),
	}}
	res := sincronizar(t, reg, conn, w)
	if res.Pulled["asistencia"] != 1 {
		t.Fatalf("asistencias aplicadas: %d (esperado 1)", res.Pulled["asistencia"])
	}
	got := asistenciasDe(t, conn, "2026-10-21")
	if len(got) != 1 || got[35] != "P" {
		t.Fatalf("las asistencias quedaron en %v (esperado {35:P})", got)
	}
	var pk string
	if err := conn.QueryRow(
		"SELECT pk FROM sync_recent_pulls WHERE tabla = 'asistencia'",
	).Scan(&pk); err != nil {
		t.Fatalf("leyendo el anti-rebote: %v", err)
	}
	if pk != `"0035"|"2026-10-21"|` {
		t.Fatalf("la clave de anti-rebote es %q, esperaba %q", pk, `"0035"|"2026-10-21"|`)
	}
}

// TestApplyPullRespetaElIdCuandoNoHayCodigo: un worker antiguo sin
// `estudiante_codigo` conserva el comportamiento previo.
func TestApplyPullRespetaElIdCuandoNoHayCodigo(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	w.pullRows = map[string][]any{"asistencia": []any{
		filaAsistencia("77", "35", "", "2026-10-21", "J"),
	}}
	sincronizar(t, reg, conn, w)
	if got := asistenciasDe(t, conn, "2026-10-21"); got[35] != "J" {
		t.Fatalf("la asistencia sin codigo no se aplicó: %v", got)
	}
}

// TestDeleteByPkAsistencia: el tombstone nuevo viaja por `codigo` (string) y
// el de una versión anterior por id local (número); cada uno borra SU fila.
func TestDeleteByPkAsistencia(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 10, "COD-A")
	nuevoEstudiante(t, conn, 11, "COD-B")
	nuevaAsistencia(t, conn, 10, "2026-10-21", "P")
	nuevaAsistencia(t, conn, 11, "2026-10-21", "P")
	w.pullDeleted = []any{
		map[string]any{"tabla": "asistencia", "pk": []any{"COD-A", "2026-10-21"}, "device_id": "dev-otra"},
	}
	sincronizar(t, reg, conn, w)
	got := asistenciasDe(t, conn, "2026-10-21")
	if _, ok := got[10]; ok {
		t.Fatalf("la asistencia de COD-A siguió viva: %v", got)
	}
	if _, ok := got[11]; !ok {
		t.Fatalf("la asistencia de COD-B se borró con el tombstone de otra alumna: %v", got)
	}
	// Tombstone heredado: un número, no un codigo.
	w.pullDeleted = []any{
		map[string]any{"tabla": "asistencia", "pk": []any{json.Number("11"), "2026-10-21"}, "device_id": "dev-otra"},
	}
	sincronizar(t, reg, conn, w)
	if got := asistenciasDe(t, conn, "2026-10-21"); len(got) != 0 {
		t.Fatalf("el tombstone heredado no borró la fila: %v", got)
	}
}

// TestTombstoneDeAsistenciaViajaPorCodigo: borrar una asistencia registra el
// pendiente con el `codigo`; una fila legada (sin codigo) sigue viajando por id.
func TestTombstoneDeAsistenciaViajaPorCodigo(t *testing.T) {
	conn, _, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	nuevaAsistencia(t, conn, 35, "2026-10-21", "P")
	if _, err := conn.Exec(
		"INSERT INTO sync_pending_deletions_asistencia (estudiante_id, fecha) VALUES (47, '2026-10-22')",
	); err != nil {
		t.Fatalf("insertando el pendiente legado: %v", err)
	}
	// Equivalente SQL del borrado por la app: la fila se va y SU pendiente
	// queda con el `codigo` (el helper real vive en la app, no en el kit).
	if _, err := conn.Exec(
		"DELETE FROM asistencia WHERE estudiante_id = 35 AND fecha = '2026-10-21'",
	); err != nil {
		t.Fatalf("borrando la asistencia: %v", err)
	}
	if _, err := conn.Exec(
		"INSERT INTO sync_pending_deletions_asistencia (estudiante_id, fecha, codigo) VALUES (35, '2026-10-21', '0035')",
	); err != nil {
		t.Fatalf("registrando el pendiente por codigo: %v", err)
	}
	dels, err := syncengine.CollectPendingDeletions(reg, conn)
	if err != nil {
		t.Fatalf("CollectPendingDeletions: %v", err)
	}
	porFecha := map[string][]any{}
	for _, d := range dels {
		if d.Tabla == "asistencia" {
			porFecha[toString(d.PK[len(d.PK)-1])] = d.PK
		}
	}
	nuevo := porFecha["2026-10-21"]
	if len(nuevo) != 2 || toString(nuevo[0]) != "0035" {
		t.Fatalf("el tombstone nuevo deberia viajar por codigo, es %v", nuevo)
	}
	legado := porFecha["2026-10-22"]
	if len(legado) != 2 || toString(legado[0]) != "47" {
		t.Fatalf("el tombstone legado perdió su id: %v", legado)
	}
}

// TestPushDeAsistenciaLlevaCodigo: el payload del push trae el `codigo` y la
// clave de anti-rebote coincide entre el push y el pull (corta el bucle).
func TestPushDeAsistenciaLlevaCodigo(t *testing.T) {
	conn, _, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	if _, err := conn.Exec(
		"INSERT INTO asistencia (id, estudiante_id, fecha, estado, updated_at) " +
			"VALUES (77, 35, '2026-10-21', 'P', '2026-10-21 17:07:00')",
	); err != nil {
		t.Fatalf("insertando la asistencia: %v", err)
	}
	data, err := syncengine.CollectLocalDataSince(reg, conn, "1970-01-01 00:00:00", nil)
	if err != nil {
		t.Fatalf("CollectLocalDataSince: %v", err)
	}
	filas := data["asistencia"]
	if len(filas) != 1 {
		t.Fatalf("asistencias en el push: %d (esperado 1)", len(filas))
	}
	push := filas[0]
	if toString(push["estudiante_codigo"]) != "0035" {
		t.Fatalf("el push no trae el codigo del alumno: %v", push)
	}
	if reg.PKPart("asistencia", push) != `"0035"|"2026-10-21"|` {
		t.Fatalf("la PK canónica del push es %q", reg.PKPart("asistencia", push))
	}
	pull := syncengine.Row{
		"id": json.Number("77"), "estudiante_id": json.Number("47"),
		"estudiante_codigo": "0035", "fecha": "2026-10-21", "estado": "P",
		"updated_at": "2026-10-21 17:07:00",
	}
	if reg.RowRecentKey("asistencia", pull) != reg.RowRecentKey("asistencia", push) {
		t.Fatal("el pull y el push calculan claves de anti-rebote distintas: la fila se re-enviaria para siempre")
	}
}

// TestInsertRestoreRowRemapaLaAsistencia: el restore autoritativo también
// reconcilia la FK por `codigo` y no filtra `estudiante_codigo` al SQL.
func TestInsertRestoreRowRemapaLaAsistencia(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	nuevaAsistencia(t, conn, 35, "2026-01-01", "P") // se va en el wipe del restore
	// El restore borra TODAS las tablas sincronizables: la alumna vuelve desde
	// la nube con su id canónico (47) y la asistencia debe recaer en ese id,
	// no en el 35 local que ya no existe.
	w.pullRows = map[string][]any{
		"estudiante": []any{filaEstudiante("47", "0035")},
		"asistencia": []any{
			filaAsistencia("77", "47", "0035", "2026-10-21", "P"),
			filaAsistencia("78", "48", "9999", "2026-10-22", "P"),
		},
	}
	if _, err := syncengine.RestoreFromCloud(reg, conn, syncengine.NewClient(w.srv.URL, "st_test"), nil); err != nil {
		t.Fatalf("RestoreFromCloud: %v", err)
	}
	if got := asistenciasDe(t, conn, "2026-10-21"); got[47] != "P" {
		t.Fatalf("la asistencia no quedó en el alumno restaurado: %v", got)
	}
	if got := asistenciasDe(t, conn, "2026-10-22"); len(got) != 0 {
		t.Fatalf("se insertó una asistencia huérfana en el restore: %v", got)
	}
	// El restore conserva el id remoto.
	ids := selectIDs(t, conn, "SELECT id FROM asistencia")
	if len(ids) != 1 || ids[0] != 77 {
		t.Fatalf("el restore no conservó el id remoto: %v", ids)
	}
}

// TestPurgaDeHuerfanosNoGeneraTombstone: la asistencia con el id de otro
// dispositivo se limpia LOCAL, sin propagar el borrado.
func TestPurgaDeHuerfanosNoGeneraTombstone(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	nuevaAsistencia(t, conn, 35, "2026-10-21", "P")
	if _, err := conn.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("desactivando las FK: %v", err)
	}
	nuevaAsistencia(t, conn, 47, "2026-10-21", "P")
	sincronizar(t, reg, conn, w)
	if got := asistenciasDe(t, conn, "2026-10-21"); len(got) != 1 || got[35] != "P" {
		t.Fatalf("la purga se llevó una asistencia válida (o dejó la huérfana): %v", got)
	}
	dels, err := syncengine.CollectPendingDeletions(reg, conn)
	if err != nil {
		t.Fatalf("CollectPendingDeletions: %v", err)
	}
	for _, d := range dels {
		if d.Tabla == "asistencia" {
			t.Fatalf("la purga de huérfanos volvió a tombstonear una asistencia: %v", d.PK)
		}
	}
}

// --- estudiante_retirado --------------------------------------------------
// TestApplyPullRemapaElRetiroPorCodigo: el retiro con el id del teléfono se
// guarda apuntando al alumno local del `codigo`, y repetir el pull no lo
// duplica.
func TestApplyPullRemapaElRetiroPorCodigo(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	nuevoEstudiante(t, conn, 47, "0047") // existe, pero NO es la alumna del payload
	w.pullRows = map[string][]any{"estudiante_retirado": []any{
		filaRetirado("47", "0035", "2026-10-05"),
		filaRetirado("99", "9999", "2026-10-05"),
	}}
	res := sincronizar(t, reg, conn, w)
	if res.Pulled["estudiante_retirado"] != 1 {
		t.Fatalf("retiros aplicados: %d (esperado 1)", res.Pulled["estudiante_retirado"])
	}
	ids := selectIDs(t, conn, "SELECT estudiante_id FROM estudiante_retirado")
	if len(ids) != 1 || ids[0] != 35 {
		t.Fatalf("el retiro quedó en %v (esperado [35])", ids)
	}
	sincronizar(t, reg, conn, w)
	if ids = selectIDs(t, conn, "SELECT estudiante_id FROM estudiante_retirado"); len(ids) != 1 {
		t.Fatalf("el retiro se duplicó: %v", ids)
	}
}

// TestDeleteByPkRetiro: el tombstone nuevo viaja por `codigo` y uno heredado
// por el id local; cada uno borra SU retiro.
func TestDeleteByPkRetiro(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 10, "COD-A")
	nuevoEstudiante(t, conn, 11, "COD-B")
	for _, id := range []int64{10, 11} {
		if _, err := conn.Exec(
			"INSERT INTO estudiante_retirado (estudiante_id, fecha_retiro, motivo, updated_at) "+
				"VALUES (?1, '2026-10-05', '', datetime('now'))", id,
		); err != nil {
			t.Fatalf("retirando a %d: %v", id, err)
		}
	}
	w.pullDeleted = []any{
		map[string]any{"tabla": "estudiante_retirado", "pk": []any{"COD-A"}, "device_id": "dev-otra"},
	}
	sincronizar(t, reg, conn, w)
	ids := selectIDs(t, conn, "SELECT estudiante_id FROM estudiante_retirado")
	if len(ids) != 1 || ids[0] != 11 {
		t.Fatalf("el tombstone por codigo borró al alumno equivocado: %v", ids)
	}
	w.pullDeleted = []any{
		map[string]any{"tabla": "estudiante_retirado", "pk": []any{json.Number("11")}, "device_id": "dev-otra"},
	}
	sincronizar(t, reg, conn, w)
	if ids = selectIDs(t, conn, "SELECT estudiante_id FROM estudiante_retirado"); len(ids) != 0 {
		t.Fatalf("el tombstone heredado no borró el retiro: %v", ids)
	}
}

// TestPushRetiroLlevaCodigo: el payload del retiro trae el `codigo` y su clave
// de anti-rebote coincide entre push y pull.
func TestPushRetiroLlevaCodigo(t *testing.T) {
	conn, _, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	if _, err := conn.Exec(
		"INSERT INTO estudiante_retirado (estudiante_id, fecha_retiro, motivo, updated_at) " +
			"VALUES (35, '2026-10-05', '', '2026-10-05 08:00:00')",
	); err != nil {
		t.Fatalf("retirando al alumno: %v", err)
	}
	data, err := syncengine.CollectLocalDataSince(reg, conn, "1970-01-01 00:00:00", nil)
	if err != nil {
		t.Fatalf("CollectLocalDataSince: %v", err)
	}
	filas := data["estudiante_retirado"]
	if len(filas) != 1 {
		t.Fatalf("retiros en el push: %d (esperado 1)", len(filas))
	}
	push := filas[0]
	if toString(push["estudiante_codigo"]) != "0035" {
		t.Fatalf("el push del retiro no trae el codigo: %v", push)
	}
	if reg.PKPart("estudiante_retirado", push) != `"0035"|` {
		t.Fatalf("la PK canónica del retiro es %q", reg.PKPart("estudiante_retirado", push))
	}
	pull := syncengine.Row{
		"estudiante_id": json.Number("47"), "estudiante_codigo": "0035",
		"fecha_retiro": "2026-10-05", "motivo": "", "updated_at": "2026-10-05 08:00:00",
	}
	if reg.RowRecentKey("estudiante_retirado", pull) != reg.RowRecentKey("estudiante_retirado", push) {
		t.Fatal("el push y el pull del retiro calculan claves de anti-rebote distintas")
	}
}

// --- nota (validate-only) ---------------------------------------------------
// TestApplyPullDescartaNotaDeOtraAlumna: la nota cuyo `estudiante_id` no es el
// id local del alumno con ese `codigo` se descarta; la que coincide se aplica.
func TestApplyPullDescartaNotaDeOtraAlumna(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	nuevoEstudiante(t, conn, 47, "0047")
	w.pullRows = map[string][]any{"nota": []any{
		filaNota("47", "0035", "18"), // id de otra alumna: se descarta
		filaNota("35", "0035", "15"), // coincide: se aplica
		filaNota("99", "9999", "20"), // alumna inexistente: se descarta
	}}
	res := sincronizar(t, reg, conn, w)
	if res.Pulled["nota"] != 1 {
		t.Fatalf("notas aplicadas: %d (esperado 1)", res.Pulled["nota"])
	}
	ids := selectIDs(t, conn, "SELECT estudiante_id FROM nota")
	if len(ids) != 1 || ids[0] != 35 {
		t.Fatalf("las notas quedaron en %v (esperado [35])", ids)
	}
}

// TestNotaSinCodigoSeAplicaComoAntes: sin `estudiante_codigo` (worker viejo)
// la nota se aplica con el id que llega si el alumno existe acá.
func TestNotaSinCodigoSeAplicaComoAntes(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	w.pullRows = map[string][]any{"nota": []any{filaNota("35", "", "14")}}
	sincronizar(t, reg, conn, w)
	ids := selectIDs(t, conn, "SELECT estudiante_id FROM nota")
	if len(ids) != 1 || ids[0] != 35 {
		t.Fatalf("la nota sin codigo no se aplicó: %v", ids)
	}
}

// TestNotaSinCodigoYSinAlumnoSeDescarta: si el id tampoco existe acá, la nota
// se descarta: sería una nota huérfana invisible para la app.
func TestNotaSinCodigoYSinAlumnoSeDescarta(t *testing.T) {
	conn, w, reg := nuevaBase(t)
	nuevoEstudiante(t, conn, 35, "0035")
	w.pullRows = map[string][]any{"nota": []any{filaNota("999", "", "10")}}
	res := sincronizar(t, reg, conn, w)
	if res.Pulled["nota"] != 0 {
		t.Fatalf("notas aplicadas: %d (esperado 0)", res.Pulled["nota"])
	}
	if ids := selectIDs(t, conn, "SELECT estudiante_id FROM nota"); len(ids) != 0 {
		t.Fatalf("se importó una nota huérfana: %v", ids)
	}
}

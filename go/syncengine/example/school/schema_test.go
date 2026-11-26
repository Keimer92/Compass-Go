package schoolschema_test

// Contract tests for the project's sync schema.
//
// The table list and the canonical PKs are the contract with the worker: if
// they diverge, the push sends rows the worker ignores and the pull brings
// nothing back. These assertions used to live in internal/d1 (hard-coded);
// now they guard the Registry built from this project's specs.

import (
	"testing"

	"github.com/Keimer92/kit-d1/go/syncengine"
	"github.com/Keimer92/kit-d1/go/syncengine/example/school"
)

func TestListaDeTablas(t *testing.T) {
	reg := schoolschema.Must()
	tablas := reg.Tables()
	if len(tablas) != 22 {
		t.Fatalf("hay %d tablas sincronizables (esperadas 22)", len(tablas))
	}
	esperadas := []string{
		"ano_lectivo", "corte_evaluativo", "colegio", "asignatura", "grado", "seccion",
		"grado_asignatura", "colegio_asignatura", "colegio_grado", "indicador", "criterio",
		"indicador_colegio_seccion_activo", "estudiante", "configuracion_calificacion",
		"multiplicador_calificacion", "colegio_dias_asistencia", "asistencia", "nota",
		"dia_sin_clases", "instrumento_plantilla", "estudiante_retirado", "app_settings",
	}
	// El ORDEN importa: es el del payload del push y el de aplicacion del pull
	// (`estudiante` debe aplicarse antes que `asistencia`, que se reconcilia
	// por su codigo).
	for i, tabla := range esperadas {
		if tablas[i] != tabla {
			t.Fatalf("tabla %d = %q (esperada %q)", i, tablas[i], tabla)
		}
	}
}

func TestPKCanonicas(t *testing.T) {
	reg := schoolschema.Must()
	esperadas := map[string][]string{
		"grado_asignatura":                 {"grado_id", "asignatura_id"},
		"colegio_asignatura":               {"colegio_id", "asignatura_id"},
		"colegio_grado":                    {"colegio_id", "grado_id"},
		"colegio_dias_asistencia":          {"colegio_id"},
		"dia_sin_clases":                   {"id"},
		"instrumento_plantilla":            {"propietario", "nombre"},
		"nota":                             {"estudiante_id", "criterio_id", "corte_evaluativo_id", "asignatura_id"},
		"estudiante_retirado":              {"estudiante_codigo"},
		"indicador_colegio_seccion_activo": {"colegio_id", "seccion_id", "indicador_id"},
		"asistencia":                       {"estudiante_codigo", "fecha"},
		"app_settings":                     {"key"},
		"estudiante":                       {"codigo"},
		"colegio":                          {"id"},
	}
	for tabla, cols := range esperadas {
		got := reg.PKCols(tabla)
		if len(got) != len(cols) {
			t.Fatalf("%s: PK %v (esperada %v)", tabla, got, cols)
		}
		for i := range cols {
			if got[i] != cols[i] {
				t.Fatalf("%s: PK %v (esperada %v)", tabla, got, cols)
			}
		}
	}
	if pk := reg.PKCols("tabla_inexistente"); pk != nil {
		t.Fatalf("una tabla desconocida no tiene PK canonica, devolvio %v", pk)
	}
}

// TestCloudPK: la identidad que guarda la nube: las columnas calculadas
// (`estudiante_codigo`) se sustituyen por el FK almacenado (`estudiante_id`).
func TestCloudPK(t *testing.T) {
	reg := schoolschema.Must()
	casos := map[string][]string{
		"asistencia":          {"estudiante_id", "fecha"},
		"estudiante_retirado": {"estudiante_id"},
		"estudiante":          {"codigo"},
	}
	for tabla, esperado := range casos {
		spec, ok := reg.Spec(tabla)
		if !ok {
			t.Fatalf("falta la tabla %q", tabla)
		}
		got := reg.CloudPK(tabla)
		if len(got) != len(esperado) {
			t.Fatalf("%s: CloudPK %v (esperada %v)", tabla, got, esperado)
		}
		for i := range esperado {
			if got[i] != esperado[i] {
				t.Fatalf("%s: CloudPK %v (esperada %v)", tabla, got, esperado)
			}
		}
		if inList(spec.WireCols, "") || len(spec.WireCols) == 0 {
			t.Fatalf("%s: sin columnas del worker", tabla)
		}
	}
}

func TestEstrategiasPorTabla(t *testing.T) {
	reg := schoolschema.Must()
	esperadas := map[string]syncengine.Strategy{
		"ano_lectivo":           syncengine.StrategyCatalog,
		"corte_evaluativo":      syncengine.StrategyCatalog,
		"colegio":               syncengine.StrategyCatalog,
		"asignatura":            syncengine.StrategyCatalog,
		"grado":                 syncengine.StrategyCatalog,
		"seccion":               syncengine.StrategyCatalog,
		"estudiante":            syncengine.StrategyCatalog,
		"instrumento_plantilla": syncengine.StrategyCatalog,
		"criterio":              syncengine.StrategyKeyUpsert,
		"asistencia":            syncengine.StrategyKeyUpsert,
		"estudiante_retirado":   syncengine.StrategyFKRewrite,
		"nota":                  syncengine.StrategyValidateOnly,
	}
	for tabla, estrategia := range esperadas {
		spec, ok := reg.Spec(tabla)
		if !ok {
			t.Fatalf("falta la tabla %q", tabla)
		}
		if spec.Strategy != estrategia {
			t.Fatalf("%s: estrategia %q (esperada %q)", tabla, spec.Strategy, estrategia)
		}
	}

	// Asistencia: conflicto local (id, fecha), solo se actualiza el estado.
	as, _ := reg.Spec("asistencia")
	if !equalCols(as.Upsert.Conflict, []string{"estudiante_id", "fecha"}) {
		t.Fatalf("asistencia: conflicto %v", as.Upsert.Conflict)
	}
	if !equalCols(as.Upsert.Set, []string{"estado", "observacion", "updated_at"}) {
		t.Fatalf("asistencia: set %v", as.Upsert.Set)
	}
	if as.Upsert.IncludeID {
		t.Fatal("asistencia: el id local lo asigna esta base, no la nube")
	}

	// Criterio: igual que el worker, conserva el id de la nube (dos equipos no
	// pueden crear "mismo orden, dos ids").
	cr, _ := reg.Spec("criterio")
	if !equalCols(cr.Upsert.Conflict, []string{"indicador_id", "orden"}) {
		t.Fatalf("criterio: conflicto %v", cr.Upsert.Conflict)
	}
	if !cr.Upsert.IncludeID {
		t.Fatal("criterio: debe insertar con el id de la nube")
	}

	// Las tablas de catalogo se reconcilian por clave de negocio.
	claves := map[string][]string{}
	for _, nombre := range reg.Tables() {
		spec, _ := reg.Spec(nombre)
		if spec.Strategy == syncengine.StrategyCatalog {
			claves[nombre] = spec.BusinessKey
		}
	}
	esperadasClaves := map[string][]string{
		"ano_lectivo":           {"nombre"},
		"corte_evaluativo":      {"nombre"},
		"colegio":               {"nombre"},
		"asignatura":            {"nombre"},
		"grado":                 {"nombre"},
		"seccion":               {"nombre"},
		"estudiante":            {"codigo"},
		"instrumento_plantilla": {"propietario", "nombre"},
	}
	if len(claves) != len(esperadasClaves) {
		t.Fatalf("hay %d tablas con clave de negocio (esperadas %d)", len(claves), len(esperadasClaves))
	}
	for tabla, clave := range esperadasClaves {
		if !equalCols(claves[tabla], clave) {
			t.Fatalf("%s: clave de negocio %v (esperada %v)", tabla, claves[tabla], clave)
		}
	}
}

func TestTablasDeTombstones(t *testing.T) {
	reg := schoolschema.Must()
	esperadas := []string{
		"sync_pending_deletions",
		"sync_pending_deletions_grado_asignatura",
		"sync_pending_deletions_colegio_asignatura",
		"sync_pending_deletions_colegio_grado",
		"sync_pending_deletions_estudiante",
		"sync_pending_deletions_asistencia",
		"sync_pending_deletions_nota",
		"sync_pending_deletions_instrumento_plantilla",
	}
	got := reg.TombstoneTables()
	if len(got) != len(esperadas) {
		t.Fatalf("hay %d tablas de tombstones: %v", len(got), got)
	}
	visibles := map[string]bool{}
	for _, nom := range got {
		visibles[nom] = true
	}
	for _, nom := range esperadas {
		if !visibles[nom] {
			t.Fatalf("falta la tabla de tombstones %q (estan %v)", nom, got)
		}
	}
}

func TestTablasRestaurables(t *testing.T) {
	reg := schoolschema.Must()
	restauradas := reg.RestoredTables()
	// `estudiante_retirado` NO se restaura: reactivar a una alumna borra su
	// retiro local sin tombstone (limitacion conocida, compartida con Rust).
	if len(restauradas) != 21 {
		t.Fatalf("se restauran %d tablas (esperadas 21): %v", len(restauradas), restauradas)
	}
	for _, nombre := range restauradas {
		if nombre == "estudiante_retirado" {
			t.Fatal("estudiante_retirado no debe borrarse en un restore autoritativo")
		}
	}
}

func TestColumnasDelWorkerCubrenLasPK(t *testing.T) {
	reg := schoolschema.Must()
	for _, nombre := range reg.Tables() {
		spec, _ := reg.Spec(nombre)
		if len(spec.WireCols) == 0 {
			t.Errorf("%s: sin columnas del worker", nombre)
		}
		computadas := map[string]bool{}
		for _, c := range spec.Computed {
			computadas[c.Column] = true
		}
		for _, pk := range spec.PK {
			if !computadas[pk] && !inList(spec.WireCols, pk) {
				t.Errorf("%s: la PK canonica %q no es columna del worker ni calculada", nombre, pk)
			}
		}
	}
}

func TestIndicadorSyncIncludesCurriculumUnit(t *testing.T) {
	reg := schoolschema.Must()
	spec, ok := reg.Spec("indicador")
	if !ok {
		t.Fatal("falta la tabla indicador")
	}
	if !inList(spec.WireCols, "unidad_numero") {
		t.Fatal("indicador debe sincronizar unidad_numero (conserva la asignacion curricular)")
	}
}

func TestPurgaDeHuerfanosDeclarada(t *testing.T) {
	reg := schoolschema.Must()
	spec, ok := reg.Spec("asistencia")
	if !ok {
		t.Fatal("falta la tabla asistencia")
	}
	if spec.Orphan == nil {
		t.Fatal("asistencia debe declarar la purga local de huerfanos")
	}
	if spec.Orphan.FK != "estudiante_id" || spec.Orphan.Parent != "estudiante" {
		t.Fatalf("purga de huerfanos incorrecta: %+v", *spec.Orphan)
	}
}

func TestRegistryRechazaNombresInvalidos(t *testing.T) {
	// El motor interpola estos nombres en el SQL: uno hostil debe fallar aqui,
	// no convertirse en inyeccion.
	casos := []syncengine.TableSpec{
		{Name: "pestaña; DROP TABLE x", PK: []string{"id"}},
		{Name: "ok", PK: []string{"id\" , (SELECT 1)"}},
		{Name: "ok2", PK: []string{"id"}, WireCols: []string{"id"},
			TombstoneTable: "t--"},
		{Name: "ok3", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog},
		{Name: "ok4", PK: []string{"id"}, Strategy: syncengine.Strategy("otra")},
	}
	for _, spec := range casos {
		if _, err := syncengine.NewRegistry([]syncengine.TableSpec{spec}); err == nil {
			t.Fatalf("el registry acepto %q (PK %v, estrategia %q)", spec.Name, spec.PK, spec.Strategy)
		}
	}
}

func TestClavesDeAntiRebote(t *testing.T) {
	reg := schoolschema.Must()
	fila := syncengine.Row{
		"estudiante_codigo": "0035", "fecha": "2026-10-21",
		"updated_at": "2026-10-21 08:00:00",
	}
	if pk := reg.PKPart("asistencia", fila); pk != `"0035"|"2026-10-21"|` {
		t.Fatalf("PKPart de asistencia = %q", pk)
	}
	clave := reg.RowRecentKey("asistencia", fila)
	// La PK part termina en "|", asi que la clave lleva "||" antes del
	// updated_at (espejo exacto del formato del worker/Rust).
	if clave != `asistencia|"0035"|"2026-10-21"||2026-10-21 08:00:00` {
		t.Fatalf("RowRecentKey de asistencia = %q", clave)
	}
}

func TestBackoffDeReintentos(t *testing.T) {
	esperado := []int64{5, 15, 30, 60, 120, 240}
	if len(syncengine.PushBackoffMin) != len(esperado) {
		t.Fatalf("el backoff tiene %d pasos", len(syncengine.PushBackoffMin))
	}
	for i, v := range esperado {
		if syncengine.PushBackoffMin[i] != v {
			t.Fatalf("el backoff[%d] = %d (esperado %d)", i, syncengine.PushBackoffMin[i], v)
		}
	}
}

func equalCols(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Package schoolschema is the example manifest shipped with the kit: the syncable schema of School Tracker.
//
// It is the project's half of the sync contract: the engine
// is generic and receives this Registry, so it never knows table names.
// These specs mirror `TABLES` in the Cloudflare Worker
// (`cloudflare-sync/src/index.ts`); when they diverge, the push sends rows the
// worker ignores and the pull brings nothing back.
package schoolschema

import (
	"fmt"

	"github.com/Keimer92/Compass-Go/Kit-D1/go/syncengine"
)

// studentCode is the computed column shared by the three tables that
// reference a student: the worker resolves `codigo` on pull and rewrites the
// local `estudiante_id` with the cloud's canonical id. Local ids diverge
// between devices, so an id captured offline on the phone was another child
// (or none) on the PC.
func studentCode() syncengine.ComputedFK {
	return syncengine.ComputedFK{
		Column:    "estudiante_codigo",
		Parent:    "estudiante",
		ParentKey: "id",
		FK:        "estudiante_id",
		Target:    "codigo",
	}
}

// specs is the syncable table list, in payload/apply order.
//
// Order matters: `estudiante` must be applied before `asistencia`, whose rows
// reconcile by the student's `codigo`.
var specs = []syncengine.TableSpec{
	{Name: "ano_lectivo", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, Restored: true,
		WireCols: []string{"id", "nombre", "activo", "cualitativo", "updated_at"}},
	{Name: "corte_evaluativo", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, NamespaceScopedConflict: true, Restored: true,
		WireCols: []string{"id", "nombre", "activo", "cualitativo", "updated_at"}},
	{Name: "colegio", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, Restored: true,
		WireCols: []string{"id", "nombre", "activo", "cualitativo", "orden", "tipo", "updated_at"}},
	{Name: "asignatura", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, Restored: true,
		WireCols: []string{"id", "nombre", "cualitativo", "updated_at"}},
	{Name: "grado", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, Restored: true,
		WireCols: []string{"id", "nombre", "cualitativo", "updated_at"}},
	{Name: "seccion", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"nombre"}, Restored: true,
		WireCols: []string{"id", "nombre", "cualitativo", "updated_at"}},

	{Name: "grado_asignatura", PK: []string{"grado_id", "asignatura_id"}, Restored: true,
		WireCols:       []string{"grado_id", "asignatura_id", "updated_at"},
		TombstoneTable: "sync_pending_deletions_grado_asignatura"},
	{Name: "colegio_asignatura", PK: []string{"colegio_id", "asignatura_id"}, Restored: true,
		WireCols:       []string{"colegio_id", "asignatura_id", "updated_at"},
		TombstoneTable: "sync_pending_deletions_colegio_asignatura"},
	{Name: "colegio_grado", PK: []string{"colegio_id", "grado_id"}, Restored: true,
		WireCols:       []string{"colegio_id", "grado_id", "activo", "cualitativo", "updated_at"},
		TombstoneTable: "sync_pending_deletions_colegio_grado"},

	{Name: "indicador", PK: []string{"id"}, Restored: true,
		WireCols: []string{"id", "grado_id", "asignatura_id", "corte_evaluativo_id",
			"ano_lectivo_id", "orden", "descripcion", "activo", "instrumento",
			"unidad_numero", "updated_at"}},
	// Criterio is reconciled by (indicador_id, orden), keeping the local id:
	// notas reference criterio_id. It carries the cloud id on insert so two
	// devices cannot create "same orden, two ids" duplicates on pull.
	{Name: "criterio", PK: []string{"id"}, Strategy: syncengine.StrategyKeyUpsert, Restored: true,
		Upsert: syncengine.Upsert{
			Conflict:  []string{"indicador_id", "orden"},
			Set:       []string{"descripcion", "puntaje_maximo", "updated_at"},
			IncludeID: true,
		},
		WireCols: []string{"id", "indicador_id", "orden", "descripcion", "puntaje_maximo", "updated_at"}},
	{Name: "indicador_colegio_seccion_activo",
		PK: []string{"colegio_id", "seccion_id", "indicador_id"}, Restored: true,
		WireCols: []string{"colegio_id", "seccion_id", "indicador_id", "activo", "updated_at"}},

	// The student's cloud identity is its `codigo`, not the local id: a
	// tombstone by id never deleted the remote row (it held the other device's
	// id) and the child reappeared on the other device.
	//
	// One row per ENROLLMENT (student + school year): promoting keeps the
	// origin row with its grades, so the new row needs its own `codigo`
	// (`<codigo_base>@<year>`) while `codigo_base` is the code the user sees
	// and which repeats across years.
	{Name: "estudiante", PK: []string{"codigo"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey: []string{"codigo"}, Restored: true,
		TombstoneTable: "sync_pending_deletions_estudiante",
		WireCols: []string{"id", "nombre", "codigo", "codigo_base", "sexo", "ano_lectivo_id",
			"grado_id", "seccion_id", "colegio_id", "foto", "updated_at"}},

	{Name: "configuracion_calificacion", PK: []string{"id"}, Restored: true,
		WireCols: []string{"id", "nombre", "color", "minimo", "updated_at"}},
	{Name: "multiplicador_calificacion", PK: []string{"id"}, Restored: true,
		WireCols: []string{"id", "multiplicador", "updated_at"}},
	{Name: "colegio_dias_asistencia", PK: []string{"colegio_id"}, Restored: true,
		WireCols: []string{"colegio_id", "lunes", "martes", "miercoles", "jueves",
			"viernes", "sabado", "domingo", "updated_at"}},

	// Attendance identity is (student code, date). The FK is re-written to
	// this device's local id before the upsert, and rows whose student is not
	// on this device are dropped: inserting them orphaned the row, and the
	// purge then tombstoned a deletion that killed good rows on every device.
	{Name: "asistencia", PK: []string{"estudiante_codigo", "fecha"},
		Strategy: syncengine.StrategyKeyUpsert, Restored: true,
		Computed: []syncengine.ComputedFK{studentCode()},
		Upsert: syncengine.Upsert{
			Conflict: []string{"estudiante_id", "fecha"},
			Set:      []string{"estado", "observacion", "updated_at"},
		},
		TombstoneTable: "sync_pending_deletions_asistencia",
		Orphan:         &syncengine.OrphanPurge{FK: "estudiante_id", Parent: "estudiante", ParentKey: "id"},
		WireCols:       []string{"id", "estudiante_id", "fecha", "estado", "observacion", "updated_at"}},

	// Notas cannot be canonicalized: their cloud PK is four ids and only the
	// student has a business key, so rewriting one column could create a
	// duplicate row. They travel validate-only.
	{Name: "nota", PK: []string{"estudiante_id", "criterio_id", "corte_evaluativo_id", "asignatura_id"},
		Strategy: syncengine.StrategyValidateOnly, Restored: true,
		Computed:       []syncengine.ComputedFK{studentCode()},
		TombstoneTable: "sync_pending_deletions_nota",
		WireCols: []string{"estudiante_id", "criterio_id", "valor", "corte_evaluativo_id",
			"asignatura_id", "updated_at"}},

	{Name: "dia_sin_clases", PK: []string{"id"}, Restored: true,
		WireCols: []string{"id", "colegio_id", "grado_id", "seccion_id", "fecha",
			"descripcion", "updated_at"}},
	// A template is a USER catalog: local ids diverge, so its identity (for
	// the pull, the anti-echo set and the deletion) is (propietario, nombre).
	// The owner is the account email, so two teachers can have a template with
	// the same name; `''` means a shared (pre-ownership) template.
	{Name: "instrumento_plantilla", PK: []string{"propietario", "nombre"}, Strategy: syncengine.StrategyCatalog,
		BusinessKey:    []string{"propietario", "nombre"},
		TombstoneTable: "sync_pending_deletions_instrumento_plantilla",
		Restored:       true, ServerAssignsID: true,
		WireCols: []string{"id", "propietario", "nombre", "tipo", "definicion", "updated_at"}},
	// A withdrawal's identity is the STUDENT: new tombstones travel by
	// `codigo` (like asistencia), not by the id the withdrawing device chose.
	{Name: "estudiante_retirado", PK: []string{"estudiante_codigo"},
		Strategy: syncengine.StrategyFKRewrite, Restored: false,
		Computed: []syncengine.ComputedFK{studentCode()},
		WireCols: []string{"estudiante_id", "fecha_retiro", "motivo", "updated_at"}},
	{Name: "app_settings", PK: []string{"key"}, Restored: true,
		WireCols: []string{"key", "value", "updated_at"}},
}

// New builds the project's registry, validating the contract.
func New() (*syncengine.Registry, error) {
	r, err := syncengine.NewRegistry(specs)
	if err != nil {
		return nil, fmt.Errorf("sync schema: %w", err)
	}
	return r, nil
}

// Must builds the registry or panics: a spec error is a programming bug, not
// a runtime condition.
func Must() *syncengine.Registry {
	r, err := New()
	if err != nil {
		panic(err)
	}
	return r
}

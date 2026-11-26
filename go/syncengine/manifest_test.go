package syncengine_test

import (
	"encoding/json"
	"testing"

	"github.com/Keimer92/kit-d1/go/syncengine"
)

func minimalSpecs() []syncengine.TableSpec {
	return []syncengine.TableSpec{
		{Name: "parent", PK: []string{"id"}, Strategy: syncengine.StrategyCatalog,
			BusinessKey: []string{"nombre"}, NamespaceScopedConflict: true,
			WireCols: []string{"id", "nombre", "updated_at"}},
		{Name: "child", PK: []string{"parent_codigo", "fecha"},
			Strategy: syncengine.StrategyKeyUpsert,
			Computed: []syncengine.ComputedFK{{
				Column: "parent_codigo", Parent: "parent", ParentKey: "id",
				FK: "parent_id", Target: "codigo",
			}},
			Upsert:   syncengine.Upsert{Conflict: []string{"parent_id", "fecha"}},
			WireCols: []string{"id", "parent_id", "fecha", "updated_at"}},
		{Name: "tpl", PK: []string{"nombre"}, Strategy: syncengine.StrategyCatalog,
			BusinessKey: []string{"nombre"}, ServerAssignsID: true,
			WireCols: []string{"id", "nombre", "updated_at"}},
		{Name: "plain", PK: []string{"id"},
			WireCols: []string{"id", "valor", "updated_at"}},
	}
}

func TestManifestProjection(t *testing.T) {
	reg, err := syncengine.NewRegistry(minimalSpecs())
	if err != nil {
		t.Fatal(err)
	}
	m := reg.Manifest("test")
	if m.Version != syncengine.ManifestVersion || m.Namespace != "test" {
		t.Fatalf("header: %+v", m)
	}
	if len(m.Tables) != 4 {
		t.Fatalf("want 4 tables, got %d", len(m.Tables))
	}

	byName := map[string]syncengine.ManifestTable{}
	for _, tbl := range m.Tables {
		byName[tbl.Name] = tbl
	}

	// Catalog: conflict is the business key; SET keeps no id; no computed.
	p := byName["parent"]
	if len(p.Conflict) != 2 || p.Conflict[0] != "nombre" || p.Conflict[1] != "sk" || !p.SetExcludeID || p.ServerAssignsID {
		t.Errorf("parent: %+v", p)
	}

	// Key-upsert with computed: upsert_pk is the stored identity, conflict
	// comes from the declared upsert target, the FK canonizes on push.
	c := byName["child"]
	if c.UpsertPK[0] != "parent_id" || c.UpsertPK[1] != "fecha" {
		t.Errorf("child upsert_pk = %v (want parent_id,fecha)", c.UpsertPK)
	}
	if c.Conflict[0] != "parent_id" || c.SetExcludeID {
		t.Errorf("child conflict = %v, set_exclude_id = %v (must keep the id: conflict == stored pk)", c.Conflict, c.SetExcludeID)
	}
	if len(c.Computed) != 1 || !c.Computed[0].Canonize || c.Computed[0].Target != "codigo" {
		t.Errorf("child computed = %+v", c.Computed)
	}

	// Server-assigned id flows through — and SET must exclude the id even
	// though conflict == PK here: the INSERT omits it, so `excluded.id` is
	// NULL and keeping it in SET would re-assign the rowid on every push.
	if !byName["tpl"].ServerAssignsID {
		t.Error("tpl must keep server_assigns_id")
	}
	if !byName["tpl"].SetExcludeID {
		t.Error("tpl: server-assigned id requires set_exclude_id")
	}

	// Plain replace: conflict == upsert_pk == stored id, id travels.
	pl := byName["plain"]
	if pl.Conflict[0] != "id" || pl.SetExcludeID || len(pl.Computed) != 0 {
		t.Errorf("plain: %+v", pl)
	}

	// JSON round-trip (the worker consumes this exact shape).
	raw, err := reg.ManifestJSON("test")
	if err != nil {
		t.Fatal(err)
	}
	var back syncengine.Manifest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if len(back.Tables) != 4 || back.Tables[1].Name != "child" {
		t.Errorf("round-trip lost order/content: %+v", back.Tables)
	}
}

func TestRegistryRechazaServerAssignsIDSinBusinessKey(t *testing.T) {
	specs := minimalSpecs()
	specs[3].ServerAssignsID = true // "plain" has no business key
	if _, err := syncengine.NewRegistry(specs); err == nil {
		t.Fatal("expected rejection of server_assigns_id without a business key")
	}
}

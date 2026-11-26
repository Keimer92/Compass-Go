package syncengine

// Manifest export: the worker-facing projection of the Registry.
//
// The Go registry is the single source of truth for the sync contract; this
// file serializes it into the JSON the worker template consumes
// (`sync.schema.json`). Everything here is DERIVED — the worker never needs
// a hand-written table list. Field semantics mirror `buildUpsert`/
// `pullSelect` in the worker engine:
//
//   - `conflict` is what the upsert targets ON CONFLICT (...): the business
//     key when the table reconciles by it, the declared upsert conflict, or
//     the stored PK.
//   - `set_exclude_id` marks tables whose conflict differs from the stored
//     PK, or whose id the server assigns: the id column must not be
//     re-written on conflict (local ids diverge from the cloud row's id).
//   - `server_assigns_id` tables also drop the id from the INSERT (the
//     server assigns it), see TableSpec.
//   - `computed[].canonize` marks FKs the push re-writes to the cloud's
//     canonical id via a parent lookup (strategy fk-rewrite / key-upsert);
//     validate-only tables attach the code but never re-write.
//   - `delete_pk` is the canonical identity a tombstone travels with; a
//     computed component is resolved through its parent by the worker.

import "encoding/json"

// ManifestVersion bumps when the projection shape changes.
const ManifestVersion = 1

// ManifestComputed is one protocol-only column in worker terms.
type ManifestComputed struct {
	Column    string `json:"column"`
	Parent    string `json:"parent"`
	ParentKey string `json:"parent_key"`
	FK        string `json:"fk"`
	Target    string `json:"target"`
	Canonize  bool   `json:"canonize"`
}

// ManifestTable is one syncable table in worker terms.
type ManifestTable struct {
	Name            string             `json:"name"`
	PK              []string           `json:"pk"`
	UpsertPK        []string           `json:"upsert_pk"`
	Cols            []string           `json:"cols"`
	Conflict        []string           `json:"conflict"`
	SetExcludeID    bool               `json:"set_exclude_id"`
	ServerAssignsID bool               `json:"server_assigns_id"`
	Computed        []ManifestComputed `json:"computed,omitempty"`
}

// Manifest is the whole project contract.
type Manifest struct {
	Version   int             `json:"version"`
	Namespace string          `json:"namespace"`
	Tables    []ManifestTable `json:"tables"`
}

// Manifest projects the registry into its worker-facing form.
func (r *Registry) Manifest(namespace string) *Manifest {
	m := &Manifest{Version: ManifestVersion, Namespace: namespace}
	for _, spec := range r.specs() {
		upsertPK := r.CloudPK(spec.Name)

		conflict := upsertPK
		if len(spec.BusinessKey) > 0 {
			conflict = spec.BusinessKey
		} else if len(spec.Upsert.Conflict) > 0 {
			conflict = spec.Upsert.Conflict
		}
		if spec.NamespaceScopedConflict && !inList(conflict, "sk") {
			conflict = append(append([]string{}, conflict...), "sk")
		}

		canonize := spec.Strategy == StrategyFKRewrite || spec.Strategy == StrategyKeyUpsert
		computed := make([]ManifestComputed, 0, len(spec.Computed))
		for _, c := range spec.Computed {
			computed = append(computed, ManifestComputed{
				Column: c.Column, Parent: c.Parent, ParentKey: c.ParentKey,
				FK: c.FK, Target: c.Target, Canonize: canonize,
			})
		}

		m.Tables = append(m.Tables, ManifestTable{
			Name: spec.Name,
			PK:   spec.PK, UpsertPK: upsertPK, Cols: spec.WireCols,
			Conflict: conflict,
			// The stored id must not be re-written when the conflict key is
			// not the stored PK (local ids diverge), and ALWAYS not when the
			// server assigns it: the INSERT omits the id, so `excluded.id`
			// is NULL — keeping it in SET would null the rowid (a brand-new
			// rowid) and make the changed-guard always true.
			SetExcludeID:    !equalLists(conflict, upsertPK) || spec.ServerAssignsID,
			ServerAssignsID: spec.ServerAssignsID,
			Computed:        computed,
		})
	}
	return m
}

// ManifestJSON renders the manifest with stable indentation.
func (r *Registry) ManifestJSON(namespace string) ([]byte, error) {
	return json.MarshalIndent(r.Manifest(namespace), "", "  ")
}

func equalLists(a, b []string) bool {
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

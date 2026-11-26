package syncengine

// Schema registry: the declarative contract between the sync engine and the
// application database.
//
// The engine NEVER knows table names: every table, its canonical identity, its
// computed (protocol-only) columns and its apply strategy come from a
// per-project Registry. This is the single source of truth that the worker
// mirrors (`cloudflare-sync` TABLES) and that other clients (Flutter) reuse.
//
// The canonical PK is the row's identity IN THE CLOUD: it travels in the
// tombstones (`deletions[].pk`) and keys the anti-echo set.

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// sqlIdent guards every registry name that reaches SQL by string
// interpolation. Table and column names come from the app's own registry (code,
// never user input), but validating them keeps a typo or a hostile config from
// becoming injected SQL, and rejects names SQLite could quote-break.
var sqlIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func checkIdent(what, name string) error {
	if !sqlIdent.MatchString(name) {
		return fmt.Errorf("%s %q is not a valid SQL identifier", what, name)
	}
	return nil
}

// Strategy selects how a pulled row is applied to the local database.
type Strategy string

const (
	// StrategyReplace applies the row with INSERT OR REPLACE by its columns.
	StrategyReplace Strategy = "replace"
	// StrategyCatalog reconciles by business key (name/code), preserving the
	// local id (local FKs reference it).
	StrategyCatalog Strategy = "catalog"
	// StrategyFKRewrite resolves each computed FK to this device's local
	// parent id (by business code) and then INSERT OR REPLACE. A row whose
	// parent does not exist locally is dropped.
	StrategyFKRewrite Strategy = "fk-rewrite"
	// StrategyKeyUpsert rewrites computed FKs and then INSERT ... ON
	// CONFLICT(conflict) DO UPDATE SET ..., keeping the local row for the
	// same business identity instead of colliding on divergent ids.
	StrategyKeyUpsert Strategy = "key-upsert"
	// StrategyValidateOnly cannot be canonicalized (its PK is a tuple of
	// per-device ids): the row travels with the parent's business code and
	// is applied ONLY when that code resolves to the same local row.
	StrategyValidateOnly Strategy = "validate-only"
)

// ComputedFK describes a protocol-only column: the worker does not store it,
// it resolves it on the fly (push: attach; pull: re-write the FK with the
// cloud's canonical id). Without it each device would guess a row's identity
// from the local id of whichever device wrote it (ids diverge per device).
type ComputedFK struct {
	// Column is the protocol column carried on the wire (e.g. a student code).
	Column string
	// Parent is the table owning the business key.
	Parent string
	// ParentKey is the parent's key column (default "id").
	ParentKey string
	// FK is this table's column referencing Parent.
	FK string
	// Target is Parent's business-key column (same on every device).
	Target string
	// CodeCol is the column of this table's dedicated tombstone table that
	// holds the business code (default: Target).
	CodeCol string
}

func (c *ComputedFK) applyDefaults() {
	if c.ParentKey == "" {
		c.ParentKey = "id"
	}
	if c.CodeCol == "" {
		c.CodeCol = c.Target
	}
}

// Upsert configures StrategyKeyUpsert.
type Upsert struct {
	// Conflict lists the local conflict-target columns.
	Conflict []string
	// Set lists the columns updated on conflict. Empty means "every inserted
	// column except the conflict target".
	Set []string
	// IncludeID inserts the payload's id (keep the cloud id) instead of
	// letting the local autoincrement assign one.
	IncludeID bool
}

// OrphanPurge declares a local-only cleanup: rows whose parent is gone are
// deleted WITHOUT a tombstone (this device never deleted them; tombstoning
// would kill good rows on every other device).
type OrphanPurge struct {
	FK        string
	Parent    string
	ParentKey string
}

func (o *OrphanPurge) applyDefaults() {
	if o.ParentKey == "" {
		o.ParentKey = "id"
	}
}

// TableSpec is one syncable table's contract.
type TableSpec struct {
	// Name is the table name (local SQLite and cloud D1 share it).
	Name string
	// PK is the canonical (cloud) identity, in tombstone order. Components
	// may be computed columns (business codes) instead of stored columns.
	PK []string
	// Strategy is the apply path for pulled rows ("" means StrategyReplace).
	Strategy Strategy
	// BusinessKey lists the columns reconciled instead of the id (catalog).
	BusinessKey []string
	// NamespaceScopedConflict adds the worker's namespace column to the
	// conflict target so business keys remain unique per tenant.
	NamespaceScopedConflict bool
	// Computed lists protocol-only columns resolved through parent joins.
	Computed []ComputedFK
	// Upsert configures StrategyKeyUpsert.
	Upsert Upsert
	// TombstoneTable is the dedicated local pending-deletions table (empty
	// means deletions travel through the generic `sync_pending_deletions`).
	TombstoneTable string
	// Orphan declares the local-only orphan purge (nil when not needed).
	Orphan *OrphanPurge
	// Restored marks tables wiped by an authoritative restore.
	Restored bool
	// ServerAssignsID marks tables whose local ids diverge between devices
	// AND whose identity is the business key: the worker must NOT insert the
	// client id (a repeated local id would collide with the PK of ANOTHER
	// row, which ON CONFLICT(business key) cannot catch). Catalog tables
	// without this flag keep the "worker inserts the client id" invariant.
	ServerAssignsID bool
	// WireCols documents the columns the worker stores for this table. It
	// mirrors the worker's registry and is validated at build time.
	WireCols []string
}

// Registry is the ordered set of syncable tables. Order matters: it is the
// push payload order and the pull apply order (parents must be applied before
// the rows reconciled by their business key).
type Registry struct {
	order  []string
	byName map[string]*TableSpec
}

// NewRegistry validates the specs and returns the injectable registry.
func NewRegistry(specs []TableSpec) (*Registry, error) {
	r := &Registry{byName: map[string]*TableSpec{}}
	for i := range specs {
		spec := specs[i]
		if spec.Name == "" {
			return nil, fmt.Errorf("spec %d: empty table name", i)
		}
		if err := checkIdent("table name", spec.Name); err != nil {
			return nil, err
		}
		if _, dup := r.byName[spec.Name]; dup {
			return nil, fmt.Errorf("spec %q: duplicate table", spec.Name)
		}
		if len(spec.PK) == 0 {
			return nil, fmt.Errorf("spec %q: empty canonical PK", spec.Name)
		}
		for _, c := range spec.PK {
			if err := checkIdent(fmt.Sprintf("spec %q PK column", spec.Name), c); err != nil {
				return nil, err
			}
		}
		for _, c := range append(append([]string{}, spec.BusinessKey...), spec.WireCols...) {
			if err := checkIdent(fmt.Sprintf("spec %q column", spec.Name), c); err != nil {
				return nil, err
			}
		}
		for _, c := range append(append([]string{}, spec.Upsert.Conflict...), spec.Upsert.Set...) {
			if err := checkIdent(fmt.Sprintf("spec %q upsert column", spec.Name), c); err != nil {
				return nil, err
			}
		}
		if spec.TombstoneTable != "" {
			if err := checkIdent("tombstone table", spec.TombstoneTable); err != nil {
				return nil, err
			}
		}
		switch spec.Strategy {
		case "", StrategyReplace, StrategyCatalog, StrategyFKRewrite, StrategyKeyUpsert, StrategyValidateOnly:
		default:
			return nil, fmt.Errorf("spec %q: unknown strategy %q", spec.Name, spec.Strategy)
		}
		if spec.Strategy == StrategyCatalog && len(spec.BusinessKey) == 0 {
			return nil, fmt.Errorf("spec %q: catalog strategy needs a business key", spec.Name)
		}
		if spec.ServerAssignsID && len(spec.BusinessKey) == 0 {
			return nil, fmt.Errorf("spec %q: server-assigned id needs a business key", spec.Name)
		}
		if spec.Strategy == StrategyKeyUpsert && len(spec.Upsert.Conflict) == 0 {
			return nil, fmt.Errorf("spec %q: key-upsert strategy needs a conflict target", spec.Name)
		}
		if (spec.Strategy == StrategyFKRewrite || spec.Strategy == StrategyValidateOnly) &&
			len(spec.Computed) == 0 {
			return nil, fmt.Errorf("spec %q: strategy %q needs computed columns", spec.Name, spec.Strategy)
		}
		computed := map[string]bool{}
		for j := range spec.Computed {
			c := &spec.Computed[j]
			c.applyDefaults()
			if c.Column == "" || c.Parent == "" || c.FK == "" || c.Target == "" {
				return nil, fmt.Errorf("spec %q: computed column %d is incomplete", spec.Name, j)
			}
			for _, id := range []string{c.Column, c.Parent, c.ParentKey, c.FK, c.Target, c.CodeCol} {
				if err := checkIdent(fmt.Sprintf("spec %q computed name", spec.Name), id); err != nil {
					return nil, err
				}
			}
			computed[c.Column] = true
		}
		for _, pk := range spec.PK {
			if !computed[pk] && len(spec.WireCols) > 0 && !inList(spec.WireCols, pk) {
				return nil, fmt.Errorf("spec %q: canonical PK column %q is neither stored nor computed", spec.Name, pk)
			}
		}
		if spec.Orphan != nil {
			spec.Orphan.applyDefaults()
			if spec.Orphan.FK == "" || spec.Orphan.Parent == "" {
				return nil, fmt.Errorf("spec %q: orphan purge is incomplete", spec.Name)
			}
		}
		r.order = append(r.order, spec.Name)
		r.byName[spec.Name] = &spec
	}
	return r, nil
}

// Tables returns the syncable tables in payload/apply order.
func (r *Registry) Tables() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// specs returns the table contracts in payload/apply order.
func (r *Registry) specs() []*TableSpec {
	out := make([]*TableSpec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}

// Spec returns one table's contract.
func (r *Registry) Spec(name string) (*TableSpec, bool) {
	spec, ok := r.byName[name]
	return spec, ok
}

// PKCols returns the canonical PK columns of a table (nil when unknown).
func (r *Registry) PKCols(name string) []string {
	spec, ok := r.byName[name]
	if !ok {
		return nil
	}
	out := make([]string, len(spec.PK))
	copy(out, spec.PK)
	return out
}

// computedColumn finds the computed spec behind a PK/protocol column.
func computedColumn(spec *TableSpec, col string) (*ComputedFK, bool) {
	for i := range spec.Computed {
		if spec.Computed[i].Column == col {
			return &spec.Computed[i], true
		}
	}
	return nil, false
}

// CloudPK returns the canonical PK with computed columns replaced by the
// stored FK column: the identity the cloud actually holds (used to tombstone
// remote rows during a wipe).
func (r *Registry) CloudPK(name string) []string {
	spec, ok := r.byName[name]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(spec.PK))
	for _, c := range spec.PK {
		if comp, isComputed := computedColumn(spec, c); isComputed {
			out = append(out, comp.FK)
		} else {
			out = append(out, c)
		}
	}
	return out
}

// RestoredTables returns the tables wiped by an authoritative restore.
func (r *Registry) RestoredTables() []string {
	out := []string{}
	for _, name := range r.order {
		if r.byName[name].Restored {
			out = append(out, name)
		}
	}
	return out
}

// TombstoneTables returns every local pending-deletions table to clear after
// a real push: the generic one plus every dedicated table in registry order.
func (r *Registry) TombstoneTables() []string {
	out := []string{"sync_pending_deletions"}
	seen := map[string]bool{"sync_pending_deletions": true}
	for _, name := range r.order {
		t := r.byName[name].TombstoneTable
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// PKPart builds the canonical PK string "v1|v2|" (trailing "|"), mirror of
// the Rust `pk_part`: strings are JSON-encoded (with quotes), nulls are
// "null". It keys the anti-echo set.
func (r *Registry) PKPart(table string, row Row) string {
	spec, ok := r.byName[table]
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, c := range spec.PK {
		b.WriteString(jsonValueString(row[c]))
		b.WriteByte('|')
	}
	return b.String()
}

// RowRecentKey builds the full anti-echo key "table|pk|updated_at".
func (r *Registry) RowRecentKey(table string, row Row) string {
	ua, _ := row["updated_at"].(string)
	return table + "|" + r.PKPart(table, row) + "|" + ua
}

// CountSyncableRows counts the syncable rows in the local database right
// now. A table may not exist yet in an old database: it is ignored.
func (r *Registry) CountSyncableRows(conn *sql.DB) (int64, error) {
	var total int64
	for _, t := range r.order {
		var n int64
		if err := conn.QueryRow("SELECT COUNT(*) FROM \"" + t + "\"").Scan(&n); err != nil {
			continue
		}
		total += n
	}
	return total, nil
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

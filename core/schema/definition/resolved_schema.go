package definition

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/common"
)

// =============================================================================
// RESOLVED SCHEMA IR TYPES
// =============================================================================
//
// ResolvedSchema is the canonical intermediate representation of a Schema.
// Produced once by Compile and immutable after construction.
//
// Design invariants:
//   - Every FieldSchemaAs, ConstraintAs, and effective-type resolution call
//     happens inside the compiler. None survive into the IR.
//   - Every three-level constraint merge is performed once per scope.
//     Results are stored as []ResolvedConstraint.
//   - All constraint field paths are pre-computed: absolute (global scope)
//     and relative (recursive scope). No consumer calls path resolution.
//   - All enum lookup tables are pre-built. No consumer calls buildEnumLookup.
//   - Recursive back-references are ResolvedRecursiveRef — not expanded trees.
//   - Fields are in stable sorted order (by UUIDv7 FieldId).

// ResolvedSchema is the fully compiled top-level schema.
type ResolvedSchema struct {
	Fields      []ResolvedField
	Constraints []ResolvedConstraint
	Indexes     map[IndexID]Index
	// All named sub-schemas, each resolved to exactly one level.
	// This is the sole authoritative registry after compilation.
	Schemas map[SchemaId]*ResolvedNestedSchema
}

// ResolvedNestedSchema is a named schema resolved to one level.
// Fields that close a recursive cycle carry a non-nil Recursive rather
// than an inline expansion (which would be infinite).
type ResolvedNestedSchema struct {
	ID            SchemaId
	Name          string
	EffectiveType FieldType
	Fields        []ResolvedField
	IsRecursive   bool
	// Non-nil only when EffectiveType == FieldTypeEnum.
	Enum *ResolvedEnum

	// RawConstraints holds the uncompiled constraint map from the source schema.
	// Constraints on a nested schema must be compiled relative to their mount
	// path, which is only known at validator-build time (not compile time).
	//
	// The validator builder calls SchemaCompiler.CompileConstraints(
	//   nested:    rns.RawConstraints,
	//   schemaRef: field.Object.RefConstraints,
	//   topLevel:  compiler.TopLevelConstraintsForPath(mountPath),
	//   basePath:  mountPath,
	// ) when inlining this schema into a validation graph.
	//
	// For recursive subgraphs, it passes basePath="" instead of the mount path,
	// since the recursive graph's traversal context has the instance as its root.
	RawConstraints SchemaConstraint

	// Indexes from the source BaseSchema. Preserved for consumers such as
	// query builders and storage adapters that need index information.
	Indexes map[IndexID]Index
}

// ResolvedField is a fully resolved field. All type ambiguity is eliminated.
// Exactly one of the type-specific pointer fields is non-nil, chosen by Type.
// Recursive is the exception: it is set instead of Object/Container/Set/Union/Composite
// when the field closes a recursive reference cycle.
type ResolvedField struct {
	ID          FieldId    // stable UUIDv7 — never changes across renames
	Name        FieldName
	Description string
	Path        string   // absolute dot-separated path from schema root
	Parts       []string // pre-split path parts for zero-alloc traversal
	Type        FieldType
	Required    bool
	Deprecated  bool
	Unique      bool
	Nullable    bool
	Default     LiteralValue

	// Exactly one is non-nil. Recursive takes precedence over Object when
	// a field closes a cycle (Object is nil; Recursive is set).
	Scalar    *ResolvedScalar
	Enum      *ResolvedEnum
	Object    *ResolvedObjectField
	Container *ResolvedContainer // covers FieldTypeArray and FieldTypeRecord
	Union     *ResolvedUnion
	Composite *ResolvedComposite
	Recursive *ResolvedRecursiveRef
	// FieldTypeGeometry needs no additional data.
}

// ResolvedScalar covers: String, Number, Integer, Decimal, Boolean, Bytes, Unknown.
type ResolvedScalar struct{}

// ResolvedEnum holds the pre-built value set for an enum field or schema.
type ResolvedEnum struct {
	Lookup        map[any]struct{} // O(1) for comparable values
	Complex       []any            // non-comparable; requires deepEqual
	ExpectNumeric bool             // true when schema type is numeric
}

// ResolvedObjectField is a resolved object-typed field referencing a named schema.
type ResolvedObjectField struct {
	Schema         *ResolvedNestedSchema // direct pointer, never nil
	RefConstraints SchemaConstraint      // call-site overrides; may be nil
}

// ResolvedContainer covers FieldTypeArray and FieldTypeRecord.
// ItemSchema and ItemType are mutually exclusive.
type ResolvedContainer struct {
	ItemSchema *ResolvedNestedSchema // set for named item schemas
	ItemType   FieldType             // set for inline descriptors
}

// ResolvedUnion holds ordered variant schemas for a union-typed field.
type ResolvedUnion struct {
	Variants []*ResolvedNestedSchema // in source declaration order
}

// ResolvedComposite holds the parts of a composite-typed field.
type ResolvedComposite struct {
	ObjectParts      []*ResolvedNestedSchema
	UnionParts       []ResolvedUnion
	MergedVocabulary map[string]bool // all fields from all parts and variants
	ObjectVocabulary map[string]bool // fields from object parts only
}

// ResolvedRecursiveRef marks a recursive boundary. It does not expand the
// schema — expansion is infinite. The base schema is available via
// ResolvedSchema.Schemas[SchemaID].
//
// Consumer contracts:
//   - Validator: builds a per-(SchemaID, RefConstraints) subgraph, cached.
//   - Query builder: emits recursive query constructs.
//   - Serialiser: recurses bounded by data depth.
//   - Documentation: emits "recursive reference to SchemaName".
type ResolvedRecursiveRef struct {
	SchemaID       SchemaId
	SchemaName     string
	RefConstraints SchemaConstraint // call-site overrides; may be nil
}

// ResolvedConstraint is a fully extracted constraint with pre-computed paths.
// Exactly one of Rule or Group is non-nil.
type ResolvedConstraint struct {
	Name  string
	Scope ConstraintScope

	Rule  *ResolvedConstraintRule
	Group *ResolvedConstraintGroup

	// AbsField* — paths relative to schema root (for global-scope evaluation).
	AbsFieldPaths []string
	AbsFieldParts [][]string

	// RelField* — paths as declared in the constraint (for recursive-scope
	// evaluation, where the validator operates on the instance root).
	RelFieldPaths []string
	RelFieldParts [][]string
}

// ResolvedConstraintRule is a fully extracted constraint rule.
type ResolvedConstraintRule struct {
	Predicate  PredicateName
	Fields     []FieldName
	Parameters LiteralValue
}

// ResolvedConstraintGroup is a recursively resolved constraint group.
type ResolvedConstraintGroup struct {
	Operator common.LogicalOperator
	Members  []ResolvedConstraint
	// Pre-computed union of all field paths across all member rules at
	// any nesting depth. Used for presence-checking before evaluation.
	RequiredFieldPaths []string
	RequiredFieldParts [][]string
}

// =============================================================================
// COMPILER
// =============================================================================

// SchemaCompiler translates a *Schema into a *ResolvedSchema in a single
// memoised pass. It is the sole consumer of *Schema.
type SchemaCompiler struct {
	source *Schema

	// building tracks schemas currently mid-compilation (cycle detection).
	building map[SchemaId]bool

	// resolved memoises completed ResolvedNestedSchema instances.
	resolved map[SchemaId]*ResolvedNestedSchema
}

// Compile translates a source schema into its resolved IR.
// The result is immutable and safe for concurrent reads after return.
func Compile(sc *Schema) (*ResolvedSchema, error) {
	c := &SchemaCompiler{
		source:   sc,
		building: make(map[SchemaId]bool),
		resolved: make(map[SchemaId]*ResolvedNestedSchema),
	}
	return c.compile()
}

// newSchemaCompiler creates a SchemaCompiler from a Schema for constraint
// compilation.  The Schema must be the same one that was (or will be) passed
// to Compile.  The returned compiler's resolved map is intentionally empty;
// it is only used for CompileConstraints and TopLevelConstraintsForPath,
// which don't need the resolved cache.
func newSchemaCompiler(sc *Schema) *SchemaCompiler {
	return &SchemaCompiler{
		source:   sc,
		building: make(map[SchemaId]bool),
		resolved: make(map[SchemaId]*ResolvedNestedSchema),
	}
}

func (c *SchemaCompiler) compile() (*ResolvedSchema, error) {
	// Pre-compile all named sub-schemas so that forward references from root
	// fields resolve correctly regardless of declaration order.
	for id := range c.source.Schemas {
		if _, err := c.compileNestedSchema(id); err != nil {
			return nil, err
		}
	}

	rootFields, err := c.compileFields(c.source.Fields, "")
	if err != nil {
		return nil, err
	}

	rootConstraints, err := c.compileConstraintMap(c.source.Constraints, "")
	if err != nil {
		return nil, err
	}

	return &ResolvedSchema{
		Fields:      rootFields,
		Constraints: rootConstraints,
		Indexes:     c.source.Indexes,
		Schemas:     c.resolved,
	}, nil
}

// =============================================================================
// NESTED SCHEMA COMPILATION
// =============================================================================

// compileNestedSchema compiles the named schema id.
// Returns the memoised result if already compiled.
// Returns nil, nil when a recursive cycle is detected — callers must emit
// a ResolvedRecursiveRef in this case.
func (c *SchemaCompiler) compileNestedSchema(id SchemaId) (*ResolvedNestedSchema, error) {
	if r, ok := c.resolved[id]; ok {
		return r, nil
	}
	if c.building[id] {
		// Cycle: signal to caller to emit a ResolvedRecursiveRef.
		return nil, nil
	}

	ns, exists := c.source.Schemas[id]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("schema '%s' not found", id))
	}

	c.building[id] = true
	defer func() { delete(c.building, id) }()

	rns := &ResolvedNestedSchema{
		ID:            id,
		Name:          ns.Name,
		EffectiveType: resolveEffectiveType(&ns),
	}
	// Optimistically store so self-references within this schema's fields
	// find the entry in resolved and return it (bypassing building check).
	// This handles the case where a schema's field references the schema
	// itself and compileNestedSchema is re-entered.
	c.resolved[id] = rns

	if len(ns.Fields) > 0 {
		fields, err := c.compileFields(ns.Fields, "")
		if err != nil {
			delete(c.resolved, id)
			return nil, fmt.Errorf("schema '%s' fields: %w", id, err)
		}
		rns.Fields = fields
	}

	// Store the raw constraint map. Constraints cannot be pre-compiled here
	// because their field paths depend on the mount path, which is only known
	// at validator-build time when the schema is inlined at a specific field path.
	rns.RawConstraints = ns.Constraints

	// Preserve indexes for downstream consumers (query builders, storage adapters).
	if len(ns.Indexes) > 0 {
		rns.Indexes = ns.Indexes
	}

	if rns.EffectiveType == FieldTypeEnum && len(ns.Values) > 0 {
		rns.Enum = buildResolvedEnum([]LiteralValue(ns.Values), ns.Type)
	}

	for _, f := range rns.Fields {
		if f.Recursive != nil {
			rns.IsRecursive = true
			break
		}
	}

	return rns, nil
}

// =============================================================================
// FIELD COMPILATION
// =============================================================================

// compileFields converts a map[FieldId]Field to a sorted []ResolvedField.
// Fields are sorted by FieldId (UUIDv7), which is time-ordered.
// This guarantees that adding a new field always appends — existing fields
// never change position.
func (c *SchemaCompiler) compileFields(fields map[FieldId]Field, basePath string) ([]ResolvedField, error) {
	if len(fields) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(fields))
	for id := range fields {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	result := make([]ResolvedField, 0, len(fields))
	for _, idStr := range ids {
		id := FieldId(idStr)
		f := fields[id]
		rf, err := c.compileField(f, id, basePath)
		if err != nil {
			return nil, err
		}
		result = append(result, rf)
	}
	return result, nil
}

// compileField compiles a single Field into a ResolvedField.
func (c *SchemaCompiler) compileField(f Field, id FieldId, basePath string) (ResolvedField, error) {
	path, parts := buildAbsPath(basePath, string(f.Name))
	typ := f.Type // FieldProperties.Type is always authoritative for Field

	rf := ResolvedField{
		ID:          id,
		Name:        f.Name,
		Description: f.Description,
		Path:        path,
		Parts:       parts,
		Type:        typ,
		Required:    f.Required,
		Deprecated:  f.Deprecated,
		Unique:      f.Unique,
		Nullable:    f.Nullable,
		Default:     f.Default,
	}

	var err error
	switch typ {
	case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeDecimal,
		FieldTypeBoolean, FieldTypeBytes, FieldTypeUnknown:
		rf.Scalar = &ResolvedScalar{}

	case FieldTypeGeometry:
		// No additional data needed.

	case FieldTypeEnum:
		rf.Enum, err = c.compileEnumField(f, path)

	case FieldTypeObject:
		rf.Object, rf.Recursive, err = c.compileObjectField(f, path)

	case FieldTypeArray, FieldTypeRecord:
		var rc *ResolvedContainer
		rc, rf.Recursive, err = c.compileContainerField(f, path)
		rf.Container = rc

	case FieldTypeUnion:
		rf.Union, err = c.compileUnionField(f, path)

	case FieldTypeComposite:
		rf.Composite, err = c.compileCompositeField(f, path)
	}

	return rf, err
}

// =============================================================================
// TYPE-SPECIFIC FIELD COMPILERS
// All return (result, *ResolvedRecursiveRef, error) when the field type can
// encounter a recursive cycle. The caller sets rf.Recursive when non-nil.
// =============================================================================

func (c *SchemaCompiler) compileEnumField(f Field, path string) (*ResolvedEnum, error) {
	if f.Schema.IsZero() {
		return nil, ErrSchemaNotFound.
			WithMessage("enum field must have a schema reference").WithPath(path)
	}

	var refs []SchemaReference
	if f.Schema.IsSingle() {
		ref, err := FieldSchemaAs[SchemaReference](f.Schema)
		if err != nil {
			return nil, ErrSchemaNotFound.
				WithMessage(fmt.Sprintf("enum schema reference: %v", err)).
				WithPath(path).WithCause(err)
		}
		refs = []SchemaReference{ref}
	} else if f.Schema.IsMultiple() {
		multi, err := FieldSchemaAs[[]SchemaReference](f.Schema)
		if err != nil {
			return nil, ErrSchemaNotFound.
				WithMessage(fmt.Sprintf("enum schema references: %v", err)).
				WithPath(path).WithCause(err)
		}
		refs = multi
	} else {
		return nil, ErrInvalidSchema.
			WithMessage("enum schema reference must be single or multiple").WithPath(path)
	}

	enum := &ResolvedEnum{Lookup: make(map[any]struct{})}
	var firstType FieldType

	for _, ref := range refs {
		var values []LiteralValue
		var typ FieldType

		if ref.IsInline() {
			if ref.Type == 0 {
				return nil, ErrInvalidSchema.
					WithMessage("inline enum descriptor missing 'type'").WithPath(path)
			}
			if len(ref.Values) == 0 {
				return nil, ErrInvalidSchema.
					WithMessage("inline enum descriptor missing 'values'").WithPath(path)
			}
			values = ref.Values
			typ = ref.Type
		} else if(len(ref.ID) != 0) {
			ns, exists := c.source.Schemas[ref.ID]
			if !exists {
				return nil, ErrSchemaNotFound.
					WithMessage(fmt.Sprintf("enum schema '%s' not found", ref.ID)).WithPath(path)
			}
			if len(ns.Values) == 0 {
				return nil, ErrInvalidSchema.
					WithMessagef("enum schema '%s' has no values defined", ref.ID).WithPath(path)
			}
			values = []LiteralValue(ns.Values)
			typ = ns.Type
		} else {
			return nil, ErrInvalidSchema.
				WithMessage("enum reference must be named or inline").WithPath(path)
		}

		if firstType == 0 {
			firstType = typ
		}
		mergeEnumValues(enum, values)
	}

	if len(enum.Lookup) == 0 && len(enum.Complex) == 0 {
		return nil, ErrInvalidSchema.
			WithMessage("enum field has no values after resolving all references").WithPath(path)
	}

	enum.ExpectNumeric = firstType == FieldTypeNumber ||
		firstType == FieldTypeDecimal ||
		firstType == FieldTypeInteger

	return enum, nil
}

// compileObjectField resolves an object field's named schema reference.
// Returns (obj, nil, nil) on success, (nil, rec, nil) on recursive cycle,
// (nil, nil, err) on error.
func (c *SchemaCompiler) compileObjectField(f Field, path string) (*ResolvedObjectField, *ResolvedRecursiveRef, error) {
	if f.Schema.IsZero() {
		return nil, nil, ErrSchemaNotFound.
			WithMessage("object field must have a schema reference").WithPath(path)
	}

	ref, err := FieldSchemaAs[SchemaReference](f.Schema)
	if err != nil {
		return nil, nil, ErrSchemaNotFound.
			WithMessage(fmt.Sprintf("object schema reference: %v", err)).
			WithPath(path).WithCause(err)
	}

	rns, rec, err := c.lookupSchema(ref.ID, ref.Constraints, path)
	if err != nil {
		return nil, nil, err
	}
	if rec != nil {
		return nil, rec, nil
	}

	return &ResolvedObjectField{Schema: rns, RefConstraints: ref.Constraints}, nil, nil
}

// compileContainerField resolves array/record item schema.
// Returns (container, nil, nil) on success, (nil, rec, nil) on recursive cycle.
func (c *SchemaCompiler) compileContainerField(f Field, path string) (*ResolvedContainer, *ResolvedRecursiveRef, error) {
	if f.Schema.IsZero() {
		return &ResolvedContainer{}, nil, nil
	}
	if !f.Schema.IsSingle() {
		return nil, nil, ErrInvalidSchema.
			WithMessage("array/record field cannot reference multiple schemas").WithPath(path)
	}

	ref, err := FieldSchemaAs[SchemaReference](f.Schema)
	if err != nil {
		return nil, nil, ErrSchemaNotFound.
			WithMessage(fmt.Sprintf("container schema reference: %v", err)).
			WithPath(path).WithCause(err)
	}

	if ref.IsInline() {
		if ref.Type == 0 {
			return nil, nil, ErrInvalidSchema.
				WithMessage("inline container descriptor missing 'type'").WithPath(path)
		}
		return &ResolvedContainer{ItemType: ref.Type}, nil, nil
	}

	rns, rec, err := c.lookupSchema(ref.ID, ref.Constraints, path)
	if err != nil {
		return nil, nil, err
	}
	if rec != nil {
		// A container whose item schema is recursive. We represent this as
		// a RecursiveRef on the container field itself. The validator will
		// build the recursive item subgraph per-traversal.
		return nil, rec, nil
	}

	return &ResolvedContainer{ItemSchema: rns}, nil, nil
}

// compileUnionField resolves a union field's variant schemas.
// Union variants cannot themselves be top-level recursive references in the
// current model — each variant is a distinct named schema. If a variant
// schema is recursive internally, that is captured in its own ResolvedNestedSchema.
func (c *SchemaCompiler) compileUnionField(f Field, path string) (*ResolvedUnion, error) {
	if f.Schema.IsZero() || !f.Schema.IsMultiple() {
		return nil, ErrSchemaNotFound.
			WithMessage("union field must reference multiple schemas").WithPath(path)
	}

	refs, err := FieldSchemaAs[[]SchemaReference](f.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.
			WithMessage(fmt.Sprintf("union schema references: %v", err)).
			WithPath(path).WithCause(err)
	}

	variants := make([]*ResolvedNestedSchema, 0, len(refs))
	for _, ref := range refs {
		rns, rec, err := c.lookupSchema(ref.ID, ref.Constraints, path)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			// A union variant that is itself a recursive schema. This is unusual
			// but valid. We cannot store a *ResolvedRecursiveRef inside a
			// []*ResolvedNestedSchema slice. Instead, store the partially-compiled
			// schema pointer that was optimistically registered — it will have
			// IsRecursive = true once fully compiled.
			rns = c.resolved[ref.ID] // may be the optimistic entry
		}
		variants = append(variants, rns)
	}

	return &ResolvedUnion{Variants: variants}, nil
}

// compileCompositeField resolves a composite field's object and union parts.
func (c *SchemaCompiler) compileCompositeField(f Field, path string) (*ResolvedComposite, error) {
	if f.Schema.IsZero() || !f.Schema.IsMultiple() {
		return nil, ErrSchemaNotFound.
			WithMessage("composite field must reference multiple schemas").WithPath(path)
	}

	refs, err := FieldSchemaAs[[]SchemaReference](f.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.
			WithMessage(fmt.Sprintf("composite schema references: %v", err)).
			WithPath(path).WithCause(err)
	}

	composite := &ResolvedComposite{
		MergedVocabulary: make(map[string]bool),
		ObjectVocabulary: make(map[string]bool),
	}

	for _, ref := range refs {
		ns, exists := c.source.Schemas[ref.ID]
		if !exists {
			return nil, ErrSchemaNotFound.
				WithMessage(fmt.Sprintf("composite part schema '%s' not found", ref.ID)).WithPath(path)
		}

		effectiveType := resolveEffectiveType(&ns)

		switch effectiveType {
		case FieldTypeObject, 0:
			rns, rec, err := c.lookupSchema(ref.ID, ref.Constraints, path)
			if err != nil {
				return nil, err
			}
			if rec != nil {
				rns = c.resolved[ref.ID]
			}
			composite.ObjectParts = append(composite.ObjectParts, rns)
			for _, field := range ns.Fields {
				n := string(field.Name)
				composite.ObjectVocabulary[n] = true
				composite.MergedVocabulary[n] = true
			}

		case FieldTypeUnion:
			if ns.Schema.IsZero() || !ns.Schema.IsMultiple() {
				return nil, ErrSchemaNotFound.
					WithMessage(fmt.Sprintf(
						"union part '%s' in composite must reference multiple schemas", ref.ID,
					)).WithPath(path)
			}
			variantRefs, err := FieldSchemaAs[[]SchemaReference](ns.Schema)
			if err != nil {
				return nil, ErrSchemaNotFound.
					WithMessage(fmt.Sprintf("composite union part '%s': %v", ref.ID, err)).
					WithPath(path).WithCause(err)
			}

			union := ResolvedUnion{Variants: make([]*ResolvedNestedSchema, 0, len(variantRefs))}
			for _, vRef := range variantRefs {
				vns, rec, err := c.lookupSchema(vRef.ID, vRef.Constraints, path)
				if err != nil {
					return nil, err
				}
				if rec != nil {
					vns = c.resolved[vRef.ID]
				}
				union.Variants = append(union.Variants, vns)

				if vSrc, ok := c.source.Schemas[vRef.ID]; ok {
					for _, field := range vSrc.Fields {
						composite.MergedVocabulary[string(field.Name)] = true
					}
				}
			}
			composite.UnionParts = append(composite.UnionParts, union)

		default:
			return nil, ErrInvalidSchema.
				WithMessagef(
					"composite part '%s' has unsupported effective type '%s'; must be object or union",
					ref.ID, effectiveType,
				).WithPath(path)
		}
	}

	return composite, nil
}

// =============================================================================
// CONSTRAINT COMPILATION
// =============================================================================

// compileConstraintMap compiles a raw constraint map into []ResolvedConstraint.
// basePath is the scope path ("" = root/global scope).
func (c *SchemaCompiler) compileConstraintMap(
	constraints map[ConstraintId]Constraint,
	basePath string,
) ([]ResolvedConstraint, error) {
	if len(constraints) == 0 {
		return nil, nil
	}

	scope := constraintScope(basePath)
	baseParts := splitFieldPath(basePath)

	// Sort for deterministic ordering.
	ids := make([]string, 0, len(constraints))
	for id := range constraints {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	result := make([]ResolvedConstraint, 0, len(constraints))
	for _, idStr := range ids {
		rc, err := c.compileConstraint(constraints[ConstraintId(idStr)], basePath, baseParts, scope)
		if err != nil {
			return nil, err
		}
		result = append(result, rc)
	}
	return result, nil
}

// CompileConstraints performs the three-level constraint merge for a nested
// schema being inlined at a specific mount path, and returns the resolved
// effective constraint set.
//
// Called by the validator builder when wiring constraints for an object field:
//
//	constraints, err := compiler.CompileConstraints(
//	    rns.RawConstraints,          // nested schema's own constraints
//	    field.Object.RefConstraints, // call-site overrides from SchemaReference
//	    compiler.TopLevelConstraintsForPath(mountPath), // top-level overrides
//	    mountPath,                   // e.g. "address" or "order.shipping"
//	)
//
// For recursive subgraphs, pass basePath="" (the subgraph's traversal root
// is the recursive instance itself, not the document root).
//
// Precedence (highest wins on name collision):
//
//	3. topLevel  — top-level schema constraints referencing this path
//	2. schemaRef — constraints from the SchemaReference call site
//	1. nested    — constraints from the NestedSchema definition
func (c *SchemaCompiler) CompileConstraints(
	nested SchemaConstraint,
	schemaRef SchemaConstraint,
	topLevel SchemaConstraint,
	basePath string,
) ([]ResolvedConstraint, error) {
	return c.compileConstraintMerged(nested, schemaRef, topLevel, basePath)
}

// TopLevelConstraintsForPath returns the subset of top-level schema constraints
// whose field references overlap with fieldPath or its sub-paths.
// Used by the validator builder as the third argument to CompileConstraints.
func (c *SchemaCompiler) TopLevelConstraintsForPath(fieldPath string) SchemaConstraint {
	return c.topLevelConstraintsForPath(fieldPath)
}

// compileConstraintMerged performs the three-level constraint merge and
// returns the resolved effective constraint set for a given scope path.
//
// Precedence (highest wins on name collision):
//   3. topLevel  — top-level schema constraints referencing this path
//   2. schemaRef — constraints from the SchemaReference call site
//   1. nested    — constraints from the NestedSchema definition
func (c *SchemaCompiler) compileConstraintMerged(
	nested SchemaConstraint,
	schemaRef SchemaConstraint,
	topLevel SchemaConstraint,
	basePath string,
) ([]ResolvedConstraint, error) {
	if len(nested) == 0 && len(schemaRef) == 0 && len(topLevel) == 0 {
		return nil, nil
	}

	type entry struct {
		constraint  Constraint
		specificity ConstraintSpecificity
	}
	registry := make(map[string]entry)

	apply := func(constraints SchemaConstraint, specificity ConstraintSpecificity) {
		for _, con := range constraints {
			existing, exists := registry[con.Name]
			if !exists || specificity >= existing.specificity {
				registry[con.Name] = entry{constraint: con, specificity: specificity}
			}
		}
	}
	apply(nested, SpecificityNestedSchema)
	apply(schemaRef, SpecificitySchemaReference)
	apply(topLevel, SpecificityTopLevel)

	scope := constraintScope(basePath)
	baseParts := splitFieldPath(basePath)

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]ResolvedConstraint, 0, len(registry))
	for _, name := range names {
		rc, err := c.compileConstraint(registry[name].constraint, basePath, baseParts, scope)
		if err != nil {
			return nil, fmt.Errorf("merged constraint '%s': %w", name, err)
		}
		result = append(result, rc)
	}
	return result, nil
}

// compileConstraint extracts and resolves a single Constraint.
func (c *SchemaCompiler) compileConstraint(
	constraint Constraint,
	basePath string,
	baseParts []string,
	scope ConstraintScope,
) (ResolvedConstraint, error) {
	rc := ResolvedConstraint{Name: constraint.Name, Scope: scope}

	switch constraint.Kind() {
	case ConstraintKindRule:
		rule, err := ConstraintAs[*ConstraintRule](constraint.ConstraintUnion)
		if err != nil {
			return ResolvedConstraint{}, ErrInvalidSchema.
				WithMessage(fmt.Sprintf("constraint '%s': extract rule: %v", constraint.Name, err))
		}
		rc.Rule = &ResolvedConstraintRule{
			Predicate:  rule.Predicate,
			Fields:     rule.Fields,
			Parameters: rule.Parameters,
		}
		rc.AbsFieldPaths, rc.AbsFieldParts = resolveAbsFieldPaths(basePath, baseParts, rule.Fields)
		rc.RelFieldPaths, rc.RelFieldParts = resolveRelFieldPaths(rule.Fields)

	case ConstraintKindGroup:
		group, err := ConstraintAs[*ConstraintGroup](constraint.ConstraintUnion)
		if err != nil {
			return ResolvedConstraint{}, ErrInvalidSchema.
				WithMessage(fmt.Sprintf("constraint '%s': extract group: %v", constraint.Name, err))
		}
		rg, err := c.compileConstraintGroup(group, basePath, baseParts, scope)
		if err != nil {
			return ResolvedConstraint{}, fmt.Errorf("constraint group '%s': %w", constraint.Name, err)
		}
		rc.Group = rg
		rc.AbsFieldPaths = rg.RequiredFieldPaths
		rc.AbsFieldParts = rg.RequiredFieldParts
		rc.RelFieldPaths, rc.RelFieldParts = stripPathPrefix(basePath, len(baseParts), rg.RequiredFieldPaths, rg.RequiredFieldParts)

	default:
		return ResolvedConstraint{}, ErrInvalidSchema.
			WithMessage(fmt.Sprintf("constraint '%s': unknown kind %d", constraint.Name, constraint.Kind()))
	}

	return rc, nil
}

// compileConstraintGroup recursively resolves a ConstraintGroup and pre-computes
// the union of all referenced field paths across all member rules.
func (c *SchemaCompiler) compileConstraintGroup(
	group *ConstraintGroup,
	basePath string,
	baseParts []string,
	scope ConstraintScope,
) (*ResolvedConstraintGroup, error) {
	rg := &ResolvedConstraintGroup{
		Operator: group.Operator,
		Members:  make([]ResolvedConstraint, 0, len(group.Rules)),
	}
	requiredMap := make(map[string][]string) // abs path → parts

	for i, ruleUnion := range group.Rules {
		// Wrap in a synthetic Constraint for compileConstraint.
		// Members within groups have no source-level name; use index as identifier.
		synthetic := Constraint{
			Name:            fmt.Sprintf("member_%d", i),
			ConstraintUnion: ruleUnion,
		}
		member, err := c.compileConstraint(synthetic, basePath, baseParts, scope)
		if err != nil {
			return nil, fmt.Errorf("group member %d: %w", i, err)
		}
		rg.Members = append(rg.Members, member)

		for j, p := range member.AbsFieldPaths {
			if _, seen := requiredMap[p]; !seen {
				requiredMap[p] = member.AbsFieldParts[j]
			}
		}
	}

	paths := make([]string, 0, len(requiredMap))
	for p := range requiredMap {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	rg.RequiredFieldPaths = paths
	rg.RequiredFieldParts = make([][]string, len(paths))
	for i, p := range paths {
		rg.RequiredFieldParts[i] = requiredMap[p]
	}

	return rg, nil
}

// topLevelConstraintsForPath extracts top-level constraints whose field
// references overlap with fieldPath or its sub-paths.
func (c *SchemaCompiler) topLevelConstraintsForPath(fieldPath string) SchemaConstraint {
	if fieldPath == "" {
		return nil
	}
	result := make(SchemaConstraint)
	matches := func(name FieldName) bool {
		s := string(name)
		return s == fieldPath || strings.HasPrefix(s, fieldPath+".")
	}
	for id, constraint := range c.source.Constraints {
		switch constraint.Kind() {
		case ConstraintKindRule:
			rule, err := ConstraintAs[*ConstraintRule](constraint.ConstraintUnion)
			if err != nil {
				continue
			}
			for _, fn := range rule.Fields {
				if matches(fn) {
					result[id] = constraint
					break
				}
			}
		case ConstraintKindGroup:
			group, err := ConstraintAs[*ConstraintGroup](constraint.ConstraintUnion)
			if err != nil {
				continue
			}
			for _, fn := range allGroupFieldNames(group) {
				if matches(fn) {
					result[id] = constraint
					break
				}
			}
		}
	}
	return result
}

// =============================================================================
// SCHEMA LOOKUP — centralised cycle detection
// =============================================================================

// lookupSchema resolves a SchemaId to its *ResolvedNestedSchema.
// It is the single point of cycle detection for all field type compilers.
//
// Returns:
//   - (rns, nil, nil)  — success
//   - (nil, rec, nil)  — recursive cycle at this call site
//   - (nil, nil, err)  — schema not found or compilation error
func (c *SchemaCompiler) lookupSchema(
	id SchemaId,
	refConstraints SchemaConstraint,
	atPath string,
) (*ResolvedNestedSchema, *ResolvedRecursiveRef, error) {
	// Cycle detected: this schema is currently being compiled.
	if c.building[id] {
		ns := c.source.Schemas[id]
		return nil, &ResolvedRecursiveRef{
			SchemaID:       id,
			SchemaName:     ns.Name,
			RefConstraints: refConstraints,
		}, nil
	}

	rns, err := c.compileNestedSchema(id)
	if err != nil {
		return nil, nil, err
	}
	if rns == nil {
		// compileNestedSchema returns nil only for cycles; but we checked
		// c.building above so this path means the schema does not exist.
		return nil, nil, ErrSchemaNotFound.
			WithMessage(fmt.Sprintf("schema '%s' not found", id)).WithPath(atPath)
	}
	return rns, nil, nil
}

// =============================================================================
// HELPERS
// =============================================================================

// resolveEffectiveType determines the canonical FieldType of a NestedSchema.
// Priority: explicit Type > inferred Object (from Fields) > FieldProperties.Type.
func resolveEffectiveType(ns *NestedSchema) FieldType {
	if ns.Type != 0 {
		return ns.Type
	}
	if len(ns.Fields) > 0 {
		return FieldTypeObject
	}
	return ns.FieldProperties.Type
}

// constraintScope returns the appropriate ConstraintScope for a given basePath.
func constraintScope(basePath string) ConstraintScope {
	if basePath == "" {
		return ConstraintScopeGlobal
	}
	return ConstraintScopeRecursive
}

// buildAbsPath constructs the absolute path and pre-split parts for a field.
func buildAbsPath(basePath, fieldName string) (string, []string) {
	if basePath == "" {
		return fieldName, []string{fieldName}
	}
	path := basePath + "." + fieldName
	baseParts := strings.Split(basePath, ".")
	parts := make([]string, len(baseParts)+1)
	copy(parts, baseParts)
	parts[len(baseParts)] = fieldName
	return path, parts
}

// splitFieldPath splits a dot-separated path string. Returns nil for "".
func splitFieldPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

// resolveAbsFieldPaths converts schema-relative FieldNames to absolute paths.
func resolveAbsFieldPaths(basePath string, baseParts []string, fields []FieldName) ([]string, [][]string) {
	paths := make([]string, len(fields))
	parts := make([][]string, len(fields))
	for i, fn := range fields {
		fParts := splitFieldPath(string(fn))
		if basePath == "" {
			paths[i] = string(fn)
			parts[i] = fParts
		} else {
			paths[i] = basePath + "." + string(fn)
			combined := make([]string, len(baseParts)+len(fParts))
			copy(combined, baseParts)
			copy(combined[len(baseParts):], fParts)
			parts[i] = combined
		}
	}
	return paths, parts
}

// resolveRelFieldPaths returns field names as declared (relative, no prefix).
// Used for recursive-scope constraint evaluation.
func resolveRelFieldPaths(fields []FieldName) ([]string, [][]string) {
	paths := make([]string, len(fields))
	parts := make([][]string, len(fields))
	for i, fn := range fields {
		paths[i] = string(fn)
		parts[i] = splitFieldPath(string(fn))
	}
	return paths, parts
}

// stripPathPrefix converts absolute paths to relative by removing the
// basePath prefix. basePrefixLen is len(splitFieldPath(basePath)).
func stripPathPrefix(basePath string, basePrefixLen int, absPaths []string, absParts [][]string) ([]string, [][]string) {
	rel := make([]string, len(absPaths))
	relParts := make([][]string, len(absParts))
	if basePath == "" {
		copy(rel, absPaths)
		copy(relParts, absParts)
		return rel, relParts
	}
	prefix := basePath + "."
	for i, p := range absPaths {
		if strings.HasPrefix(p, prefix) {
			rel[i] = strings.TrimPrefix(p, prefix)
			relParts[i] = absParts[i][basePrefixLen:]
		} else {
			rel[i] = p
			relParts[i] = absParts[i]
		}
	}
	return rel, relParts
}

// mergeEnumValues adds LiteralValues into a ResolvedEnum's lookup and complex slices.
func mergeEnumValues(enum *ResolvedEnum, values []LiteralValue) {
	for _, v := range values {
		if v.IsZero() || v.IsNull() {
			continue
		}
		val := v.Value()
		if v.IsSimple() {
			enum.Lookup[val] = struct{}{}
		} else {
			enum.Complex = append(enum.Complex, val)
		}
	}
}

// buildResolvedEnum constructs a ResolvedEnum from a LiteralValue slice and
// the enum schema's declared type (used to determine ExpectNumeric).
func buildResolvedEnum(values []LiteralValue, typ FieldType) *ResolvedEnum {
	enum := &ResolvedEnum{
		Lookup: make(map[any]struct{}, len(values)),
		ExpectNumeric: typ == FieldTypeNumber ||
			typ == FieldTypeDecimal ||
			typ == FieldTypeInteger,
	}
	mergeEnumValues(enum, values)
	return enum
}

// allGroupFieldNames recursively collects all FieldNames referenced by rules
// within a ConstraintGroup and its nested groups.
func allGroupFieldNames(g *ConstraintGroup) []FieldName {
	var names []FieldName
	for _, ruleUnion := range g.Rules {
		switch ruleUnion.Kind() {
		case ConstraintKindRule:
			if r, err := ConstraintAs[*ConstraintRule](ruleUnion); err == nil {
				names = append(names, r.Fields...)
			}
		case ConstraintKindGroup:
			if nested, err := ConstraintAs[*ConstraintGroup](ruleUnion); err == nil {
				names = append(names, allGroupFieldNames(nested)...)
			}
		}
	}
	return names
}

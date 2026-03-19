package ir

import (
	"sort"
)

// compile.go provides the two public entry points:
//
//   Parse(src []byte) (*SourceSchema, error)
//     Unmarshals and validates the raw JSON source. Returns an opaque
//     *SourceSchema that can be passed to Compile. Useful for pre-validating
//     source documents and for unit-testing passes independently.
//
//   Compile(src *SourceSchema, predicates PredicateMap) (*Schema, error)
//     Runs Passes 2–11 over the parsed source and returns the immutable IR.
//
// Errors are always returned as CompileErrors (a []CompileError). The compiler
// collects all errors within each pass before stopping, so a single run can
// surface multiple diagnostics.

// SourceSchema is the opaque parsed representation of a source document.
// It is the output of Parse and the input of Compile.
type SourceSchema struct {
	inner *sourceSchema
}

// Parse unmarshals src into the source model and validates required top-level
// fields. Returns a *SourceSchema on success, or CompileErrors on failure.
func Parse(src []byte) (*SourceSchema, error) {
	inner, errs := parseSource(src)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}
	if errs = validateSource(inner); len(errs) > 0 {
		return nil, CompileErrors(errs)
	}
	return &SourceSchema{inner: inner}, nil
}

// Compile runs the full compilation pipeline over a parsed source document and
// returns an immutable *Schema. predicates may be nil or empty if the
// source contains no constraints; a non-empty PredicateMap is required for
// constraint resolution (Pass 11).
//
// Errors from all passes are collected and returned together as CompileErrors.
// The pipeline stops at the first pass that produces errors to avoid cascading
// failures from invalid intermediate state.
func Compile(src *SourceSchema, predicates PredicateMap) (*Schema, error) {
	if src == nil {
		return nil, CompileErrors{{
			Pass:    PassParse,
			Message: "src is nil",
		}}
	}
	if predicates == nil {
		predicates = PredicateMap{}
	}
	s := src.inner

	// ── Pass 2: schema indices ────────────────────────────────────────────────
	si, errs := buildSchemaIndex(s)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// ── Pass 3: field indices ─────────────────────────────────────────────────
	fi, errs := buildFieldIndex(s, si)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// ── Passes 4+5: descriptors + terminal bits ───────────────────────────────
	descriptors, variantRefs, errs := buildDescriptors(s, si, fi)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// Reconstruct the flat entries slice for passes that need it after
	// buildDescriptors (offsets, store, meta). We reuse the same traversal
	// order (schema index, then field index) to produce a parallel slice.
	entries := collectEntries(s, si, fi)

	// ── Pass 6: variants ──────────────────────────────────────────────────────
	variants, errs := buildVariants(variantRefs, si)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// ── Pass 7: schema offsets ────────────────────────────────────────────────
	// objectSchemas is the set of schema indices that have at least one field.
	// Type schemas (enum, union, composite, array/set) are absent from this set.
	totalSchemas := 1 + len(si.order)
	objectSchemas := make(map[uint8]bool, totalSchemas)
	if len(s.Fields) > 0 {
		objectSchemas[0] = true
	}
	for _, uuid := range si.order {
		if len(s.Schemas[uuid].Fields) > 0 {
			objectSchemas[si.byUUID[uuid]] = true
		}
	}
	offsets, errs := buildSchemaOffsets(entries, totalSchemas, objectSchemas)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// ── Pass 8: store ─────────────────────────────────────────────────────────
	store, errs := buildStore(s, si, entries, descriptors)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// ── Pass 9: meta ──────────────────────────────────────────────────────────
	meta, errs := buildMeta(s, si, fi, entries, descriptors)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}

	// Assemble the partial Schema. The address space build (below)
	// reads cs.Descriptors, cs.SchemaOffsets, cs.Variants, and cs.Meta.
	cs := &Schema{
		Descriptors:   descriptors,
		SchemaOffsets: offsets,
		Variants:      variants,
		Store:         store,
		Meta:          meta,
		PathCache:     NewPathRegistry(),
	}

	// ── Build address space ───────────────────────────────────────────────────
	// Must follow Pass 9 (Meta) since it reads SchemaMetadata.Fields to build
	// FieldNames. Must precede Passes 10 and 11 since they call cs.Address().
	addressSpace, errs := buildAddressSpace(cs)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}
	cs.AddressSpace = addressSpace

	// ── Pass 10: resolve indexes ──────────────────────────────────────────────
	resolvedIndexes, errs := buildResolvedIndexes(cs, si)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}
	cs.ResolvedIndexes = resolvedIndexes

	// ── Pass 11: resolve constraints ──────────────────────────────────────────
	resolvedConstraints, errs := buildResolvedConstraints(cs, predicates, s)
	if len(errs) > 0 {
		return nil, CompileErrors(errs)
	}
	cs.ResolvedConstraints = resolvedConstraints

	return cs, nil
}

// collectEntries rebuilds the flat []fieldEntry in the same schema-index /
// field-index order used by buildDescriptors. This is the canonical layout
// order for offsets, store, and meta passes.
//
// Note: entries here carry fd=0 (we only need schemaIdx + fieldUUID for the
// passes that consume this slice alongside the descriptors slice).
func collectEntries(s *sourceSchema, si *schemaIndex, fi *fieldIndex) []fieldEntry {
	var entries []fieldEntry

	// Root schema (index 0).
	for _, fieldUUID := range fi.byOrder[0] {
		entries = append(entries, fieldEntry{schemaIdx: 0, fieldUUID: fieldUUID})
	}

	// Nested schemas in stable order.
	for _, uuid := range si.order {
		nested := s.Schemas[uuid]
		if len(nested.Fields) == 0 {
			continue
		}
		schemaIdx := si.byUUID[uuid]
		for _, fieldUUID := range fi.byOrder[schemaIdx] {
			entries = append(entries, fieldEntry{schemaIdx: schemaIdx, fieldUUID: fieldUUID})
		}
	}

	return entries
}

// sortStrings sorts a []string in place. Used by meta.go and other passes.
// Centralised here so we have a single import of "sort" in compile.go.
func sortStrings(ss []string) {
	sort.Strings(ss)
}

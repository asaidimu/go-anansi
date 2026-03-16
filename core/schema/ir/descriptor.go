package ir

import (
	"encoding/json"
	"sort"
)

// descriptor.go implements Pass 4 (build descriptors pre-terminal) and
// Pass 5 (cycle detection + terminal bit assignment).
//
// Pass 4 packs all bits except terminal (bit 31) for every field in every
// schema. Schema-bearing fields that reference unknown UUIDs are compile errors.
//
// Pass 5 runs a DFS over the schema reference graph. For each schema-bearing
// field, terminal=1 iff none of its targets are on the current DFS path
// (i.e. no cycle is reachable via this specific field). This is path-sensitive:
// the same target schema may be terminal from one field and non-terminal from
// another, depending on the graph above each field.

// fieldEntry is the intermediate representation of one field during Passes 4–5.
type fieldEntry struct {
	schemaIdx    uint8
	fieldUUID    string
	fd           uint32   // packed descriptor; terminal bit is zero until Pass 5
	variantUUIDs []string // non-nil for TypeUnion / TypeComposite fields only
}

// buildDescriptors executes Passes 4 and 5, returning:
//   - descriptors: the global flat []uint32 with terminal bits set.
//   - variantRefs: map from final descriptor value (with terminal bit) to
//     ordered variant UUIDs. Consumed by Pass 6 (variants.go).
func buildDescriptors(
	src *sourceSchema,
	si *schemaIndex,
	fi *fieldIndex,
) (descriptors []uint32, variantRefs map[uint32][]string, errs []CompileError) {

	// ── Pass 4: build pre-terminal descriptors ────────────────────────────────

	var entries []fieldEntry

	// Root schema is always index 0.
	rootEntries, rootErrs := buildSchemaDescriptors(src.Fields, 0, si, fi)
	errs = append(errs, rootErrs...)
	entries = append(entries, rootEntries...)

	// Nested schemas in stable (UUID lex) order.
	for _, uuid := range si.order {
		nested := src.Schemas[uuid]
		schemaIdx := si.byUUID[uuid]

		// Type schemas (enum, union, composite, array/set) have no fields.
		if len(nested.Fields) == 0 {
			continue
		}

		nestedEntries, nestedErrs := buildSchemaDescriptors(nested.Fields, schemaIdx, si, fi)
		for i := range nestedErrs {
			nestedErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, nestedErrs...)
		entries = append(entries, nestedEntries...)
	}

	if len(errs) > 0 {
		return nil, nil, errs
	}

	// Flatten to []uint32. Terminal bits are all zero at this point.
	descriptors = make([]uint32, len(entries))
	for i, e := range entries {
		descriptors[i] = e.fd
	}

	// ── Pass 5: cycle detection + terminal bits ───────────────────────────────

	// Build adjacency: schemaIdx → slice of (descriptors position, target indices)
	// for every schema-bearing field in that schema.
	type fieldTarget struct {
		pos     int
		targets []uint8
	}
	adj := make(map[uint8][]fieldTarget)

	for pos, e := range entries {
		if !IsSchemaBearing(e.fd) {
			continue
		}
		typ := ExtractType(e.fd)
		var targets []uint8
		if typ == TypeUnion || typ == TypeComposite {
			for _, varUUID := range e.variantUUIDs {
				targets = append(targets, si.byUUID[varUUID])
			}
		} else {
			targets = []uint8{ExtractTargetSchema(e.fd)}
		}
		adj[e.schemaIdx] = append(adj[e.schemaIdx], fieldTarget{pos: pos, targets: targets})
	}

	// DFS from root. visiting[idx] is true while schema idx is on the current
	// path — a target found in visiting is a back-edge (cycle).
	visiting := [128]bool{}

	var dfs func(schemaIdx uint8)
	dfs = func(schemaIdx uint8) {
		visiting[schemaIdx] = true
		defer func() { visiting[schemaIdx] = false }()

		for _, ft := range adj[schemaIdx] {
			terminal := true
			for _, target := range ft.targets {
				if visiting[target] {
					terminal = false
					break
				}
			}
			if terminal {
				descriptors[ft.pos] |= FDMaskTerminal
				for _, target := range ft.targets {
					if !visiting[target] {
						dfs(target)
					}
				}
			}
			// Non-terminal: there is a cycle on this path; do not recurse further.
		}
	}

	dfs(0)

	// Scalar (non-schema-bearing) fields always get terminal=1.
	for pos, e := range entries {
		if !IsSchemaBearing(e.fd) {
			descriptors[pos] |= FDMaskTerminal
		}
	}

	// Build variantRefs keyed on the final descriptor values (terminal bit set).
	variantRefs = make(map[uint32][]string)
	for pos, e := range entries {
		if len(e.variantUUIDs) > 0 {
			variantRefs[descriptors[pos]] = e.variantUUIDs
		}
	}

	return descriptors, variantRefs, nil
}

// computeSchemaRanges returns start position and field count in entries for
// each schema index. Used by offsets.go (Pass 7).
func computeSchemaRanges(entries []fieldEntry) (schemaStart map[uint8]int, schemaCount map[uint8]int) {
	schemaCount = make(map[uint8]int)
	for _, e := range entries {
		schemaCount[e.schemaIdx]++
	}

	idxs := make([]int, 0, len(schemaCount))
	for idx := range schemaCount {
		idxs = append(idxs, int(idx))
	}
	sort.Ints(idxs)

	schemaStart = make(map[uint8]int, len(idxs))
	cursor := 0
	for _, idx := range idxs {
		schemaStart[uint8(idx)] = cursor
		cursor += schemaCount[uint8(idx)]
	}
	return schemaStart, schemaCount
}

// buildSchemaDescriptors builds pre-terminal fieldEntry values for all fields
// of one schema, in field_index order.
func buildSchemaDescriptors(
	fields map[string]sourceField,
	schemaIdx uint8,
	si *schemaIndex,
	fi *fieldIndex,
) ([]fieldEntry, []CompileError) {
	orderedUUIDs := fi.byOrder[schemaIdx]
	entries := make([]fieldEntry, 0, len(orderedUUIDs))
	var errs []CompileError

	for _, fieldUUID := range orderedUUIDs {
		f := fields[fieldUUID]
		fieldIdx := fi.bySchemaField[schemaFieldKey{schemaIdx, fieldUUID}]

		ft, errStr := parseFieldType(f.Type)
		if errStr != "" {
			errs = append(errs, CompileError{
				Pass:      PassDescriptor,
				FieldUUID: fieldUUID,
				Message:   errStr,
			})
			continue
		}

		var targetSchema uint8
		var variantUUIDs []string

		if ft.IsSchemaBearing() {
			var resolveErrs []CompileError
			targetSchema, variantUUIDs, resolveErrs = resolveFieldSchema(f, ft, fieldUUID, si)
			errs = append(errs, resolveErrs...)
		}

		fd := packDescriptor(ft, schemaIdx, fieldIdx, targetSchema, f.Required, f.Unique, f.Deprecated)
		entries = append(entries, fieldEntry{
			schemaIdx:    schemaIdx,
			fieldUUID:    fieldUUID,
			fd:           fd,
			variantUUIDs: variantUUIDs,
		})
	}

	return entries, errs
}

// packDescriptor packs all bits except terminal (bit 31) into a uint32.
// The reserved bit 0 is always zero.
func packDescriptor(
	ft FieldTypeEnum,
	ownerSchema uint8,
	fieldIndex uint8,
	targetSchema uint8,
	required, unique, deprecated bool,
) uint32 {
	var fd uint32
	fd |= uint32(ft) << 1
	if required {
		fd |= FDMaskRequired
	}
	if unique {
		fd |= FDMaskUnique
	}
	if deprecated {
		fd |= FDMaskDeprecated
	}
	fd |= uint32(fieldIndex) << 8
	fd |= uint32(ownerSchema) << 15
	fd |= uint32(targetSchema) << 23
	// bit 0 (reserved) and bit 31 (terminal) remain zero
	return fd
}

// resolveFieldSchema resolves the schema reference(s) for a schema-bearing field.
// Returns targetSchema (always 0 for union/composite), variantUUIDs (non-nil for
// union/composite), and any errors.
func resolveFieldSchema(
	f sourceField,
	ft FieldTypeEnum,
	fieldUUID string,
	si *schemaIndex,
) (targetSchema uint8, variantUUIDs []string, errs []CompileError) {

	if ft == TypeUnion || ft == TypeComposite {
		refs, errStr := parseFieldSchemaArray(f.Schema)
		if errStr != "" {
			return 0, nil, []CompileError{{
				Pass:      PassDescriptor,
				FieldUUID: fieldUUID,
				Message:   errStr,
			}}
		}
		for _, ref := range refs {
			if _, ok := si.byUUID[ref.ID]; !ok {
				errs = append(errs, CompileError{
					Pass:      PassDescriptor,
					FieldUUID: fieldUUID,
					Message:   "unresolved schema reference: " + ref.ID,
				})
				continue
			}
			variantUUIDs = append(variantUUIDs, ref.ID)
		}
		// targetSchema stays 0; variants live in the Variants map.
		return 0, variantUUIDs, errs
	}

	// Single FieldSchema ref for all other schema-bearing types.
	ref, errStr := parseFieldSchemaSingle(f.Schema)
	if errStr != "" {
		return 0, nil, []CompileError{{
			Pass:      PassDescriptor,
			FieldUUID: fieldUUID,
			Message:   errStr,
		}}
	}

	if ref.ID != "" {
		idx, ok := si.byUUID[ref.ID]
		if !ok {
			return 0, nil, []CompileError{{
				Pass:      PassDescriptor,
				FieldUUID: fieldUUID,
				Message:   "unresolved schema reference: " + ref.ID,
			}}
		}
		return idx, nil, nil
	}

	// Inline primitive element type (e.g. array of string). targetSchema=0;
	// consumers use the type bits directly.
	return 0, nil, nil
}

// parseFieldType maps a source type string to a FieldTypeEnum.
func parseFieldType(s string) (FieldTypeEnum, string) {
	switch s {
	case "unknown":
		return TypeUnknown, ""
	case "string":
		return TypeString, ""
	case "number":
		return TypeNumber, ""
	case "integer":
		return TypeInteger, ""
	case "decimal":
		return TypeDecimal, ""
	case "boolean":
		return TypeBoolean, ""
	case "bytes":
		return TypeBytes, ""
	case "array":
		return TypeArray, ""
	case "set":
		return TypeSet, ""
	case "enum":
		return TypeEnum, ""
	case "object":
		return TypeObject, ""
	case "record":
		return TypeRecord, ""
	case "union":
		return TypeUnion, ""
	case "composite":
		return TypeComposite, ""
	case "geometry":
		return TypeGeometry, ""
	default:
		return TypeUnknown, "unknown field type: " + s
	}
}

// parseFieldSchemaSingle interprets a raw schema value as a single FieldSchema.
func parseFieldSchemaSingle(schema any) (*sourceFieldRef, string) {
	if schema == nil {
		return &sourceFieldRef{}, ""
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, "failed to marshal schema ref: " + err.Error()
	}
	if len(b) > 0 && b[0] == '[' {
		return nil, "expected single schema ref, got array"
	}
	var ref sourceFieldRef
	if err := json.Unmarshal(b, &ref); err != nil {
		return nil, "failed to unmarshal schema ref: " + err.Error()
	}
	return &ref, ""
}

// parseFieldSchemaArray interprets a raw schema value as a FieldSchemaArray.
func parseFieldSchemaArray(schema any) ([]sourceFieldRef, string) {
	if schema == nil {
		return nil, "union/composite field missing schema array"
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, "failed to marshal schema array: " + err.Error()
	}
	if len(b) > 0 && b[0] != '[' {
		return nil, "expected array of schema refs, got object"
	}
	var refs []sourceFieldRef
	if err := json.Unmarshal(b, &refs); err != nil {
		return nil, "failed to unmarshal schema array: " + err.Error()
	}
	return refs, ""
}

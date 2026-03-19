package ir

// meta.go implements Pass 9: build the cold SchemaMetadata for every schema
// in the document. Metadata is keyed by schema index in Schema.Meta.
// It is loaded only by consumers that require it (code generators, diff tools,
// error reporters) and is never touched by the validator or binary serializer.

// buildMeta constructs the Meta map. entries and descriptors are parallel
// slices from Passes 4 and 5.
func buildMeta(
	src *sourceSchema,
	si *schemaIndex,
	fi *fieldIndex,
	entries []fieldEntry,
	descriptors []uint32,
) (map[uint8]*SchemaMetadata, []CompileError) {
	// Build a lookup: (schemaIdx, fieldUUID) → final descriptor value.
	fdLookup := make(map[schemaFieldKey]uint32, len(entries))
	for pos, e := range entries {
		fdLookup[schemaFieldKey{e.schemaIdx, e.fieldUUID}] = descriptors[pos]
	}

	meta := make(map[uint8]*SchemaMetadata, 1+len(si.order))
	var errs []CompileError

	// Root schema (index 0). The root has no UUID in the source (it is the
	// document itself), so UUID is left empty.
	rootMeta, rootErrs := buildSchemaMeta(
		"",         // uuid
		src.Name,
		src.Version,
		src.Description,
		src.Concrete,
		nil, // values
		src.Metadata,
		src.Fields,
		src.Indexes,
		0,
		fi,
		fdLookup,
	)
	errs = append(errs, rootErrs...)
	meta[0] = rootMeta

	// Nested schemas.
	for _, uuid := range si.order {
		nested := src.Schemas[uuid]
		schemaIdx := si.byUUID[uuid]

		var (
			name        = nested.Name
			version     string // nested schemas do not carry a version field
			description = nested.Description
			concrete    = nested.Concrete
			values      = nested.Values
			metadata    = nested.Metadata
			fields      = nested.Fields
			indexes     = nested.Indexes
		)

		m, mErrs := buildSchemaMeta(
			uuid, name, version, description, concrete, values, metadata,
			fields, indexes,
			schemaIdx, fi, fdLookup,
		)
		for i := range mErrs {
			mErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, mErrs...)

		// ── Resolve type schema target/variants ──────────────────────────────
		if nested.Type != "" {
			ft, _ := parseFieldType(nested.Type)
			m.Type = ft
			if ft.IsSchemaBearing() {
				if ft == TypeUnion || ft == TypeComposite {
					refs, errStr := parseFieldSchemaArray(nested.Schema)
					if errStr != "" {
						errs = append(errs, CompileError{
							Pass:       PassMeta,
							SchemaUUID: uuid,
							Message:    errStr,
						})
					} else {
						for _, ref := range refs {
							if idx, ok := si.byUUID[ref.ID]; ok {
								m.Variants = append(m.Variants, idx)
							} else {
								errs = append(errs, CompileError{
									Pass:       PassMeta,
									SchemaUUID: uuid,
									Message:    "unresolved variant reference: " + ref.ID,
								})
							}
						}
					}
				} else {
					ref, errStr := parseFieldSchemaSingle(nested.Schema)
					if errStr != "" {
						errs = append(errs, CompileError{
							Pass:       PassMeta,
							SchemaUUID: uuid,
							Message:    errStr,
						})
					} else if ref.ID != "" {
						if idx, ok := si.byUUID[ref.ID]; ok {
							m.TargetSchema = idx
						} else {
							errs = append(errs, CompileError{
								Pass:       PassMeta,
								SchemaUUID: uuid,
								Message:    "unresolved schema reference: " + ref.ID,
							})
						}
					}
				}
			}
		}

		meta[schemaIdx] = m
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return meta, nil
}

// buildSchemaMeta builds a single SchemaMetadata for one schema.
func buildSchemaMeta(
	uuid, name, version, description string,
	concrete bool,
	values []any,
	metadata map[string]any,
	fields map[string]sourceField,
	indexes map[string]sourceIndex,
	schemaIdx uint8,
	fi *fieldIndex,
	fdLookup map[schemaFieldKey]uint32,
) (*SchemaMetadata, []CompileError) {
	var errs []CompileError

	// ── Fields map ────────────────────────────────────────────────────────────
	fieldsMeta := make(map[uint32]FieldMeta, len(fields))
	for fieldUUID, f := range fields {
		fd, ok := fdLookup[schemaFieldKey{schemaIdx, fieldUUID}]
		if !ok {
			// Should not happen if Passes 4–5 succeeded.
			errs = append(errs, CompileError{
				Pass:      PassMeta,
				FieldUUID: fieldUUID,
				Message:   "descriptor not found for field — internal compiler error",
			})
			continue
		}
		fieldsMeta[fd] = FieldMeta{
			UUID:        fieldUUID,
			Name:        f.Name,
			Description: f.Description,
		}
	}

	// ── Cold indexes ──────────────────────────────────────────────────────────
	coldIndexes := make(map[string]IndexDescriptor, len(indexes))
	for idxUUID, idx := range indexes {
		cold, idxErrs := buildIndexDescriptor(idx)
		for i := range idxErrs {
			idxErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, idxErrs...)
		coldIndexes[idxUUID] = cold
	}

	m := &SchemaMetadata{
		UUID:          uuid,
		Name:          name,
		Version:       version,
		Description:   description,
		Concrete:      concrete,
		Values:        values,
		Fields:        fieldsMeta,
		Indexes:       coldIndexes,
		IndexOrdinals: make(map[string]uint8),
		Metadata:      metadata,
	}

	return m, errs
}

// buildIndexDescriptor converts a sourceIndex to its cold IndexDescriptor form.
func buildIndexDescriptor(src sourceIndex) (IndexDescriptor, []CompileError) {
	var errs []CompileError

	it, ok := parseIndexType(src.Type)
	if !ok {
		errs = append(errs, CompileError{
			Pass:    PassMeta,
			Message: "unknown index type: " + src.Type,
		})
	}

	io, ok := parseIndexOrder(src.Order)
	if !ok && src.Order != "" {
		errs = append(errs, CompileError{
			Pass:    PassMeta,
			Message: "unknown index order: " + src.Order,
		})
	}

	var condition IndexCondition
	if src.Condition != nil {
		var condErrs []CompileError
		condition, condErrs = buildIndexCondition(src.Condition)
		errs = append(errs, condErrs...)
	}

	return IndexDescriptor{
		Name:        src.Name,
		Description: src.Description,
		Type:        it,
		Order:       io,
		Unique:      src.Unique,
		Fields:      src.Fields,
		Condition:   condition,
	}, errs
}

// buildIndexCondition recursively converts a sourceIndexCondition to an
// IndexCondition (leaf or group).
func buildIndexCondition(src *sourceIndexCondition) (IndexCondition, []CompileError) {
	if src == nil {
		return nil, nil
	}

	// Group: has Conditions slice.
	if len(src.Conditions) > 0 {
		op, ok := parseLogicalOperator(src.Operator)
		if !ok {
			return nil, []CompileError{{
				Pass:    PassMeta,
				Message: "unknown logical operator in index condition: " + src.Operator,
			}}
		}
		group := IndexConditionGroup{Operator: op}
		var errs []CompileError
		for _, child := range src.Conditions {
			c, childErrs := buildIndexCondition(child)
			errs = append(errs, childErrs...)
			group.Conditions = append(group.Conditions, c)
		}
		return group, errs
	}

	// Leaf: has Field + Operator + Value.
	op, ok := parseComparisonOperator(src.Operator)
	if !ok {
		return nil, []CompileError{{
			Pass:    PassMeta,
			Message: "unknown comparison operator in index condition: " + src.Operator,
		}}
	}
	return IndexConditionLeaf{
		Field:    src.Field,
		Operator: op,
		Value:    src.Value,
	}, nil
}

// ── Enum helpers ──────────────────────────────────────────────────────────────

func parseIndexType(s string) (IndexType, bool) {
	switch s {
	case "normal":
		return IndexTypeNormal, true
	case "unique":
		return IndexTypeUnique, true
	case "primary":
		return IndexTypePrimary, true
	case "spatial":
		return IndexTypeSpatial, true
	case "fulltext":
		return IndexTypeFulltext, true
	default:
		return IndexTypeNormal, false
	}
}

func parseIndexOrder(s string) (IndexOrder, bool) {
	switch s {
	case "asc", "":
		return IndexOrderAsc, true
	case "desc":
		return IndexOrderDesc, true
	default:
		return IndexOrderAsc, false
	}
}

func parseLogicalOperator(s string) (LogicalOperator, bool) {
	switch s {
	case "and":
		return LogicalAnd, true
	case "or":
		return LogicalOr, true
	case "not":
		return LogicalNot, true
	case "nor":
		return LogicalNor, true
	case "xor":
		return LogicalXor, true
	case "nand":
		return LogicalNand, true
	case "xnor":
		return LogicalXnor, true
	default:
		return LogicalAnd, false
	}
}

func parseComparisonOperator(s string) (ComparisonOperator, bool) {
	switch s {
	case "eq":
		return ComparisonEq, true
	case "neq":
		return ComparisonNeq, true
	case "lt":
		return ComparisonLt, true
	case "lte":
		return ComparisonLte, true
	case "gt":
		return ComparisonGt, true
	case "gte":
		return ComparisonGte, true
	case "in":
		return ComparisonIn, true
	case "nin":
		return ComparisonNin, true
	case "contains":
		return ComparisonContains, true
	case "ncontains":
		return ComparisonNcontains, true
	case "exists":
		return ComparisonExists, true
	case "nexists":
		return ComparisonNexists, true
	default:
		return ComparisonEq, false
	}
}

// sortedKeys returns the keys of a map[string]T in lexicographic order.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

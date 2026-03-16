package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// store.go implements Pass 8: populate the Store DataContainer with:
//   - Enum value sets (one typed array per enum-typed field, keyed by that
//     field's descriptor-derived DataPoint).
//   - Field defaults (one scalar value per field that carries a default).
//
// Store is nil if the compiled document contains no enum fields and no
// fields with defaults.
//
// Enum value sets are always stored against the *referencing field's*
// descriptor, not the enum schema's index. This means two fields in different
// schemas that both reference the same named enum schema each get their own
// Store entry, keyed by their own descriptor. Consumers look up the value set
// by the field descriptor they already hold — no indirection through the schema
// index is required.

// buildStore constructs the Store DataContainer.
// entries and descriptors are parallel slices — entries[i] and descriptors[i]
// describe the same field.
func buildStore(
	src *sourceSchema,
	si *schemaIndex,
	fi *fieldIndex,
	entries []fieldEntry,
	descriptors []uint32,
) (*document.DataContainer, []CompileError) {
	var store *document.DataContainer
	var errs []CompileError

	ensureStore := func() {
		if store == nil {
			store = document.NewDataContainer()
		}
	}

	// posLookup: (schemaIdx, fieldUUID) → position in descriptors.
	posLookup := make(map[schemaFieldKey]int, len(entries))
	for pos, e := range entries {
		posLookup[schemaFieldKey{e.schemaIdx, e.fieldUUID}] = pos
	}

	// Process root schema fields.
	rootErrs := processStoreFields(src.Fields, 0, src.Schemas, descriptors, posLookup, &store, ensureStore)
	errs = append(errs, rootErrs...)

	// Process nested object schema fields.
	for _, uuid := range si.order {
		nested := src.Schemas[uuid]
		if len(nested.Fields) == 0 {
			continue
		}
		schemaIdx := si.byUUID[uuid]
		nestedErrs := processStoreFields(nested.Fields, schemaIdx, src.Schemas, descriptors, posLookup, &store, ensureStore)
		for i := range nestedErrs {
			nestedErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, nestedErrs...)
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return store, nil
}

// processStoreFields handles Store population for one schema's fields.
// nestedSchemas is the full source schemas map, used to resolve named enum refs.
func processStoreFields(
	fields map[string]sourceField,
	schemaIdx uint8,
	nestedSchemas map[string]sourceNestedSchema,
	descriptors []uint32,
	posLookup map[schemaFieldKey]int,
	store **document.DataContainer,
	ensureStore func(),
) []CompileError {
	var errs []CompileError

	for fieldUUID, f := range fields {
		pos, ok := posLookup[schemaFieldKey{schemaIdx, fieldUUID}]
		if !ok {
			continue
		}
		fd := descriptors[pos]
		ft := ExtractType(fd)

		// ── Enum value sets ───────────────────────────────────────────────────
		if ft == TypeEnum {
			values, errStr := resolveEnumValues(f, nestedSchemas)
			if errStr != "" {
				errs = append(errs, CompileError{
					Pass:      PassStore,
					FieldUUID: fieldUUID,
					Message:   errStr,
				})
			} else if len(values) > 0 {
				ensureStore()
				if errStr := storeEnumValues(fd, values, *store); errStr != "" {
					errs = append(errs, CompileError{
						Pass:      PassStore,
						FieldUUID: fieldUUID,
						Message:   errStr,
					})
				}
			}
		}

		// ── Field defaults ────────────────────────────────────────────────────
		if f.Default != nil {
			ensureStore()
			if errStr := storeDefault(fd, ft, f.Default, *store); errStr != "" {
				errs = append(errs, CompileError{
					Pass:      PassStore,
					FieldUUID: fieldUUID,
					Message:   errStr,
				})
			}
		}
	}

	return errs
}

// resolveEnumValues returns the value set for an enum-typed field.
// It handles both inline enum refs (values embedded in the field's schema ref)
// and named enum refs (values in the referenced NestedSchemaType).
func resolveEnumValues(f sourceField, nestedSchemas map[string]sourceNestedSchema) ([]any, string) {
	if f.Schema == nil {
		return nil, ""
	}

	ref, errStr := parseFieldSchemaSingle(f.Schema)
	if errStr != "" {
		return nil, errStr
	}

	// Inline enum: values are embedded directly in the field's schema ref.
	if len(ref.Values) > 0 {
		return ref.Values, ""
	}

	// Named enum ref: values live in the referenced nested schema.
	if ref.ID != "" {
		nested, ok := nestedSchemas[ref.ID]
		if !ok {
			// The UUID was already validated in Pass 4; absence here is a bug.
			return nil, "named enum schema not found: " + ref.ID + " — internal compiler error"
		}
		return nested.Values, ""
	}

	return nil, ""
}

// storeEnumValues writes an enum field's value set into the store as a typed
// array, keyed by the DataPoint derived from the field's descriptor.
func storeEnumValues(fd uint32, values []any, store *document.DataContainer) string {
	if len(values) == 0 {
		return ""
	}

	elemType := inferEnumElemType(values[0])
	arrayType := enumElemTypeToArrayDataType(elemType)
	dp := descriptorToEnumDataPoint(fd, arrayType)

	switch arrayType {
	case document.TypeArrayString:
		ss := make([]string, 0, len(values))
		for _, v := range values {
			s, ok := v.(string)
			if !ok {
				return "enum value is not a string"
			}
			ss = append(ss, s)
		}
		if err := store.AppendArrayString(dp, ss); err != nil {
			return "store: enum string values: " + err.Error()
		}

	case document.TypeArrayInt:
		is := make([]int64, 0, len(values))
		for _, v := range values {
			n, ok := toInt64(v)
			if !ok {
				return "enum value is not an integer"
			}
			is = append(is, n)
		}
		if err := store.AppendArrayInt(dp, is); err != nil {
			return "store: enum integer values: " + err.Error()
		}

	case document.TypeArrayFloat:
		fs := make([]float64, 0, len(values))
		for _, v := range values {
			f, ok := toFloat64(v)
			if !ok {
				return "enum value is not a number"
			}
			fs = append(fs, f)
		}
		if err := store.AppendArrayFloat(dp, fs); err != nil {
			return "store: enum float values: " + err.Error()
		}

	case document.TypeArrayBool:
		bs := make([]bool, 0, len(values))
		for _, v := range values {
			b, ok := v.(bool)
			if !ok {
				return "enum value is not a boolean"
			}
			bs = append(bs, b)
		}
		if err := store.AppendArrayBool(dp, bs); err != nil {
			return "store: enum bool values: " + err.Error()
		}

	default:
		if err := store.AppendArrayUnknown(dp, values); err != nil {
			return "store: enum unknown values: " + err.Error()
		}
	}

	return ""
}

// storeDefault writes a single field default into the store.
func storeDefault(fd uint32, ft FieldTypeEnum, value any, store *document.DataContainer) string {
	dt := fieldTypeToDataType(ft)
	id := int32((fd >> 8) & 0x7FFF)
	dp, err := document.NewDataPoint(dt, id)
	if err != nil {
		return "store: default DataPoint: " + err.Error()
	}

	switch dt {
	case document.TypeString:
		s, ok := value.(string)
		if !ok {
			return "default value is not a string"
		}
		if err := store.AppendString(dp, s); err != nil {
			return "store: default string: " + err.Error()
		}
	case document.TypeInt:
		n, ok := toInt64(value)
		if !ok {
			return "default value is not an integer"
		}
		if err := store.AppendInt(dp, n); err != nil {
			return "store: default int: " + err.Error()
		}
	case document.TypeFloat:
		f, ok := toFloat64(value)
		if !ok {
			return "default value is not a number"
		}
		if err := store.AppendFloat(dp, f); err != nil {
			return "store: default float: " + err.Error()
		}
	case document.TypeBool:
		b, ok := value.(bool)
		if !ok {
			return "default value is not a boolean"
		}
		if err := store.AppendBool(dp, b); err != nil {
			return "store: default bool: " + err.Error()
		}
	default:
		unknownDp, err := document.NewDataPoint(document.TypeUnknown, id)
		if err != nil {
			return "store: default unknown DataPoint: " + err.Error()
		}
		if err := store.AppendUnknown(unknownDp, value); err != nil {
			return "store: default unknown: " + err.Error()
		}
	}

	return ""
}

// descriptorToEnumDataPoint creates a DataPoint for an enum value set using
// the array DataType appropriate for the enum's element type.
func descriptorToEnumDataPoint(fd uint32, arrayType document.DataType) document.DataPoint {
	id := int32((fd >> 8) & 0x7FFF)
	dp, _ := document.NewDataPoint(arrayType, id)
	return dp
}

// inferEnumElemType infers a FieldTypeEnum from a single sample enum value.
func inferEnumElemType(v any) FieldTypeEnum {
	switch v.(type) {
	case string:
		return TypeString
	case bool:
		return TypeBoolean
	case float64:
		f := v.(float64)
		if f == float64(int64(f)) {
			return TypeInteger
		}
		return TypeNumber
	case int64:
		return TypeInteger
	default:
		return TypeUnknown
	}
}

// toInt64 converts a JSON-unmarshalled numeric value to int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	}
	return 0, false
}

// toFloat64 converts a JSON-unmarshalled numeric value to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

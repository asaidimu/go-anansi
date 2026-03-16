package ir

import "sort"

// field_index.go implements Pass 3: for every schema (root and every nested
// object schema), sort field UUIDs lexicographically and assign zero-based
// field_index values (0–126). Compilation fails if any schema exceeds 127 fields.

const (
	maxFieldsPerSchema = 127 // field_index is 7 bits (0–126); value 127 is reserved
)

// fieldIndex holds the results of Pass 3.
type fieldIndex struct {
	// byKey maps a composite key (schemaIdx<<8 | fieldIdx) to the source field UUID.
	// The primary lookup direction used during descriptor building is the reverse:
	// given a schema UUID and field UUID, what is the field_index?
	// That is stored in bySchemaField.
	//
	// byOrder maps schemaIdx → sorted field UUIDs, giving the canonical order for
	// descriptor emission.
	byOrder        map[uint8][]string // schemaIdx → field UUIDs in lex order
	bySchemaField  map[schemaFieldKey]uint8 // (schemaIdx, fieldUUID) → field_index
}

// schemaFieldKey is a composite lookup key: the schema index and the field UUID.
type schemaFieldKey struct {
	schemaIdx uint8
	fieldUUID string
}

// buildFieldIndex assigns field indices for the root schema and all nested
// object schemas (schemas that have a Fields map).
func buildFieldIndex(src *sourceSchema, si *schemaIndex) (*fieldIndex, []CompileError) {
	fi := &fieldIndex{
		byOrder:       make(map[uint8][]string),
		bySchemaField: make(map[schemaFieldKey]uint8),
	}

	var errs []CompileError

	// Root schema is always index 0.
	if rootErrs := assignFieldIndices(fi, 0, src.Fields); len(rootErrs) > 0 {
		errs = append(errs, rootErrs...)
	}

	// Nested object schemas only — type schemas (enum, union, composite, array)
	// do not have fields of their own.
	for _, uuid := range si.order {
		nested := src.Schemas[uuid]
		if len(nested.Fields) == 0 {
			continue
		}
		schemaIdx := si.byUUID[uuid]
		if nestedErrs := assignFieldIndices(fi, schemaIdx, nested.Fields); len(nestedErrs) > 0 {
			for _, e := range nestedErrs {
				e.SchemaUUID = uuid
				errs = append(errs, e)
			}
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return fi, nil
}

// assignFieldIndices sorts the field UUIDs for one schema and records their
// indices into fi. Returns errors if the field count exceeds the hard limit.
func assignFieldIndices(fi *fieldIndex, schemaIdx uint8, fields map[string]sourceField) []CompileError {
	uuids := make([]string, 0, len(fields))
	for uuid := range fields {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)

	if len(uuids) > maxFieldsPerSchema {
		return []CompileError{{
			Pass: PassFieldIndex,
			Message: "schema exceeds maximum field count: " +
				itoa(len(uuids)) + " > " + itoa(maxFieldsPerSchema),
		}}
	}

	fi.byOrder[schemaIdx] = uuids
	for i, uuid := range uuids {
		fi.bySchemaField[schemaFieldKey{schemaIdx, uuid}] = uint8(i)
	}
	return nil
}

package ir

import "sort"

// schema_index.go implements Pass 2: assign a stable uint8 index to every
// schema in the document. The root schema is always index 0. Nested schemas
// are assigned indices 1–127 in UUID lexicographic order. Compilation fails
// if the document contains more than 127 nested schemas (exceeding the 8-bit
// target_schema field which reserves index 0 for the root).

const (
	maxNestedSchemas = 127 // target_schema is 8 bits; root = 0, nested = 1–127
)

// schemaIndex holds the results of Pass 2.
type schemaIndex struct {
	// byUUID maps a schema UUID to its compiler-assigned uint8 index.
	byUUID map[string]uint8
	// order is the slice of nested schema UUIDs in the order indices were
	// assigned (UUID lexicographic). Does not include the root schema.
	order []string
}

// buildSchemaIndex assigns indices to all nested schemas in src.Schemas.
// The root schema (the document itself) is implicitly index 0 and is not
// represented in src.Schemas — no UUID is recorded for it here.
func buildSchemaIndex(src *sourceSchema) (*schemaIndex, []CompileError) {
	uuids := make([]string, 0, len(src.Schemas))
	for uuid := range src.Schemas {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)

	if len(uuids) > maxNestedSchemas {
		return nil, []CompileError{{
			Pass: PassSchemaIndex,
			Message: "document exceeds maximum nested schema count: " +
				itoa(len(uuids)) + " > " + itoa(maxNestedSchemas),
		}}
	}

	idx := &schemaIndex{
		byUUID: make(map[string]uint8, len(uuids)+1),
		order:  uuids,
	}
	// Root schema is always 0. Nested schemas start at 1.
	for i, uuid := range uuids {
		idx.byUUID[uuid] = uint8(i + 1)
	}
	return idx, nil
}

// itoa converts a non-negative int to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

package ir

// offsets.go implements Pass 7: build SchemaOffsets from the global descriptor
// layout established in Pass 4.
//
// SchemaOffsets[N] packs the start (bits 15–0) and end (bits 31–16) of
// schema N's descriptor range in the Descriptors slice. Both values are
// uint16.
//
// Object schemas (those with fields) satisfy start < end.
// Type schemas (enum, union, composite, array/set) have no fields and no
// descriptor entries — their offset entry is the zero value (start == end == 0),
// which consumers must not dereference. Consumers distinguish them via the
// FieldTypeEnum of whatever field points to them.

// buildSchemaOffsets produces the SchemaOffsets slice for CompiledSchema.
//
// totalSchemas is 1 + len(nested schemas), determining the slice length.
// objectSchemas is the set of schema indices that have at least one field —
// used to distinguish object schemas from type schemas when validating.
func buildSchemaOffsets(
	entries []fieldEntry,
	totalSchemas int,
	objectSchemas map[uint8]bool,
) ([]uint32, []CompileError) {
	schemaStart, schemaCount := computeSchemaRanges(entries)

	offsets := make([]uint32, totalSchemas)
	var errs []CompileError

	for idx := 0; idx < totalSchemas; idx++ {
		count, ok := schemaCount[uint8(idx)]

		if !ok || count == 0 {
			// Type schemas legitimately have no fields. Object schemas must not.
			if objectSchemas[uint8(idx)] {
				errs = append(errs, CompileError{
					Pass:    PassOffsets,
					Message: "object schema index " + itoa(idx) + " has no fields",
				})
			}
			// Leave offsets[idx] = 0 (zero sentinel for type schemas).
			continue
		}

		start := schemaStart[uint8(idx)]
		end := start + count

		if start > 0xFFFF || end > 0xFFFF {
			errs = append(errs, CompileError{
				Pass:    PassOffsets,
				Message: "descriptor range for schema " + itoa(idx) + " exceeds uint16 bounds",
			})
			continue
		}

		offsets[idx] = uint32(start) | uint32(end)<<16
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return offsets, nil
}

package ir

// variants.go implements Pass 6: build the Variants map on Schema.
//
// variantRefs (produced by Pass 5) maps each union/composite descriptor value
// to the ordered slice of variant schema UUIDs. Pass 6 translates UUIDs to
// their uint8 schema indices using the schemaIndex from Pass 2.

// buildVariants converts variantRefs (UUID strings) into the Variants map
// (uint8 schema indices) keyed on the final descriptor value.
//
// Returns a CompileError for any variant UUID that cannot be resolved — which
// should not occur if Pass 4 ran cleanly, but is checked defensively.
func buildVariants(
	variantRefs map[uint32][]string,
	si *schemaIndex,
) (map[uint32][]uint8, []CompileError) {
	if len(variantRefs) == 0 {
		return nil, nil
	}

	variants := make(map[uint32][]uint8, len(variantRefs))
	var errs []CompileError

	for fd, uuids := range variantRefs {
		indices := make([]uint8, 0, len(uuids))
		for _, uuid := range uuids {
			idx, ok := si.byUUID[uuid]
			if !ok {
				errs = append(errs, CompileError{
					Pass:    PassVariants,
					Message: "variant references unknown schema UUID: " + uuid,
				})
				continue
			}
			indices = append(indices, idx)
		}
		variants[fd] = indices
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return variants, nil
}

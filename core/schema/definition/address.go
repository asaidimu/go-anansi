package definition

// =============================================================================
// ADDRESS SPACE
// =============================================================================
//
// Every terminal field reachable via any valid path in the schema graph is
// assigned a unique 27-bit integer address. Non-terminal fields (object/array/
// complex containers) are structural — they own sub-blocks but do not receive
// addresses themselves. Only leaf values are addressable.
//
// Single-step paths (root-level fields) occupy [0, 2^14).
// Multi-step paths occupy [2^14, 2^27).
//
// The address space uses a footprint-based allocation:
//
//   At link time, each schema's Footprint is computed bottom-up:
//     Footprint(s) = T(s) + Σ Footprint(child_i)
//   where T(s) is the number of terminal fields in schema s.
//
//   At address time, the multi-step region is divided into sub-blocks sized
//   exactly to each child's Footprint. Within a schema's block, terminals and
//   non-terminal sub-blocks are interleaved in field order:
//
//     [f0_term | f1_child_block | f2_term | f3_child_block | ...]
//
//   Since each sub-block is sized to its subtree's exact needs, the scheme
//   is collision-free: a terminal inside child n's block can never alias a
//   terminal inside child m's block because the blocks are disjoint by
//   construction.

func Address(cs *CompiledSchema, path ResolvedPath) uint32 {
	if len(path) == 0 {
		return 0
	}

	// --- Single-step: terminal field's slot position ---
	if len(path) == 1 {
		step := path[0]
		slot := &cs.Schemas[step.SchemaIdx()]
		fd := cs.Descriptors[int(slot.FieldStart)+int(step.FieldIdx())]
		if !fd.Terminal() {
			return 0
		}
		return uint32(slot.FieldStart) + uint32(step.FieldIdx())
	}

	// --- Multi-step: traverse path accumulating slot positions ---
	base := uint32(MultiStepBase)

	for i := 0; i < len(path); i++ {
		step := path[i]
		slot := &cs.Schemas[step.SchemaIdx()]
		abs := int(slot.FieldStart) + int(step.FieldIdx())
		fd := cs.Descriptors[abs]

		// Skip fields before this one in the same schema: terminals get
		// one slot; non-terminals consume their child's Footprint.
		for j := int(slot.FieldStart); j < abs; j++ {
			f := cs.Descriptors[j]
			if f.Terminal() {
				base++
			} else if f.ChildSchemaIdx() != FdNoChild {
				base += cs.Schemas[f.ChildSchemaIdx()].Footprint
			}
		}

		if i == len(path)-1 {
			// Final step: the target field itself.
			if !fd.Terminal() {
				return 0
			}
			return base
		}

		// Intermediate step: must be non-terminal so we can descend.
		if fd.Terminal() {
			return 0
		}
	}

	return base
}

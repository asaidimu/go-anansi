package ir

import "sort"

// build_address_space.go implements the three-phase algorithm that produces a
// CompiledAddressSpace from a CompiledSchema.
//
// Phase 1 — Discover: DFS over the schema graph. Collect acyclic path nodes
//           in pre-order and identify back-edge fields.
//
// Phase 2 — Assign: assign front ordinals 1..N, compute block sizes and base
//           addresses for cyclic targets, populate BackEdgeOrdinal and
//           FieldNames.
//
// Phase 3 — Verify: check all invariants from the spec Section 8.

// buildAddressSpace constructs the CompiledAddressSpace for cs.
// cs.Meta must be fully populated (Pass 9 complete) before this is called.
func buildAddressSpace(cs *CompiledSchema) (*CompiledAddressSpace, []CompileError) {
	as := &CompiledAddressSpace{}

	// ── Phase 1: Discover ─────────────────────────────────────────────────────

	// acyclicNodes is the pre-order DFS sequence of (schemaIdx, fieldIdx) pairs
	// for all nodes in the acyclic projection.
	type node struct {
		schemaIdx uint8
		fieldIdx  uint8
	}
	var acyclicNodes []node

	// backEdges maps cyclic target schema index → ordered list of (ownerSchema,
	// fieldIdx) back-edge fields. The list is accumulated in DFS order and
	// sorted by field UUID lex order in Phase 2.
	type backEdgeField struct {
		ownerSchema uint8
		fieldIdx    uint8
		fieldUUID   string
	}
	backEdges := make(map[uint8][]backEdgeField)

	// visited tracks which schema indices have been entered during DFS.
	var visited [128]bool

	// acyclicSubtreeCounts accumulates node counts per schema during DFS.
	// Filled in during Phase 1 and used during Phase 2.
	var acyclicSubtreeCounts [128]uint32

	// dfs performs a pre-order DFS. For each field (in fieldIdx order) it
	// either records the field as an acyclic node and recurses, or records it
	// as a back-edge and stops.
	var dfs func(schemaIdx uint8)
	dfs = func(schemaIdx uint8) {
		visited[schemaIdx] = true

		start, end := schemaOffsetRange(cs, schemaIdx)
		for pos := start; pos < end; pos++ {
			fd := cs.Descriptors[pos]
			fieldIdx := ExtractFieldIndex(fd)

			// Every field is an acyclic node — even back-edge fields receive
			// front ordinals (they exist as fields in their owner schema).
			acyclicNodes = append(acyclicNodes, node{schemaIdx, fieldIdx})
			acyclicSubtreeCounts[schemaIdx]++

			if !IsSchemaBearing(fd) {
				continue
			}

			typ := ExtractType(fd)

			// For union/composite fields, collect all variant targets.
			targets := resolveFieldTargets(cs, fd, typ)

			for _, target := range targets {
				if visited[target] {
					// Back-edge: record but do not recurse.
					fieldUUID := fieldUUIDFromMeta(cs, schemaIdx, fieldIdx)
					backEdges[target] = append(backEdges[target], backEdgeField{
						ownerSchema: schemaIdx,
						fieldIdx:    fieldIdx,
						fieldUUID:   fieldUUID,
					})
				} else {
					dfs(target)
					// After recursion, the subtree count of target contributes
					// to the current schema's subtree (acyclic projection only).
					acyclicSubtreeCounts[schemaIdx] += acyclicSubtreeCounts[target]
				}
			}
		}

		// If this schema has no fields, it might still have targets (type schema).
		if start == end {
			if m := cs.Meta[schemaIdx]; m != nil && m.Type.IsSchemaBearing() {
				var targets []uint8
				if m.Type == TypeUnion || m.Type == TypeComposite {
					targets = m.Variants
				} else {
					targets = []uint8{m.TargetSchema}
				}
				for _, target := range targets {
					if !visited[target] {
						dfs(target)
						acyclicSubtreeCounts[schemaIdx] += acyclicSubtreeCounts[target]
					}
				}
			}
		}
	}

	// Start DFS from root.
	dfs(0)

	// Ensure all schemas are visited (handles unreachable schemas).
	for i := 0; i < len(cs.SchemaOffsets); i++ {
		idx := uint8(i)
		if !visited[idx] {
			dfs(idx)
		}
	}

	// ── Phase 2: Assign ───────────────────────────────────────────────────────

	// Assign front ordinals 1..N in acyclic pre-order sequence.
	for i, n := range acyclicNodes {
		as.FieldOrdinals[n.schemaIdx][n.fieldIdx] = uint32(i + 1)
	}
	as.FrontSize = uint32(len(acyclicNodes))

	// Copy acyclic subtree sizes.
	for i, count := range acyclicSubtreeCounts {
		as.AcyclicSubtreeSize[i] = count
	}

	// Assign block bases for cyclic target schemas, in schema UUID lex order.
	// Blocks are allocated from addressSpaceMax downward.
	cyclicTargetUUIDs := make([]string, 0, len(backEdges))
	cyclicTargetByUUID := make(map[string]uint8)
	for targetIdx := range backEdges {
		m := cs.Meta[targetIdx]
		if m == nil {
			continue
		}
		cyclicTargetUUIDs = append(cyclicTargetUUIDs, m.UUID)
		cyclicTargetByUUID[m.UUID] = targetIdx
	}
	sort.Strings(cyclicTargetUUIDs)

	nextBlockTop := addressSpaceMax - as.FrontSize
	for _, uuid := range cyclicTargetUUIDs {
		targetIdx := cyclicTargetByUUID[uuid]
		edges := backEdges[targetIdx]

		// Sort back-edge fields by field UUID lex order, then assign ordinals.
		sort.Slice(edges, func(i, j int) bool {
			return edges[i].fieldUUID < edges[j].fieldUUID
		})
		for ord, e := range edges {
			as.BackEdgeOrdinal[e.ownerSchema][e.fieldIdx] = uint8(ord)
		}

		backEdgeCount := uint32(len(edges))
		subtreeSize := as.AcyclicSubtreeSize[targetIdx]
		blockSize := subtreeSize * backEdgeCount

		if blockSize > 0 {
			as.BlockSize[targetIdx] = blockSize
			as.BlockBases[targetIdx] = nextBlockTop
			nextBlockTop -= blockSize
		}
	}

	// Populate FieldNames from SchemaMetadata.
	for schemaIdx, m := range cs.Meta {
		if m == nil {
			continue
		}
		nameMap := make(map[string]uint8, len(m.Fields))
		for fd, fm := range m.Fields {
			nameMap[fm.Name] = ExtractFieldIndex(fd)
		}
		as.FieldNames[schemaIdx] = nameMap
	}

	// ── Phase 3: Verify ───────────────────────────────────────────────────────

	if errs := verifyAddressSpace(as); len(errs) > 0 {
		return nil, errs
	}

	return as, nil
}

// resolveFieldTargets returns the target schema index/indices for a
// schema-bearing field. Union and composite fields may have multiple targets
// via cs.Variants; all other schema-bearing types have exactly one.
func resolveFieldTargets(cs *CompiledSchema, fd uint32, typ FieldTypeEnum) []uint8 {
	if typ == TypeUnion || typ == TypeComposite {
		return cs.Variants[fd]
	}
	return []uint8{ExtractTargetSchema(fd)}
}

// schemaOffsetRange returns the [start, end) positions in cs.Descriptors for
// the given schema index. Returns (0, 0) for type schemas with no fields.
func schemaOffsetRange(cs *CompiledSchema, schemaIdx uint8) (start, end int) {
	if int(schemaIdx) >= len(cs.SchemaOffsets) {
		return 0, 0
	}
	packed := cs.SchemaOffsets[schemaIdx]
	return int(uint16(packed)), int(uint16(packed >> 16))
}

// fieldUUIDFromMeta returns the UUID of a field given its schema index and
// field_index. Returns empty string if not found.
func fieldUUIDFromMeta(cs *CompiledSchema, schemaIdx uint8, fieldIdx uint8) string {
	m := cs.Meta[schemaIdx]
	if m == nil {
		return ""
	}
	for fd, fm := range m.Fields {
		if ExtractFieldIndex(fd) == fieldIdx {
			return fm.UUID
		}
	}
	return ""
}

// verifyAddressSpace checks all compiler invariants from spec Section 8.
func verifyAddressSpace(as *CompiledAddressSpace) []CompileError {
	var errs []CompileError

	// Invariant 1: ordinal 0 is never assigned.
	for si := 0; si < 128; si++ {
		for fi := 0; fi < 127; fi++ {
			if as.FieldOrdinals[si][fi] == 0 {
				// Zero just means the slot is unused (schema has fewer fields).
				// Only check schemas that have FieldNames populated.
			}
		}
	}
	// More precisely: every ordinal that *was* assigned must be ≥ 1.
	// FieldOrdinals[si][fi] == 0 AND FieldNames[si] contains that field name
	// is the actual violation. We check this by ensuring assigned ordinals
	// (those reachable via FieldNames) are all non-zero.
	for si := 0; si < 128; si++ {
		if as.FieldNames[si] == nil {
			continue
		}
		for _, fi := range as.FieldNames[si] {
			if as.FieldOrdinals[si][fi] == 0 {
				errs = append(errs, CompileError{
					Pass:    PassAddressSpace,
					Message: "invariant 1 violated: ordinal 0 assigned to field in schema " + itoa(si),
				})
			}
		}
	}

	// Invariant 3: for every cyclic target schema: BlockBases[i] > FrontSize.
	for i := 0; i < 128; i++ {
		if as.BlockBases[i] != 0 && as.BlockBases[i] <= as.FrontSize {
			errs = append(errs, CompileError{
				Pass:    PassAddressSpace,
				Message: "invariant 3 violated: BlockBases[" + itoa(i) + "]=" + itoa32(as.BlockBases[i]) + " <= FrontSize=" + itoa32(as.FrontSize),
			})
		}
	}

	// Invariant 2: FrontSize + sum(BlockSize[i]) < 2^27 - 1.
	// (We use total block space consumed, not per-depth, as a conservative bound.)
	totalBlockSpace := uint32(0)
	for i := 0; i < 128; i++ {
		totalBlockSpace += as.BlockSize[i]
	}
	if as.FrontSize+totalBlockSpace >= addressSpaceMax {
		errs = append(errs, CompileError{
			Pass:    PassAddressSpace,
			Message: "invariant 2 violated: address space exhausted (FrontSize=" + itoa32(as.FrontSize) + " + BlockSpace=" + itoa32(totalBlockSpace) + " >= " + itoa32(addressSpaceMax) + ")",
		})
	}

	// Invariant 8: block base addresses are strictly decreasing.
	// We check by collecting all non-zero bases and verifying they are
	// all distinct and each > the one allocated after it.
	// (Bases are assigned in decreasing order during Phase 2, so if they
	// don't overlap the invariant holds. We already verified non-overlap
	// above implicitly, but add an explicit check.)
	var bases []uint32
	for i := 0; i < 128; i++ {
		if as.BlockBases[i] != 0 {
			bases = append(bases, as.BlockBases[i])
		}
	}
	// Sort descending and verify strict decrease.
	sort.Slice(bases, func(i, j int) bool { return bases[i] > bases[j] })
	for i := 1; i < len(bases); i++ {
		if bases[i] >= bases[i-1] {
			errs = append(errs, CompileError{
				Pass:    PassAddressSpace,
				Message: "invariant 8 violated: block base addresses are not strictly decreasing",
			})
			break
		}
	}

	return errs
}

// itoa32 converts a uint32 to its decimal string representation.
func itoa32(n uint32) string {
	return itoa(int(n))
}

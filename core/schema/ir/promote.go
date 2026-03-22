package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// promote.go implements Schema.Promote, which takes a sub-schema index and
// returns a fully compiled standalone *Schema rooted at that sub-schema.
//
// The promoted schema is equivalent to compiling the sub-schema as a root
// document: it has its own AddressSpace, PathCache, SchemaOffsets, Meta, and
// Variants. Its Descriptors are rewritten so that ownerSchema bits reflect the
// new index space. Store is rebuilt with remapped keys. ResolvedConstraints
// and ResolvedIndexes are nil — constraints and indexes are only meaningful at
// the document root.
//
// Promote is the mechanism by which the validator obtains schemas for nested
// documents in TypeArray, TypeRecord (with schema), and TypeUnion/TypeComposite
// (scalar-or-object variant). Each promoted schema has its own PathCache
// populated during its own address space build, giving callers key resolution
// and human-readable path lookup without touching the parent schema.

// Promote returns a fully compiled *Schema rooted at subSchemaIdx.
// It returns an error if subSchemaIdx is out of range or has no fields
// (type schemas — enum, union, composite, array — cannot be promoted as roots
// because they have no fields of their own; only object schemas can).
func (cs *Schema) Promote(subSchemaIdx uint8) (*Schema, error) {
	if int(subSchemaIdx) >= len(cs.SchemaOffsets) {
		return nil, CompileErrors{{
			Pass:    PassMeta,
			Message: "Promote: schema index " + itoa(int(subSchemaIdx)) + " out of range",
		}}
	}

	start, end := schemaOffsetRange(cs, subSchemaIdx)
	if start == end {
		return nil, CompileErrors{{
			Pass:    PassMeta,
			Message: "Promote: schema index " + itoa(int(subSchemaIdx)) + " is a type schema with no fields and cannot be promoted",
		}}
	}

	// ── Step 1: Discover all schema indices reachable from subSchemaIdx ───────
	//
	// We need the full reachable set so we can assign new contiguous indices
	// and remap all cross-references.
	reachable := collectReachable(cs, subSchemaIdx)

	// Assign new indices: subSchemaIdx → 0, others in stable order.
	// We use a BFS-stable order (same as collectReachable) to keep the mapping
	// deterministic.
	oldToNew := make(map[uint8]uint8, len(reachable))
	newToOld := make([]uint8, len(reachable))
	var nextNew uint8
	// subSchemaIdx must be index 0 in the promoted schema.
	oldToNew[subSchemaIdx] = 0
	newToOld[0] = subSchemaIdx
	nextNew = 1
	for _, old := range reachable {
		if old == subSchemaIdx {
			continue
		}
		oldToNew[old] = nextNew
		newToOld[nextNew] = old
		nextNew++
	}
	totalSchemas := int(nextNew)

	// ── Step 2: Build promoted Descriptors with rewritten ownerSchema bits ────

	// Collect all descriptors for all reachable schemas, grouped by new index.
	// We preserve field-index order within each schema (descriptors are already
	// stored in field-index order in the parent).
	type schemaDescs struct {
		newIdx uint8
		descs  []uint32
	}
	promoted := make([]schemaDescs, totalSchemas)
	for newIdx := 0; newIdx < totalSchemas; newIdx++ {
		oldIdx := newToOld[newIdx]
		s, e := schemaOffsetRange(cs, oldIdx)
		descs := make([]uint32, e-s)
		for i, fd := range cs.Descriptors[s:e] {
			descs[i] = rewriteOwnerSchema(fd, oldToNew)
		}
		promoted[newIdx] = schemaDescs{newIdx: uint8(newIdx), descs: descs}
	}

	// Flatten into a single Descriptors slice and build SchemaOffsets.
	var allDescs []uint32
	offsets := make([]uint32, totalSchemas)
	for newIdx := 0; newIdx < totalSchemas; newIdx++ {
		start := len(allDescs)
		allDescs = append(allDescs, promoted[newIdx].descs...)
		end := len(allDescs)
		if start > 0xFFFF || end > 0xFFFF {
			return nil, CompileErrors{{
				Pass:    PassOffsets,
				Message: "Promote: descriptor range exceeds uint16 bounds",
			}}
		}
		offsets[newIdx] = uint32(start) | uint32(end)<<16
	}

	// ── Step 3: Build promoted Variants with remapped descriptor keys ─────────
	//
	// Variant keys are descriptor values. After rewriting ownerSchema bits the
	// descriptor values change, so we must re-key the Variants map.
	// We also remap the variant indices themselves.
	var promotedVariants map[uint32][]uint8
	if len(cs.Variants) > 0 {
		promotedVariants = make(map[uint32][]uint8)
		for _, sd := range promoted {
			for _, fd := range sd.descs {
				// Only union/composite fields have Variants entries.
				if !IsSchemaBearing(fd) {
					continue
				}
				typ := ExtractType(fd)
				if typ != TypeUnion && typ != TypeComposite {
					continue
				}
				// The original descriptor for this field — before ownerSchema rewrite.
				// We need to find the original fd to look up cs.Variants.
				origFD := findOriginalDescriptor(cs, fd, oldToNew)
				if origFD == 0 {
					continue
				}
				oldVariants, ok := cs.Variants[origFD]
				if !ok {
					continue
				}
				newVariants := make([]uint8, 0, len(oldVariants))
				for _, oldV := range oldVariants {
					if newV, ok := oldToNew[oldV]; ok {
						newVariants = append(newVariants, newV)
					}
				}
				promotedVariants[fd] = newVariants
			}
		}
	}

	// ── Step 4: Build promoted Meta ───────────────────────────────────────────
	//
	// Remap schema indices in Meta entries. Descriptor keys in Fields maps
	// change because ownerSchema bits are rewritten.
	promotedMeta := make(map[uint8]*SchemaMetadata, totalSchemas)
	for newIdx := 0; newIdx < totalSchemas; newIdx++ {
		oldIdx := newToOld[newIdx]
		oldM := cs.Meta[oldIdx]
		if oldM == nil {
			continue
		}

		newM := &SchemaMetadata{
			UUID:          oldM.UUID,
			Name:          oldM.Name,
			Version:       oldM.Version,
			Description:   oldM.Description,
			Concrete:      oldM.Concrete,
			Type:          oldM.Type,
			Values:        oldM.Values,
			Indexes:       oldM.Indexes,
			IndexOrdinals: oldM.IndexOrdinals,
			Metadata:      oldM.Metadata,
		}

		// Remap TargetSchema.
		if oldM.TargetSchema != 0 {
			if newTarget, ok := oldToNew[oldM.TargetSchema]; ok {
				newM.TargetSchema = newTarget
			}
		}

		// Remap Variants slice.
		if len(oldM.Variants) > 0 {
			newM.Variants = make([]uint8, 0, len(oldM.Variants))
			for _, oldV := range oldM.Variants {
				if newV, ok := oldToNew[oldV]; ok {
					newM.Variants = append(newM.Variants, newV)
				}
			}
		}

		// Remap Fields map: keys are descriptor values, rewrite ownerSchema bits.
		if len(oldM.Fields) > 0 {
			newM.Fields = make(map[uint32]FieldMeta, len(oldM.Fields))
			for oldFD, fm := range oldM.Fields {
				newFD := rewriteOwnerSchema(oldFD, oldToNew)
				newM.Fields[newFD] = fm
			}
		}

		promotedMeta[uint8(newIdx)] = newM
	}

	// ── Step 5: Rebuild Store with remapped keys ──────────────────────────────
	//
	// Store keys are DocumentKeys whose DataPoint encodes the field's 15-bit
	// composite id: (ownerSchema << 7) | fieldIndex — see DescriptorToDataPoint.
	// When ownerSchema changes, the DataPoint id changes and therefore the
	// DocumentKey changes. We must rebuild Store by re-deriving keys from the
	// promoted descriptors.
	//
	// We rebuild Store only for fields reachable from the promoted root.
	var promotedStore *document.Document
	if cs.Store != nil {
		promotedStore = rebuildStore(cs, allDescs, oldToNew)
	}

	// ── Step 6: Assemble partial promoted Schema ──────────────────────────────

	promoted2 := &Schema{
		Descriptors:   allDescs,
		SchemaOffsets: offsets,
		Variants:      promotedVariants,
		Store:         promotedStore,
		Meta:          promotedMeta,
		PathCache:     NewPathRegistry(),
	}

	// ── Step 7: Build address space ───────────────────────────────────────────
	//
	// buildAddressSpace reads cs.Meta and cs.SchemaOffsets and cs.Descriptors —
	// all of which are now set on promoted2. It also calls cs.DocumentKey() via
	// the FieldNames population path, which uses cs.Meta. This is safe because
	// promoted2.Meta is fully populated above.
	as, asErrs := buildAddressSpace(promoted2)
	if len(asErrs) > 0 {
		return nil, CompileErrors(asErrs)
	}
	promoted2.AddressSpace = as

	// PathCache is populated as a side effect of DocumentKey() calls during
	// address space verification and by callers. The validator's buildGraph
	// will fill it completely during graph construction.

	// ResolvedConstraints and ResolvedIndexes are intentionally nil:
	// constraints and indexes are only meaningful at the document root.

	return promoted2, nil
}

// =============================================================================
// HELPERS
// =============================================================================

// collectReachable returns all schema indices reachable from root via BFS,
// in stable BFS order (root first).
func collectReachable(cs *Schema, root uint8) []uint8 {
	visited := make(map[uint8]bool)
	queue := []uint8{root}
	visited[root] = true
	var result []uint8

	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		result = append(result, idx)

		start, end := schemaOffsetRange(cs, idx)
		for _, fd := range cs.Descriptors[start:end] {
			if !IsSchemaBearing(fd) {
				continue
			}
			typ := ExtractType(fd)
			if typ == TypeUnion || typ == TypeComposite {
				for _, v := range cs.Variants[fd] {
					if !visited[v] {
						visited[v] = true
						queue = append(queue, v)
					}
				}
			} else {
				target := ExtractTargetSchema(fd)
				if !visited[target] {
					visited[target] = true
					queue = append(queue, target)
				}
			}
		}

		// Type schemas: follow via Meta.
		if start == end {
			if m := cs.Meta[idx]; m != nil && m.Type.IsSchemaBearing() {
				if m.Type == TypeUnion || m.Type == TypeComposite {
					for _, v := range m.Variants {
						if !visited[v] {
							visited[v] = true
							queue = append(queue, v)
						}
					}
				} else if !visited[m.TargetSchema] {
					visited[m.TargetSchema] = true
					queue = append(queue, m.TargetSchema)
				}
			}
		}
	}

	return result
}

// rewriteOwnerSchema rewrites the ownerSchema bits of fd using the oldToNew
// index mapping, leaving all other bits unchanged.
func rewriteOwnerSchema(fd uint32, oldToNew map[uint8]uint8) uint32 {
	oldOwner := ExtractOwnerSchema(fd)
	newOwner, ok := oldToNew[oldOwner]
	if !ok {
		// Not reachable from the promoted root — leave unchanged.
		return fd
	}
	// Clear ownerSchema bits (22–15) and write new owner.
	fd = (fd &^ FDMaskOwnerSchema) | (uint32(newOwner) << 15)

	// Also remap targetSchema bits for schema-bearing fields.
	if IsSchemaBearing(fd) {
		typ := ExtractType(fd)
		if typ != TypeUnion && typ != TypeComposite {
			oldTarget := ExtractTargetSchema(fd)
			if newTarget, ok := oldToNew[oldTarget]; ok {
				fd = (fd &^ FDMaskTargetSchema) | (uint32(newTarget) << 23)
			}
		}
		// Union/composite: targets live in Variants map, not in the descriptor.
	}
	return fd
}

// findOriginalDescriptor finds the original (pre-rewrite) descriptor
// corresponding to a rewritten fd. Used to look up cs.Variants.
func findOriginalDescriptor(cs *Schema, rewrittenFD uint32, oldToNew map[uint8]uint8) uint32 {
	// Reverse the ownerSchema rewrite to find the original owner.
	newOwner := ExtractOwnerSchema(rewrittenFD)
	// Find which old index maps to newOwner.
	for old, new_ := range oldToNew {
		if new_ == newOwner {
			// Reconstruct the original fd with the old ownerSchema.
			origFD := (rewrittenFD &^ FDMaskOwnerSchema) | (uint32(old) << 15)
			// Also restore original targetSchema if present.
			if IsSchemaBearing(origFD) {
				typ := ExtractType(origFD)
				if typ != TypeUnion && typ != TypeComposite {
					newTarget := ExtractTargetSchema(rewrittenFD)
					for oldT, newT := range oldToNew {
						if newT == newTarget {
							origFD = (origFD &^ FDMaskTargetSchema) | (uint32(oldT) << 23)
							break
						}
					}
				}
			}
			return origFD
		}
	}
	return 0
}

// rebuildStore rebuilds the Store Document for the promoted schema.
// For each descriptor in the promoted schema's flat descriptor slice, it
// derives the original descriptor (pre-rewrite), looks up the value in the
// parent store, and writes it under the new promoted key.
func rebuildStore(cs *Schema, promotedDescs []uint32, oldToNew map[uint8]uint8) *document.Document {
	// Build reverse mapping: new fd → old fd, for all promoted descriptors.
	// We need the old fd to look up the parent store.
	type fdPair struct{ old, new_ uint32 }
	pairs := make([]fdPair, 0, len(promotedDescs))
	for _, newFD := range promotedDescs {
		oldFD := findOriginalDescriptor(cs, newFD, oldToNew)
		if oldFD != 0 {
			pairs = append(pairs, fdPair{old: oldFD, new_: newFD})
		}
	}

	if len(pairs) == 0 {
		return nil
	}

	store := document.NewDocument()
	wrote := false

	for _, p := range pairs {
		ft := ExtractType(p.old)

		// ── Enum value sets ───────────────────────────────────────────────────
		if ft == TypeEnum {
			if copyEnumValues(cs.Store, store, p.old, p.new_) {
				wrote = true
			}
		}

		// ── Field defaults ────────────────────────────────────────────────────
		if copyDefault(cs.Store, store, p.old, p.new_, ft) {
			wrote = true
		}
	}

	if !wrote {
		return nil
	}
	return store
}

// copyEnumValues copies an enum field's value set from src to dst,
// re-keying from oldFD to newFD.
func copyEnumValues(src, dst *document.Document, oldFD, newFD uint32) bool {
	wrote := false

	// Try each array type — at most one will hit.
	if strKey := DescriptorToEnumDocumentKey(oldFD, document.TypeArrayString); strKey != 0 {
		if vals, ok, _ := src.GetArrayString(strKey); ok {
			newKey := DescriptorToEnumDocumentKey(newFD, document.TypeArrayString)
			dst.AppendArrayString(newKey, vals)
			wrote = true
		}
	}
	if intKey := DescriptorToEnumDocumentKey(oldFD, document.TypeArrayInt); intKey != 0 {
		if vals, ok, _ := src.GetArrayInt(intKey); ok {
			newKey := DescriptorToEnumDocumentKey(newFD, document.TypeArrayInt)
			dst.AppendArrayInt(newKey, vals)
			wrote = true
		}
	}
	if fltKey := DescriptorToEnumDocumentKey(oldFD, document.TypeArrayFloat); fltKey != 0 {
		if vals, ok, _ := src.GetArrayFloat(fltKey); ok {
			newKey := DescriptorToEnumDocumentKey(newFD, document.TypeArrayFloat)
			dst.AppendArrayFloat(newKey, vals)
			wrote = true
		}
	}
	if boolKey := DescriptorToEnumDocumentKey(oldFD, document.TypeArrayBool); boolKey != 0 {
		if vals, ok, _ := src.GetArrayBool(boolKey); ok {
			newKey := DescriptorToEnumDocumentKey(newFD, document.TypeArrayBool)
			dst.AppendArrayBool(newKey, vals)
			wrote = true
		}
	}

	return wrote
}

// copyDefault copies a field's default value from src to dst,
// re-keying from oldFD to newFD.
func copyDefault(src, dst *document.Document, oldFD, newFD uint32, ft FieldTypeEnum) bool {
	dt := FieldTypeToDataType(ft)
	oldDP, err := document.NewDataPoint(dt, int32((oldFD>>8)&0x7FFF))
	if err != nil {
		return false
	}
	newDP, err := document.NewDataPoint(dt, int32((newFD>>8)&0x7FFF))
	if err != nil {
		return false
	}
	oldKey := document.NewDocumentKey(oldDP, oldFD)
	newKey := document.NewDocumentKey(newDP, newFD)

	switch dt {
	case document.TypeString:
		if v, ok, _ := src.GetString(oldKey); ok {
			dst.AppendString(newKey, v)
			return true
		}
	case document.TypeInt:
		if v, ok, _ := src.GetInt(oldKey); ok {
			dst.AppendInt(newKey, v)
			return true
		}
	case document.TypeFloat:
		if v, ok, _ := src.GetFloat(oldKey); ok {
			dst.AppendFloat(newKey, v)
			return true
		}
	case document.TypeBool:
		if v, ok, _ := src.GetBool(oldKey); ok {
			dst.AppendBool(newKey, v)
			return true
		}
	case document.TypeBytes:
		if v, ok, _ := src.GetBytes(oldKey); ok {
			dst.AppendBytes(newKey, v)
			return true
		}
	}
	return false
}

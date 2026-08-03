package definition

import (
	"fmt"
	"strings"
)

// =============================================================================
// ADDRESS SPACE
// =============================================================================
//
// Every terminal field reachable via any valid path in the schema graph is
// assigned a unique 27-bit integer address. Non-terminal fields (object/array/
// complex containers) are structural — they own sub-blocks but do not receive
// addresses themselves. Only leaf values are addressable.
//
// Single-step paths (root-level fields) occupy [1, 2^14); address 0 is
// reserved as the "not addressable" sentinel (empty path or non-terminal end).
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
//
// The address is derived entirely from a CompiledSchema (Descriptors, Schemas,
// LocalOffsets), so Address is a method of CompiledSchema and its results are
// memoised in the schema's internal cache — there is no separate AddressCache
// type to coordinate.

// Address resolves path to its flat user-data address in O(depth), memoising
// the result in the CompiledSchema's internal cache keyed by PathKey().
//
// Returns 0 when the path is empty or ends in a non-terminal field (only
// terminal/leaf values are addressable). The address is the 27-bit id that,
// combined with the leaf FieldDescriptor, forms the DataContainerKey for flat
// storage of the value.
func (cs *CompiledSchema) Address(path ResolvedPath) uint32 {
	if len(path) == 0 {
		return 0
	}
	key := path.PathKey()

	cs.addrMu.RLock()
	if a, ok := cs.addrCache[key]; ok {
		cs.addrMu.RUnlock()
		return a
	}
	cs.addrMu.RUnlock()

	a := cs.computeAddress(path)

	cs.addrMu.Lock()
	if cs.addrCache == nil {
		cs.addrCache = make(map[string]uint32)
		cs.pathByAddr = make(map[uint32]ResolvedPath)
		cs.nameByAddr = make(map[uint32]string)
	}
	cs.addrCache[key] = a
	if a != 0 {
		// Record the reverse mapping for addressable (terminal) paths. The
		// caller already resolved this path to an address, so storing the
		// path we already hold is free — downstream readers holding a value's
		// address can recover its path without re-walking the schema. A copy
		// is kept so later mutation of the caller's slice can't corrupt it.
		// nameByAddr caches the dotted form so naming a value is a single
		// lookup, not a per-read re-join.
		cp := append(ResolvedPath(nil), path...)
		cs.pathByAddr[a] = cp
		cs.nameByAddr[a] = cs.joinPath(cp)
	}
	cs.addrMu.Unlock()
	return a
}

// computeAddress is the uncached address computation.
//
// Each step's sibling-offset sum is precomputed at link time in
// CompiledSchema.LocalOffsets (parallel to Descriptors), so every step here
// is a single array lookup instead of a rescan of the fields declared before
// it in the same schema slot.
func (cs *CompiledSchema) computeAddress(path ResolvedPath) uint32 {
	if len(path) == 0 {
		return 0
	}
	if len(path) == 1 {
		step := path[0]
		slot := &cs.Schemas[step.SchemaIdx()]
		abs := int(slot.FieldStart) + int(step.FieldIdx())
		if !cs.Descriptors[abs].Terminal() {
			return 0
		}
		// +1 keeps 0 reserved for "not addressable": a single-step path always
		// lives in the root slot (FieldStart 0, FieldIdx < 256), so addresses
		// stay in [1, 2^14) and can never alias a terminal whose computed
		// address is 0 (the very first root field).
		return uint32(abs) + 1
	}

	base := uint32(MultiStepBase)
	for i, step := range path {
		slot := &cs.Schemas[step.SchemaIdx()]
		abs := int(slot.FieldStart) + int(step.FieldIdx())
		fd := cs.Descriptors[abs]
		base += cs.LocalOffsets[abs]

		if i == len(path)-1 {
			if !fd.Terminal() {
				return 0
			}
			return base
		}
		if fd.Terminal() {
			return 0
		}
	}
	return base
}

// ResolvePath resolves a dot-separated path (e.g. "product.dimensions.width")
// into a ResolvedPath of (SchemaIdx, FieldIdx) steps, starting from the root
// schema slot. Each non-final segment must name a non-terminal field whose
// child schema can be descended into (object, array/record-of-object, or
// recursive). Union and composite fields have no single child schema, so a
// path cannot be resolved through them.
//
// The result is suitable for Address()/key construction; Address() returns 0
// for paths that resolve but end in a non-terminal field.
func (cs *CompiledSchema) ResolvePath(path string) (ResolvedPath, error) {
	if path == "" {
		return nil, ErrInvalidSchema.WithMessage("cannot resolve an empty path")
	}
	segments := strings.Split(path, ".")
	rp := make(ResolvedPath, 0, len(segments))
	schemaIdx := uint8(0) // root schema slot
	for i, segment := range segments {
		step, fd, err := cs.ResolveFieldStep(schemaIdx, segment)
		if err != nil {
			return nil, err
		}
		rp = append(rp, step)
		if i == len(segments)-1 {
			return rp, nil
		}
		if fd.Terminal() {
			return nil, ErrInvalidSchema.WithMessage(
				fmt.Sprintf("field %q is terminal and cannot be descended into", segment),
			)
		}
		if fd.ChildSchemaIdx() == FdNoChild {
			return nil, ErrInvalidSchema.WithMessage(
				fmt.Sprintf("field %q has no single child schema; union/composite fields cannot be path-resolved", segment),
			)
		}
		schemaIdx = fd.ChildSchemaIdx()
	}
	return rp, nil
}

// ResolveFieldStep finds the (SchemaIdx, FieldIdx) step and FieldDescriptor for
// the field named name within schema slot schemaIdx. It is exported so
// downstream consumers that resolve view-relative paths (e.g. the document
// layer, whose nested views are anchored at a non-root slot) can share the
// single field-lookup implementation instead of re-walking the schema.
func (cs *CompiledSchema) ResolveFieldStep(schemaIdx uint8, name string) (ResolvedStep, FieldDescriptor, error) {
	if int(schemaIdx) >= len(cs.Schemas) {
		return 0, 0, ErrInvalidSchema.WithMessage(
			fmt.Sprintf("schema slot %d is out of range (compiled schema has %d)", schemaIdx, len(cs.Schemas)),
		)
	}
	slot := &cs.Schemas[schemaIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		if abs < len(cs.FieldsMeta) && cs.FieldsMeta[abs].Name == name {
			return NewResolvedStep(schemaIdx, uint8(j)), cs.Descriptors[abs], nil
		}
	}
	return 0, 0, ErrFieldNotFound.WithMessage(
		fmt.Sprintf("no field named %q in schema slot %d", name, schemaIdx),
	)
}

// PathForAddress returns the ResolvedPath that produced addr, if any caller has
// resolved a path to this address (Address() records the reverse mapping for
// every addressable path it computes). Downstream code that holds a stored
// value's address can recover its path without re-walking the schema; because a
// value can only be read after its path was resolved to an address, the entry
// is guaranteed to be present for any addressable value that was stored.
func (cs *CompiledSchema) PathForAddress(addr uint32) (ResolvedPath, bool) {
	cs.addrMu.RLock()
	defer cs.addrMu.RUnlock()
	rp, ok := cs.pathByAddr[addr]
	return rp, ok
}

// PathString renders a ResolvedPath as its dotted form (e.g. "address.zip") by
// resolving each step's field name. This is the single place flattened paths
// are rendered — downstream code uses it instead of re-walking the schema.
func (cs *CompiledSchema) PathString(path ResolvedPath) string {
	return cs.joinPath(path)
}

// PathNameForAddress returns the cached dotted form (e.g. "address.zip") of the
// path that produced addr. Address() records both the ResolvedPath and its
// joined name for every addressable path it computes, so naming a stored value
// from its address is a single map lookup with no allocation. Returns false for
// addresses that were never resolved or that belong to non-addressable
// (non-terminal) values.
func (cs *CompiledSchema) PathNameForAddress(addr uint32) (string, bool) {
	cs.addrMu.RLock()
	defer cs.addrMu.RUnlock()
	s, ok := cs.nameByAddr[addr]
	return s, ok
}

// joinPath resolves each step's field name and joins them with ".". The result
// is cached in nameByAddr for addressable paths, so this only runs once per
// unique path at address-resolution time.
func (cs *CompiledSchema) joinPath(path ResolvedPath) string {
	parts := make([]string, 0, len(path))
	for _, step := range path {
		if int(step.SchemaIdx()) >= len(cs.Schemas) {
			continue
		}
		slot := &cs.Schemas[step.SchemaIdx()]
		abs := int(slot.FieldStart) + int(step.FieldIdx())
		if abs >= 0 && abs < len(cs.FieldsMeta) {
			parts = append(parts, cs.FieldsMeta[abs].Name)
		}
	}
	return strings.Join(parts, ".")
}

// FieldPath renders a field descriptor's local path (its single-step path
// within its own schema slot). For a root-level field this equals the field's
// fully-qualified path; it is used to name values stored under internal keys
// (records, array-of-object fields, unions) which carry no flat address.
func (cs *CompiledSchema) FieldPath(fd FieldDescriptor) string {
	return cs.PathString(ResolvedPath{NewResolvedStep(fd.SchemaIdx(), fd.FieldIdx())})
}

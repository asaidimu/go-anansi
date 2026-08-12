package definition

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

// =============================================================================
// LINK PHASE
// =============================================================================

const (
	// maxSchemaSlots bounds the total number of schema slots (including the
	// root) that Link can produce. SchemaIdx/ChildSchemaIdx are 6-bit fields
	// in FieldDescriptor, and 0x3F (FdNoChild) is reserved as the "no child"
	// sentinel, so valid slot indices are [0, 62] — 63 slots total. This is
	// derived from FdNoChild (compiled.go) so the two can never drift apart.
	maxSchemaSlots = int(FdNoChild) // 63

	// maxFieldsPerSchema bounds the number of fields any single schema slot
	// may declare. FieldIdx is a 7-bit field in FieldDescriptor, giving 128
	// distinct values (0-127). Exceeding this would silently alias field
	// indices via the &0x7F mask in MakeFieldDescriptor.
	maxFieldsPerSchema = 128
)

func Link(rs *ResolvedSchema) (*CompiledSchema, error) {
	if rs == nil {
		return nil, fmt.Errorf("cannot link a nil ResolvedSchema")
	}

	defaults := container.NewDataContainer()

	lc := &linkContext{
		schemas:             make([]SchemaSlot, 0, 16),
		schemasMeta:         make([]SchemaMeta, 0, 16),
		slots:               make(map[*ResolvedNestedSchema][]uint8),
		defaults:            defaults,
		enums:               container.NewDataContainer(),
		variants:            make(map[uint32][]uint8),
		schemaConstraints:   make([]SchemaConstraint, 0, 16),
		fieldRefConstraints: make(map[uint32]SchemaConstraint),
		rs:                  rs,
	}

	// Root schema slot 0. No SchemaId: the root is identified by the
	// top-level Schema.Name/Version, not a nested-schema UUID, so its
	// SchemaMeta.ID stays at the zero value.
	lc.schemas = append(lc.schemas, SchemaSlot{})
	lc.schemasMeta = append(lc.schemasMeta, SchemaMeta{Name: "root"})
	lc.schemaConstraints = append(lc.schemaConstraints, nil) // root has no raw constraints
	rootStart := uint16(0)

	rootCount, err := lc.linkFields(rs.Fields, 0)
	if err != nil {
		return nil, err
	}
	lc.schemas[0] = SchemaSlot{
		FieldStart: rootStart,
		FieldCount: rootCount,
	}

	// Defensive invariant: assignSlot() already rejects any request that
	// would push the slot count past maxSchemaSlots, so this should be
	// unreachable. Kept as a guard against internal bugs (e.g. a future
	// code path that appends to lc.schemas without going through assignSlot).
	if len(lc.schemas) > maxSchemaSlots {
		return nil, fmt.Errorf("compiled schema exceeds maximum of %d nested schemas (got %d): reduce schema nesting or inline complexity", maxSchemaSlots, len(lc.schemas))
	}

	// Compute footprints bottom-up (schemas are indexed DFS: parent before child).
	for i := len(lc.schemas) - 1; i >= 0; i-- {
		slot := &lc.schemas[i]
		var fp uint32
		for j := uint16(0); j < slot.FieldCount; j++ {
			fd := lc.descriptors[slot.FieldStart+j]
			if fd.Terminal() {
				fp++
			} else if fd.ChildSchemaIdx() != FdNoChild {
				fp += lc.schemas[fd.ChildSchemaIdx()].Footprint
			}
		}
		slot.Footprint = fp
	}

	// Validate root's non-terminal children fit in the multi-step region.
	var rootFP uint32
	rootSlot := &lc.schemas[0]
	for j := uint16(0); j < rootSlot.FieldCount; j++ {
		fd := lc.descriptors[rootSlot.FieldStart+j]
		if !fd.Terminal() && fd.ChildSchemaIdx() != FdNoChild {
			rootFP += lc.schemas[fd.ChildSchemaIdx()].Footprint
		}
	}
	if rootFP > MultiStepSize {
		return nil, fmt.Errorf("schema tree too large: need %d address slots but multi-step region has %d", rootFP, MultiStepSize)
	}

	// Compute LocalOffsets: the prefix-sum offset of each descriptor within its
	// own schema's address block. Within a block, a terminal field consumes one
	// slot and a non-terminal field consumes Footprint(child) slots. Address()
	// relies on this table to resolve a multi-step path in O(depth).
	localOffsets := make([]uint32, len(lc.descriptors))
	for s := range lc.schemas {
		slot := &lc.schemas[s]
		var acc uint32
		for j := uint16(0); j < slot.FieldCount; j++ {
			abs := int(slot.FieldStart) + int(j)
			fd := lc.descriptors[abs]
			localOffsets[abs] = acc
			if fd.Terminal() {
				acc++
			} else if fd.ChildSchemaIdx() != FdNoChild {
				acc += lc.schemas[fd.ChildSchemaIdx()].Footprint
			}
		}
	}

	return &CompiledSchema{
		Descriptors:         lc.descriptors,
		FieldsMeta:          lc.fieldsMeta,
		FieldTypes:          lc.fieldTypes,
		Schemas:             lc.schemas,
		SchemasMeta:         lc.schemasMeta,
		Defaults:            lc.defaults,
		Enums:               lc.enums,
		Variants:            lc.variants,
		Constraints:         rs.Constraints,
		Indexes:             rs.Indexes,
		SchemaConstraints:   lc.schemaConstraints,
		FieldRefConstraints: lc.fieldRefConstraints,
		LocalOffsets:        localOffsets,
	}, nil
}

// =============================================================================
// LINK CONTEXT
// =============================================================================

type linkContext struct {
	descriptors []FieldDescriptor
	fieldsMeta  []FieldMeta
	fieldTypes  []FieldType
	schemas     []SchemaSlot
	schemasMeta []SchemaMeta
	slots       map[*ResolvedNestedSchema][]uint8
	defaults    *container.DataContainer
	enums       *container.DataContainer
	variants    map[uint32][]uint8

	schemaConstraints   []SchemaConstraint          // per slot
	fieldRefConstraints map[uint32]SchemaConstraint // keyed by DataPoint
	rs                  *ResolvedSchema
}

// assignSlot allocates a new schema slot for rns. It rejects the request
// once the slot count reaches maxSchemaSlots, before the next index would
// collide with the FdNoChild sentinel (0x3F) or, in extreme cases, before
// the uint8 conversion below could silently wrap around.
func (lc *linkContext) assignSlot(rns *ResolvedNestedSchema) (uint8, error) {
	if len(lc.schemas) >= maxSchemaSlots {
		return 0, fmt.Errorf("schema definition exceeds maximum of %d nested schemas while assigning a slot for %q: reduce schema nesting or inline complexity", maxSchemaSlots, rns.Name)
	}
	idx := uint8(len(lc.schemas))
	lc.schemas = append(lc.schemas, SchemaSlot{})
	lc.schemasMeta = append(lc.schemasMeta, SchemaMeta{
		ID:          rns.ID,
		Name:        rns.Name,
		Description: "",
	})
	lc.slots[rns] = append(lc.slots[rns], idx)
	lc.schemaConstraints = append(lc.schemaConstraints, rns.RawConstraints)
	return idx, nil
}

// childSlotForField pre-assigns a child schema slot for a non-terminal field
// that has child fields flattened into the CompiledSchema.
// For recursive fields the field's own schema slot is stored as the child,
// allowing the graph builder to identify the recursive target.
// Returns the child slot index, or 0x7F if the field has no flattenable child.
func (lc *linkContext) childSlotForField(rf *ResolvedField, schemaIdx uint8) (uint8, error) {
	switch {
	case rf.Recursive != nil:
		return schemaIdx, nil
	case rf.Object != nil:
		return lc.assignSlot(rf.Object.Schema)
	case rf.Composite != nil:
		// Composites collapse to a single child schema, just like an object.
		return lc.assignSlot(&ResolvedNestedSchema{Name: "composite:" + string(rf.Name)})
	case rf.Container != nil && rf.Container.ItemSchema != nil:
		return lc.assignSlot(rf.Container.ItemSchema)
	}
	return 0x7F, nil
}

func (lc *linkContext) linkFields(fields []ResolvedField, schemaIdx uint8) (uint16, error) {
	if len(fields) > maxFieldsPerSchema {
		return 0, fmt.Errorf("schema slot %d declares %d fields, exceeds maximum of %d fields per schema (FieldIdx is a 7-bit descriptor field)", schemaIdx, len(fields), maxFieldsPerSchema)
	}

	// Descriptors must be laid out grouped per schema: a schema's own fields are
	// contiguous (FieldStart..FieldStart+FieldCount), and every non-terminal
	// child subtree is linked after all sibling descriptors. Address() relies on
	// this so that slot.FieldStart+fieldIdx indexes the field's descriptor and
	// single-step paths map to unique absolute descriptor indices.
	//
	// Pass 1: create the descriptors and metadata for every field in this
	// schema. Pass 2: link non-terminal child subtrees, now that the whole
	// sibling block is in place.
	type childWork struct {
		rf       *ResolvedField
		fd       FieldDescriptor
		childIdx uint8
	}
	var children []childWork

	for i := range fields {
		rf := &fields[i]
		dt, kind, terminal := classifyField(rf)

		childSchemaIdx, err := lc.childSlotForField(rf, schemaIdx)
		if err != nil {
			return 0, err
		}
		hasDefault := !rf.Default.IsZero()

		fd := MakeFieldDescriptor(
			dt, kind, schemaIdx, uint8(i),
			rf.Required, hasDefault, rf.Deprecated, rf.Unique, terminal, rf.Nullable, rf.Recursive != nil,
			childSchemaIdx,
		)
		lc.descriptors = append(lc.descriptors, fd)
		lc.fieldTypes = append(lc.fieldTypes, rf.Type)
		dp := fd.DataPoint()

		lc.fieldsMeta = append(lc.fieldsMeta, FieldMeta{
			ID:          string(rf.ID),
			Name:        string(rf.Name),
			Path:        rf.Path,
			Parts:       rf.Parts,
			Description: rf.Description,
			Default:     rf.Default,
		})

		// Store call-site constraint overrides for object/recursive fields.
		if rf.Recursive != nil && len(rf.Recursive.RefConstraints) > 0 {
			lc.fieldRefConstraints[dp] = rf.Recursive.RefConstraints
		} else if rf.Object != nil && len(rf.Object.RefConstraints) > 0 {
			lc.fieldRefConstraints[dp] = rf.Object.RefConstraints
		}

		// Set default value in the defaults DataContainer if present.
		if hasDefault {
			if err := setDefault(lc.defaults, dp, dt, rf.Default); err != nil {
				return 0, err
			}
		}

		// Store enum values in the Enums document if this field has an enum schema.
		if rf.Enum != nil {
			if err := setEnumValues(lc.enums, fd, rf.Enum); err != nil {
				return 0, err
			}
		}

		if !terminal {
			children = append(children, childWork{rf: rf, fd: fd, childIdx: childSchemaIdx})
		}
	}

	for _, cw := range children {
		if err := lc.linkChildFields(cw.rf, cw.fd, cw.childIdx); err != nil {
			return 0, err
		}
	}

	return uint16(len(fields)), nil
}

func (lc *linkContext) linkChildFields(rf *ResolvedField, fd FieldDescriptor, childIdx uint8) error {
	switch {
	case rf.Object != nil && rf.Recursive == nil:
		childStart := uint16(len(lc.descriptors))
		childCount, err := lc.linkFields(rf.Object.Schema.Fields, childIdx)
		if err != nil {
			return err
		}
		lc.schemas[childIdx] = SchemaSlot{
			FieldStart: childStart,
			FieldCount: childCount,
		}

	case rf.Container != nil && rf.Recursive == nil && rf.Container.ItemSchema != nil:
		childStart := uint16(len(lc.descriptors))
		childCount, err := lc.linkFields(rf.Container.ItemSchema.Fields, childIdx)
		if err != nil {
			return err
		}
		lc.schemas[childIdx] = SchemaSlot{
			FieldStart: childStart,
			FieldCount: childCount,
		}

	case rf.Union != nil:
		var variantSlots []uint8
		for _, variant := range rf.Union.Variants {
			childIdx, err := lc.assignSlot(variant)
			if err != nil {
				return err
			}
			variantSlots = append(variantSlots, childIdx)
			childStart := uint16(len(lc.descriptors))
			childCount, err := lc.linkFields(variant.Fields, childIdx)
			if err != nil {
				return err
			}
			lc.schemas[childIdx] = SchemaSlot{
				FieldStart: childStart,
				FieldCount: childCount,
			}
		}
		lc.variants[fd.DataPoint()] = variantSlots

	case rf.Composite != nil:
		// All parts collapse into the single pre-assigned child slot, exactly
		// as if the composite were one object schema.
		merged, err := collapsedCompositeFields(rf)
		if err != nil {
			return err
		}
		childStart := uint16(len(lc.descriptors))
		childCount, err := lc.linkFields(merged, childIdx)
		if err != nil {
			return err
		}
		lc.schemas[childIdx] = SchemaSlot{
			FieldStart: childStart,
			FieldCount: childCount,
		}
		lc.variants[fd.DataPoint()] = []uint8{childIdx}
	}

	return nil
}

// collapsedCompositeFields merges every part of a composite field into a single
// field list, as if the composite were one object schema. Duplicate field names
// across parts are rejected — they would collide in the flattened key space.
func collapsedCompositeFields(rf *ResolvedField) ([]ResolvedField, error) {
	var merged []ResolvedField
	seen := make(map[FieldName]struct{}, len(rf.Composite.ObjectParts))
	appendPart := func(fields []ResolvedField) error {
		for _, f := range fields {
			if _, dup := seen[f.Name]; dup {
				return fmt.Errorf("composite %q: field %q is declared by more than one part and cannot be collapsed", rf.Name, f.Name)
			}
			seen[f.Name] = struct{}{}
			merged = append(merged, f)
		}
		return nil
	}
	for _, part := range rf.Composite.ObjectParts {
		if err := appendPart(part.Fields); err != nil {
			return nil, err
		}
	}
	for _, up := range rf.Composite.UnionParts {
		for _, variant := range up.Variants {
			if err := appendPart(variant.Fields); err != nil {
				return nil, err
			}
		}
	}
	return merged, nil
}

// =============================================================================
// DEFAULT VALUE SETUP
// =============================================================================

func setDefault(doc *container.DataContainer, dp uint32, dt container.DataType, lv LiteralValue) error {
	// DataContainerKey with descriptor=0 (not used for defaults).
	key := container.NewDataContainerKey(container.DataPoint(dp), 0)
	if lv.IsNull() {
		doc.SetNull(key)
		return nil
	}
	val := lv.Value()
	if val == nil {
		return nil
	}

	switch dt {
	case container.TypeInt:
		v, ok := val.(int64)
		if !ok {
			return nil
		}
		return doc.SetInt(key, v)

	case container.TypeFloat:
		v, ok := val.(float64)
		if !ok {
			return nil
		}
		return doc.SetFloat(key, v)

	case container.TypeString:
		v, ok := val.(string)
		if !ok {
			return nil
		}
		return doc.SetString(key, v)

	case container.TypeBool:
		v, ok := val.(bool)
		if !ok {
			return nil
		}
		return doc.SetBool(key, v)

	case container.TypeBytes:
		v, ok := val.([]byte)
		if !ok {
			return nil
		}
		return doc.SetBytes(key, v)
	}

	return nil
}

// =============================================================================
// FIELD CLASSIFICATION
// =============================================================================

func classifyField(rf *ResolvedField) (container.DataType, FieldKind, bool) {
	switch {
	case rf.Type == FieldTypeGeometry:
		return container.TypeGeometry, KindSimple, true
	case rf.Scalar != nil:
		return scalarDataType(rf.Type), KindSimple, true
	case rf.Enum != nil:
		return enumDataType(rf), KindSimple, true
	case rf.Recursive != nil:
		return container.TypeUnknown, KindObject, true
	case rf.Object != nil:
		// Named objects flatten their children into the key space (no nested
		// container value), so the field itself carries no value type.
		return container.TypeUnknown, KindObject, false
	case rf.Container != nil:
		terminal := rf.Container.ItemSchema == nil
		if rf.Container.Record {
			// Records are schema-free sub-objects: stored as map[string]any in
			// the dedicated TypeRecord slot.
			return container.TypeRecord, KindObject, terminal
		}
		return containerDataType(rf.Container.ItemSchema, rf.Container.ItemType), KindArrayField, terminal
	case rf.Union != nil:
		// Union values live in the TypeUnknown (any) channel; the variant
		// children are still flattened into the key space.
		return container.TypeUnknown, KindComplex, false
	case rf.Composite != nil:
		// Composites are variadic objects — their parts collapse into a single
		// child schema at link time, so the field is classified exactly like an
		// object: children flatten into the key space.
		return container.TypeUnknown, KindObject, false
	}
	return container.TypeUnknown, KindSimple, true
}

func scalarDataType(ft FieldType) container.DataType {
	switch ft {
	case FieldTypeString:
		return container.TypeString
	case FieldTypeNumber:
		return container.TypeFloat
	case FieldTypeDecimal:
		return container.TypeString // canonical decimal string
	case FieldTypeInteger:
		return container.TypeInt
	case FieldTypeBoolean:
		return container.TypeBool
	case FieldTypeBytes:
		return container.TypeBytes
	default:
		return container.TypeUnknown
	}
}

func enumDataType(rf *ResolvedField) container.DataType {
	if rf.Enum != nil && rf.Enum.ExpectNumeric {
		return container.TypeInt
	}
	return container.TypeString
}

func containerDataType(itemSchema *ResolvedNestedSchema, itemType FieldType) container.DataType {
	if itemSchema != nil {
		return container.TypeArrayObject
	}
	switch itemType {
	case FieldTypeString:
		return container.TypeArrayString
	case FieldTypeNumber:
		return container.TypeArrayFloat
	case FieldTypeDecimal:
		return container.TypeArrayString // array of canonical decimal strings
	case FieldTypeInteger:
		return container.TypeArrayInt
	case FieldTypeBoolean:
		return container.TypeArrayBool
	case FieldTypeBytes:
		return container.TypeArrayBytes
	case FieldTypeGeometry:
		return container.TypeArrayGeometry
	default:
		return container.TypeArrayUnknown
	}
}

func setEnumValues(doc *container.DataContainer, fd FieldDescriptor, re *ResolvedEnum) error {
	// Extract the 27-bit field ID from the field descriptor's DataPoint.
	dp := fd.DataPoint()
	id := int32(dp) >> 5 // bits 5-31

	// Store based on enum type. ExpectNumeric=true means the values are int64;
	// otherwise they're strings (or complex/mixed).
	if len(re.Complex) > 0 {
		edp, err := container.NewDataPoint(container.TypeArrayUnknown, id)
		if err != nil {
			return err
		}
		ek := container.NewDataContainerKey(edp, 0)
		all := make([]any, 0, len(re.Lookup)+len(re.Complex))
		for v := range re.Lookup {
			all = append(all, v)
		}
		all = append(all, re.Complex...)
		return doc.SetArrayUnknown(ek, all)
	}

	if re.ExpectNumeric {
		edp, err := container.NewDataPoint(container.TypeArrayInt, id)
		if err != nil {
			return err
		}
		ek := container.NewDataContainerKey(edp, 0)
		vals := make([]int64, 0, len(re.Lookup))
		for v := range re.Lookup {
			if vi, ok := v.(int64); ok {
				vals = append(vals, vi)
			}
		}
		return doc.SetArrayInt(ek, vals)
	}

	// String enum (default)
	edp, err := container.NewDataPoint(container.TypeArrayString, id)
	if err != nil {
		return err
	}
	ek := container.NewDataContainerKey(edp, 0)
	vals := make([]string, 0, len(re.Lookup))
	for v := range re.Lookup {
		if vs, ok := v.(string); ok {
			vals = append(vals, vs)
		}
	}
	return doc.SetArrayString(ek, vals)
}

package definition

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v7/core/document"
)

// =============================================================================
// LINK PHASE
// =============================================================================

func Link(rs *ResolvedSchema) (*CompiledSchema, error) {
	defaults := document.NewDocument()

	lc := &linkContext{
		schemas:              make([]SchemaSlot, 0, 16),
		schemasMeta:          make([]SchemaMeta, 0, 16),
		slots:                make(map[*ResolvedNestedSchema][]uint8),
		defaults:             defaults,
		enums:                document.NewDocument(),
		variants:             make(map[uint32][]uint8),
		schemaConstraints:    make([]SchemaConstraint, 0, 16),
		fieldRefConstraints:  make(map[uint32]SchemaConstraint),
		rs:                   rs,
	}

	// Root schema slot 0.
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

	if len(lc.schemas) > 63 {
		return nil, fmt.Errorf("compiled schema exceeds maximum of 63 nested schemas (got %d): reduce schema nesting or inline complexity", len(lc.schemas))
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
	defaults    *document.Document
	enums       *document.Document
	variants    map[uint32][]uint8

	schemaConstraints   []SchemaConstraint          // per slot
	fieldRefConstraints map[uint32]SchemaConstraint  // keyed by DataPoint
	rs                  *ResolvedSchema
}

func (lc *linkContext) assignSlot(rns *ResolvedNestedSchema) uint8 {
	idx := uint8(len(lc.schemas))
	lc.schemas = append(lc.schemas, SchemaSlot{})
	lc.schemasMeta = append(lc.schemasMeta, SchemaMeta{
		Name:        rns.Name,
		Description: "",
	})
	lc.slots[rns] = append(lc.slots[rns], idx)
	lc.schemaConstraints = append(lc.schemaConstraints, rns.RawConstraints)
	return idx
}

// childSlotForField pre-assigns a child schema slot for a non-terminal field
// that has child fields flattened into the CompiledSchema.
// For recursive fields the field's own schema slot is stored as the child,
// allowing the graph builder to identify the recursive target.
// Returns the child slot index, or 0x7F if the field has no flattenable child.
func (lc *linkContext) childSlotForField(rf *ResolvedField, schemaIdx uint8) uint8 {
	switch {
	case rf.Recursive != nil:
		return schemaIdx
	case rf.Object != nil:
		return lc.assignSlot(rf.Object.Schema)
	case rf.Container != nil && rf.Container.ItemSchema != nil:
		return lc.assignSlot(rf.Container.ItemSchema)
	}
	return 0x7F
}

func (lc *linkContext) linkFields(fields []ResolvedField, schemaIdx uint8) (uint16, error) {
	start := uint16(len(lc.descriptors))

	for i := range fields {
		rf := &fields[i]
		dt, kind, terminal := classifyField(rf)

		childSchemaIdx := lc.childSlotForField(rf, schemaIdx)
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

		// Set default value in the defaults Document if present.
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
			if err := lc.linkChildFields(rf, fd); err != nil {
				return 0, err
			}
		}
	}

	return uint16(len(lc.descriptors) - int(start)), nil
}

func (lc *linkContext) linkChildFields(rf *ResolvedField, fd FieldDescriptor) error {
	switch {
	case rf.Object != nil && rf.Recursive == nil:
		childStart := uint16(len(lc.descriptors))
		childIdx := lc.slots[rf.Object.Schema][len(lc.slots[rf.Object.Schema])-1]
		_, err := lc.linkFields(rf.Object.Schema.Fields, childIdx)
		if err != nil {
			return err
		}
		lc.schemas[childIdx] = SchemaSlot{
			FieldStart: childStart,
			FieldCount: uint16(len(lc.descriptors)) - childStart,
		}

	case rf.Container != nil && rf.Recursive == nil && rf.Container.ItemSchema != nil:
		childStart := uint16(len(lc.descriptors))
		childIdx := lc.slots[rf.Container.ItemSchema][len(lc.slots[rf.Container.ItemSchema])-1]
		_, err := lc.linkFields(rf.Container.ItemSchema.Fields, childIdx)
		if err != nil {
			return err
		}
		lc.schemas[childIdx] = SchemaSlot{
			FieldStart: childStart,
			FieldCount: uint16(len(lc.descriptors)) - childStart,
		}

	case rf.Union != nil:
		var variantSlots []uint8
		for _, variant := range rf.Union.Variants {
			childIdx := lc.assignSlot(variant)
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
		var partSlots []uint8
		for _, part := range rf.Composite.ObjectParts {
			childIdx := lc.assignSlot(part)
			partSlots = append(partSlots, childIdx)
			childStart := uint16(len(lc.descriptors))
			childCount, err := lc.linkFields(part.Fields, childIdx)
			if err != nil {
				return err
			}
			lc.schemas[childIdx] = SchemaSlot{
				FieldStart: childStart,
				FieldCount: childCount,
			}
		}
		for _, up := range rf.Composite.UnionParts {
			for _, variant := range up.Variants {
				childIdx := lc.assignSlot(variant)
				partSlots = append(partSlots, childIdx)
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
		}
		lc.variants[fd.DataPoint()] = partSlots
	}

	return nil
}

// =============================================================================
// DEFAULT VALUE SETUP
// =============================================================================

func setDefault(doc *document.Document, dp uint32, dt document.DataType, lv LiteralValue) error {
	// DocumentKey with descriptor=0 (not used for defaults).
	key := document.NewDocumentKey(document.DataPoint(dp), 0)
	if lv.IsNull() {
		doc.SetNull(key)
		return nil
	}
	val := lv.Value()
	if val == nil {
		return nil
	}

	switch dt {
	case document.TypeInt:
		v, ok := val.(int64)
		if !ok {
			return nil
		}
		return doc.SetInt(key, v)

	case document.TypeFloat:
		v, ok := val.(float64)
		if !ok {
			return nil
		}
		return doc.SetFloat(key, v)

	case document.TypeString:
		v, ok := val.(string)
		if !ok {
			return nil
		}
		return doc.SetString(key, v)

	case document.TypeBool:
		v, ok := val.(bool)
		if !ok {
			return nil
		}
		return doc.SetBool(key, v)

	case document.TypeBytes:
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

func classifyField(rf *ResolvedField) (document.DataType, FieldKind, bool) {
	switch {
	case rf.Type == FieldTypeGeometry:
		return document.TypeGeometry, KindSimple, true
	case rf.Scalar != nil:
		return scalarDataType(rf.Type), KindSimple, true
	case rf.Enum != nil:
		return enumDataType(rf), KindSimple, true
	case rf.Recursive != nil:
		return document.TypeRecord, KindObject, true
	case rf.Object != nil:
		return document.TypeRecord, KindObject, false
	case rf.Container != nil:
		terminal := rf.Container.ItemSchema == nil
		return containerDataType(rf.Container.ItemSchema, rf.Container.ItemType), KindArrayField, terminal
	case rf.Union != nil:
		return document.TypeRecord, KindComplex, false
	case rf.Composite != nil:
		return document.TypeRecord, KindComplex, false
	}
	return document.TypeUnknown, KindSimple, true
}

func scalarDataType(ft FieldType) document.DataType {
	switch ft {
	case FieldTypeString:
		return document.TypeString
	case FieldTypeNumber, FieldTypeDecimal:
		return document.TypeFloat
	case FieldTypeInteger:
		return document.TypeInt
	case FieldTypeBoolean:
		return document.TypeBool
	case FieldTypeBytes:
		return document.TypeBytes
	default:
		return document.TypeUnknown
	}
}

func enumDataType(rf *ResolvedField) document.DataType {
	if rf.Enum != nil && rf.Enum.ExpectNumeric {
		return document.TypeInt
	}
	return document.TypeString
}

func containerDataType(itemSchema *ResolvedNestedSchema, itemType FieldType) document.DataType {
	if itemSchema != nil {
		return document.TypeArrayObject
	}
	switch itemType {
	case FieldTypeString:
		return document.TypeArrayString
	case FieldTypeNumber, FieldTypeDecimal:
		return document.TypeArrayFloat
	case FieldTypeInteger:
		return document.TypeArrayInt
	case FieldTypeBoolean:
		return document.TypeArrayBool
	case FieldTypeBytes:
		return document.TypeArrayBytes
	case FieldTypeGeometry:
		return document.TypeArrayGeometry
	default:
		return document.TypeArrayUnknown
	}
}

func setEnumValues(doc *document.Document, fd FieldDescriptor, re *ResolvedEnum) error {
	// Extract the 27-bit field ID from the field descriptor's DataPoint.
	dp := fd.DataPoint()
	id := int32(dp) >> 5 // bits 5-31

	// Store based on enum type. ExpectNumeric=true means the values are int64;
	// otherwise they're strings (or complex/mixed).
	if len(re.Complex) > 0 {
		edp, err := document.NewDataPoint(document.TypeArrayUnknown, id)
		if err != nil {
			return err
		}
		ek := document.NewDocumentKey(edp, 0)
		all := make([]any, 0, len(re.Lookup)+len(re.Complex))
		for v := range re.Lookup {
			all = append(all, v)
		}
		all = append(all, re.Complex...)
		return doc.SetArrayUnknown(ek, all)
	}

	if re.ExpectNumeric {
		edp, err := document.NewDataPoint(document.TypeArrayInt, id)
		if err != nil {
			return err
		}
		ek := document.NewDocumentKey(edp, 0)
		vals := make([]int64, 0, len(re.Lookup))
		for v := range re.Lookup {
			if vi, ok := v.(int64); ok {
				vals = append(vals, vi)
			}
		}
		return doc.SetArrayInt(ek, vals)
	}

	// String enum (default)
	edp, err := document.NewDataPoint(document.TypeArrayString, id)
	if err != nil {
		return err
	}
	ek := document.NewDocumentKey(edp, 0)
	vals := make([]string, 0, len(re.Lookup))
	for v := range re.Lookup {
		if vs, ok := v.(string); ok {
			vals = append(vals, vs)
		}
	}
	return doc.SetArrayString(ek, vals)
}

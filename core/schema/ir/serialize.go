package ir

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// Serialize converts a Schema back into its original JSON representation.
// It relies on the presence of Schema.Meta. If Meta is nil or incomplete,
// serialization will fail or produce partial output.
func Serialize(cs *Schema) ([]byte, error) {
	if cs.Meta == nil {
		return nil, fmt.Errorf("serialize: Schema.Meta is nil")
	}

	rootMeta, ok := cs.Meta[0]
	if !ok {
		return nil, fmt.Errorf("serialize: root schema metadata not found")
	}

	src := sourceSchema{
		Name:        rootMeta.Name,
		Description: rootMeta.Description,
		Version:     rootMeta.Version,
		Concrete:    rootMeta.Concrete,
		Fields:      make(map[string]sourceField),
		Schemas:     make(map[string]sourceNestedSchema),
		Indexes:     make(map[string]sourceIndex),
		Constraints: make(map[string]sourceConstraint),
		Metadata:    rootMeta.Metadata,
	}

	// ── Root Constraints ──────────────────────────────────────────────────────
	if cs.ResolvedConstraints != nil {
		for _, root := range cs.ResolvedConstraints.Roots {
			uuid, sc := serializeConstraint(cs, root)
			if uuid != "" {
				src.Constraints[uuid] = sc
			}
		}
	}

	// ── Root Fields ───────────────────────────────────────────────────────────
	for fd, fm := range rootMeta.Fields {
		src.Fields[fm.UUID] = serializeField(cs, 0, fd, fm)
	}

	// ── Root Indexes ──────────────────────────────────────────────────────────
	for uuid, idx := range rootMeta.Indexes {
		src.Indexes[uuid] = serializeIndex(idx)
	}

	// ── Nested Schemas ────────────────────────────────────────────────────────
	var indices []int
	for idx := range cs.Meta {
		if idx != 0 {
			indices = append(indices, int(idx))
		}
	}
	sort.Ints(indices)

	for _, idx := range indices {
		m := cs.Meta[uint8(idx)]
		nested := sourceNestedSchema{
			Name:        m.Name,
			Description: m.Description,
			Concrete:    m.Concrete,
			Values:      m.Values,
			Fields:      make(map[string]sourceField),
			Indexes:     make(map[string]sourceIndex),
			Metadata:    m.Metadata,
		}

		if m.Type != TypeUnknown {
			nested.Type = serializeFieldType(m.Type)
			if m.Type.IsSchemaBearing() {
				if m.Type == TypeUnion || m.Type == TypeComposite {
					var refs []sourceFieldRef
					for _, vIdx := range m.Variants {
						if vm := cs.Meta[vIdx]; vm != nil {
							refs = append(refs, sourceFieldRef{ID: vm.UUID})
						}
					}
					nested.Schema = refs
				} else if m.TargetSchema != 0 {
					if tm := cs.Meta[m.TargetSchema]; tm != nil {
						nested.Schema = sourceFieldRef{ID: tm.UUID}
					}
				}
			}
		}

		for fd, fm := range m.Fields {
			nested.Fields[fm.UUID] = serializeField(cs, uint8(idx), fd, fm)
		}

		for uuid, index := range m.Indexes {
			nested.Indexes[uuid] = serializeIndex(index)
		}

		src.Schemas[m.UUID] = nested
	}

	return json.MarshalIndent(src, "", "  ")
}

func serializeField(cs *Schema, schemaIdx uint8, fd uint32, fm FieldMeta) sourceField {
	ft := ExtractType(fd)
	sf := sourceField{
		Name:        fm.Name,
		Description: fm.Description,
		Type:        serializeFieldType(ft),
		Required:    IsRequired(fd),
		Unique:      IsUnique(fd),
		Deprecated:  IsDeprecated(fd),
	}

	// ── Resolve Schema ────────────────────────────────────────────────────────
	if ft.IsSchemaBearing() {
		if ft == TypeUnion || ft == TypeComposite {
			var refs []sourceFieldRef
			for _, vIdx := range cs.Variants[fd] {
				if vm := cs.Meta[vIdx]; vm != nil {
					refs = append(refs, sourceFieldRef{ID: vm.UUID})
				}
			}
			sf.Schema = refs
		} else {
			target := ExtractTargetSchema(fd)
			if target != 0 {
				if tm := cs.Meta[target]; tm != nil {
					sf.Schema = sourceFieldRef{ID: tm.UUID}
				}
			} else if ft == TypeEnum {
				if values := getEnumValuesFromStore(cs, fd); len(values) > 0 {
					sf.Schema = sourceFieldRef{Values: values}
				}
			}
		}
	}

	// ── Resolve Default ───────────────────────────────────────────────────────
	sf.Default = getDefaultFromStore(cs, fd, ft)

	return sf
}

func serializeFieldType(ft FieldTypeEnum) string {
	switch ft {
	case TypeString:
		return "string"
	case TypeNumber:
		return "number"
	case TypeInteger:
		return "integer"
	case TypeDecimal:
		return "decimal"
	case TypeBoolean:
		return "boolean"
	case TypeBytes:
		return "bytes"
	case TypeArray:
		return "array"
	case TypeSet:
		return "set"
	case TypeEnum:
		return "enum"
	case TypeObject:
		return "object"
	case TypeRecord:
		return "record"
	case TypeUnion:
		return "union"
	case TypeComposite:
		return "composite"
	case TypeGeometry:
		return "geometry"
	default:
		return "unknown"
	}
}

func serializeIndex(idx IndexDescriptor) sourceIndex {
	si := sourceIndex{
		Name:        idx.Name,
		Description: idx.Description,
		Type:        serializeIndexType(idx.Type),
		Order:       serializeIndexOrder(idx.Order),
		Unique:      idx.Unique,
		Fields:      idx.Fields,
	}
	if idx.Condition != nil {
		si.Condition = serializeIndexCondition(idx.Condition)
	}
	return si
}

func serializeIndexType(it IndexType) string {
	switch it {
	case IndexTypeNormal:
		return "normal"
	case IndexTypeUnique:
		return "unique"
	case IndexTypePrimary:
		return "primary"
	case IndexTypeSpatial:
		return "spatial"
	case IndexTypeFulltext:
		return "fulltext"
	default:
		return "normal"
	}
}

func serializeIndexOrder(io IndexOrder) string {
	switch io {
	case IndexOrderAsc:
		return "asc"
	case IndexOrderDesc:
		return "desc"
	default:
		return "asc"
	}
}

func serializeIndexCondition(cond IndexCondition) *sourceIndexCondition {
	switch c := cond.(type) {
	case IndexConditionLeaf:
		return &sourceIndexCondition{
			Field:    c.Field,
			Operator: serializeComparisonOperator(c.Operator),
			Value:    c.Value,
		}
	case IndexConditionGroup:
		sic := &sourceIndexCondition{
			Operator: serializeLogicalOperator(c.Operator),
		}
		for _, child := range c.Conditions {
			sic.Conditions = append(sic.Conditions, serializeIndexCondition(child))
		}
		return sic
	}
	return nil
}

func serializeConstraint(cs *Schema, node ResolvedConstraintNode) (string, sourceConstraint) {
	switch n := node.(type) {
	case ResolvedConstraint:
		return n.UUID, sourceConstraint{
			Name:        n.Name,
			Description: n.Description,
			Predicate:   n.PredicateName,
			Fields:      dataPointsToPaths(cs, n.Fields),
			Parameters:  n.Parameters,
		}
	case ResolvedConstraintGroup:
		sc := sourceConstraint{
			Name:        n.Name,
			Description: n.Description,
			Operator:    serializeLogicalOperator(n.Operator),
		}
		for _, child := range n.Constraints {
			_, childSc := serializeConstraint(cs, child)
			sc.Rules = append(sc.Rules, &childSc)
		}
		return n.UUID, sc
	}
	return "", sourceConstraint{}
}

func dataPointsToPaths(cs *Schema, dps []document.DocumentKey) []string {
	if len(dps) == 0 {
		return nil
	}
	paths := make([]string, len(dps))
	for i, dk := range dps {
		if path, ok := cs.PathCache.GetPath(dk); ok {
			paths[i] = path
		}
	}
	return paths
}

func serializeLogicalOperator(op LogicalOperator) string {
	switch op {
	case LogicalAnd:
		return "and"
	case LogicalOr:
		return "or"
	case LogicalNot:
		return "not"
	case LogicalNor:
		return "nor"
	case LogicalXor:
		return "xor"
	case LogicalNand:
		return "nand"
	case LogicalXnor:
		return "xnor"
	default:
		return "and"
	}
}

func serializeComparisonOperator(op ComparisonOperator) string {
	switch op {
	case ComparisonEq:
		return "eq"
	case ComparisonNeq:
		return "neq"
	case ComparisonLt:
		return "lt"
	case ComparisonLte:
		return "lte"
	case ComparisonGt:
		return "gt"
	case ComparisonGte:
		return "gte"
	case ComparisonIn:
		return "in"
	case ComparisonNin:
		return "nin"
	case ComparisonContains:
		return "contains"
	case ComparisonNcontains:
		return "ncontains"
	case ComparisonExists:
		return "exists"
	case ComparisonNexists:
		return "nexists"
	default:
		return "eq"
	}
}

func getEnumValuesFromStore(cs *Schema, fd uint32) []any {
	if cs.Store == nil {
		return nil
	}
	dkStr := descriptorToEnumDocumentKey(fd, document.TypeArrayString)
	if val, ok, _ := cs.Store.GetArrayString(dkStr); ok {
		res := make([]any, len(val))
		for i, x := range val {
			res[i] = x
		}
		return res
	}
	dkInt := descriptorToEnumDocumentKey(fd, document.TypeArrayInt)
	if val, ok, _ := cs.Store.GetArrayInt(dkInt); ok {
		res := make([]any, len(val))
		for i, x := range val {
			res[i] = x
		}
		return res
	}
	dkFlt := descriptorToEnumDocumentKey(fd, document.TypeArrayFloat)
	if val, ok, _ := cs.Store.GetArrayFloat(dkFlt); ok {
		res := make([]any, len(val))
		for i, x := range val {
			res[i] = x
		}
		return res
	}
	dkBool := descriptorToEnumDocumentKey(fd, document.TypeArrayBool)
	if val, ok, _ := cs.Store.GetArrayBool(dkBool); ok {
		res := make([]any, len(val))
		for i, x := range val {
			res[i] = x
		}
		return res
	}
	dkUnk := descriptorToEnumDocumentKey(fd, document.TypeArrayUnknown)
	if val, ok, _ := cs.Store.GetArrayUnknown(dkUnk); ok {
		return val
	}
	return nil
}

func getDefaultFromStore(cs *Schema, fd uint32, ft FieldTypeEnum) any {
	if cs.Store == nil {
		return nil
	}
	dt := fieldTypeToDataType(ft)
	dp, err := document.NewDataPoint(dt, int32((fd>>8)&0x7FFF))
	if err != nil {
		return nil
	}
	dk := document.NewDocumentKey(dp, fd)
	switch dt {
	case document.TypeString:
		if val, ok, _ := cs.Store.GetString(dk); ok {
			return val
		}
	case document.TypeInt:
		if val, ok, _ := cs.Store.GetInt(dk); ok {
			return val
		}
	case document.TypeFloat:
		if val, ok, _ := cs.Store.GetFloat(dk); ok {
			return val
		}
	case document.TypeBool:
		if val, ok, _ := cs.Store.GetBool(dk); ok {
			return val
		}
	case document.TypeBytes:
		if val, ok, _ := cs.Store.GetBytes(dk); ok {
			return val
		}
	case document.TypeGeometry:
		if val, ok, _ := cs.Store.GetGeometry(dk); ok {
			return val
		}
	case document.TypeRecord:
		if val, ok, _ := cs.Store.GetRecord(dk); ok {
			return val
		}
	case document.TypeArrayObject:
		if val, ok, _ := cs.Store.GetArrayObject(dk); ok {
			return val
		}
	case document.TypeUnknown:
		unknownDp, err := document.NewDataPoint(document.TypeUnknown, int32((fd>>8)&0x7FFF))
		if err != nil {
			return nil
		}
		unknownDk := document.NewDocumentKey(unknownDp, fd)
		if val, ok, _ := cs.Store.GetUnknown(unknownDk); ok {
			return val
		}
	}
	return nil
}

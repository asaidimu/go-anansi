package definition
// DeepCopy creates a deep copy of the schema
func (s *Schema) DeepCopy() *Schema {
	if s == nil {
		return nil
	}

	copy := &Schema{
		Version: s.Version,
		BaseSchema: BaseSchema{
			Name:        s.Name,
			Description: s.Description,
			Fields:      make(map[FieldId]Field, len(s.Fields)),
			Constraints: make(map[ConstraintId]Constraint, len(s.Constraints)),
			Indexes:     make(map[IndexId]Index, len(s.Indexes)),
			Metadata:    make(map[string]any, len(s.Metadata)),
		},
		Schemas: make(map[SchemaId]NestedSchema, len(s.Schemas)),
	}

	// Copy fields
	for id, field := range s.Fields {
		copy.Fields[id] = field.deepCopy()
	}

	// Copy nested schemas
	for id, schema := range s.Schemas {
		copy.Schemas[id] = schema.deepCopy()
	}

	// Copy constraints
	for id, constraint := range s.Constraints {
		copy.Constraints[id] = constraint.deepCopy()
	}

	// Copy indexes
	for id, index := range s.Indexes {
		copy.Indexes[id] = index.deepCopy()
	}

	// Deep copy metadata
	copy.Metadata = deepCopyMap(s.Metadata)

	return copy
}

// deepCopy creates a deep copy of a Field
func (f *Field) deepCopy() Field {
	copy := Field{
		Name:        f.Name,
		Description: f.Description,
		Required:    f.Required,
		Deprecated:  f.Deprecated,
		Unique:      f.Unique,
		FieldProperties: FieldProperties{
			Type:    f.Type,
			Default: f.Default.deepCopy(),
			Schema:  f.Schema.deepCopy(),
		},
	}
	return copy
}

// deepCopy creates a deep copy of a NestedSchema
func (ns *NestedSchema) deepCopy() NestedSchema {
	copy := NestedSchema{
		BaseSchema: BaseSchema{
			Name:        ns.Name,
			Description: ns.Description,
			Fields:      make(map[FieldId]Field, len(ns.Fields)),
			Constraints: make(map[ConstraintId]Constraint, len(ns.Constraints)),
			Indexes:     make(map[IndexId]Index, len(ns.Indexes)),
			Metadata:    make(map[string]any, len(ns.Metadata)),
		},
		FieldProperties: FieldProperties{
			Type:    ns.Type,
			Default: ns.Default.deepCopy(),
			Schema:  ns.Schema.deepCopy(),
		},
		Concrete: ns.Concrete,
		Values:   make([]LiteralValue, len(ns.Values)),
	}

	// Copy fields
	for id, field := range ns.Fields {
		copy.Fields[id] = field.deepCopy()
	}

	// Copy constraints
	for id, constraint := range ns.Constraints {
		copy.Constraints[id] = constraint.deepCopy()
	}

	// Copy indexes
	for id, index := range ns.Indexes {
		copy.Indexes[id] = index.deepCopy()
	}

	// Copy metadata
	copy.Metadata = deepCopyMap(ns.Metadata)

	// Copy values
	for i, val := range ns.Values {
		copy.Values[i] = val.deepCopy()
	}

	return copy
}

// deepCopy creates a deep copy of a Constraint
func (c *Constraint) deepCopy() Constraint {
	copy := Constraint{
		Name:        c.Name,
		Description: c.Description,
		ConstraintUnion: ConstraintUnion{
			kind:        c.kind,
		},
	}

	switch c.kind {
	case ConstraintKindRule:
		if rule, err := ConstraintAs[*ConstraintRule](c.ConstraintUnion); err == nil {
			copy.ConstraintUnion = NewConstrainUnion(rule.deepCopy())
		}
	case ConstraintKindGroup:
		if group, err := ConstraintAs[*ConstraintGroup](c.ConstraintUnion); err == nil {
			copy.ConstraintUnion = NewConstrainUnion(group.deepCopy())
		}
	}

	return copy
}

// deepCopy creates a deep copy of a ConstraintRule
func (cr *ConstraintRule) deepCopy() *ConstraintRule {
	copy := &ConstraintRule{
		Predicate:  cr.Predicate,
		Parameters: cr.Parameters.deepCopy(),
		Fields:     make([]FieldName, len(cr.Fields)),
	}

	for i, field := range cr.Fields {
		copy.Fields[i] = field
	}

	return copy
}

// deepCopy creates a deep copy of a ConstraintGroup
func (cg *ConstraintGroup) deepCopy() *ConstraintGroup {
	copy := &ConstraintGroup{
		Operator: cg.Operator,
		Rules:    make([]ConstraintUnion, len(cg.Rules)),
	}

	for i, ruleUnion := range cg.Rules {
		switch ruleUnion.kind {
		case ConstraintKindRule:
			if rule, ok := ConstraintAs[*ConstraintRule](ruleUnion); ok == nil {
				copy.Rules[i] = NewConstrainUnion(rule.deepCopy())
			}
		case ConstraintKindGroup:
			if group, ok := ConstraintAs[*ConstraintGroup](ruleUnion); ok == nil {
				copy.Rules[i] = NewConstrainUnion(group.deepCopy())
			}
		}
	}

	return copy
}

// deepCopy creates a deep copy of an Index
func (idx *Index) deepCopy() Index {
	var copiedFields []FieldId
	if idx.Fields != nil {
		copiedFields = make([]FieldId, len(idx.Fields))
		for i, field := range idx.Fields {
			copiedFields[i] = field
		}
	}

	copy := Index{
		Name:        idx.Name,
		Description: idx.Description,
		Type:        idx.Type,
		Fields:      copiedFields, // Use the conditionally created slice
		Order:       idx.Order,
		Unique:      idx.Unique,
		Condition:   idx.Condition.deepCopy(),
	}

	return copy
}

// deepCopy creates a deep copy of an IndexConditionUnion
func (icu IndexConditionUnion) deepCopy() IndexConditionUnion {
	switch icu.kind {
	case IndexConditionKindSingle:
		if cond, ok := IndexConditionAs[*IndexCondition](icu); ok == nil {
			return NewIndexConditionUnion(cond.deepCopy())
		}
	case IndexConditionKindGroup:
		if group, ok := IndexConditionAs[*IndexConditionGroup](icu); ok == nil {
			return NewIndexConditionUnion(group.deepCopy())
		}
	}
	return IndexConditionUnion{}
}

// deepCopy creates a deep copy of an IndexCondition
func (ic *IndexCondition) deepCopy() *IndexCondition {
	return &IndexCondition{
		Field:    ic.Field,
		Operator: ic.Operator,
		Value:    ic.Value.deepCopy(),
	}
}

// deepCopy creates a deep copy of an IndexConditionGroup
func (icg *IndexConditionGroup) deepCopy() *IndexConditionGroup {
	copy := &IndexConditionGroup{
		Operator:   icg.Operator,
		Conditions: make([]IndexConditionUnion, len(icg.Conditions)),
	}

	for i, condUnion := range icg.Conditions {
		copy.Conditions[i] = condUnion.deepCopy()
	}

	return copy
}

// deepCopy creates a deep copy of a FieldSchemaReference
func (fsr FieldSchemaReference) deepCopy() FieldSchemaReference {
	if fsr.IsZero() {
		return FieldSchemaReference{}
	}

	if fsr.IsSingle() {
		if ref, ok := FieldSchemaAs[SchemaReference](fsr); ok == nil {
			return NewSchemaReference(ref.deepCopy())
		}
	} else if fsr.IsMultiple() {
		if refs, ok := FieldSchemaAs[[]SchemaReference](fsr); ok == nil {
			copyRefs := make([]SchemaReference, len(refs))
			for i, ref := range refs {
				copyRefs[i] = ref.deepCopy()
			}
			return NewSchemaReference(copyRefs)
		}
	}

	return FieldSchemaReference{}
}

// deepCopy creates a deep copy of a SchemaReference
func (sr SchemaReference) deepCopy() SchemaReference {
	copy := SchemaReference{
		ID:          sr.ID,
		Constraints: make(map[ConstraintId]Constraint, len(sr.Constraints)),
		Indexes:     make(map[IndexId]Index, len(sr.Indexes)),
	}

	for id, constraint := range sr.Constraints {
		copy.Constraints[id] = constraint.deepCopy()
	}

	for id, index := range sr.Indexes {
		copy.Indexes[id] = index.deepCopy()
	}

	return copy
}

// deepCopy creates a deep copy of a LiteralValue
func (lv LiteralValue) deepCopy() LiteralValue {
	if lv.IsZero() || lv.IsNull() {
		return lv
	}

	val := lv.Value()
	return mustNewLiteralValue(deepCopyValue(val))
}

// deepCopyValue recursively copies any value
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		return deepCopySlice(val)
	case string, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, bool:
		// Primitive types are copied by value
		return val
	default:
		// For other types, attempt to return as-is
		// This may need extension based on LiteralValue's actual supported types
		return val
	}
}

// deepCopyMap creates a deep copy of a map[string]any
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	copy := make(map[string]any, len(m))
	for k, v := range m {
		copy[k] = deepCopyValue(v)
	}
	return copy
}

// deepCopySlice creates a deep copy of a []any
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}

	copy := make([]any, len(s))
	for i, v := range s {
		copy[i] = deepCopyValue(v)
	}
	return copy
}


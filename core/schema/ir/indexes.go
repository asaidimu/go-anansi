package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// indexes.go implements Pass 10: resolve every index across all schemas into
// its hot ResolvedIndex form. Field path strings are resolved to DataPoints
// via cs.Address(). Index ordinals are assigned and written back into
// SchemaMetadata.IndexOrdinals.
//
// Pass 10 runs after the address space is built so cs.Address() is available.

// buildResolvedIndexes populates CompiledSchema.ResolvedIndexes and fills
// SchemaMetadata.IndexOrdinals for every schema that has indexes.
func buildResolvedIndexes(
	cs *CompiledSchema,
	si *schemaIndex,
) (map[uint16]ResolvedIndex, []CompileError) {
	resolved := make(map[uint16]ResolvedIndex)
	var errs []CompileError

	rootErrs := resolveSchemaIndexes(cs, 0, cs.Meta[0], resolved)
	errs = append(errs, rootErrs...)

	for _, uuid := range si.order {
		schemaIdx := si.byUUID[uuid]
		m := cs.Meta[schemaIdx]
		if m == nil || len(m.Indexes) == 0 {
			continue
		}
		idxErrs := resolveSchemaIndexes(cs, schemaIdx, m, resolved)
		for i := range idxErrs {
			idxErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, idxErrs...)
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return resolved, nil
}

func resolveSchemaIndexes(
	cs *CompiledSchema,
	schemaIdx uint8,
	m *SchemaMetadata,
	resolved map[uint16]ResolvedIndex,
) []CompileError {
	if m == nil || len(m.Indexes) == 0 {
		return nil
	}

	var errs []CompileError
	uuids := sortedKeys(m.Indexes)

	for ordinal, uuid := range uuids {
		cold := m.Indexes[uuid]
		m.IndexOrdinals[uuid] = uint8(ordinal)
		key := uint16(schemaIdx)<<8 | uint16(ordinal)

		ri, idxErrs := resolveIndex(cs, cold)
		for i := range idxErrs {
			idxErrs[i].Message = "index " + cold.Name + ": " + idxErrs[i].Message
		}
		errs = append(errs, idxErrs...)
		resolved[key] = ri
	}

	return errs
}

func resolveIndex(cs *CompiledSchema, cold IndexDescriptor) (ResolvedIndex, []CompileError) {
	var errs []CompileError

	fields := make([]document.DataPoint, 0, len(cold.Fields))
	for _, path := range cold.Fields {
		dp, err := cs.Address(path)
		if err != nil {
			errs = append(errs, CompileError{
				Pass:    PassIndexes,
				Message: "cannot resolve field path " + path + ": " + err.Error(),
			})
			continue
		}
		fields = append(fields, dp)
	}

	var condition ResolvedCondition
	if cold.Condition != nil {
		var condErrs []CompileError
		condition, condErrs = resolveIndexCondition(cs, cold.Condition)
		errs = append(errs, condErrs...)
	}

	return ResolvedIndex{
		Type:      cold.Type,
		Order:     cold.Order,
		Unique:    cold.Unique,
		Fields:    fields,
		Condition: condition,
	}, errs
}

func resolveIndexCondition(cs *CompiledSchema, cond IndexCondition) (ResolvedCondition, []CompileError) {
	if cond == nil {
		return nil, nil
	}

	switch c := cond.(type) {
	case IndexConditionLeaf:
		dp, err := cs.Address(c.Field)
		if err != nil {
			return nil, []CompileError{{
				Pass:    PassIndexes,
				Message: "cannot resolve condition field " + c.Field + ": " + err.Error(),
			}}
		}
		return ResolvedConditionLeaf{
			Field:    dp,
			Operator: c.Operator,
			Value:    c.Value,
		}, nil

	case IndexConditionGroup:
		group := ResolvedConditionGroup{Operator: c.Operator}
		var errs []CompileError
		for _, child := range c.Conditions {
			rc, childErrs := resolveIndexCondition(cs, child)
			errs = append(errs, childErrs...)
			group.Conditions = append(group.Conditions, rc)
		}
		return group, errs

	default:
		return nil, []CompileError{{
			Pass:    PassIndexes,
			Message: "unknown IndexCondition type",
		}}
	}
}

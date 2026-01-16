package definition

// getMapAtPath navigates to the map at the given path, creating maps as needed
func getMapAtPath(root map[string]any, path []string) map[string]any {
	current := root
	for i := 1; i < len(path); i++ { // Skip first element which is "schema"
		segment := path[i]
		if next, ok := current[segment].(map[string]any); ok {
			current = next
		} else {
			// Create the map if it doesn't exist
			newMap := make(map[string]any)
			current[segment] = newMap
			current = newMap
		}
	}
	return current
}

// AsMap converts the schema to a map[string]any representation
// This avoids json.Marshal allocations while maintaining the same structure
func (s *Schema) AsMap() map[string]any {
	result := make(map[string]any)

	_, _ = s.Walk(result, func(acc any, ctx *NodeContext) (any, error) {
		root := acc.(map[string]any)

		switch ctx.Type {
		case NodeTypeSchema:
			schema := ctx.Value.(*Schema)
			root["version"] = schema.Version.String()
			root["name"] = schema.Name
			if schema.Description != "" {
				root["description"] = schema.Description
			}

		case NodeTypeFieldsMap:
			// Boundary marker - fields map will be created as we add fields
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			parent["fields"] = make(map[string]any)

		case NodeTypeField:
			field := ctx.Value.(*Field)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			fieldMap := make(map[string]any)
			fieldMap["name"] = string(field.Name)
			if field.Description != "" {
				fieldMap["description"] = field.Description
			}
			if field.Required {
				fieldMap["required"] = true
			}
			if field.Deprecated {
				fieldMap["deprecated"] = true
			}
			if field.Unique {
				fieldMap["unique"] = true
			}
			if field.Type != 0 {
				fieldMap["type"] = field.Type.String()
			}

			parent[ctx.Key] = fieldMap

		case NodeTypeSchemasMap:
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			parent["schemas"] = make(map[string]any)

		case NodeTypeNestedSchema:
			ns := ctx.Value.(*NestedSchema)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			schemaMap := make(map[string]any)
			schemaMap["name"] = ns.Name
			if ns.Description != "" {
				schemaMap["description"] = ns.Description
			}
			if ns.Concrete {
				schemaMap["concrete"] = true
			}
			if ns.Type != 0 {
				schemaMap["type"] = ns.Type.String()
			}

			parent[ctx.Key] = schemaMap

		case NodeTypeConstraintsMap:
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			parent["constraints"] = make(map[string]any)

		case NodeTypeConstraint:
			constraint := ctx.Value.(*Constraint)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			constraintMap := make(map[string]any)
			constraintMap["name"] = constraint.Name
			if constraint.Description != "" {
				constraintMap["description"] = constraint.Description
			}

			parent[ctx.Key] = constraintMap

		case NodeTypeConstraintRule:
			rule := ctx.Value.(*ConstraintRule)
			parentConstraintMap := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1]) // This is the map for the parent Constraint

			ruleMap := make(map[string]any)
			ruleMap["predicate"] = string(rule.Predicate)
			if len(rule.Fields) > 0 {
				fields := make([]string, len(rule.Fields))
				for i, f := range rule.Fields {
					fields[i] = string(f)
				}
				ruleMap["fields"] = fields
			}
			// Parameters will be added by NodeTypeLiteralValue to this ruleMap

			parentConstraintMap[ctx.GetPath()[ctx.PathLen-1]] = ruleMap

		case NodeTypeConstraintGroup:
			group := ctx.Value.(*ConstraintGroup)
			current := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			current["operator"] = group.Operator.String()
			// Rules will be added as we traverse them
			current["rules"] = make([]map[string]any, 0)

		case NodeTypeIndexesMap:
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			parent["indexes"] = make(map[string]any)

		case NodeTypeIndex:
			index := ctx.Value.(*Index)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			indexMap := make(map[string]any)
			indexMap["name"] = index.Name
			if index.Description != "" {
				indexMap["description"] = index.Description
			}
			indexMap["type"] = index.Type.String()
			if len(index.Fields) > 0 {
				fields := make([]string, len(index.Fields))
				for i, f := range index.Fields {
					fields[i] = string(f)
				}
				indexMap["fields"] = fields
			}
			if index.Order != "" {
				indexMap["order"] = index.Order
			}
			if index.Unique {
				indexMap["unique"] = true
			}

			parent[ctx.Key] = indexMap

		case NodeTypeIndexCondition:
			condition := ctx.Value.(*IndexCondition)
			current := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			condMap := make(map[string]any)
			condMap["field"] = string(condition.Field)
			condMap["operator"] = condition.Operator.String()
			if !condition.Value.IsZero() && !condition.Value.IsNull() {
				condMap["value"] = condition.Value.Value()
			}

			current["condition"] = condMap

		case NodeTypeIndexConditionGroup:
			group := ctx.Value.(*IndexConditionGroup)
			current := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			groupMap := make(map[string]any)
			groupMap["operator"] = group.Operator.String()
			groupMap["conditions"] = make([]map[string]any, 0)

			current["condition"] = groupMap

		case NodeTypeFieldSchema:
			ref := ctx.Value.(FieldSchemaReference)
			current := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])

			if ref.IsSingle() {
				sr, _ := FieldSchemaAs[SchemaReference](ref)
				refMap := make(map[string]any)
				refMap["id"] = string(sr.ID)
				if len(sr.Indexes) > 0 {
					// TODO: Properly serialize indexes
					refMap["indexes"] = make(map[string]any)
				}
				if len(sr.Constraints) > 0 {
					// TODO: Properly serialize constraints
					refMap["constraints"] = make(map[string]any)
				}
				current["schema"] = refMap
			} else if ref.IsMultiple() {
				refs, _ := FieldSchemaAs[[]SchemaReference](ref)
				refsArray := make([]map[string]any, len(refs))
				for i, sr := range refs {
					refMap := make(map[string]any)
					refMap["id"] = string(sr.ID)
					refsArray[i] = refMap
				}
				current["schema"] = refsArray
			}

		case NodeTypeValuesArray:
			values := ctx.Value.([]LiteralValue)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			valuesArray := make([]any, len(values))
			for i, lv := range values {
				valuesArray[i] = lv.Value()
			}
			parent[ctx.Key] = valuesArray

		case NodeTypeConstraintParameters:
			params := ctx.Value.(LiteralValue)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			if !params.IsZero() && !params.IsNull() {
				parent[ctx.Key] = params.Value()
			}

		case NodeTypeConstraintFields:
			fields := ctx.Value.([]FieldName)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			stringFields := make([]string, len(fields))
			for i, f := range fields {
				stringFields[i] = string(f)
			}
			parent[ctx.Key] = stringFields

		case NodeTypeIndexFields:
			fields := ctx.Value.([]FieldId)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			stringFields := make([]string, len(fields))
			for i, f := range fields {
				stringFields[i] = string(f)
			}
			parent[ctx.Key] = stringFields

		case NodeTypeFieldDefault:
			def := ctx.Value.(LiteralValue)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			if !def.IsZero() && !def.IsNull() {
				parent[ctx.Key] = def.Value()
			}

		case NodeTypeLiteralValue:
			lv := ctx.Value.(LiteralValue)
			if !lv.IsZero() && !lv.IsNull() {
				current := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
				current[ctx.Key] = lv.Value()
			}


		case NodeTypeMetadataMap:
			metadata := ctx.Value.(map[string]any)
			parent := getMapAtPath(root, ctx.GetPath()[:ctx.PathLen-1])
			parent["metadata"] = metadata
		}

		return root, nil
	})

	return result
}

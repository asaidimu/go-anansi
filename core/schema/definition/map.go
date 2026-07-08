package definition

// stackItem tracks the container and depth to mimic JSON scoping
type stackItem struct {
	depth int
	val   any // map[string]any or *[]any
	onPop func(any)
}

// mapBuilder holds the state for constructing the map during traversal
type mapBuilder struct {
	root  map[string]any
	stack []stackItem
}

func (mb *mapBuilder) push(depth int, val any, onPop func(any)) {
	mb.stack = append(mb.stack, stackItem{depth, val, onPop})
}

// popTo cleans up contexts that are deeper than or equal to the current depth
func (mb *mapBuilder) popTo(depth int) {
	for len(mb.stack) > 0 && mb.stack[len(mb.stack)-1].depth >= depth {
		item := mb.stack[len(mb.stack)-1]
		mb.stack = mb.stack[:len(mb.stack)-1]
		// If there is a callback (e.g., assigning an array back to its parent), execute it
		if item.onPop != nil {
			item.onPop(item.val)
		}
	}
}

func (mb *mapBuilder) current() any {
	if len(mb.stack) == 0 {
		return nil
	}
	return mb.stack[len(mb.stack)-1].val
}

func (mb *mapBuilder) currentMap() map[string]any {
	if m, ok := mb.current().(map[string]any); ok {
		return m
	}
	return nil
}

// AsMap converts the schema to a map[string]any matching the exact structure of ToJSON
func (s *Schema) AsMap() map[string]any {
	mb := &mapBuilder{
		stack: make([]stackItem, 0, 32),
	}

	_, _ = s.Walk(mb, func(acc any, ctx *NodeContext) (any, error) {
		builder := acc.(*mapBuilder)

		// Pop contexts that are shallower than current depth
		builder.popTo(ctx.Depth)

		switch ctx.Type {
		case NodeTypeSchema:
			schema := ctx.Value.(*Schema)
			root := make(map[string]any)
			if schema.Version != nil {
				root["version"] = schema.Version.String()
			}
			root["name"] = schema.Name
			if schema.Description != "" {
				root["description"] = schema.Description
			}
			builder.root = root
			builder.push(ctx.Depth, root, nil)

		case NodeTypeFieldsMap:
			m := make(map[string]any)
			builder.currentMap()["fields"] = m
			builder.push(ctx.Depth, m, nil)

		case NodeTypeSchemasMap:
			m := make(map[string]any)
			builder.currentMap()["schemas"] = m
			builder.push(ctx.Depth, m, nil)

		case NodeTypeConstraintsMap:
			m := make(map[string]any)
			builder.currentMap()["constraints"] = m
			builder.push(ctx.Depth, m, nil)

		case NodeTypeIndexesMap:
			m := make(map[string]any)
			builder.currentMap()["indexes"] = m
			builder.push(ctx.Depth, m, nil)

		case NodeTypeField:
			field := ctx.Value.(*Field)
			m := make(map[string]any)
			builder.currentMap()[ctx.Key] = m
			builder.push(ctx.Depth, m, nil)

			m["name"] = string(field.Name)
			if field.Description != "" {
				m["description"] = field.Description
			}
			if field.Required {
				m["required"] = true
			}
			if field.Deprecated {
				m["deprecated"] = true
			}
			if field.Unique {
				m["unique"] = true
			}
			if field.Type != 0 {
				m["type"] = field.Type.String()
			}

		case NodeTypeNestedSchema:
			ns := ctx.Value.(*NestedSchema)
			m := make(map[string]any)
			builder.currentMap()[ctx.Key] = m
			builder.push(ctx.Depth, m, nil)

			m["name"] = ns.Name
			if ns.Description != "" {
				m["description"] = ns.Description
			}
			if ns.Concrete {
				m["concrete"] = true
			}
			if ns.Type != 0 {
				m["type"] = ns.Type.String()
			}
			if !ns.Schema.IsZero() {
				if ns.Schema.IsSingle() {
					sr, _ := FieldSchemaAs[SchemaReference](ns.Schema)
					if sr.IsInline() {
						refMap := map[string]any{"type": sr.Type.String()}
						if len(sr.Values) > 0 {
							vals := make([]any, len(sr.Values))
							for i, lv := range sr.Values {
								vals[i] = lv.Value()
							}
							refMap["values"] = vals
						}
						m["schema"] = refMap
					} else {
						m["schema"] = map[string]any{"id": string(sr.ID)}
					}
				} else if ns.Schema.IsMultiple() {
					refs, _ := FieldSchemaAs[[]SchemaReference](ns.Schema)
					arr := make([]any, len(refs))
					for i, sr := range refs {
						arr[i] = map[string]any{"id": string(sr.ID)}
					}
					m["schema"] = arr
				}
			}
		case NodeTypeConstraint:
			constraint := ctx.Value.(*Constraint)
			m := make(map[string]any)
			builder.currentMap()[ctx.Key] = m
			builder.push(ctx.Depth, m, nil)

			m["name"] = constraint.Name
			if constraint.Description != "" {
				m["description"] = constraint.Description
			}

		case NodeTypeConstraintRule:
			rule := ctx.Value.(*ConstraintRule)
			curr := builder.current()
			var targetMap map[string]any

			if arr, ok := curr.(*[]any); ok {
				// We are nested inside a ConstraintGroup Rules array
				targetMap = make(map[string]any)
				*arr = append(*arr, targetMap)
				builder.push(ctx.Depth, targetMap, nil)
			} else if m, ok := curr.(map[string]any); ok {
				// We are directly under a top-level Constraint (Flattening)
				targetMap = m
			}

			targetMap["predicate"] = string(rule.Predicate)

		case NodeTypeConstraintGroup:
			group := ctx.Value.(*ConstraintGroup)
			curr := builder.current()
			var targetMap map[string]any

			if arr, ok := curr.(*[]any); ok {
				// Nested group inside an array
				targetMap = make(map[string]any)
				*arr = append(*arr, targetMap)
				builder.push(ctx.Depth, targetMap, nil)
			} else if m, ok := curr.(map[string]any); ok {
				targetMap = m
			}

			targetMap["operator"] = group.Operator.String()
			rulesArr := make([]any, 0)

			// Push array pointer so children can append, assign to map when done
			builder.push(ctx.Depth, &rulesArr, func(v any) {
				targetMap["rules"] = *(v.(*[]any))
			})

		case NodeTypeIndex:
			index := ctx.Value.(*Index)
			m := make(map[string]any)
			builder.currentMap()[ctx.Key] = m
			builder.push(ctx.Depth, m, nil)

			m["name"] = index.Name
			if index.Description != "" {
				m["description"] = index.Description
			}
			m["type"] = index.Type.String()
			if index.Order != "" {
				m["order"] = index.Order
			}
			if index.Unique {
				m["unique"] = true
			}

		case NodeTypeIndexCondition:
			condition := ctx.Value.(*IndexCondition)
			curr := builder.current()
			var targetMap map[string]any

			if arr, ok := curr.(*[]any); ok {
				targetMap = make(map[string]any)
				*arr = append(*arr, targetMap)
				builder.push(ctx.Depth, targetMap, nil)
			} else if m, ok := curr.(map[string]any); ok {
				targetMap = make(map[string]any)
				m["condition"] = targetMap
				builder.push(ctx.Depth, targetMap, nil)
			}

			targetMap["field"] = string(condition.Field)
			targetMap["operator"] = condition.Operator.String()

		case NodeTypeIndexConditionGroup:
			group := ctx.Value.(*IndexConditionGroup)
			curr := builder.current()
			var targetMap map[string]any

			if arr, ok := curr.(*[]any); ok {
				targetMap = make(map[string]any)
				*arr = append(*arr, targetMap)
				builder.push(ctx.Depth, targetMap, nil)
			} else if m, ok := curr.(map[string]any); ok {
				targetMap = make(map[string]any)
				m["condition"] = targetMap
				builder.push(ctx.Depth, targetMap, nil)
			}

			targetMap["operator"] = group.Operator.String()
			condArr := make([]any, 0)

			builder.push(ctx.Depth, &condArr, func(v any) {
				targetMap["conditions"] = *(v.(*[]any))
			})

		case NodeTypeFieldSchema:
			ref := ctx.Value.(FieldSchemaReference)
			m := builder.currentMap()
			if ref.IsSingle() {
				sr, _ := FieldSchemaAs[SchemaReference](ref)
				if sr.IsInline() {
					refMap := map[string]any{"type": sr.Type.String()}
					if len(sr.Values) > 0 {
						vals := make([]any, len(sr.Values))
						for i, lv := range sr.Values {
							vals[i] = lv.Value()
						}
						refMap["values"] = vals
					}
					m["schema"] = refMap
				} else {
					m["schema"] = map[string]any{"id": string(sr.ID)}
				}
			} else if ref.IsMultiple() {
				refs, _ := FieldSchemaAs[[]SchemaReference](ref)
				arr := make([]any, len(refs))
				for i, sr := range refs {
					arr[i] = map[string]any{"id": string(sr.ID)}
				}
				m["schema"] = arr
			}

		case NodeTypeValuesArray:
			values := ctx.Value.([]LiteralValue)
			arr := make([]any, len(values))
			for i, lv := range values {
				arr[i] = lv.Value()
			}
			builder.currentMap()[ctx.Key] = arr

		case NodeTypeConstraintParameters:
			params := ctx.Value.(LiteralValue)
			if !params.IsZero() && !params.IsNull() {
				builder.currentMap()[ctx.Key] = params.Value()
			}

		case NodeTypeConstraintFields:
			fields := ctx.Value.([]FieldName)
			arr := make([]any, len(fields))
			for i, f := range fields {
				arr[i] = string(f)
			}
			builder.currentMap()[ctx.Key] = arr

		case NodeTypeIndexFields:
			fields := ctx.Value.([]FieldName)
			arr := make([]any, len(fields))
			for i, f := range fields {
				arr[i] = string(f)
			}
			builder.currentMap()[ctx.Key] = arr

		case NodeTypeFieldDefault:
			def := ctx.Value.(LiteralValue)
			if !def.IsZero() && !def.IsNull() {
				builder.currentMap()[ctx.Key] = def.Value()
			}

		case NodeTypeLiteralValue:
			lv := ctx.Value.(LiteralValue)
			if !lv.IsZero() && !lv.IsNull() {
				builder.currentMap()[ctx.Key] = lv.Value()
			}

		case NodeTypeMetadataMap:
			metadata := ctx.Value.(map[string]any)
			builder.currentMap()["metadata"] = metadata
		}

		return builder, nil
	})

	// Clean up and execute any final onPop callbacks
	mb.popTo(0)

	return mb.root
}

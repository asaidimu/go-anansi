package definition

import "strings"

// Skeleton holds the pre-built structure and navigation cache
type Skeleton struct {
	root  map[string]any
	cache map[string]any // path -> location for O(1) navigation
}

// buildSkeleton creates the complete map structure from the index
func buildSkeleton(index *SchemaIndex) *Skeleton {
	skeleton := &Skeleton{
		root:  make(map[string]any),
		cache: make(map[string]any, len(index.nodes)),
	}

	// Collect all paths and sort by depth to ensure parents are created first
	type pathDepth struct {
		path  string
		depth int
	}

	paths := make([]pathDepth, 0, len(index.nodes))
	for path := range index.nodes {
		depth := strings.Count(path, "/")
		paths = append(paths, pathDepth{path, depth})
	}

	// Simple bubble sort by depth
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if paths[j].depth < paths[i].depth {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	// Build structure in dependency order
	for _, pd := range paths {
		path := pd.path
		info := index.nodes[path]

		if path == "schema" {
			// Root is already created
			skeleton.cache[path] = skeleton.root
			continue
		}

		// Get parent location
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash == -1 {
			continue
		}

		parentPath := path[:lastSlash]
		parent, ok := skeleton.cache[parentPath]
		if !ok {
			// Parent doesn't exist yet, skip (shouldn't happen with sorted order)
			continue
		}

		// Get the key for this node (last segment of path)
		key := path[lastSlash+1:]

		// Create the appropriate container
		var container any
		switch info.kind {
		case KindMap:
			container = make(map[string]any, info.capacity)
			// Cache map locations for O(1) lookup
			skeleton.cache[path] = container
		case KindArray:
			container = make([]any, 0, info.capacity)
			// Arrays are not cached since we don't navigate into them
		case KindValue:
			// Value nodes are placeholders, will be assigned during population
			container = nil
		}

		// Insert into parent
		if parentMap, ok := parent.(map[string]any); ok {
			parentMap[key] = container
		}
	}

	return skeleton
}

// getLocation returns the map at the given path using the cache
func (sk *Skeleton) getLocation(path []string) map[string]any {
	pathKey := pathToString(path)
	if loc, ok := sk.cache[pathKey]; ok {
		return loc.(map[string]any)
	}
	return nil
}

// pathToString converts a path slice to a string key
func pathToString(path []string) string {
	return strings.Join(path, "/")
}

// AsMap converts the schema to a map[string]any
func (s *Schema) AsMap() map[string]any {
	// Phase 1: Build index
	index := s.BuildIndex()

	// Phase 2: Build skeleton
	skeleton := buildSkeleton(index)

	// Phase 3: Populate values
	_, _ = s.Walk(skeleton, func(acc any, ctx *NodeContext) (any, error) {
		sk := acc.(*Skeleton)

		switch ctx.Type {
		case NodeTypeSchema:
			schema := ctx.Value.(*Schema)
			root := sk.root
			// TODO Re-think this method, as we can see it is unsafe
			if schema.Version != nil {
				root["version"] = schema.Version.String()
			}
			root["name"] = schema.Name
			if schema.Description != "" {
				root["description"] = schema.Description
			}

		case NodeTypeField:
			field := ctx.Value.(*Field)
			fieldMap := sk.getLocation(ctx.GetPath())
			if fieldMap == nil {
				return sk, nil
			}

			// Populate field properties directly into the pre-created map
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

		case NodeTypeNestedSchema:
			ns := ctx.Value.(*NestedSchema)
			schemaMap := sk.getLocation(ctx.GetPath())
			if schemaMap == nil {
				return sk, nil
			}

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

		case NodeTypeConstraint:
			constraint := ctx.Value.(*Constraint)
			constraintMap := sk.getLocation(ctx.GetPath())
			if constraintMap == nil {
				return sk, nil
			}

			constraintMap["name"] = constraint.Name
			if constraint.Description != "" {
				constraintMap["description"] = constraint.Description
			}

		case NodeTypeConstraintRule:
			rule := ctx.Value.(*ConstraintRule)
			ruleMap := sk.getLocation(ctx.GetPath())
			if ruleMap == nil {
				return sk, nil
			}

			ruleMap["predicate"] = string(rule.Predicate)

		case NodeTypeConstraintGroup:
			group := ctx.Value.(*ConstraintGroup)
			groupMap := sk.getLocation(ctx.GetPath())
			if groupMap == nil {
				return sk, nil
			}

			groupMap["operator"] = group.Operator.String()

		case NodeTypeIndex:
			index := ctx.Value.(*Index)
			indexMap := sk.getLocation(ctx.GetPath())
			if indexMap == nil {
				return sk, nil
			}

			indexMap["name"] = index.Name
			if index.Description != "" {
				indexMap["description"] = index.Description
			}
			indexMap["type"] = index.Type.String()
			if index.Order != "" {
				indexMap["order"] = index.Order
			}
			if index.Unique {
				indexMap["unique"] = true
			}

		case NodeTypeIndexCondition:
			condition := ctx.Value.(*IndexCondition)
			condMap := sk.getLocation(ctx.GetPath())
			if condMap == nil {
				return sk, nil
			}

			condMap["field"] = string(condition.Field)
			condMap["operator"] = condition.Operator.String()
			if !condition.Value.IsZero() && !condition.Value.IsNull() {
				condMap["value"] = condition.Value.Value()
			}

		case NodeTypeIndexConditionGroup:
			group := ctx.Value.(*IndexConditionGroup)
			groupMap := sk.getLocation(ctx.GetPath())
			if groupMap == nil {
				return sk, nil
			}

			groupMap["operator"] = group.Operator.String()

		case NodeTypeFieldSchema:
			ref := ctx.Value.(FieldSchemaReference)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}

			if ref.IsSingle() {
				sr, _ := FieldSchemaAs[SchemaReference](ref)
				// Get the pre-created schema map
				refMap := sk.getLocation(ctx.GetPath())
				if refMap != nil {
					refMap["id"] = string(sr.ID)
					if len(sr.Indexes) > 0 {
						refMap["indexes"] = make(map[string]any)
					}
					if len(sr.Constraints) > 0 {
						refMap["constraints"] = make(map[string]any)
					}
				}
			} else if ref.IsMultiple() {
				// Multiple references - create array inline
				refs, _ := FieldSchemaAs[[]SchemaReference](ref)
				refsArray := make([]map[string]any, len(refs))
				for i, sr := range refs {
					refMap := make(map[string]any)
					refMap["id"] = string(sr.ID)
					refsArray[i] = refMap
				}
				parent["schema"] = refsArray
			}

		case NodeTypeValuesArray:
			values := ctx.Value.([]LiteralValue)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}
			valuesArray := make([]any, len(values))
			for i, lv := range values {
				valuesArray[i] = lv.Value()
			}
			parent[ctx.Key] = valuesArray

		case NodeTypeConstraintParameters:
			params := ctx.Value.(LiteralValue)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}
			if !params.IsZero() && !params.IsNull() {
				parent[ctx.Key] = params.Value()
			}

		case NodeTypeConstraintFields:
			fields := ctx.Value.([]FieldName)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}
			stringFields := make([]string, len(fields))
			for i, f := range fields {
				stringFields[i] = string(f)
			}
			parent[ctx.Key] = stringFields

		case NodeTypeIndexFields:
			fields := ctx.Value.([]FieldId)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}
			stringFields := make([]string, len(fields))
			for i, f := range fields {
				stringFields[i] = string(f)
			}
			parent[ctx.Key] = stringFields

		case NodeTypeFieldDefault:
			def := ctx.Value.(LiteralValue)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent == nil {
				return sk, nil
			}
			if !def.IsZero() && !def.IsNull() {
				parent[ctx.Key] = def.Value()
			}

		case NodeTypeLiteralValue:
			lv := ctx.Value.(LiteralValue)
			if !lv.IsZero() && !lv.IsNull() {
				parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
				if parent != nil {
					parent[ctx.Key] = lv.Value()
				}
			}

		case NodeTypeMetadataMap:
			metadata := ctx.Value.(map[string]any)
			parent := sk.getLocation(ctx.GetPath()[:ctx.PathLen-1])
			if parent != nil {
				parent["metadata"] = metadata
			}

		// Boundary nodes don't need handling - structure already exists
		case NodeTypeFieldsMap, NodeTypeSchemasMap, NodeTypeConstraintsMap, NodeTypeIndexesMap:
			// Skip - structure already pre-created in skeleton
		}

		return sk, nil
	})

	return skeleton.root
}

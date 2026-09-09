package definition

// NodeKind represents the type of container or value at a path
type NodeKind byte

const (
	KindMap NodeKind = iota + 1
	KindArray
	KindValue
)

// NodeInfo describes a node in the schema tree
type NodeInfo struct {
	kind     NodeKind
	capacity int
	children []string
}

// SchemaIndex contains structural information about the schema tree
type SchemaIndex struct {
	depth int
	nodes map[string]*NodeInfo
}

// BuildIndex analyzes the schema tree and builds an index
// It only indexes paths that the walker actually visits
func (s *Schema) BuildIndex() *SchemaIndex {
	index := &SchemaIndex{
		depth: 0,
		nodes: make(map[string]*NodeInfo),
	}

	// Track siblings to count children for capacity
	childCounts := make(map[string]int)

	_, _ = s.Walk(index, func(acc any, ctx *NodeContext) (any, error) {
		idx := acc.(*SchemaIndex)

		// Update max depth
		if ctx.Depth > idx.depth {
			idx.depth = ctx.Depth
		}

		pathKey := pathToString(ctx.GetPath())
		parentPathKey := ""
		if ctx.PathLen > 1 {
			parentPathKey = pathToString(ctx.GetPath()[:ctx.PathLen-1])
		}

		// Track that parent has this child
		if parentPathKey != "" {
			childCounts[parentPathKey]++
		}

		switch ctx.Type {
		case NodeTypeSchema:
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeFieldsMap, NodeTypeSchemasMap, NodeTypeConstraintsMap, NodeTypeIndexesMap, NodeTypeMetadataMap:
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeField:
			// Field is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeNestedSchema:
			// Nested schema is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeConstraint:
			// Constraint is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeConstraintRule:
			// Rule is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeConstraintGroup:
			// Group is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeIndex:
			// Index is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeIndexCondition:
			// Condition is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeIndexConditionGroup:
			// Condition group is a map container
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindMap,
			}

		case NodeTypeFieldSchema:
			// Schema reference can be single map or array of maps
			ref := ctx.Value.(FieldSchemaReference)
			if ref.IsSingle() {
				idx.nodes[pathKey] = &NodeInfo{
					kind: KindMap,
				}
			} else if ref.IsMultiple() {
				refs, _ := FieldSchemaAs[[]SchemaReference](ref)
				idx.nodes[pathKey] = &NodeInfo{
					kind:     KindArray,
					capacity: len(refs),
				}
			}

		case NodeTypeValuesArray:
			values := ctx.Value.([]LiteralValue)
			idx.nodes[pathKey] = &NodeInfo{
				kind:     KindArray,
				capacity: len(values),
			}

		case NodeTypeConstraintParameters:
			// Parameters is a value node (will be assigned directly)
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindValue,
			}

		case NodeTypeConstraintFields:
			fields := ctx.Value.([]FieldName)
			idx.nodes[pathKey] = &NodeInfo{
				kind:     KindArray,
				capacity: len(fields),
			}

		case NodeTypeIndexFields:
			fields := ctx.Value.([]FieldName)
			idx.nodes[pathKey] = &NodeInfo{
				kind:     KindArray,
				capacity: len(fields),
			}

		case NodeTypeFieldDefault, NodeTypeLiteralValue:
			// Default/literal values are value nodes
			idx.nodes[pathKey] = &NodeInfo{
				kind: KindValue,
			}
		}

		return idx, nil
	})

	// Update capacities based on child counts
	for path, count := range childCounts {
		if info, exists := index.nodes[path]; exists && info.kind == KindMap {
			info.capacity = count
		}
	}

	return index
}


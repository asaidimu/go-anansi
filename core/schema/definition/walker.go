package definition

import (
	"sync"
)

const (
	// Most schemas won't exceed this depth
	typicalMaxDepth = 100
)

// NodeType represents the type of node being visited
type NodeType byte

const (
	NodeTypeSchema NodeType = iota + 1
	NodeTypeFieldsMap
	NodeTypeField
	NodeTypeSchemasMap
	NodeTypeNestedSchema
	NodeTypeConstraintsMap
	NodeTypeConstraint
	NodeTypeConstraintRule
	NodeTypeConstraintGroup
	NodeTypeIndexesMap
	NodeTypeIndex
	NodeTypeIndexCondition
	NodeTypeIndexConditionGroup
	NodeTypeLiteralValue
	NodeTypeFieldSchema
	NodeTypeMetadataMap
	NodeTypeValuesArray
	NodeTypeConstraintParameters
	NodeTypeConstraintFields
	NodeTypeIndexFields
	NodeTypeFieldDefault
)

// NodeContext carries information about the current node being visited
type NodeContext struct {
	Type    NodeType
	Value   any
	Key     string                    // Map key if this node is in a map
	Path    [typicalMaxDepth]string   // Fixed array to avoid allocations
	PathLen int                       // Actual length used
	Parent  *NodeContext
	Depth   int
}

// pushPath adds a segment to the path
func (ctx *NodeContext) pushPath(segment string) {
	if ctx.PathLen < len(ctx.Path) {
		ctx.Path[ctx.PathLen] = segment
		ctx.PathLen++
	}
}

// GetPath returns the current path as a slice
func (ctx *NodeContext) GetPath() []string {
	return ctx.Path[:ctx.PathLen]
}

// Context pools by depth for efficient reuse
var contextPools = [typicalMaxDepth]*sync.Pool{}


func acquireContext(depth int) *NodeContext {
	if depth >= typicalMaxDepth {
		// Rare case - allocate on heap
		return &NodeContext{
			Depth: depth,
			Path:  [typicalMaxDepth]string{},
		}
	}
	ctx := contextPools[depth].Get().(*NodeContext)
	ctx.Depth = depth
	return ctx
}

func releaseContext(ctx *NodeContext) {
	if ctx.Depth < typicalMaxDepth {
		ctx.Type = 0
		ctx.Value = nil
		ctx.Key = ""
		ctx.PathLen = 0
		ctx.Parent = nil
		contextPools[ctx.Depth].Put(ctx)
	}
}

// Walk traverses the entire schema tree, calling the walker function for each node
func (s *Schema) Walk(
	accumulator any,
	walker func(acc any, ctx *NodeContext) (any, error),
) (any, error) {
	ctx := acquireContext(0)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeSchema
	ctx.Value = s
	ctx.pushPath("schema")

	acc, err := walker(accumulator, ctx)
	if err != nil {
		return acc, err
	}

	// Walk fields map
	if len(s.Fields) > 0 {
		acc, err = s.walkFieldsMap(acc, walker, ctx, s.Fields)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schemas map
	if len(s.Schemas) > 0 {
		acc, err = s.walkSchemasMap(acc, walker, ctx, s.Schemas)
		if err != nil {
			return acc, err
		}
	}

	// Walk constraints map
	if len(s.Constraints) > 0 {
		acc, err = s.walkConstraintsMap(acc, walker, ctx, s.Constraints)
		if err != nil {
			return acc, err
		}
	}

	// Walk indexes map
	if len(s.Indexes) > 0 {
		acc, err = s.walkIndexesMap(acc, walker, ctx, s.Indexes)
		if err != nil {
			return acc, err
		}
	}

	// Walk metadata map
	if len(s.Metadata) > 0 {
		acc, err = s.walkMetadataMap(acc, walker, ctx, s.Metadata)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkFieldsMap visits the fields map boundary and then each field
func (s *Schema) walkFieldsMap(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	fields map[FieldId]Field,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeFieldsMap
	ctx.Value = fields
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("fields")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Visit each field
	for id, field := range fields {
		acc, err = s.walkField(acc, walker, ctx, string(id), &field)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkField visits a single field and its nested structures
func (s *Schema) walkField(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	field *Field,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeField
	ctx.Value = field
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk field's default value
	if !field.Default.IsZero() && !field.Default.IsNull() {
		acc, err = s.walkFieldDefault(acc, walker, ctx, "default", field.Default)
		if err != nil {
			return acc, err
		}
	}

	// Walk field's schema reference
	if !field.Schema.IsZero() {
		acc, err = s.walkFieldSchemaReference(acc, walker, ctx, field.Schema)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkSchemasMap visits the schemas map boundary and then each nested schema
func (s *Schema) walkSchemasMap(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	schemas map[SchemaId]NestedSchema,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeSchemasMap
	ctx.Value = schemas
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("schemas")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Visit each nested schema
	for id, schema := range schemas {
		acc, err = s.walkNestedSchema(acc, walker, ctx, string(id), &schema)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkNestedSchema visits a nested schema and its contents
func (s *Schema) walkNestedSchema(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	schema *NestedSchema,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeNestedSchema
	ctx.Value = schema
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk nested schema's fields
	if len(schema.Fields) > 0 {
		acc, err = s.walkFieldsMap(acc, walker, ctx, schema.Fields)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schema's constraints
	if len(schema.Constraints) > 0 {
		acc, err = s.walkConstraintsMap(acc, walker, ctx, schema.Constraints)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schema's indexes
	if len(schema.Indexes) > 0 {
		acc, err = s.walkIndexesMap(acc, walker, ctx, schema.Indexes)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schema's metadata
	if len(schema.Metadata) > 0 {
		acc, err = s.walkMetadataMap(acc, walker, ctx, schema.Metadata)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schema's default value
	if !schema.Default.IsZero() && !schema.Default.IsNull() {
		acc, err = s.walkLiteralValue(acc, walker, ctx, "default", schema.Default)
		if err != nil {
			return acc, err
		}
	}

	// Walk nested schema's values (for enums)
	if len(schema.Values) > 0 {
		acc, err = s.walkValuesArray(acc, walker, ctx, "values", schema.Values)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkConstraintsMap visits the constraints map boundary and then each constraint
func (s *Schema) walkConstraintsMap(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	constraints map[ConstraintId]Constraint,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraintsMap
	ctx.Value = constraints
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("constraints")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Visit each constraint
	for id, constraint := range constraints {
		acc, err = s.walkConstraint(acc, walker, ctx, string(id), &constraint)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkConstraint visits a constraint and its union contents
func (s *Schema) walkConstraint(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	constraint *Constraint,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraint
	ctx.Value = constraint
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk the constraint union
	switch constraint.kind {
	case ConstraintKindRule:
		rule, _ := ConstraintAs[*ConstraintRule](constraint.ConstraintUnion)
		acc, err = s.walkConstraintRule(acc, walker, ctx, rule)
		if err != nil {
			return acc, err
		}
	case ConstraintKindGroup:
		group, _ := ConstraintAs[*ConstraintGroup](constraint.ConstraintUnion)
		acc, err = s.walkConstraintGroup(acc, walker, ctx, group)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkConstraintRule visits a constraint rule
func (s *Schema) walkConstraintRule(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	rule *ConstraintRule,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraintRule
	ctx.Value = rule
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("rule")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk parameters if present
	if !rule.Parameters.IsZero() && !rule.Parameters.IsNull() {
		acc, err = s.walkConstraintParameters(acc, walker, ctx, "parameters", rule.Parameters)
		if err != nil {
			return acc, err
		}
	}

	// Walk fields if present
	if len(rule.Fields) > 0 {
		acc, err = s.walkConstraintFields(acc, walker, ctx, "fields", rule.Fields)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkConstraintGroup visits a constraint group and its nested rules
func (s *Schema) walkConstraintGroup(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	group *ConstraintGroup,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraintGroup
	ctx.Value = group
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("group")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk each rule in the group
	for i, ruleUnion := range group.Rules {
		switch ruleUnion.kind {
		case ConstraintKindRule:
			rule, _ := ConstraintAs[*ConstraintRule](ruleUnion)
			acc, err = s.walkConstraintRule(acc, walker, ctx, rule)
			if err != nil {
				return acc, err
			}
		case ConstraintKindGroup:
			nestedGroup, _ := ConstraintAs[*ConstraintGroup](ruleUnion)
			acc, err = s.walkConstraintGroup(acc, walker, ctx, nestedGroup)
			if err != nil {
				return acc, err
			}
		}
		_ = i // Could use for indexing if needed
	}

	return acc, nil
}

// walkIndexesMap visits the indexes map boundary and then each index
func (s *Schema) walkIndexesMap(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	indexes map[IndexId]Index,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeIndexesMap
	ctx.Value = indexes
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("indexes")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Visit each index
	for id, index := range indexes {
		acc, err = s.walkIndex(acc, walker, ctx, string(id), &index)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkIndex visits an index and its condition
func (s *Schema) walkIndex(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	index *Index,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeIndex
	ctx.Value = index
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk index fields
	if len(index.Fields) > 0 {
		acc, err = s.walkIndexFields(acc, walker, ctx, "fields", index.Fields)
		if err != nil {
			return acc, err
		}
	}

	// Walk index condition if present
	if !index.Condition.IsZero() {
		switch index.Condition.kind {
		case IndexConditionKindSingle:
			cond, _ := IndexConditionAs[*IndexCondition](index.Condition)
			acc, err = s.walkIndexCondition(acc, walker, ctx, cond)
			if err != nil {
				return acc, err
			}
		case IndexConditionKindGroup:
			group, _ := IndexConditionAs[*IndexConditionGroup](index.Condition)
			acc, err = s.walkIndexConditionGroup(acc, walker, ctx, group)
			if err != nil {
				return acc, err
			}
		}
	}

	return acc, nil
}

// walkIndexCondition visits a single index condition
func (s *Schema) walkIndexCondition(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	condition *IndexCondition,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeIndexCondition
	ctx.Value = condition
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("condition")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk condition value
	if !condition.Value.IsZero() && !condition.Value.IsNull() {
		acc, err = s.walkLiteralValue(acc, walker, ctx, "value", condition.Value)
		if err != nil {
			return acc, err
		}
	}

	return acc, nil
}

// walkIndexConditionGroup visits an index condition group
func (s *Schema) walkIndexConditionGroup(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	group *IndexConditionGroup,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeIndexConditionGroup
	ctx.Value = group
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("conditionGroup")

	acc, err := walker(acc, ctx)
	if err != nil {
		return acc, err
	}

	// Walk each condition in the group
	for i, condUnion := range group.Conditions {
		switch condUnion.kind {
		case IndexConditionKindSingle:
			cond, _ := IndexConditionAs[*IndexCondition](condUnion)
			acc, err = s.walkIndexCondition(acc, walker, ctx, cond)
			if err != nil {
				return acc, err
			}
		case IndexConditionKindGroup:
			nestedGroup, _ := IndexConditionAs[*IndexConditionGroup](condUnion)
			acc, err = s.walkIndexConditionGroup(acc, walker, ctx, nestedGroup)
			if err != nil {
				return acc, err
			}
		}
		_ = i // Could use for indexing if needed
	}

	return acc, nil
}

// walkLiteralValue visits a literal value
func (s *Schema) walkLiteralValue(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	value LiteralValue,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeLiteralValue
	ctx.Value = value
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

// walkFieldSchemaReference visits a field schema reference
func (s *Schema) walkFieldSchemaReference(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	ref FieldSchemaReference,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeFieldSchema
	ctx.Value = ref
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("schema")

	return walker(acc, ctx)
}

// walkMetadataMap visits the metadata map
func (s *Schema) walkMetadataMap(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	metadata map[string]any,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeMetadataMap
	ctx.Value = metadata
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath("metadata")

	return walker(acc, ctx)
}

// walkValuesArray visits a values array
func (s *Schema) walkValuesArray(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	values []LiteralValue,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeValuesArray
	ctx.Value = values
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

// walkConstraintParameters visits constraint parameters
func (s *Schema) walkConstraintParameters(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	parameters LiteralValue,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraintParameters
	ctx.Value = parameters
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

// walkConstraintFields visits constraint fields
func (s *Schema) walkConstraintFields(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	fields []FieldName,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeConstraintFields
	ctx.Value = fields
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

// walkIndexFields visits index fields
func (s *Schema) walkIndexFields(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	fields []FieldId,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeIndexFields
	ctx.Value = fields
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

// walkFieldDefault visits a field's default value
func (s *Schema) walkFieldDefault(
	acc any,
	walker func(any, *NodeContext) (any, error),
	parent *NodeContext,
	key string,
	defaultValue LiteralValue,
) (any, error) {
	ctx := acquireContext(parent.Depth + 1)
	defer releaseContext(ctx)

	ctx.Type = NodeTypeFieldDefault
	ctx.Value = defaultValue
	ctx.Key = key
	ctx.Parent = parent

	copy(ctx.Path[:], parent.Path[:parent.PathLen])
	ctx.PathLen = parent.PathLen
	ctx.pushPath(key)

	return walker(acc, ctx)
}

func init() {
	for i := range typicalMaxDepth {
		contextPools[i] = &sync.Pool{
			New: func() any {
				return &NodeContext{
					Path: [typicalMaxDepth]string{},
				}
			},
		}
	}
}

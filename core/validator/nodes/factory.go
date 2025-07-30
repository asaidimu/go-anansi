package nodes

import (
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// NodeFactory defines an interface for creating various validation nodes
// This allows for centralizing node creation and potentially swapping
// out different node implementations or adding new ones easily.
type NodeFactory interface {
	NewUnexpectedFieldsNode(path string, expectedFields map[string]bool) *UnexpectedFieldsNode
	NewRequiredFieldNode(path string, deps []string, fieldName string) *RequiredFieldNode
	NewTypeCheckNode(path string, deps []string, fieldDef *schema.FieldDefinition) *TypeCheckNode
	NewEnumValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition) *EnumValidationNode
	NewArrayValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *ArrayValidationNode
	NewRecordValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *RecordValidationNode
	NewSetValidationNode(path string, deps []string) *SetValidationNode
	NewNestedSchemaNode(path, suffix string, deps []string) *NestedSchemaNode
	NewUnionValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *UnionValidationNode
	NewConstraintNode(path, id string, deps []string, constraint schema.Constraint[schema.FieldType]) *ConstraintNode
	NewConstraintGroupNode(path string, group schema.ConstraintGroup[schema.FieldType], deps []string) *ConstraintGroupNode
	NewConditionalFieldNode(path string, deps []string, condition *schema.FieldInclusionCondition, fieldName string) *ConditionalFieldNode
	NewConditionalUnexpectedFieldsNode(path string, conditionalFields map[string]*schema.FieldInclusionCondition, baseFields map[string]bool) *ConditionalUnexpectedFieldsNode
	NewConditionalRequiredFieldNode(path string, deps []string, fieldName string, condition *schema.FieldInclusionCondition) *ConditionalRequiredFieldNode
}

// defaultNodeFactory is the default implementation of NodeFactory.
type defaultNodeFactory struct {
	fmap    *schema.FunctionMap
	factory types.ValidatorFactory
}

// NewDefaultNodeFactory creates a new defaultNodeFactory.
func NewDefaultNodeFactory(fmap *schema.FunctionMap, factory types.ValidatorFactory) NodeFactory {
	return &defaultNodeFactory{
		fmap:    fmap,
		factory: factory,
	}
}

// newBaseNode creates a baseNode with the specified generic type
func (f *defaultNodeFactory) newBaseNode(path, nodeType, suffix string, fieldType *schema.FieldType, deps []string) BaseNode[any] {
	return NewBaseNode[any](path, nodeType, suffix, fieldType, deps, f.factory, *f.fmap)
}

// NewUnexpectedFieldsNode creates an UnexpectedFieldsNode.
func (f *defaultNodeFactory) NewUnexpectedFieldsNode(path string, expectedFields map[string]bool) *UnexpectedFieldsNode {
	base := f.newBaseNode(path, "unexpected_fields", "", nil, nil)
	node := &UnexpectedFieldsNode{
		BaseNode:       BaseNode[map[string]any](base),
		expectedFields: expectedFields,
	}
	return node
}

// NewRequiredFieldNode creates a RequiredFieldNode.
func (f *defaultNodeFactory) NewRequiredFieldNode(path string, deps []string, fieldName string) *RequiredFieldNode {
	base := f.newBaseNode(path, "required_field", fieldName, nil, deps)
	node := &RequiredFieldNode{
		BaseNode:  BaseNode[map[string]any](base),
		fieldName: fieldName,
	}
	return node
}

// NewTypeCheckNode creates a TypeCheckNode.
func (f *defaultNodeFactory) NewTypeCheckNode(path string, deps []string, fieldDef *schema.FieldDefinition) *TypeCheckNode {
	base := f.newBaseNode(path, "type_check", fieldDef.Name, &fieldDef.Type, deps)
	node := &TypeCheckNode{
		BaseNode: base,
		FieldDef: fieldDef,
	}
	return node
}

// NewEnumValidationNode creates an EnumValidationNode.
func (f *defaultNodeFactory) NewEnumValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition) *EnumValidationNode {
	base := f.newBaseNode(path, "enum_validation", fieldDef.Name, &fieldDef.Type, deps)
	node := &EnumValidationNode{
		BaseNode: base,
		fieldDef: fieldDef,
	}
	return node
}

// NewArrayValidationNode creates an ArrayValidationNode.
func (f *defaultNodeFactory) NewArrayValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *ArrayValidationNode {
	arrayType := schema.FieldTypeArray
	base := f.newBaseNode(path, "array_validation", fieldDef.Name, &arrayType, deps)
	node := &ArrayValidationNode{
		BaseNode:         BaseNode[[]any](base),
		fieldDef:         fieldDef,
		schema:           sc,
	}
	return node
}

// NewRecordValidationNode creates a RecordValidationNode.
func (f *defaultNodeFactory) NewRecordValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *RecordValidationNode {
	recordType := schema.FieldTypeRecord
	base := f.newBaseNode(path, "record_validation", fieldDef.Name, &recordType, deps)
	node := &RecordValidationNode{
		BaseNode:         BaseNode[map[string]any](base),
		fieldDef:         fieldDef,
		schema:           sc,
		issues:           []types.Issue{},
	}
	return node
}

// NewSetValidationNode creates a SetValidationNode.
func (f *defaultNodeFactory) NewSetValidationNode(path string, deps []string) *SetValidationNode {
	setType := schema.FieldTypeSet
	base := f.newBaseNode(path, "set_validation", "", &setType, deps)
	node := &SetValidationNode{
		BaseNode: BaseNode[[]any](base),
	}
	return node
}

// NewNestedSchemaNode creates a NestedSchemaNode.
func (f *defaultNodeFactory) NewNestedSchemaNode(path, suffix string, deps []string) *NestedSchemaNode {
	base := f.newBaseNode(path, "nested_schema", suffix, nil, deps)
	node := &NestedSchemaNode{
		BaseNode: base,
	}
	return node
}

// NewUnionValidationNode creates a UnionValidationNode.
func (f *defaultNodeFactory) NewUnionValidationNode(path string, deps []string, fieldDef *schema.FieldDefinition, sc *schema.SchemaDefinition) *UnionValidationNode {
	unionType := schema.FieldTypeUnion
	base := f.newBaseNode(path, "union_validation", fieldDef.Name, &unionType, deps)
	node := &UnionValidationNode{
		BaseNode:         base,
		fieldDef:         fieldDef,
		schema:           sc,
	}
	return node
}

// NewConstraintNode creates a ConstraintNode.
func (f *defaultNodeFactory) NewConstraintNode(path, id string, deps []string, constraint schema.Constraint[schema.FieldType]) *ConstraintNode {
	base := f.newBaseNode(path, "constraint", constraint.Name, nil, deps)
	node := &ConstraintNode{
		BaseNode:   base,
		constraint: constraint,
		fmap:       f.fmap,
	}
	// Override ID with the provided 'id' to ensure uniqueness based on constraint.Name
	node.BaseNode.id = id
	return node
}

// NewConstraintGroupNode creates a ConstraintGroupNode.
func (f *defaultNodeFactory) NewConstraintGroupNode(path string, group schema.ConstraintGroup[schema.FieldType], deps []string) *ConstraintGroupNode {
	base := f.newBaseNode(path, "constraint_group", group.Name, nil, deps)
	node := &ConstraintGroupNode{
		BaseNode:  base,
		group:     group,
		memberIDs: deps, // The dependencies are the member constraint IDs
	}
	return node
}

// NewConditionalFieldNode creates a ConditionalFieldNode.
func (f *defaultNodeFactory) NewConditionalFieldNode(path string, deps []string, condition *schema.FieldInclusionCondition, fieldName string) *ConditionalFieldNode {
	base := f.newBaseNode(path, "conditional_field", fieldName, nil, deps)
	node := &ConditionalFieldNode{
		BaseNode:  BaseNode[map[string]any](base),
		condition: condition,
		fieldName: fieldName,
	}
	return node
}

// NewConditionalUnexpectedFieldsNode creates a ConditionalUnexpectedFieldsNode.
func (f *defaultNodeFactory) NewConditionalUnexpectedFieldsNode(path string, conditionalFields map[string]*schema.FieldInclusionCondition, baseFields map[string]bool) *ConditionalUnexpectedFieldsNode {
	base := f.newBaseNode(path, "conditional_unexpected_fields", "", nil, nil)
	node := &ConditionalUnexpectedFieldsNode{
		BaseNode:          BaseNode[map[string]any](base),
		conditionalFields: conditionalFields,
		baseFields:        baseFields,
	}
	return node
}

// NewConditionalRequiredFieldNode creates a ConditionalRequiredFieldNode.
func (f *defaultNodeFactory) NewConditionalRequiredFieldNode(path string, deps []string, fieldName string, condition *schema.FieldInclusionCondition) *ConditionalRequiredFieldNode {
	base := f.newBaseNode(path, "conditional_required_field", fieldName, nil, deps)
	node := &ConditionalRequiredFieldNode{
		BaseNode:  BaseNode[map[string]any](base),
		fieldName: fieldName,
		condition: condition,
	}
	return node
}

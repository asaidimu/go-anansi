package schema

import (
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// --- Core Interfaces and Structs from ValidatorLogic.md ---

type DocumentValidator struct {
	fmap  *FunctionMap
	graph *ValidationGraph
}

// ValidationNode represents a single validation operation in the graph.
type ValidationNode interface {
	Execute(ctx *ValidationContext) *NodeResult
	GetDependencies() []string
	GetID() string
	GetPath() string
}

// NodeResult holds the outcome of a single node's execution.
type NodeResult struct {
	Success bool
	Value   any
	Issues  []Issue
}

// ValidationContext holds the state during a validation traversal.
type ValidationContext struct {
	RootData    any
	Data        any
	Results     map[string]*NodeResult
	FunctionMap *FunctionMap
}

// ValidationGraph represents the compiled set of validation operations.
type ValidationGraph struct {
	nodes        map[string]ValidationNode
	dependencies map[string][]string
}

// --- GraphBuilder for Phase 1: Construction ---

const (
	dfsUnvisited = 0
	dfsVisiting  = 1
	dfsVisited   = 2
)

type graphBuilder struct {
	graph        *ValidationGraph
	fmap         *FunctionMap
	visitedState map[string]int // For cycle detection during graph building
}

// --- Specific Node Type Implementations ---

// BaseNode provides common fields for all nodes.
type baseNode struct {
	id   string
	path string
	deps []string
}

func (n *baseNode) GetID() string             { return n.id }
func (n *baseNode) GetPath() string           { return n.path }
func (n *baseNode) GetDependencies() []string { return n.deps }

// UnexpectedFieldsNode checks for fields not defined in the schema.
type UnexpectedFieldsNode struct {
	baseNode
	expectedFields map[string]bool
}

// RequiredFieldNode checks for the presence of a required field.
type RequiredFieldNode struct {
	baseNode
	fieldName string
}

// TypeCheckNode validates and coerces the type of a field.
type TypeCheckNode struct {
	baseNode
	fieldDef *FieldDefinition
}

// EnumValidationNode validates a value against a list of allowed enum values.
type EnumValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
}

// ArrayValidationNode validates each item in an array.
type ArrayValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

// RecordValidationNode validates each value in a record map against a schema.
type RecordValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

// SetValidationNode validates uniqueness in a set.
type SetValidationNode struct {
	baseNode
}

// NestedSchemaNode triggers validation of a nested schema.
type NestedSchemaNode struct {
	baseNode
	// This node is a marker; its dependencies trigger the actual nested validation.
}

// UnionValidationNode validates a value against one of several possible schemas.
type UnionValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

// ConstraintNode executes a single predicate function.
type ConstraintNode struct {
	baseNode
	constraint Constraint[FieldType]
	fmap       *FunctionMap
}

// ConstraintGroupNode evaluates a logical group of constraints.
type ConstraintGroupNode struct {
	baseNode
	group     ConstraintGroup[FieldType]
	memberIDs []string
}

type ConditionalFieldNode struct {
	baseNode
	condition *FieldInclusionCondition
	fieldName string
}

// Add this new node type for conditional unexpected fields checking
type ConditionalUnexpectedFieldsNode struct {
	baseNode
	conditionalFields map[string]*FieldInclusionCondition // field -> condition
	baseFields        map[string]bool                     // unconditional fields
}

type ConditionalRequiredFieldNode struct {
	baseNode
	fieldName string
	condition *FieldInclusionCondition
}
// --- Phase 1: Graph Construction ---

func (b *graphBuilder) dfsCheck(nodeID string) bool {
	b.visitedState[nodeID] = dfsVisiting

	for _, depID := range b.graph.dependencies[nodeID] {
		if b.visitedState[depID] == dfsVisiting {
			return true // Cycle detected
		}
		if b.visitedState[depID] == dfsUnvisited {
			if b.dfsCheck(depID) {
				return true // Cycle detected in recursive call
			}
		}
	}

	b.visitedState[nodeID] = dfsVisited
	return false
}

// NewDocumentValidator creates a new Validator instance by building the validation graph.
// It also performs cycle detection to ensure the graph is a DAG.
func NewDocumentValidator(schema *SchemaDefinition, fmap *FunctionMap) (*DocumentValidator, error) {
	// First, validate schema semantics before building the graph
	if err := validateSchemaSematics(schema); err != nil {
		return nil, fmt.Errorf("schema semantic validation failed: %w", err)
	}

	builder := newGraphBuilder(fmap)
	addedConstraints := make(map[string]bool)
	builder.buildFromSchema(schema, "", nil, addedConstraints, nil)

	// Perform cycle detection after graph construction
	for nodeID := range builder.graph.nodes {
		if builder.visitedState[nodeID] == dfsUnvisited {
			if builder.dfsCheck(nodeID) {
				return nil, fmt.Errorf("circular dependency detected in validation graph involving node: %s", nodeID)
			}
		}
	}

	return &DocumentValidator{
		graph: builder.graph,
		fmap:  fmap,
	}, nil
}

// Validate schema semantics before graph construction
func validateSchemaSematics(schema *SchemaDefinition) error {
	return validateSchemaSemanticRecursive(schema, "")
}

func validateSchemaSemanticRecursive(schema *SchemaDefinition, basePath string) error {
	for fieldName, fieldDef := range schema.Fields {
		fieldPath := buildPath(basePath, fieldName)

		if err := validateFieldSemantic(fieldDef, schema, fieldPath); err != nil {
			return err
		}

		// Recursively validate nested schemas
		if err := validateNestedSchemaSemantics(fieldDef, schema, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldSemantic(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	// Validate direct schema references
	if fieldDef.Schema != nil {
		switch fieldDef.Type {
		case FieldTypeObject:
			if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
				if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
					if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
						return fmt.Errorf("object field '%s' cannot reference literal nested schema '%s' - only structured schemas are allowed",
							fieldPath, ref.ID)
					}
				} else {
					return fmt.Errorf("object field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
				}
			}

		case FieldTypeRecord:
			if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
				if _, exists := schema.FindNestedSchema(ref.ID); !exists {
					return fmt.Errorf("record field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
				}
				// Both structured and literal schemas are valid for records
			}

		case FieldTypeUnion:
			if refs, ok := fieldDef.Schema.([]NestedSchemaReference); ok {
				for _, ref := range refs {
					if _, exists := schema.FindNestedSchema(ref.ID); !exists {
						return fmt.Errorf("union field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
					}
					// Both structured and literal schemas are valid for unions
				}
			}

		case FieldTypeArray, FieldTypeSet:
			// Array/Set fields CAN have schema references when ItemsType is complex
			// The schema applies to the items, not the container
			if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
				if fieldDef.ItemsType == nil {
					return fmt.Errorf("array/set field '%s' has schema reference but no ItemsType specified", fieldPath)
				}

				if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
					switch *fieldDef.ItemsType {
					case FieldTypeObject:
						// Object ItemsType REQUIRES structured schema
						if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
							return fmt.Errorf("array/set field '%s' with object ItemsType cannot reference literal nested schema '%s' - only structured schemas are allowed",
								fieldPath, ref.ID)
						}
					case FieldTypeRecord, FieldTypeUnion:
						// Record and Union ItemsType can reference both structured and literal schemas
						// No additional validation needed
					case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeBoolean, FieldTypeDecimal, FieldTypeEnum:
						// Primitive ItemsType should not have schema references
						return fmt.Errorf("array/set field '%s' with primitive ItemsType '%s' cannot have schema references",
							fieldPath, *fieldDef.ItemsType)
					}
				} else {
					return fmt.Errorf("array/set field '%s' references unknown nested schema '%s'", fieldPath, ref.ID)
				}
			}

		case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeBoolean, FieldTypeDecimal, FieldTypeEnum:
			return fmt.Errorf("primitive field type '%s' at '%s' cannot have schema references", fieldDef.Type, fieldPath)
		}
	}

	// Validate ItemsType schema references for arrays and sets
	if fieldDef.ItemsType != nil && (fieldDef.Type == FieldTypeArray || fieldDef.Type == FieldTypeSet) {
		// This would need to be implemented to validate ItemsType schemas recursively
		// The logic would be similar but applied to the ItemsType context
	}

	return nil
}

func validateNestedSchemaSemantics(fieldDef *FieldDefinition, schema *SchemaDefinition, fieldPath string) error {
	// Recursively validate any nested schema definitions referenced by this field
	// This ensures deep semantic validation of the entire schema tree

	if fieldDef.Schema != nil {
		switch fieldDef.Type {
		case FieldTypeObject:
			if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
				if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
					if nestedSchemaDef.IsStructured != nil && *nestedSchemaDef.IsStructured {
						// Recursively validate the structured nested schema
						var tempSchema *SchemaDefinition
						if nestedSchemaDef.StructuredFieldsMap != nil {
							tempSchema = &SchemaDefinition{
								Name:          nestedSchemaDef.Name,
								Fields:        nestedSchemaDef.StructuredFieldsMap,
								NestedSchemas: schema.NestedSchemas,
							}
						}
						if tempSchema != nil {
							if err := validateSchemaSemanticRecursive(tempSchema, fieldPath); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func newGraphBuilder(fmap *FunctionMap) *graphBuilder {
	return &graphBuilder{
		graph: &ValidationGraph{
			nodes:        make(map[string]ValidationNode),
			dependencies: make(map[string][]string),
		},
		fmap:         fmap,
		visitedState: make(map[string]int),
	}
}

func (b *graphBuilder) addNode(node ValidationNode) {
	nodeID := node.GetID()
	if _, exists := b.graph.nodes[nodeID]; exists {
		return
	}
	b.graph.nodes[nodeID] = node
	b.graph.dependencies[nodeID] = node.GetDependencies()
}

func buildNodeID(path, nodeType, suffix string) string {
	id := fmt.Sprintf("%s:%s", path, nodeType)
	if path == "" {
		id = nodeType
	}
	if suffix != "" {
		id = fmt.Sprintf("%s:%s", id, suffix)
	}
	return id
}

func (b *graphBuilder) buildFromSchema(schema *SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *NestedSchemaDefinition) []*baseNode {
	var rootNodes []*baseNode

	// Determine if we're dealing with conditional fields
	hasConditionalFields := nsd != nil && nsd.StructuredFieldsArray != nil &&
		func() bool {
			for _, entry := range nsd.StructuredFieldsArray {
				if entry.When != nil {
					return true
				}
			}
			return false
		}()

	if hasConditionalFields {
		// Handle structured schema with conditional fields
		baseFields := make(map[string]bool)
		conditionalFields := make(map[string]*FieldInclusionCondition)
		allFields := make(map[string]*FieldDefinition)

		// Process all field entries
		for _, structuredFieldEntry := range nsd.StructuredFieldsArray {
			for fieldName, fieldDef := range structuredFieldEntry.Fields {
				allFields[fieldName] = fieldDef

				if structuredFieldEntry.When == nil {
					baseFields[fieldName] = true
				} else {
					conditionalFields[fieldName] = structuredFieldEntry.When
				}
			}
		}

		// Create conditional unexpected fields node
		unexpectedNode := &ConditionalUnexpectedFieldsNode{
			baseNode:          baseNode{id: buildNodeID(basePath, "conditional_unexpected_fields", ""), path: basePath},
			conditionalFields: conditionalFields,
			baseFields:        baseFields,
		}
		b.addNode(unexpectedNode)
		rootNodes = append(rootNodes, &unexpectedNode.baseNode)

		// Process all fields with conditional awareness
		var allFieldNodes []*baseNode
		for fieldName, fieldDef := range allFields {
			fieldNodes := b.buildFromFieldWithCondition(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints, conditionalFields[fieldName])
			allFieldNodes = append(allFieldNodes, fieldNodes...)
		}

		constraintDeps := getDependencyIDs(allFieldNodes)
		schemaConstraintIDs := b.buildFromConstraints(schema.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

		if len(schemaConstraintIDs) > 0 {
			completionNode := &NestedSchemaNode{
				baseNode: baseNode{
					id:   buildNodeID(basePath, "schema_completion", ""),
					path: basePath,
					deps: schemaConstraintIDs,
				},
			}
			b.addNode(completionNode)
			rootNodes = append(rootNodes, &completionNode.baseNode)
		}
	} else {
		// Handle regular schema (original logic)
		expectedFields := make(map[string]bool)
		fieldsToProcess := schema.Fields

		// If we have a structured schema without conditions, use its fields
		if nsd != nil && nsd.StructuredFieldsMap != nil {
			fieldsToProcess = nsd.StructuredFieldsMap
		}

		for name := range fieldsToProcess {
			expectedFields[name] = true
		}

		unexpectedNode := &UnexpectedFieldsNode{
			baseNode:       baseNode{id: buildNodeID(basePath, "unexpected_fields", ""), path: basePath},
			expectedFields: expectedFields,
		}
		b.addNode(unexpectedNode)
		rootNodes = append(rootNodes, &unexpectedNode.baseNode)

		var allFieldNodes []*baseNode
		for _, fieldDef := range fieldsToProcess {
			fieldNodes := b.buildFromField(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints)
			allFieldNodes = append(allFieldNodes, fieldNodes...)
		}

		constraintDeps := getDependencyIDs(allFieldNodes)
		schemaConstraintIDs := b.buildFromConstraints(schema.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

		if len(schemaConstraintIDs) > 0 {
			completionNode := &NestedSchemaNode{
				baseNode: baseNode{
					id:   buildNodeID(basePath, "schema_completion", ""),
					path: basePath,
					deps: schemaConstraintIDs,
				},
			}
			b.addNode(completionNode)
			rootNodes = append(rootNodes, &completionNode.baseNode)
		}
	}

	return rootNodes
}

// New method for building fields with conditional awareness
func (b *graphBuilder) buildFromFieldWithCondition(fieldDef *FieldDefinition, basePath string, baseDeps []string, schema *SchemaDefinition, dataContext any, addedConstraints map[string]bool, condition *FieldInclusionCondition) []*baseNode {
	fieldPath := buildPath(basePath, fieldDef.Name)
	currentDeps := baseDeps
	var nodes []*baseNode

	// Add conditional field presence check if this field has a condition
	if condition != nil {
		conditionalNode := &ConditionalFieldNode{
			baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional", ""), path: basePath, deps: currentDeps},
			condition: condition,
			fieldName: fieldDef.Name,
		}
		b.addNode(conditionalNode)
		currentDeps = []string{conditionalNode.id}
		nodes = append(nodes, &conditionalNode.baseNode)
	}

	// Handle required field check with conditional awareness
	if fieldDef.Required != nil && *fieldDef.Required {
		if condition != nil {
			// Use conditional required field node
			reqNode := &ConditionalRequiredFieldNode{
				baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional_required", ""), path: fieldPath, deps: currentDeps},
				fieldName: fieldDef.Name,
				condition: condition,
			}
			b.addNode(reqNode)
			currentDeps = []string{reqNode.id}
			nodes = append(nodes, &reqNode.baseNode)
		} else {
			// Use regular required field node
			reqNode := &RequiredFieldNode{
				baseNode:  baseNode{id: buildNodeID(fieldPath, "required", ""), path: fieldPath, deps: currentDeps},
				fieldName: fieldDef.Name,
			}
			b.addNode(reqNode)
			currentDeps = []string{reqNode.id}
			nodes = append(nodes, &reqNode.baseNode)
		}
	}

	// Continue with the rest of the regular field processing
	// (This would include the TypeCheckNode and all other field-type-specific nodes)
	typeNode := &TypeCheckNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "type_check", ""), path: fieldPath, deps: currentDeps},
		fieldDef: fieldDef,
	}
	b.addNode(typeNode)
	currentDeps = []string{typeNode.id}
	nodes = append(nodes, &typeNode.baseNode)

	// ... rest of the field type processing would continue as in the original buildFromField method ...
	// (enum, array, set, object, record, union processing)

	switch fieldDef.Type {
	case FieldTypeEnum:
		enumNode := &EnumValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "enum", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
		}
		b.addNode(enumNode)
		currentDeps = []string{enumNode.id}
		nodes = append(nodes, &enumNode.baseNode)
	case FieldTypeArray:
		arrayNode := &ArrayValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "array", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(arrayNode)
		currentDeps = []string{arrayNode.id}
		nodes = append(nodes, &arrayNode.baseNode)
	case FieldTypeSet:
		setNode := &SetValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "set", ""), path: fieldPath, deps: currentDeps},
		}
		b.addNode(setNode)
		currentDeps = []string{setNode.id}
		nodes = append(nodes, &setNode.baseNode)
	case FieldTypeObject:
		if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
			if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
				// Validate that the referenced nested schema is appropriate for object fields
				if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
					break
				}

				// CORRECT: Handle structured nested schema for object field
				refConstraintIDs := b.buildFromConstraints(ref.Constraints, fieldPath, []string{typeNode.id}, dataContext, addedConstraints)

				// Update currentDeps to include reference constraints
				if len(refConstraintIDs) > 0 {
					currentDeps = append(currentDeps, refConstraintIDs...)
				}

				// Build the structured schema
				tempSchema := &SchemaDefinition{
					Name:          nestedSchemaDef.Name,
					Fields:        make(map[string]*FieldDefinition),
					NestedSchemas: schema.NestedSchemas,
					Constraints:   nestedSchemaDef.Constraints,
				}

				// Handle structured fields
				if nestedSchemaDef.StructuredFieldsArray != nil {
					for _, structuredFieldEntry := range nestedSchemaDef.StructuredFieldsArray {
						// For graph building, we add all potential fields.
						// The 'When' condition is handled at execution time
						maps.Copy(tempSchema.Fields, structuredFieldEntry.Fields)
					}
				} else if nestedSchemaDef.StructuredFieldsMap != nil {
					tempSchema.Fields = nestedSchemaDef.StructuredFieldsMap
				}

				nestedNodes := b.buildFromSchema(tempSchema, fieldPath, nil, addedConstraints, nestedSchemaDef)

				markerNode := &NestedSchemaNode{
					baseNode: baseNode{
						id:   buildNodeID(fieldPath, "nested_schema", ""),
						path: fieldPath,
						deps: getDependencyIDs(nestedNodes),
					},
				}
				b.addNode(markerNode)
				currentDeps = []string{markerNode.id}
				nodes = append(nodes, &markerNode.baseNode)
			}
		}
	case FieldTypeRecord:
		recordNode := &RecordValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(recordNode)
		currentDeps = []string{recordNode.id}
		nodes = append(nodes, &recordNode.baseNode)
	case FieldTypeUnion:
		unionNode := &UnionValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "union", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(unionNode)
		currentDeps = []string{unionNode.id}
		nodes = append(nodes, &unionNode.baseNode)
	}

	// FIX 5: Capture and use the returned constraint IDs from field-level constraints
	fieldConstraintIDs := b.buildFromConstraints(fieldDef.Constraints, fieldPath, currentDeps, dataContext, addedConstraints)

	// If there are field constraints, create a completion node that depends on them
	if len(fieldConstraintIDs) > 0 {
		completionNode := &NestedSchemaNode{
			baseNode: baseNode{
				id:   buildNodeID(fieldPath, "field_completion", ""),
				path: fieldPath,
				deps: fieldConstraintIDs,
			},
		}
		b.addNode(completionNode)
		nodes = append(nodes, &completionNode.baseNode)
	}

	return nodes
}

/* func (b *graphBuilder) buildFromSchema(schema *SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool) []*baseNode {
	var rootNodes []*baseNode

	expectedFields := make(map[string]bool)
	for name := range schema.Fields {
		expectedFields[name] = true
	}
	unexpectedNode := &UnexpectedFieldsNode{
		baseNode:       baseNode{id: buildNodeID(basePath, "unexpected_fields", ""), path: basePath},
		expectedFields: expectedFields,
	}
	b.addNode(unexpectedNode)
	rootNodes = append(rootNodes, &unexpectedNode.baseNode)

	var allFieldNodes []*baseNode
	for _, fieldDef := range schema.Fields {
		fieldNodes := b.buildFromField(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints)
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	constraintDeps := getDependencyIDs(allFieldNodes)

	// FIX 1: Capture and use the returned constraint IDs for schema-level constraints
	schemaConstraintIDs := b.buildFromConstraints(schema.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

	// The schema-level constraints should be considered as additional root nodes
	// that must complete before the schema validation is considered finished
	if len(schemaConstraintIDs) > 0 {
		// Create a completion marker node that depends on all schema constraints
		completionNode := &NestedSchemaNode{
			baseNode: baseNode{
				id:   buildNodeID(basePath, "schema_completion", ""),
				path: basePath,
				deps: schemaConstraintIDs,
			},
		}
		b.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes
} */

func (b *graphBuilder) buildFromField(fieldDef *FieldDefinition, basePath string, baseDeps []string, schema *SchemaDefinition, dataContext any, addedConstraints map[string]bool) []*baseNode {
	fieldPath := buildPath(basePath, fieldDef.Name)
	currentDeps := baseDeps
	var nodes []*baseNode

	if fieldDef.Required != nil && *fieldDef.Required {
		reqNode := &RequiredFieldNode{
			baseNode:  baseNode{id: buildNodeID(fieldPath, "required", ""), path: fieldPath, deps: currentDeps},
			fieldName: fieldDef.Name,
		}
		b.addNode(reqNode)
		currentDeps = []string{reqNode.id}
		nodes = append(nodes, &reqNode.baseNode)
	}

	typeNode := &TypeCheckNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "type_check", ""), path: fieldPath, deps: currentDeps},
		fieldDef: fieldDef,
	}
	b.addNode(typeNode)
	currentDeps = []string{typeNode.id}
	nodes = append(nodes, &typeNode.baseNode)

	switch fieldDef.Type {
	case FieldTypeEnum:
		enumNode := &EnumValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "enum", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
		}
		b.addNode(enumNode)
		currentDeps = []string{enumNode.id}
		nodes = append(nodes, &enumNode.baseNode)
	case FieldTypeArray:
		arrayNode := &ArrayValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "array", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(arrayNode)
		currentDeps = []string{arrayNode.id}
		nodes = append(nodes, &arrayNode.baseNode)
	case FieldTypeSet:
		setNode := &SetValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "set", ""), path: fieldPath, deps: currentDeps},
		}
		b.addNode(setNode)
		currentDeps = []string{setNode.id}
		nodes = append(nodes, &setNode.baseNode)
	case FieldTypeObject:
		if ref, ok := fieldDef.Schema.(NestedSchemaReference); ok {
			if nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID); exists {
				// Validate that the referenced nested schema is appropriate for object fields
				if nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
					break
				}

				// CORRECT: Handle structured nested schema for object field
				refConstraintIDs := b.buildFromConstraints(ref.Constraints, fieldPath, []string{typeNode.id}, dataContext, addedConstraints)

				// Update currentDeps to include reference constraints
				if len(refConstraintIDs) > 0 {
					currentDeps = append(currentDeps, refConstraintIDs...)
				}

				// Build the structured schema
				tempSchema := &SchemaDefinition{
					Name:          nestedSchemaDef.Name,
					Fields:        make(map[string]*FieldDefinition),
					NestedSchemas: schema.NestedSchemas,
					Constraints:   nestedSchemaDef.Constraints,
				}

				// Handle structured fields
				if nestedSchemaDef.StructuredFieldsArray != nil {
					for _, structuredFieldEntry := range nestedSchemaDef.StructuredFieldsArray {
						// For graph building, we add all potential fields.
						// The 'When' condition is handled at execution time
						maps.Copy(tempSchema.Fields, structuredFieldEntry.Fields)
					}
				} else if nestedSchemaDef.StructuredFieldsMap != nil {
					tempSchema.Fields = nestedSchemaDef.StructuredFieldsMap
				}

				nestedNodes := b.buildFromSchema(tempSchema, fieldPath, nil, addedConstraints, nestedSchemaDef)

				markerNode := &NestedSchemaNode{
					baseNode: baseNode{
						id:   buildNodeID(fieldPath, "nested_schema", ""),
						path: fieldPath,
						deps: getDependencyIDs(nestedNodes),
					},
				}
				b.addNode(markerNode)
				currentDeps = []string{markerNode.id}
				nodes = append(nodes, &markerNode.baseNode)
			}
		}
	case FieldTypeRecord:
		recordNode := &RecordValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(recordNode)
		currentDeps = []string{recordNode.id}
		nodes = append(nodes, &recordNode.baseNode)
	case FieldTypeUnion:
		unionNode := &UnionValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "union", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		b.addNode(unionNode)
		currentDeps = []string{unionNode.id}
		nodes = append(nodes, &unionNode.baseNode)
	}

	// FIX 5: Capture and use the returned constraint IDs from field-level constraints
	fieldConstraintIDs := b.buildFromConstraints(fieldDef.Constraints, fieldPath, currentDeps, dataContext, addedConstraints)

	// If there are field constraints, create a completion node that depends on them
	if len(fieldConstraintIDs) > 0 {
		completionNode := &NestedSchemaNode{
			baseNode: baseNode{
				id:   buildNodeID(fieldPath, "field_completion", ""),
				path: fieldPath,
				deps: fieldConstraintIDs,
			},
		}
		b.addNode(completionNode)
		nodes = append(nodes, &completionNode.baseNode)
	}

	return nodes
}

func (b *graphBuilder) buildFromConstraints(constraints SchemaConstraint[FieldType], path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
	var ruleDepIDs []string
	for _, rule := range constraints {
		rules := b.buildFromConstraintRule(rule, path, deps, dataContext, addedConstraints)
		ruleDepIDs = append(ruleDepIDs, rules...)
	}
	return ruleDepIDs
}

func (b *graphBuilder) buildFromConstraintRule(rule SchemaConstraintRule[FieldType], path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
	var ruleDepIDs []string
	switch r := rule.(type) {
	case Constraint[FieldType]:
		nodeID := buildNodeID(path, "constraint", r.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID} // Already added, return its ID as a dependency
		}
		fieldDeps := deps
		if r.Field != nil && *r.Field != "" {
			targetPath := buildPath(path, *r.Field)
			fieldDeps = append(fieldDeps, buildNodeID(targetPath, "type_check", ""))
		}
		node := &ConstraintNode{
			baseNode:   baseNode{id: nodeID, path: path, deps: fieldDeps},
			constraint: r,
			fmap:       b.fmap,
		}
		b.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	case ConstraintGroup[FieldType]:
		var memberDeps []string
		for _, memberRule := range r.Rules {
			memberDeps = append(memberDeps, b.buildFromConstraintRule(memberRule, path, deps, dataContext, addedConstraints)...)
		}
		nodeID := buildNodeID(path, "constraint_group", r.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID} // Already added, return its ID as a dependency
		}
		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: path, deps: memberDeps},
			group:    r,
		}
		b.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}
	return ruleDepIDs
}

// --- Phase 2: Graph Traversal (`Validate`) ---

func (v *DocumentValidator) Validate(document map[string]any, loose bool) ([]Issue, bool) {
	ctx := &ValidationContext{
		RootData:    document,
		Data:        document,
		Results:     make(map[string]*NodeResult),
		FunctionMap: v.fmap,
	}

	visited := make(map[string]bool)
	var allIssues []Issue

	nodeIDs := make([]string, 0, len(v.graph.nodes))
	for id := range v.graph.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	toProcess := nodeIDs

	for len(toProcess) > 0 {
		var nextRound []string
		progressMade := false

		for _, nodeID := range toProcess {
			if visited[nodeID] {
				continue
			}

			if v.dependenciesSatisfied(nodeID, visited) {
				node := v.graph.nodes[nodeID]

				ctx.Data = ctx.RootData

				// All nodes should now extract data using getValueByPath(ctx.RootData, node.GetPath())
				// The 'exists' check for RequiredFieldNode is handled by its Execute method.
				// For other nodes, if data doesn't exist at path, their Execute methods should handle it gracefully.
				result := node.Execute(ctx)
				ctx.Results[nodeID] = result
				if result != nil {
					allIssues = append(allIssues, result.Issues...)
				}

				visited[nodeID] = true
				progressMade = true
			} else {
				nextRound = append(nextRound, nodeID)
			}
		}

		if !progressMade && len(nextRound) > 0 {
			allIssues = append(allIssues, Issue{Code: "CIRCULAR_DEPENDENCY", Message: "Circular dependency detected"})
			break
		}
		toProcess = nextRound
	}

	if loose {
		filteredIssues := make([]Issue, 0, len(allIssues))
		for _, issue := range allIssues {
			if issue.Code != "REQUIRED_FIELD_MISSING" {
				filteredIssues = append(filteredIssues, issue)
			}
		}
		allIssues = filteredIssues
	}

	// Sort issues for consistent test results
	sort.Slice(allIssues, func(i, j int) bool {
		// Primary sort by path, secondary sort by code for consistent ordering
		if allIssues[i].Path != allIssues[j].Path {
			return allIssues[i].Path < allIssues[j].Path
		}
		return allIssues[i].Code < allIssues[j].Code
	})

	return allIssues, len(allIssues) == 0
}

func (v *DocumentValidator) dependenciesSatisfied(nodeID string, visited map[string]bool) bool {
	deps := v.graph.dependencies[nodeID]
	for _, depID := range deps {
		if !visited[depID] {
			return false
		}
	}
	return true
}

// --- Node Execution Implementations ---

func (n *UnexpectedFieldsNode) Execute(ctx *ValidationContext) *NodeResult {
	currentData, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true} // No data at path, nothing to check for unexpected fields
	}

	dataMap, ok := currentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for unexpected field check", Path: n.path}}}
	}

	var issues []Issue
	for key := range dataMap {
		if !n.expectedFields[key] {
			issues = append(issues, Issue{Code: "UNEXPECTED_FIELD", Message: fmt.Sprintf("Unexpected field '%s'", key), Path: buildPath(n.path, key)})
		}
	}
	return &NodeResult{Success: len(issues) == 0, Issues: issues}
}

func (n *RequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentPath := getScopedPath(n.path)
	parentData, exists := getValueByPath(ctx.RootData, parentPath)
	if !exists {
		return &NodeResult{Success: true} // Parent data doesn't exist, so field can't be present. Handled by other nodes.
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "INVALID_DATA_STRUCTURE", Message: "Cannot check for required fields on non-object parent", Path: parentPath}}}
	}

	// Extract the field name from the full path
	fieldName := n.path[len(parentPath)+1:]
	if parentPath == "" {
		fieldName = n.path
	}

	if _, exists := dataMap[fieldName]; !exists {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
	}
	return &NodeResult{Success: true}
}

func (n *TypeCheckNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	coercedValue, _ := coerceValue(value, n.fieldDef.Type)

	valid, issue := validateFieldType(coercedValue, n.fieldDef, n.path)
	if !valid {
		return &NodeResult{Success: false, Issues: []Issue{*issue}, Value: value}
	}
	return &NodeResult{Success: true, Value: coercedValue}
}

func (n *EnumValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}
	for _, allowedValue := range n.fieldDef.Values {
		if reflect.DeepEqual(value, allowedValue) {
			return &NodeResult{Success: true}
		}
	}
	return &NodeResult{Success: false, Issues: []Issue{{Code: "ENUM_VIOLATION", Message: fmt.Sprintf("Value must be one of: %v", n.fieldDef.Values), Path: n.path}}}
}

func (n *ArrayValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true} // Not present, handled by required check.
	}

	items, ok := value.([]any)
	if !ok {
		// This should be caught by the TypeCheckNode dependency, but serves as a safeguard.
		return &NodeResult{Success: false, Issues: []Issue{{Code: "TYPE_MISMATCH", Message: "Expected array", Path: n.path}}}
	}

	if n.fieldDef.ItemsType == nil {
		return &NodeResult{Success: true} // No item type specified, nothing to validate.
	}

	var allIssues []Issue
	itemType := *n.fieldDef.ItemsType

	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)

		// Create a temporary schema and validator for this single item.
		tempRootField := FieldDefinition{
			Name:      "item",
			Type:      itemType,
			Schema:    n.fieldDef.Schema, // Pass along schema ref for Object/Record/Union
			ItemsType: nil,               // ItemsType is not nested within another array validator
		}

		tempSchema := &SchemaDefinition{
			Name:          "temp_array_item_check",
			Fields:        map[string]*FieldDefinition{"item": &tempRootField},
			NestedSchemas: n.schema.NestedSchemas, // Provide access to all known nested schemas
		}

		validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
		if err != nil {
			return &NodeResult{Success: false, Issues: []Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: err.Error(), Path: itemPath}}}
		}
		itemIssues, _ := validator.Validate(map[string]any{"item": item}, false)

		// The temporary validator reports paths starting with "item". We must rewrite them
		// to correspond to the correct array index path.
		for j := range itemIssues {
			itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
		}
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *RecordValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true} // Handled by the required field check.
	}

	recordMap, ok := value.(map[string]any)
	if !ok {
		// This is handled by the TypeCheckNode, but serves as a safeguard.
		return &NodeResult{Success: false, Issues: []Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for record", Path: n.path}}}
	}

	// Case 1: If schema is not defined, the value must be a map.
	// The TypeCheckNode has already confirmed this, so we are done.
	if n.fieldDef.Schema == nil {
		return &NodeResult{Success: true}
	}

	// Case 2: If schema is defined, all values must conform.
	ref, ok := n.fieldDef.Schema.(NestedSchemaReference)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "INVALID_RECORD_SCHEMA", Message: "Record schema must be a NestedSchemaReference", Path: n.path}}}
	}

	nestedDef, exists := n.schema.FindNestedSchema(ref.ID)
	if !exists {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "NESTED_SCHEMA_NOT_FOUND", Message: fmt.Sprintf("Nested schema '%s' not found for record items", ref.ID), Path: n.path}}}
	}

	var allIssues []Issue

	for key, itemValue := range recordMap {
		itemPath := buildPath(n.path, key)

		// Create a temporary validator for each value in the record.
		var tempRootField FieldDefinition
		if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
			// Handle structured item values (i.e., objects).
			tempRootField = FieldDefinition{Name: "item", Type: FieldTypeObject, Schema: ref}
		} else if nestedDef.Type != nil {
			// Handle unstructured item values (i.e., literals like string, number).
			tempRootField = FieldDefinition{Name: "item", Type: *nestedDef.Type, Schema: nestedDef.Schema, ItemsType: nestedDef.ItemsType}
		} else {
			continue // Should not happen with a valid nested schema.
		}

		tempSchema := &SchemaDefinition{
			Name:          "temp_record_item_check",
			Fields:        map[string]*FieldDefinition{"item": &tempRootField},
			NestedSchemas: n.schema.NestedSchemas,
		}

		validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
		if err != nil {
			return &NodeResult{Success: false, Issues: []Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: err.Error(), Path: itemPath}}}
		}
		itemIssues, _ := validator.Validate(map[string]any{"item": itemValue}, false)

		// Rewrite paths from "item.subpath" to "recordName.key.subpath" for correct error reporting.
		for j := range itemIssues {
			itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
		}
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *SetValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	items, ok := value.([]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "TYPE_MISMATCH", Message: "Expected array for set", Path: n.path}}}
	}

	seen := make(map[string]bool)
	for i, item := range items {
		key := fmt.Sprintf("%v", item)
		if seen[key] {
			return &NodeResult{Success: false, Issues: []Issue{{Code: "SET_DUPLICATE", Message: fmt.Sprintf("Duplicate value found in set at index %d", i), Path: n.path}}}
		}
		seen[key] = true
	}
	return &NodeResult{Success: true}
}

func (n *NestedSchemaNode) Execute(ctx *ValidationContext) *NodeResult {
	return &NodeResult{Success: true}
}

func (n *UnionValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	schemas, ok := n.fieldDef.Schema.([]NestedSchemaReference)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "INVALID_UNION_SCHEMA", Path: n.path}}}
	}

	var specificConstraintViolations []Issue // Stores issues where type matched, but constraints failed

	for _, schemaRef := range schemas {
		nestedDef, exists := n.schema.FindNestedSchema(schemaRef.ID)
		if !exists {
			continue // Skip if nested schema definition not found
		}

		var tempRootField FieldDefinition
		if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
			tempRootField = FieldDefinition{Name: "root", Type: FieldTypeObject, Schema: schemaRef}
		} else if nestedDef.Type != nil {
			// FIX: For literal nested schemas, include the constraints
			tempRootField = FieldDefinition{
				Name:        "root",
				Type:        *nestedDef.Type,
				Schema:      nestedDef.Schema,
				ItemsType:   nestedDef.ItemsType,
				Constraints: nestedDef.Constraints, // Include constraints from nested schema
			}
		} else {
			continue // Should not happen with a valid nested schema.
		}

		tempSchema := &SchemaDefinition{
			Name:          "temp_union_check",
			Fields:        map[string]*FieldDefinition{"root": &tempRootField},
			NestedSchemas: n.schema.NestedSchemas,
		}

		validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
		if err != nil {
			// If validator creation fails for this union option, it cannot be matched.
			continue
		}

		// Perform validation for this specific union branch
		itemIssues, matched := validator.Validate(map[string]any{"root": value}, false)

		// If this branch fully matched, we are done.
		if matched {
			return &NodeResult{Success: true}
		}

		// If not matched, we need to categorize the issues.
		// Rewrite paths first to be consistent with the parent path.
		for i := range itemIssues {
			itemIssues[i].Path = strings.Replace(itemIssues[i].Path, "root", n.path, 1)
		}

		// NEW LOGIC: Only collect constraint violations if there were no structural issues
		hasStructuralIssues := false
		var constraintViolations []Issue

		for _, issue := range itemIssues {
			switch issue.Code {
			case "TYPE_MISMATCH", "UNEXPECTED_FIELD", "REQUIRED_FIELD_MISSING", "ENUM_VIOLATION":
				// These indicate structural incompatibility with this union branch
				hasStructuralIssues = true
			case "CONSTRAINT_VIOLATION":
				// This indicates the structure matched but business rules failed
				constraintViolations = append(constraintViolations, issue)
			}
		}

		// Only collect constraint violations if the branch was structurally compatible
		if !hasStructuralIssues && len(constraintViolations) > 0 {
			specificConstraintViolations = append(specificConstraintViolations, constraintViolations...)
		}
	}

	// After trying all schemas in the union:
	if len(specificConstraintViolations) > 0 {
		// If at least one schema was structurally compatible but failed constraints,
		// return those specific constraint violations.
		return &NodeResult{Success: false, Issues: specificConstraintViolations}
	} else {
		// If no schema fully matched, and no structurally compatible branches had constraint violations,
		// then it means all branches were either structurally incompatible or had no issues at all.
		// In this case, return a single UNION_NO_MATCH issue.
		return &NodeResult{Success: false, Issues: []Issue{{Code: "UNION_NO_MATCH", Message: "Value does not match any of the union schemas", Path: n.path}}}
	}
}

func (n *ConstraintNode) Execute(ctx *ValidationContext) *NodeResult {
	predicateFunc, exists := (*n.fmap)[n.constraint.Predicate]
	if !exists {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "MISSING_PREDICATE", Message: fmt.Sprintf("Predicate '%s' not found", n.constraint.Predicate), Path: n.path}}}
	}

	predicate, ok := predicateFunc.(func(PredicateParams[any]) bool)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "INVALID_PREDICATE_TYPE", Message: fmt.Sprintf("Predicate '%s' has invalid type", n.constraint.Predicate), Path: n.path}}}
	}

	// The data for the predicate is the value at the node's path.
	predicateData, dataExists := getValueByPath(ctx.RootData, n.path)
	if !dataExists {
		// If the data for the constraint doesn't exist, we can't evaluate it.
		// This is not a failure of the constraint itself, but of data presence,
		// which should be handled by 'required' checks.
		return &NodeResult{Success: true}
	}

	// If the constraint targets a sub-field, the predicateData is already the container.
	// If it doesn't, predicateData is the value to be tested itself.
	// The predicate function's implementation handles this distinction with the 'Field' parameter.

	params := PredicateParams[any]{
		Data:  predicateData,
		Field: n.constraint.Field,
		Args:  n.constraint.Parameters,
	}

	if !predicate(params) {
		message := fmt.Sprintf("Constraint '%s' failed", n.constraint.Name)
		if n.constraint.ErrorMessage != nil {
			message = *n.constraint.ErrorMessage
		}

		// The path of the issue should be more specific if a field is targeted.
		issuePath := n.path
		if n.constraint.Field != nil && *n.constraint.Field != "" {
			// Check if predicateData is a map to avoid panic
			if _, ok := predicateData.(map[string]any); ok {
				// Only append the field to the path if it's not a global constraint (path is empty)
				if n.path != "" {
					issuePath = buildPath(n.path, *n.constraint.Field)
				} else {
					issuePath = *n.constraint.Field
				}
			}
		}

		return &NodeResult{Success: false, Issues: []Issue{{Code: "CONSTRAINT_VIOLATION", Message: message, Path: issuePath}}}
	}

	return &NodeResult{Success: true}
}

func (n *ConstraintGroupNode) Execute(ctx *ValidationContext) *NodeResult {
	var results []bool

	for _, depID := range n.deps {
		if result, ok := ctx.Results[depID]; ok {
			results = append(results, result.Success)
		} else {
			results = append(results, false)
		}
	}

	if !evaluateLogicalOperator(n.group.Operator, results) {
		// If the group constraint fails, add the group violation issue
		groupViolationIssue := Issue{Code: "CONSTRAINT_GROUP_VIOLATION", Message: fmt.Sprintf("Constraint group '%s' failed", n.group.Name), Path: n.path}
		return &NodeResult{Success: false, Issues: []Issue{groupViolationIssue}}
	}

	return &NodeResult{Success: true}
}

// Execute method for ConditionalFieldNode
func (n *ConditionalFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	// Get the data at the current path (the containing object)
	parentData, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true} // Parent doesn't exist, condition can't be evaluated
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: true} // Parent is not an object, condition can't be evaluated
	}

	// Evaluate the condition
	conditionMet := evaluateFieldInclusionCondition(n.condition, dataMap)

	if !conditionMet {
		// Condition not met, this field should not be present
		if _, fieldExists := dataMap[n.fieldName]; fieldExists {
			return &NodeResult{
				Success: false,
				Issues: []Issue{{
					Code:    "CONDITIONAL_FIELD_PRESENT",
					Message: fmt.Sprintf("Field '%s' should not be present when %s != %v", n.fieldName, n.condition.Field, n.condition.Value),
					Path:    buildPath(n.path, n.fieldName),
				}},
			}
		}
		// Field correctly absent
		return &NodeResult{Success: true}
	}

	// Condition met, field validation will be handled by regular field nodes
	return &NodeResult{Success: true}
}

// Execute method for ConditionalUnexpectedFieldsNode
func (n *ConditionalUnexpectedFieldsNode) Execute(ctx *ValidationContext) *NodeResult {
	currentData, exists := getValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	dataMap, ok := currentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for conditional field check", Path: n.path}}}
	}

	var issues []Issue

	// Check each field in the data
	for fieldName := range dataMap {
		isExpected := false

		// Check if it's a base field (always allowed)
		if n.baseFields[fieldName] {
			isExpected = true
		} else {
			// Check if it's a conditional field whose condition is met
			if condition, exists := n.conditionalFields[fieldName]; exists {
				if evaluateFieldInclusionCondition(condition, dataMap) {
					isExpected = true
				}
			}
		}

		if !isExpected {
			issues = append(issues, Issue{
				Code:    "UNEXPECTED_FIELD",
				Message: fmt.Sprintf("Unexpected field '%s'", fieldName),
				Path:    buildPath(n.path, fieldName),
			})
		}
	}

	return &NodeResult{Success: len(issues) == 0, Issues: issues}
}

func (n *ConditionalRequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentPath := getScopedPath(n.path)
	parentData, exists := getValueByPath(ctx.RootData, parentPath)
	if !exists {
		return &NodeResult{Success: true} // Parent doesn't exist
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []Issue{{Code: "INVALID_DATA_STRUCTURE", Message: "Cannot check for required fields on non-object parent", Path: parentPath}}}
	}

	// Check if the condition is met
	conditionMet := evaluateFieldInclusionCondition(n.condition, dataMap)

	if conditionMet {
		// Condition met, field is required
		fieldName := n.path[len(parentPath)+1:]
		if parentPath == "" {
			fieldName = n.path
		}

		if _, exists := dataMap[fieldName]; !exists {
			return &NodeResult{Success: false, Issues: []Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
		}
	}

	// Either condition not met (field optional) or field present when required
	return &NodeResult{Success: true}
}

// --- Helper Functions ---

func evaluateFieldInclusionCondition(condition *FieldInclusionCondition, data map[string]any) bool {
	if condition == nil {
		return true // No condition means always included
	}

	fieldValue, exists := data[condition.Field]
	if !exists {
		return false // Condition field doesn't exist
	}

	// Use reflect.DeepEqual for robust value comparison
	return reflect.DeepEqual(fieldValue, condition.Value)
}

func buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

func getScopedPath(path string) string {
	if !strings.Contains(path, ".") {
		return ""
	}
	parts := strings.Split(path, ".")
	return strings.Join(parts[:len(parts)-1], ".")
}

func getDependencyIDs(nodes []*baseNode) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.id
	}
	return ids
}

func coerceValue(value any, expectedType FieldType) (any, bool) {
	str, ok := value.(string)
	if !ok {
		return value, false
	}
	switch expectedType {
	case FieldTypeBoolean:
		lower := strings.ToLower(str)
		if lower == "true" {
			return true, true
		}
		if lower == "false" {
			return false, true
		}
	case FieldTypeInteger:
		if intVal, err := strconv.ParseInt(str, 10, 64); err == nil {
			return int(intVal), true
		}
	case FieldTypeNumber, FieldTypeDecimal:
		if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
			return floatVal, true
		}
	}
	return value, false
}

func validateFieldType(value any, fieldDef *FieldDefinition, path string) (bool, *Issue) {
	if value == nil {
		return true, nil
	}
	var ok bool
	switch fieldDef.Type {
	case FieldTypeString:
		_, ok = value.(string)
	case FieldTypeNumber, FieldTypeDecimal:
		switch value.(type) {
		case float64, float32, int, int64, int32:
			ok = true
		default:
			ok = false
		}
	case FieldTypeInteger:
		switch value.(type) {
		case int, int64, int32, int16, int8:
			ok = true
		default:
			ok = false
		}
	case FieldTypeBoolean:
		_, ok = value.(bool)
	case FieldTypeArray, FieldTypeSet:
		_, ok = value.([]any)
	case FieldTypeObject, FieldTypeRecord:
		_, ok = value.(map[string]any)
	case FieldTypeUnion, FieldTypeEnum:
		return true, nil
	}
	if !ok {
		return false, &Issue{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Expected %s, got %T", fieldDef.Type, value), Path: path}
	}
	return true, nil
}

func evaluateLogicalOperator(operator LogicalOperator, results []bool) bool {
	switch operator {
	case LogicalAnd:
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	case LogicalOr:
		for _, r := range results {
			if r {
				return true
			}
		}
		return len(results) == 0
	case LogicalNot:
		return len(results) == 1 && !results[0]
	case LogicalNor:
		for _, r := range results {
			if r {
				return false
			}
		}
		return true
	case LogicalXor:
		trueCount := 0
		for _, r := range results {
			if r {
				trueCount++
			}
		}
		return trueCount == 1
	}
	return false
}

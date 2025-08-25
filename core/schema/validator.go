package schema

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// =============================================================================
// CORE INTERFACES AND TYPES
// =============================================================================

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
	Issues  []common.Issue
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
	visitedState map[string]int // For cycle detection during graph building
}

// =============================================================================
// GRAPH BUILDER TYPES AND CONSTANTS
// =============================================================================

const (
	dfsUnvisited = 0
	dfsVisiting  = 1
	dfsVisited   = 2
)

// =============================================================================
// NODE TYPE DEFINITIONS
// =============================================================================

// BaseNode provides common fields for all nodes.
type baseNode struct {
	id   string
	path string
	deps []string
}

func (n *baseNode) GetID() string             { return n.id }
func (n *baseNode) GetPath() string           { return n.path }
func (n *baseNode) GetDependencies() []string { return n.deps }

// Validation node types
type UnexpectedFieldsNode struct {
	baseNode
	expectedFields map[string]bool
}

type RequiredFieldNode struct {
	baseNode
	fieldName string
}

type ConditionalRequiredFieldNode struct {
	baseNode
	fieldName string
	condition *FieldInclusionCondition
}

type ConditionalFieldNode struct {
	baseNode
	condition *FieldInclusionCondition
	fieldName string
}

type ConditionalUnexpectedFieldsNode struct {
	baseNode
	conditionalFields map[string]*FieldInclusionCondition
	baseFields        map[string]bool
}

type TypeCheckNode struct {
	baseNode
	fieldDef *FieldDefinition
}

type EnumValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
}

type ArrayValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

type RecordValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

type SetValidationNode struct {
	baseNode
}

type NestedSchemaNode struct {
	baseNode
}

type UnionValidationNode struct {
	baseNode
	fieldDef *FieldDefinition
	schema   *SchemaDefinition
}

type ConstraintNode struct {
	baseNode
	constraint Constraint[FieldType]
}

type ConstraintGroupNode struct {
	baseNode
	group     ConstraintGroup[FieldType]
	memberIDs []string
}

// =============================================================================
// HELPER UTILITIES
// =============================================================================

// Path manipulation utilities
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

func getDependencyIDs(nodes []*baseNode) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.id
	}
	return ids
}

// =============================================================================
// GRAPH BUILDER FACTORY METHODS
// =============================================================================

func newValidationGraph() *ValidationGraph {
	return &ValidationGraph{
		nodes:        make(map[string]ValidationNode),
		dependencies: make(map[string][]string),
		visitedState: make(map[string]int),
	}
}

func (graph *ValidationGraph) addNode(node ValidationNode) {
	nodeID := node.GetID()
	if _, exists := graph.nodes[nodeID]; exists {
		return
	}
	graph.nodes[nodeID] = node
	graph.dependencies[nodeID] = node.GetDependencies()
}

// Node factory methods
func (graph *ValidationGraph) createUnexpectedFieldsNode(path string, expectedFields map[string]bool) *UnexpectedFieldsNode {
	return &UnexpectedFieldsNode{
		baseNode:       baseNode{id: buildNodeID(path, "unexpected_fields", ""), path: path},
		expectedFields: expectedFields,
	}
}

func (graph *ValidationGraph) createConditionalUnexpectedFieldsNode(path string, conditionalFields map[string]*FieldInclusionCondition, baseFields map[string]bool) *ConditionalUnexpectedFieldsNode {
	return &ConditionalUnexpectedFieldsNode{
		baseNode:          baseNode{id: buildNodeID(path, "conditional_unexpected_fields", ""), path: path},
		conditionalFields: conditionalFields,
		baseFields:        baseFields,
	}
}

func (graph *ValidationGraph) createRequiredFieldNode(fieldPath string, fieldName string, deps []string) *RequiredFieldNode {
	return &RequiredFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "required", ""), path: fieldPath, deps: deps},
		fieldName: fieldName,
	}
}

func (graph *ValidationGraph) createConditionalRequiredFieldNode(fieldPath string, fieldName string, condition *FieldInclusionCondition, deps []string) *ConditionalRequiredFieldNode {
	return &ConditionalRequiredFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional_required", ""), path: fieldPath, deps: deps},
		fieldName: fieldName,
		condition: condition,
	}
}

func (graph *ValidationGraph) createConditionalFieldNode(fieldPath string, fieldName string, condition *FieldInclusionCondition, deps []string) *ConditionalFieldNode {
	return &ConditionalFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional", ""), path: fieldPath, deps: deps},
		condition: condition,
		fieldName: fieldName,
	}
}

func (graph *ValidationGraph) createTypeCheckNode(fieldPath string, fieldDef *FieldDefinition, deps []string) *TypeCheckNode {
	return &TypeCheckNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "type_check", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
	}
}

func (graph *ValidationGraph) createCompletionNode(path string, nodeType string, deps []string) *NestedSchemaNode {
	return &NestedSchemaNode{
		baseNode: baseNode{
			id:   buildNodeID(path, nodeType, ""),
			path: path,
			deps: deps,
		},
	}
}

// =============================================================================
// GRAPH CONSTRUCTION LOGIC
// =============================================================================

func (graph *ValidationGraph) dfsCheck(nodeID string) bool {
	graph.visitedState[nodeID] = dfsVisiting

	for _, depID := range graph.dependencies[nodeID] {
		if graph.visitedState[depID] == dfsVisiting {
			return true // Cycle detected
		}
		if graph.visitedState[depID] == dfsUnvisited {
			if graph.dfsCheck(depID) {
				return true // Cycle detected in recursive call
			}
		}
	}

	graph.visitedState[nodeID] = dfsVisited
	return false
}

func (graph *ValidationGraph) buildFromSchema(schema *SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *NestedSchemaDefinition) []*baseNode {
	var rootNodes []*baseNode

	// Check if we have conditional fields
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
		rootNodes = graph.buildConditionalSchema(schema, basePath, dataContext, addedConstraints, nsd)
	} else {
		rootNodes = graph.buildRegularSchema(schema, basePath, dataContext, addedConstraints, nsd)
	}

	return rootNodes
}

func (graph *ValidationGraph) buildConditionalSchema(schema *SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *NestedSchemaDefinition) []*baseNode {
	var rootNodes []*baseNode

	// Process structured schema with conditional fields
	baseFields := make(map[string]bool)
	conditionalFields := make(map[string]*FieldInclusionCondition)
	allFields := make(map[string]*FieldDefinition)

	// Collect all field entries
	for _, structuredFieldEntry := range nsd.StructuredFieldsArray {
		for _, fieldDef := range structuredFieldEntry.Fields {
			// CORRECTED: Use fieldDef.Name as the key, not the map key.
			fieldName := fieldDef.Name
			allFields[fieldName] = fieldDef

			if structuredFieldEntry.When == nil {
				baseFields[fieldName] = true
			} else {
				conditionalFields[fieldName] = structuredFieldEntry.When
			}
		}
	}

	// Create conditional unexpected fields node
	unexpectedNode := graph.createConditionalUnexpectedFieldsNode(basePath, conditionalFields, baseFields)
	graph.addNode(unexpectedNode)
	rootNodes = append(rootNodes, &unexpectedNode.baseNode)

	// Process all fields with conditional awareness
	var allFieldNodes []*baseNode
	// CORRECTED: The 'allFields' map is now correctly keyed by field name.
	for fieldName, fieldDef := range allFields {
		condition := conditionalFields[fieldName] // This will be nil for base fields
		fieldNodes := graph.buildFieldNodes(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints, condition)
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	// Add schema completion node if needed
	constraintDeps := getDependencyIDs(allFieldNodes)
	schemaConstraintIDs := graph.buildFromConstraints(schema.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

	if len(schemaConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, "schema_completion", schemaConstraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes
}

func (graph *ValidationGraph) buildRegularSchema(schema *SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *NestedSchemaDefinition) []*baseNode {
	var rootNodes []*baseNode

	// Determine fields to process
	fieldsToProcess := schema.Fields
	if nsd != nil && nsd.StructuredFieldsMap != nil {
		fieldsToProcess = nsd.StructuredFieldsMap
	}

	// Create expected fields map
	expectedFields := make(map[string]bool)
	// CORRECTED: Iterate over the field definitions and use the 'Name' property, not the map key.
	for _, fieldDef := range fieldsToProcess {
		expectedFields[fieldDef.Name] = true
	}

	// Create unexpected fields node
	unexpectedNode := graph.createUnexpectedFieldsNode(basePath, expectedFields)
	graph.addNode(unexpectedNode)
	rootNodes = append(rootNodes, &unexpectedNode.baseNode)

	// Process all fields
	var allFieldNodes []*baseNode
	for _, fieldDef := range fieldsToProcess {
		fieldNodes := graph.buildFieldNodes(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints, nil)
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	// Add schema completion node if needed
	constraintDeps := getDependencyIDs(allFieldNodes)
	schemaConstraintIDs := graph.buildFromConstraints(schema.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

	if len(schemaConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, "schema_completion", schemaConstraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes
}

// Unified field processing method
func (graph *ValidationGraph) buildFieldNodes(fieldDef *FieldDefinition, basePath string, baseDeps []string, schema *SchemaDefinition, dataContext any, addedConstraints map[string]bool, condition *FieldInclusionCondition) []*baseNode {
	fieldPath := buildPath(basePath, fieldDef.Name)
	currentDeps := baseDeps
	var nodes []*baseNode

	// Add conditional field node if needed
	if condition != nil {
		conditionalNode := graph.createConditionalFieldNode(basePath, fieldDef.Name, condition, currentDeps)
		graph.addNode(conditionalNode)
		currentDeps = []string{conditionalNode.id}
		nodes = append(nodes, &conditionalNode.baseNode)
	}

	// Add required field check
	if fieldDef.Required != nil && *fieldDef.Required {
		var reqNode ValidationNode
		if condition != nil {
			reqNode = graph.createConditionalRequiredFieldNode(fieldPath, fieldDef.Name, condition, currentDeps)
		} else {
			reqNode = graph.createRequiredFieldNode(fieldPath, fieldDef.Name, currentDeps)
		}
		graph.addNode(reqNode)
		currentDeps = []string{reqNode.GetID()}
		nodes = append(nodes, &baseNode{id: reqNode.GetID(), path: reqNode.GetPath(), deps: reqNode.GetDependencies()})
	}

	// Add type check node
	typeNode := graph.createTypeCheckNode(fieldPath, fieldDef, currentDeps)
	graph.addNode(typeNode)
	currentDeps = []string{typeNode.id}
	nodes = append(nodes, &typeNode.baseNode)

	// Add field-type-specific nodes
	typeSpecificNodes := graph.buildFieldTypeNodes(fieldDef, fieldPath, currentDeps, schema)
	nodes = append(nodes, typeSpecificNodes...)
	if len(typeSpecificNodes) > 0 {
		currentDeps = []string{typeSpecificNodes[len(typeSpecificNodes)-1].id}
	}

	// Add field constraints
	fieldConstraintIDs := graph.buildFromConstraints(fieldDef.Constraints, fieldPath, currentDeps, dataContext, addedConstraints)
	if len(fieldConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(fieldPath, "field_completion", fieldConstraintIDs)
		graph.addNode(completionNode)
		nodes = append(nodes, &completionNode.baseNode)
	}

	return nodes
}

func (graph *ValidationGraph) buildFieldTypeNodes(fieldDef *FieldDefinition, fieldPath string, currentDeps []string, schema *SchemaDefinition) []*baseNode {
	var nodes []*baseNode

	switch fieldDef.Type {
	case FieldTypeEnum:
		enumNode := &EnumValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "enum", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
		}
		graph.addNode(enumNode)
		nodes = append(nodes, &enumNode.baseNode)

	case FieldTypeArray:
		arrayNode := &ArrayValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "array", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		graph.addNode(arrayNode)
		nodes = append(nodes, &arrayNode.baseNode)

	case FieldTypeSet:
		setNode := &SetValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "set", ""), path: fieldPath, deps: currentDeps},
		}
		graph.addNode(setNode)
		nodes = append(nodes, &setNode.baseNode)

	case FieldTypeObject:
		if objectNodes := graph.buildObjectFieldNodes(fieldDef, fieldPath, currentDeps, schema); len(objectNodes) > 0 {
			nodes = append(nodes, objectNodes...)
		}

	case FieldTypeRecord:
		recordNode := &RecordValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		graph.addNode(recordNode)
		nodes = append(nodes, &recordNode.baseNode)

	case FieldTypeUnion:
		unionNode := &UnionValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "union", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
			schema:   schema,
		}
		graph.addNode(unionNode)
		nodes = append(nodes, &unionNode.baseNode)
	}

	return nodes
}

func (graph *ValidationGraph) buildObjectFieldNodes(fieldDef *FieldDefinition, fieldPath string, currentDeps []string, schema *SchemaDefinition) []*baseNode {
	var nodes []*baseNode

	ref, ok := fieldDef.Schema.(NestedSchemaReference)
	if !ok {
		return nodes
	}

	nestedSchemaDef, exists := schema.FindNestedSchema(ref.ID)
	if !exists || nestedSchemaDef.IsStructured == nil || !*nestedSchemaDef.IsStructured {
		return nodes
	}

	// Build reference constraints
	refConstraintIDs := graph.buildFromConstraints(ref.Constraints, fieldPath, currentDeps, nil, make(map[string]bool))
	if len(refConstraintIDs) > 0 {
		currentDeps = append(currentDeps, refConstraintIDs...)
	}

	// Build structured schema
	tempSchema := &SchemaDefinition{
		Name:          nestedSchemaDef.Name,
		Fields:        make(map[string]*FieldDefinition),
		NestedSchemas: schema.NestedSchemas,
		Constraints:   nestedSchemaDef.Constraints,
	}

	// Handle structured fields
	// CORRECTED: Rebuild the Fields map using fieldDef.Name as the key.
	if nestedSchemaDef.StructuredFieldsArray != nil {
		for _, structuredFieldEntry := range nestedSchemaDef.StructuredFieldsArray {
			for _, def := range structuredFieldEntry.Fields {
				tempSchema.Fields[def.Name] = def
			}
		}
	} else if nestedSchemaDef.StructuredFieldsMap != nil {
		for _, def := range nestedSchemaDef.StructuredFieldsMap {
			tempSchema.Fields[def.Name] = def
		}
	}

	nestedNodes := graph.buildFromSchema(tempSchema, fieldPath, nil, make(map[string]bool), nestedSchemaDef)

	markerNode := &NestedSchemaNode{
		baseNode: baseNode{
			id:   buildNodeID(fieldPath, "nested_schema", ""),
			path: fieldPath,
			deps: getDependencyIDs(nestedNodes),
		},
	}
	graph.addNode(markerNode)
	nodes = append(nodes, &markerNode.baseNode)

	return nodes
}

func (graph *ValidationGraph) buildFromConstraints(constraints SchemaConstraint[FieldType], path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
	var ruleDepIDs []string
	for _, rule := range constraints {
		rules := graph.buildFromConstraintRule(rule, path, deps, dataContext, addedConstraints)
		ruleDepIDs = append(ruleDepIDs, rules...)
	}
	return ruleDepIDs
}

func (graph *ValidationGraph) dependenciesSatisfied(nodeID string, visited map[string]bool) bool {
	deps := graph.dependencies[nodeID]
	for _, depID := range deps {
		if !visited[depID] {
			return false
		}
	}
	return true
}

func (graph *ValidationGraph) buildFromConstraintRule(rule SchemaConstraintRule[FieldType], path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
	var ruleDepIDs []string
	switch r := rule.(type) {
	case Constraint[FieldType]:
		nodeID := buildNodeID(path, "constraint", r.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID}
		}
		fieldDeps := deps
		if r.Field != nil && *r.Field != "" {
			targetPath := buildPath(path, *r.Field)
			fieldDeps = append(fieldDeps, buildNodeID(targetPath, "type_check", ""))
		}
		node := &ConstraintNode{
			baseNode:   baseNode{id: nodeID, path: path, deps: fieldDeps},
			constraint: r,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	case ConstraintGroup[FieldType]:
		var memberDeps []string
		for _, memberRule := range r.Rules {
			memberDeps = append(memberDeps, graph.buildFromConstraintRule(memberRule, path, deps, dataContext, addedConstraints)...)
		}
		nodeID := buildNodeID(path, "constraint_group", r.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID}
		}
		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: path, deps: memberDeps},
			group:    r,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}
	return ruleDepIDs
}

func (graph *ValidationGraph) traverse(fmap *FunctionMap, document map[string]any, loose bool) ([]common.Issue, bool) {
	ctx := &ValidationContext{
		RootData:    document,
		Data:        document,
		Results:     make(map[string]*NodeResult),
		FunctionMap: fmap,
	}

	visited := make(map[string]bool)
	var allIssues []common.Issue

	nodeIDs := make([]string, 0, len(graph.nodes))
	for id := range graph.nodes {
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

			if graph.dependenciesSatisfied(nodeID, visited) {
				node := graph.nodes[nodeID]
				ctx.Data = ctx.RootData

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
			allIssues = append(allIssues, common.Issue{Code: "CIRCULAR_DEPENDENCY", Message: "Circular dependency detected"})
			break
		}
		toProcess = nextRound
	}

	if loose {
		filteredIssues := make([]common.Issue, 0, len(allIssues))
		for _, issue := range allIssues {
			if issue.Code != "REQUIRED_FIELD_MISSING" {
				filteredIssues = append(filteredIssues, issue)
			}
		}
		allIssues = filteredIssues
	}

	// Sort issues for consistent test results
	sort.Slice(allIssues, func(i, j int) bool {
		if allIssues[i].Path != allIssues[j].Path {
			return allIssues[i].Path < allIssues[j].Path
		}
		return allIssues[i].Code < allIssues[j].Code
	})

	return allIssues, len(allIssues) == 0
}

// =============================================================================
// MAIN VALIDATOR CONSTRUCTION
// =============================================================================

func NewDocumentValidator(schema *SchemaDefinition, fmap *FunctionMap) (*DocumentValidator, error) {
	// Validate schema before building the graph
	if err := schema.Validate(); err != nil {
		return nil, &SchemaError{
			Operation: "NewDocumentValidator",
			Message:   "schema validation failed",
			Cause:     err,
		}
	}

	graph := newValidationGraph()
	addedConstraints := make(map[string]bool)
	graph.buildFromSchema(schema, "", nil, addedConstraints, nil)

	// Perform cycle detection after graph construction
	for nodeID := range graph.nodes {
		if graph.visitedState[nodeID] == dfsUnvisited {
			if graph.dfsCheck(nodeID) {
				return nil, &SchemaError{
					Operation: "NewDocumentValidator",
					Message:   fmt.Sprintf("circular dependency detected in validation graph involving node: %s", nodeID),
					Cause:     errors.New("circular dependency detected"), // No specific error variable for this
				}
			}
		}
	}

	return &DocumentValidator{
		graph: graph,
		fmap:  fmap,
	}, nil
}

// =============================================================================
// GRAPH TRAVERSAL AND VALIDATION
// =============================================================================

func (v *DocumentValidator) Validate(document map[string]any, loose bool) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, loose)
}

// =============================================================================
// NODE EXECUTION IMPLEMENTATIONS
// =============================================================================

func (n *UnexpectedFieldsNode) Execute(ctx *ValidationContext) *NodeResult {
	currentData, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	dataMap, ok := currentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for unexpected field check", Path: n.path}}}
	}

	var issues []common.Issue
	for key := range dataMap {
		if !n.expectedFields[key] {
			issues = append(issues, common.Issue{Code: "UNEXPECTED_FIELD", Message: fmt.Sprintf("Unexpected field '%s'", key), Path: buildPath(n.path, key)})
		}
	}
	return &NodeResult{Success: len(issues) == 0, Issues: issues}
}

func (n *ConditionalUnexpectedFieldsNode) Execute(ctx *ValidationContext) *NodeResult {
	currentData, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	dataMap, ok := currentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for conditional field check", Path: n.path}}}
	}

	var issues []common.Issue

	for fieldName := range dataMap {
		isExpected := false

		if n.baseFields[fieldName] {
			isExpected = true
		} else if condition, exists := n.conditionalFields[fieldName]; exists {
			if condition.Evaluate(dataMap) {
				isExpected = true
			}
		}

		if !isExpected {
			issues = append(issues, common.Issue{
				Code:    "UNEXPECTED_FIELD",
				Message: fmt.Sprintf("Unexpected field '%s'", fieldName),
				Path:    buildPath(n.path, fieldName),
			})
		}
	}

	return &NodeResult{Success: len(issues) == 0, Issues: issues}
}

func (n *RequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentPath := getScopedPath(n.path)
	parentData, exists := utils.GetValueByPath(ctx.RootData, parentPath)
	if !exists {
		// If parent doesn't exist, a required check on a child is moot, but for required at root, we need to check.
		if parentPath == "" {
			// This case is handled by checking the document directly.
		} else {
			return &NodeResult{Success: true}
		}
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_DATA_STRUCTURE", Message: "Cannot check for required fields on non-object parent", Path: parentPath}}}
	}

	fieldName := n.path[len(parentPath)+1:]
	if parentPath == "" {
		fieldName = n.path
	}

	if _, exists := dataMap[fieldName]; !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
	}
	return &NodeResult{Success: true}
}

func (n *ConditionalRequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentPath := getScopedPath(n.path)
	parentData, exists := utils.GetValueByPath(ctx.RootData, parentPath)
	if !exists {
		return &NodeResult{Success: true}
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_DATA_STRUCTURE", Message: "Cannot check for required fields on non-object parent", Path: parentPath}}}
	}

	conditionMet := n.condition.Evaluate(dataMap)
	if conditionMet {
		fieldName := n.path[len(parentPath)+1:]
		if parentPath == "" {
			fieldName = n.path
		}

		if _, exists := dataMap[fieldName]; !exists {
			return &NodeResult{Success: false, Issues: []common.Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", fieldName), Path: n.path}}}
		}
	}

	return &NodeResult{Success: true}
}

func (n *ConditionalFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentData, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	dataMap, ok := parentData.(map[string]any)
	if !ok {
		// Not an object, so can't evaluate condition or check for field.
		return &NodeResult{Success: true}
	}

	conditionMet := n.condition.Evaluate(dataMap)
	if !conditionMet {
		if _, fieldExists := dataMap[n.fieldName]; fieldExists {
			return &NodeResult{
				Success: false,
				Issues: []common.Issue{{
					Code:    "CONDITIONAL_FIELD_PRESENT",
					Message: fmt.Sprintf("Field '%s' should not be present when %s != %v", n.fieldName, n.condition.Field, n.condition.Value),
					Path:    buildPath(n.path, n.fieldName),
				}},
			}
		}
		// Condition not met and field is absent, which is correct.
		return &NodeResult{Success: true}
	}

	// Condition met, so field is allowed to be present. This node doesn't check for its presence, only its absence when the condition is false.
	return &NodeResult{Success: true}
}

func (n *TypeCheckNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	coercedValue, _ := n.fieldDef.Type.Coerce(value)
	valid := n.fieldDef.ValidateType(coercedValue)

	if !valid {
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Expected %s, got %T", n.fieldDef.Type, value), Path: n.path},
		}, Value: value}
	}
	return &NodeResult{Success: true, Value: coercedValue}
}

func (n *EnumValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}
	for _, allowedValue := range n.fieldDef.Values {
		if reflect.DeepEqual(value, allowedValue) {
			return &NodeResult{Success: true}
		}
	}
	return &NodeResult{Success: false, Issues: []common.Issue{{Code: "ENUM_VIOLATION", Message: fmt.Sprintf("Value must be one of: %v", n.fieldDef.Values), Path: n.path}}}
}

func (n *ArrayValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	return n.executeArrayValidation(ctx)
}

func (n *ArrayValidationNode) executeArrayValidation(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	items, ok := value.([]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected array", Path: n.path}}}
	}

	if n.fieldDef.ItemsType == nil {
		return &NodeResult{Success: true}
	}

	var allIssues []common.Issue
	itemType := *n.fieldDef.ItemsType

	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)
		itemIssues := n.validateArrayItem(item, itemType, itemPath, ctx)
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *ArrayValidationNode) validateArrayItem(item any, itemType FieldType, itemPath string, ctx *ValidationContext) []common.Issue {
	tempRootField := FieldDefinition{
		Name:      "item",
		Type:      itemType,
		Schema:    n.fieldDef.Schema,
		ItemsType: nil,
	}

	tempSchema := &SchemaDefinition{
		Name:          "temp_array_item_check",
		Fields:        map[string]*FieldDefinition{"item": &tempRootField},
		NestedSchemas: n.schema.NestedSchemas,
	}

	validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
	if err != nil {
		return []common.Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: err.Error(), Path: itemPath}}
	}

	itemIssues, _ := validator.Validate(map[string]any{"item": item}, false)

	for j := range itemIssues {
		itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
	}

	return itemIssues
}

func (n *RecordValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	return n.executeRecordValidation(ctx)
}

func (n *RecordValidationNode) executeRecordValidation(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	recordMap, ok := value.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for record", Path: n.path}}}
	}

	if n.fieldDef.Schema == nil {
		return &NodeResult{Success: true}
	}

	ref, ok := n.fieldDef.Schema.(NestedSchemaReference)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_RECORD_SCHEMA", Message: "Record schema must be a NestedSchemaReference", Path: n.path}}}
	}

	nestedDef, exists := n.schema.FindNestedSchema(ref.ID)
	if !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "NESTED_SCHEMA_NOT_FOUND", Message: fmt.Sprintf("Nested schema '%s' not found for record items", ref.ID), Path: n.path}}}
	}

	var allIssues []common.Issue

	for key, itemValue := range recordMap {
		itemPath := buildPath(n.path, key)
		itemIssues := n.validateRecordItem(itemValue, itemPath, ref, nestedDef, ctx)
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *RecordValidationNode) validateRecordItem(itemValue any, itemPath string, ref NestedSchemaReference, nestedDef *NestedSchemaDefinition, ctx *ValidationContext) []common.Issue {
	var tempRootField FieldDefinition
	if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
		tempRootField = FieldDefinition{Name: "item", Type: FieldTypeObject, Schema: ref}
	} else if nestedDef.Type != nil {
		tempRootField = FieldDefinition{Name: "item", Type: *nestedDef.Type, Schema: nestedDef.Schema, ItemsType: nestedDef.ItemsType}
	} else {
		return nil
	}

	tempSchema := &SchemaDefinition{
		Name:          "temp_record_item_check",
		Fields:        map[string]*FieldDefinition{"item": &tempRootField},
		NestedSchemas: n.schema.NestedSchemas,
	}

	validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
	if err != nil {
		return []common.Issue{{Code: "VALIDATOR_CREATION_ERROR", Message: fmt.Sprintf("Failed to create validator for item at path '%s': %v", itemPath, err), Path: itemPath}}
	}

	itemIssues, _ := validator.Validate(map[string]any{"item": itemValue}, false)

	for j := range itemIssues {
		itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
	}

	return itemIssues
}

func (n *SetValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	items, ok := value.([]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected array for set", Path: n.path}}}
	}

	seen := make(map[string]bool)
	for i, item := range items {
		key := fmt.Sprintf("%v", item)
		if seen[key] {
			return &NodeResult{Success: false, Issues: []common.Issue{{Code: "SET_DUPLICATE", Message: fmt.Sprintf("Duplicate value found in set at index %d", i), Path: n.path}}}
		}
		seen[key] = true
	}
	return &NodeResult{Success: true}
}

func (n *NestedSchemaNode) Execute(ctx *ValidationContext) *NodeResult {
	return &NodeResult{Success: true}
}

func (n *UnionValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	return n.executeUnionValidation(ctx)
}

func (n *UnionValidationNode) executeUnionValidation(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	schemas, ok := n.fieldDef.Schema.([]NestedSchemaReference)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_UNION_SCHEMA", Path: n.path}}}
	}

	var specificConstraintViolations []common.Issue

	for _, schemaRef := range schemas {
		if matched, constraintViolations := n.tryUnionSchema(value, schemaRef, ctx); matched {
			return &NodeResult{Success: true}
		} else if len(constraintViolations) > 0 {
			specificConstraintViolations = append(specificConstraintViolations, constraintViolations...)
		}
	}

	if len(specificConstraintViolations) > 0 {
		return &NodeResult{Success: false, Issues: specificConstraintViolations}
	}

	return &NodeResult{Success: false, Issues: []common.Issue{{Code: "UNION_NO_MATCH", Message: "Value does not match any of the union schemas", Path: n.path}}}
}

func (n *UnionValidationNode) tryUnionSchema(value any, schemaRef NestedSchemaReference, ctx *ValidationContext) (bool, []common.Issue) {
	nestedDef, exists := n.schema.FindNestedSchema(schemaRef.ID)
	if !exists {
		return false, nil
	}

	var tempRootField FieldDefinition
	if nestedDef.IsStructured != nil && *nestedDef.IsStructured {
		tempRootField = FieldDefinition{Name: "root", Type: FieldTypeObject, Schema: schemaRef}
	} else if nestedDef.Type != nil {
		tempRootField = FieldDefinition{
			Name:        "root",
			Type:        *nestedDef.Type,
			Schema:      nestedDef.Schema,
			ItemsType:   nestedDef.ItemsType,
			Constraints: nestedDef.Constraints,
		}
	} else {
		return false, nil
	}

	tempSchema := &SchemaDefinition{
		Name:          "temp_union_check",
		Fields:        map[string]*FieldDefinition{"root": &tempRootField},
		NestedSchemas: n.schema.NestedSchemas,
	}

	validator, err := NewDocumentValidator(tempSchema, ctx.FunctionMap)
	if err != nil {
		return false, nil
	}

	itemIssues, matched := validator.Validate(map[string]any{"root": value}, false)

	if matched {
		return true, nil
	}

	// Rewrite paths
	for i := range itemIssues {
		itemIssues[i].Path = strings.Replace(itemIssues[i].Path, "root", n.path, 1)
	}

	// Check for structural vs constraint issues
	hasStructuralIssues := false
	var constraintViolations []common.Issue

	for _, issue := range itemIssues {
		switch issue.Code {
		case "TYPE_MISMATCH", "UNEXPECTED_FIELD", "REQUIRED_FIELD_MISSING", "ENUM_VIOLATION":
			hasStructuralIssues = true
		case "CONSTRAINT_VIOLATION":
			constraintViolations = append(constraintViolations, issue)
		}
	}

	if !hasStructuralIssues && len(constraintViolations) > 0 {
		return false, constraintViolations
	}

	return false, nil
}

func (n *ConstraintNode) Execute(ctx *ValidationContext) *NodeResult {
	predicateFunc, exists := (*ctx.FunctionMap)[n.constraint.Predicate]
	if !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "MISSING_PREDICATE", Message: fmt.Sprintf("Predicate '%s' not found", n.constraint.Predicate), Path: n.path}}}
	}

	predicate, ok := predicateFunc.(func(PredicateParams[any]) bool)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_PREDICATE_TYPE", Message: fmt.Sprintf("Predicate '%s' has invalid type", n.constraint.Predicate), Path: n.path}}}
	}

	predicateData, dataExists := utils.GetValueByPath(ctx.RootData, n.path)
	if !dataExists {
		return &NodeResult{Success: true}
	}

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

		issuePath := n.path
		if n.constraint.Field != nil && *n.constraint.Field != "" {
			if _, ok := predicateData.(map[string]any); ok {
				if n.path != "" {
					issuePath = buildPath(n.path, *n.constraint.Field)
				} else {
					issuePath = *n.constraint.Field
				}
			}
		}

		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "CONSTRAINT_VIOLATION", Message: message, Path: issuePath}}}
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

	if ok, err := n.group.Operator.Evaluate(results); !ok || err != nil {
		groupViolationIssue := common.Issue{Code: "CONSTRAINT_GROUP_VIOLATION", Message: fmt.Sprintf("Constraint group '%s' failed", n.group.Name), Path: n.path}
		return &NodeResult{Success: false, Issues: []common.Issue{groupViolationIssue}}
	}

	return &NodeResult{Success: true}
}

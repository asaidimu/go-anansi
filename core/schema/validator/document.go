package validator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

type DocumentValidator struct {
	fmap  *schema.FunctionMap
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
	FunctionMap *schema.FunctionMap
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
	condition *schema.FieldInclusionCondition
}

type ConditionalFieldNode struct {
	baseNode
	condition *schema.FieldInclusionCondition
	fieldName string
}

type ConditionalUnexpectedFieldsNode struct {
	baseNode
	conditionalFields map[string]*schema.FieldInclusionCondition
	baseFields        map[string]bool
}

type TypeCheckNode struct {
	baseNode
	fieldDef *schema.FieldDefinition
}

type EnumValidationNode struct {
	baseNode
	fieldDef *schema.FieldDefinition
}

type ArrayValidationNode struct {
	baseNode
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
	graph    *ValidationGraph
}

type RecordValidationNode struct {
	baseNode
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
	graph    *ValidationGraph
}

type SetValidationNode struct {
	baseNode
}

type NestedSchemaNode struct {
	baseNode
}

type UnionValidationNode struct {
	baseNode
	fieldDef *schema.FieldDefinition
	schema   *schema.SchemaDefinition
	graphs   []*ValidationGraph
}

type ConstraintNode struct {
	baseNode
	constraint schema.Constraint
}

type ConstraintGroupNode struct {
	baseNode
	group     schema.ConstraintGroup
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

func (graph *ValidationGraph) createConditionalUnexpectedFieldsNode(path string, conditionalFields map[string]*schema.FieldInclusionCondition, baseFields map[string]bool) *ConditionalUnexpectedFieldsNode {
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

func (graph *ValidationGraph) createConditionalRequiredFieldNode(fieldPath string, fieldName string, condition *schema.FieldInclusionCondition, deps []string) *ConditionalRequiredFieldNode {
	return &ConditionalRequiredFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional_required", ""), path: fieldPath, deps: deps},
		fieldName: fieldName,
		condition: condition,
	}
}

func (graph *ValidationGraph) createConditionalFieldNode(fieldPath string, fieldName string, condition *schema.FieldInclusionCondition, deps []string) *ConditionalFieldNode {
	return &ConditionalFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "conditional", ""), path: fieldPath, deps: deps},
		condition: condition,
		fieldName: fieldName,
	}
}

func (graph *ValidationGraph) createTypeCheckNode(fieldPath string, fieldDef *schema.FieldDefinition, deps []string) *TypeCheckNode {
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

func (graph *ValidationGraph) buildFromSchema(schema *schema.SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *schema.NestedSchemaDefinition) ([]*baseNode, error) {
	var rootNodes []*baseNode

	// Check if we have conditional fields
	hasConditionalFields := nsd != nil && nsd.Fields.FieldsArray != nil &&
		func() bool {
			for _, entry := range nsd.Fields.FieldsArray {
				if entry.When != nil {
					return true
				}
			}
			return false
		}()

	var err error
	if hasConditionalFields {
		rootNodes, err = graph.buildConditionalSchema(schema, basePath, dataContext, addedConstraints, nsd)
	} else {
		rootNodes, err = graph.buildRegularSchema(schema, basePath, dataContext, addedConstraints, nsd)
	}

	if err != nil {
		return nil, err
	}
	return rootNodes, nil
}

func (graph *ValidationGraph) buildConditionalSchema(sc *schema.SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *schema.NestedSchemaDefinition) ([]*baseNode, error) {
	var rootNodes []*baseNode

	// Process structured schema with conditional fields
	baseFields := make(map[string]bool)
	conditionalFields := make(map[string]*schema.FieldInclusionCondition)
	allFields := make(map[string]*schema.FieldDefinition)

	// Collect all field entries
	for _, structuredFieldEntry := range nsd.Fields.FieldsArray {
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
	for fieldName, fieldDef := range allFields {
		condition := conditionalFields[fieldName] // This will be nil for base fields
		fieldNodes, err := graph.buildFieldNodes(fieldDef, basePath, []string{unexpectedNode.id}, sc, dataContext, addedConstraints, condition)
		if err != nil {
			return nil, err
		}
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	// Add schema completion node if needed
	constraintDeps := getDependencyIDs(allFieldNodes)
	schemaConstraintIDs := graph.buildFromConstraints(sc.Constraints, basePath, constraintDeps, dataContext, addedConstraints)

	if len(schemaConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, "schema_completion", schemaConstraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes, nil
}

func (graph *ValidationGraph) buildRegularSchema(schema *schema.SchemaDefinition, basePath string, dataContext any, addedConstraints map[string]bool, nsd *schema.NestedSchemaDefinition) ([]*baseNode, error) {
	var rootNodes []*baseNode

	// Determine fields to process
	fieldsToProcess := schema.Fields
	if nsd != nil && nsd.Fields.FieldsMap != nil {
		fieldsToProcess = nsd.Fields.FieldsMap
	}

	// Create expected fields map
	expectedFields := make(map[string]bool)
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
		fieldNodes, err := graph.buildFieldNodes(fieldDef, basePath, []string{unexpectedNode.id}, schema, dataContext, addedConstraints, nil)
		if err != nil {
			return nil, err
		}
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

	return rootNodes, nil
}

// Unified field processing method
func (graph *ValidationGraph) buildFieldNodes(fieldDef *schema.FieldDefinition, basePath string, baseDeps []string, sc *schema.SchemaDefinition, dataContext any, addedConstraints map[string]bool, condition *schema.FieldInclusionCondition) ([]*baseNode, error) {
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
	typeSpecificNodes, err := graph.buildFieldTypeNodes(fieldDef, fieldPath, currentDeps, sc)
	if err != nil {
		return nil, err
	}
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

	return nodes, nil
}

// TODO: 2025-12-08. Review this method
// NOTE:
// 1. Refactor required, go iteration is not deterministic, we need to check
// on this when formatting/parsing issue paths
// 2. Cache subgraphs
func (graph *ValidationGraph) buildFieldTypeNodes(fieldDef *schema.FieldDefinition, fieldPath string, currentDeps []string, sc *schema.SchemaDefinition) ([]*baseNode, error) {
	var node ValidationNode
	var nodes []*baseNode
	var err error

	switch fieldDef.Type {
	case schema.FieldTypeEnum:
		node = &EnumValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "enum", ""), path: fieldPath, deps: currentDeps},
			fieldDef: fieldDef,
		}

	case schema.FieldTypeArray:
		node, err = graph.buildArrayNode(fieldDef, fieldPath, currentDeps, sc)

	case schema.FieldTypeSet:
		node = &SetValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "set", ""), path: fieldPath, deps: currentDeps},
		}

	case schema.FieldTypeRecord:
		node, err = graph.buildRecordNode(fieldDef, fieldPath, currentDeps, sc)

	case schema.FieldTypeUnion:
		node, err = graph.buildUnionNode(fieldDef, fieldPath, currentDeps, sc)

	case schema.FieldTypeObject:
		objectNodes, err := graph.buildObjectFieldNodes(fieldDef, fieldPath, currentDeps, sc)
		if err != nil {
			return nil, err
		}
		if len(objectNodes) > 0 {
			nodes = append(nodes, objectNodes...)
		}
		return nodes, nil
	}

	if err != nil {
		return nil, err
	}

	if node != nil {
		graph.addNode(node)
		if bn, ok := node.(interface {
			GetID() string
			GetPath() string
			GetDependencies() []string
		}); ok {
			nodes = append(nodes, &baseNode{id: bn.GetID(), path: bn.GetPath(), deps: bn.GetDependencies()})
		}
	}

	return nodes, nil
}

func (graph *ValidationGraph) buildObjectFieldNodes(fieldDef *schema.FieldDefinition, fieldPath string, currentDeps []string, sc *schema.SchemaDefinition) ([]*baseNode, error) {
	var nodes []*baseNode

	ref, ok := fieldDef.Schema.(schema.NestedSchemaReference)
	if !ok {
		return nil, schema.ErrInvalidSchema.WithMessage("Could not find a nested reference")
	}

	nestedSchemaDef, exists := sc.FindNestedSchemaById(ref.ID)
	if !exists || !nestedSchemaDef.IsStructured() {
		return nil, schema.ErrInvalidSchema.WithMessage(fmt.Sprintf("Could not resolve nested schema with reference id `%s`", ref.ID))
	}

	// Build reference constraints
	refConstraintIDs := graph.buildFromConstraints(ref.Constraints, fieldPath, currentDeps, nil, make(map[string]bool))
	if len(refConstraintIDs) > 0 {
		currentDeps = append(currentDeps, refConstraintIDs...)
	}

	// Build structured schema
	tempSchema := &schema.SchemaDefinition{
		Name:          nestedSchemaDef.Name,
		Fields:        make(map[string]*schema.FieldDefinition),
		NestedSchemas: sc.NestedSchemas,
		Constraints:   nestedSchemaDef.Constraints,
	}

	// Handle structured fields
	if nestedSchemaDef.Fields.FieldsArray != nil {
		for _, structuredFieldEntry := range nestedSchemaDef.Fields.FieldsArray {
			for _, def := range structuredFieldEntry.Fields {
				tempSchema.Fields[def.Name] = def
			}
		}
	} else if nestedSchemaDef.Fields.FieldsMap != nil {
		for _, def := range nestedSchemaDef.Fields.FieldsMap {
			tempSchema.Fields[def.Name] = def
		}
	}

	nestedNodes, err := graph.buildFromSchema(tempSchema, fieldPath, nil, make(map[string]bool), nestedSchemaDef)

	if err != nil {
		return nil, err
	}
	markerNode := &NestedSchemaNode{
		baseNode: baseNode{
			id:   buildNodeID(fieldPath, "nested_schema", ""),
			path: fieldPath,
			deps: getDependencyIDs(nestedNodes),
		},
	}
	graph.addNode(markerNode)
	nodes = append(nodes, &markerNode.baseNode)

	return nodes, nil
}

func (graph *ValidationGraph) buildFromConstraints(constraints schema.SchemaConstraint, path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
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

// createSubGraph builds a standalone validation graph for a single field definition.
// This is used for complex types (Array, Record, Union) that need to validate their
// children as if they were root elements of a new schema.
func (graph *ValidationGraph) createSubGraph(
	rootFieldName string,
	rootFieldDef *schema.FieldDefinition,
	parentSchema *schema.SchemaDefinition,
	debugName string,
) (*ValidationGraph, error) {
	tempSchema := &schema.SchemaDefinition{
		Name:          debugName,
		Fields:        map[string]*schema.FieldDefinition{rootFieldName: rootFieldDef},
		NestedSchemas: parentSchema.NestedSchemas,
	}

	subGraph := newValidationGraph()
	addedConstraints := make(map[string]bool)

	// We pass nil for dataContext as sub-graphs usually start fresh validation contexts
	if _, err := subGraph.buildFromSchema(tempSchema, "", nil, addedConstraints, nil); err != nil {
		return nil, err
	}

	return subGraph, nil
}

// buildArrayNode handles the specific logic for array validation construction
func (graph *ValidationGraph) buildArrayNode(fieldDef *schema.FieldDefinition, fieldPath string, deps []string, sc *schema.SchemaDefinition) (ValidationNode, error) {
	// Construct the definition for the items inside the array
	tempRootField := &schema.FieldDefinition{
		Name:      "item",
		Type:      *fieldDef.ItemsType,
		Schema:    fieldDef.Schema,
		ItemsType: nil, // Recursion handled by the new subgraph
	}

	// Create the isolated validation environment for the array items
	arrayGraph, err := graph.createSubGraph("item", tempRootField, sc, "temp_array_item_check")
	if err != nil {
		return nil, err
	}

	return &ArrayValidationNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "array", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graph:    arrayGraph,
	}, nil
}

// buildRecordNode handles the specific logic for record validation construction
func (graph *ValidationGraph) buildRecordNode(fieldDef *schema.FieldDefinition, fieldPath string, deps []string, sc *schema.SchemaDefinition) (ValidationNode, error) {
	ref, ok := fieldDef.Schema.(schema.NestedSchemaReference)
	if fieldDef.Schema != nil && !ok {
		return nil, schema.ErrInvalidSchema.WithMessage("Record schema must be a NestedSchemaReference")
	}

	nested, ok := sc.FindNestedSchemaById(ref.ID)
	if fieldDef.Schema != nil && !ok {
		return nil, schema.ErrInvalidSchema.WithMessage(fmt.Sprintf("Nested schema '%s' not found for record items", ref.ID))
	}

	var recordGraph *ValidationGraph
	var err error

	// If we have a nested schema, build the subgraph for the record values
	if nested != nil {
		var tempRootField *schema.FieldDefinition

		if nested.IsStructured() {
			tempRootField = &schema.FieldDefinition{Name: "item", Type: schema.FieldTypeObject, Schema: ref}
		} else if nested.Type != nil {
			tempRootField = &schema.FieldDefinition{Name: "item", Type: *nested.Type, Schema: nested.Schema, ItemsType: nested.ItemsType}
		}

		if tempRootField != nil {
			recordGraph, err = graph.createSubGraph("item", tempRootField, sc, "temp_record_item_check")
			if err != nil {
				return nil, err
			}
		}
	}

	return &RecordValidationNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graph:    recordGraph,
	}, nil
}

// buildUnionNode handles the specific logic for union validation construction
func (graph *ValidationGraph) buildUnionNode(fieldDef *schema.FieldDefinition, fieldPath string, deps []string, sc *schema.SchemaDefinition) (ValidationNode, error) {
	refs, ok := fieldDef.Schema.([]schema.NestedSchemaReference)
	if !ok {
		return nil, schema.ErrInvalidSchema.WithMessage("Union schema must be an array of NestedSchemaReference")
	}

	graphs := make([]*ValidationGraph, 0, len(refs))

	for _, ref := range refs {
		nestedDef, exists := sc.FindNestedSchemaById(ref.ID)
		if !exists {
			return nil, schema.ErrInvalidSchema.WithMessage(fmt.Sprintf("Nested schema '%s' not found for union option", ref.ID))
		}

		var tempRootField *schema.FieldDefinition

		// Logic to wrap the nested definition into a field for the temporary graph
		if nestedDef.IsStructured() {
			tempRootField = &schema.FieldDefinition{Name: "root", Type: schema.FieldTypeObject, Schema: ref}
		} else if nestedDef.Type != nil {
			tempRootField = &schema.FieldDefinition{
				Name:        "root",
				Type:        *nestedDef.Type,
				Schema:      nestedDef.Schema,
				ItemsType:   nestedDef.ItemsType,
				Constraints: nestedDef.Constraints,
			}
		} else {
			return nil, schema.ErrInvalidSchema.WithMessage(fmt.Sprintf("Invalid nested schema for '%s'", ref.ID))
		}

		unionGraph, err := graph.createSubGraph("root", tempRootField, sc, fmt.Sprintf("temp_union_check_%s", ref.ID))
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, unionGraph)
	}

	return &UnionValidationNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "union", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graphs:   graphs,
	}, nil
}

func (graph *ValidationGraph) buildFromConstraintRule(rule schema.ConstraintRule, path string, deps []string, dataContext any, addedConstraints map[string]bool) []string {
	var ruleDepIDs []string
	if rule.Constraint != nil {
		r := rule.Constraint
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
			constraint: *r,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	} else if rule.ConstraintGroup != nil {
		r := rule.ConstraintGroup
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
			group:    *r,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}
	return ruleDepIDs
}

func (graph *ValidationGraph) traverse(fmap *schema.FunctionMap, document map[string]any, loose bool) ([]common.Issue, bool) {
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

func NewDocumentValidator(sc *schema.SchemaDefinition, fmap *schema.FunctionMap) (*DocumentValidator, error) {
	// Validate schema before building the graph
	if err := sc.Validate(); err != nil {
		return nil, schema.ErrValidatorSchemaValidationFailed.WithCause(err).
			WithOperation("schema.NewDocumentValidator").
			WithMessage("schema validation failed during validator creation")
	}

	graph := newValidationGraph()
	addedConstraints := make(map[string]bool)
	if _, err := graph.buildFromSchema(sc, "", nil, addedConstraints, nil); err != nil {
		return nil, err
	}

	// Perform cycle detection after graph construction
	for nodeID := range graph.nodes {
		if graph.visitedState[nodeID] == dfsUnvisited {
			if graph.dfsCheck(nodeID) {
				return nil, schema.ErrValidatorCircularDependency.
					WithOperation("schema.NewDocumentValidator").
					WithMessage(fmt.Sprintf("circular dependency detected in validation graph involving node: %s", nodeID))
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

	dataMap, ok := utils.GetMapStringAny(currentData)
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

	dataMap, ok := utils.GetMapStringAny(currentData)
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

	dataMap, ok := utils.GetMapStringAny(parentData)
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

	dataMap, ok := utils.GetMapStringAny(parentData)
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

	dataMap, ok := utils.GetMapStringAny(parentData)
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

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected array", Path: n.path}}}
	}

	if n.fieldDef.ItemsType == nil {
		return &NodeResult{Success: true}
	}

	var allIssues []common.Issue

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, false)
		for j := range itemIssues {
			itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
		}
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *RecordValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	return n.executeRecordValidation(ctx)
}

func (n *RecordValidationNode) executeRecordValidation(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	recordMap, ok := utils.GetMapStringAny(value)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for record", Path: n.path}}}
	}

	if n.graph == nil {
		return &NodeResult{Success: true}
	}

	var allIssues []common.Issue
	for key, item := range recordMap {
		itemPath := buildPath(n.path, key)
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, false)
		for j := range itemIssues {
			itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
		}
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *SetValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected array for set", Path: n.path}}}
	}

	seen := make(map[string]bool)
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
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

	var specificConstraintViolations []common.Issue
	for _, graph := range n.graphs {
		itemIssues, matched := graph.traverse(ctx.FunctionMap, map[string]any{"root": value}, false)
		if matched {
			return &NodeResult{Success: true}
		}

		for i := range itemIssues {
			itemIssues[i].Path = strings.Replace(itemIssues[i].Path, "root", n.path, 1)
		}

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
			specificConstraintViolations = append(specificConstraintViolations, constraintViolations...)
		}
	}

	if len(specificConstraintViolations) > 0 {
		return &NodeResult{Success: false, Issues: specificConstraintViolations}
	}

	return &NodeResult{Success: false, Issues: []common.Issue{{Code: "UNION_NO_MATCH", Message: "Value does not match any of the union schemas", Path: n.path}}}
}

func (n *ConstraintNode) Execute(ctx *ValidationContext) *NodeResult {
	predicateFunc, exists := (*ctx.FunctionMap)[n.constraint.Predicate]
	if !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "MISSING_PREDICATE", Message: fmt.Sprintf("Predicate '%s' not found", n.constraint.Predicate), Path: n.path}}}
	}

	predicate, ok := predicateFunc.(func(schema.PredicateParams[any]) bool)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_PREDICATE_TYPE", Message: fmt.Sprintf("Predicate '%s' has invalid type", n.constraint.Predicate), Path: n.path}}}
	}

	predicateData, dataExists := utils.GetValueByPath(ctx.RootData, n.path)
	if !dataExists {
		return &NodeResult{Success: true}
	}

	params := schema.PredicateParams[any]{
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
			if _, ok := utils.GetMapStringAny(predicateData); ok {
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

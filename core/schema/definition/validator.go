package definition

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

type SchemaConstraint map[ConstraintId]Constraint

// ValidationConfig holds configuration for validation behavior
type ValidationConfig struct {
	MaxDepth int            // Maximum nesting depth for circular references
	Mode     ValidationMode // Validation strictness mode
}

// DefaultValidationConfig returns sensible defaults
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxDepth: 20,
		Mode:     ValidationModeStrict,
	}
}

type DocumentValidator struct {
	fmap   PredicateMap
	graph  *ValidationGraph
	config ValidationConfig
}

// ValidationMode defines the strictness level for validation operations.
type ValidationMode byte

const (
	// ValidationModeStrict validates all fields and applies all constraints,
	// returning issues for missing required fields, type mismatches, and unexpected fields.
	ValidationModeStrict ValidationMode = iota + 1

	// ValidationModePartialStrict skips validation for missing required fields if they are not present in the document,
	// but still validates present fields for type mismatches, unexpected fields, and constraints.
	ValidationModePartialStrict

	// ValidationModeLoose skips validation for missing required fields and unexpected fields.
	// It only validates present fields for type mismatches and constraints.
	ValidationModeLoose
)

var success = &NodeResult{Success: true}
var skipped = &NodeResult{Success: true, Skipped: true}

// ValidationNode represents a single validation operation in the graph.
type ValidationNode interface {
	Execute(ctx *ValidationContext) *NodeResult
	GetDependencies() []int
	GetID() int
	GetPath() string
	GetPathParts() []string
}

// NodeResult holds the outcome of a single node's execution.
type NodeResult struct {
	Issues  []common.Issue
	Success bool
	Skipped bool
}

// ValidationContext holds the state during a validation traversal.
type ValidationContext struct {
	RootData    any
	Data        any
	FunctionMap PredicateMap
	MaxDepth    int // Maximum allowed nesting depth
	Mode        ValidationMode
	Visited     common.ResultSet
	Issues      []common.Issue
}

// ValidationGraph represents the compiled set of validation operations.
type ValidationGraph struct {
	nodes          map[int]ValidationNode
	dependencies   map[int][]int
	visitedState   map[int]int // For cycle detection during graph building
	executionOrder []int       // Pre-computed topological order
	ctxPool        sync.Pool
	nextNodeID     int // Local counter for node IDs
}

// ConstraintScope defines where a constraint applies
type ConstraintScope byte

const (
	ConstraintScopeGlobal    ConstraintScope = iota + 1 // Top-level schema, sees entire document
	ConstraintScopeRecursive                            // Recursive boundary, applies to entire subtree
)

// ConstraintSpecificity represents the level at which a constraint is defined
type ConstraintSpecificity int

const (
	SpecificityNestedSchema ConstraintSpecificity = iota + 1
	SpecificitySchemaReference
	SpecificityTopLevel
)

// EffectiveConstraint represents a constraint with its specificity level and scope
type EffectiveConstraint struct {
	Constraint  Constraint
	Specificity ConstraintSpecificity
	BasePath    string
	Scope       ConstraintScope
}

// ConstraintRegistry manages constraint overrides based on specificity
type ConstraintRegistry struct {
	constraints map[string]EffectiveConstraint // keyed by constraint name
}

func newConstraintRegistry() *ConstraintRegistry {
	return &ConstraintRegistry{
		constraints: make(map[string]EffectiveConstraint),
	}
}

// Add a constraint, applying override rules based on specificity
func (cr *ConstraintRegistry) Add(name string, constraint Constraint, specificity ConstraintSpecificity, basePath string, scope ConstraintScope) {
	existing, exists := cr.constraints[name]

	// Override rules:
	// 1. Higher specificity wins
	// 2. For same specificity, newer wins (last write)
	if !exists || specificity >= existing.Specificity {
		cr.constraints[name] = EffectiveConstraint{
			Constraint:  constraint,
			Specificity: specificity,
			BasePath:    basePath,
			Scope:       scope,
		}
	}
}

// GetEffective returns all effective constraints after applying override rules
func (cr *ConstraintRegistry) GetEffective() []EffectiveConstraint {
	result := make([]EffectiveConstraint, 0, len(cr.constraints))
	for _, ec := range cr.constraints {
		result = append(result, ec)
	}
	return result
}

// =============================================================================
// BUILD CONTEXT FOR RECURSION TRACKING
// =============================================================================

type buildContext struct {
	// Track which schemas are currently being built (using ref count)
	buildingSchemas map[SchemaId]int

	// Cache for recursive graphs WITH their constraint sets
	// Key: schema ID + constraint hash
	recursiveGraphCache map[string]*ValidationGraph
}

func newBuildContext() *buildContext {
	return &buildContext{
		buildingSchemas:     make(map[SchemaId]int),
		recursiveGraphCache: make(map[string]*ValidationGraph),
	}
}

func (ctx *buildContext) isRecursive(schemaID SchemaId) bool {
	return ctx.buildingSchemas[schemaID] > 0
}

func (ctx *buildContext) markBuilding(schemaID SchemaId) {
	ctx.buildingSchemas[schemaID]++
}

func (ctx *buildContext) unmarkBuilding(schemaID SchemaId) {
	ctx.buildingSchemas[schemaID]--
	if ctx.buildingSchemas[schemaID] <= 0 {
		delete(ctx.buildingSchemas, schemaID)
	}
}

// makeGraphCacheKey creates a unique key for a schema + constraints combination
func makeGraphCacheKey(schemaID SchemaId, constraints SchemaConstraint) string {
	if len(constraints) == 0 {
		return string(schemaID)
	}

	// Create stable hash of constraints
	constraintIDs := make([]string, 0, len(constraints))
	for id := range constraints {
		constraintIDs = append(constraintIDs, string(id))
	}
	sort.Strings(constraintIDs)

	return fmt.Sprintf("%s:%s", schemaID, strings.Join(constraintIDs, ","))
}

func (ctx *buildContext) getOrBuildRecursiveGraph(
	schemaID SchemaId,
	schemaDef NestedSchema,
	instanceConstraints SchemaConstraint,
	topLevelSchema *Schema,
) (*ValidationGraph, error) {
	// Check cache with constraint-specific key
	cacheKey := makeGraphCacheKey(schemaID, instanceConstraints)
	if cached, exists := ctx.recursiveGraphCache[cacheKey]; exists {
		return cached, nil
	}

	// Build new recursive graph
	graph := newValidationGraph()
	addedConstraints := make(map[int]bool)

	// Optimistically cache to handle self-references within this graph
	ctx.recursiveGraphCache[cacheKey] = graph

	// Mark as building (for nested recursion detection)
	ctx.markBuilding(schemaID)
	defer ctx.unmarkBuilding(schemaID)

	effectiveSchema := &Schema{BaseSchema: schemaDef.BaseSchema}

	// Build with merged constraints (base + instance overrides)
	// These constraints will apply to entire recursive subtree
	_, err := graph.buildFromSchema(
		effectiveSchema,
		"",  // Root path for the subgraph
		nil, // Root path parts
		addedConstraints,
		&schemaDef,
		instanceConstraints, // Instance constraints baked into recursive graph
		topLevelSchema,
		ctx,
	)

	if err != nil {
		delete(ctx.recursiveGraphCache, cacheKey) // Rollback
		return nil, err
	}

	if err := graph.finalize(); err != nil {
		delete(ctx.recursiveGraphCache, cacheKey) // Rollback
		return nil, err
	}

	return graph, nil
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
	id        int
	path      string
	pathParts []string
	deps      []int
}

func (n *baseNode) GetID() int             { return n.id }
func (n *baseNode) GetPath() string        { return n.path }
func (n *baseNode) GetPathParts() []string { return n.pathParts }
func (n *baseNode) GetDependencies() []int { return n.deps }

// Validation node types
type UnexpectedFieldsNode struct {
	baseNode
	expectedFields map[string]bool
}

type RequiredFieldNode struct {
	baseNode
	fieldName       string
	parentPath      string
	parentPathParts []string
}

type TypeCheckNode struct {
	baseNode
	expected FieldType
}

type EnumValidationNode struct {
	baseNode
	fieldDef  *Field
	schema    *Schema
	schemaRef SchemaReference
	lookup    map[any]struct{}
	complex   []any
}

type ArrayValidationNode struct {
	baseNode
	fieldDef *Field
	schema   *Schema
	graph    *ValidationGraph
}

type RecordValidationNode struct {
	baseNode
	fieldDef *Field
	schema   *Schema
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
	fieldDef *Field
	schema   *Schema
	graphs   []*ValidationGraph
}

type CompositeValidationNode struct {
	baseNode
	fieldDef *Field
	schema   *Schema
	graphs   []*ValidationGraph
}

type GeometryValidationNode struct {
	baseNode
}

type ConstraintNode struct {
	baseNode
	constraint          Constraint
	fieldPaths          []string
	fieldPathParts      [][]string
	constraintPath      string
	constraintPathParts []string
	scope               ConstraintScope
}

type ConstraintGroupNode struct {
	baseNode
	group     ConstraintGroup
	memberIDs []string
	Name      string
	scope     ConstraintScope
}

// RecursionMarkerNode marks a recursive schema reference
type RecursionMarkerNode struct {
	baseNode
	validationGraph *ValidationGraph // Graph with instance constraints baked in
	schemaName      string
}

// =============================================================================
// HELPER UTILITIES
// =============================================================================

// buildPathAndParts creates both string path and pre-split parts
func buildPathAndParts(basePath string, baseParts []string, fieldName string) (string, []string) {
	if basePath == "" {
		return fieldName, []string{fieldName}
	}

	path := basePath + "." + fieldName
	parts := make([]string, len(baseParts)+1)
	copy(parts, baseParts)
	parts[len(baseParts)] = fieldName

	return path, parts
}

// buildPath creates just the string path (for backward compatibility)
func buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

// splitPath converts a path string to parts (used during construction only)
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func getDependencyIDs(nodes []*baseNode) []int {
	ids := make([]int, len(nodes))
	for i, node := range nodes {
		ids[i] = node.id
	}
	return ids
}

// resolveConstraintFieldPaths converts relative field paths to absolute paths
func resolveConstraintFieldPaths(basePath string, baseParts []string, fieldNames []FieldName) ([]string, [][]string) {
	paths := make([]string, len(fieldNames))
	parts := make([][]string, len(fieldNames))

	for i, fieldName := range fieldNames {
		fieldParts := splitPath(string(fieldName))

		if basePath == "" {
			paths[i] = string(fieldName)
			parts[i] = fieldParts
		} else {
			paths[i] = buildPath(basePath, string(fieldName))
			combinedParts := make([]string, len(baseParts)+len(fieldParts))
			copy(combinedParts, baseParts)
			copy(combinedParts[len(baseParts):], fieldParts)
			parts[i] = combinedParts
		}
	}

	return paths, parts
}

// getPathDepth returns the nesting depth of path parts
func getPathDepth(pathParts []string) int {
	depth := len(pathParts)
	for _, part := range pathParts {
		depth += strings.Count(part, "[")
	}
	return depth
}

// getNodeValue is a helper to retrieve value using pre-split path parts
func getNodeValue(ctx *ValidationContext, pathParts []string) (any, bool) {
	return utils.GetValueByParts(ctx.RootData, pathParts)
}

// =============================================================================
// GRAPH BUILDER FACTORY METHODS
// =============================================================================

func newValidationGraph() *ValidationGraph {
	return &ValidationGraph{
		nodes:          make(map[int]ValidationNode),
		dependencies:   make(map[int][]int),
		visitedState:   make(map[int]int),
		executionOrder: nil,
		nextNodeID:     1, // Start IDs at 1
	}
}

// buildNodeID generates a unique ID for the node within this graph instance
func (graph *ValidationGraph) buildNodeID() int {
	id := graph.nextNodeID
	graph.nextNodeID++
	return id
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
func (graph *ValidationGraph) createUnexpectedFieldsNode(path string, pathParts []string, expectedFields map[string]bool) *UnexpectedFieldsNode {
	return &UnexpectedFieldsNode{
		baseNode:       baseNode{id: graph.buildNodeID(), path: path, pathParts: pathParts},
		expectedFields: expectedFields,
	}
}

func (graph *ValidationGraph) createRequiredFieldNode(fieldPath string, fieldPathParts []string, fieldName string, parentPath string, parentPathParts []string, deps []int) *RequiredFieldNode {
	return &RequiredFieldNode{
		baseNode:        baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		fieldName:       fieldName,
		parentPath:      parentPath,
		parentPathParts: parentPathParts,
	}
}

func (graph *ValidationGraph) createTypeCheckNode(fieldPath string, fieldPathParts []string, fieldDef *Field, deps []int) *TypeCheckNode {
	return &TypeCheckNode{
		baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		expected: fieldDef.Type,
	}
}

func (graph *ValidationGraph) createCompletionNode(path string, pathParts []string, deps []int) *NestedSchemaNode {
	return &NestedSchemaNode{
		baseNode: baseNode{
			id:        graph.buildNodeID(),
			path:      path,
			pathParts: pathParts,
			deps:      deps,
		},
	}
}

// =============================================================================
// CONSTRAINT COLLECTION AND OVERRIDE LOGIC
// =============================================================================

// collectConstraints gathers constraints from all three levels and applies override rules
func collectConstraints(
	nestedSchemaConstraints SchemaConstraint,
	schemaRefConstraints SchemaConstraint,
	topLevelConstraints SchemaConstraint,
	basePath string,
) []EffectiveConstraint {
	registry := newConstraintRegistry()

	// Determine scope based on context
	scope := ConstraintScopeRecursive
	if basePath == "" {
		scope = ConstraintScopeGlobal
	}

	// Level 1: Nested schema constraints (base template)
	for _, constraint := range nestedSchemaConstraints {
		registry.Add(constraint.Name, constraint, SpecificityNestedSchema, basePath, scope)
	}

	// Level 2: Schema reference constraints (override base, apply to subtree)
	for _, constraint := range schemaRefConstraints {
		registry.Add(constraint.Name, constraint, SpecificitySchemaReference, basePath, scope)
	}

	// Level 3: Top-level constraints (most specific)
	for _, constraint := range topLevelConstraints {
		registry.Add(constraint.Name, constraint, SpecificityTopLevel, basePath, scope)
	}

	return registry.GetEffective()
}

// getTopLevelConstraintsForPath extracts top-level constraints that apply to a specific field path
func getTopLevelConstraintsForPath(topLevelSchema *Schema, fieldPath string) SchemaConstraint {
	result := make(SchemaConstraint)

	for id, constraint := range topLevelSchema.Constraints {
		var constraintRule *ConstraintRule
		switch constraint.Kind() {
		case ConstraintKindRule:
			r, err := ConstraintAs[*ConstraintRule](constraint.ConstraintUnion)
			if err == nil {
				constraintRule = r
			}
		}

		if constraintRule != nil {
			for _, fieldName := range constraintRule.Fields {
				if string(fieldName) == fieldPath || strings.HasPrefix(string(fieldName), fieldPath+".") {
					result[id] = constraint
					break
				}
			}
		}
	}

	return result
}

// =============================================================================
// GRAPH CONSTRUCTION LOGIC
// =============================================================================

func (graph *ValidationGraph) finalize() error {
	graph.visitedState = make(map[int]int)
	var order []int
	var hasCycle bool

	var visit func(nodeID int)
	visit = func(nodeID int) {
		if hasCycle {
			return
		}

		if graph.visitedState[nodeID] == dfsVisiting {
			hasCycle = true
			return
		}
		if graph.visitedState[nodeID] == dfsVisited {
			return
		}

		graph.visitedState[nodeID] = dfsVisiting
		for _, depID := range graph.dependencies[nodeID] {
			visit(depID)
		}

		graph.visitedState[nodeID] = dfsVisited
		order = append(order, nodeID)
	}

	for nodeID := range graph.nodes {
		if graph.visitedState[nodeID] == dfsUnvisited {
			visit(nodeID)
			if hasCycle {
				return ErrValidationCircularDependency.
					WithOperation("ValidationGraph.Sort").
					WithMessage(fmt.Sprintf("circular dependency detected at node: %d", nodeID))
			}
		}
	}

	graph.ctxPool.New = func() any {
		return &ValidationContext{
			// Ensure BitState is large enough for all IDs in this graph.
			// IDs are 1-based, so size needs to be > maxID.
			Visited: *common.NewResultState(graph.nextNodeID + 1),
			Issues:  make([]common.Issue, 0, 3),
		}
	}

	graph.executionOrder = order
	return nil
}

func (graph *ValidationGraph) buildFromSchema(
	schema *Schema,
	basePath string,
	baseParts []string,
	addedConstraints map[int]bool,
	nsd *NestedSchema,
	schemaRefConstraints SchemaConstraint,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	var rootNodes []*baseNode

	fieldsToProcess := schema.Fields
	if nsd != nil && nsd.Fields != nil {
		fieldsToProcess = nsd.Fields
	}

	expectedFields := make(map[string]bool)
	for _, fieldDef := range fieldsToProcess {
		expectedFields[string(fieldDef.Name)] = true
	}

	unexpectedNode := graph.createUnexpectedFieldsNode(basePath, baseParts, expectedFields)
	graph.addNode(unexpectedNode)
	rootNodes = append(rootNodes, &unexpectedNode.baseNode)

	var allFieldNodes []*baseNode
	for _, fieldDef := range fieldsToProcess {
		fieldNodes, err := graph.buildFieldNodes(&fieldDef, basePath, baseParts, []int{unexpectedNode.id}, schema, addedConstraints, topLevelSchema, buildCtx)
		if err != nil {
			return nil, err
		}
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	var nestedConstraints SchemaConstraint
	if nsd != nil {
		nestedConstraints = nsd.Constraints
	} else {
		nestedConstraints = schema.Constraints
	}

	var topLevelConstraints SchemaConstraint
	if topLevelSchema != nil && basePath != "" {
		topLevelConstraints = getTopLevelConstraintsForPath(topLevelSchema, basePath)
	}

	effectiveConstraints := collectConstraints(
		nestedConstraints,
		schemaRefConstraints,
		topLevelConstraints,
		basePath,
	)

	constraintDeps := getDependencyIDs(allFieldNodes)
	schemaConstraintIDs := graph.buildFromEffectiveConstraints(effectiveConstraints, baseParts, constraintDeps, addedConstraints)

	if len(schemaConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, baseParts, schemaConstraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes, nil
}

func (graph *ValidationGraph) buildFieldNodes(
	fieldDef *Field,
	basePath string,
	baseParts []string,
	baseDeps []int,
	sc *Schema,
	addedConstraints map[int]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	fieldPath, fieldPathParts := buildPathAndParts(basePath, baseParts, string(fieldDef.Name))
	currentDeps := baseDeps
	var nodes []*baseNode

	if fieldDef.Required {
		parentPath := basePath
		parentPathParts := baseParts
		reqNode := graph.createRequiredFieldNode(fieldPath, fieldPathParts, string(fieldDef.Name), parentPath, parentPathParts, currentDeps)
		graph.addNode(reqNode)
		currentDeps = []int{reqNode.GetID()}
		nodes = append(nodes, &reqNode.baseNode)
	}

	isContainer := fieldDef.Type.IsContainer()
	if !isContainer {
		typeNode := graph.createTypeCheckNode(fieldPath, fieldPathParts, fieldDef, currentDeps)
		graph.addNode(typeNode)
		currentDeps = []int{typeNode.id}
		nodes = append(nodes, &typeNode.baseNode)
	}

	typeSpecificNodes, err := graph.buildFieldTypeNodes(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, addedConstraints, topLevelSchema, buildCtx)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, typeSpecificNodes...)

	return nodes, nil
}

func (graph *ValidationGraph) buildFieldTypeNodes(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	currentDeps []int,
	sc *Schema,
	addedConstraints map[int]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	var node ValidationNode
	var nodes []*baseNode
	var err error

	switch fieldDef.Type {
	case FieldTypeEnum:
		node, err = graph.buildEnumNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema)

	case FieldTypeArray:
		node, err = graph.buildArrayNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeSet:
		node = &SetValidationNode{
			baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: currentDeps},
		}

	case FieldTypeRecord:
		node, err = graph.buildRecordNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeUnion:
		node, err = graph.buildUnionNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeComposite:
		node, err = graph.buildCompositeNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeGeometry:
		node = &GeometryValidationNode{
			baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: currentDeps},
		}

	case FieldTypeObject:
		objectNodes, err := graph.buildObjectFieldNodes(fieldDef, fieldPath, fieldPathParts, sc, addedConstraints, topLevelSchema, buildCtx)
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
			GetID() int
			GetPath() string
			GetPathParts() []string
			GetDependencies() []int
		}); ok {
			nodes = append(nodes, &baseNode{id: bn.GetID(), path: bn.GetPath(), pathParts: bn.GetPathParts(), deps: bn.GetDependencies()})
		}
	}

	return nodes, nil
}

func (graph *ValidationGraph) buildEnumNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	currentDeps []int,
	sc *Schema,
	topLevelSchema *Schema,
) (ValidationNode, error) {
	if fieldDef.Schema.IsZero() {
		return nil, ErrSchemaNotFound.WithMessage("Enum field must have schema reference").WithPath(fieldPath)
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve enum schema: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedSchema, exists := topLevelSchema.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Enum schema '%s' not found", schemaRef.ID)).WithPath(fieldPath)
	}

	if len(nestedSchema.Values) == 0 {
		return nil, ErrInvalidSchema.WithMessagef("Enum schema %s has no values defined", schemaRef.ID).WithPath(fieldPath)
	}

	lookup := make(map[any]struct{}, len(nestedSchema.Values))
	var complex []any

	for _, v := range nestedSchema.Values {
		if v.IsZero() || v.IsNull() {
			continue
		}

		val := v.Value()
		if v.IsSimple() {
			lookup[val] = struct{}{}
		} else {
			complex = append(complex, val)
		}
	}

	return &EnumValidationNode{
		baseNode:  baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: currentDeps},
		fieldDef:  fieldDef,
		schema:    sc,
		schemaRef: schemaRef,
		lookup:    lookup,
		complex:   complex,
	}, nil
}

func (graph *ValidationGraph) buildObjectFieldNodes(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	sc *Schema,
	addedConstraints map[int]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	var nodes []*baseNode

	if fieldDef.Schema.IsZero() {
		return nil, ErrSchemaNotFound.WithMessage("FieldSchemaReference is zero/uninitialized").WithPath(fieldPath)
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedSchema, exists := sc.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for field '%s'", schemaRef.ID, fieldDef.Name)).WithPath(fieldPath)
	}

	// Check if this creates a recursive reference
	if buildCtx.isRecursive(schemaRef.ID) {
		// Build recursive graph with instance constraints baked in
		recursiveGraph, err := buildCtx.getOrBuildRecursiveGraph(
			schemaRef.ID,
			nestedSchema,
			schemaRef.Constraints,
			topLevelSchema,
		)
		if err != nil {
			return nil, err
		}

		// Create recursion marker
		markerNode := &RecursionMarkerNode{
			baseNode: baseNode{
				id:        graph.buildNodeID(),
				path:      fieldPath,
				pathParts: fieldPathParts,
			},
			validationGraph: recursiveGraph,
			schemaName:      string(schemaRef.ID),
		}
		graph.addNode(markerNode)
		nodes = append(nodes, &markerNode.baseNode)

		return nodes, nil
	}

	// Non-recursive case: build normally
	effectiveSchema := &Schema{BaseSchema: nestedSchema.BaseSchema}

	// Mark this schema as being built so nested references (e.g. inside arrays) detect the recursion
	buildCtx.markBuilding(schemaRef.ID)
	defer buildCtx.unmarkBuilding(schemaRef.ID)

	nestedNodes, err := graph.buildFromSchema(effectiveSchema, fieldPath, fieldPathParts, addedConstraints, &nestedSchema, schemaRef.Constraints, topLevelSchema, buildCtx)
	if err != nil {
		return nil, err
	}

	markerNode := &NestedSchemaNode{
		baseNode: baseNode{
			id:        graph.buildNodeID(),
			path:      fieldPath,
			pathParts: fieldPathParts,
			deps:      getDependencyIDs(nestedNodes),
		},
	}
	graph.addNode(markerNode)
	nodes = append(nodes, &markerNode.baseNode)

	return nodes, nil
}

func (graph *ValidationGraph) buildFromEffectiveConstraints(
	effectiveConstraints []EffectiveConstraint,
	baseParts []string,
	deps []int,
	addedConstraints map[int]bool,
) []int {
	var ruleDepIDs []int

	for _, ec := range effectiveConstraints {
		rules := graph.buildFromConstraintRuleWithScope(ec.Constraint, ec.BasePath, baseParts, deps, addedConstraints, ec.Scope)
		ruleDepIDs = append(ruleDepIDs, rules...)
	}

	return ruleDepIDs
}

// createSubGraph builds a standalone validation graph for a single field definition
func (graph *ValidationGraph) createSubGraph(
	rootFieldName string,
	rootFieldDef *Field,
	basePath string,
	originalTopLevelSchema *Schema,
	buildCtx *buildContext,
) (*ValidationGraph, error) {
	tempSchema := &Schema{
		BaseSchema: BaseSchema{
			Name:   fmt.Sprintf("subgraph_%s", basePath),
			Fields: map[FieldId]Field{FieldId(rootFieldName): *rootFieldDef},
		},
		Schemas: originalTopLevelSchema.Schemas,
	}

	subGraph := newValidationGraph()
	addedConstraints := make(map[int]bool)

	if _, err := subGraph.buildFromSchema(tempSchema, "", nil, addedConstraints, nil, nil, originalTopLevelSchema, buildCtx); err != nil {
		return nil, err
	}

	err := subGraph.finalize()
	return subGraph, err
}

func (graph *ValidationGraph) buildArrayNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	sc *Schema,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) (ValidationNode, error) {

	if fieldDef.Schema.IsZero() {
		return &ArrayValidationNode{
			baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
			fieldDef: fieldDef,
			schema:   sc,
			graph:    nil,
		}, nil
	}

	var tempRootField *Field
	var arrayGraph *ValidationGraph
	var err error

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference for array items: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedDef, exists := topLevelSchema.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for array items", schemaRef.ID)).WithPath(fieldPath)
	}

	tempRootFieldType := nestedDef.Type
	var tempRootFieldSchema FieldSchemaReference

	if tempRootFieldType == 0 && len(nestedDef.Fields) > 0 {
		tempRootFieldType = FieldTypeObject
		tempRootFieldSchema = NewSchemaReference(SchemaReference{ID: SchemaId(nestedDef.Name)})
	} else {
		tempRootFieldSchema = nestedDef.FieldProperties.Schema
	}

	tempRootField = &Field{
		Name: "item",
		FieldProperties: FieldProperties{
			Type:    tempRootFieldType,
			Schema:  tempRootFieldSchema,
			Default: nestedDef.Default,
		},
	}

	arrayGraph, err = graph.createSubGraph("item", tempRootField, fieldPath, topLevelSchema, buildCtx)
	if err != nil {
		return nil, err
	}

	return &ArrayValidationNode{
		baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graph:    arrayGraph,
	}, nil
}

func (graph *ValidationGraph) buildRecordNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	sc *Schema,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) (ValidationNode, error) {
	var tempRootField *Field
	var recordGraph *ValidationGraph
	var err error

	if fieldDef.Schema.IsZero() {
		return &RecordValidationNode{
			baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
			fieldDef: fieldDef,
			schema:   sc,
			graph:    nil,
		}, nil
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference for record items: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedDef, exists := topLevelSchema.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for record items", schemaRef.ID)).WithPath(fieldPath)
	}

	tempRootFieldType := nestedDef.Type
	var tempRootFieldSchema FieldSchemaReference

	if tempRootFieldType == 0 && len(nestedDef.Fields) > 0 {
		tempRootFieldType = FieldTypeObject
		tempRootFieldSchema = NewSchemaReference(SchemaReference{ID: SchemaId(nestedDef.Name)})
	} else {
		tempRootFieldSchema = nestedDef.FieldProperties.Schema
	}

	tempRootField = &Field{
		Name: "item",
		FieldProperties: FieldProperties{
			Type:    tempRootFieldType,
			Schema:  tempRootFieldSchema,
			Default: nestedDef.Default,
		},
	}

	recordGraph, err = graph.createSubGraph("item", tempRootField, fieldPath, topLevelSchema, buildCtx)
	if err != nil {
		return nil, err
	}

	return &RecordValidationNode{
		baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graph:    recordGraph,
	}, nil
}

func (graph *ValidationGraph) buildUnionNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	sc *Schema,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) (ValidationNode, error) {
	if fieldDef.Schema.IsZero() || !fieldDef.Schema.IsMultiple() {
		return nil, ErrSchemaNotFound.WithMessage("Union field must reference multiple schemas").WithPath(fieldPath)
	}

	refs, err := FieldSchemaAs[[]SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve schemas for union: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	graphs := make([]*ValidationGraph, 0, len(refs))

	for _, ref := range refs {
		nestedDef, exists := topLevelSchema.Schemas[ref.ID]
		if !exists {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for union", ref.ID)).WithPath(fieldPath)
		}

		var tempRootField *Field
		if len(nestedDef.Fields) > 0 {
			tempRootField = &Field{
				Name: "root",
				FieldProperties: FieldProperties{
					Type:   FieldTypeObject,
					Schema: NewSchemaReference(ref),
				},
			}
		} else {
			tempRootField = &Field{
				Name:            "root",
				FieldProperties: nestedDef.FieldProperties,
			}
		}

		unionGraph, err := graph.createSubGraph("root", tempRootField, fieldPath, topLevelSchema, buildCtx)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, unionGraph)
	}

	return &UnionValidationNode{
		baseNode: baseNode{
			id:        graph.buildNodeID(),
			path:      fieldPath,
			pathParts: fieldPathParts,
			deps:      deps,
		},
		fieldDef: fieldDef,
		schema:   sc,
		graphs:   graphs,
	}, nil
}

func (graph *ValidationGraph) buildCompositeNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	sc *Schema,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) (ValidationNode, error) {
	if fieldDef.Schema.IsZero() || !fieldDef.Schema.IsMultiple() {
		return nil, ErrSchemaNotFound.WithMessage("Composite field must reference multiple schemas").WithPath(fieldPath)
	}

	refs, err := FieldSchemaAs[[]SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve schemas for composite: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	graphs := make([]*ValidationGraph, 0, len(refs))

	for _, ref := range refs {
		nestedDef, exists := topLevelSchema.Schemas[ref.ID]
		if !exists {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for composite", ref.ID)).WithPath(fieldPath)
		}

		if !nestedDef.isEffectivelyObject(sc) {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Composite schema '%s' must effectively represent an object type", ref.ID)).WithPath(fieldPath)
		}

		var tempRootField *Field
		if len(nestedDef.Fields) > 0 {
			tempRootField = &Field{
				Name: "root",
				FieldProperties: FieldProperties{
					Type:   FieldTypeObject,
					Schema: NewSchemaReference(ref),
				},
			}
		} else {
			tempRootField = &Field{
				Name:            "root",
				FieldProperties: nestedDef.FieldProperties,
			}
		}

		compositeGraph, err := graph.createSubGraph("root", tempRootField, fieldPath, topLevelSchema, buildCtx)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, compositeGraph)
	}

	return &CompositeValidationNode{
		baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graphs:   graphs,
	}, nil
}

func (graph *ValidationGraph) buildFromConstraintRuleWithScope(
	rule Constraint,
	basePath string,
	baseParts []string,
	deps []int,
	addedConstraints map[int]bool,
	scope ConstraintScope,
) []int {
	var ruleDepIDs []int

	switch rule.Kind() {
	case ConstraintKindRule:
		r, err := ConstraintAs[*ConstraintRule](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		absoluteFieldPaths, absoluteFieldPathParts := resolveConstraintFieldPaths(basePath, baseParts, r.Fields)

		nodeID := graph.buildNodeID()
		if addedConstraints[nodeID] {
			return []int{nodeID}
		}

		node := &ConstraintNode{
			baseNode:            baseNode{id: nodeID, path: basePath, pathParts: baseParts, deps: deps},
			constraint:          rule,
			fieldPaths:          absoluteFieldPaths,
			fieldPathParts:      absoluteFieldPathParts,
			constraintPath:      basePath,
			constraintPathParts: baseParts,
			scope:               scope,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)

	case ConstraintKindGroup:
		g, err := ConstraintAs[*ConstraintGroup](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		nodeID := graph.buildNodeID()
		if addedConstraints[nodeID] {
			return []int{nodeID}
		}

		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: basePath, pathParts: baseParts, deps: deps},
			group:    *g,
			Name:     rule.Name,
			scope:    scope,
		}

		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}

	return ruleDepIDs
}

func (graph *ValidationGraph) traverse(fmap PredicateMap, document map[string]any, mode ValidationMode, maxDepth int) ([]common.Issue, bool) {
	ctx := graph.ctxPool.Get().(*ValidationContext)
	defer graph.ctxPool.Put(ctx)
	ctx.RootData = document
	ctx.Data = document
	ctx.Mode = mode

	ctx.FunctionMap = fmap
	ctx.MaxDepth = maxDepth

	ctx.Visited.Clear()
	ctx.Issues = ctx.Issues[:0]

	for _, nodeID := range graph.executionOrder {
		node := graph.nodes[nodeID]

		shouldSkip := false
		for _, depID := range graph.dependencies[nodeID] {
			if ctx.Visited.HasValue(depID) && !ctx.Visited.Value(depID) {
				ctx.Visited.Set(nodeID, skipped.Success)
				shouldSkip = true
				break
			}
		}

		if shouldSkip {
			continue
		}

		nodePathParts := node.GetPathParts()
		nodePath := node.GetPath()
		val, keyExists := getNodeValue(ctx, nodePathParts)

		_, isRequiredNode := node.(*RequiredFieldNode)
		if (!keyExists || val == nil) && !isRequiredNode && nodePath != "" {
			ctx.Visited.Set(nodeID, skipped.Success)
			continue
		}

		result := node.Execute(ctx)
		ctx.Visited.Set(nodeID, result.Success)

		if result != nil && !result.Skipped && len(result.Issues) > 0 {
			for _, issue := range result.Issues {
				switch mode {
				case ValidationModeStrict:
					ctx.Issues = append(ctx.Issues, issue)
				case ValidationModePartialStrict:
					if issue.Code != "REQUIRED_FIELD_MISSING" {
						ctx.Issues = append(ctx.Issues, issue)
					}
				case ValidationModeLoose:
					if issue.Code != "REQUIRED_FIELD_MISSING" && issue.Code != "UNEXPECTED_FIELD" {
						ctx.Issues = append(ctx.Issues, issue)
					}
				}
			}
		}
	}

	return ctx.Issues, len(ctx.Issues) == 0
}

// =============================================================================
// MAIN VALIDATOR CONSTRUCTION
// =============================================================================

func NewDocumentValidator(sc *Schema, fmap PredicateMap) (*DocumentValidator, error) {
	return NewDocumentValidatorWithConfig(sc, fmap, DefaultValidationConfig())
}

func NewDocumentValidatorWithConfig(sc *Schema, fmap PredicateMap, config ValidationConfig) (*DocumentValidator, error) {
	graph := newValidationGraph()
	addedConstraints := make(map[int]bool)
	buildCtx := newBuildContext()

	if _, err := graph.buildFromSchema(sc, "", nil, addedConstraints, nil, nil, sc, buildCtx); err != nil {
		return nil, err
	}

	if err := graph.finalize(); err != nil {
		return nil, err
	}

	v := &DocumentValidator{
		graph:  graph,
		fmap:   fmap,
		config: config,
	}
	return v, nil
}

// =============================================================================
// GRAPH TRAVERSAL AND VALIDATION
// =============================================================================

func (v *DocumentValidator) Validate(document map[string]any) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, ValidationModeStrict, v.config.MaxDepth)
}

func (v *DocumentValidator) ValidatePartial(document map[string]any) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, ValidationModePartialStrict, v.config.MaxDepth)
}

func (v *DocumentValidator) ValidateLoose(document map[string]any) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, ValidationModeLoose, v.config.MaxDepth)
}

// =============================================================================
// NODE EXECUTION IMPLEMENTATIONS
// =============================================================================

func (n *UnexpectedFieldsNode) Execute(ctx *ValidationContext) *NodeResult {
	currentData, exists := getNodeValue(ctx, n.pathParts)
	if !exists {
		return success
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

	if len(issues) > 0 {
		return &NodeResult{Success: len(issues) == 0, Issues: issues}
	}

	return success
}

func (n *RequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentData, exists := getNodeValue(ctx, n.parentPathParts)
	if !exists {
		if n.parentPath != "" {
			return success
		}
	}

	if parentData == nil {
		return skipped
	}

	dataMap, ok := utils.GetMapStringAny(parentData)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "INVALID_DATA_STRUCTURE", Message: "Cannot check for required fields on non-object parent", Path: n.parentPath}}}
	}

	if _, exists := dataMap[n.fieldName]; !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "REQUIRED_FIELD_MISSING", Message: fmt.Sprintf("Required field '%s' is missing", n.fieldName), Path: n.path}}}
	}
	return success
}

func failTypeMismatch(path, expected string, actual any) *NodeResult {
	return &NodeResult{
		Success: false,
		Issues: []common.Issue{{
			Code:    "TYPE_MISMATCH",
			Message: fmt.Sprintf("Expected %s, got %T", expected, actual),
			Path:    path,
		}},
	}
}

func (n *TypeCheckNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	switch n.expected {
	case FieldTypeString:
		if _, ok := value.(string); ok {
			return success
		}
	case FieldTypeBoolean:
		if _, ok := value.(bool); ok {
			return success
		}
	case FieldTypeNumber:
		switch value.(type) {
		case float64, float32, int64, int, uint64, uint:
			return success
		}
	case FieldTypeInteger:
		switch value.(type) {
		case int64, int, uint64, uint, int32, uint32, int16, uint16, int8, uint8:
			return success
		}
	case FieldTypeDecimal:
		switch value.(type) {
		case float64, float32:
			return success
		}
	case FieldTypeUnknown:
		return success
	default:
		v := reflect.ValueOf(value)
		if !v.IsValid() {
			return &NodeResult{Success: false, Issues: []common.Issue{{
				Code:    "TYPE_MISMATCH",
				Message: fmt.Sprintf("Expected %s, got invalid value", n.expected),
				Path:    n.path,
			}}}
		}
	}

	return failTypeMismatch(n.path, n.expected.String(), value)
}

func (n *EnumValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	switch value.(type) {
	case string, float64, float32, int64, int, uint64, uint:
	default:
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "TYPE_MISMATCH",
			Message: fmt.Sprintf("Expected %s, got invalid value", n.fieldDef.Type),
			Path:    n.path,
		}}}
	}

	nested, _ := n.schema.Schemas[n.schemaRef.ID]
	expectNumeric := (nested.Type == FieldTypeNumber || nested.Type == FieldTypeDecimal || nested.Type == FieldTypeInteger)
	switch v := value.(type) {
	case string, int64, float64, bool:
		if _, found := n.lookup[v]; found {
			return success
		}
	case int:
		if _, found := n.lookup[int64(v)]; found {
			return success
		}
	}

	for _, allowed := range n.complex {
		if deepEqual(value, allowed, expectNumeric) {
			return success
		}
	}

	return &NodeResult{
		Success: false,
		Issues: []common.Issue{{
			Code:    "ENUM_VIOLATION",
			Message: fmt.Sprintf("Value %v is not in the allowed list", value),
			Path:    n.path,
		}},
	}
}

func (n *ArrayValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "ARRAY_TYPE_MISMATCH", Message: "Expected array", Path: n.path}}}
	}

	currentDepth := getPathDepth(n.pathParts)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	if n.graph == nil {
		return success
	}

	var allIssues []common.Issue

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)

		remainingDepth := ctx.MaxDepth - currentDepth
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ctx.Mode, remainingDepth)

		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "item") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
		}
		allIssues = append(allIssues, itemIssues...)
	}

	if len(allIssues) > 0 {
		return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
	}

	return success
}

func (n *RecordValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	recordMap, ok := value.(map[string]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "OBJECT_TYPE_MISMATCH", Message: "Expected object for record", Path: n.path}}}
	}

	currentDepth := getPathDepth(n.pathParts)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	if n.graph == nil {
		return success
	}

	var allIssues []common.Issue
	for key, item := range recordMap {
		itemPath := buildPath(n.path, key)

		remainingDepth := ctx.MaxDepth - currentDepth
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ctx.Mode, remainingDepth)

		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "item") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
		}
		allIssues = append(allIssues, itemIssues...)
	}

	if len(allIssues) > 0 {
		return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
	}
	return success
}

func (n *SetValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "SET_TYPE_MISMATCH",
			Message: "Expected array for set validation",
			Path:    n.path,
		}}}
	}

	length := val.Len()
	if length <= 1 {
		return success
	}

	seenComparable := make(map[any]int, length)

	type complexItem struct {
		val   any
		index int
	}
	seenComplex := make([]complexItem, 0)

	for i := range length {
		item := val.Index(i).Interface()

		if isSafeComparable(item) {
			if firstIdx, found := seenComparable[item]; found {
				return n.createDuplicateError(i, firstIdx)
			}
			seenComparable[item] = i
		} else {
			for _, prev := range seenComplex {
				if deepEqual(item, prev.val, false) {
					return n.createDuplicateError(i, prev.index)
				}
			}
			seenComplex = append(seenComplex, complexItem{val: item, index: i})
		}
	}

	return success
}

func (n *SetValidationNode) createDuplicateError(currentIndex, firstIndex int) *NodeResult {
	return &NodeResult{
		Success: false,
		Issues: []common.Issue{{
			Code:    "SET_DUPLICATE",
			Message: fmt.Sprintf("Duplicate value at index %d (first seen at index %d)", currentIndex, firstIndex),
			Path:    n.path,
		}},
	}
}

func isSafeComparable(v any) bool {
	if v == nil {
		return true
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		return false
	}
	return t.Comparable()
}

func deepEqual(a, b any, numericEquivalent bool) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return a == b
	}

	switch va := a.(type) {
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case int64:
		if numericEquivalent {
			return compareNumeric(va, b)
		}
		vb, ok := b.(int64)
		return ok && va == vb
	case float64:
		if numericEquivalent {
			return compareNumeric(va, b)
		}
		vb, ok := b.(float64)
		return ok && va == vb
	case int:
		if numericEquivalent {
			return compareNumeric(va, b)
		}
		vb, ok := b.(int)
		return ok && va == vb
	}

	vA := reflect.ValueOf(a)
	vB := reflect.ValueOf(b)

	if !numericEquivalent && vA.Kind() != vB.Kind() {
		return false
	}

	switch vA.Kind() {
	case reflect.Slice, reflect.Array:
		if vB.Kind() != reflect.Slice && vB.Kind() != reflect.Array {
			return false
		}
		if vA.Len() != vB.Len() {
			return false
		}
		for i := 0; i < vA.Len(); i++ {
			if !deepEqual(vA.Index(i).Interface(), vB.Index(i).Interface(), numericEquivalent) {
				return false
			}
		}
		return true

	case reflect.Map:
		if vB.Kind() != reflect.Map || vA.Len() != vB.Len() {
			return false
		}
		iter := vA.MapRange()
		for iter.Next() {
			valB := vB.MapIndex(iter.Key())
			if !valB.IsValid() || !deepEqual(iter.Value().Interface(), valB.Interface(), numericEquivalent) {
				return false
			}
		}
		return true
	}

	if numericEquivalent && isReflectNumeric(vA) && isReflectNumeric(vB) {
		return compareNumeric(a, b)
	}

	return reflect.DeepEqual(a, b)
}

func compareNumeric(a, b any) bool {
	fa, okA := toFloat64(a)
	fb, okB := toFloat64(b)
	if !okA || !okB {
		return false
	}
	const epsilon = 1e-12
	diff := fa - fb
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

func toFloat64(v any) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case int64:
		return float64(i), true
	case int:
		return float64(i), true
	case float32:
		return float64(i), true
	case int32:
		return float64(i), true
	case uint64:
		return float64(i), true
	default:
		return 0, false
	}
}

func isReflectNumeric(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Int && k <= reflect.Uint64) || k == reflect.Float32 || k == reflect.Float64
}

func (n *NestedSchemaNode) Execute(ctx *ValidationContext) *NodeResult {
	return success
}

// RecursionMarkerNode executes by delegating to the cached recursive graph
func (n *RecursionMarkerNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists || value == nil {
		return success
	}

	// CHECK TYPE
	mapValue, ok := utils.GetMapStringAny(value)
	if !ok {
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code:    "TYPE_MISMATCH",
				Message: fmt.Sprintf("Expected object for recursive schema %s, got %T", n.schemaName, value),
				Path:    n.path,
			}},
		}
	}

	// Check depth limit
	currentDepth := getPathDepth(n.pathParts)
		if currentDepth >= ctx.MaxDepth {
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code:    "MAX_DEPTH_EXCEEDED",
				Message: fmt.Sprintf("Recursive schema '%s' exceeds maximum depth of %d", n.schemaName, ctx.MaxDepth),
				Path:    n.path,
			}},
		}
	}

	// Traverse the cached recursive graph
	// The graph already has instance constraints baked in, which will be inherited
	issues, _ := n.validationGraph.traverse(
		ctx.FunctionMap,
		mapValue, // Pass value directly, do NOT wrap in "root"
		ctx.Mode,
		ctx.MaxDepth,
	)

	// Rewrite paths from subgraph context to parent context
	for i := range issues {
		if issues[i].Path == "" {
			issues[i].Path = n.path
		} else {
			issues[i].Path = buildPath(n.path, issues[i].Path)
		}
	}

	if len(issues) > 0 {
		return &NodeResult{Success: false, Issues: issues}
	}

	return success
}

func (n *UnionValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists {
		return success
	}

	currentDepth := getPathDepth(n.pathParts)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	var allIssues [][]common.Issue

	for _, graph := range n.graphs {
		itemIssues, matched := graph.traverse(
			ctx.FunctionMap,
			map[string]any{"root": value},
			ctx.Mode,
			ctx.MaxDepth,
		)

		if matched {
			return success
		}

		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "root") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "root", n.path, 1)
			}
		}
		allIssues = append(allIssues, itemIssues)
	}

	return &NodeResult{
		Success: false,
		Issues: []common.Issue{{
			Code:    "UNION_MISMATCH",
			Message: fmt.Sprintf("Value does not match any union variant (tried %d variants)", len(n.graphs)),
			Path:    n.path,
		}},
	}
}

func (n *CompositeValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists {
		return success
	}

	currentDepth := getPathDepth(n.pathParts)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	var allIssues []common.Issue

	for _, graph := range n.graphs {
		itemIssues, matched := graph.traverse(
			ctx.FunctionMap,
			map[string]any{"root": value},
			ctx.Mode,
			ctx.MaxDepth,
		)

		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "root") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "root", n.path, 1)
			}
		}

		if !matched {
			allIssues = append(allIssues, itemIssues...)
		}
	}

	if len(allIssues) > 0 {
		return &NodeResult{Success: false, Issues: allIssues}
	}

	return success
}

func (n *GeometryValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := getNodeValue(ctx, n.pathParts)
	if !exists {
		return success
	}

	outer, ok := value.([]any)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "GEOMETRY_TYPE_MISMATCH",
			Message: "Geometry must be an array of coordinate arrays",
			Path:    n.path,
		}}}
	}

	for i, innerRaw := range outer {
		inner, ok := innerRaw.([]any)
		if !ok {
			return &NodeResult{Success: false, Issues: []common.Issue{{
				Code:    "GEOMETRY_TYPE_MISMATCH",
				Message: fmt.Sprintf("Geometry inner element at index %d must be an array", i),
				Path:    fmt.Sprintf("%s[%d]", n.path, i),
			}}}
		}

		for j, elem := range inner {
			switch elem.(type) {
			case int, int64, float64, float32:
				continue
			default:
				return &NodeResult{Success: false, Issues: []common.Issue{{
					Code:    "GEOMETRY_TYPE_MISMATCH",
					Message: fmt.Sprintf("Geometry element at [%d][%d] is not a valid number", i, j),
					Path:    fmt.Sprintf("%s[%d][%d]", n.path, i, j),
				}}}
			}
		}
	}

	return success
}

func (n *ConstraintNode) Execute(ctx *ValidationContext) *NodeResult {
	r, _ := ConstraintAs[*ConstraintRule](n.constraint.ConstraintUnion)

	switch n.scope {
	case ConstraintScopeGlobal:
		return n.executeGlobalConstraint(ctx, r)
	case ConstraintScopeRecursive:
		return n.executeRecursiveConstraint(ctx, r)
	}

	return success
}

func (n *ConstraintNode) executeGlobalConstraint(ctx *ValidationContext, r *ConstraintRule) *NodeResult {
	return evaluateWithPresenceCheck(
		ctx,
		n.constraint.Name,
		n.constraintPath,
		n.fieldPaths,
		n.fieldPathParts,
		func() *NodeResult {
			globalCtx := *ctx
			globalCtx.Data = ctx.RootData
			return runConstraintPredicate(&globalCtx, r, n.constraintPath)
		},
	)
}

func (n *ConstraintNode) executeRecursiveConstraint(ctx *ValidationContext, r *ConstraintRule) *NodeResult {
	instanceData, exists := getNodeValue(ctx, n.constraintPathParts)
	if !exists {
		return skipped
	}

	relativeFieldPaths := make([]string, len(n.fieldPaths))
	relativeFieldPathParts := make([][]string, len(n.fieldPathParts))

	for i, absPath := range n.fieldPaths {
		if n.constraintPath != "" && strings.HasPrefix(absPath, n.constraintPath+".") {
			relativeFieldPaths[i] = strings.TrimPrefix(absPath, n.constraintPath+".")
			prefixLen := len(n.constraintPathParts)
			relativeFieldPathParts[i] = n.fieldPathParts[i][prefixLen:]
		} else {
			relativeFieldPaths[i] = absPath
			relativeFieldPathParts[i] = n.fieldPathParts[i]
		}
	}

	return evaluateWithPresenceCheck(
		ctx,
		n.constraint.Name,
		n.constraintPath,
		relativeFieldPaths,
		relativeFieldPathParts,
		func() *NodeResult {
			instanceCtx := *ctx
			instanceCtx.Data = instanceData
			instanceCtx.RootData = instanceData
			return runConstraintPredicate(&instanceCtx, r, n.constraintPath)
		},
	)
}

func (n *ConstraintGroupNode) Execute(ctx *ValidationContext) *NodeResult {
	allRequiredFields, allRequiredFieldParts := n.collectAllRequiredFields()

	return evaluateWithPresenceCheck(
		ctx,
		n.Name,
		n.path,
		allRequiredFields,
		allRequiredFieldParts,
		func() *NodeResult {
			return n.executeGroup(ctx)
		},
	)
}

func (n *ConstraintGroupNode) collectAllRequiredFields() ([]string, [][]string) {
	allRequiredFieldsMap := make(map[string][]string)

	for _, ruleUnion := range n.group.Rules {
		r, err := ConstraintAs[*ConstraintRule](ruleUnion)
		if err != nil {
			continue
		}

		absPaths, absPathParts := resolveConstraintFieldPaths(n.path, n.pathParts, r.Fields)
		for i, p := range absPaths {
			allRequiredFieldsMap[p] = absPathParts[i]
		}
	}

	fields := make([]string, 0, len(allRequiredFieldsMap))
	for field := range allRequiredFieldsMap {
		fields = append(fields, field)
	}

	fieldParts := make([][]string, len(fields))
	for i, field := range fields {
		fieldParts[i] = allRequiredFieldsMap[field]
	}

	return fields, fieldParts
}

func (n *ConstraintGroupNode) executeGroup(ctx *ValidationContext) *NodeResult {
	var results []bool
	var memberIssues []common.Issue

	for _, ruleUnion := range n.group.Rules {
		r, err := ConstraintAs[*ConstraintRule](ruleUnion)
		if err != nil {
			return &NodeResult{
				Success: false,
				Issues: []common.Issue{{
					Code:    "INTERNAL_ERROR",
					Message: fmt.Sprintf("Failed to extract constraint rule: %v", err),
					Path:    n.path,
				}},
			}
		}

		res := runConstraintPredicate(ctx, r, n.path)

		results = append(results, res.Success)
		if !res.Success {
			memberIssues = append(memberIssues, res.Issues...)
		}
	}

	if ok, _ := n.group.Operator.Evaluate(results); !ok {
		return &NodeResult{
			Success: false,
			Issues: append([]common.Issue{{
				Code:    "CONSTRAINT_GROUP_VIOLATION",
				Message: fmt.Sprintf("Constraint group '%s' failed", n.Name),
				Path:    n.path,
			}}, memberIssues...),
		}
	}

	return success
}

func evaluateWithPresenceCheck(
	ctx *ValidationContext,
	constraintName string,
	constraintPath string,
	requiredFields []string,
	requiredFieldParts [][]string,
	executor func() *NodeResult,
) *NodeResult {
	var presentFields []string
	var missingFields []string

	for i, fieldPath := range requiredFields {
		if _, exists := getNodeValue(ctx, requiredFieldParts[i]); exists {
			presentFields = append(presentFields, fieldPath)
		} else {
			missingFields = append(missingFields, fieldPath)
		}
	}

	if len(missingFields) == 0 {
		return executor()
	}

	if len(presentFields) == 0 {
		switch ctx.Mode {
		case ValidationModeStrict:
			return &NodeResult{
				Success: false,
				Issues: []common.Issue{{
					Code: "CONSTRAINT_INCOMPLETE",
					Message: fmt.Sprintf(
						"Constraint '%s' cannot be evaluated: missing required fields %v",
						constraintName, missingFields,
					),
					Path: constraintPath,
				}},
			}

		case ValidationModePartialStrict, ValidationModeLoose:
			return skipped
		}
	}

	switch ctx.Mode {
	case ValidationModeStrict:
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code: "CONSTRAINT_INCOMPLETE",
				Message: fmt.Sprintf(
					"Constraint '%s' cannot be evaluated: missing required fields %v",
					constraintName, missingFields,
				),
				Path: constraintPath,
			}},
		}

	case ValidationModePartialStrict:
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code: "CONSTRAINT_PARTIAL_UPDATE",
				Message: fmt.Sprintf(
					"Constraint '%s' couples fields %v. Cannot update only %v - all coupled fields must be updated together",
					constraintName,
					requiredFields,
					presentFields,
				),
				Path: constraintPath,
			}},
		}

	case ValidationModeLoose:
		return skipped
	}

	return success
}

func runConstraintPredicate(
	ctx *ValidationContext,
	r *ConstraintRule,
	contextPath string,
) *NodeResult {
	predicateFunc, exists := ctx.FunctionMap[r.Predicate]
	if !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MISSING_PREDICATE",
			Message: fmt.Sprintf("Predicate '%s' not found", r.Predicate),
			Path:    contextPath,
		}}}
	}

	issues := predicateFunc(PredicateParams{
		Data:       ctx.RootData,
		Keys:       r.Fields,
		Parameters: r.Parameters,
	})

	if len(issues) > 0 {
		for i := range issues {
			if issues[i].Path == "" {
				issues[i].Path = contextPath
			} else {
				issues[i].Path = buildPath(contextPath, issues[i].Path)
			}
		}
		return &NodeResult{Success: false, Issues: issues}
	}

	return success
}

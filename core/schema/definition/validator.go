package definition

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/utils"
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
	OriginalRoot map[string]any
	RootData     any
	Data         any
	FunctionMap  PredicateMap
	MaxDepth     int // Maximum allowed nesting depth
	Mode         ValidationMode
	Visited      common.ResultSet
	Issues       []common.Issue
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
	addedConstraints := make(map[string]bool)

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
		false,
		false,
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
	parent    int
}

func (n *baseNode) GetPath() string        { return n.path } // builds path on demand from parents
func (n *baseNode) GetID() int             { return n.id }
func (n *baseNode) GetPathParts() []string { return n.pathParts }
func (n *baseNode) GetDependencies() []int { return n.deps }

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
	fieldDef      *Field
	schema        *Schema
	schemaRef     SchemaReference
	lookup        map[any]struct{}
	complex       []any
	expectNumeric bool // pre-computed when using resolved schema
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

type NestedSchemaNode struct {
	baseNode
}

type UnionValidationNode struct {
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

// collectGroupFieldNames recursively collects all field names referenced by
// any ConstraintRule within a ConstraintGroup (and any nested groups).
func collectGroupFieldNames(g *ConstraintGroup) []FieldName {
	var names []FieldName
	for _, ruleUnion := range g.Rules {
		switch ruleUnion.Kind() {
		case ConstraintKindRule:
			r, err := ConstraintAs[*ConstraintRule](ruleUnion)
			if err == nil {
				names = append(names, r.Fields...)
			}
		case ConstraintKindGroup:
			nested, err := ConstraintAs[*ConstraintGroup](ruleUnion)
			if err == nil {
				names = append(names, collectGroupFieldNames(nested)...)
			}
		}
	}
	return names
}

// getTopLevelConstraintsForPath extracts top-level constraints that apply to a specific field path.
// A ConstraintRule applies if any of its Fields references fieldPath or a sub-path of fieldPath.
// A ConstraintGroup applies if any rule within it (at any nesting depth) references such a field.
func getTopLevelConstraintsForPath(topLevelSchema *Schema, fieldPath string) SchemaConstraint {
	result := make(SchemaConstraint)

	fieldMatches := func(fieldName FieldName) bool {
		s := string(fieldName)
		return s == fieldPath || strings.HasPrefix(s, fieldPath+".")
	}

	for id, constraint := range topLevelSchema.Constraints {
		switch constraint.Kind() {
		case ConstraintKindRule:
			r, err := ConstraintAs[*ConstraintRule](constraint.ConstraintUnion)
			if err != nil {
				continue
			}
			if slices.ContainsFunc(r.Fields, fieldMatches) {
					result[id] = constraint
				}

		case ConstraintKindGroup:
			g, err := ConstraintAs[*ConstraintGroup](constraint.ConstraintUnion)
			if err != nil {
				continue
			}
			if slices.ContainsFunc(collectGroupFieldNames(g), fieldMatches) {
					result[id] = constraint
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
	addedConstraints map[string]bool,
	nsd *NestedSchema,
	schemaRefConstraints SchemaConstraint,
	topLevelSchema *Schema,
	buildCtx *buildContext,
	skipUnexpectedCheck bool,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	var rootNodes []*baseNode

	fieldsToProcess := schema.Fields
	if nsd != nil && nsd.Fields != nil {
		fieldsToProcess = nsd.Fields
	}

	var unexpectedNode *UnexpectedFieldsNode
	if !skipUnexpectedCheck {
		expectedFields := make(map[string]bool)
		for _, fieldDef := range fieldsToProcess {
			expectedFields[string(fieldDef.Name)] = true
		}

		unexpectedNode = graph.createUnexpectedFieldsNode(basePath, baseParts, expectedFields)
		graph.addNode(unexpectedNode)
		rootNodes = append(rootNodes, &unexpectedNode.baseNode)
	}

	var baseDepsForFields []int
	if unexpectedNode != nil {
		baseDepsForFields = []int{unexpectedNode.id}
	}

	var allFieldNodes []*baseNode
	for _, fieldDef := range fieldsToProcess {
		fieldNodes, err := graph.buildFieldNodes(&fieldDef, basePath, baseParts, baseDepsForFields, schema, addedConstraints, topLevelSchema, buildCtx, skipUnexpectedForObjects)

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
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	fieldPath, fieldPathParts := buildPathAndParts(basePath, baseParts, string(fieldDef.Name))
	currentDeps := baseDeps
	var nodes []*baseNode

	// TODO: Handle nullable — when true, skip required check and allow null
	if fieldDef.Required {
		parentPath := basePath
		parentPathParts := baseParts
		reqNode := graph.createRequiredFieldNode(fieldPath, fieldPathParts, string(fieldDef.Name), parentPath, parentPathParts, currentDeps)
		graph.addNode(reqNode)
		currentDeps = []int{reqNode.GetID()}
		nodes = append(nodes, &reqNode.baseNode)
	}

	isContainer := fieldDef.Type.IsComplex()
	if !isContainer {
		typeNode := graph.createTypeCheckNode(fieldPath, fieldPathParts, fieldDef, currentDeps)
		graph.addNode(typeNode)
		currentDeps = []int{typeNode.id}
		nodes = append(nodes, &typeNode.baseNode)
	}

	typeSpecificNodes, err := graph.buildFieldTypeNodes(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, addedConstraints, topLevelSchema, buildCtx, skipUnexpectedForObjects)
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
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	var node ValidationNode
	var nodes []*baseNode
	var err error

	switch fieldDef.Type {
	case FieldTypeEnum:
		node, err = graph.buildEnumNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema)

	case FieldTypeArray:
		node, err = graph.buildArrayNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeRecord:
		node, err = graph.buildRecordNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeUnion:
		node, err = graph.buildUnionNode(fieldDef, fieldPath, fieldPathParts, currentDeps, sc, topLevelSchema, buildCtx)

	case FieldTypeComposite:
		compositeNodes, err := graph.buildCompositeNode(fieldDef, fieldPath, fieldPathParts, topLevelSchema, buildCtx)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, compositeNodes...)
		return nodes, nil

	case FieldTypeGeometry:
		node = &GeometryValidationNode{
			baseNode: baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: currentDeps},
		}

	case FieldTypeObject:
		objectNodes, err := graph.buildObjectFieldNodes(fieldDef, fieldPath, fieldPathParts, addedConstraints, topLevelSchema, buildCtx, skipUnexpectedForObjects)
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

	// Single or multiple references
	var refs []SchemaReference
	if fieldDef.Schema.IsSingle() {
		single, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
		if err != nil {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve enum schema: %v", err)).WithPath(fieldPath).WithCause(err)
		}
		refs = []SchemaReference{single}
	} else if fieldDef.Schema.IsMultiple() {
		multi, err := FieldSchemaAs[[]SchemaReference](fieldDef.Schema)
		if err != nil {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve multiple enum schemas: %v", err)).WithPath(fieldPath).WithCause(err)
		}
		refs = multi
	} else {
		return nil, ErrInvalidSchema.WithMessage("Enum schema reference must be single or multiple").WithPath(fieldPath)
	}

	// Collect all enum values from all referenced schemas
	var allLookup map[any]struct{}
	var allComplex []any
	var enumType FieldType // will be the common type; if inconsistent, we may need to handle or error

	for _, ref := range refs {
		var lookup map[any]struct{}
		var complex []any
		var typ FieldType

		// Named schema
		if ref.ID != "" {
			nestedSchema, exists := topLevelSchema.Schemas[ref.ID]
			if !exists {
				return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Enum schema '%s' not found", ref.ID)).WithPath(fieldPath)
			}
			if len(nestedSchema.Values) == 0 {
				return nil, ErrInvalidSchema.WithMessagef("Enum schema %s has no values defined", ref.ID).WithPath(fieldPath)
			}
			typ = nestedSchema.Type
			lookup, complex = buildEnumLookup(nestedSchema.Values)
		} else {
			// Inline descriptor (empty ID)
			if ref.Type == 0 {
				return nil, ErrInvalidSchema.WithMessage("Inline enum descriptor missing 'type'").WithPath(fieldPath)
			}
			if len(ref.Values) == 0 {
				return nil, ErrInvalidSchema.WithMessage("Inline enum descriptor missing 'values'").WithPath(fieldPath)
			}
			typ = ref.Type
			lookup, complex = buildEnumLookup(ref.Values)
		}

		// Merge into the global set
		if allLookup == nil {
			allLookup = lookup
			allComplex = complex
			enumType = typ
		} else {
			// Merge maps
			for k := range lookup {
				allLookup[k] = struct{}{}
			}
			allComplex = append(allComplex, complex...)
			// Optionally check that types are consistent across references
			if typ != enumType && enumType != 0 {
				// Log a warning or return an error? We'll allow mixed types but validation may be loose.
				// For now, keep the first type.
			}
		}
	}

	if allLookup == nil && len(allComplex) == 0 {
		return nil, ErrInvalidSchema.WithMessage("Enum field has no values after merging references").WithPath(fieldPath)
	}

	// For simplicity, we store the merged lookup and complex values in the node.
	// The original refs are not needed at runtime; we keep the first ref for metadata.
	firstRef := refs[0]

	return &EnumValidationNode{
		baseNode:  baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: currentDeps},
		fieldDef:  fieldDef,
		schema:    sc,
		schemaRef: firstRef, // just for metadata; not used in validation
		lookup:    allLookup,
		complex:   allComplex,
	}, nil
}

func buildEnumLookup(values []LiteralValue) (map[any]struct{}, []any) {
	lookup := make(map[any]struct{}, len(values))
	var complex []any
	for _, v := range values {
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
	return lookup, complex
}

func (graph *ValidationGraph) buildObjectFieldNodes(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
	buildCtx *buildContext,
	skipUnexpected bool,
) ([]*baseNode, error) {
	var nodes []*baseNode

	if fieldDef.Schema.IsZero() {
		return nil, ErrSchemaNotFound.WithMessage("FieldSchemaReference is zero/uninitialized").WithPath(fieldPath)
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedSchema, exists := topLevelSchema.Schemas[schemaRef.ID]
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

	nestedNodes, err := graph.buildFromSchema(effectiveSchema, fieldPath, fieldPathParts, addedConstraints, &nestedSchema, schemaRef.Constraints, topLevelSchema, buildCtx, skipUnexpected, false)
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
	addedConstraints map[string]bool,
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
	skipUnexpectedCheck bool,
	skipUnexpectedForObjects bool,
) (*ValidationGraph, error) {
	tempSchema := &Schema{
		BaseSchema: BaseSchema{
			Name:   fmt.Sprintf("subgraph_%s", basePath),
			Fields: map[FieldId]Field{FieldId(rootFieldName): *rootFieldDef},
		},
		Schemas: originalTopLevelSchema.Schemas,
	}

	subGraph := newValidationGraph()
	addedConstraints := make(map[string]bool)

	if _, err := subGraph.buildFromSchema(tempSchema, "", nil, addedConstraints, nil, nil, originalTopLevelSchema, buildCtx, skipUnexpectedCheck, skipUnexpectedForObjects); err != nil {
		return nil, err
	}

	err := subGraph.finalize()
	return subGraph, err
}

// buildContainerNode is the shared implementation for array and record nodes.
// Both field types follow the same schema-resolution, effective-type detection,
// and sub-graph construction logic; they differ only in the concrete node type
// they return when a schema is absent, and in the node they wrap the sub-graph in.
func (graph *ValidationGraph) buildContainerNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	sc *Schema,
	topLevelSchema *Schema,
	buildCtx *buildContext,
	itemKind FieldType, // FieldTypeArray or FieldTypeRecord
	errContext string,
) (ValidationNode, error) {
	// Helper to create the final node (ArrayValidationNode or RecordValidationNode)
	makeNode := func(subGraph *ValidationGraph) ValidationNode {
		bn := baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps}
		if itemKind == FieldTypeArray {
			return &ArrayValidationNode{baseNode: bn, fieldDef: fieldDef, schema: sc, graph: subGraph}
		}
		return &RecordValidationNode{baseNode: bn, fieldDef: fieldDef, schema: sc, graph: subGraph}
	}

	// No schema reference → no item validation (treat as []any or map[string]any)
	if fieldDef.Schema.IsZero() {
		return makeNode(nil), nil
	}

	// Only single references are supported for inline descriptors
	if !fieldDef.Schema.IsSingle() {
		return nil, ErrInvalidSchema.WithMessage(fmt.Sprintf("%s cannot reference multiple schemas", errContext)).WithPath(fieldPath)
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference for %s: %v", errContext, err)).WithPath(fieldPath).WithCause(err)
	}

	// --- Case 1: Named schema (non‑empty ID) ---------------------------------
	if schemaRef.ID != "" {
		nestedDef, exists := topLevelSchema.Schemas[schemaRef.ID]
		if !exists {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for %s", schemaRef.ID, errContext)).WithPath(fieldPath)
		}

		var tempRootFieldSchema FieldSchemaReference
		if nestedDef.Type == 0 && len(nestedDef.Fields) > 0 {
			tempRootFieldSchema = NewSchemaReference(SchemaReference{ID: schemaRef.ID})
		} else {
			tempRootFieldSchema = nestedDef.Schema
		}

		effectiveType := getNestedSchemaEffectiveType(&nestedDef)
		skipUnexpected := effectiveType == FieldTypeComposite || effectiveType == FieldTypeUnion

		tempRootField := &Field{
			Name: "item",
			FieldProperties: FieldProperties{
				Type:    effectiveType,
				Schema:  tempRootFieldSchema,
				Default: nestedDef.Default,
			},
		}

		subGraph, err := graph.createSubGraph("item", tempRootField, fieldPath, topLevelSchema, buildCtx, skipUnexpected, false)
		if err != nil {
			return nil, err
		}
		return makeNode(subGraph), nil
	}

	// --- Case 2: Inline descriptor (ID empty) --------------------------------
	if schemaRef.IsInline() {
		itemType := schemaRef.Type
		if itemType == 0 {
			return nil, ErrInvalidSchema.WithMessage("Inline descriptor missing 'type' field").WithPath(fieldPath)
		}

		// Build a simple sub‑graph that validates each item against the inline type
		subGraph := newValidationGraph()
		addedConstraints := make(map[string]bool)

		// Create a temporary field representing a single item
		itemField := &Field{
			Name: "item",
			FieldProperties: FieldProperties{
				Type: itemType,
				// No schema reference for primitive types; for "record" we need a simple object check
			},
		}
		if itemType == FieldTypeObject {
			// Inline "record" means any map[string]any, no further fields.
			// We create a synthetic object schema with no fields and skip unexpected checks.
			syntheticSchema := &Schema{
				BaseSchema: BaseSchema{
					Name:   "__inline_record",
					Fields: map[FieldId]Field{}, // no expected fields
				},
			}
			_, err := subGraph.buildFromSchema(syntheticSchema, "", nil, addedConstraints, nil, nil, topLevelSchema, buildCtx, true, false)
			if err != nil {
				return nil, err
			}
		} else {
			// Primitive type: just a TypeCheckNode
			typeNode := subGraph.createTypeCheckNode("item", []string{"item"}, itemField, nil)
			subGraph.addNode(typeNode)
		}

		if err := subGraph.finalize(); err != nil {
			return nil, err
		}
		return makeNode(subGraph), nil
	}

	// Unreachable: neither named nor inline
	return nil, ErrInvalidSchema.WithMessage(fmt.Sprintf("Invalid schema reference for %s", errContext)).WithPath(fieldPath)
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
	return graph.buildContainerNode(fieldDef, fieldPath, fieldPathParts, deps, sc, topLevelSchema, buildCtx, FieldTypeArray, "array items")
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
	return graph.buildContainerNode(fieldDef, fieldPath, fieldPathParts, deps, sc, topLevelSchema, buildCtx, FieldTypeRecord, "record items")
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

		unionGraph, err := graph.createSubGraph("root", tempRootField, fieldPath, topLevelSchema, buildCtx, true, false)
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

// collectCompositeVocabulary returns the set of all field names that are
// valid at the top level of a composite value.  It walks object parts
// directly and recurses one level into each variant of union parts.
// The walk is deliberately shallow: fields nested inside an object field
// live at a deeper path and are guarded by their own UnexpectedFieldsNode.
func collectCompositeVocabulary(refs []SchemaReference, topLevelSchema *Schema) map[string]bool {
	vocab := make(map[string]bool)

	var walk func(nestedDef *NestedSchema)
	walk = func(nestedDef *NestedSchema) {
		effectiveType := getNestedSchemaEffectiveType(nestedDef)

		switch effectiveType {
		case FieldTypeObject, 0: // object part: contribute its direct fields
			for _, f := range nestedDef.Fields {
				vocab[string(f.Name)] = true
			}

		case FieldTypeUnion, FieldTypeComposite:
			// resolve the union/composite refs and recurse into each variant
			if nestedDef.Schema.IsZero() {
				return
			}
			variantRefs, err := FieldSchemaAs[[]SchemaReference](nestedDef.Schema)
			if err != nil {
				return
			}
			for _, vRef := range variantRefs {
				variantDef, exists := topLevelSchema.Schemas[vRef.ID]
				if !exists {
					continue
				}
				walk(&variantDef)
			}
		}
	}

	for _, ref := range refs {
		nestedDef, exists := topLevelSchema.Schemas[ref.ID]
		if !exists {
			continue
		}
		walk(&nestedDef)
	}

	return vocab
}

// buildCompositeNode implements the merged-schema strategy for composite
// fields.  Instead of building one independent sub-graph per part (which
// causes each part's UnexpectedFieldsNode to reject fields that belong to
// sibling parts), we:
//
//  1. Build a single UnexpectedFieldsNode whose vocabulary is the union of
//     all fields reachable from every part (including union variant fields).
//  2. Inline object parts directly into the parent graph at the composite
//     path, exactly like buildObjectFieldNodes does for plain object fields.
//  3. For union parts, insert a UnionMemberNode — a marker that runs the
//     union's variant graphs against the composite-level value at execution
//     time, without any extra wrapping layer.
//
// This ensures UNEXPECTED_FIELD checks fire only for fields that are not
// in the vocabulary of any part, while the structural union check still
// enforces that exactly one variant matches.
func (graph *ValidationGraph) buildCompositeNode(
	fieldDef *Field,
	fieldPath string,
	fieldPathParts []string,
	topLevelSchema *Schema,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	if fieldDef.Schema.IsZero() || !fieldDef.Schema.IsMultiple() {
		return nil, ErrSchemaNotFound.WithMessage("Composite field must reference multiple schemas").WithPath(fieldPath)
	}

	refs, err := FieldSchemaAs[[]SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve schemas for composite: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	// --- 1. Build the merged UnexpectedFieldsNode --------------------------
	vocab := collectCompositeVocabulary(refs, topLevelSchema)
	unexpectedNode := graph.createUnexpectedFieldsNode(fieldPath, fieldPathParts, vocab)
	graph.addNode(unexpectedNode)

	// All part nodes depend on the shared unexpected-fields gate.
	baseDeps := []int{unexpectedNode.id}
	var allPartNodes []*baseNode

	// --- 2. Walk each part -------------------------------------------------
	for _, ref := range refs {
		nestedDef, exists := topLevelSchema.Schemas[ref.ID]
		if !exists {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for composite", ref.ID)).WithPath(fieldPath)
		}

		effectiveType := getNestedSchemaEffectiveType(&nestedDef)

		switch effectiveType {

		case FieldTypeObject, 0:
			// Object part: inline its fields directly into the parent graph
			// at the composite path.  skipUnexpectedCheck=true because we
			// already have one unified UnexpectedFieldsNode above.
			effectiveSchema := &Schema{BaseSchema: nestedDef.BaseSchema}

			buildCtx.markBuilding(ref.ID)
			partNodes, err := graph.buildFromSchema(
				effectiveSchema,
				fieldPath,
				fieldPathParts,
				make(map[string]bool), // fresh constraint dedup per part
				&nestedDef,
				ref.Constraints,
				topLevelSchema,
				buildCtx,
				true, // skip unexpected check — handled by unified node above
				true,
			)
			buildCtx.unmarkBuilding(ref.ID)

			if err != nil {
				return nil, err
			}

			// Re-wire: when skipUnexpectedCheck=true, buildFromSchema sets
			// baseDepsForFields to nil, so the field nodes have no gating
			// dependency. We inject the shared UnexpectedFieldsNode as a
			// dependency on each returned root node so that field validation
			// is still ordered after the boundary check.
			for _, pn := range partNodes {
				pn.deps = append([]int{unexpectedNode.id}, pn.deps...)
				graph.dependencies[pn.id] = pn.deps
			}

			allPartNodes = append(allPartNodes, partNodes...)

		case FieldTypeUnion:
			// Union part: build per-variant sub-graphs (reusing the existing
			// union graph builder) then wrap them in a UnionMemberNode.
			if nestedDef.Schema.IsZero() || !nestedDef.Schema.IsMultiple() {
				return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Union part '%s' in composite must reference multiple schemas", ref.ID)).WithPath(fieldPath)
			}

			variantRefs, err := FieldSchemaAs[[]SchemaReference](nestedDef.Schema)
			if err != nil {
				return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve union variants for composite part '%s': %v", ref.ID, err)).WithPath(fieldPath).WithCause(err)
			}

			variantGraphs := make([]*ValidationGraph, 0, len(variantRefs))
			for _, vRef := range variantRefs {
				variantDef, exists := topLevelSchema.Schemas[vRef.ID]
				if !exists {
					return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Union variant schema '%s' not found", vRef.ID)).WithPath(fieldPath)
				}

				var tempRootField *Field
				if len(variantDef.Fields) > 0 {
					tempRootField = &Field{
						Name: "root",
						FieldProperties: FieldProperties{
							Type:   FieldTypeObject,
							Schema: NewSchemaReference(vRef),
						},
					}
				} else {
					tempRootField = &Field{
						Name:            "root",
						FieldProperties: variantDef.FieldProperties,
					}
				}

				// skipUnexpectedObjects=true propagates into any
				// buildObjectFieldNodes call inside this sub-graph so that
				// nested object expansions also skip their own
				// UnexpectedFieldsNode — the composite's unified node is
				// the sole boundary for the whole flat merged object.
				vGraph, err := graph.createSubGraph("root", tempRootField, fieldPath, topLevelSchema, buildCtx, true, true)
				if err != nil {
					return nil, err
				}
				variantGraphs = append(variantGraphs, vGraph)
			}

			memberNode := &UnionValidationNode{
				baseNode: baseNode{
					id:        graph.buildNodeID(),
					path:      fieldPath,
					pathParts: fieldPathParts,
					deps:      baseDeps,
				},
				graphs: variantGraphs,
			}
			graph.addNode(memberNode)
			allPartNodes = append(allPartNodes, &memberNode.baseNode)

		default:
			return nil, ErrInvalidSchema.WithMessagef(
				"Composite part '%s' has unsupported effective type '%s'; composite parts must be object or union schemas",
				ref.ID, effectiveType,
			).WithPath(fieldPath)
		}
	}

	// Return the unexpected node + all part nodes so the caller can wire
	// constraints that depend on all of them.
	result := make([]*baseNode, 0, 1+len(allPartNodes))
	result = append(result, &unexpectedNode.baseNode)
	result = append(result, allPartNodes...)
	return result, nil
}

func (graph *ValidationGraph) buildFromConstraintRuleWithScope(
	rule Constraint,
	basePath string,
	baseParts []string,
	deps []int,
	addedConstraints map[string]bool,
	scope ConstraintScope,
) []int {
	var ruleDepIDs []int

	// Dedup key is stable: constraint name + the path at which it is installed.
	// This prevents the same named constraint from being added more than once
	// at the same path (e.g. when composite parts share effective constraints).
	dedupKey := basePath + ":" + rule.Name

	switch rule.Kind() {
	case ConstraintKindRule:
		r, err := ConstraintAs[*ConstraintRule](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		if addedConstraints[dedupKey] {
			return nil
		}

		absoluteFieldPaths, absoluteFieldPathParts := resolveConstraintFieldPaths(basePath, baseParts, r.Fields)

		nodeID := graph.buildNodeID()
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
		addedConstraints[dedupKey] = true
		ruleDepIDs = append(ruleDepIDs, node.id)

	case ConstraintKindGroup:
		_, err := ConstraintAs[*ConstraintGroup](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		if addedConstraints[dedupKey] {
			return nil
		}

		g, _ := ConstraintAs[*ConstraintGroup](rule.ConstraintUnion)
		nodeID := graph.buildNodeID()
		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: basePath, pathParts: baseParts, deps: deps},
			group:    *g,
			Name:     rule.Name,
			scope:    scope,
		}

		graph.addNode(node)
		addedConstraints[dedupKey] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}

	return ruleDepIDs
}

// =============================================================================
// RESOLVED-SCHEMA GRAPH BUILDER
// =============================================================================
//
// These methods mirror the raw-*Schema builder above but consume the
// pre-compiled ResolvedSchema IR.  They produce the same ValidationNode DAG
// without re-resolving types, building enum lookups, or computing field paths.

// resolvedConstraintToConstraint converts a ResolvedConstraint back to a raw
// Constraint so the existing runtime Execute methods can consume it unchanged.
func resolvedConstraintToConstraint(rc ResolvedConstraint) Constraint {
	c := Constraint{Name: rc.Name}
	if rc.Rule != nil {
		c.ConstraintUnion = NewConstrainUnion(&ConstraintRule{
			Predicate:  rc.Rule.Predicate,
			Fields:     rc.Rule.Fields,
			Parameters: rc.Rule.Parameters,
		})
	} else if rc.Group != nil {
		members := make([]ConstraintUnion, len(rc.Group.Members))
		for i, m := range rc.Group.Members {
			mc := resolvedConstraintToConstraint(m)
			members[i] = mc.ConstraintUnion
		}
		c.ConstraintUnion = NewConstrainUnion(&ConstraintGroup{
			Operator: rc.Group.Operator,
			Rules:    members,
		})
	}
	return c
}

// buildFromResolvedConstraintRuleWithScope is the resolved equivalent of
// buildFromConstraintRuleWithScope.  It uses the pre-computed paths from
// ResolvedConstraint rather than calling resolveConstraintFieldPaths.
func (graph *ValidationGraph) buildFromResolvedConstraintRuleWithScope(
	rc ResolvedConstraint,
	basePath string,
	baseParts []string,
	deps []int,
	addedConstraints map[string]bool,
) []int {
	dedupKey := basePath + ":" + rc.Name

	if rc.Rule != nil {
		if addedConstraints[dedupKey] {
			return nil
		}

		nodeID := graph.buildNodeID()
		node := &ConstraintNode{
			baseNode:            baseNode{id: nodeID, path: basePath, pathParts: baseParts, deps: deps},
			constraint:          resolvedConstraintToConstraint(rc),
			fieldPaths:          rc.AbsFieldPaths,
			fieldPathParts:      rc.AbsFieldParts,
			constraintPath:      basePath,
			constraintPathParts: baseParts,
			scope:               rc.Scope,
		}
		graph.addNode(node)
		addedConstraints[dedupKey] = true
		return []int{node.id}
	}

	if rc.Group != nil {
		if addedConstraints[dedupKey] {
			return nil
		}

		memberUnions := make([]ConstraintUnion, len(rc.Group.Members))
		for i, m := range rc.Group.Members {
			mc := resolvedConstraintToConstraint(m)
			memberUnions[i] = mc.ConstraintUnion
		}

		nodeID := graph.buildNodeID()
		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: basePath, pathParts: baseParts, deps: deps},
			group: ConstraintGroup{
				Operator: rc.Group.Operator,
				Rules:    memberUnions,
			},
			Name:  rc.Name,
			scope: rc.Scope,
		}
		graph.addNode(node)
		addedConstraints[dedupKey] = true
		return []int{node.id}
	}

	return nil
}

// buildFromResolvedConstraints is the resolved equivalent of
// buildFromEffectiveConstraints.
func (graph *ValidationGraph) buildFromResolvedConstraints(
	constraints []ResolvedConstraint,
	baseParts []string,
	deps []int,
	addedConstraints map[string]bool,
) []int {
	var ruleDepIDs []int
	for _, rc := range constraints {
		ids := graph.buildFromResolvedConstraintRuleWithScope(rc, "", baseParts, deps, addedConstraints)
		ruleDepIDs = append(ruleDepIDs, ids...)
	}
	return ruleDepIDs
}

// buildResolvedEnumNode creates an EnumValidationNode from a resolved enum field.
func (graph *ValidationGraph) buildResolvedEnumNode(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
) (ValidationNode, error) {
	return &EnumValidationNode{
		baseNode:      baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps},
		fieldDef:      &Field{FieldProperties: FieldProperties{Type: rf.Type}},
		lookup:        rf.Enum.Lookup,
		complex:       rf.Enum.Complex,
		expectNumeric: rf.Enum.ExpectNumeric,
	}, nil
}

// buildResolvedObjectFieldNodes is the resolved equivalent of
// buildObjectFieldNodes.  It uses the pre-resolved pointer rather than
// looking up schemas by ID.
func (graph *ValidationGraph) buildResolvedObjectFieldNodes(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	rs *ResolvedSchema,
	addedConstraints map[string]bool,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
	skipUnexpected bool,
) ([]*baseNode, error) {
	var nodes []*baseNode

	// Determine the target schema and constraints from either the
	// pre-resolved object pointer or the recursive back-reference.
	var (
		schemaID       SchemaId
		schemaName     string
		refConstraints SchemaConstraint
		rns            *ResolvedNestedSchema
	)
	if rf.Recursive != nil {
		schemaID = rf.Recursive.SchemaID
		schemaName = rf.Recursive.SchemaName
		refConstraints = rf.Recursive.RefConstraints
		var exists bool
		rns, exists = rs.Schemas[schemaID]
		if !exists {
			return nil, ErrSchemaNotFound.
				WithMessage(fmt.Sprintf("recursive schema '%s' not found", schemaID)).
				WithPath(fieldPath)
		}
	} else if rf.Object != nil && rf.Object.Schema != nil {
		schemaID = rf.Object.Schema.ID
		schemaName = rf.Object.Schema.Name
		refConstraints = rf.Object.RefConstraints
		rns = rf.Object.Schema
	} else {
		return nil, ErrSchemaNotFound.
			WithMessage("object field has no resolved schema").WithPath(fieldPath)
	}

	// Runtime recursion detection via build context.
	if buildCtx.isRecursive(schemaID) {
		recursiveGraph, err := buildCtx.getOrBuildResolvedRecursiveGraph(
			schemaID, rns, refConstraints, compiler, rs,
		)
		if err != nil {
			return nil, err
		}

		markerNode := &RecursionMarkerNode{
			baseNode: baseNode{
				id:        graph.buildNodeID(),
				path:      fieldPath,
				pathParts: fieldPathParts,
			},
			validationGraph: recursiveGraph,
			schemaName:      schemaName,
		}
		graph.addNode(markerNode)
		nodes = append(nodes, &markerNode.baseNode)
		return nodes, nil
	}

	// Non-recursive: inline the nested schema at this path.
	buildCtx.markBuilding(schemaID)
	defer buildCtx.unmarkBuilding(schemaID)

	nestedNodes, err := graph.buildFromResolvedSchema(
		rs, fieldPath, fieldPathParts, addedConstraints,
		rns, refConstraints,
		compiler, buildCtx, skipUnexpected, false,
	)
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

// buildResolvedContainerNode is the resolved equivalent of buildContainerNode.
func (graph *ValidationGraph) buildResolvedContainerNode(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	rs *ResolvedSchema,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
	itemKind FieldType,
) (ValidationNode, error) {
	makeNode := func(subGraph *ValidationGraph) ValidationNode {
		bn := baseNode{id: graph.buildNodeID(), path: fieldPath, pathParts: fieldPathParts, deps: deps}
		if itemKind == FieldTypeArray {
			return &ArrayValidationNode{baseNode: bn, fieldDef: &Field{FieldProperties: FieldProperties{Type: rf.Type}}, graph: subGraph}
		}
		return &RecordValidationNode{baseNode: bn, fieldDef: &Field{FieldProperties: FieldProperties{Type: rf.Type}}, graph: subGraph}
	}

	// Handle recursive container: rf.Recursive is set instead of
	// rf.Container when the item schema closes a recursive cycle.
	if rf.Recursive != nil {
		rns, exists := rs.Schemas[rf.Recursive.SchemaID]
		if !exists {
			return nil, ErrSchemaNotFound.
				WithMessage(fmt.Sprintf("recursive container schema '%s' not found", rf.Recursive.SchemaID)).
				WithPath(fieldPath)
		}

		itemRF := &ResolvedField{
			Name:  "item",
			Path:  "item",
			Parts: []string{"item"},
			Type:  rns.EffectiveType,
			Object: &ResolvedObjectField{
				Schema:         rns,
				RefConstraints: rf.Recursive.RefConstraints,
			},
		}

		subGraph := newValidationGraph()
		subAdded := make(map[string]bool)
		subRs := &ResolvedSchema{Fields: []ResolvedField{*itemRF}, Schemas: rs.Schemas}
		if _, err := subGraph.buildFromResolvedSchema(subRs, "", nil, subAdded, nil, nil, compiler, buildCtx, true, false); err != nil {
			return nil, err
		}
		if err := subGraph.finalize(); err != nil {
			return nil, err
		}
		return makeNode(subGraph), nil
	}

	ct := rf.Container
	if ct == nil {
		return makeNode(nil), nil
	}

	// Named item schema
	if ct.ItemSchema != nil {
		skipUnexpected := ct.ItemSchema.EffectiveType == FieldTypeComposite ||
			ct.ItemSchema.EffectiveType == FieldTypeUnion

		// Composite and union-typed item schemas need the original schema
		// reference to reconstruct their validation graph (the resolved IR
		// stores composite/union data at the field level, not the schema
		// level).  Fall back to the raw schema path for these cases.
		if ct.ItemSchema.EffectiveType == FieldTypeComposite ||
			ct.ItemSchema.EffectiveType == FieldTypeUnion {
			if compiler != nil {
				ns, exists := compiler.source.Schemas[ct.ItemSchema.ID]
				if !exists {
					return nil, ErrSchemaNotFound.
						WithMessage(fmt.Sprintf("item schema '%s' not found", ct.ItemSchema.ID)).
						WithPath(fieldPath)
				}

				tempRootField := &Field{
					Name: "item",
					FieldProperties: FieldProperties{
						Type:   ct.ItemSchema.EffectiveType,
						Schema: ns.Schema,
					},
				}
				subGraph, err := graph.createSubGraph("item", tempRootField, fieldPath, compiler.source, buildCtx, skipUnexpected, false)
				if err != nil {
					return nil, err
				}
				return makeNode(subGraph), nil
			}
			return makeNode(nil), nil
		}

		subGraph := newValidationGraph()
		subAdded := make(map[string]bool)

		// Wrap the item schema as a field named "item"
		var itemRF *ResolvedField
		switch ct.ItemSchema.EffectiveType {
		case FieldTypeObject:
			itemRF = &ResolvedField{
				Name:   "item",
				Path:   "item",
				Parts:  []string{"item"},
				Type:   FieldTypeObject,
				Object: &ResolvedObjectField{Schema: ct.ItemSchema},
			}
		case FieldTypeEnum:
			itemRF = &ResolvedField{
				Name:  "item",
				Path:  "item",
				Parts: []string{"item"},
				Type:  ct.ItemSchema.EffectiveType,
				Enum:  ct.ItemSchema.Enum,
			}
		default:
			itemRF = &ResolvedField{
				Name:   "item",
				Path:   "item",
				Parts:  []string{"item"},
				Type:   ct.ItemSchema.EffectiveType,
				Scalar: &ResolvedScalar{},
			}
		}

		subRs := &ResolvedSchema{Fields: []ResolvedField{*itemRF}, Schemas: rs.Schemas}
		if _, err := subGraph.buildFromResolvedSchema(subRs, "", nil, subAdded, nil, nil, compiler, buildCtx, skipUnexpected, false); err != nil {
			return nil, err
		}
		if err := subGraph.finalize(); err != nil {
			return nil, err
		}
		return makeNode(subGraph), nil
	}

	// Inline item type descriptor
	if ct.ItemType != 0 {
		subGraph := newValidationGraph()
		subAdded := make(map[string]bool)

		if ct.ItemType == FieldTypeObject {
			syntheticSchema := &Schema{
				BaseSchema: BaseSchema{
					Name:   "__inline_record",
					Fields: map[FieldId]Field{},
				},
			}
			_, err := subGraph.buildFromSchema(syntheticSchema, "", nil, subAdded, nil, nil, nil, buildCtx, true, false)
			if err != nil {
				return nil, err
			}
		} else {
			itemField := &Field{
				Name: "item",
				FieldProperties: FieldProperties{
					Type: ct.ItemType,
				},
			}
			typeNode := subGraph.createTypeCheckNode("item", []string{"item"}, itemField, nil)
			subGraph.addNode(typeNode)
		}

		if err := subGraph.finalize(); err != nil {
			return nil, err
		}
		return makeNode(subGraph), nil
	}

	return makeNode(nil), nil
}

// buildResolvedUnionNode is the resolved equivalent of buildUnionNode.
func (graph *ValidationGraph) buildResolvedUnionNode(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	deps []int,
	rs *ResolvedSchema,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
) (ValidationNode, error) {
	if rf.Union == nil || len(rf.Union.Variants) == 0 {
		return nil, ErrSchemaNotFound.
			WithMessage("union field has no resolved variants").WithPath(fieldPath)
	}

	graphs := make([]*ValidationGraph, 0, len(rf.Union.Variants))
	for _, variant := range rf.Union.Variants {
		subGraph := newValidationGraph()
		subAdded := make(map[string]bool)

		itemRF := &ResolvedField{
			Name:  "root",
			Path:  "root",
			Parts: []string{"root"},
			Type:  variant.EffectiveType,
		}
		if variant.EffectiveType == FieldTypeObject {
			itemRF.Object = &ResolvedObjectField{Schema: variant}
		} else {
			itemRF.Scalar = &ResolvedScalar{}
		}

		subRs := &ResolvedSchema{Fields: []ResolvedField{*itemRF}, Schemas: rs.Schemas}
		if _, err := subGraph.buildFromResolvedSchema(subRs, "", nil, subAdded, nil, nil, compiler, buildCtx, true, false); err != nil {
			return nil, err
		}
		if err := subGraph.finalize(); err != nil {
			return nil, err
		}
		graphs = append(graphs, subGraph)
	}

	return &UnionValidationNode{
		baseNode: baseNode{
			id:        graph.buildNodeID(),
			path:      fieldPath,
			pathParts: fieldPathParts,
			deps:      deps,
		},
		fieldDef: &Field{FieldProperties: FieldProperties{Type: rf.Type}},
		graphs:   graphs,
	}, nil
}

// buildResolvedCompositeNode is the resolved equivalent of buildCompositeNode.
func (graph *ValidationGraph) buildResolvedCompositeNode(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	rs *ResolvedSchema,
	addedConstraints map[string]bool,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
) ([]*baseNode, error) {
	if rf.Composite == nil {
		return nil, ErrSchemaNotFound.
			WithMessage("composite field has no resolved data").WithPath(fieldPath)
	}

	// Build unified UnexpectedFieldsNode from pre-computed vocabulary
	vocab := rf.Composite.MergedVocabulary
	if vocab == nil {
		vocab = make(map[string]bool)
	}
	unexpectedNode := graph.createUnexpectedFieldsNode(fieldPath, fieldPathParts, vocab)
	graph.addNode(unexpectedNode)

	baseDeps := []int{unexpectedNode.id}
	var allPartNodes []*baseNode

	// Object parts
	for _, objPart := range rf.Composite.ObjectParts {
		buildCtx.markBuilding(objPart.ID)

		partNodes, err := graph.buildFromResolvedSchema(
			rs, fieldPath, fieldPathParts, addedConstraints,
			objPart, nil,
			compiler, buildCtx, true, true,
		)
		buildCtx.unmarkBuilding(objPart.ID)
		if err != nil {
			return nil, err
		}

		for _, pn := range partNodes {
			pn.deps = append([]int{unexpectedNode.id}, pn.deps...)
			graph.dependencies[pn.id] = pn.deps
		}
		allPartNodes = append(allPartNodes, partNodes...)
	}

	// Union parts
	for _, unionPart := range rf.Composite.UnionParts {
		variantGraphs := make([]*ValidationGraph, 0, len(unionPart.Variants))
		for _, variant := range unionPart.Variants {
			subGraph := newValidationGraph()
			subAdded := make(map[string]bool)

			itemRF := &ResolvedField{
				Name:  "root",
				Path:  "root",
				Parts: []string{"root"},
				Type:  variant.EffectiveType,
			}
			if variant.EffectiveType == FieldTypeObject {
				itemRF.Object = &ResolvedObjectField{Schema: variant}
			} else {
				itemRF.Scalar = &ResolvedScalar{}
			}

			subRs := &ResolvedSchema{Fields: []ResolvedField{*itemRF}, Schemas: rs.Schemas}
			if _, err := subGraph.buildFromResolvedSchema(subRs, "", nil, subAdded, nil, nil, compiler, buildCtx, true, true); err != nil {
				return nil, err
			}
			if err := subGraph.finalize(); err != nil {
				return nil, err
			}
			variantGraphs = append(variantGraphs, subGraph)
		}

		memberNode := &UnionValidationNode{
			baseNode: baseNode{
				id:        graph.buildNodeID(),
				path:      fieldPath,
				pathParts: fieldPathParts,
				deps:      baseDeps,
			},
			graphs: variantGraphs,
		}
		graph.addNode(memberNode)
		allPartNodes = append(allPartNodes, &memberNode.baseNode)
	}

	result := make([]*baseNode, 0, 1+len(allPartNodes))
	result = append(result, &unexpectedNode.baseNode)
	result = append(result, allPartNodes...)
	return result, nil
}

// buildResolvedFieldTypeNodes is the resolved equivalent of buildFieldTypeNodes.
func (graph *ValidationGraph) buildResolvedFieldTypeNodes(
	rf *ResolvedField,
	fieldPath string,
	fieldPathParts []string,
	currentDeps []int,
	rs *ResolvedSchema,
	addedConstraints map[string]bool,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	var node ValidationNode
	var nodes []*baseNode
	var err error

	// NOTE: Unlike the original raw-schema builder, we do NOT short-circuit
	// on rf.Recursive here.  Recursive back-references in the resolved IR
	// replace the type-specific pointer (Container, Object, etc.) but the
	// recursion must still be detected at graph-build time via buildCtx,
	// just like the original code.  The per-type handlers below (object,
	// container) check buildCtx.isRecursive() and create RecursionMarkerNode
	// in the correct context (e.g. inside a container's item sub-graph).

	switch rf.Type {
	case FieldTypeString, FieldTypeNumber, FieldTypeInteger, FieldTypeDecimal,
		FieldTypeBoolean, FieldTypeBytes, FieldTypeUnknown, FieldTypeGeometry:
		// No additional data needed; TypeCheckNode handles these.

	case FieldTypeEnum:
		node, err = graph.buildResolvedEnumNode(rf, fieldPath, fieldPathParts, currentDeps)

	case FieldTypeObject:
		objectNodes, err := graph.buildResolvedObjectFieldNodes(
			rf, fieldPath, fieldPathParts, rs, addedConstraints,
			compiler, buildCtx, skipUnexpectedForObjects,
		)
		if err != nil {
			return nil, err
		}
		if len(objectNodes) > 0 {
			nodes = append(nodes, objectNodes...)
		}
		return nodes, nil

	case FieldTypeArray:
		node, err = graph.buildResolvedContainerNode(rf, fieldPath, fieldPathParts, currentDeps, rs, compiler, buildCtx, FieldTypeArray)

	case FieldTypeRecord:
		node, err = graph.buildResolvedContainerNode(rf, fieldPath, fieldPathParts, currentDeps, rs, compiler, buildCtx, FieldTypeRecord)

	case FieldTypeUnion:
		node, err = graph.buildResolvedUnionNode(rf, fieldPath, fieldPathParts, currentDeps, rs, compiler, buildCtx)

	case FieldTypeComposite:
		compositeNodes, cerr := graph.buildResolvedCompositeNode(
			rf, fieldPath, fieldPathParts, rs, addedConstraints,
			compiler, buildCtx,
		)
		if cerr != nil {
			return nil, cerr
		}
		nodes = append(nodes, compositeNodes...)
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

// buildResolvedFieldNodes is the resolved equivalent of buildFieldNodes.
func (graph *ValidationGraph) buildResolvedFieldNodes(
	rf *ResolvedField,
	basePath string,
	baseParts []string,
	baseDeps []int,
	rs *ResolvedSchema,
	addedConstraints map[string]bool,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	// Compute the field path from the mount base path and field name.
	// We cannot use rf.Path / rf.Parts directly because those are the
	// absolute paths within the schema definition; when a nested schema
	// is inlined at a mount path (basePath != ""), the effective document
	// path is basePath "." fieldName.
	fieldPath, fieldPathParts := buildPathAndParts(basePath, baseParts, string(rf.Name))
	currentDeps := baseDeps
	var nodes []*baseNode

	if rf.Required {
		parentPath := basePath
		parentPathParts := baseParts
		reqNode := graph.createRequiredFieldNode(fieldPath, fieldPathParts, string(rf.Name), parentPath, parentPathParts, currentDeps)
		graph.addNode(reqNode)
		currentDeps = []int{reqNode.GetID()}
		nodes = append(nodes, &reqNode.baseNode)
	}

	isContainer := rf.Type.IsComplex()
	if !isContainer {
		typeNode := graph.createTypeCheckNode(fieldPath, fieldPathParts, &Field{FieldProperties: FieldProperties{Type: rf.Type}}, currentDeps)
		graph.addNode(typeNode)
		currentDeps = []int{typeNode.id}
		nodes = append(nodes, &typeNode.baseNode)
	}

	typeSpecificNodes, err := graph.buildResolvedFieldTypeNodes(
		rf, fieldPath, fieldPathParts, currentDeps,
		rs, addedConstraints, compiler, buildCtx, skipUnexpectedForObjects,
	)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, typeSpecificNodes...)

	return nodes, nil
}

// buildFromResolvedSchema builds validation graph nodes from a ResolvedSchema.
// It is the resolved equivalent of buildFromSchema.
func (graph *ValidationGraph) buildFromResolvedSchema(
	rs *ResolvedSchema,
	basePath string,
	baseParts []string,
	addedConstraints map[string]bool,
	rns *ResolvedNestedSchema,
	schemaRefConstraints SchemaConstraint,
	compiler *SchemaCompiler,
	buildCtx *buildContext,
	skipUnexpectedCheck bool,
	skipUnexpectedForObjects bool,
) ([]*baseNode, error) {
	var rootNodes []*baseNode

	// Determine which fields to process
	var fields []ResolvedField
	if rns != nil {
		fields = rns.Fields
	} else {
		fields = rs.Fields
	}

	// Unexpected fields gate
	var unexpectedNode *UnexpectedFieldsNode
	if !skipUnexpectedCheck {
		expectedFields := make(map[string]bool, len(fields))
		for _, f := range fields {
			expectedFields[string(f.Name)] = true
		}
		unexpectedNode = graph.createUnexpectedFieldsNode(basePath, baseParts, expectedFields)
		graph.addNode(unexpectedNode)
		rootNodes = append(rootNodes, &unexpectedNode.baseNode)
	}

	var baseDepsForFields []int
	if unexpectedNode != nil {
		baseDepsForFields = []int{unexpectedNode.id}
	}

	// Build field nodes
	var allFieldNodes []*baseNode
	for _, rf := range fields {
		fieldNodes, err := graph.buildResolvedFieldNodes(
			&rf, basePath, baseParts, baseDepsForFields,
			rs, addedConstraints, compiler, buildCtx, skipUnexpectedForObjects,
		)
		if err != nil {
			return nil, err
		}
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	// Build constraint nodes
	var constraintNodes []ResolvedConstraint
	if basePath == "" && rns == nil {
		// Root schema: use pre-compiled constraints
		constraintNodes = rs.Constraints
	} else if compiler != nil {
		// Nested schema at mount path: compile with three-level merge
		var nested SchemaConstraint
		if rns != nil {
			nested = rns.RawConstraints
		}
		var topLevel SchemaConstraint
		if basePath != "" {
			topLevel = compiler.TopLevelConstraintsForPath(basePath)
		}
		var err error
		constraintNodes, err = compiler.CompileConstraints(
			nested, schemaRefConstraints, topLevel, basePath,
		)
		if err != nil {
			return nil, err
		}
	}

	constraintDeps := getDependencyIDs(allFieldNodes)
	var constraintIDs []int
	if len(constraintNodes) > 0 {
		constraintIDs = graph.buildFromResolvedConstraints(constraintNodes, baseParts, constraintDeps, addedConstraints)
	}

	if len(constraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, baseParts, constraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes, nil
}

// getOrBuildResolvedRecursiveGraph is the resolved equivalent of
// getOrBuildRecursiveGraph.
func (ctx *buildContext) getOrBuildResolvedRecursiveGraph(
	schemaID SchemaId,
	rns *ResolvedNestedSchema,
	instanceConstraints SchemaConstraint,
	compiler *SchemaCompiler,
	rs *ResolvedSchema,
) (*ValidationGraph, error) {
	cacheKey := makeGraphCacheKey(schemaID, instanceConstraints)
	if cached, exists := ctx.recursiveGraphCache[cacheKey]; exists {
		return cached, nil
	}

	subGraph := newValidationGraph()
	subAdded := make(map[string]bool)
	ctx.recursiveGraphCache[cacheKey] = subGraph

	ctx.markBuilding(schemaID)
	defer ctx.unmarkBuilding(schemaID)

	if _, err := subGraph.buildFromResolvedSchema(
		rs, "", nil, subAdded,
		rns, instanceConstraints,
		compiler, ctx, false, false,
	); err != nil {
		delete(ctx.recursiveGraphCache, cacheKey)
		return nil, err
	}

	if err := subGraph.finalize(); err != nil {
		delete(ctx.recursiveGraphCache, cacheKey)
		return nil, err
	}

	return subGraph, nil
}

func (graph *ValidationGraph) traverse(fmap PredicateMap, document map[string]any, mode ValidationMode, maxDepth int, originalRoot map[string]any) ([]common.Issue, bool) {
	ctx := graph.ctxPool.Get().(*ValidationContext)
	defer graph.ctxPool.Put(ctx)
	ctx.OriginalRoot = originalRoot
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
	// Compile the schema into its resolved IR.
	rs, err := Compile(sc)
	if err != nil {
		return nil, err
	}

	// Create a SchemaCompiler for per-mount-path constraint compilation.
	compiler := newSchemaCompiler(sc)

	graph := newValidationGraph()
	addedConstraints := make(map[string]bool)
	buildCtx := newBuildContext()

	if _, err := graph.buildFromResolvedSchema(rs, "", nil, addedConstraints, nil, nil, compiler, buildCtx, false, false); err != nil {
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
	return v.graph.traverse(v.fmap, document, ValidationModeStrict, v.config.MaxDepth, document)
}

func (v *DocumentValidator) ValidatePartial(document map[string]any) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, ValidationModePartialStrict, v.config.MaxDepth, document)
}

func (v *DocumentValidator) ValidateLoose(document map[string]any) ([]common.Issue, bool) {
	return v.graph.traverse(v.fmap, document, ValidationModeLoose, v.config.MaxDepth, document)
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
	case FieldTypeBytes:
		if _, ok := value.([]byte); ok {
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
			Message: fmt.Sprintf("Expected %s, got invalid value '%v'", n.fieldDef.Type, value),
			Path:    n.path,
		}}}
	}

	expectNumeric := n.expectNumeric
	// Legacy fallback for old code path
	if n.schema != nil && n.schemaRef.ID != "" {
		if nested, ok := n.schema.Schemas[n.schemaRef.ID]; ok {
			expectNumeric = (nested.Type == FieldTypeNumber || nested.Type == FieldTypeDecimal || nested.Type == FieldTypeInteger)
		}
	}
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
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ctx.Mode, remainingDepth, ctx.OriginalRoot)

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
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "OBJECT_TYPE_MISMATCH", Message: fmt.Sprintf("Expected object for record, got %v", value), Path: n.path}}}
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
		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ctx.Mode, remainingDepth, ctx.OriginalRoot)

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

func isReflectNumeric(v reflect.Value) bool {
	k := v.Kind()
	return (k >= reflect.Int && k <= reflect.Uint64) || k == reflect.Float32 || k == reflect.Float64
}

// Execute is intentionally a no-op. NestedSchemaNode exists solely as a DAG
// synchronization barrier: its dependency edges encode "all field-validation
// nodes for this nested schema have completed". Constraints that must fire
// after the full schema has been checked declare a dependency on this node,
// ensuring correct topological ordering without performing any validation
// themselves.
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
		ctx.OriginalRoot)

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
	if !exists || value == nil {
		return success
	}

	currentDepth := getPathDepth(n.pathParts)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code:    "MAX_DEPTH_EXCEEDED",
				Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
				Path:    n.path,
			}},
		}
	}

	type variantResult struct {
		success bool
		issues  []common.Issue
	}
	results := make([]variantResult, len(n.graphs))

	// Evaluate all variants
	for idx, graph := range n.graphs {
		wrapped := map[string]any{"root": value}
		issues, ok := graph.traverse(ctx.FunctionMap, wrapped, ctx.Mode, ctx.MaxDepth, ctx.OriginalRoot)

		// Rewrite paths and add variant context
		for i := range issues {
			if strings.HasPrefix(issues[i].Path, "root") {
				issues[i].Path = strings.Replace(issues[i].Path, "root", n.path, 1)
			}
			issues[i].Message = fmt.Sprintf("[variant %d] %s", idx, issues[i].Message)
		}

		// If a variant fails silently (no issues), add an internal error
		if !ok && len(issues) == 0 {
			issues = []common.Issue{{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("Variant %d failed without reporting any issues", idx),
				Path:    n.path,
			}}
		}

		results[idx] = variantResult{
			success: ok,
			issues:  issues,
		}
	}

	// Find any variant that succeeded with zero issues
	var succeeded bool
	var allIssues []common.Issue
	for _, res := range results {
		if res.success && len(res.issues) == 0 {
			succeeded = true
		}
		allIssues = append(allIssues, res.issues...)
	}

	if succeeded {
		return success
	}

	// No variant matched → add synthetic UNION_MISMATCH issue
	syntheticIssue := common.Issue{
		Code:    "UNION_MISMATCH",
		Message: fmt.Sprintf("Value at '%s' did not match any variant of the union", n.path),
		Path:    n.path,
	}

	allIssues = append([]common.Issue{syntheticIssue}, allIssues...)

	return &NodeResult{
		Success: false,
		Issues:  allIssues,
	}
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
	r, err := ConstraintAs[*ConstraintRule](n.constraint.ConstraintUnion)
	if err != nil {
		return &NodeResult{
			Success: false,
			Issues: []common.Issue{{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("ConstraintNode '%s': failed to extract ConstraintRule: %v", n.constraint.Name, err),
				Path:    n.constraintPath,
			}},
		}
	}

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
	return n.evaluateGroup(ctx, n.group, n.path, n.Name)
}

func (n *ConstraintGroupNode) evaluateGroup(ctx *ValidationContext, group ConstraintGroup, path string, name string) *NodeResult {
	var results []bool
	var memberIssues []common.Issue

	for _, ruleUnion := range group.Rules {
		var res *NodeResult

		switch ruleUnion.Kind() {
		case ConstraintKindRule:
			r, err := ConstraintAs[*ConstraintRule](ruleUnion)
			if err != nil {
				return &NodeResult{
					Success: false,
					Issues: []common.Issue{{
						Code:    "INTERNAL_ERROR",
						Message: fmt.Sprintf("Failed to extract constraint rule: %v", err),
						Path:    path,
					}},
				}
			}
			res = runConstraintPredicate(ctx, r, path)

		case ConstraintKindGroup:
			g, err := ConstraintAs[*ConstraintGroup](ruleUnion)
			if err != nil {
				return &NodeResult{
					Success: false,
					Issues: []common.Issue{{
						Code:    "INTERNAL_ERROR",
						Message: fmt.Sprintf("Failed to extract constraint group: %v", err),
						Path:    path,
					}},
				}
			}
			res = n.evaluateGroup(ctx, *g, path, name)

		default:
			return &NodeResult{
				Success: false,
				Issues: []common.Issue{{
					Code:    "INTERNAL_ERROR",
					Message: fmt.Sprintf("Unknown constraint kind %d in group '%s'", ruleUnion.Kind(), name),
					Path:    path,
				}},
			}
		}

		results = append(results, res.Success)
		if !res.Success {
			memberIssues = append(memberIssues, res.Issues...)
		}
	}

	if ok, _ := group.Operator.Evaluate(results); !ok {
		return &NodeResult{
			Success: false,
			Issues: append([]common.Issue{{
				Code:    "CONSTRAINT_GROUP_VIOLATION",
				Message: fmt.Sprintf("Constraint group '%s' failed", name),
				Path:    path,
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
		Root:       ctx.OriginalRoot,
		Data:       ctx.Data,
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
func getNestedSchemaEffectiveType(nestedDef *NestedSchema) FieldType {
	if nestedDef.Type != 0 {
		return nestedDef.Type
	}
	if len(nestedDef.Fields) > 0 {
		return FieldTypeObject
	}
	return nestedDef.Type
}

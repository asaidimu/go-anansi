package definition

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

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
		MaxDepth: 100,
		Mode:     ValidationModeStrict,
	}
}

type DocumentValidator struct {
	fmap   PredicateMap
	graph  *ValidationGraph
	config ValidationConfig
}

// ValidationMode defines the strictness level for validation operations.
type ValidationMode string

const (
	// ValidationModeStrict validates all fields and applies all constraints,
	// returning issues for missing required fields, type mismatches, and unexpected fields.
	ValidationModeStrict ValidationMode = "strict"

	// ValidationModePartialStrict skips validation for missing required fields if they are not present in the document,
	// but still validates present fields for type mismatches, unexpected fields, and constraints.
	ValidationModePartialStrict ValidationMode = "partial_strict"

	// ValidationModeLoose skips validation for missing required fields and unexpected fields.
	// It only validates present fields for type mismatches and constraints.
	ValidationModeLoose ValidationMode = "loose"
)

// ValidationNode represents a single validation operation in the graph.
type ValidationNode interface {
	Execute(ctx *ValidationContext) *NodeResult
	GetDependencies() []string
	GetID() string
	GetPath() string
}

// NodeResult holds the outcome of a single node's execution.
type NodeResult struct {
	Value   any
	Issues  []common.Issue
	Success bool
	Skipped bool
}

// ValidationContext holds the state during a validation traversal.
type ValidationContext struct {
	RootData    any
	Data        any
	Results     map[string]*NodeResult
	FunctionMap PredicateMap
	MaxDepth    int // Maximum allowed nesting depth
	Mode        ValidationMode
}

// ValidationGraph represents the compiled set of validation operations.
type ValidationGraph struct {
	nodes        map[string]ValidationNode
	dependencies map[string][]string
	visitedState map[string]int // For cycle detection during graph building
}

// ConstraintSpecificity represents the level at which a constraint is defined
type ConstraintSpecificity int

const (
	SpecificityNestedSchema ConstraintSpecificity = iota
	SpecificitySchemaReference
	SpecificityTopLevel
)

// EffectiveConstraint represents a constraint with its specificity level
type EffectiveConstraint struct {
	Constraint  Constraint
	Specificity ConstraintSpecificity
	BasePath    string // The base path where this constraint is defined
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
func (cr *ConstraintRegistry) Add(name string, constraint Constraint, specificity ConstraintSpecificity, basePath string) {
	existing, exists := cr.constraints[name]

	// If doesn't exist or new constraint is more specific, add it
	if !exists || specificity > existing.Specificity {
		cr.constraints[name] = EffectiveConstraint{
			Constraint:  constraint,
			Specificity: specificity,
			BasePath:    basePath,
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

type TypeCheckNode struct {
	baseNode
	fieldDef *Field
}

type EnumValidationNode struct {
	baseNode
	fieldDef  *Field
	schema    *Schema
	schemaRef SchemaReference
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
	constraint     Constraint
	fieldPaths     []string // Absolute paths to the fields this constraint validates
	constraintPath string   // The path context where constraint is defined
}

type ConstraintGroupNode struct {
	baseNode
	group     ConstraintGroup
	memberIDs []string
	Name      string
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

// resolveConstraintFieldPaths converts relative field paths to absolute paths
// basePath: where the constraint is defined (e.g., "user")
// fieldNames: field paths from constraint (e.g., ["email.address"])
// returns: absolute paths (e.g., ["user.email.address"])
func resolveConstraintFieldPaths(basePath string, fieldNames []FieldName) []string {
	result := make([]string, len(fieldNames))
	for i, fieldName := range fieldNames {
		if basePath == "" {
			result[i] = string(fieldName)
		} else {
			result[i] = buildPath(basePath, string(fieldName))
		}
	}
	return result
}

// getPathDepth returns the nesting depth of a path
func getPathDepth(path string) int {
	if path == "" {
		return 0
	}
	// Count dots and array indices
	depth := strings.Count(path, ".")
	// Count array accesses [N]
	depth += strings.Count(path, "[")
	return depth
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

func (graph *ValidationGraph) createRequiredFieldNode(fieldPath string, fieldName string, deps []string) *RequiredFieldNode {
	return &RequiredFieldNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "required", ""), path: fieldPath, deps: deps},
		fieldName: fieldName,
	}
}

func (graph *ValidationGraph) createTypeCheckNode(fieldPath string, fieldDef *Field, deps []string) *TypeCheckNode {
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

	// Level 1: Nested schema constraints (least specific)
	for _, constraint := range nestedSchemaConstraints {
		registry.Add(constraint.Name, constraint, SpecificityNestedSchema, basePath)
	}

	// Level 2: Schema reference constraints (medium specific)
	for _, constraint := range schemaRefConstraints {
		registry.Add(constraint.Name, constraint, SpecificitySchemaReference, basePath)
	}

	// Level 3: Top-level constraints (most specific)
	for _, constraint := range topLevelConstraints {
		registry.Add(constraint.Name, constraint, SpecificityTopLevel, basePath)
	}

	return registry.GetEffective()
}

// getTopLevelConstraintsForPath extracts top-level constraints that apply to a specific field path
func getTopLevelConstraintsForPath(topLevelSchema *Schema, fieldPath string) SchemaConstraint {
	result := make(SchemaConstraint)

	// Get constraints that reference this field path
	for id, constraint := range topLevelSchema.Constraints {
		// Check if any constraint field matches this path
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

func (graph *ValidationGraph) dfsCheck(nodeID string) bool {
	graph.visitedState[nodeID] = dfsVisiting

	for _, depID := range graph.dependencies[nodeID] {
		if graph.visitedState[depID] == dfsVisiting {
			return true // Cycle detected
		}
		if graph.visitedState[depID] == dfsUnvisited {
			if graph.dfsCheck(depID) {
				return true
			}
		}
	}

	graph.visitedState[nodeID] = dfsVisited
	return false
}

func (graph *ValidationGraph) buildFromSchema(
	schema *Schema,
	basePath string,
	addedConstraints map[string]bool,
	nsd *NestedSchema,
	schemaRefConstraints SchemaConstraint,
	topLevelSchema *Schema,
) ([]*baseNode, error) {
	var rootNodes []*baseNode

	// Determine fields to process
	fieldsToProcess := schema.Fields
	if nsd != nil && nsd.Fields != nil {
		fieldsToProcess = nsd.Fields
	}

	// Create expected fields map
	expectedFields := make(map[string]bool)
	for _, fieldDef := range fieldsToProcess {
		expectedFields[string(fieldDef.Name)] = true
	}

	// Create unexpected fields node
	unexpectedNode := graph.createUnexpectedFieldsNode(basePath, expectedFields)
	graph.addNode(unexpectedNode)
	rootNodes = append(rootNodes, &unexpectedNode.baseNode)

	// Process all fields
	var allFieldNodes []*baseNode
	for _, fieldDef := range fieldsToProcess {
		fieldNodes, err := graph.buildFieldNodes(&fieldDef, basePath, []string{unexpectedNode.id}, schema, addedConstraints, topLevelSchema)
		if err != nil {
			return nil, err
		}
		allFieldNodes = append(allFieldNodes, fieldNodes...)
	}

	// Collect constraints with override rules
	var nestedConstraints SchemaConstraint
	if nsd != nil {
		nestedConstraints = nsd.Constraints
	} else {
		nestedConstraints = schema.Constraints
	}

	// Get top-level constraints that apply to this path
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

	// Build constraint nodes
	constraintDeps := getDependencyIDs(allFieldNodes)
	schemaConstraintIDs := graph.buildFromEffectiveConstraints(effectiveConstraints, constraintDeps, addedConstraints)

	if len(schemaConstraintIDs) > 0 {
		completionNode := graph.createCompletionNode(basePath, "schema_completion", schemaConstraintIDs)
		graph.addNode(completionNode)
		rootNodes = append(rootNodes, &completionNode.baseNode)
	}

	return rootNodes, nil
}

// Unified field processing method
func (graph *ValidationGraph) buildFieldNodes(
	fieldDef *Field,
	basePath string,
	baseDeps []string,
	sc *Schema,
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
) ([]*baseNode, error) {
	fieldPath := buildPath(basePath, string(fieldDef.Name))
	currentDeps := baseDeps
	var nodes []*baseNode

	// Add required field check
	if fieldDef.Required {
		// We should also check that parent is required otherwise we will try to validate required fields on nil parents
		// So the parent of this field is a dependency
		reqNode := graph.createRequiredFieldNode(fieldPath, string(fieldDef.Name), currentDeps)
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
	typeSpecificNodes, err := graph.buildFieldTypeNodes(fieldDef, fieldPath, currentDeps, sc, addedConstraints, topLevelSchema)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, typeSpecificNodes...)
	if len(typeSpecificNodes) > 0 {
		currentDeps = []string{typeSpecificNodes[len(typeSpecificNodes)-1].id}
	}

	return nodes, nil
}

func (graph *ValidationGraph) buildFieldTypeNodes(
	fieldDef *Field,
	fieldPath string,
	currentDeps []string,
	sc *Schema,
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
) ([]*baseNode, error) {
	var node ValidationNode
	var nodes []*baseNode
	var err error

	switch fieldDef.Type {
	case FieldTypeEnum:
		node, err = graph.buildEnumNode(fieldDef, fieldPath, currentDeps, sc)

	case FieldTypeArray:
		node, err = graph.buildArrayNode(fieldDef, fieldPath, currentDeps, sc)

	case FieldTypeSet:
		node = &SetValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "set", ""), path: fieldPath, deps: currentDeps},
		}

	case FieldTypeRecord:
		node, err = graph.buildRecordNode(fieldDef, fieldPath, currentDeps, sc)

	case FieldTypeUnion:
		node, err = graph.buildUnionNode(fieldDef, fieldPath, currentDeps, sc)

	case FieldTypeComposite:
		node, err = graph.buildCompositeNode(fieldDef, fieldPath, currentDeps, sc)

	case FieldTypeGeometry:
		node = &GeometryValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "geometry", ""), path: fieldPath, deps: currentDeps},
		}

	case FieldTypeObject:
		objectNodes, err := graph.buildObjectFieldNodes(fieldDef, fieldPath, sc, addedConstraints, topLevelSchema)
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

func (graph *ValidationGraph) buildEnumNode(
	fieldDef *Field,
	fieldPath string,
	currentDeps []string,
	sc *Schema,
) (ValidationNode, error) {
	if fieldDef.Schema.IsZero() {
		return nil, ErrSchemaNotFound.WithMessage("Enum field must have schema reference").WithPath(fieldPath)
	}

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve enum schema: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	_, exists := sc.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Enum schema '%s' not found", schemaRef.ID)).WithPath(fieldPath)
	}

	return &EnumValidationNode{
		baseNode:  baseNode{id: buildNodeID(fieldPath, "enum", ""), path: fieldPath, deps: currentDeps},
		fieldDef:  fieldDef,
		schema:    sc,
		schemaRef: schemaRef,
	}, nil
}

func (graph *ValidationGraph) buildObjectFieldNodes(
	fieldDef *Field,
	fieldPath string,
	sc *Schema,
	addedConstraints map[string]bool,
	topLevelSchema *Schema,
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

	effectiveSchema := &Schema{BaseSchema: nestedSchema.BaseSchema}

	// Pass the original top-level schema for constraint resolution
	nestedNodes, err := graph.buildFromSchema(effectiveSchema, fieldPath, addedConstraints, &nestedSchema, schemaRef.Constraints, topLevelSchema)
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

func (graph *ValidationGraph) buildFromEffectiveConstraints(
	effectiveConstraints []EffectiveConstraint,
	deps []string,
	addedConstraints map[string]bool,
) []string {
	var ruleDepIDs []string

	for _, ec := range effectiveConstraints {
		rules := graph.buildFromConstraintRule(ec.Constraint, ec.BasePath, deps, addedConstraints)
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

// createSubGraph builds a standalone validation graph for a single field definition
func (graph *ValidationGraph) createSubGraph(
	rootFieldName string,
	rootFieldDef *Field,
	parentSchema *Schema,
	basePath string,
) (*ValidationGraph, error) {
	tempSchema := &Schema{
		BaseSchema: BaseSchema{
			Name:   fmt.Sprintf("subgraph_%s", basePath),
			Fields: map[FieldId]Field{FieldId(rootFieldName): *rootFieldDef},
		},
		Schemas: parentSchema.Schemas,
	}

	subGraph := newValidationGraph()
	addedConstraints := make(map[string]bool)

	if _, err := subGraph.buildFromSchema(tempSchema, "", addedConstraints, nil, nil, parentSchema); err != nil {
		return nil, err
	}

	return subGraph, nil
}

// buildArrayNode handles the specific logic for array validation construction
func (graph *ValidationGraph) buildArrayNode(
	fieldDef *Field,
	fieldPath string,
	deps []string,
	sc *Schema,
) (ValidationNode, error) {

	if fieldDef.Schema.IsZero() {
		return &ArrayValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "array", ""), path: fieldPath, deps: deps},
			fieldDef: fieldDef,
			schema:   sc,
			graph:    nil, // Allows untyped arrays?
		}, nil
	}

	var tempRootField *Field
	var arrayGraph *ValidationGraph
	var err error

	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference for array items: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedDef, exists := sc.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for array items", schemaRef.ID)).WithPath(fieldPath)
	}

	tempRootField = &Field{
		Name: "item",
		FieldProperties: FieldProperties{
			Type:    nestedDef.Type,
			Schema:  nestedDef.FieldProperties.Schema,
			Default: nestedDef.Default,
		},
	}

	arrayGraph, err = graph.createSubGraph("item", tempRootField, sc, fieldPath)
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
func (graph *ValidationGraph) buildRecordNode(
	fieldDef *Field,
	fieldPath string,
	deps []string,
	sc *Schema,
) (ValidationNode, error) {
	var recordGraph *ValidationGraph
	var err error

	// Untyped record - no validation needed (Rule 10)
	if fieldDef.Schema.IsZero() {
		return &RecordValidationNode{
			baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: deps},
			fieldDef: fieldDef,
			schema:   sc,
			graph:    nil, // No subgraph = untyped map[string]any
		}, nil
	}

	// Typed record - validate values
	schemaRef, err := FieldSchemaAs[SchemaReference](fieldDef.Schema)
	if err != nil {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Failed to resolve FieldSchemaReference for record items: %v", err)).WithPath(fieldPath).WithCause(err)
	}

	nestedDef, exists := sc.Schemas[schemaRef.ID]
	if !exists {
		return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for record items", schemaRef.ID)).WithPath(fieldPath)
	}

	tempRootField := &Field{
		Name: "item",
		FieldProperties: FieldProperties{
			Type:    nestedDef.Type,
			Schema:  nestedDef.FieldProperties.Schema,
			Default: nestedDef.Default,
		},
	}

	recordGraph, err = graph.createSubGraph("item", tempRootField, sc, fieldPath)
	if err != nil {
		return nil, err
	}

	return &RecordValidationNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "record", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graph:    recordGraph,
	}, nil
}

// buildUnionNode handles the specific logic for union validation construction
func (graph *ValidationGraph) buildUnionNode(
	fieldDef *Field,
	fieldPath string,
	deps []string,
	sc *Schema,
) (ValidationNode, error) {
	return nil, common.NewSystemError("NOT IMPLEMENTED")
}

func (graph *ValidationGraph) buildCompositeNode(
	fieldDef *Field,
	fieldPath string,
	deps []string,
	sc *Schema,
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
		nestedDef, exists := sc.Schemas[ref.ID]
		if !exists {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Nested schema '%s' not found for composite", ref.ID)).WithPath(fieldPath)
		}

		// Composite schemas must be in Schema mode (have Fields)
		if len(nestedDef.Fields) == 0 {
			return nil, ErrSchemaNotFound.WithMessage(fmt.Sprintf("Composite schema '%s' must be in Schema mode (have Fields defined)", ref.ID)).WithPath(fieldPath)
		}

		tempRootField := &Field{
			Name: "root",
			FieldProperties: FieldProperties{
				Type:   FieldTypeObject,
				Schema: NewSchemaReference(ref),
			},
		}

		compositeGraph, err := graph.createSubGraph("root", tempRootField, sc, fieldPath)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, compositeGraph)
	}

	return &CompositeValidationNode{
		baseNode: baseNode{id: buildNodeID(fieldPath, "composite", ""), path: fieldPath, deps: deps},
		fieldDef: fieldDef,
		schema:   sc,
		graphs:   graphs,
	}, nil
}

func (graph *ValidationGraph) buildFromConstraintRule(
	rule Constraint,
	basePath string,
	deps []string,
	addedConstraints map[string]bool,
) []string {
	var ruleDepIDs []string

	switch rule.Kind() {
	case ConstraintKindRule:
		r, err := ConstraintAs[*ConstraintRule](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		// Resolve field paths relative to basePath
		absoluteFieldPaths := resolveConstraintFieldPaths(basePath, r.Fields)

		// Create unique node ID using constraint name and base path
		nodeID := buildNodeID(basePath, "constraint", rule.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID}
		}

		node := &ConstraintNode{
			baseNode:       baseNode{id: nodeID, path: basePath, deps: deps},
			constraint:     rule,
			fieldPaths:     absoluteFieldPaths,
			constraintPath: basePath,
		}
		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)

	case ConstraintKindGroup:
		g, err := ConstraintAs[*ConstraintGroup](rule.ConstraintUnion)
		if err != nil {
			return nil
		}

		nodeID := buildNodeID(basePath, "constraint_group", rule.Name)
		if addedConstraints[nodeID] {
			return []string{nodeID}
		}

		node := &ConstraintGroupNode{
			baseNode: baseNode{id: nodeID, path: basePath, deps: deps},
			group:    *g,
			Name:     rule.Name,
		}

		graph.addNode(node)
		addedConstraints[nodeID] = true
		ruleDepIDs = append(ruleDepIDs, node.id)
	}

	return ruleDepIDs
}

func (graph *ValidationGraph) traverse(fmap PredicateMap, document map[string]any, mode ValidationMode, maxDepth int) ([]common.Issue, bool) {
	ctx := &ValidationContext{
		RootData:    document,
		Data:        document,
		Results:     make(map[string]*NodeResult),
		FunctionMap: fmap,
		MaxDepth:    maxDepth,
		Mode:        mode,
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
				canRun := true
				for _, depID := range graph.dependencies[nodeID] {
					if res, exists := ctx.Results[depID]; exists && !res.Success {
						ctx.Results[nodeID] = &NodeResult{Success: false, Skipped: true}
						canRun = false
						break
					}
				}

				if !canRun {
					visited[nodeID] = true
					progressMade = true
					continue
				}

				nodePath := node.GetPath()
				val, keyExists := utils.GetValueByPath(ctx.RootData, nodePath)
				_, isRequiredNode := node.(*RequiredFieldNode)
				if (!keyExists || val == nil) && !isRequiredNode && nodePath != "" {
					ctx.Results[nodeID] = &NodeResult{Success: true, Skipped: true}
					visited[nodeID] = true
					progressMade = true
					continue
				}

				result := node.Execute(ctx)
				ctx.Results[nodeID] = result
				if result != nil && !result.Skipped {
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

	// Filter issues based on validation mode
	filteredIssues := make([]common.Issue, 0, len(allIssues))
	for _, issue := range allIssues {
		switch mode {
		case ValidationModeStrict:
			filteredIssues = append(filteredIssues, issue)
		case ValidationModePartialStrict:
			if issue.Code != "REQUIRED_FIELD_MISSING" {
				filteredIssues = append(filteredIssues, issue)
			}
		case ValidationModeLoose:
			if issue.Code != "REQUIRED_FIELD_MISSING" && issue.Code != "UNEXPECTED_FIELD" {
				filteredIssues = append(filteredIssues, issue)
			}
		}
	}

	sort.Slice(filteredIssues, func(i, j int) bool {
		if filteredIssues[i].Path != filteredIssues[j].Path {
			return filteredIssues[i].Path < filteredIssues[j].Path
		}
		return filteredIssues[i].Code < filteredIssues[j].Code
	})

	return filteredIssues, len(filteredIssues) == 0
}

// =============================================================================
// MAIN VALIDATOR CONSTRUCTION
// =============================================================================

func NewDocumentValidator(sc *Schema, fmap PredicateMap) (*DocumentValidator, error) {
	return NewDocumentValidatorWithConfig(sc, fmap, DefaultValidationConfig())
}

func NewDocumentValidatorWithConfig(sc *Schema, fmap PredicateMap, config ValidationConfig) (*DocumentValidator, error) {
	graph := newValidationGraph()
	addedConstraints := make(map[string]bool)
	if _, err := graph.buildFromSchema(sc, "", addedConstraints, nil, nil, sc); err != nil {
		return nil, err
	}

	// Perform cycle detection after graph construction
	for nodeID := range graph.nodes {
		if graph.visitedState[nodeID] == dfsUnvisited {
			if graph.dfsCheck(nodeID) {
				return nil, ErrValidationCircularDependency.
					WithOperation("NewDocumentValidator").
					WithMessage(fmt.Sprintf("circular dependency detected in validation graph involving node: %s", nodeID))
			}
		}
	}

	return &DocumentValidator{
		graph:  graph,
		fmap:   fmap,
		config: config,
	}, nil
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

func (n *RequiredFieldNode) Execute(ctx *ValidationContext) *NodeResult {
	parentPath := getScopedPath(n.path)
	parentData, exists := utils.GetValueByPath(ctx.RootData, parentPath)
	if !exists {
		if parentPath != "" {
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

func (n *TypeCheckNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	if value == nil {
		switch n.fieldDef.Type {
		case FieldTypeObject, FieldTypeArray, FieldTypeSet, FieldTypeRecord, FieldTypeUnion, FieldTypeComposite:
			return &NodeResult{Success: true}
		default:
			return &NodeResult{Success: false, Issues: []common.Issue{
				{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Expected %s, got nil", n.fieldDef.Type), Path: n.path},
			}, Value: value}
		}
	}

	actualValue := reflect.ValueOf(value)
	if actualValue.Kind() == reflect.Invalid {
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Expected %s, got invalid", n.fieldDef.Type), Path: n.path},
		}, Value: value}
	}

	var isValid bool
	actualType := actualValue.Kind().String()

	switch n.fieldDef.Type {
	case FieldTypeString:
		isValid = actualValue.Kind() == reflect.String
	case FieldTypeNumber:
		isValid = actualValue.Kind() == reflect.Int || actualValue.Kind() == reflect.Int8 || actualValue.Kind() == reflect.Int16 || actualValue.Kind() == reflect.Int32 || actualValue.Kind() == reflect.Int64 ||
			actualValue.Kind() == reflect.Uint || actualValue.Kind() == reflect.Uint8 || actualValue.Kind() == reflect.Uint16 || actualValue.Kind() == reflect.Uint32 || actualValue.Kind() == reflect.Uint64 ||
			actualValue.Kind() == reflect.Float32 || actualValue.Kind() == reflect.Float64
	case FieldTypeInteger:
		isValid = actualValue.Kind() == reflect.Int || actualValue.Kind() == reflect.Int8 || actualValue.Kind() == reflect.Int16 || actualValue.Kind() == reflect.Int32 || actualValue.Kind() == reflect.Int64 ||
			actualValue.Kind() == reflect.Uint || actualValue.Kind() == reflect.Uint8 || actualValue.Kind() == reflect.Uint16 || actualValue.Kind() == reflect.Uint32 || actualValue.Kind() == reflect.Uint64
	case FieldTypeDecimal:
		isValid = actualValue.Kind() == reflect.Float32 || actualValue.Kind() == reflect.Float64
	case FieldTypeBoolean:
		isValid = actualValue.Kind() == reflect.Bool
	case FieldTypeObject, FieldTypeRecord, FieldTypeComposite:
		isValid = actualValue.Kind() == reflect.Map
	case FieldTypeArray, FieldTypeSet, FieldTypeGeometry:
		isValid = actualValue.Kind() == reflect.Slice
	case FieldTypeEnum:
		isValid = actualValue.Kind() == reflect.String || actualValue.Kind() == reflect.Int || actualValue.Kind() == reflect.Int64
	case FieldTypeUnion:
		isValid = true
	default:
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "UNKNOWN_FIELD_TYPE", Message: fmt.Sprintf("Unknown FieldType '%s'", n.fieldDef.Type), Path: n.path},
		}, Value: value}
	}

	if !isValid {
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Expected %s, got %s", n.fieldDef.Type, actualType), Path: n.path},
		}, Value: value}
	}
	return &NodeResult{Success: true, Value: value}
}

func (n *EnumValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	nestedSchema, exists := n.schema.Schemas[n.schemaRef.ID]
	if !exists {
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "SCHEMA_ERROR", Message: fmt.Sprintf("Enum schema '%s' not found", n.schemaRef.ID), Path: n.path},
		}}
	}

	if len(nestedSchema.Values) == 0 {
		return &NodeResult{Success: false, Issues: []common.Issue{
			{Code: "SCHEMA_ERROR", Message: "Enum schema has no values defined", Path: n.path},
		}}
	}

	for _, allowedValue := range nestedSchema.Values {
		if allowedValue.IsZero() || allowedValue.IsNull() {
			continue
		}
		if value == allowedValue.Value() {
			return &NodeResult{Success: true}
		}
	}

	return &NodeResult{
		Success: false,
		Issues: []common.Issue{{
			Code:    "ENUM_VIOLATION",
			Message: fmt.Sprintf("Value must be one of %v, found %v", nestedSchema.Values, value),
			Path:    n.path,
		}},
	}
}

func (n *ArrayValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected array", Path: n.path}}}
	}

	// Check max depth
	currentDepth := getPathDepth(n.path)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	if n.graph == nil {
		return &NodeResult{Success: true}
	}

	var allIssues []common.Issue

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)

		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ValidationModeStrict, ctx.MaxDepth)

		// Rewrite paths from subgraph context to parent context
		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "item") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
		}
		allIssues = append(allIssues, itemIssues...)
	}

	return &NodeResult{Success: len(allIssues) == 0, Issues: allIssues}
}

func (n *RecordValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	recordMap, ok := utils.GetMapStringAny(value)
	if !ok {
		return &NodeResult{Success: false, Issues: []common.Issue{{Code: "TYPE_MISMATCH", Message: "Expected object for record", Path: n.path}}}
	}

	// Check max depth
	currentDepth := getPathDepth(n.path)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	if n.graph == nil {
		return &NodeResult{Success: true}
	}

	var allIssues []common.Issue
	for key, item := range recordMap {
		itemPath := buildPath(n.path, key)

		itemIssues, _ := n.graph.traverse(ctx.FunctionMap, map[string]any{"item": item}, ValidationModeStrict, ctx.MaxDepth)

		// Rewrite paths from subgraph context to parent context
		for j := range itemIssues {
			if strings.HasPrefix(itemIssues[j].Path, "item") {
				itemIssues[j].Path = strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
			}
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
	return &NodeResult{Success: false, Issues: []common.Issue{{
		Code: "NOT_IMPLEMENTE",
		Path: n.path,
	}}}
}

func (n *CompositeValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	// Check max depth
	currentDepth := getPathDepth(n.path)
	if currentDepth >= ctx.MaxDepth {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Maximum nesting depth of %d exceeded", ctx.MaxDepth),
			Path:    n.path,
		}}}
	}

	// Value must match ALL schemas (logical AND)
	var allIssues []common.Issue

	for _, graph := range n.graphs {
		itemIssues, matched := graph.traverse(ctx.FunctionMap, map[string]any{"root": value}, ValidationModeStrict, ctx.MaxDepth)

		// Rewrite paths from subgraph context to parent context
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

	return &NodeResult{Success: true}
}

func (n *GeometryValidationNode) Execute(ctx *ValidationContext) *NodeResult {
	value, exists := utils.GetValueByPath(ctx.RootData, n.path)
	if !exists {
		return &NodeResult{Success: true}
	}

	outerVal := reflect.ValueOf(value)
	if outerVal.Kind() != reflect.Slice {
		return &NodeResult{Success: false, Issues: []common.Issue{{
			Code:    "TYPE_MISMATCH",
			Message: "Geometry must be an array",
			Path:    n.path,
		}}}
	}

	for i := 0; i < outerVal.Len(); i++ {
		innerVal := outerVal.Index(i)
		if innerVal.Kind() != reflect.Slice {
			return &NodeResult{Success: false, Issues: []common.Issue{{
				Code:    "TYPE_MISMATCH",
				Message: fmt.Sprintf("Geometry inner element at index %d must be an array", i),
				Path:    fmt.Sprintf("%s[%d]", n.path, i),
			}}}
		}

		// Check all elements are numbers
		inner := innerVal.Interface()
		innerSlice := reflect.ValueOf(inner)
		for j := 0; j < innerSlice.Len(); j++ {
			elem := innerSlice.Index(j)
			kind := elem.Kind()
			isNumber := kind == reflect.Int || kind == reflect.Int8 || kind == reflect.Int16 ||
				kind == reflect.Int32 || kind == reflect.Int64 || kind == reflect.Uint ||
				kind == reflect.Uint8 || kind == reflect.Uint16 || kind == reflect.Uint32 ||
				kind == reflect.Uint64 || kind == reflect.Float32 || kind == reflect.Float64

			if !isNumber {
				return &NodeResult{Success: false, Issues: []common.Issue{{
					Code:    "TYPE_MISMATCH",
					Message: fmt.Sprintf("Geometry element at [%d][%d] must be a number, got %s", i, j, kind),
					Path:    fmt.Sprintf("%s[%d][%d]", n.path, i, j),
				}}}
			}
		}
	}

	return &NodeResult{Success: true}
}

func (n *ConstraintNode) Execute(ctx *ValidationContext) *NodeResult {
	r, _ := ConstraintAs[*ConstraintRule](n.constraint.ConstraintUnion)

	// Requirement Check
	for _, path := range n.fieldPaths {
		if _, exists := utils.GetValueByPath(ctx.RootData, path); !exists {
			return &NodeResult{Success: true, Skipped: true}
		}
	}

	return runConstraintPredicate(ctx, r, n.fieldPaths, n.constraintPath)
}

func (n *ConstraintGroupNode) Execute(ctx *ValidationContext) *NodeResult {
	var results []bool
	var memberIssues []common.Issue
	allSkipped := true

	for _, ruleUnion := range n.group.Rules {
		r, _ := ConstraintAs[*ConstraintRule](ruleUnion)
		absPaths := resolveConstraintFieldPaths(n.path, r.Fields)

		// Loose Mode Skip Logic
		allFieldsExist := true
		for _, p := range absPaths {
			if _, exists := utils.GetValueByPath(ctx.RootData, p); !exists {
				allFieldsExist = false
				break
			}
		}

		if !allFieldsExist && ctx.Mode == ValidationModeLoose {
			continue
		}

		allSkipped = false
		res := runConstraintPredicate(ctx, r, absPaths, n.path)

		results = append(results, res.Success)
		if !res.Success {
			memberIssues = append(memberIssues, res.Issues...)
		}
	}

	if allSkipped {
		return &NodeResult{Success: true, Skipped: true}
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

	return &NodeResult{Success: true}
}

func runConstraintPredicate(
	ctx *ValidationContext,
	r *ConstraintRule,
	absFieldPaths []string,
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

	// 1. Extract Data
	fieldData := make(map[string]any)
	for i, fPath := range absFieldPaths {
		val, _ := utils.GetValueByPath(ctx.RootData, fPath)
		if i < len(r.Fields) {
			fieldData[string(r.Fields[i])] = val
		}
	}

	// 2. Pack Predicate Data
	var predicateData any
	if len(fieldData) == 1 {
		for _, v := range fieldData {
			predicateData = v
			break
		}
	} else {
		predicateData = fieldData
	}

	// 3. Execute
	issues := predicateFunc(PredicateParams{
		Data:       predicateData,
		Keys:       r.Fields,
		Parameters: r.Parameters,
	})

	// 4. Map Issue Paths
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

	return &NodeResult{Success: true}
}

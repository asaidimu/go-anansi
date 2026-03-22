// Package validator provides a compiled, graph-based validator for Document
// instances against a compiled ir.Schema.
//
// Build once with New — the schema is walked exactly once to construct the
// execution graph. Call Validate/ValidatePartial/ValidateLoose many times.
// The schema is never read again after construction.
//
// Storage model:
//   - TypeObject, TypeComposite: fields are flattened into the parent Document.
//     No nested *Document. Fields are addressed via full dot-paths from root.
//   - TypeArray / TypeSet: []*Document (object elements) or typed scalar slice.
//     Each *Document element is validated against a promoted sub-schema.
//   - TypeRecord (with schema): map[string]*Document. Each value is validated
//     against a promoted sub-schema.
//   - TypeRecord (without schema): map[string]any — no element validation.
//   - TypeUnion: variant determines storage. Object variants → *Document
//     validated against the promoted variant schema. Scalar variants → stored
//     directly in the parent document.
package validate

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// =============================================================================
// VALIDATION MODE
// =============================================================================

type ValidationMode byte

const (
	ValidationModeStrict        ValidationMode = iota + 1
	ValidationModePartialStrict
	ValidationModeLoose
)

// =============================================================================
// VALIDATION CONTEXT  (pooled per graph)
// =============================================================================

type validationContext struct {
	doc    *document.Document
	mode   ValidationMode
	issues []common.Issue
	failed []bool
}

func (c *validationContext) reset(doc *document.Document, mode ValidationMode, nodeCount int) {
	c.doc = doc
	c.mode = mode
	c.issues = c.issues[:0]
	if cap(c.failed) < nodeCount {
		c.failed = make([]bool, nodeCount)
	} else {
		c.failed = c.failed[:nodeCount]
		for i := range c.failed {
			c.failed[i] = false
		}
	}
}

// =============================================================================
// NODE INTERFACE
// =============================================================================

type node interface {
	execute(ctx *validationContext) bool
	nodeID() int
	nodeDeps() []int
}

type baseNode struct {
	nid   int
	ndeps []int
}

func (b *baseNode) nodeID() int     { return b.nid }
func (b *baseNode) nodeDeps() []int { return b.ndeps }

// =============================================================================
// VALIDATION GRAPH
// =============================================================================

type validationGraph struct {
	nodes     []node
	order     []int
	deps      [][]int
	ctxPool   sync.Pool
	nodeCount int
}

func newValidationGraph() *validationGraph {
	return &validationGraph{}
}

func (g *validationGraph) addNode(n node) {
	nid := n.nodeID()
	for nid >= len(g.nodes) {
		g.nodes = append(g.nodes, nil)
		g.deps = append(g.deps, nil)
	}
	g.nodes[nid] = n
	g.deps[nid] = n.nodeDeps()
}

func (g *validationGraph) finalize() error {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make([]int, len(g.nodes))
	order := make([]int, 0, len(g.nodes))
	hasCycle := false

	var dfs func(id int)
	dfs = func(id int) {
		if hasCycle || g.nodes[id] == nil {
			return
		}
		if state[id] == visiting {
			hasCycle = true
			return
		}
		if state[id] == done {
			return
		}
		state[id] = visiting
		for _, dep := range g.deps[id] {
			dfs(dep)
		}
		state[id] = done
		order = append(order, id)
	}

	for id := range g.nodes {
		if g.nodes[id] != nil && state[id] == unvisited {
			dfs(id)
			if hasCycle {
				return fmt.Errorf("validator: circular dependency at node %d", id)
			}
		}
	}

	g.order = order
	g.nodeCount = len(g.nodes)
	g.ctxPool.New = func() any {
		return &validationContext{
			issues: make([]common.Issue, 0, 8),
			failed: make([]bool, g.nodeCount),
		}
	}
	return nil
}

func (g *validationGraph) traverse(doc *document.Document, mode ValidationMode) ([]common.Issue, bool) {
	ctx := g.ctxPool.Get().(*validationContext)
	defer g.ctxPool.Put(ctx)
	ctx.reset(doc, mode, g.nodeCount)

	for _, nid := range g.order {
		n := g.nodes[nid]
		if n == nil {
			continue
		}
		skip := false
		for _, dep := range g.deps[nid] {
			if ctx.failed[dep] {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		ok := n.execute(ctx)
		ctx.failed[nid] = !ok
	}

	issues := make([]common.Issue, len(ctx.issues))
	copy(issues, ctx.issues)
	return issues, len(issues) == 0
}

// =============================================================================
// NODE ID ALLOCATOR
// =============================================================================

type idGen struct{ next int }

func (g *idGen) alloc() int {
	id := g.next
	g.next++
	return id
}

// =============================================================================
// NODE IMPLEMENTATIONS
// =============================================================================

// unexpectedFieldsNode walks the document's positions map and reports any key
// not in the expected set built during graph construction.
type unexpectedFieldsNode struct {
	baseNode
	expected map[document.DocumentKey]struct{}
	path     string
}

func (n *unexpectedFieldsNode) execute(ctx *validationContext) bool {
	if ctx.mode == ValidationModeLoose {
		return true
	}
	ok := true
	ctx.doc.Walk(func(positions map[int64]int32, _ func(document.DataType, ...int) unsafe.Pointer) (any, error) {
		for rawKey := range positions {
			dk := document.DocumentKey(rawKey)
			if _, expected := n.expected[dk]; !expected {
				fd := dk.Descriptor()
				name := fmt.Sprintf("field[%d]", ir.ExtractFieldIndex(fd))
				path := joinPath(n.path, name)
				ctx.issues = append(ctx.issues, common.Issue{
					Code:    "UNEXPECTED_FIELD",
					Message: fmt.Sprintf("Unexpected field '%s'", path),
					Path:    path,
				})
				ok = false
			}
		}
		return nil, nil
	})
	return ok
}

// requiredFieldNode checks that a field is present and holds a concrete value.
type requiredFieldNode struct {
	baseNode
	key  document.DocumentKey
	path string
}

func (n *requiredFieldNode) execute(ctx *validationContext) bool {
	if ctx.mode != ValidationModeStrict {
		return true
	}
	if ctx.doc.HasValue(n.key) {
		return true
	}
	ctx.issues = append(ctx.issues, common.Issue{
		Code:    "REQUIRED_FIELD_MISSING",
		Message: fmt.Sprintf("Required field '%s' is missing", n.path),
		Path:    n.path,
	})
	return false
}

// enumValidationNode checks a field's value against a pre-built allowed set
// extracted from cs.Store at graph construction time.
type enumValidationNode struct {
	baseNode
	key           document.DocumentKey
	path          string
	allowedInt    map[int64]struct{}
	allowedString map[string]struct{}
	allowedFloat  map[float64]struct{}
}

func (n *enumValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	switch n.key.Type() {
	case document.TypeInt:
		v, _, _ := ctx.doc.GetInt(n.key)
		if _, ok := n.allowedInt[v]; ok {
			return true
		}
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "ENUM_VIOLATION",
			Message: fmt.Sprintf("Value %d is not in the allowed enum set at '%s'", v, n.path),
			Path:    n.path,
		})
		return false
	case document.TypeString:
		v, _, _ := ctx.doc.GetString(n.key)
		if _, ok := n.allowedString[v]; ok {
			return true
		}
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "ENUM_VIOLATION",
			Message: fmt.Sprintf("Value %q is not in the allowed enum set at '%s'", v, n.path),
			Path:    n.path,
		})
		return false
	case document.TypeFloat:
		v, _, _ := ctx.doc.GetFloat(n.key)
		if _, ok := n.allowedFloat[v]; ok {
			return true
		}
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "ENUM_VIOLATION",
			Message: fmt.Sprintf("Value %v is not in the allowed enum set at '%s'", v, n.path),
			Path:    n.path,
		})
		return false
	}
	return true
}

// arrayValidationNode iterates a TypeArrayObject field and validates each
// element *Document against a sub-graph built from the promoted element schema.
type arrayValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	subGraph *validationGraph
}

func (n *arrayValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) || n.subGraph == nil {
		return true
	}
	items, _, _ := ctx.doc.GetArrayObject(n.key)
	ok := true
	for i, item := range items {
		if item == nil {
			continue
		}
		itemPath := fmt.Sprintf("%s[%d]", n.path, i)
		issues, itemOk := n.subGraph.traverse(item, ctx.mode)
		for j := range issues {
			issues[j].Path = joinPath(itemPath, issues[j].Path)
		}
		ctx.issues = append(ctx.issues, issues...)
		if !itemOk {
			ok = false
		}
	}
	return ok
}

// recordValidationNode iterates a TypeRecord field (map[string]*Document) and
// validates each value *Document against a sub-graph built from the promoted
// value schema.
type recordValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	subGraph *validationGraph
}

func (n *recordValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) || n.subGraph == nil {
		return true
	}
	recordMap, _, _ := ctx.doc.GetRecord(n.key)
	ok := true
	for recordKey, rawItem := range recordMap {
		item, isDoc := rawItem.(*document.Document)
		if !isDoc || item == nil {
			continue
		}
		itemPath := joinPath(n.path, recordKey)
		issues, itemOk := n.subGraph.traverse(item, ctx.mode)
		for j := range issues {
			issues[j].Path = joinPath(itemPath, issues[j].Path)
		}
		ctx.issues = append(ctx.issues, issues...)
		if !itemOk {
			ok = false
		}
	}
	return ok
}

// unionVariant pairs a sub-graph (nil for scalar variants) with the document
// DataType used to store the union value.
type unionVariant struct {
	subGraph *validationGraph
	dataType document.DataType
}

// unionValidationNode validates a field against one or more variant sub-graphs.
// The value must match at least one variant.
type unionValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	variants []unionVariant
}

func (n *unionValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	for _, v := range n.variants {
		if v.subGraph == nil {
			// Scalar variant: type match is sufficient.
			if n.key.Type() == v.dataType {
				return true
			}
			continue
		}
		// Object variant: stored as *document.Document under TypeRecord.
		recordMap, ok, _ := ctx.doc.GetRecord(n.key)
		if !ok || len(recordMap) == 0 {
			continue
		}
		rawItem := recordMap[""]
		item, isDoc := rawItem.(*document.Document)
		if !isDoc || item == nil {
			continue
		}
		if _, ok := v.subGraph.traverse(item, ctx.mode); ok {
			return true
		}
	}
	ctx.issues = append(ctx.issues, common.Issue{
		Code:    "UNION_MISMATCH",
		Message: fmt.Sprintf("Value at '%s' does not match any union variant", n.path),
		Path:    n.path,
	})
	return false
}

// recursionMarkerNode handles a recursive TypeObject back-edge.
// subGraph is an indirect pointer that is filled in after the owning graph is
// finalized, allowing the marker to reference the graph it lives in without
// causing infinite recursion during construction.
type recursionMarkerNode struct {
	baseNode
	key        document.DocumentKey
	path       string
	subGraph   **validationGraph // filled after finalize
	schemaName string
	maxDepth   int
	depth      int
}

func (n *recursionMarkerNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	if n.depth >= n.maxDepth {
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("Recursive schema '%s' exceeds maximum depth of %d", n.schemaName, n.maxDepth),
			Path:    n.path,
		})
		return false
	}
	recordMap, ok, _ := ctx.doc.GetRecord(n.key)
	if !ok || len(recordMap) == 0 {
		return true
	}
	rawItem := recordMap[""]
	item, isDoc := rawItem.(*document.Document)
	if !isDoc || item == nil {
		return true
	}
	g := *n.subGraph
	if g == nil {
		return true
	}
	issues, ok := g.traverse(item, ctx.mode)
	for i := range issues {
		issues[i].Path = joinPath(n.path, issues[i].Path)
	}
	ctx.issues = append(ctx.issues, issues...)
	return ok
}

// constraintNode evaluates a single resolved predicate.
type constraintNode struct {
	baseNode
	constraint ir.ResolvedConstraint
	path       string
}

func (n *constraintNode) execute(ctx *validationContext) bool {
	var presentCount, missingCount int
	for _, key := range n.constraint.Fields {
		if ctx.doc.HasValue(key) {
			presentCount++
		} else {
			missingCount++
		}
	}

	if missingCount == 0 {
		if n.constraint.Predicate(ctx.doc, n.constraint.Fields, n.constraint.Parameters) {
			return true
		}
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "CONSTRAINT_VIOLATION",
			Message: fmt.Sprintf("Constraint '%s' failed", n.constraint.Name),
			Path:    n.path,
		})
		return false
	}

	if presentCount == 0 {
		switch ctx.mode {
		case ValidationModeStrict:
			ctx.issues = append(ctx.issues, common.Issue{
				Code:    "CONSTRAINT_INCOMPLETE",
				Message: fmt.Sprintf("Constraint '%s' cannot be evaluated: all required fields are missing", n.constraint.Name),
				Path:    n.path,
			})
			return false
		default:
			return true
		}
	}

	switch ctx.mode {
	case ValidationModeStrict:
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "CONSTRAINT_INCOMPLETE",
			Message: fmt.Sprintf("Constraint '%s' cannot be evaluated: some required fields are missing", n.constraint.Name),
			Path:    n.path,
		})
		return false
	case ValidationModePartialStrict:
		ctx.issues = append(ctx.issues, common.Issue{
			Code:    "CONSTRAINT_PARTIAL_UPDATE",
			Message: fmt.Sprintf("Constraint '%s' couples fields that must be updated together", n.constraint.Name),
			Path:    n.path,
		})
		return false
	default:
		return true
	}
}

// constraintGroupNode folds member results through a LogicalOperator.
type constraintGroupNode struct {
	baseNode
	group ir.ResolvedConstraintGroup
	path  string
}

func (n *constraintGroupNode) execute(ctx *validationContext) bool {
	results := make([]bool, 0, len(n.group.Constraints))
	issuesBefore := len(ctx.issues)

	for _, member := range n.group.Constraints {
		var ok bool
		switch m := member.(type) {
		case ir.ResolvedConstraint:
			cn := &constraintNode{constraint: m, path: n.path}
			ok = cn.execute(ctx)
		case ir.ResolvedConstraintGroup:
			sub := &constraintGroupNode{group: m, path: n.path}
			ok = sub.execute(ctx)
		}
		results = append(results, ok)
	}

	if evaluateLogicalOperator(n.group.Operator, results) {
		ctx.issues = ctx.issues[:issuesBefore]
		return true
	}

	memberIssues := make([]common.Issue, len(ctx.issues)-issuesBefore)
	copy(memberIssues, ctx.issues[issuesBefore:])
	ctx.issues = ctx.issues[:issuesBefore]
	ctx.issues = append(ctx.issues, common.Issue{
		Code:    "CONSTRAINT_GROUP_VIOLATION",
		Message: fmt.Sprintf("Constraint group '%s' failed", n.group.Name),
		Path:    n.path,
	})
	ctx.issues = append(ctx.issues, memberIssues...)
	return false
}

func evaluateLogicalOperator(op ir.LogicalOperator, results []bool) bool {
	if len(results) == 0 {
		return true
	}
	switch op {
	case ir.LogicalAnd:
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	case ir.LogicalOr:
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	case ir.LogicalNot:
		return len(results) == 1 && !results[0]
	case ir.LogicalNor:
		for _, r := range results {
			if r {
				return false
			}
		}
		return true
	case ir.LogicalXor:
		count := 0
		for _, r := range results {
			if r {
				count++
			}
		}
		return count == 1
	case ir.LogicalNand:
		for _, r := range results {
			if !r {
				return true
			}
		}
		return false
	case ir.LogicalXnor:
		count := 0
		for _, r := range results {
			if r {
				count++
			}
		}
		return count == 0 || count == len(results)
	}
	return false
}

// =============================================================================
// GRAPH BUILDER
// =============================================================================

// buildContext tracks which original schema indices are currently being built
// to detect recursive back-edges. Keyed by original (pre-promotion) index.
//
// graphPtrs holds **validationGraph slots for recursive schemas. When a
// recursive back-edge is encountered, a marker node is given a pointer to the
// slot. After the enclosing buildGraph call completes, the caller writes the
// finished graph into the slot so all markers resolve correctly.
type buildContext struct {
	building  map[uint8]int
	graphPtrs map[uint8]**validationGraph
}

func newBuildContext() *buildContext {
	return &buildContext{
		building:  make(map[uint8]int),
		graphPtrs: make(map[uint8]**validationGraph),
	}
}

func (bc *buildContext) isRecursive(idx uint8) bool { return bc.building[idx] > 0 }
func (bc *buildContext) push(idx uint8)              { bc.building[idx]++ }
func (bc *buildContext) pop(idx uint8) {
	bc.building[idx]--
	if bc.building[idx] <= 0 {
		delete(bc.building, idx)
	}
}

// buildGraph walks cs.Descriptors once for schemaIdx and constructs the graph.
// cs is always the root schema. basePath accumulates the dot-path prefix for
// flattened object/composite fields. For array, record, and union fields,
// sub-schemas are promoted and their graphs are built independently.
func buildGraph(
	cs *ir.Schema,
	schemaIdx uint8,
	basePath string,
	gen *idGen,
	bc *buildContext,
	maxDepth int,
	depth int,
) (*validationGraph, error) {
	g := newValidationGraph()

	// Build expected-key set and populate PathCache in one pass.
	expectedKeys := make(map[document.DocumentKey]struct{})
	if err := collectExpectedKeys(cs, schemaIdx, basePath, expectedKeys); err != nil {
		return nil, err
	}

	unexpID := gen.alloc()
	g.addNode(&unexpectedFieldsNode{
		baseNode: baseNode{nid: unexpID},
		expected: expectedKeys,
		path:     basePath,
	})

	fieldNodeIDs, err := buildFieldNodes(cs, schemaIdx, basePath, []int{unexpID}, gen, bc, maxDepth, depth, g)
	if err != nil {
		return nil, err
	}

	if cs.ResolvedConstraints != nil {
		constraintDeps := make([]int, 0, 1+len(fieldNodeIDs))
		constraintDeps = append(constraintDeps, unexpID)
		constraintDeps = append(constraintDeps, fieldNodeIDs...)
		for _, root := range cs.ResolvedConstraints.Roots {
			if _, err := buildConstraintNode(root, basePath, constraintDeps, gen, g); err != nil {
				return nil, err
			}
		}
	}

	if err := g.finalize(); err != nil {
		return nil, err
	}

	return g, nil
}

// collectExpectedKeys populates expected with every DocumentKey valid at
// schemaIdx and all its flattened (object/composite) sub-schemas.
// Calling cs.DocumentKey populates PathCache as a side effect.
func collectExpectedKeys(
	cs *ir.Schema,
	schemaIdx uint8,
	basePath string,
	expected map[document.DocumentKey]struct{},
) error {
	start, end := schemaOffsetRange(cs, schemaIdx)
	for _, fd := range cs.Descriptors[start:end] {
		path := resolveFieldPath(cs, fd, basePath)
		key, err := cs.DocumentKey(path)
		if err != nil {
			return fmt.Errorf("validator: cannot resolve key for path %q: %w", path, err)
		}
		expected[key] = struct{}{}

		if !ir.IsTerminal(fd) {
			continue
		}
		typ := ir.ExtractType(fd)
		switch typ {
		case ir.TypeObject:
			if err := collectExpectedKeys(cs, ir.ExtractTargetSchema(fd), path, expected); err != nil {
				return err
			}
		case ir.TypeComposite:
			for _, variantIdx := range cs.Variants[fd] {
				if err := collectExpectedKeys(cs, variantIdx, path, expected); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// buildFieldNodes emits nodes for all fields of schemaIdx into g, flattening
// object and composite sub-schemas recursively. Returns all emitted node IDs.
func buildFieldNodes(
	cs *ir.Schema,
	schemaIdx uint8,
	basePath string,
	parentDeps []int,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
	g *validationGraph,
) ([]int, error) {
	start, end := schemaOffsetRange(cs, schemaIdx)
	var nodeIDs []int

	for _, fd := range cs.Descriptors[start:end] {
		path := resolveFieldPath(cs, fd, basePath)
		key, err := cs.DocumentKey(path)
		if err != nil {
			return nil, fmt.Errorf("validator: cannot resolve key for path %q: %w", path, err)
		}

		fieldDeps := parentDeps

		if ir.IsRequired(fd) {
			reqID := gen.alloc()
			g.addNode(&requiredFieldNode{
				baseNode: baseNode{nid: reqID, ndeps: fieldDeps},
				key:      key,
				path:     path,
			})
			fieldDeps = []int{reqID}
			nodeIDs = append(nodeIDs, reqID)
		}

		typ := ir.ExtractType(fd)

		switch typ {
		case ir.TypeEnum:
			enumID := gen.alloc()
			enumNode, err := buildEnumNode(cs, fd, key, path, fieldDeps, enumID)
			if err != nil {
				return nil, err
			}
			g.addNode(enumNode)
			nodeIDs = append(nodeIDs, enumID)

		case ir.TypeObject:
			targetIdx := ir.ExtractTargetSchema(fd)
			if bc.isRecursive(targetIdx) {
				nid, err := buildRecursiveObjectNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
				if err != nil {
					return nil, err
				}
				nodeIDs = append(nodeIDs, nid)
			} else {
				bc.push(targetIdx)
				subIDs, err := buildFieldNodes(cs, targetIdx, path, fieldDeps, gen, bc, maxDepth, depth, g)
				bc.pop(targetIdx)
				if err != nil {
					return nil, err
				}
				nodeIDs = append(nodeIDs, subIDs...)
			}

		case ir.TypeComposite:
			for _, variantIdx := range cs.Variants[fd] {
				if bc.isRecursive(variantIdx) {
					nid, err := buildRecursiveObjectNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
					if err != nil {
						return nil, err
					}
					nodeIDs = append(nodeIDs, nid)
					break
				}
				bc.push(variantIdx)
				subIDs, err := buildFieldNodes(cs, variantIdx, path, fieldDeps, gen, bc, maxDepth, depth, g)
				bc.pop(variantIdx)
				if err != nil {
					return nil, err
				}
				nodeIDs = append(nodeIDs, subIDs...)
			}

		case ir.TypeArray, ir.TypeSet:
			nid, err := buildArrayNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			nodeIDs = append(nodeIDs, nid)

		case ir.TypeRecord:
			nid, err := buildRecordNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			nodeIDs = append(nodeIDs, nid)

		case ir.TypeUnion:
			nid, err := buildUnionNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			nodeIDs = append(nodeIDs, nid)
		}
	}

	return nodeIDs, nil
}

func buildRecursiveObjectNode(
	cs *ir.Schema,
	fd uint32,
	key document.DocumentKey,
	path string,
	deps []int,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
	g *validationGraph,
) (int, error) {
	targetIdx := ir.ExtractTargetSchema(fd)

	// Every recursive back-edge for the same targetIdx shares one **validationGraph
	// slot. All recursionMarkerNodes for the same targetIdx share the same slot.
	// The slot is filled once the promoted graph is built.
	ptr, alreadyBuilding := bc.graphPtrs[targetIdx]
	if !alreadyBuilding {
		ptr = new(*validationGraph)
		bc.graphPtrs[targetIdx] = ptr

		// Build the promoted graph now. We use a fresh buildContext so that
		// recursive fields within the promoted schema (which reference back to
		// promoted index 0) reuse the same ptr slot via the promoted bc,
		// not the parent bc.
		promoted, err := cs.Promote(targetIdx)
		if err != nil {
			return 0, fmt.Errorf("validator: cannot promote recursive schema %d: %w", targetIdx, err)
		}
		subGen := &idGen{}
		subBc := newBuildContext()
		// Register the slot in subBc under index 0 (the promoted root index),
		// so that when the promoted graph encounters its own recursive field
		// it finds the slot and doesn't recurse again.
		subBc.graphPtrs[0] = ptr
		subBc.push(0)

		promoted_g, err := buildGraph(promoted, 0, "", subGen, subBc, maxDepth, depth+1)
		subBc.pop(0)
		if err != nil {
			return 0, err
		}
		// Fill the slot — all marker nodes now resolve.
		*ptr = promoted_g
	}

	nid := gen.alloc()
	g.addNode(&recursionMarkerNode{
		baseNode:   baseNode{nid: nid, ndeps: deps},
		key:        key,
		path:       path,
		subGraph:   ptr,
		schemaName: schemaNameFromMeta(cs, targetIdx),
		maxDepth:   maxDepth,
		depth:      depth,
	})
	return nid, nil
}

func buildEnumNode(
	cs *ir.Schema,
	fd uint32,
	key document.DocumentKey,
	path string,
	deps []int,
	nid int,
) (*enumValidationNode, error) {
	node := &enumValidationNode{
		baseNode:      baseNode{nid: nid, ndeps: deps},
		key:           key,
		path:          path,
		allowedInt:    make(map[int64]struct{}),
		allowedString: make(map[string]struct{}),
		allowedFloat:  make(map[float64]struct{}),
	}
	if cs.Store == nil {
		return node, nil
	}
	if strKey := ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayString); strKey != 0 {
		if vals, ok, _ := cs.Store.GetArrayString(strKey); ok {
			for _, v := range vals {
				node.allowedString[v] = struct{}{}
			}
		}
	}
	if intKey := ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayInt); intKey != 0 {
		if vals, ok, _ := cs.Store.GetArrayInt(intKey); ok {
			for _, v := range vals {
				node.allowedInt[v] = struct{}{}
			}
		}
	}
	if fltKey := ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayFloat); fltKey != 0 {
		if vals, ok, _ := cs.Store.GetArrayFloat(fltKey); ok {
			for _, v := range vals {
				node.allowedFloat[v] = struct{}{}
			}
		}
	}
	return node, nil
}

func buildArrayNode(
	cs *ir.Schema,
	fd uint32,
	key document.DocumentKey,
	path string,
	deps []int,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
	g *validationGraph,
) (int, error) {
	targetIdx := ir.ExtractTargetSchema(fd)
	var subGraph *validationGraph
	if targetIdx != 0 {
		promoted, err := cs.Promote(targetIdx)
		if err != nil {
			return 0, fmt.Errorf("validator: cannot promote array element schema %d: %w", targetIdx, err)
		}
		subGen := &idGen{}
		subBc := newBuildContext()
		subGraph, err = buildGraph(promoted, 0, "", subGen, subBc, maxDepth, depth+1)
		if err != nil {
			return 0, err
		}
	}
	nid := gen.alloc()
	g.addNode(&arrayValidationNode{
		baseNode: baseNode{nid: nid, ndeps: deps},
		key:      key,
		path:     path,
		subGraph: subGraph,
	})
	return nid, nil
}

func buildRecordNode(
	cs *ir.Schema,
	fd uint32,
	key document.DocumentKey,
	path string,
	deps []int,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
	g *validationGraph,
) (int, error) {
	targetIdx := ir.ExtractTargetSchema(fd)
	var subGraph *validationGraph
	if targetIdx != 0 {
		promoted, err := cs.Promote(targetIdx)
		if err != nil {
			return 0, fmt.Errorf("validator: cannot promote record value schema %d: %w", targetIdx, err)
		}
		subGen := &idGen{}
		subBc := newBuildContext()
		subGraph, err = buildGraph(promoted, 0, "", subGen, subBc, maxDepth, depth+1)
		if err != nil {
			return 0, err
		}
	}
	nid := gen.alloc()
	g.addNode(&recordValidationNode{
		baseNode: baseNode{nid: nid, ndeps: deps},
		key:      key,
		path:     path,
		subGraph: subGraph,
	})
	return nid, nil
}

func buildUnionNode(
	cs *ir.Schema,
	fd uint32,
	key document.DocumentKey,
	path string,
	deps []int,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
	g *validationGraph,
) (int, error) {
	variantIdxs := cs.Variants[fd]
	variants := make([]unionVariant, 0, len(variantIdxs))

	for _, variantIdx := range variantIdxs {
		m := cs.Meta[variantIdx]
		if m == nil {
			continue
		}
		if m.Type.IsSchemaBearing() && m.Type != ir.TypeEnum {
			// Object variant: promote and build a sub-graph.
			promoted, err := cs.Promote(variantIdx)
			if err != nil {
				return 0, fmt.Errorf("validator: cannot promote union variant schema %d: %w", variantIdx, err)
			}
			subGen := &idGen{}
			subBc := newBuildContext()
			subGraph, err := buildGraph(promoted, 0, "", subGen, subBc, maxDepth, depth+1)
			if err != nil {
				return 0, err
			}
			variants = append(variants, unionVariant{subGraph: subGraph, dataType: document.TypeRecord})
		} else {
			// Scalar variant.
			dt := ir.FieldTypeToDataType(m.Type)
			variants = append(variants, unionVariant{subGraph: nil, dataType: dt})
		}
	}

	nid := gen.alloc()
	g.addNode(&unionValidationNode{
		baseNode: baseNode{nid: nid, ndeps: deps},
		key:      key,
		path:     path,
		variants: variants,
	})
	return nid, nil
}

func buildConstraintNode(
	node ir.ResolvedConstraintNode,
	path string,
	deps []int,
	gen *idGen,
	g *validationGraph,
) (int, error) {
	switch n := node.(type) {
	case ir.ResolvedConstraint:
		nid := gen.alloc()
		g.addNode(&constraintNode{
			baseNode:   baseNode{nid: nid, ndeps: deps},
			constraint: n,
			path:       path,
		})
		return nid, nil
	case ir.ResolvedConstraintGroup:
		nid := gen.alloc()
		g.addNode(&constraintGroupNode{
			baseNode: baseNode{nid: nid, ndeps: deps},
			group:    n,
			path:     path,
		})
		return nid, nil
	}
	return 0, fmt.Errorf("validator: unknown constraint node type %T", node)
}

// =============================================================================
// PUBLIC API
// =============================================================================

type ValidationConfig struct {
	MaxDepth int
	Mode     ValidationMode
}

func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{MaxDepth: 20, Mode: ValidationModeStrict}
}

type DocumentValidator struct {
	graph  *validationGraph
	config ValidationConfig
}

func New(cs *ir.Schema) (*DocumentValidator, error) {
	return NewWithConfig(cs, DefaultValidationConfig())
}

func NewWithConfig(cs *ir.Schema, config ValidationConfig) (*DocumentValidator, error) {
	gen := &idGen{}
	bc := newBuildContext()
	g, err := buildGraph(cs, 0, "", gen, bc, config.MaxDepth, 0)
	if err != nil {
		return nil, fmt.Errorf("validator: failed to build graph: %w", err)
	}
	return &DocumentValidator{graph: g, config: config}, nil
}

func (v *DocumentValidator) Validate(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModeStrict)
}

func (v *DocumentValidator) ValidatePartial(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModePartialStrict)
}

func (v *DocumentValidator) ValidateLoose(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModeLoose)
}

// =============================================================================
// HELPERS
// =============================================================================

func schemaOffsetRange(cs *ir.Schema, schemaIdx uint8) (int, int) {
	if int(schemaIdx) >= len(cs.SchemaOffsets) {
		return 0, 0
	}
	packed := cs.SchemaOffsets[schemaIdx]
	return int(uint16(packed)), int(uint16(packed >> 16))
}

// resolveFieldPath returns the dot-separated path for a field descriptor,
// reading the field name from Meta.Fields[fd]. Falls back to index notation.
func resolveFieldPath(cs *ir.Schema, fd uint32, basePath string) string {
	ownerIdx := ir.ExtractOwnerSchema(fd)
	if m := cs.Meta[ownerIdx]; m != nil {
		if fm, ok := m.Fields[fd]; ok {
			if basePath == "" {
				return fm.Name
			}
			return basePath + "." + fm.Name
		}
	}
	fieldIdx := ir.ExtractFieldIndex(fd)
	if basePath == "" {
		return fmt.Sprintf("field[%d]", fieldIdx)
	}
	return fmt.Sprintf("%s.field[%d]", basePath, fieldIdx)
}

func joinPath(base, suffix string) string {
	if base == "" {
		return suffix
	}
	if suffix == "" {
		return base
	}
	if suffix[0] == '[' {
		return base + suffix
	}
	return base + "." + suffix
}

func schemaNameFromMeta(cs *ir.Schema, schemaIdx uint8) string {
	if m := cs.Meta[schemaIdx]; m != nil && m.Name != "" {
		return m.Name
	}
	return fmt.Sprintf("schema[%d]", schemaIdx)
}

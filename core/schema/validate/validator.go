// Package validator provides a compiled, graph-based validator for Document
// instances against a compiled ir.Schema.
//
// Build once with New — the schema is walked exactly once to construct the
// execution graph. Call Validate/ValidatePartial/ValidateLoose many times
// after that. The schema is never read again after construction.
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

// ValidationMode defines the strictness level for validation operations.
type ValidationMode byte

const (
	// ValidationModeStrict validates all fields and applies all constraints.
	ValidationModeStrict ValidationMode = iota + 1

	// ValidationModePartialStrict skips REQUIRED_FIELD_MISSING but enforces
	// all other checks including constraints and unexpected fields.
	ValidationModePartialStrict

	// ValidationModeLoose skips REQUIRED_FIELD_MISSING and UNEXPECTED_FIELD.
	ValidationModeLoose
)

// =============================================================================
// VALIDATION CONTEXT  (pooled)
// =============================================================================

type validationContext struct {
	doc    *document.Document
	mode   ValidationMode
	issues []common.Issue
	// failed tracks which node ids have failed. Dependents check this to
	// decide whether to skip. Indexed by node id.
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

// node is a single compiled validation operation.
// execute appends any issues it finds to ctx.issues and returns true on
// success (or clean skip). A false return causes dependent nodes to skip.
type node interface {
	execute(ctx *validationContext) bool
	nodeID() int
	nodeDeps() []int
}

// baseNode provides common fields shared by all node types.
type baseNode struct {
	nid  int
	ndeps []int
}

func (b *baseNode) nodeID() int     { return b.nid }
func (b *baseNode) nodeDeps() []int { return b.ndeps }

// =============================================================================
// VALIDATION GRAPH
// =============================================================================

type validationGraph struct {
	nodes     []node
	order     []int // topological execution order
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

// finalize computes topological execution order via iterative post-order DFS.
// Must be called once after all nodes are added.
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

// traverse executes the compiled graph against doc in the given mode.
func (g *validationGraph) traverse(doc *document.Document, mode ValidationMode) ([]common.Issue, bool) {
	ctx := g.ctxPool.Get().(*validationContext)
	defer g.ctxPool.Put(ctx)
	ctx.reset(doc, mode, g.nodeCount)

	for _, nid := range g.order {
		n := g.nodes[nid]
		if n == nil {
			continue
		}

		// Skip if any dependency failed.
		skip := false
		for _, dep := range g.deps[nid] {
			if ctx.failed[dep] {
				skip = true
				break
			}
		}
		if skip {
			// skipped-clean: do not mark failed, dependents are not blocked.
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

// unexpectedFieldsNode checks that every DocumentKey present in the document
// belongs to the expected set for this schema level. The expected set is a
// map[document.DocumentKey]struct{} built once during graph construction by
// iterating the schema's descriptor slice.
//
// At validation time this node walks doc.positions — the only place a document
// walk is appropriate, since we are asking "what is present that should not be".
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
				path := joinPath(n.path, keyName(dk))
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

// requiredFieldNode checks that a single required field is present and holds
// a concrete value. The DocumentKey it closes over was resolved once during
// graph construction via cs.DocumentKey(path).
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

// enumValidationNode checks that a field's value is a member of the allowed
// set. The allowed sets are extracted from cs.Store once at construction time.
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

// nestedObjectNode delegates to a pre-compiled sub-graph for a single nested
// *Document stored under a TypeRecord field.
type nestedObjectNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	subGraph *validationGraph
}

func (n *nestedObjectNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	recordMap, _, _ := ctx.doc.GetRecord(n.key)
	if recordMap == nil {
		return true
	}
	// A plain object field stores its nested Document under the empty string key.
	nestedDoc, ok := recordMap[""]
	if !ok || nestedDoc == nil {
		return true
	}
	issues, ok := n.subGraph.traverse(nestedDoc, ctx.mode)
	for i := range issues {
		issues[i].Path = joinPath(n.path, issues[i].Path)
	}
	ctx.issues = append(ctx.issues, issues...)
	return ok
}

// arrayValidationNode iterates each *Document element of a TypeArrayObject
// field and validates it against the pre-compiled element sub-graph.
type arrayValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	subGraph *validationGraph // nil = untyped array, no element validation
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

// recordValidationNode iterates each value *Document in a TypeRecord field
// (map[string]*Document) and validates each against the element sub-graph.
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
	for recordKey, item := range recordMap {
		if item == nil {
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

// unionValidationNode validates a nested document against multiple variant
// sub-graphs. Succeeds if the document matches at least one variant.
type unionValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	variants []*validationGraph
}

func (n *unionValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	recordMap, _, _ := ctx.doc.GetRecord(n.key)
	nestedDoc := recordMap[""]
	if nestedDoc == nil {
		return true
	}
	for _, g := range n.variants {
		if _, ok := g.traverse(nestedDoc, ctx.mode); ok {
			return true
		}
	}
	ctx.issues = append(ctx.issues, common.Issue{
		Code:    "UNION_MISMATCH",
		Message: fmt.Sprintf("Value at '%s' does not match any union variant (tried %d variants)", n.path, len(n.variants)),
		Path:    n.path,
	})
	return false
}

// compositeValidationNode validates a nested document against multiple variant
// sub-graphs. Succeeds only if the document matches ALL variants.
type compositeValidationNode struct {
	baseNode
	key      document.DocumentKey
	path     string
	variants []*validationGraph
}

func (n *compositeValidationNode) execute(ctx *validationContext) bool {
	if !ctx.doc.HasValue(n.key) {
		return true
	}
	recordMap, _, _ := ctx.doc.GetRecord(n.key)
	nestedDoc := recordMap[""]
	if nestedDoc == nil {
		return true
	}
	ok := true
	for _, g := range n.variants {
		issues, variantOk := g.traverse(nestedDoc, ctx.mode)
		ctx.issues = append(ctx.issues, issues...)
		if !variantOk {
			ok = false
		}
	}
	return ok
}

// recursionMarkerNode handles a back-edge in the schema reference graph.
// It delegates to a cached sub-graph built for the recursive schema, guarded
// by a depth counter closed over at construction time.
type recursionMarkerNode struct {
	baseNode
	key        document.DocumentKey
	path       string
	subGraph   *validationGraph
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
	recordMap, _, _ := ctx.doc.GetRecord(n.key)
	nestedDoc := recordMap[""]
	if nestedDoc == nil {
		return true
	}
	issues, ok := n.subGraph.traverse(nestedDoc, ctx.mode)
	for i := range issues {
		issues[i].Path = joinPath(n.path, issues[i].Path)
	}
	ctx.issues = append(ctx.issues, issues...)
	return ok
}

// constraintNode evaluates a single resolved constraint predicate.
// All DocumentKeys in constraint.Fields were resolved at compile time.
// At validation time only doc.HasValue calls are made — no schema reads.
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

	// All fields present — run the predicate.
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

	// No fields present at all.
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
			return true // skip cleanly
		}
	}

	// Some present, some missing — partial update.
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
		return true // ValidationModeLoose: skip cleanly
	}
}

// constraintGroupNode evaluates a logical group of constraint nodes.
// Member results are collected first, then folded through the LogicalOperator.
type constraintGroupNode struct {
	baseNode
	group ir.ResolvedConstraintGroup
	path  string
}

func (n *constraintGroupNode) execute(ctx *validationContext) bool {
	results := make([]bool, 0, len(n.group.Constraints))
	issuesBefore := len(ctx.issues)

	for _, member := range n.group.Constraints {
		memberBefore := len(ctx.issues)
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
		_ = memberBefore // member issues accumulated into ctx.issues directly
	}

	if evaluateLogicalOperator(n.group.Operator, results) {
		// Group passed — discard any member issues accumulated speculatively.
		ctx.issues = ctx.issues[:issuesBefore]
		return true
	}

	// Group failed — prepend a group-level issue before the member issues.
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

// =============================================================================
// LOGICAL OPERATOR EVALUATION
// =============================================================================

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

// buildContext tracks which schema indices are currently being built so that
// back-edges (recursive references) can be detected and handled.
type buildContext struct {
	building map[uint8]int
	cache    map[uint8]*validationGraph
}

func newBuildContext() *buildContext {
	return &buildContext{
		building: make(map[uint8]int),
		cache:    make(map[uint8]*validationGraph),
	}
}

func (bc *buildContext) isRecursive(idx uint8) bool { return bc.building[idx] > 0 }
func (bc *buildContext) push(idx uint8)             { bc.building[idx]++ }
func (bc *buildContext) pop(idx uint8) {
	bc.building[idx]--
	if bc.building[idx] <= 0 {
		delete(bc.building, idx)
	}
}

// buildGraph walks cs.Descriptors once for schemaIdx, constructs all nodes,
// and finalizes the execution order. After this returns the schema is not
// read again.
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

	start, end := schemaOffsetRange(cs, schemaIdx)

	// ── Build the expected-key set for unexpected-field detection ─────────────
	// Walk the descriptor slice once to collect every DocumentKey that is valid
	// at this schema level. This set is closed over by unexpectedFieldsNode and
	// never consulted again after graph construction.
	expectedKeys := make(map[document.DocumentKey]struct{}, end-start)
	for _, fd := range cs.Descriptors[start:end] {
		path := fieldPath(cs, fd, basePath)
		key, err := cs.DocumentKey(path)
		if err != nil {
			return nil, fmt.Errorf("validator: cannot resolve key for path %q: %w", path, err)
		}
		expectedKeys[key] = struct{}{}
	}

	unexpID := gen.alloc()
	g.addNode(&unexpectedFieldsNode{
		baseNode: baseNode{nid: unexpID},
		expected: expectedKeys,
		path:     basePath,
	})

	// ── Per-field nodes ───────────────────────────────────────────────────────
	// Single pass over cs.Descriptors[start:end]. After this loop the schema
	// descriptor slice is not read again.
	var fieldNodeIDs []int

	for _, fd := range cs.Descriptors[start:end] {
		path := fieldPath(cs, fd, basePath)
		key, err := cs.DocumentKey(path)
		if err != nil {
			return nil, fmt.Errorf("validator: cannot resolve key for path %q: %w", path, err)
		}

		fieldDeps := []int{unexpID}

		// Required check node.
		if ir.IsRequired(fd) {
			reqID := gen.alloc()
			g.addNode(&requiredFieldNode{
				baseNode: baseNode{nid: reqID, ndeps: fieldDeps},
				key:      key,
				path:     path,
			})
			fieldDeps = []int{reqID}
			fieldNodeIDs = append(fieldNodeIDs, reqID)
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
			fieldNodeIDs = append(fieldNodeIDs, enumID)

		case ir.TypeObject:
			nid, err := buildObjectNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			fieldNodeIDs = append(fieldNodeIDs, nid)

		case ir.TypeArray, ir.TypeSet:
			nid, err := buildArrayNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			fieldNodeIDs = append(fieldNodeIDs, nid)

		case ir.TypeRecord:
			nid, err := buildRecordNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			fieldNodeIDs = append(fieldNodeIDs, nid)

		case ir.TypeUnion:
			nid, err := buildUnionNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			fieldNodeIDs = append(fieldNodeIDs, nid)

		case ir.TypeComposite:
			nid, err := buildCompositeNode(cs, fd, key, path, fieldDeps, gen, bc, maxDepth, depth, g)
			if err != nil {
				return nil, err
			}
			fieldNodeIDs = append(fieldNodeIDs, nid)

		// Scalar fields: required node (if any) is sufficient.
		// No TypeCheckNode — Document's typed storage enforces correctness.
		}
	}

	// ── Constraint nodes ──────────────────────────────────────────────────────
	// ResolvedConstraints already holds []document.DocumentKey — no schema read.
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

// ── Per-type node builders ────────────────────────────────────────────────────

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
	// Enum value sets are stored in cs.Store under keys built by
	// ir.DescriptorToEnumDocumentKey(fd, arrayType) — NOT under the field's
	// path-derived DocumentKey. Try each scalar array type; at most one will hit.
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

func buildObjectNode(
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

	if bc.isRecursive(targetIdx) {
		cached, ok := bc.cache[targetIdx]
		if !ok {
			var err error
			bc.push(targetIdx)
			cached, err = buildGraph(cs, targetIdx, path, gen, bc, maxDepth, depth+1)
			bc.pop(targetIdx)
			if err != nil {
				return 0, err
			}
			bc.cache[targetIdx] = cached
		}
		nid := gen.alloc()
		g.addNode(&recursionMarkerNode{
			baseNode:   baseNode{nid: nid, ndeps: deps},
			key:        key,
			path:       path,
			subGraph:   cached,
			schemaName: schemaName(cs, targetIdx),
			maxDepth:   maxDepth,
			depth:      depth,
		})
		return nid, nil
	}

	bc.push(targetIdx)
	subGraph, err := buildGraph(cs, targetIdx, path, gen, bc, maxDepth, depth+1)
	bc.pop(targetIdx)
	if err != nil {
		return 0, err
	}
	nid := gen.alloc()
	g.addNode(&nestedObjectNode{
		baseNode: baseNode{nid: nid, ndeps: deps},
		key:      key,
		path:     path,
		subGraph: subGraph,
	})
	return nid, nil
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
		bc.push(targetIdx)
		var err error
		subGraph, err = buildGraph(cs, targetIdx, path+"[*]", gen, bc, maxDepth, depth+1)
		bc.pop(targetIdx)
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
		bc.push(targetIdx)
		var err error
		subGraph, err = buildGraph(cs, targetIdx, path+"[*]", gen, bc, maxDepth, depth+1)
		bc.pop(targetIdx)
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
	variants, err := buildVariantGraphs(cs, fd, path, gen, bc, maxDepth, depth)
	if err != nil {
		return 0, err
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

func buildCompositeNode(
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
	variants, err := buildVariantGraphs(cs, fd, path, gen, bc, maxDepth, depth)
	if err != nil {
		return 0, err
	}
	nid := gen.alloc()
	g.addNode(&compositeValidationNode{
		baseNode: baseNode{nid: nid, ndeps: deps},
		key:      key,
		path:     path,
		variants: variants,
	})
	return nid, nil
}

func buildVariantGraphs(
	cs *ir.Schema,
	fd uint32,
	basePath string,
	gen *idGen,
	bc *buildContext,
	maxDepth, depth int,
) ([]*validationGraph, error) {
	variantIdxs := cs.Variants[fd]
	graphs := make([]*validationGraph, 0, len(variantIdxs))
	for _, variantIdx := range variantIdxs {
		bc.push(variantIdx)
		vg, err := buildGraph(cs, variantIdx, basePath, gen, bc, maxDepth, depth+1)
		bc.pop(variantIdx)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, vg)
	}
	return graphs, nil
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

// ValidationConfig holds configuration for validation behaviour.
type ValidationConfig struct {
	MaxDepth int
	Mode     ValidationMode
}

// DefaultValidationConfig returns sensible defaults.
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxDepth: 20,
		Mode:     ValidationModeStrict,
	}
}

// DocumentValidator is the compiled, reusable validator.
// Build once with New; call Validate/ValidatePartial/ValidateLoose many times.
// The schema is not read after construction.
type DocumentValidator struct {
	graph  *validationGraph
	config ValidationConfig
}

// New builds a DocumentValidator from a compiled ir.Schema using default config.
func New(cs *ir.Schema) (*DocumentValidator, error) {
	return NewWithConfig(cs, DefaultValidationConfig())
}

// NewWithConfig builds a DocumentValidator with explicit configuration.
func NewWithConfig(cs *ir.Schema, config ValidationConfig) (*DocumentValidator, error) {
	gen := &idGen{}
	bc := newBuildContext()

	g, err := buildGraph(cs, 0, "", gen, bc, config.MaxDepth, 0)
	if err != nil {
		return nil, fmt.Errorf("validator: failed to build graph: %w", err)
	}

	return &DocumentValidator{graph: g, config: config}, nil
}

// Validate runs strict validation.
func (v *DocumentValidator) Validate(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModeStrict)
}

// ValidatePartial runs partial-strict validation (skips missing required fields).
func (v *DocumentValidator) ValidatePartial(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModePartialStrict)
}

// ValidateLoose runs loose validation (skips missing required and unexpected fields).
func (v *DocumentValidator) ValidateLoose(doc *document.Document) ([]common.Issue, bool) {
	return v.graph.traverse(doc, ValidationModeLoose)
}

// =============================================================================
// HELPERS
// =============================================================================

// schemaOffsetRange unpacks the start and end descriptor positions for a schema.
func schemaOffsetRange(cs *ir.Schema, schemaIdx uint8) (int, int) {
	if int(schemaIdx) >= len(cs.SchemaOffsets) {
		return 0, 0
	}
	packed := cs.SchemaOffsets[schemaIdx]
	return int(uint16(packed)), int(uint16(packed >> 16))
}

// fieldPath reconstructs the dot-separated path for a field descriptor.
// Uses PathCache when available; falls back to field index notation.
func fieldPath(cs *ir.Schema, fd uint32, basePath string) string {
	if cs.PathCache != nil {
		// PathCache is keyed by DocumentKey — but we only have a descriptor here.
		// We ask the Meta for the field name via FieldMeta stored in the schema.
		ownerIdx := ir.ExtractOwnerSchema(fd)
		if m := cs.Meta[ownerIdx]; m != nil {
			if fm, ok := m.Fields[fd]; ok {
				if basePath == "" {
					return fm.Name
				}
				return basePath + "." + fm.Name
			}
		}
	}
	fieldIdx := ir.ExtractFieldIndex(fd)
	if basePath == "" {
		return fmt.Sprintf("field[%d]", fieldIdx)
	}
	return fmt.Sprintf("%s.field[%d]", basePath, fieldIdx)
}

// keyName returns a short human-readable label for a DocumentKey, used in
// unexpected-field issue messages.
func keyName(dk document.DocumentKey) string {
	fd := dk.Descriptor()
	return fmt.Sprintf("field[%d]", ir.ExtractFieldIndex(fd))
}

// joinPath concatenates two path segments.
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

// schemaName returns the schema's human-readable name from Meta.
func schemaName(cs *ir.Schema, schemaIdx uint8) string {
	if m := cs.Meta[schemaIdx]; m != nil && m.Name != "" {
		return m.Name
	}
	return fmt.Sprintf("schema[%d]", schemaIdx)
}

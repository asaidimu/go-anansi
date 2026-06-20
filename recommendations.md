# Validation Engine Refactoring & Design Recommendations

This document captures a structured set of recommendations for improving the architecture, performance, and maintainability of the validation graph engine. They are grouped by priority, with rationale and specific actions.

---

# 🚀 High-Impact Architectural Improvements

## 1. Introduce a Structured Path Type (Replace String Paths)

### Problem

Paths are currently represented as raw strings and manipulated through slicing and string replacement:

* brittle (`fieldName := n.path[len(parentPath)+1:]`)
* error-prone (`strings.Replace("item", ...)`)
* cannot distinguish between identical substrings appearing for different reasons
* hard to compose and refactor

### Recommendation

Introduce a dedicated path type:

```go
type Path struct {
    Parts []string
}
```

Render it to a string **only at the final stage**.

Benefits:

* eliminates brittle string manipulation
* supports arrays and records cleanly (`Parts = ["users", "[3]"]`)
* avoids collisions with user field names (`"item"`)
* simplifies constraints and nested schemas

---

## 2. Cache Subgraphs for Nested Schemas, Arrays, Records, and Unions

### Problem

Each nested schema, array item schema, and union branch creates a **fresh ValidationGraph**, even when identical.

This can explode validation time for deeply nested or repeated structures.

### Recommendation

Add a cache keyed by:

```
(schemaID, fieldType, fieldDef signature)
```

For example:

```go
type SubgraphKey struct {
    SchemaID string
    FieldName string
    FieldType FieldType
}

subgraphCache map[SubgraphKey]*ValidationGraph
```

This makes nested schemas *O(1)* to reuse instead of rebuilding each time.

---

## 3. Replace the Round-Based Traversal With a Proper Topological Sort

### Problem

Traversal is currently an iterative round algorithm similar to a repeated Kahn's loop:

* can degrade to O(N²)
* is harder to reason about
* mixes cycle detection into execution

### Recommendation

Compute a **topological order at graph-build time**:

1. Build adjacency & indegree maps
2. Use Kahn’s algorithm or DFS ordering
3. Store ordered node list on the graph

Then traversal becomes:

```go
for _, nodeID := range graph.topoOrder {
    execute node
}
```

This:

* guarantees correctness
* eliminates O(N²) behavior
* makes cycle detection explicit and early

---

## 4. Separate Responsibilities Into Compiler Phases

### Problem

`buildFromSchema` and related methods mix:

* schema understanding
* node creation
* dependency wiring
* nested compilation
* constraint compilation

This makes the builder huge and fragile.

### Recommendation

Break the process into clear phases:

```
Phase 1: Schema IR extraction (field → semantic description)
Phase 2: Graph assembly (nodes + dependencies)
Phase 3: Graph optimization (dedupe, pruning)
Phase 4: Validation execution (topo traversal)
```

This is the architecture used by:

* compilers
* GUI renderers
* query planners
* streaming pipelines

Your validator is complex enough to justify this.

---

## 5. Eliminate Path Rewriting in Array/Record/Union Validators

### Problem

Current logic rewrites `"item"` using string replacement:

```go
strings.Replace(itemIssues[j].Path, "item", itemPath, 1)
```

This breaks when:

* a user field is named "item"
* nested schemas use "item"
* multiple replacements are needed

### Recommendation

Use path objects (See #1).
Instead of rewriting strings, pass original path components and append index keys:

```go
childPath := parentPath.AppendIndex(i)
```

This becomes trivial when paths are structured.

---

# ⚙️ Medium-Impact Implementation Improvements

## 6. Reduce `*baseNode` Copying and Return NodeIDs Instead

Currently, the builder returns `*baseNode`, which often is a **copy** of the base node embedded in the actual node. This is fragile and unnecessary.

Recommendation:

* Stop returning `*baseNode`
* Return `nodeID` strings only
* Let graph store real nodes

---

## 7. Unify ConstraintNode and ConstraintGroupNode Logic

Constraint groups are handled via:

* direct dependency ID injection
* graph-based evaluation

But constraints and groups should be compiled from a single IR.

Recommendation:

* Create a constraint IR node
* Convert to graph nodes uniformly

---

## 8. Avoid Repeated Lookups into RootData

Every node does:

```go
value, exists := utils.GetValueByPath(ctx.RootData, n.path)
```

Better:

* Precompute scoped data once per node
* Or use a path → data cache

---

## 9. Simplify Union Logic into Subroutines

The current union validator:

* identifies structural errors per branch
* distinguishes constraint-only failures
* aggregates errors
* rewrites paths

This logic should be decomposed into:

* `validateUnionBranch`
* `classifyUnionIssues`
* `mergeUnionIssues`

For testability and clarity.

---

# 🧹 Low-Impact Cleanups

## 10. Consolidate Cycle Detection

You currently detect cycles:

* during graph build (DFS)
* during execution (progressMade = false)

After a successful build-time cycle check, the traversal SHOULD NEVER detect a cycle.

Remove the runtime check or convert it to a panic (internal error).

---

## 11. Replace Format-Based Node IDs

Node IDs like:

```
path:type_check
path:required
```

are brittle.

Recommendation:

```go
type NodeID struct {
    Path Path
    Kind NodeType
    Suffix string
}
```

Or generate numeric IDs and maintain a lookup table.

---

## 12. Make FieldTypeObject Build Logic Less Entangled

Object building is the most complex path in the builder.
Large parts are duplicated between `buildFromSchema` and `buildObjectFieldNodes`.

Extract common logic into dedicated helpers.



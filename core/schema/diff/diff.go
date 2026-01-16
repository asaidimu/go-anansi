package diff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// Copy creates a deep copy of the path to prevent mutation of shared backing arrays
func (p Path) Copy() Path {
	if len(p.Segments) == 0 {
		return Path{}
	}
	segments := make([]PathSegment, len(p.Segments))
	copy(segments, p.Segments)
	return Path{Segments: segments}
}

// Append creates a new Path with the added segment
func (p Path) Append(seg PathSegment) Path {
	newPath := make([]PathSegment, len(p.Segments)+1)
	copy(newPath, p.Segments)
	newPath[len(p.Segments)] = seg
	return Path{Segments: newPath}
}

// -----------------------------------------------------------------------------
// CORE LOGIC
// -----------------------------------------------------------------------------

// Diff generates a diff between two normalized JSON maps (using json.Number).
func Diff(fromSchema, toSchema definition.Schema) (*SchemaDiff, error) {

	from := fromSchema.AsMap()
	to := toSchema.AsMap()

	diff := &SchemaDiff{Changes: make([]SemanticChange, 0, 8)} // Pre-allocate hint

	// 1. Diff Root Properties
	if rootChange := diffRoot(from, to); rootChange != nil {
		diff.Changes = append(diff.Changes, *rootChange)
	}

	// 2. Diff Collections (Fields, Indexes, Constraints, Schemas)
	diff.Changes = append(diff.Changes, diffCollection(from, to, "fields", FieldAdded, FieldRemoved, FieldModified)...)
	diff.Changes = append(diff.Changes, diffCollection(from, to, "indexes", IndexAdded, IndexRemoved, IndexModified)...)
	diff.Changes = append(diff.Changes, diffCollection(from, to, "constraints", ConstraintAdded, ConstraintRemoved, ConstraintModified)...)
	diff.Changes = append(diff.Changes, diffSchemas(from, to)...)

	// 3. Diff Metadata (Generic map comparison)
	diff.Changes = append(diff.Changes, diffMetadata(from, to)...)

	return diff, nil
}

// -----------------------------------------------------------------------------
// OPTIMIZED COMPARISON (The "Secret Sauce")
// -----------------------------------------------------------------------------

// fastEqual compares two values assuming they are normalized JSON types.
// It avoids reflect.DeepEqual for common types to save CPU.
func fastEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}

	switch va := a.(type) {
	case json.Number:
		vb, ok := b.(json.Number)
		return ok && va == vb
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case map[string]any:
		vb, ok := b.(map[string]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for k, v1 := range va {
			if v2, ok := vb[k]; !ok || !fastEqual(v1, v2) {
				return false
			}
		}
		return true
	case []any:
		vb, ok := b.([]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !fastEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// -----------------------------------------------------------------------------
// COLLECTION DIFFERS
// -----------------------------------------------------------------------------

func diffRoot(from, to map[string]any) *SemanticChange {
	var forward, backward []Operation
	pathBase := Path{Segments: []PathSegment{}}

	// Fast lookup via switch
	rootProps := []string{"version", "name", "description"}

	for _, prop := range rootProps {
		valA, okA := from[prop]
		valB, okB := to[prop]

		if !fastEqual(valA, valB) {
			segType := getSegmentType(prop, "root")
			path := pathBase.Append(PathSegment{Type: segType})
			fwd, back := generateOps(path, valA, valB, okA, okB)
			forward = append(forward, fwd)
			backward = append(backward, back)
		}
	}

	if len(forward) > 0 {
		return &SemanticChange{
			Kind:     RootModified,
			Forward:  forward,
			Backward: backward,
		}
	}
	return nil
}

func diffCollection(fromRoot, toRoot map[string]any, collectionName string, addKind, remKind, modKind ChangeKind) []SemanticChange {
	fromMap, _ := fromRoot[collectionName].(map[string]any)
	toMap, _ := toRoot[collectionName].(map[string]any)

	return diffEntityMap(fromMap, toMap, addKind, remKind, modKind, collectionName)
}

func diffSchemas(fromRoot, toRoot map[string]any) []SemanticChange {
	// Standard collection diff for schemas
	changes := diffCollection(fromRoot, toRoot, "schemas", SchemaAdded, SchemaRemoved, SchemaModified)

	// Recursive Check: Look inside modified or existing schemas for nested changes
	fromSchemas, _ := fromRoot["schemas"].(map[string]any)
	toSchemas, _ := toRoot["schemas"].(map[string]any)

	for id, toVal := range toSchemas {
		fromVal, exists := fromSchemas[id]
		if !exists {
			continue // Already handled as SchemaAdded
		}

		// Recurse into nested collections
		fromNested, _ := fromVal.(map[string]any)
		toNested, _ := toVal.(map[string]any)
		compositeId := id // Schema ID

		// Helper to merge nested changes
		appendNested := func(subCol string, a, r, m ChangeKind) {
			nestedChanges := diffNestedCollection(compositeId, fromNested, toNested, subCol, a, r, m)
			changes = append(changes, nestedChanges...)
		}

		appendNested("fields", FieldAdded, FieldRemoved, FieldModified)
		appendNested("indexes", IndexAdded, IndexRemoved, IndexModified)
		appendNested("constraints", ConstraintAdded, ConstraintRemoved, ConstraintModified)
	}

	return changes
}

func diffEntityMap(from, to map[string]any, addKind, remKind, modKind ChangeKind, contextType string) []SemanticChange {
	changes := make([]SemanticChange, 0)
	unionKeys := getUnionKeys(from, to) // Sorted keys ensures deterministic output

	for _, id := range unionKeys {
		fromVal, inFrom := from[id]
		toVal, inTo := to[id]

		path := Path{Segments: []PathSegment{{Type: PathEntity, Key: id}}}

		if inTo && !inFrom {
			// Added
			changes = append(changes, SemanticChange{
				Kind:     addKind,
				EntityId: id,
				Forward:  []Operation{{Type: OpAdd, Path: path, Value: toVal}},
				Backward: []Operation{{Type: OpRemove, Path: path}},
			})
		} else if inFrom && !inTo {
			// Removed
			changes = append(changes, SemanticChange{
				Kind:     remKind,
				EntityId: id,
				Forward:  []Operation{{Type: OpRemove, Path: path}},
				Backward: []Operation{{Type: OpAdd, Path: path, Value: fromVal}},
			})
		} else if !fastEqual(fromVal, toVal) {
			// Modified
			fromEnt, _ := fromVal.(map[string]any)
			toEnt, _ := toVal.(map[string]any)

			fwd, back := diffProperties(path, fromEnt, toEnt, contextType)
			if len(fwd) > 0 {
				changes = append(changes, SemanticChange{
					Kind:     modKind,
					EntityId: id,
					Forward:  fwd,
					Backward: back,
				})
			}
		}
	}
	return changes
}

func diffNestedCollection(parentId string, fromRoot, toRoot map[string]any, colName string, a, r, m ChangeKind) []SemanticChange {
	fromMap, _ := fromRoot[colName].(map[string]any)
	toMap, _ := toRoot[colName].(map[string]any)

	// Similar to diffEntityMap but builds composite ID
	changes := make([]SemanticChange, 0)
	unionKeys := getUnionKeys(fromMap, toMap)

	for _, id := range unionKeys {
		fromVal, inFrom := fromMap[id]
		toVal, inTo := toMap[id]
		compositeId := fmt.Sprintf("%s.%s", parentId, id)
		path := Path{Segments: []PathSegment{{Type: PathEntity, Key: compositeId}}}

		if inTo && !inFrom {
			changes = append(changes, SemanticChange{
				Kind: a, EntityId: compositeId,
				Forward: []Operation{{Type: OpAdd, Path: path, Value: toVal}},
				Backward: []Operation{{Type: OpRemove, Path: path}},
			})
		} else if inFrom && !inTo {
			changes = append(changes, SemanticChange{
				Kind: r, EntityId: compositeId,
				Forward: []Operation{{Type: OpRemove, Path: path}},
				Backward: []Operation{{Type: OpAdd, Path: path, Value: fromVal}},
			})
		} else if !fastEqual(fromVal, toVal) {
			fromEnt, _ := fromVal.(map[string]any)
			toEnt, _ := toVal.(map[string]any)
			fwd, back := diffProperties(path, fromEnt, toEnt, colName) // Pass colName as context
			if len(fwd) > 0 {
				changes = append(changes, SemanticChange{
					Kind: m, EntityId: compositeId,
					Forward: fwd, Backward: back,
				})
			}
		}
	}
	return changes
}

// diffProperties iterates ALL keys in the maps, not just hardcoded ones.
func diffProperties(basePath Path, from, to map[string]any, context string) ([]Operation, []Operation) {
	var forward, backward []Operation
	unionKeys := getUnionKeys(from, to)

	for _, key := range unionKeys {
		fromVal, inFrom := from[key]
		toVal, inTo := to[key]

		if fastEqual(fromVal, toVal) {
			continue
		}

		// Dynamic Path Segment Resolution
		segType := getSegmentType(key, context)
		propPath := basePath.Append(PathSegment{Type: segType})

		// Detect Array vs Scalar
		fromArr, fromIsArr := fromVal.([]any)
		toArr, toIsArr := toVal.([]any)

		if fromIsArr && toIsArr {
			// Array Diffing
			f, b := diffArray(propPath, fromArr, toArr)
			forward = append(forward, f...)
			backward = append(backward, b...)
		} else {
			// Scalar/Object Diffing
			f, b := generateOps(propPath, fromVal, toVal, inFrom, inTo)
			forward = append(forward, f)
			backward = append(backward, b)
		}
	}
	return forward, backward
}

// diffArray uses the robust N^2 approach with fastEqual
func diffArray(path Path, from, to []any) ([]Operation, []Operation) {
	var forward, backward []Operation

	// 1. Deletions (Backwards iteration to preserve indices)
	for i := len(from) - 1; i >= 0; i-- {
		found := false
		for _, toVal := range to {
			if fastEqual(from[i], toVal) {
				found = true
				break
			}
		}
		if !found {
			forward = append(forward, Operation{
				Type: OpCollectionDelete, Path: path, Index: ptr(i), Count: ptr(1),
			})
		}
	}

	// 2. Insertions
	for i, toVal := range to {
		found := false
		for _, fromVal := range from {
			if fastEqual(toVal, fromVal) {
				found = true
				break
			}
		}
		if !found {
			forward = append(forward, Operation{
				Type: OpCollectionInsert, Path: path, Index: ptr(i), Value: toVal,
			})
		}
	}

	// 3. Generate Backward Ops
	for i := len(forward) - 1; i >= 0; i-- {
		op := forward[i]
		if op.Type == OpCollectionInsert {
			backward = append(backward, Operation{
				Type: OpCollectionDelete, Path: path, Index: op.Index, Count: ptr(1),
			})
		} else {
			// For delete, we need the original value to restore it
			backward = append(backward, Operation{
				Type: OpCollectionInsert, Path: path, Index: op.Index, Value: from[*op.Index],
			})
		}
	}

	return forward, backward
}

func diffMetadata(from, to map[string]any) []SemanticChange {
	fromMeta, _ := from["metadata"].(map[string]any)
	toMeta, _ := to["metadata"].(map[string]any)

	changes := make([]SemanticChange, 0)
	unionKeys := getUnionKeys(fromMeta, toMeta)

	for _, key := range unionKeys {
		valA, inA := fromMeta[key]
		valB, inB := toMeta[key]

		if !fastEqual(valA, valB) {
			path := Path{Segments: []PathSegment{{Type: PathSchemaMetadata}}}
			keyPtr := key

			var kind ChangeKind
			var fwd, back []Operation

			if inB && !inA {
				kind = MetadataAdded
				fwd = []Operation{{Type: OpAdd, Path: path, Key: &keyPtr, Value: valB}}
				back = []Operation{{Type: OpRemove, Path: path, Key: &keyPtr}}
			} else if inA && !inB {
				kind = MetadataRemoved
				fwd = []Operation{{Type: OpRemove, Path: path, Key: &keyPtr}}
				back = []Operation{{Type: OpAdd, Path: path, Key: &keyPtr, Value: valA}}
			} else {
				kind = MetadataModified
				fwd = []Operation{{Type: OpSet, Path: path, Key: &keyPtr, Value: valB}}
				back = []Operation{{Type: OpSet, Path: path, Key: &keyPtr, Value: valA}}
			}

			changes = append(changes, SemanticChange{
				Kind: kind, EntityId: key, Forward: fwd, Backward: back,
			})
		}
	}
	return changes
}

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

func generateOps(path Path, from, to any, inFrom, inTo bool) (fwd, back Operation) {
	if !inFrom && inTo {
		fwd = Operation{Type: OpSet, Path: path, Value: to}
		back = Operation{Type: OpRemove, Path: path}
	} else if inFrom && !inTo {
		fwd = Operation{Type: OpRemove, Path: path}
		back = Operation{Type: OpSet, Path: path, Value: from}
	} else {
		fwd = Operation{Type: OpSet, Path: path, Value: to}
		back = Operation{Type: OpSet, Path: path, Value: from}
	}
	return
}

func getUnionKeys(a, b map[string]any) []string {
	keys := make([]string, 0, len(a)+len(b))
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
		seen[k] = struct{}{}
	}
	for k := range b {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys) // Deterministic order
	return keys
}

func ptr[T any](v T) *T { return &v }

// getSegmentType uses a fast switch to map strings to enums
func getSegmentType(key, context string) PathSegmentType {
	// Root properties have specific PathSchema... types
	if context == "root" {
		switch key {
		case "version":
			return PathSchemaVersion
		case "name":
			return PathSchemaName
		case "description":
			return PathSchemaDescription
		case "metadata":
			return PathSchemaMetadata
		}
	}

	// Other common global keys
	switch key {
	case "name":
		return PathName
	case "description":
		return PathDescription
	case "type":
		if context == "indexes" {
			return PathIndexType
		}
		return PathType
	case "unique":
		if context == "indexes" {
			return PathIndexUnique
		}
		return PathUnique
	}

	// Context specific
	switch context {
	case "fields":
		switch key {
		case "required":
			return PathRequired
		case "deprecated":
			return PathDeprecated
		case "default":
			return PathDefault
		case "schema":
			return PathFieldSchema
		}
	case "indexes":
		switch key {
		case "order":
			return PathOrder
		case "fields":
			return PathFields
		case "condition":
			return PathCondition
		}
	case "constraints":
		switch key {
		case "fields":
			return PathConstraintFields
		case "predicate":
			return PathPredicate
		case "parameters":
			return PathParameters
		case "operator":
			return PathOperator
		case "rules":
			return PathRules
		}
	case "schemas":
		switch key {
		case "values":
			return PathValues
		case "concrete":
			return PathConcrete
		case "fields":
			return PathFields
		case "indexes":
			return PathIndexes
		case "constraints":
			return PathConstraints
		}
	}

	return PathUnknown
}

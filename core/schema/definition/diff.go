package definition

import (
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

type VersionBump int

const (
	BumpNone  VersionBump = 0
	BumpPatch VersionBump = 1
	BumpMinor VersionBump = 2
	BumpMajor VersionBump = 3
)

func (b VersionBump) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "none"
	}
}

func (b VersionBump) Apply(v common.Version) common.Version {
	switch b {
	case BumpMajor:
		return v.BumpMajor()
	case BumpMinor:
		return v.BumpMinor()
	case BumpPatch:
		return v.BumpPatch()
	default:
		return v
	}
}

// isSystemEntityName returns true for internal system fields/indexes/schemas
// that are injected by EnrichSchema and should be invisible to the diff engine.
func isSystemEntityName(name string) bool {
	return name == "_id_" || name == "_metadata_" || name == "pk_id"
}

// isSystemChange returns true if the change refers to a system entity.
func isSystemChange(c SemanticChange) bool {
	if isSystemEntityName(c.EntityId) {
		return true
	}
	// Check operation values — nested schemas use UUID IDs, not names.
	for _, op := range c.Forward {
		if op.Type == OpAdd || op.Type == OpSet {
			switch v := op.Value.(type) {
			case NestedSchema:
				if isSystemEntityName(v.Name) {
					return true
				}
			case Field:
				if isSystemEntityName(string(v.Name)) {
					return true
				}
			}
		}
	}
	for _, op := range c.Backward {
		if op.Type == OpAdd || op.Type == OpSet {
			switch v := op.Value.(type) {
			case NestedSchema:
				if isSystemEntityName(v.Name) {
					return true
				}
			case Field:
				if isSystemEntityName(string(v.Name)) {
					return true
				}
			}
		}
	}
	return false
}

func Diff(oldSchema, newSchema *Schema) (*SchemaDiff, error) {
	if oldSchema == nil && newSchema == nil {
		return &SchemaDiff{}, nil
	}
	if oldSchema == nil {
		oldSchema = &Schema{}
	}
	if newSchema == nil {
		newSchema = &Schema{}
	}

	diff := &SchemaDiff{}

	diffFields(oldSchema, newSchema, diff)
	diffIndexes(oldSchema, newSchema, diff)
	diffConstraints(oldSchema, newSchema, diff)
	diffNestedSchemas(oldSchema, newSchema, diff)
	diffMetadata(oldSchema, newSchema, diff)
	diffRootProperties(oldSchema, newSchema, diff)

	// Remove system-level changes (injected by EnrichSchema) that should
	// never be visible to the migration engine.
	filtered := diff.Changes[:0]
	for _, c := range diff.Changes {
		if !isSystemChange(c) {
			filtered = append(filtered, c)
		}
	}
	diff.Changes = filtered

	return diff, nil
}

func entityPath(entityID string) Path {
	return Path{Segments: []PathSegment{{Type: PathEntity, Key: entityID}}}
}

func propertyPath(entityID string, prop PathSegmentType) Path {
	return Path{Segments: []PathSegment{
		{Type: PathEntity, Key: entityID},
		{Type: prop},
	}}
}

func rootPropPath(prop PathSegmentType) Path {
	return Path{Segments: []PathSegment{{Type: prop}}}
}

// nestedEntityPath builds a composite entity path referencing an entity (field,
// index, or constraint) inside a nested schema. Composite entity paths are how
// nested entity changes are represented (see the PathEntity docs: "possibly
// composite for nested").
func nestedEntityPath(schemaID, entityID string) Path {
	return Path{Segments: []PathSegment{
		{Type: PathEntity, Key: schemaID},
		{Type: PathEntity, Key: entityID},
	}}
}

func nestedPropertyPath(schemaID, entityID string, prop PathSegmentType) Path {
	return Path{Segments: []PathSegment{
		{Type: PathEntity, Key: schemaID},
		{Type: PathEntity, Key: entityID},
		{Type: prop},
	}}
}

// IsNestedEntityPath reports whether the path refers to an entity inside a
// nested schema (i.e. a composite entity path). Consumers use it to
// distinguish nested-schema entity changes from top-level ones.
func IsNestedEntityPath(p Path) bool {
	return len(p.Segments) >= 2 && p.Segments[0].Type == PathEntity && p.Segments[1].Type == PathEntity
}

// --- Fields ---

func diffFields(old, new *Schema, diff *SchemaDiff) {
	handled := make(map[string]bool)

	for id, newField := range new.Fields {
		oldField, exists := old.Fields[id]
		if !exists {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     FieldAdded,
				EntityId: string(newField.Name),
				Forward:  []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: newField}},
				Backward: []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			})
			handled[string(id)] = true
			continue
		}

		handled[string(id)] = true
		diffModifiedField(id, &oldField, &newField, diff)
	}

	for id, oldField := range old.Fields {
		if handled[string(id)] {
			continue
		}
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     FieldRemoved,
			EntityId: string(oldField.Name),
			Forward:  []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			Backward: []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: oldField}},
		})
	}
}

func diffModifiedField(id FieldId, old, new *Field, diff *SchemaDiff) {
	forward, backward := collectModifiedFieldOps(id, old, new, propertyPath)
	if len(forward) > 0 {
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     FieldModified,
			EntityId: string(new.Name),
			Forward:  forward,
			Backward: backward,
		})
	}
}

// collectModifiedFieldOps emits OpSet operations for every differing field
// property, using the given path builder. Top-level fields use propertyPath;
// nested fields use a composite nested entity path.
func collectModifiedFieldOps(id FieldId, old, new *Field, propPath func(fieldID string, prop PathSegmentType) Path) (forward, backward []Operation) {
	emit := func(prop PathSegmentType, val, oldVal any) {
		forward = append(forward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: val})
		backward = append(backward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: oldVal})
	}

	if old.Name != new.Name {
		emit(PathName, string(new.Name), string(old.Name))
	}
	if old.Description != new.Description {
		emit(PathDescription, new.Description, old.Description)
	}
	if old.Required != new.Required {
		emit(PathRequired, new.Required, old.Required)
	}
	if old.Deprecated != new.Deprecated {
		emit(PathDeprecated, new.Deprecated, old.Deprecated)
	}
	if old.Unique != new.Unique {
		emit(PathUnique, new.Unique, old.Unique)
	}
	if old.Nullable != new.Nullable {
		emit(PathNullable, new.Nullable, old.Nullable)
	}
	if old.Type != new.Type {
		emit(PathType, new.Type, old.Type)
	}
	if !literalValueEqual(old.Default, new.Default) {
		emit(PathDefault, new.Default, old.Default)
	}
	if !fieldSchemaRefEqual(old.Schema, new.Schema) {
		emit(PathFieldSchema, new.Schema, old.Schema)
	}
	return forward, backward
}

// --- Indexes ---

func diffIndexes(old, new *Schema, diff *SchemaDiff) {
	handled := make(map[string]bool)

	for id, newIdx := range new.Indexes {
		oldIdx, exists := old.Indexes[id]
		if !exists {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     IndexAdded,
				EntityId: newIdx.Name,
				Forward:  []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: newIdx}},
				Backward: []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			})
			handled[string(id)] = true
			continue
		}

		handled[string(id)] = true
		diffModifiedIndex(id, &oldIdx, &newIdx, diff)
	}

	for id, oldIdx := range old.Indexes {
		if handled[string(id)] {
			continue
		}
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     IndexRemoved,
			EntityId: oldIdx.Name,
			Forward:  []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			Backward: []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: oldIdx}},
		})
	}
}

func diffModifiedIndex(id IndexID, old, new *Index, diff *SchemaDiff) {
	forward, backward := collectModifiedIndexOps(id, old, new, propertyPath)
	if len(forward) > 0 {
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     IndexModified,
			EntityId: new.Name,
			Forward:  forward,
			Backward: backward,
		})
	}
}

// collectModifiedIndexOps emits OpSet operations for every differing index
// property, using the given path builder. Top-level indexes use propertyPath;
// nested indexes use a composite nested entity path.
func collectModifiedIndexOps(id IndexID, old, new *Index, propPath func(entityID string, prop PathSegmentType) Path) (forward, backward []Operation) {
	emit := func(prop PathSegmentType, val, oldVal any) {
		forward = append(forward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: val})
		backward = append(backward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: oldVal})
	}

	if old.Name != new.Name {
		emit(PathName, new.Name, old.Name)
	}
	if old.Description != new.Description {
		emit(PathDescription, new.Description, old.Description)
	}
	if old.Type != new.Type {
		emit(PathIndexType, new.Type, old.Type)
	}
	if old.Order != new.Order {
		emit(PathOrder, new.Order, old.Order)
	}
	if old.Unique != new.Unique {
		emit(PathIndexUnique, new.Unique, old.Unique)
	}
	if !fieldNamesEqual(old.Fields, new.Fields) {
		emit(PathFields, new.Fields, old.Fields)
	}
	if !indexConditionEqual(old.Condition, new.Condition) {
		emit(PathCondition, new.Condition, old.Condition)
	}
	return forward, backward
}

// --- Constraints ---

func diffConstraints(old, new *Schema, diff *SchemaDiff) {
	handled := make(map[string]bool)

	for id, newCons := range new.Constraints {
		oldCons, exists := old.Constraints[id]
		if !exists {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     ConstraintAdded,
				EntityId: newCons.Name,
				Forward:  []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: newCons}},
				Backward: []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			})
			handled[string(id)] = true
			continue
		}

		handled[string(id)] = true
		diffModifiedConstraint(id, &oldCons, &newCons, diff)
	}

	for id, oldCons := range old.Constraints {
		if handled[string(id)] {
			continue
		}
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     ConstraintRemoved,
			EntityId: oldCons.Name,
			Forward:  []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			Backward: []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: oldCons}},
		})
	}
}

func diffModifiedConstraint(id ConstraintId, old, new *Constraint, diff *SchemaDiff) {
	forward, backward := collectModifiedConstraintOps(id, old, new, propertyPath)
	if len(forward) > 0 {
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     ConstraintModified,
			EntityId: new.Name,
			Forward:  forward,
			Backward: backward,
		})
	}
}

// collectModifiedConstraintOps emits OpSet operations for every differing
// constraint property, using the given path builder. Top-level constraints use
// propertyPath; nested constraints use a composite nested entity path.
func collectModifiedConstraintOps(id ConstraintId, old, new *Constraint, propPath func(entityID string, prop PathSegmentType) Path) (forward, backward []Operation) {
	emit := func(prop PathSegmentType, val, oldVal any) {
		forward = append(forward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: val})
		backward = append(backward, Operation{Type: OpSet, Path: propPath(string(id), prop), Value: oldVal})
	}

	if old.Name != new.Name {
		emit(PathName, new.Name, old.Name)
	}
	if old.Description != new.Description {
		emit(PathDescription, new.Description, old.Description)
	}

	oldKind := old.ConstraintUnion.Kind()
	newKind := new.ConstraintUnion.Kind()
	if oldKind != newKind {
		emit(PathConstraintKind, new.ConstraintUnion, old.ConstraintUnion)
	} else if oldKind == ConstraintKindRule && newKind == ConstraintKindRule {
		oldRule, _ := ConstraintAs[*ConstraintRule](old.ConstraintUnion)
		newRule, _ := ConstraintAs[*ConstraintRule](new.ConstraintUnion)
		if oldRule != nil && newRule != nil {
			if !fieldNamesEqual(oldRule.Fields, newRule.Fields) {
				emit(PathConstraintFields, newRule.Fields, oldRule.Fields)
			}
			if oldRule.Predicate != newRule.Predicate {
				emit(PathPredicate, newRule.Predicate, oldRule.Predicate)
			}
			if !literalValueEqual(oldRule.Parameters, newRule.Parameters) {
				emit(PathParameters, newRule.Parameters, oldRule.Parameters)
			}
		}
	}

	return forward, backward
}

// --- Nested Schemas ---

func diffNestedSchemas(old, new *Schema, diff *SchemaDiff) {
	handled := make(map[string]bool)

	for id, newSchema := range new.Schemas {
		// System nested schemas (e.g. _metadata_) are injected by EnrichSchema
		// and must stay invisible to the migration engine.
		if isSystemEntityName(newSchema.Name) {
			continue
		}
		oldSchema, exists := old.Schemas[id]
		if !exists {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     SchemaAdded,
				EntityId: string(id),
				Forward:  []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: newSchema}},
				Backward: []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			})
			handled[string(id)] = true
			continue
		}

		handled[string(id)] = true
		diffModifiedNestedSchema(id, &oldSchema, &newSchema, diff)
	}

	for id, oldSchema := range old.Schemas {
		if handled[string(id)] {
			continue
		}
		if isSystemEntityName(oldSchema.Name) {
			continue
		}
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     SchemaRemoved,
			EntityId: string(id),
			Forward:  []Operation{{Type: OpRemove, Path: entityPath(string(id))}},
			Backward: []Operation{{Type: OpAdd, Path: entityPath(string(id)), Value: oldSchema}},
		})
	}
}

func diffModifiedNestedSchema(id SchemaId, old, new *NestedSchema, diff *SchemaDiff) {
	var changes SemanticChange
	changes.Kind = SchemaModified
	changes.EntityId = string(id)

	if old.Name != new.Name {
		changes.Forward = append(changes.Forward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathName), Value: new.Name,
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathName), Value: old.Name,
		})
	}
	if old.Description != new.Description {
		changes.Forward = append(changes.Forward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathDescription), Value: new.Description,
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathDescription), Value: old.Description,
		})
	}
	if old.Concrete != new.Concrete {
		changes.Forward = append(changes.Forward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathConcrete), Value: new.Concrete,
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathConcrete), Value: old.Concrete,
		})
	}

	oldType := old.Type
	newType := new.Type
	if oldType != newType {
		changes.Forward = append(changes.Forward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathType), Value: newType,
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathType), Value: oldType,
		})
	}

	if !literalValuesEqual(old.Values, new.Values) {
		changes.Forward = append(changes.Forward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathValues), Value: new.Values,
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpSet, Path: propertyPath(string(id), PathValues), Value: old.Values,
		})
	}

	// Recurse into the nested schema's fields, indexes, and constraints so that
	// structural changes produce migration-relevant operations (mirroring how
	// top-level schema entities are diffed). See migration_semantics.md:
	// modifySchema carries nested addField/modifyField/removeField changes.
	diffNestedSchemaContents(id, old, new, &changes)

	if len(changes.Forward) > 0 {
		diff.Changes = append(diff.Changes, changes)
	}
}

// diffNestedSchemaContents appends operations describing structural changes to
// a nested schema's fields, indexes, and constraints into the given change.
// All emitted paths are composite entity paths ([PathEntity{schemaID},
// PathEntity{entityID}, ...]) so consumers can distinguish nested entity
// changes from top-level ones via IsNestedEntityPath.
func diffNestedSchemaContents(id SchemaId, old, new *NestedSchema, changes *SemanticChange) {
	diffNestedFields(id, old.Fields, new.Fields, changes)
	diffNestedIndexes(id, old.Indexes, new.Indexes, changes)
	diffNestedConstraints(id, old.Constraints, new.Constraints, changes)
}

func diffNestedFields(id SchemaId, old, new map[FieldId]Field, changes *SemanticChange) {
	handled := make(map[string]bool)

	for fid, newField := range new {
		oldField, exists := old[fid]
		if !exists {
			changes.Forward = append(changes.Forward, Operation{
				Type: OpAdd, Path: nestedEntityPath(string(id), string(fid)), Value: newField,
			})
			changes.Backward = append(changes.Backward, Operation{
				Type: OpRemove, Path: nestedEntityPath(string(id), string(fid)),
			})
			handled[string(fid)] = true
			continue
		}

		handled[string(fid)] = true
		forward, backward := collectModifiedFieldOps(fid, &oldField, &newField, func(eid string, prop PathSegmentType) Path {
			return nestedPropertyPath(string(id), eid, prop)
		})
		changes.Forward = append(changes.Forward, forward...)
		changes.Backward = append(changes.Backward, backward...)
	}

	for fid, oldField := range old {
		if handled[string(fid)] {
			continue
		}
		changes.Forward = append(changes.Forward, Operation{
			Type: OpRemove, Path: nestedEntityPath(string(id), string(fid)),
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpAdd, Path: nestedEntityPath(string(id), string(fid)), Value: oldField,
		})
	}
}

func diffNestedIndexes(id SchemaId, old, new map[IndexID]Index, changes *SemanticChange) {
	handled := make(map[string]bool)

	for iid, newIdx := range new {
		oldIdx, exists := old[iid]
		if !exists {
			changes.Forward = append(changes.Forward, Operation{
				Type: OpAdd, Path: nestedEntityPath(string(id), string(iid)), Value: newIdx,
			})
			changes.Backward = append(changes.Backward, Operation{
				Type: OpRemove, Path: nestedEntityPath(string(id), string(iid)),
			})
			handled[string(iid)] = true
			continue
		}

		handled[string(iid)] = true
		forward, backward := collectModifiedIndexOps(iid, &oldIdx, &newIdx, func(eid string, prop PathSegmentType) Path {
			return nestedPropertyPath(string(id), eid, prop)
		})
		changes.Forward = append(changes.Forward, forward...)
		changes.Backward = append(changes.Backward, backward...)
	}

	for iid, oldIdx := range old {
		if handled[string(iid)] {
			continue
		}
		changes.Forward = append(changes.Forward, Operation{
			Type: OpRemove, Path: nestedEntityPath(string(id), string(iid)),
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpAdd, Path: nestedEntityPath(string(id), string(iid)), Value: oldIdx,
		})
	}
}

func diffNestedConstraints(id SchemaId, old, new map[ConstraintId]Constraint, changes *SemanticChange) {
	handled := make(map[string]bool)

	for cid, newCons := range new {
		oldCons, exists := old[cid]
		if !exists {
			changes.Forward = append(changes.Forward, Operation{
				Type: OpAdd, Path: nestedEntityPath(string(id), string(cid)), Value: newCons,
			})
			changes.Backward = append(changes.Backward, Operation{
				Type: OpRemove, Path: nestedEntityPath(string(id), string(cid)),
			})
			handled[string(cid)] = true
			continue
		}

		handled[string(cid)] = true
		forward, backward := collectModifiedConstraintOps(cid, &oldCons, &newCons, func(eid string, prop PathSegmentType) Path {
			return nestedPropertyPath(string(id), eid, prop)
		})
		changes.Forward = append(changes.Forward, forward...)
		changes.Backward = append(changes.Backward, backward...)
	}

	for cid, oldCons := range old {
		if handled[string(cid)] {
			continue
		}
		changes.Forward = append(changes.Forward, Operation{
			Type: OpRemove, Path: nestedEntityPath(string(id), string(cid)),
		})
		changes.Backward = append(changes.Backward, Operation{
			Type: OpAdd, Path: nestedEntityPath(string(id), string(cid)), Value: oldCons,
		})
	}
}

// --- Metadata ---

func diffMetadata(old, new *Schema, diff *SchemaDiff) {
	if old == nil || new == nil {
		return
	}
	handled := make(map[string]bool)

	for key, newVal := range new.Metadata {
		oldVal, exists := old.Metadata[key]
		if !exists {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     MetadataAdded,
				EntityId: key,
				Forward:  []Operation{{Type: OpAdd, Path: entityPath(key), Value: newVal}},
				Backward: []Operation{{Type: OpRemove, Path: entityPath(key)}},
			})
			handled[key] = true
			continue
		}

		handled[key] = true
		if !reflect.DeepEqual(oldVal, newVal) {
			diff.Changes = append(diff.Changes, SemanticChange{
				Kind:     MetadataModified,
				EntityId: key,
				Forward:  []Operation{{Type: OpSet, Path: entityPath(key), Value: newVal}},
				Backward: []Operation{{Type: OpSet, Path: entityPath(key), Value: oldVal}},
			})
		}
	}

	for key := range old.Metadata {
		if handled[key] {
			continue
		}
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     MetadataRemoved,
			EntityId: key,
			Forward:  []Operation{{Type: OpRemove, Path: entityPath(key)}},
			Backward: []Operation{{Type: OpAdd, Path: entityPath(key), Value: old.Metadata[key]}},
		})
	}
}

// --- Root Properties ---

func diffRootProperties(old, new *Schema, diff *SchemaDiff) {
	var (
		forward  []Operation
		backward []Operation
	)

	if old.Name != new.Name {
		forward = append(forward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaName), Value: new.Name,
		})
		backward = append(backward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaName), Value: old.Name,
		})
	}
	if old.Description != new.Description {
		forward = append(forward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaDescription), Value: new.Description,
		})
		backward = append(backward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaDescription), Value: old.Description,
		})
	}
	if old.Version != nil && new.Version != nil && old.Version.Compare(new.Version) != 0 {
		forward = append(forward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaVersion), Value: new.Version,
		})
		backward = append(backward, Operation{
			Type: OpSet, Path: rootPropPath(PathSchemaVersion), Value: old.Version,
		})
	}

	if len(forward) > 0 {
		diff.Changes = append(diff.Changes, SemanticChange{
			Kind:     RootModified,
			EntityId: "root",
			Forward:  forward,
			Backward: backward,
		})
	}
}

// --- Version Impact ---

func VersionImpact(diff *SchemaDiff) VersionBump {
	impact := BumpPatch

	for _, c := range diff.Changes {
		switch c.Kind {
		case FieldAdded:
			for _, op := range c.Forward {
				if op.Type == OpAdd {
					if f, ok := op.Value.(Field); ok {
						if f.Required {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					}
				}
			}

		case FieldRemoved:
			return BumpMajor

		case FieldModified:
			for _, op := range c.Forward {
				if op.Type == OpSet {
					switch op.Path.Segments[len(op.Path.Segments)-1].Type {
					case PathType:
						return BumpMajor
					case PathRequired:
						if v, ok := op.Value.(bool); ok && v {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case PathName:
						return BumpMajor
					case PathUnique:
						if v, ok := op.Value.(bool); ok && v {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case PathDefault:
						return BumpMajor
					case PathDeprecated:
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case PathFieldSchema:
						return BumpMajor
					}
				}
			}

		case IndexAdded:
			for _, op := range c.Forward {
				if op.Type == OpAdd {
					if idx, ok := op.Value.(Index); ok && idx.Unique {
						return BumpMajor
					}
				}
			}

		case IndexRemoved:
			for _, op := range c.Backward {
				if op.Type == OpAdd {
					if idx, ok := op.Value.(Index); ok && idx.Unique {
						if impact < BumpMinor {
							impact = BumpMinor
						}
					}
				}
			}

		case IndexModified:
			for _, op := range c.Forward {
				if op.Type == OpSet {
					switch op.Path.Segments[len(op.Path.Segments)-1].Type {
					case PathIndexUnique:
						if v, ok := op.Value.(bool); ok && v {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case PathFields:
						return BumpMajor
					}
				}
			}

		case ConstraintAdded:
			return BumpMajor

		case ConstraintModified:
			for _, op := range c.Forward {
				if op.Type == OpSet {
					switch op.Path.Segments[len(op.Path.Segments)-1].Type {
					case PathPredicate, PathParameters, PathConstraintFields:
						return BumpMajor
					}
				}
			}

		case SchemaAdded:
			return BumpMajor

		case SchemaRemoved:
			return BumpMajor

		case SchemaModified:
			for _, op := range c.Forward {
				if op.Type == OpAdd && IsNestedEntityPath(op.Path) {
					switch v := op.Value.(type) {
					case Field:
						if v.Required {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case Index:
						if v.Unique {
							return BumpMajor
						}
					case Constraint:
						return BumpMajor
					}
					continue
				}
				if op.Type == OpRemove && IsNestedEntityPath(op.Path) {
					return BumpMajor
				}
				if op.Type != OpSet {
					continue
				}
				lastSeg := op.Path.Segments[len(op.Path.Segments)-1].Type
				if IsNestedEntityPath(op.Path) {
					switch lastSeg {
					case PathName, PathType, PathDefault, PathFieldSchema, PathIndexType, PathFields,
						PathPredicate, PathParameters, PathConstraintFields, PathConstraintKind:
						return BumpMajor
					case PathRequired, PathUnique, PathIndexUnique:
						if v, ok := op.Value.(bool); ok && v {
							return BumpMajor
						}
						if impact < BumpMinor {
							impact = BumpMinor
						}
					case PathDeprecated:
						if impact < BumpMinor {
							impact = BumpMinor
						}
					}
					continue
				}
				switch lastSeg {
				case PathType, PathValues:
					return BumpMajor
				}
			}
		}
	}

	return impact
}

// --- Equality Helpers ---

func literalValueEqual(a, b LiteralValue) bool {
	return reflect.DeepEqual(a, b)
}

func literalValuesEqual(a, b LiteralValues) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !literalValueEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func fieldNamesEqual(a, b []FieldName) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func fieldSchemaRefEqual(a, b FieldSchemaReference) bool {
	return reflect.DeepEqual(a, b)
}

func indexConditionEqual(a, b IndexConditionUnion) bool {
	if a.IsZero() && b.IsZero() {
		return true
	}
	if a.IsZero() != b.IsZero() {
		return false
	}
	if a.IsCondition() && b.IsCondition() {
		ca, _ := IndexConditionAs[*IndexCondition](a)
		cb, _ := IndexConditionAs[*IndexCondition](b)
		if ca == nil || cb == nil {
			return false
		}
		return ca.Field == cb.Field &&
			ca.Operator == cb.Operator &&
			literalValueEqual(ca.Value, cb.Value)
	}
	if a.IsConditionGroup() && b.IsConditionGroup() {
		ga, _ := IndexConditionAs[*IndexConditionGroup](a)
		gb, _ := IndexConditionAs[*IndexConditionGroup](b)
		if ga == nil || gb == nil {
			return false
		}
		return reflect.DeepEqual(ga, gb)
	}
	return false
}

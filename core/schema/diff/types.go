package diff

import (
	"encoding/json"
)

type SchemaDiff struct {
	Changes []SemanticChange `json:"changes"`
}

type SemanticChange struct {
	Kind     ChangeKind    `json:"kind"`
	EntityId string        `json:"entity_id"`

	Forward  []Operation   `json:"forward"`
	Backward []Operation   `json:"backward"`
}

type ChangeKind byte

const (
	// Fields
	FieldAdded ChangeKind = iota + 1
	FieldRemoved
	FieldModified

	// Indexes
	IndexAdded
	IndexRemoved
	IndexModified

	// Constraints
	ConstraintAdded
	ConstraintRemoved
	ConstraintModified

	// Schemas
	SchemaAdded
	SchemaRemoved
	SchemaModified

	// Metadata
	MetadataAdded
	MetadataRemoved
	MetadataModified

	// Root schema properties
	RootModified
)

type Operation struct {
	Type  OperationType `json:"type"`
	Path  Path          `json:"path"`
	Value any           `json:"value,omitempty"`
	Key   *string       `json:"key,omitempty"`   // For map operations
	Index *int          `json:"index,omitempty"` // For array operations
	Count *int          `json:"count,omitempty"` // For array deletes
}

type OperationType byte

const (
	OpAdd OperationType = iota + 1
	OpRemove
	OpSet
	OpCollectionInsert
	OpCollectionDelete
)

type Path struct {
	Segments []PathSegment `json:"segments"`
}

type PathSegment struct {
	Type PathSegmentType `json:"type"`
	Key  string          `json:"key,omitempty"`
}

type PathSegmentType byte

const (
	// Root schema properties
	PathSchemaVersion PathSegmentType = iota + 1
	PathSchemaName
	PathSchemaDescription
	PathSchemaMetadata

	// Entity reference (globally unique ID, possibly composite for nested)
	PathEntity

	// Shared properties
	PathName
	PathDescription

	// Field properties
	PathRequired
	PathDeprecated
	PathUnique
	PathType
	PathDefault
	PathFieldSchema

	// Index properties
	PathIndexType
	PathOrder
	PathIndexUnique
	PathFields
	PathCondition
	PathConditions  // For IndexConditionGroup.Conditions array

	// Constraint properties
	PathConstraintKind
	PathConstraintFields
	PathPredicate
	PathParameters
	PathOperator
	PathRules

	// Nested schema specific properties
	PathValues
	PathConcrete

	PathIndexes
	PathConstraints
	PathUnknown // Catch-all for new/unmapped fields
)

func (ot OperationType) String() string {
	switch ot {
	case OpAdd:
		return "add"
	case OpRemove:
		return "remove"
	case OpSet:
		return "set"
	case OpCollectionInsert:
		return "collection_insert"
	case OpCollectionDelete:
		return "collection_delete"
	default:
		return "unknown"
	}
}

func (ot OperationType) MarshalJSON() ([]byte, error) {
	return json.Marshal(ot.String())
}

func (ot *OperationType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "add":
		*ot = OpAdd
	case "remove":
		*ot = OpRemove
	case "set":
		*ot = OpSet
	case "collection_insert":
		*ot = OpCollectionInsert
	case "collection_delete":
		*ot = OpCollectionDelete
	default:
		*ot = 0 // Unknown or invalid
	}
	return nil
}

func (ck ChangeKind) String() string {
	switch ck {
	case FieldAdded:
		return "field_added"
	case FieldRemoved:
		return "field_removed"
	case FieldModified:
		return "field_modified"
	case IndexAdded:
		return "index_added"
	case IndexRemoved:
		return "index_removed"
	case IndexModified:
		return "index_modified"
	case ConstraintAdded:
		return "constraint_added"
	case ConstraintRemoved:
		return "constraint_removed"
	case ConstraintModified:
		return "constraint_modified"
	case SchemaAdded:
		return "schema_added"
	case SchemaRemoved:
		return "schema_removed"
	case SchemaModified:
		return "schema_modified"
	case MetadataAdded:
		return "metadata_added"
	case MetadataRemoved:
		return "metadata_removed"
	case MetadataModified:
		return "metadata_modified"
	case RootModified:
		return "root_modified"
	default:
		return "unknown"
	}
}

func (ck ChangeKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(ck.String())
}

func (ck *ChangeKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "field_added":
		*ck = FieldAdded
	case "field_removed":
		*ck = FieldRemoved
	case "field_modified":
		*ck = FieldModified
	case "index_added":
		*ck = IndexAdded
	case "index_removed":
		*ck = IndexRemoved
	case "index_modified":
		*ck = IndexModified
	case "constraint_added":
		*ck = ConstraintAdded
	case "constraint_removed":
		*ck = ConstraintRemoved
	case "constraint_modified":
		*ck = ConstraintModified
	case "schema_added":
		*ck = SchemaAdded
	case "schema_removed":
		*ck = SchemaRemoved
	case "schema_modified":
		*ck = SchemaModified
	case "metadata_added":
		*ck = MetadataAdded
	case "metadata_removed":
		*ck = MetadataRemoved
	case "metadata_modified":
		*ck = MetadataModified
	case "root_modified":
		*ck = RootModified
	default:
		*ck = 0 // Unknown or invalid
	}
	return nil
}



func (pst PathSegmentType) String() string {
	switch pst {
	case PathSchemaVersion:
		return "version"
	case PathSchemaName:
		return "schema_name"
	case PathSchemaDescription:
		return "schema_description"
	case PathSchemaMetadata:
		return "metadata"
	case PathEntity:
		return "entity"
	case PathName:
		return "name"
	case PathDescription:
		return "description"
	case PathRequired:
		return "required"
	case PathDeprecated:
		return "deprecated"
	case PathUnique:
		return "unique"
	case PathType:
		return "type"
	case PathDefault:
		return "default"
	case PathFieldSchema:
		return "schema"
	case PathIndexType:
		return "index_type"
	case PathOrder:
		return "order"
	case PathIndexUnique:
		return "index_unique"
	case PathFields:
		return "fields"
	case PathCondition:
		return "condition"
	case PathConditions:
		return "conditions"
	case PathConstraintKind:
		return "constraint_kind"
	case PathConstraintFields:
		return "constraint_fields"
	case PathPredicate:
		return "predicate"
	case PathParameters:
		return "parameters"
	case PathOperator:
		return "operator"
	case PathRules:
		return "rules"
	case PathValues:
		return "values"
	case PathConcrete:
		return "concrete"
	default:
		return "unknown"
	}
}

func (pst PathSegmentType) MarshalJSON() ([]byte, error) {
	return json.Marshal(pst.String())
}

func (pst *PathSegmentType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "version":
		*pst = PathSchemaVersion
	case "schema_name":
		*pst = PathSchemaName
	case "schema_description":
		*pst = PathSchemaDescription
	case "metadata":
		*pst = PathSchemaMetadata
	case "entity":
		*pst = PathEntity
	case "name":
		*pst = PathName
	case "description":
		*pst = PathDescription
	case "required":
		*pst = PathRequired
	case "deprecated":
		*pst = PathDeprecated
	case "unique":
		*pst = PathUnique
	case "type":
		*pst = PathType
	case "default":
		*pst = PathDefault
	case "schema":
		*pst = PathFieldSchema
	case "index_type":
		*pst = PathIndexType
	case "order":
		*pst = PathOrder
	case "index_unique":
		*pst = PathIndexUnique
	case "fields":
		*pst = PathFields
	case "condition":
		*pst = PathCondition
	case "conditions":
		*pst = PathConditions
	case "constraint_kind":
		*pst = PathConstraintKind
	case "constraint_fields":
		*pst = PathConstraintFields
	case "predicate":
		*pst = PathPredicate
	case "parameters":
		*pst = PathParameters
	case "operator":
		*pst = PathOperator
	case "rules":
		*pst = PathRules
	case "values":
		*pst = PathValues
	case "concrete":
		*pst = PathConcrete
	default:
		*pst = PathUnknown
	}
	return nil
}

func init() {
	_ = FieldAdded
	_ = FieldRemoved
	_ = FieldModified
	_ = IndexAdded
	_ = IndexRemoved
	_ = IndexModified
	_ = ConstraintAdded
	_ = ConstraintRemoved
	_ = ConstraintModified
	_ = SchemaAdded
	_ = SchemaRemoved
	_ = SchemaModified
	_ = MetadataAdded
	_ = MetadataRemoved
	_ = MetadataModified
	_ = RootModified

	_ = OpAdd
	_ = OpRemove
	_ = OpSet
	_ = OpCollectionInsert
	_ = OpCollectionDelete

	_ = PathSchemaVersion
	_ = PathSchemaName
	_ = PathSchemaDescription
	_ = PathSchemaMetadata
	_ = PathEntity
	_ = PathName
	_ = PathDescription
	_ = PathRequired
	_ = PathDeprecated
	_ = PathUnique
	_ = PathType
	_ = PathDefault
	_ = PathFieldSchema
	_ = PathIndexType
	_ = PathOrder
	_ = PathIndexUnique
	_ = PathFields
	_ = PathCondition
	_ = PathConditions
	_ = PathConstraintKind
	_ = PathConstraintFields
	_ = PathPredicate
	_ = PathParameters
	_ = PathOperator
	_ = PathRules
	_ = PathValues
	_ = PathConcrete
	_ = PathIndexes
	_ = PathConstraints
	_ = PathUnknown // Catch-all for new/unmapped fields
}

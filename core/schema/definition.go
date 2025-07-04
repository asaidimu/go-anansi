// Package schema provides the foundational types and structures for defining data schemas.
// It includes definitions for fields, constraints, indexes, and migrations, forming a
// comprehensive framework for data modeling and validation.
package schema

import (
	"encoding/json"
	"fmt"
	"maps"
)

// LogicalOperator defines the logical operators that can be used to combine conditions
// in constraints and filters.
type LogicalOperator string

// Supported logical operators.
const (
	LogicalAnd LogicalOperator = "and" // Represents a logical AND.
	LogicalOr  LogicalOperator = "or"  // Represents a logical OR.
	LogicalNot LogicalOperator = "not" // Represents a logical NOT.
	LogicalNor LogicalOperator = "nor" // Represents a logical NOR.
	LogicalXor LogicalOperator = "xor" // Represents a logical XOR.
)

// FieldType represents the data type of a field in a schema.
type FieldType string

// Supported field types.
const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeInteger FieldType = "integer"
	FieldTypeDecimal FieldType = "decimal"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeArray   FieldType = "array"
	FieldTypeSet     FieldType = "set"
	FieldTypeEnum    FieldType = "enum"
	FieldTypeObject  FieldType = "object"
	FieldTypeRecord  FieldType = "record"
	FieldTypeUnion   FieldType = "union"
)

// IndexType represents the type of an index, which is used to optimize database queries.
type IndexType string

// Supported index types.
const (
	IndexTypeNormal   IndexType = "normal"
	IndexTypeUnique   IndexType = "unique"
	IndexTypePrimary  IndexType = "primary"
	IndexTypeSpatial  IndexType = "spatial"
	IndexTypeFullText IndexType = "fulltext"
)

// PredicateParameters is an interface that all predicate parameter types must satisfy.
type PredicateParameters any

// PredicateParams is a generic struct that holds the parameters for a predicate function.
type PredicateParams[T any] struct {
	Data  T                   // The data being validated.
	Field *string             // The specific field being validated.
	Args  PredicateParameters // The arguments for the predicate.
}

// Predicate defines a function for data validation.
type Predicate[T any] func(params PredicateParams[T]) bool

// PredicateMap is a map of predicate names to their validation functions.
type PredicateMap map[string]any

// FunctionMap is a map of function names to generic functions.
type FunctionMap map[string]any

// PredicateName represents the name of a supported predicate.
type PredicateName string

// PredicateParam is a map of parameters for a predicate.
type PredicateParam map[string]any

// ConstraintType represents the type of a constraint.
type ConstraintType string

// Supported constraint types.
const (
	ConstraintTypeSchema ConstraintType = "schema"
)

// Constraint defines a validation rule for a field or schema.
type Constraint[T FieldType] struct {
	Type         *ConstraintType `json:"type,omitempty"`
	Predicate    string          `json:"predicate"`
	Field        *string         `json:"field,omitempty"`
	Parameters   any             `json:"parameters,omitempty"`
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	ErrorMessage *string         `json:"errorMessage,omitempty"`
}

// IsSchemaConstraintRule is a marker method to satisfy the SchemaConstraintRule interface.
func (c Constraint[T]) IsSchemaConstraintRule() {}

// ConstraintGroup defines a group of multiple constraints with a logical operator.
type ConstraintGroup[T FieldType] struct {
	Name     string                    `json:"name"`
	Operator LogicalOperator           `json:"operator"`
	Rules    []SchemaConstraintRule[T] `json:"rules"`
}

// IsSchemaConstraintRule is a marker method to satisfy the SchemaConstraintRule interface.
func (cg ConstraintGroup[T]) IsSchemaConstraintRule() {}

// SchemaConstraintRule is an interface that both Constraint and ConstraintGroup must implement.
type SchemaConstraintRule[T FieldType] interface {
	IsSchemaConstraintRule()
}

// SchemaConstraint represents a collection of constraints or groups applied at the schema level.
type SchemaConstraint[T FieldType] []SchemaConstraintRule[T]

// FieldSchema defines a reference to a nested schema.
type FieldSchema struct {
	ID          string                      `json:"id"`
	Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	Indexes     []IndexDefinition           `json:"indexes,omitempty"`
}

// FieldDefinition defines a field within a schema.
type FieldDefinition struct {
	Name        string                      `json:"name"`
	Type        FieldType                   `json:"type"`
	Required    *bool                       `json:"required,omitempty"`
	Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	Default     any                         `json:"default,omitempty"`
	Values      []any                       `json:"values,omitempty"`
	Schema      any                         `json:"schema,omitempty"`
	ItemsType   *FieldType                  `json:"itemsType,omitempty"`
	Deprecated  *bool                       `json:"deprecated,omitempty"`
	Description *string                     `json:"description,omitempty"`
	Unique      *bool                       `json:"unique,omitempty"`
	Hint        *struct {
		Input InputHint `json:"input"`
	} `json:"hint,omitempty"`
}

func (fd *FieldDefinition) UnmarshalJSON(data []byte) error {
	type Alias FieldDefinition // Create an alias to avoid infinite recursion

	// Unmarshal into a temporary struct to access the 'type' field and raw 'schema' field
	var temp struct {
		Type   FieldType       `json:"type"`
		Schema json.RawMessage `json:"schema,omitempty"`
		*Alias
	}

	temp.Alias = (*Alias)(fd) // Point Alias to the actual FieldDefinition
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the unmarshaled data back to the original FieldDefinition
	*fd = FieldDefinition(*temp.Alias)

	// Now, handle the 'Schema' field based on its 'Type'
	if temp.Schema != nil {
		switch temp.Type {
		case FieldTypeObject, FieldTypeUnion:
			// For FieldTypeObject or FieldTypeUnion, Schema should be FieldSchema or []FieldSchema
			var singleSchema FieldSchema
			if err := json.Unmarshal(temp.Schema, &singleSchema); err == nil {
				fd.Schema = singleSchema
				return nil
			}
			var multiSchema []FieldSchema
			if err := json.Unmarshal(temp.Schema, &multiSchema); err == nil {
				fd.Schema = multiSchema
				return nil
			}
			// Fall through to generic unmarshaling if specific types don't match
		case FieldTypeRecord:
			// For FieldTypeRecord, Schema should be SchemaDefinition
			var recordSchema SchemaDefinition
			if err := json.Unmarshal(temp.Schema, &recordSchema); err == nil {
				fd.Schema = recordSchema
				return nil
			}
			// Fall through to generic unmarshaling if specific types don't match
		}

		// For any other types or if specific unmarshaling failed,
		// unmarshal Schema into a generic any. This will likely be map[string]any for objects.
		var genericSchema any
		if err := json.Unmarshal(temp.Schema, &genericSchema); err != nil {
			return fmt.Errorf("failed to unmarshal FieldDefinition.Schema into expected types or generic any: %w", err)
		}
		fd.Schema = genericSchema
	}
	return nil
}

// PartialIndexCondition defines a condition for a partial index.
type PartialIndexCondition struct {
	Operator   LogicalOperator         `json:"operator"`
	Field      string                  `json:"field"`
	Value      any                     `json:"value,omitempty"`
	Conditions []PartialIndexCondition `json:"conditions,omitempty"`
}

// IndexDefinition defines an index for a collection.
type IndexDefinition struct {
	Fields      []string               `json:"fields"`
	Type        IndexType              `json:"type"`
	Unique      *bool                  `json:"unique,omitempty"`
	Partial     *PartialIndexCondition `json:"partial,omitempty"`
	Description *string                `json:"description,omitempty"`
	Order       *string                `json:"order,omitempty"`
	Name        string                 `json:"name"`
}

// NestedSchemaDefinition represents a reusable, nested schema structure.
type NestedSchemaDefinition struct {
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Indexes     []IndexDefinition `json:"indexes,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Concrete    *bool             `json:"concrete,omitempty"`

	Type               *FieldType                  `json:"type,omitempty"`
	LiteralConstraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	LiteralDefault     any                         `json:"default,omitempty"`
	LiteralSchema      any                         `json:"schema,omitempty"`
	LiteralItemsType   *FieldType                  `json:"itemsType,omitempty"`

	StructuredFieldsMap   map[string]*FieldDefinition `json:"fields,omitempty"`
	StructuredFieldsArray []struct {
		Fields map[string]*FieldDefinition `json:"fields"`
		When   *struct {
			Field string `json:"field"`
			Value any    `json:"value"`
		} `json:"when,omitempty"`
	} `json:"fields,omitempty"`

	isStructured bool
}

// UnmarshalJSON implements the json.Unmarshaler interface for NestedSchemaDefinition.
func (nsd *NestedSchemaDefinition) UnmarshalJSON(data []byte) error {
	var temp struct {
		Name        string            `json:"name"`
		Description *string           `json:"description"`
		Indexes     []IndexDefinition `json:"indexes"`
		Metadata    map[string]any    `json:"metadata"`
		Concrete    *bool             `json:"concrete"`

		Type               *FieldType                  `json:"type"`
		LiteralConstraints SchemaConstraint[FieldType] `json:"constraints"`
		LiteralDefault     any                         `json:"default"`
		LiteralSchema      json.RawMessage             `json:"schema"`
		LiteralItemsType   *FieldType                  `json:"itemsType"`

		Fields json.RawMessage `json:"fields"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	nsd.Name = temp.Name
	nsd.Description = temp.Description
	nsd.Indexes = temp.Indexes
	nsd.Metadata = temp.Metadata
	nsd.Concrete = temp.Concrete

	hasFields := temp.Fields != nil
	hasType := temp.Type != nil

	if hasFields && hasType {
		return fmt.Errorf("NestedSchemaDefinition cannot have both 'fields' and 'type'")
	}

	if hasFields {
		nsd.isStructured = true
		var fieldsMap map[string]*FieldDefinition
		if err := json.Unmarshal(temp.Fields, &fieldsMap); err == nil {
			nsd.StructuredFieldsMap = fieldsMap
		} else {
			var fieldsArray []struct {
				Fields map[string]*FieldDefinition `json:"fields"`
				When   *struct {
					Field string `json:"field"`
					Value any    `json:"value"`
				} `json:"when,omitempty"`
			}
			if err := json.Unmarshal(temp.Fields, &fieldsArray); err == nil {
				nsd.StructuredFieldsArray = fieldsArray
			} else {
				return fmt.Errorf("failed to unmarshal NestedSchemaDefinition.fields")
			}
		}
	} else if hasType {
		nsd.isStructured = false
		nsd.Type = temp.Type
		nsd.LiteralConstraints = temp.LiteralConstraints
		nsd.LiteralDefault = temp.LiteralDefault
		nsd.LiteralItemsType = temp.LiteralItemsType

		if temp.LiteralSchema != nil {
			var singleSchema FieldSchema
			if err := json.Unmarshal(temp.LiteralSchema, &singleSchema); err == nil {
				nsd.LiteralSchema = singleSchema
			} else {
				var multiSchema []FieldSchema
				if err := json.Unmarshal(temp.LiteralSchema, &multiSchema); err == nil {
					nsd.LiteralSchema = multiSchema
				} else {
					return fmt.Errorf("failed to unmarshal NestedSchemaDefinition.literalSchema")
				}
			}
		}
	} else {
		return fmt.Errorf("NestedSchemaDefinition must contain either 'fields' or 'type'")
	}

	return nil
}

// MarshalJSON implements the json.Marshaler interface for NestedSchemaDefinition.
func (nsd NestedSchemaDefinition) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)

	m["name"] = nsd.Name
	if nsd.Description != nil {
		m["description"] = *nsd.Description
	}
	if nsd.Indexes != nil {
		m["indexes"] = nsd.Indexes
	}
	if nsd.Metadata != nil {
		m["metadata"] = nsd.Metadata
	}
	if nsd.Concrete != nil {
		m["concrete"] = *nsd.Concrete
	}

	if nsd.isStructured {
		if nsd.StructuredFieldsMap != nil {
			m["fields"] = nsd.StructuredFieldsMap
		} else if nsd.StructuredFieldsArray != nil {
			m["fields"] = nsd.StructuredFieldsArray
		}
	} else {
		if nsd.Type != nil {
			m["type"] = *nsd.Type
		}
		if nsd.LiteralConstraints != nil {
			m["constraints"] = nsd.LiteralConstraints
		}
		if nsd.LiteralDefault != nil {
			m["default"] = nsd.LiteralDefault
		}
		if nsd.LiteralSchema != nil {
			m["schema"] = nsd.LiteralSchema
		}
		if nsd.LiteralItemsType != nil {
			m["itemsType"] = *nsd.LiteralItemsType
		}
	}

	return json.Marshal(m)
}

// SchemaDefinition defines a complete schema for a collection.
type SchemaDefinition struct {
	Name          string                             `json:"name"`
	Version       string                             `json:"version"`
	Description   *string                            `json:"description,omitempty"`
	Fields        map[string]*FieldDefinition        `json:"fields"`
	NestedSchemas map[string]*NestedSchemaDefinition `json:"nestedSchemas,omitempty"`
	Indexes       []IndexDefinition                  `json:"indexes,omitempty"`
	Constraints   SchemaConstraint[FieldType]        `json:"constraints,omitempty"`
	Metadata      map[string]any                     `json:"metadata,omitempty"`
	Migrations    []Migration                        `json:"migrations,omitempty"`
	Hint          *SchemaHint                        `json:"hint,omitempty"`
	Mock          func(faker any) (any, error)       `json:"-"`
}

// SchemaChangeType defines the type of change in a migration.
type SchemaChangeType string

// Supported schema change types.
const (
	SchemaChangeTypeModifyProperty     SchemaChangeType = "modifyProperty"
	SchemaChangeTypeAddField           SchemaChangeType = "addField"
	SchemaChangeTypeRemoveField        SchemaChangeType = "removeField"
	SchemaChangeTypeModifyField        SchemaChangeType = "modifyField"
	SchemaChangeTypeAddIndex           SchemaChangeType = "addIndex"
	SchemaChangeTypeRemoveIndex        SchemaChangeType = "removeIndex"
	SchemaChangeTypeModifyIndex        SchemaChangeType = "modifyIndex"
	SchemaChangeTypeAddConstraint      SchemaChangeType = "addConstraint"
	SchemaChangeTypeRemoveConstraint   SchemaChangeType = "removeConstraint"
	SchemaChangeTypeModifyConstraint   SchemaChangeType = "modifyConstraint"
	SchemaChangeTypeDeprecateField     SchemaChangeType = "deprecateField"
	SchemaChangeTypeAddNestedSchema    SchemaChangeType = "addNestedSchema"
	SchemaChangeTypeRemoveNestedSchema SchemaChangeType = "removeNestedSchema"
	SchemaChangeTypeModifyNestedSchema SchemaChangeType = "modifyNestedSchema"
)

// SchemaChangeModifyPropertyPayload is the payload for a ModifyProperty schema change.
type SchemaChangeModifyPropertyPayload struct {
	Name        *string        `json:"name,omitempty"`
	Version     *string        `json:"version,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Hint        *SchemaHint    `json:"hint,omitempty"`
}

// SchemaChangeAddFieldPayload is the payload for an AddField schema change.
type SchemaChangeAddFieldPayload struct {
	Definition FieldDefinition `json:"definition"`
}

// SchemaChangeModifyFieldPayload is the payload for a ModifyField schema change.
type SchemaChangeModifyFieldPayload struct {
	Changes             PartialFieldDefinition `json:"changes"`
	NestedSchemaChanges *struct {
		ID          *string                     `json:"id,omitempty"`
		Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
		Indexes     []IndexDefinition           `json:"indexes,omitempty"`
	} `json:"nestedSchemaChanges,omitempty"`
}

// SchemaChangeAddIndexPayload is the payload for an AddIndex schema change.
type SchemaChangeAddIndexPayload struct {
	Definition IndexDefinition `json:"definition"`
}

// SchemaChangeModifyIndexPayload is the payload for a ModifyIndex schema change.
type SchemaChangeModifyIndexPayload struct {
	Changes PartialIndexDefinition `json:"changes"`
}

// SchemaChangeAddConstraintPayload is the payload for an AddConstraint schema change.
type SchemaChangeAddConstraintPayload struct {
	Constraint SchemaConstraintRule[FieldType] `json:"constraint"`
}

// SchemaChangeModifyConstraintPayload is the payload for a ModifyConstraint schema change.
type SchemaChangeModifyConstraintPayload struct {
	Changes any `json:"changes"`
}

// SchemaChangeAddNestedSchemaPayload is the payload for an AddNestedSchema schema change.
type SchemaChangeAddNestedSchemaPayload struct {
	Definition NestedSchemaDefinition `json:"definition"`
}

// SchemaChangeModifyNestedSchemaPayload is the payload for a ModifyNestedSchema schema change.
type SchemaChangeModifyNestedSchemaPayload struct {
	Changes PartialNestedSchemaDefinition `json:"changes"`
}

// SchemaChange defines a single change to be made to a schema during a migration.
type SchemaChange struct {
	Type SchemaChangeType `json:"type"`

	ID *string `json:"id,omitempty"`

	*SchemaChangeModifyPropertyPayload
	*SchemaChangeAddFieldPayload
	*SchemaChangeModifyFieldPayload
	*SchemaChangeAddIndexPayload
	*SchemaChangeModifyIndexPayload
	*SchemaChangeAddConstraintPayload
	*SchemaChangeModifyConstraintPayload
	*SchemaChangeAddNestedSchemaPayload
	*SchemaChangeModifyNestedSchemaPayload
}

// UnmarshalJSON implements the json.Unmarshaler interface for SchemaChange.
func (sc *SchemaChange) UnmarshalJSON(data []byte) error {
	var common struct {
		Type SchemaChangeType `json:"type"`
		ID   *string          `json:"id"`
	}
	if err := json.Unmarshal(data, &common); err != nil {
		return err
	}

	sc.Type = common.Type
	sc.ID = common.ID

	switch sc.Type {
	case SchemaChangeTypeModifyProperty:
		sc.SchemaChangeModifyPropertyPayload = &SchemaChangeModifyPropertyPayload{}
		return json.Unmarshal(data, sc.SchemaChangeModifyPropertyPayload)
	case SchemaChangeTypeAddField:
		sc.SchemaChangeAddFieldPayload = &SchemaChangeAddFieldPayload{}
		return json.Unmarshal(data, sc.SchemaChangeAddFieldPayload)
	case SchemaChangeTypeRemoveField:
		return nil
	case SchemaChangeTypeModifyField:
		sc.SchemaChangeModifyFieldPayload = &SchemaChangeModifyFieldPayload{}
		return json.Unmarshal(data, sc.SchemaChangeModifyFieldPayload)
	case SchemaChangeTypeAddIndex:
		sc.SchemaChangeAddIndexPayload = &SchemaChangeAddIndexPayload{}
		return json.Unmarshal(data, sc.SchemaChangeAddIndexPayload)
	case SchemaChangeTypeRemoveIndex:
		return nil
	case SchemaChangeTypeModifyIndex:
		sc.SchemaChangeModifyIndexPayload = &SchemaChangeModifyIndexPayload{}
		return json.Unmarshal(data, sc.SchemaChangeModifyIndexPayload)
	case SchemaChangeTypeAddConstraint:
		sc.SchemaChangeAddConstraintPayload = &SchemaChangeAddConstraintPayload{}
		return json.Unmarshal(data, sc.SchemaChangeAddConstraintPayload)
	case SchemaChangeTypeRemoveConstraint:
		return nil
	case SchemaChangeTypeModifyConstraint:
		sc.SchemaChangeModifyConstraintPayload = &SchemaChangeModifyConstraintPayload{}
		return json.Unmarshal(data, sc.SchemaChangeModifyConstraintPayload)
	case SchemaChangeTypeAddNestedSchema:
		sc.SchemaChangeAddNestedSchemaPayload = &SchemaChangeAddNestedSchemaPayload{}
		return json.Unmarshal(data, sc.SchemaChangeAddNestedSchemaPayload)
	case SchemaChangeTypeRemoveNestedSchema:
		return nil
	case SchemaChangeTypeModifyNestedSchema:
		sc.SchemaChangeModifyNestedSchemaPayload = &SchemaChangeModifyNestedSchemaPayload{}
		return json.Unmarshal(data, sc.SchemaChangeModifyNestedSchemaPayload)
	default:
		return fmt.Errorf("unknown schema change type: %s", sc.Type)
	}
}

// MarshalJSON implements the json.Marshaler interface for SchemaChange.
func (sc SchemaChange) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	m["type"] = sc.Type
	if sc.ID != nil && *sc.ID != "" {
		m["id"] = *sc.ID
	}

	var payloadBytes []byte
	var err error

	switch sc.Type {
	case SchemaChangeTypeModifyProperty:
		if sc.SchemaChangeModifyPropertyPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifyPropertyPayload)
		}
	case SchemaChangeTypeAddField:
		if sc.SchemaChangeAddFieldPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeAddFieldPayload)
		}
	case SchemaChangeTypeModifyField:
		if sc.SchemaChangeModifyFieldPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifyFieldPayload)
		}
	case SchemaChangeTypeAddIndex:
		if sc.SchemaChangeAddIndexPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeAddIndexPayload)
		}
	case SchemaChangeTypeModifyIndex:
		if sc.SchemaChangeModifyIndexPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifyIndexPayload)
		}
	case SchemaChangeTypeAddConstraint:
		if sc.SchemaChangeAddConstraintPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeAddConstraintPayload)
		}
	case SchemaChangeTypeModifyConstraint:
		if sc.SchemaChangeModifyConstraintPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifyConstraintPayload)
		}
	case SchemaChangeTypeAddNestedSchema:
		if sc.SchemaChangeAddNestedSchemaPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeAddNestedSchemaPayload)
		}
	case SchemaChangeTypeModifyNestedSchema:
		if sc.SchemaChangeModifyNestedSchemaPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifyNestedSchemaPayload)
		}
	case SchemaChangeTypeRemoveField, SchemaChangeTypeRemoveIndex, SchemaChangeTypeRemoveConstraint, SchemaChangeTypeDeprecateField, SchemaChangeTypeRemoveNestedSchema:
		return json.Marshal(m)
	default:
		return json.Marshal(m)
	}

	if err != nil {
		return nil, err
	}

	if payloadBytes != nil {
		var payloadMap map[string]any
		if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
			return nil, err
		}
		maps.Copy(m, payloadMap)
	}

	return json.Marshal(m)
}

// PartialFieldDefinition represents a partial definition of a field, used for modifications.
type PartialFieldDefinition struct {
	Name        *string                     `json:"name,omitempty"`
	Type        *FieldType                  `json:"type,omitempty"`
	Required    *bool                       `json:"required,omitempty"`
	Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	Default     any                         `json:"default,omitempty"`
	Values      []any                       `json:"values,omitempty"`
	Schema      any                         `json:"schema,omitempty"`
	ItemsType   *FieldType                  `json:"itemsType,omitempty"`
	Deprecated  *bool                       `json:"deprecated,omitempty"`
	Description *string                     `json:"description,omitempty"`
	Unique      *bool                       `json:"unique,omitempty"`
	Hint        *struct {
		Input InputHint `json:"input"`
	} `json:"hint,omitempty"`
}

func (fd *PartialFieldDefinition) UnmarshalJSON(data []byte) error {
	type Alias PartialFieldDefinition // Create an alias to avoid infinite recursion

	// Unmarshal into a temporary struct to access the 'type' field and raw 'schema' field
	var temp struct {
		Type   FieldType       `json:"type"`
		Schema json.RawMessage `json:"schema,omitempty"`
		*Alias
	}

	temp.Alias = (*Alias)(fd) // Point Alias to the actual FieldDefinition
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the unmarshaled data back to the original FieldDefinition
	*fd = PartialFieldDefinition(*temp.Alias)

	// Now, handle the 'Schema' field based on its 'Type'
	if temp.Schema != nil {
		switch temp.Type {
		case FieldTypeObject, FieldTypeUnion:
			var singleSchema FieldSchema
			if err := json.Unmarshal(temp.Schema, &singleSchema); err == nil {
				fd.Schema = singleSchema
				return nil
			}
			var multiSchema []FieldSchema
			if err := json.Unmarshal(temp.Schema, &multiSchema); err == nil {
				fd.Schema = multiSchema
				return nil
			}
		case FieldTypeRecord:
			var recordSchema SchemaDefinition
			if err := json.Unmarshal(temp.Schema, &recordSchema); err == nil {
				fd.Schema = recordSchema
				return nil
			}
		}

		// For any other types or if specific unmarshaling failed,
		// unmarshal Schema into a generic any. This will likely be map[string]any for objects.
		var genericSchema any
		if err := json.Unmarshal(temp.Schema, &genericSchema); err != nil {
			return fmt.Errorf("failed to unmarshal FieldDefinition.Schema into expected types or generic any: %w", err)
		}
		fd.Schema = genericSchema
	}
	return nil
}

// PartialIndexDefinition represents a partial definition of an index, used for modifications.
type PartialIndexDefinition struct {
	Fields      []string               `json:"fields,omitempty"`
	Type        *IndexType             `json:"type,omitempty"`
	Unique      *bool                  `json:"unique,omitempty"`
	Partial     *PartialIndexCondition `json:"partial,omitempty"`
	Description *string                `json:"description,omitempty"`
	Order       *string                `json:"order,omitempty"`
	Name        *string                `json:"name,omitempty"`
}

// PartialNestedSchemaDefinition represents a partial definition of a nested schema, used for modifications.
type PartialNestedSchemaDefinition struct {
	Name               *string                     `json:"name,omitempty"`
	Description        *string                     `json:"description,omitempty"`
	Indexes            []IndexDefinition           `json:"indexes,omitempty"`
	Metadata           map[string]any              `json:"metadata,omitempty"`
	Concrete           *bool                       `json:"concrete,omitempty"`
	Fields             any                         `json:"fields,omitempty"`
	Type               *FieldType                  `json:"type,omitempty"`
	LiteralConstraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	LiteralDefault     any                         `json:"default,omitempty"`
	LiteralSchema      any                         `json:"schema,omitempty"`
	LiteralItemsType   *FieldType                  `json:"itemsType,omitempty"`
}

// TransformFunction defines a function for transforming data from one schema version to another.
type TransformFunction[Initial, Next any] func(data Initial) (Next, error)

// DataTransform represents a pair of transformations for bidirectional data migration.
type DataTransform[Initial, Next any] struct {
	Forward  TransformFunction[Initial, Next] `json:"-"`
	Backward TransformFunction[Next, Initial] `json:"-"`
}

// Migration defines a single migration, consisting of schema changes and data transformations.
type Migration struct {
	ID            string         `json:"id"`
	SchemaVersion string         `json:"schemaVersion"`
	Changes       []SchemaChange `json:"changes"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	Rollback      []SchemaChange `json:"rollback,omitempty"`
	Transform     string         `json:"transform"`
	CreatedAt     string         `json:"createdAt"`
	Checksum      string         `json:"checksum"`
}

// InputHint provides hints for UI generation or tooling.
type InputHint map[string]any

// SchemaHint provides hints for the schema as a whole.
type SchemaHint map[string]any

// Issue represents a validation or operational issue.
type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Path        string `json:"path,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
}

// ValidationResult represents the result of a validation operation.
type ValidationResult struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

// Document represents a single document or row of data.
type Document map[string]any

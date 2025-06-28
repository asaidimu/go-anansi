package schema

import (
	"encoding/json"
	"fmt"
	"maps"
)

// LogicalOperator for combining conditions.
type LogicalOperator string

const (
	LogicalAnd LogicalOperator = "and" // All conditions must be true
	LogicalOr  LogicalOperator = "or"  // At least one condition must be true
	LogicalNot LogicalOperator = "not" // Negates a condition or group of conditions
	LogicalNor LogicalOperator = "nor" // None of the conditions must be true
	LogicalXor LogicalOperator = "xor" // Exactly one of the conditions must be true
)

// FieldType represents the basic field types supported by the schema system.
type FieldType string

const (
	FieldTypeString  FieldType = "string"  // Text data
	FieldTypeNumber  FieldType = "number"  // Numeric data
	FieldTypeInteger FieldType = "integer" // Numeric data
	FieldTypeDecimal FieldType = "decimal" // Numeric data
	FieldTypeBoolean FieldType = "boolean" // True/false values
	FieldTypeArray   FieldType = "array"   // Ordered list of items
	FieldTypeSet     FieldType = "set"     // Unordered list with unique items
	FieldTypeEnum    FieldType = "enum"    // One out of a set of pre-defined items
	FieldTypeObject  FieldType = "object"  // Structured data with nested fields
	FieldTypeRecord  FieldType = "record"  // Unorganized key-value object, resolves to map[string]any
	FieldTypeUnion   FieldType = "union"   // One of a set of nested schemas, identified by schema IDs
)

// IndexType represents index types for optimizing different query patterns.
type IndexType string

const (
	IndexTypeNormal    IndexType = "normal"    // General-purpose index
	IndexTypeUnique    IndexType = "unique"    // Unique index
	IndexTypePrimary   IndexType = "primary"   // Primary key index (implies unique)
	IndexTypeSpatial   IndexType = "spatial"   // Index for geometric or geographical data
	IndexTypeFullText  IndexType = "fulltext"  // Full-text search index
)

// PredicateParameters is an interface that all predicate parameter types must satisfy.
// This allows for generic handling of different parameter types in the Predicate function.
// Concrete types implementing this might be simple Go primitives (string, int, bool)
// or the specialized structs defined below.
// Consumers are responsible for populating/asserting the correct type.
type PredicateParameters any

type PredicateParams[T any] struct {
    Data T
    Field *string // Pointer allows this to be nil (optional)
    Args PredicateParameters
}
// Predicate defines a predicate function for data validation.
// The 'data' parameter is generic, and 'field' is a string key (corresponds to keyof T).
// 'args' will be a PredicateParameters interface, requiring type assertion within
// the predicate implementation to access specific parameter structures.
type Predicate[T any] func(params PredicateParams[T]) bool

// PredicateMap is a map of predicate names to their validation functions.
// In Go, due to the varying generic type 'T' in Predicate, we use any
// to allow storage of different predicate instantiations. Type assertion will be needed
// when retrieving specific predicates.
type PredicateMap map[string]any

// FunctionMap is a map of function names to generic functions.
// Similar to PredicateMap, any is used for flexibility as Go's function types
// are not as flexible for generic arguments in maps without explicit type parameters.
type FunctionMap map[string]any

// PredicateName represents the names of supported predicates, derived from a PredicateMap.
// In Go, this typically maps to a string as map keys are strings.
type PredicateName string

type PredicateParam map[string]any

// ConstraintParameters is a deprecated type, use PredicateParameters instead.
// REMOVED: @deprecated Use PredicateParameters instead.
// type ConstraintParameters any

// ConstraintType represents the optional type of the constraint, e.g., "schema".
type ConstraintType string

const (
	ConstraintTypeSchema ConstraintType = "schema"
)

// Constraint defines a constraint on a field or schema, using a predicate for validation.
type Constraint[T FieldType] struct {
	// Type specifies the optional type of the constraint. For example, "schema".
	// It's a pointer to allow it to be omitted in JSON.
	Type *ConstraintType `json:"type,omitempty"`
	// Predicate is the name of the predicate function to use for validation.
	Predicate string `json:"predicate"`
	// Field is an optional field name that the predicate applies to within the data.
	// Corresponds to `keyof any` in TS, which is typically a string in this context.
	Field *string `json:"field,omitempty"`
	// Parameters are the arguments passed to the predicate. This can be various types.
	// Uses any to hold simple types or structs like PredicateFieldParam.
	// Consumers are responsible for populating/asserting the correct type.
	Parameters any `json:"parameters,omitempty"`
	// Name is the unique name of the constraint.
	Name string `json:"name"`
	// Description provides a brief explanation of the constraint.
	Description *string `json:"description,omitempty"`
	// ErrorMessage is the custom error message to display if the constraint fails.
	ErrorMessage *string `json:"errorMessage,omitempty"`
}

// IsSchemaConstraintRule is a marker method to satisfy the SchemaConstraintRule interface.
func (c Constraint[T]) IsSchemaConstraintRule() {}

// ConstraintGroup defines a group of multiple constraints with a logical operator.
type ConstraintGroup[T FieldType] struct {
	// Name is the unique name of the constraint group.
	Name string `json:"name"`
	// Operator specifies the logical operator (e.g., "and", "or") to apply to the rules.
	Operator LogicalOperator `json:"operator"`
	// Rules is an array of individual constraints or nested constraint groups.
	// Uses the SchemaConstraintRule interface for polymorphism.
	Rules []SchemaConstraintRule[T] `json:"rules"`
}

// IsSchemaConstraintRule is a marker method to satisfy the SchemaConstraintRule interface.
func (cg ConstraintGroup[T]) IsSchemaConstraintRule() {}

// SchemaConstraintRule is an interface that both Constraint and ConstraintGroup must implement
// to allow them to be used interchangeably in slices like SchemaConstraint.
type SchemaConstraintRule[T FieldType] interface {
	IsSchemaConstraintRule()
}

// SchemaConstraint represents a collection of constraints or groups applied at the schema or nested level.
// It is a slice of SchemaConstraintRule, enabling a mix of individual constraints and constraint groups.
type SchemaConstraint[T FieldType] []SchemaConstraintRule[T]

// FieldSchema defines a reference to a nested schema (mini-SchemaDefinition) with optional overrides.
type FieldSchema struct {
	// ID references a key in the parent SchemaDefinition's nestedSchemas map.
	ID string `json:"id"`
	// Constraints overrides or adds to nested schema constraints. Uses any for any type.
	Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	// Indexes overrides or adds to nested schema indexes.
	Indexes []IndexDefinition `json:"indexes,omitempty"`
}

// FieldDefinition defines a field within a schema, including its type, constraints, and nesting.
type FieldDefinition struct {
	Name string    `json:"name"`
	Type FieldType `json:"type"`
	// Required indicates if the field is mandatory.
	Required *bool `json:"required,omitempty"`
	// Constraints are an array of validation rules for this field.
	Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	// Default provides a default value for the field. Uses any for any type.
	Default any `json:"default,omitempty"`
	// Values specifies the allowed values for an 'enum' type field. Uses any for any type (string or number).
	Values []any `json:"values,omitempty"` // Can be string or number
	// Schema specifies the schema for 'union' or 'object' types. Can be a single FieldSchema or an array.
	// Consumers are responsible for populating/asserting the correct type.
	Schema any `json:"schema,omitempty"` // FieldSchema | []FieldSchema
	// ItemsType specifies the type of items in 'array' or 'set' fields.
	ItemsType *FieldType `json:"itemsType,omitempty"`
	// Deprecated marks the field as deprecated.
	Deprecated *bool `json:"deprecated,omitempty"`
	// Description provides a brief explanation of the field.
	Description *string `json:"description,omitempty"`
	// Unique indicates if the field must have unique values.
	Unique *bool `json:"unique,omitempty"`
	// Hint provides input hints for UI generation or tooling.
	Hint *struct {
		Input InputHint `json:"input"` // Assuming InputHint is a simple struct or map
	} `json:"hint,omitempty"`
}

// PartialIndexCondition defines a condition for partial indexes, allowing conditional indexing based on field values.
type PartialIndexCondition struct {
	Operator   LogicalOperator         `json:"operator"`
	Field      string                  `json:"field"`
	Value      any                     `json:"value,omitempty"` // Any type for value
	Conditions []PartialIndexCondition `json:"conditions,omitempty"`
}

// IndexDefinition defines an index for optimizing queries or enforcing uniqueness.
type IndexDefinition struct {
	Fields      []string               `json:"fields"`
	Type        IndexType              `json:"type"`
	Unique      *bool                  `json:"unique,omitempty"`
	Partial     *PartialIndexCondition `json:"partial,omitempty"`
	Description *string                `json:"description,omitempty"`
	Order       *string                `json:"order,omitempty"` // "asc" | "desc"
	Name        string                 `json:"name"`
}

// NestedSchemaDefinition represents a reusable nested schema structure.
// It can represent either a complex object with defined fields, or a direct primitive literal (string, number, boolean).
// The union type in TypeScript is handled by implementing custom UnmarshalJSON and MarshalJSON methods
// to enforce mutual exclusivity and proper JSON serialization/deserialization.
type NestedSchemaDefinition struct {
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Indexes     []IndexDefinition `json:"indexes,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Concrete    *bool             `json:"concrete,omitempty"`

	// These fields are for 'literal' nested schemas (when `Type` is present in TS).
	Type               *FieldType                  `json:"type,omitempty"` // "string" | "number" | "boolean" | "array" | "set" | "enum" | "record"
	LiteralConstraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
	LiteralDefault     any                         `json:"default,omitempty"`
	LiteralSchema      any                         `json:"schema,omitempty"` // FieldSchema | []FieldSchema
	LiteralItemsType   *FieldType                  `json:"itemsType,omitempty"`

	// These fields are for 'structured' nested schemas (when `Fields` is present in TS).
	// Using pointers allows them to be nil, which is correctly omitted by json:"omitempty".
	// The complex "fields" type from TS needs to be explicitly modeled.
	StructuredFieldsMap   map[string]*FieldDefinition `json:"fields,omitempty"` // Corresponds to Record<string, FieldDefinition<any>>
	StructuredFieldsArray []struct {
		Fields map[string]*FieldDefinition `json:"fields"`
		When   *struct {
			Field string `json:"field"`
			Value any    `json:"value"`
		} `json:"when,omitempty"`
	} `json:"fields,omitempty"` // Corresponds to Array<{ Fields: Record<string, FieldDefinition<any>>; When: ... }>

	// Internal flag to track which type of schema it is after unmarshaling
	isStructured bool
}

// UnmarshalJSON implements the json.Unmarshaler interface for NestedSchemaDefinition.
// It checks for the presence of "fields" or "type" to determine the schema type and unmarshals accordingly.
func (nsd *NestedSchemaDefinition) UnmarshalJSON(data []byte) error {
	// Use an anonymous struct to unmarshal all possible JSON fields initially
	var temp struct {
		Name        string            `json:"name"`
		Description *string           `json:"description"`
		Indexes     []IndexDefinition `json:"indexes"`
		Metadata    map[string]any    `json:"metadata"`
		Concrete    *bool             `json:"concrete"`

		// For literal schemas
		Type               *FieldType                  `json:"type"`
		LiteralConstraints SchemaConstraint[FieldType] `json:"constraints"`
		LiteralDefault     any                         `json:"default"`
		LiteralSchema      json.RawMessage             `json:"schema"` // Read as RawMessage to determine array or object later
		LiteralItemsType   *FieldType                  `json:"itemsType"`

		// For structured schemas
		Fields json.RawMessage `json:"fields"` // Read fields as raw JSON
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Assign common fields
	nsd.Name = temp.Name
	nsd.Description = temp.Description
	nsd.Indexes = temp.Indexes
	nsd.Metadata = temp.Metadata
	nsd.Concrete = temp.Concrete

	// Determine if it's a structured schema (has "fields") or literal (has "type")
	hasFields := temp.Fields != nil
	hasType := temp.Type != nil

	if hasFields && hasType {
		return fmt.Errorf("NestedSchemaDefinition cannot have both 'fields' and 'type' (mutual exclusivity violation)")
	}

	if hasFields {
		nsd.isStructured = true
		// Unmarshal 'Fields' based on its content (map or array)
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
				return fmt.Errorf("failed to unmarshal NestedSchemaDefinition.fields: neither map nor array")
			}
		}
	} else if hasType {
		nsd.isStructured = false
		// Literal schema fields
		nsd.Type = temp.Type
		nsd.LiteralConstraints = temp.LiteralConstraints
		nsd.LiteralDefault = temp.LiteralDefault
		nsd.LiteralItemsType = temp.LiteralItemsType

		// Unmarshal LiteralSchema which can be FieldSchema or []FieldSchema
		if temp.LiteralSchema != nil {
			var singleSchema FieldSchema
			if err := json.Unmarshal(temp.LiteralSchema, &singleSchema); err == nil {
				nsd.LiteralSchema = singleSchema
			} else {
				var multiSchema []FieldSchema
				if err := json.Unmarshal(temp.LiteralSchema, &multiSchema); err == nil {
					nsd.LiteralSchema = multiSchema
				} else {
					return fmt.Errorf("failed to unmarshal NestedSchemaDefinition.literalSchema: neither single object nor array")
				}
			}
		}
	} else {
		return fmt.Errorf("NestedSchemaDefinition must contain either 'fields' or 'type'")
	}

	return nil
}

// MarshalJSON implements the json.Marshaler interface for NestedSchemaDefinition.
// It marshals based on whether the schema is structured or literal,
// ensuring only relevant fields are included in the output JSON.
func (nsd NestedSchemaDefinition) MarshalJSON() ([]byte, error) {
	// Create a map to build the output JSON
	m := make(map[string]any)

	// Marshal common fields
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
		// Marshal structured fields
		if nsd.StructuredFieldsMap != nil {
			m["fields"] = nsd.StructuredFieldsMap
		} else if nsd.StructuredFieldsArray != nil {
			m["fields"] = nsd.StructuredFieldsArray
		}
	} else { // It's a literal schema
		// Marshal literal schema fields
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

// SchemaDefinition defines a complete schema, intended as an atomic unit within a larger domain model.
type SchemaDefinition struct {
	Name        string                           `json:"name"`
	Version     string                           `json:"version"`
	Description *string                          `json:"description,omitempty"`
	Fields      map[string]*FieldDefinition `json:"fields"` // Map of field names to FieldDefinition
	// Reusable nested schema definitions, now as mini-SchemaDefinitions.
	NestedSchemas map[string]*NestedSchemaDefinition `json:"nestedSchemas,omitempty"`
	Indexes       []IndexDefinition                  `json:"indexes,omitempty"`
	Constraints   SchemaConstraint[FieldType]        `json:"constraints,omitempty"`
	Metadata      map[string]any                     `json:"metadata,omitempty"`
	Migrations    []Migration[any]                   `json:"migrations,omitempty"`
	Hint          *SchemaHint                        `json:"hint,omitempty"` // Assuming SchemaHint is a struct or map
	// Mock is a function to generate mock data. In Go, this would be a function signature.
	// The `faker` dependency would need to be brought in or mocked.
	Mock func(faker any) (any, error) `json:"-"` // func(faker *faker.Faker) (any, error)
}

// SchemaChangeType defines the type of change in a migration.
type SchemaChangeType string

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

// Specific payload structs for each SchemaChangeType
// These mirror the specific object shapes for each change.

// SchemaChangeModifyPropertyPayload for SchemaChangeTypeModifyProperty
type SchemaChangeModifyPropertyPayload struct {
	Name        *string        `json:"name,omitempty"`
	Version     *string        `json:"version,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Hint        *SchemaHint    `json:"hint,omitempty"`
}

// SchemaChangeAddFieldPayload for SchemaChangeTypeAddField
type SchemaChangeAddFieldPayload struct {
	Definition FieldDefinition `json:"definition"`
}

// SchemaChangeModifyFieldPayload for SchemaChangeTypeModifyField
type SchemaChangeModifyFieldPayload[T any] struct {
	Changes             PartialFieldDefinition[T] `json:"changes"`
	NestedSchemaChanges *struct {
		ID          *string                     `json:"id,omitempty"`
		Constraints SchemaConstraint[FieldType] `json:"constraints,omitempty"`
		Indexes     []IndexDefinition           `json:"indexes,omitempty"`
	} `json:"nestedSchemaChanges,omitempty"`
}

// SchemaChangeAddIndexPayload for SchemaChangeTypeAddIndex
type SchemaChangeAddIndexPayload struct {
	Definition IndexDefinition `json:"definition"`
}

// SchemaChangeModifyIndexPayload for SchemaChangeTypeModifyIndex
type SchemaChangeModifyIndexPayload struct {
	Changes PartialIndexDefinition `json:"changes"`
}

// SchemaChangeAddConstraintPayload for SchemaChangeTypeAddConstraint
type SchemaChangeAddConstraintPayload struct {
	Constraint SchemaConstraintRule[FieldType] `json:"constraint"`
}

// SchemaChangeModifyConstraintPayload for SchemaChangeTypeModifyConstraint
type SchemaChangeModifyConstraintPayload struct {
	Changes any `json:"changes"` // Partial<SchemaConstraint<any> | Constraint<any>>
}

// SchemaChangeAddNestedSchemaPayload for SchemaChangeTypeAddNestedSchema
type SchemaChangeAddNestedSchemaPayload struct {
	Definition NestedSchemaDefinition `json:"definition"`
}

// SchemaChangeModifyNestedSchemaPayload for SchemaChangeTypeModifyNestedSchema
type SchemaChangeModifyNestedSchemaPayload struct {
	Changes PartialNestedSchemaDefinition `json:"changes"`
}

// SchemaChange defines a change that can be made to a schema during migration.
// This struct handles the discriminated union from TypeScript using a common 'Type' field
// and embedding specific payload structs for each change type.
type SchemaChange[T any] struct {
	Type SchemaChangeType `json:"type"` // The discriminator field

	// Common ID field used for several change types (e.g., removeField, modifyField)
	ID *string `json:"id,omitempty"` // Pointer for optionality and omitempty

	// Specific payloads for each change type, embedded or as pointers.
	// Only one of these should be meaningfully populated based on 'Type'.
	// json:"inline" means fields of the embedded struct are marshaled at the same level.
	// We'll combine this with custom MarshalJSON to ensure mutual exclusivity.

	// ModifyProperty specific fields
	*SchemaChangeModifyPropertyPayload

	// AddField specific fields
	*SchemaChangeAddFieldPayload

	// ModifyField specific fields (ID already handled by common ID field)
	*SchemaChangeModifyFieldPayload[T]

	// AddIndex specific fields
	*SchemaChangeAddIndexPayload

	// ModifyIndex specific fields (Name already handled by common ID field)
	*SchemaChangeModifyIndexPayload

	// AddConstraint specific fields
	*SchemaChangeAddConstraintPayload

	// ModifyConstraint specific fields (Name already handled by common ID field)
	*SchemaChangeModifyConstraintPayload

	// AddNestedSchema specific fields
	*SchemaChangeAddNestedSchemaPayload

	// ModifyNestedSchema specific fields (ID already handled by common ID field)
	*SchemaChangeModifyNestedSchemaPayload
}

// UnmarshalJSON implements the json.Unmarshaler interface for SchemaChange.
// It reads the 'type' field and then unmarshals the remaining JSON data
// into the appropriate specific payload struct.
func (sc *SchemaChange[T]) UnmarshalJSON(data []byte) error {
	// First, unmarshal into a temporary struct to get the 'type' and 'id'
	var common struct {
		Type SchemaChangeType `json:"type"`
		ID   *string          `json:"id"`
	}
	if err := json.Unmarshal(data, &common); err != nil {
		return err
	}

	sc.Type = common.Type
	sc.ID = common.ID

	// Now, based on the changeType, unmarshal the rest of the data
	// into the correct specific payload field.
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
		sc.SchemaChangeModifyFieldPayload = &SchemaChangeModifyFieldPayload[T]{}
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
// It marshals only the common fields ('type', 'id') and the relevant
// specific payload based on the 'type' field.
func (sc SchemaChange[T]) MarshalJSON() ([]byte, error) {
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

	// Merge payload fields into the main map
	if payloadBytes != nil {
		var payloadMap map[string]any
		if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
			return nil, err
		}
		maps.Copy(m, payloadMap)
	}

	return json.Marshal(m)
}

type PartialFieldDefinition[T any] struct {
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

type PartialIndexDefinition struct {
	Fields      []string               `json:"fields,omitempty"`
	Type        *IndexType             `json:"type,omitempty"`
	Unique      *bool                  `json:"unique,omitempty"`
	Partial     *PartialIndexCondition `json:"partial,omitempty"`
	Description *string                `json:"description,omitempty"`
	Order       *string                `json:"order,omitempty"`
	Name        *string                `json:"name,omitempty"`
}

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

// TransformFunction defines a transform function for data migration between schema versions.
type TransformFunction[Initial, Next any] func(data Initial) (Next, error)

// DataTransform represents a pair of transformations for bidirectional data migration.
type DataTransform[Initial, Next any] struct {
	Forward  TransformFunction[Initial, Next] `json:"-"`
	Backward TransformFunction[Next, Initial] `json:"-"`
}

// Migration defines a migration, consisting of schema changes and data transforms.
type Migration[T any] struct {
	ID            string            `json:"id"`
	SchemaVersion string            `json:"schemaVersion"`
	Changes       []SchemaChange[T] `json:"changes"`
	Description   string            `json:"description"`
	Status        string            `json:"status"`
	Rollback      []SchemaChange[T] `json:"rollback,omitempty"`
	Transform     any               `json:"transform"`
	CreatedAt     string            `json:"createdAt"`
	Checksum      string            `json:"checksum"`
}

type InputHint map[string]any
type SchemaHint map[string]any

// Issue represents a validation or operational issue.
type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Path        string `json:"path,omitempty"`
	Severity    string `json:"severity,omitempty"` // e.g., "error", "warning"
	Description string `json:"description,omitempty"`
}

type ValidationResult struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

type Document map[string]any

// This file provides the foundational types and structures for defining data schemas.
// It includes definitions for fields, constraints, indexes, and migrations, forming a
// comprehensive framework for data modeling and validation.
package schema

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// VersionFieldName is the reserved name for the field used in optimistic concurrency control.
const VersionFieldName = "_version_"

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
	FieldTypeUnknown FieldType = "unknown"
	FieldTypeDynamic FieldType = "dynamic" // Deprecated: Use FieldTypeRecord instead
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
	Data   T                   // The data being validated.
	Field  *string             // The specific field being validated.
	Fields []string            // The specific field being validated.
	Args   PredicateParameters // The arguments for the predicate.
}

// Predicate defines a function for data validation.
type Predicate[T any] func(params PredicateParams[T]) bool

// PredicateMap is a map of predicate names to their validation functions.
type PredicateMap map[string]any

// FunctionMap is a map of function names to generic functions.
type FunctionMap map[string]any

// PredicateName represents the name of a supported predicate.
type PredicateName string

// ConstraintType represents the type of a constraint.
type ConstraintType string

// Supported constraint types.
const (
	ConstraintTypeSchema ConstraintType = "schema"
)

// Constraint defines a validation rule for a field or schema.
type Constraint struct {
	Type         *ConstraintType `json:"type,omitempty"`
	Predicate    string          `json:"predicate"`
	Field        *string         `json:"field,omitempty"`
	Fields       []string        `json:"fields,omitempty"`
	Parameters   any             `json:"parameters,omitempty"`
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	ErrorMessage *string         `json:"errorMessage,omitempty"`
}

// ConstraintGroup defines a group of multiple constraints with a logical operator.
type ConstraintGroup struct {
	Name     string                 `json:"name"`
	Operator common.LogicalOperator `json:"operator"`
	Rules    []ConstraintRule    `json:"rules"`
}

// ResourceReference defines a reference to a component in the registry.
type ResourceReference struct {
	ID string `json:"id"`
}

// ConstraintRule represents a discriminated union of Constraint, ConstraintGroup, or ResourceReference.
// Exactly one field should be non-nil at any time.
type ConstraintRule struct {
	Constraint      *Constraint
	ConstraintGroup *ConstraintGroup

	// TODO: put into use
	Reference *ResourceReference
}

// UnmarshalJSON implements custom unmarshaling for ConstraintRule.
func (cr *ConstraintRule) UnmarshalJSON(data []byte) error {
	// Try ResourceReference first (simplest structure)
	var ref ResourceReference
	if err := json.Unmarshal(data, &ref); err == nil && ref.ID != "" {
		cr.Reference = &ref
		return nil
	}

	// Try ConstraintGroup (has operator field)
	var temp struct {
		Operator *common.LogicalOperator `json:"operator"`
	}
	if err := json.Unmarshal(data, &temp); err == nil && temp.Operator != nil {
		var group ConstraintGroup
		if err := json.Unmarshal(data, &group); err == nil {
			cr.ConstraintGroup = &group
			return nil
		}
	}

	// Default to Constraint
	var constraint Constraint
	if err := json.Unmarshal(data, &constraint); err != nil {
		return err
	}
	cr.Constraint = &constraint
	return nil
}

// MarshalJSON implements custom marshaling for ConstraintRule.
func (cr ConstraintRule) MarshalJSON() ([]byte, error) {
	if cr.Reference != nil {
		return json.Marshal(cr.Reference)
	}
	if cr.ConstraintGroup != nil {
		return json.Marshal(cr.ConstraintGroup)
	}
	if cr.Constraint != nil {
		return json.Marshal(cr.Constraint)
	}
	return nil, fmt.Errorf("ConstraintRule has no active variant")
}


// SchemaConstraint represents a collection of constraint rules.
type SchemaConstraint []ConstraintRule

// IndexOrReference represents a discriminated union of IndexDefinition or ResourceReference.
// Exactly one field should be non-nil at any time.
type IndexOrReference struct {
	Index *IndexDefinition

	// TODO: put into use
	Reference *ResourceReference
}

// UnmarshalJSON implements custom unmarshaling for IndexOrReference.
func (ior *IndexOrReference) UnmarshalJSON(data []byte) error {
	// Try ResourceReference first
	var ref ResourceReference
	if err := json.Unmarshal(data, &ref); err == nil && ref.ID != "" {
		ior.Reference = &ref
		return nil
	}

	// Default to IndexDefinition
	var index IndexDefinition
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}
	ior.Index = &index
	return nil
}

// MarshalJSON implements custom marshaling for IndexOrReference.
func (ior IndexOrReference) MarshalJSON() ([]byte, error) {
	if ior.Reference != nil {
		return json.Marshal(ior.Reference)
	}
	if ior.Index != nil {
		return json.Marshal(ior.Index)
	}
	return nil, fmt.Errorf("IndexOrReference has no active variant")
}

// NestedSchemaReference defines a reference to a nested schema.
type NestedSchemaReference struct {
	ID          string                      `json:"id"`
	Constraints SchemaConstraint`json:"constraints,omitempty"`
	Indexes     []IndexOrReference          `json:"indexes,omitempty"`
}

// FieldDefinition defines a field within a schema.
type FieldDefinition struct {
	Name        string                      `json:"name"`
	Type        FieldType                   `json:"type"`
	Required    *bool                       `json:"required,omitempty"`
	Constraints SchemaConstraint `json:"constraints,omitempty"`
	Default     any                         `json:"default,omitempty"`
	Values      []any                       `json:"values,omitempty"`
	Schema      any                         `json:"schema,omitempty"` // should be nested schema reference
	ItemsType   *FieldType                  `json:"itemsType,omitempty"`
	Deprecated  *bool                       `json:"deprecated,omitempty"`
	Description *string                     `json:"description,omitempty"`
	Unique      *bool                       `json:"unique,omitempty"`
	Hint        *struct {
		Input InputHint `json:"input"`
	} `json:"hint,omitempty"`
}

func (fd *FieldDefinition) UnmarshalJSON(data []byte) error {
	type Alias FieldDefinition

	var temp struct {
		Type   FieldType       `json:"type"`
		Schema json.RawMessage `json:"schema,omitempty"`
		*Alias
	}

	temp.Alias = (*Alias)(fd)
	if err := utils.FromJSON(data, &temp); err != nil {
		return err
	}

	*fd = FieldDefinition(*temp.Alias)
	fd.Type = temp.Type

	// Normalize deprecated "dynamic" to "record"
	if fd.Type == FieldTypeDynamic {
		fd.Type = FieldTypeRecord
	}

	if temp.Schema != nil {
		handled := false
		switch temp.Type {
		case FieldTypeObject, FieldTypeArray, FieldTypeRecord, FieldTypeDynamic:
			var singleSchema NestedSchemaReference
			if err := utils.FromJSON(temp.Schema, &singleSchema); err == nil {
				if singleSchema.ID != "" {
					fd.Schema = singleSchema
					handled = true
				}
			}
		case FieldTypeUnion:
			var multiSchema []NestedSchemaReference
			if err := utils.FromJSON(temp.Schema, &multiSchema); err == nil {
				fd.Schema = multiSchema
				handled = true
			}
		}

		if !handled {
			if temp.Type != FieldTypeObject && temp.Type != FieldTypeArray && temp.Type != FieldTypeRecord && temp.Type != FieldTypeDynamic && temp.Type != FieldTypeUnion {
				return ErrFieldTypeCannotHaveSchemaReference.WithOperation("schema.FieldDefinition.UnmarshalJSON").
					WithMessage(fmt.Sprintf("field of type '%s' cannot have a 'schema' reference", temp.Type))
			}
			var genericSchema any
			if err := utils.FromJSON(temp.Schema, &genericSchema); err != nil {
				return ErrFailedToUnmarshalSchema.WithCause(err).WithOperation("schema.FieldDefinition.UnmarshalJSON")
			}
			fd.Schema = genericSchema
		}
	}

	if fd.ItemsType != nil && fd.Type != FieldTypeArray && fd.Type != FieldTypeSet {
		return ErrFieldTypeCannotHaveItemsType.WithOperation("schema.FieldDefinition.UnmarshalJSON").
			WithMessage(fmt.Sprintf("field of type '%s' cannot have an 'itemsType'", fd.Type))
	}
	return nil
}

// PartialIndexCondition defines a condition for a partial index.
type PartialIndexCondition struct {
	Operator   common.LogicalOperator  `json:"operator"`
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

type FieldInclusionCondition struct {
	Field string `json:"field"`
	Value any    `json:"value"`
}

// ConditionalFieldSet represents a set of fields that apply when a condition is met.
type ConditionalFieldSet struct {
	Fields map[string]*FieldDefinition `json:"fields"`
	When   *FieldInclusionCondition    `json:"when,omitempty"`
}

// NestedSchemaFields represents the discriminated union of field definitions.
// Either FieldsMap or FieldsArray should be set, but not both.
type NestedSchemaFields struct {
	FieldsMap   map[string]*FieldDefinition
	FieldsArray []ConditionalFieldSet
}

// UnmarshalJSON implements custom unmarshaling for NestedSchemaFields.
func (nsf *NestedSchemaFields) UnmarshalJSON(data []byte) error {
	// Try map form first
	var fieldsMap map[string]*FieldDefinition
	if err := json.Unmarshal(data, &fieldsMap); err == nil {
		nsf.FieldsMap = fieldsMap
		return nil
	}

	// Try array form
	var fieldsArray []ConditionalFieldSet
	if err := json.Unmarshal(data, &fieldsArray); err == nil {
		nsf.FieldsArray = fieldsArray
		return nil
	}

	return fmt.Errorf("failed to unmarshal NestedSchemaFields: must be map or array")
}

// MarshalJSON implements custom marshaling for NestedSchemaFields.
func (nsf NestedSchemaFields) MarshalJSON() ([]byte, error) {
	if nsf.FieldsMap != nil {
		return json.Marshal(nsf.FieldsMap)
	}
	if nsf.FieldsArray != nil {
		return json.Marshal(nsf.FieldsArray)
	}
	return nil, fmt.Errorf("NestedSchemaFields has no active variant")
}

// NestedSchemaDefinition represents a reusable, nested schema structure.
// This is a discriminated union: either it has Fields (structured) or Type (primitive/typed).
type NestedSchemaDefinition struct {
	ID          *string        `json:"id,omitempty"`
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Concrete    *bool          `json:"concrete,omitempty"`

	Indexes     []IndexOrReference          `json:"indexes,omitempty"`
	Constraints SchemaConstraint `json:"constraints,omitempty"`

	// Structured variant (has fields)
	Fields *NestedSchemaFields `json:"fields,omitempty"`

	// Typed variant (primitive or has type)
	Type      *FieldType `json:"type,omitempty"`
	Default   any        `json:"default,omitempty"`
	Schema    any        `json:"schema,omitempty"`
	ItemsType *FieldType `json:"itemsType,omitempty"`
}

func (nsd *NestedSchemaDefinition) UnmarshalJSON(data []byte) error {
	type Alias NestedSchemaDefinition
	var temp struct {
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		Metadata    map[string]any  `json:"metadata"`
		Concrete    *bool           `json:"concrete"`
		Indexes     json.RawMessage `json:"indexes"`
		Constraints json.RawMessage `json:"constraints"`
		Type        *FieldType      `json:"type"`
		Default     any             `json:"default"`
		Schema      json.RawMessage `json:"schema"`
		ItemsType   *FieldType      `json:"itemsType"`
		Fields      json.RawMessage `json:"fields"`
	}

	if err := utils.FromJSON(data, &temp); err != nil {
		return err
	}

	nsd.Name = temp.Name
	nsd.Description = temp.Description
	nsd.Metadata = temp.Metadata
	nsd.Concrete = temp.Concrete

	// Unmarshal indexes
	if temp.Indexes != nil {
		var indexes []IndexOrReference
		if err := json.Unmarshal(temp.Indexes, &indexes); err != nil {
			return err
		}
		nsd.Indexes = indexes
	}

	// Unmarshal constraints
	if temp.Constraints != nil {
		var constraints SchemaConstraint
		if err := json.Unmarshal(temp.Constraints, &constraints); err != nil {
			return err
		}
		nsd.Constraints = constraints
	}

	hasFields := temp.Fields != nil
	hasType := temp.Type != nil

	if hasFields && hasType {
		return ErrNestedSchemaDefCannotHaveBothFieldsAndType.WithOperation("schema.NestedSchemaDefinition.UnmarshalJSON")
	}

	if hasFields {
		var fields NestedSchemaFields
		if err := json.Unmarshal(temp.Fields, &fields); err != nil {
			return ErrFailedToUnmarshalNestedSchemaDefFields.WithCause(err).WithOperation("schema.NestedSchemaDefinition.UnmarshalJSON")
		}
		nsd.Fields = &fields
	} else if hasType {
		nsd.Type = temp.Type

		// Normalize deprecated "dynamic" to "record"
		if *nsd.Type == FieldTypeDynamic {
			*nsd.Type = FieldTypeRecord
		}

		nsd.Default = temp.Default
		nsd.ItemsType = temp.ItemsType

		// For record/dynamic types, we don't need schema
		if *temp.Type == FieldTypeRecord || *temp.Type == FieldTypeDynamic {
			return nil
		}

		if temp.Schema != nil {
			var singleSchema NestedSchemaReference
			if err := utils.FromJSON(temp.Schema, &singleSchema); err == nil {
				nsd.Schema = singleSchema
			} else {
				var multiSchema []NestedSchemaReference
				if err := utils.FromJSON(temp.Schema, &multiSchema); err == nil {
					nsd.Schema = multiSchema
				} else {
					return ErrFailedToUnmarshalNestedSchemaDefSchema.WithCause(err).WithOperation("schema.NestedSchemaDefinition.UnmarshalJSON")
				}
			}
		}
	} else {
		return ErrNestedSchemaMissingFieldsOrType.WithOperation("schema.NestedSchemaDefinition.UnmarshalJSON").
			WithMessage(fmt.Sprintf("NestedSchemaDefinition must contain either 'fields' or 'type' for '%s'", temp.Name))
	}

	return nil
}

func (nsd NestedSchemaDefinition) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)

	m["name"] = nsd.Name
	if nsd.Description != nil {
		m["description"] = *nsd.Description
	}
	if nsd.Metadata != nil {
		m["metadata"] = nsd.Metadata
	}
	if nsd.Concrete != nil {
		m["concrete"] = *nsd.Concrete
	}
	if len(nsd.Indexes) > 0 {
		m["indexes"] = nsd.Indexes
	}
	if len(nsd.Constraints) > 0 {
		m["constraints"] = nsd.Constraints
	}

	// Marshal based on which variant is active
	if nsd.Fields != nil {
		m["fields"] = nsd.Fields
	} else if nsd.Type != nil {
		m["type"] = *nsd.Type
		if nsd.Default != nil {
			m["default"] = nsd.Default
		}
		if nsd.Schema != nil {
			m["schema"] = nsd.Schema
		}
		if nsd.ItemsType != nil {
			m["itemsType"] = *nsd.ItemsType
		}
	}

	return json.Marshal(m)
}

// ConstraintOrGroup represents a discriminated union for registry constraints.
// Exactly one field should be non-nil at any time.
type ConstraintOrGroup struct {
	Constraint      *Constraint
	ConstraintGroup *ConstraintGroup
}

// UnmarshalJSON implements custom unmarshaling for ConstraintOrGroup.
func (cog *ConstraintOrGroup) UnmarshalJSON(data []byte) error {
	// Try ConstraintGroup first (has operator field)
	var temp struct {
		Operator *common.LogicalOperator `json:"operator"`
	}
	if err := json.Unmarshal(data, &temp); err == nil && temp.Operator != nil {
		var group ConstraintGroup
		if err := json.Unmarshal(data, &group); err == nil {
			cog.ConstraintGroup = &group
			return nil
		}
	}

	// Default to Constraint
	var constraint Constraint
	if err := json.Unmarshal(data, &constraint); err != nil {
		return err
	}
	cog.Constraint = &constraint
	return nil
}

// MarshalJSON implements custom marshaling for ConstraintOrGroup.
func (cog ConstraintOrGroup) MarshalJSON() ([]byte, error) {
	if cog.ConstraintGroup != nil {
		return json.Marshal(cog.ConstraintGroup)
	}
	if cog.Constraint != nil {
		return json.Marshal(cog.Constraint)
	}
	return nil, fmt.Errorf("ConstraintOrGroup has no active variant")
}

// Registry contains reusable schema components.
type Registry struct {
	Schemas     map[string]*NestedSchemaDefinition `json:"schemas,omitempty"`
	Constraints map[string]*ConstraintOrGroup      `json:"constraints,omitempty"`
	Indexes     map[string]*IndexDefinition        `json:"indexes,omitempty"`
}

// SchemaDefinition defines a complete schema for a collection.
type SchemaDefinition struct {
	Name        string                      `json:"name"`
	Description *string                     `json:"description,omitempty"`
	Version     string                      `json:"version"`
	Fields      map[string]*FieldDefinition `json:"fields,omitempty"`
	// Registry      *Registry
	// `json:"registry,omitempty"` TODO IMPLEMENT LATER
	Indexes       []IndexOrReference                 `json:"indexes,omitempty"`
	Migrations    []Migration                        `json:"migrations,omitempty"`
	Constraints   SchemaConstraint        `json:"constraints,omitempty"`
	Hint          *SchemaHint                        `json:"hint,omitempty"`
	NestedSchemas map[string]*NestedSchemaDefinition `json:"nestedSchemas,omitempty"` // Deprecated: Use Registry.Schemas instead

	Metadata map[string]any               `json:"metadata,omitempty"`
	Mock     func(faker any) (any, error) `json:"-"`
}

// SchemaChangeType defines the type of change in a migration.
type SchemaChangeType string

// Supported schema change types.
const (
	SchemaChangeTypeModifyProperty        SchemaChangeType = "modifyProperty"
	SchemaChangeTypeAddField              SchemaChangeType = "addField"
	SchemaChangeTypeRemoveField           SchemaChangeType = "removeField"
	SchemaChangeTypeModifyField           SchemaChangeType = "modifyField"
	SchemaChangeTypeAddIndex              SchemaChangeType = "addIndex"
	SchemaChangeTypeRemoveIndex           SchemaChangeType = "removeIndex"
	SchemaChangeTypeModifyIndex           SchemaChangeType = "modifyIndex"
	SchemaChangeTypeAddConstraint         SchemaChangeType = "addConstraint"
	SchemaChangeTypeRemoveConstraint      SchemaChangeType = "removeConstraint"
	SchemaChangeTypeModifyConstraint      SchemaChangeType = "modifyConstraint"
	SchemaChangeTypeAddSchema             SchemaChangeType = "addSchema"
	SchemaChangeTypeRemoveSchema          SchemaChangeType = "removeSchema"
	SchemaChangeTypeModifySchema          SchemaChangeType = "modifySchema"
	SchemaChangeTypeModifySchemaReference SchemaChangeType = "modifySchemaReference"
)

// SchemaChangeModifyPropertyPayload is the payload for a ModifyProperty schema change.
type SchemaChangeModifyPropertyPayload struct {
	Value any `json:"value"`
}

// SchemaChangeAddFieldPayload is the payload for an AddField schema change.
type SchemaChangeAddFieldPayload struct {
	Definition FieldDefinition `json:"definition"`
}

// SchemaChangeModifySchemaReferencePayload is the payload for SchemaChangeTypeModifySchemaReference.
// It applies a list of SchemaChange objects to a NestedSchemaReference within a FieldDefinition.
type SchemaChangeModifySchemaReferencePayload struct {
	Field   string         `json:"field,omitempty"`
	ID      *string        `json:"id,omitempty"` // Optional: If FieldDefinition.Schema is a list, identifies the specific NestedSchemaReference to modify.
	Changes []SchemaChange `json:"changes"`      // Changes to apply to the NestedSchemaReference's properties (constraints, indexes)
}

// SchemaChangeModifyFieldPayload is the payload for a ModifyField schema change.
type SchemaChangeModifyFieldPayload struct {
	Changes PartialFieldDefinition `json:"changes"`
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
	Constraint ConstraintRule `json:"constraint"`
}

// SchemaChangeModifyConstraintPayload is the payload for a ModifyConstraint schema change.
type SchemaChangeModifyConstraintPayload struct {
	Changes PartialConstraint `json:"changes"`
}

// SchemaChangeAddSchemaPayload is the payload for an AddSchema schema change.
type SchemaChangeAddSchemaPayload struct {
	Definition NestedSchemaDefinition `json:"definition"`
}

// SchemaChangeModifySchemaPayload is the payload for a ModifySchema schema change.
type SchemaChangeModifySchemaPayload struct {
	Changes []SchemaChange `json:"changes"`
}

// SchemaChange defines a single change to be made to a schema during a migration.
type SchemaChange struct {
	ID   *string          `json:"id,omitempty"`
	Type SchemaChangeType `json:"type"`
	Name *string          `json:"name,omitempty"` // For removeIndex, removeConstraint

	*SchemaChangeModifyPropertyPayload
	*SchemaChangeAddFieldPayload
	*SchemaChangeModifyFieldPayload
	*SchemaChangeAddIndexPayload
	*SchemaChangeModifyIndexPayload
	*SchemaChangeAddConstraintPayload
	*SchemaChangeModifyConstraintPayload
	*SchemaChangeAddSchemaPayload
	*SchemaChangeModifySchemaPayload
	*SchemaChangeModifySchemaReferencePayload
}

func (sc *SchemaChange) UnmarshalJSON(data []byte) error {
	var tempCommon struct {
		Type SchemaChangeType `json:"type"`
		ID   *string          `json:"id"`
		Name *string          `json:"name"`
	}
	if err := utils.FromJSON(data, &tempCommon); err != nil {
		return err
	}

	sc.Type = tempCommon.Type
	sc.ID = tempCommon.ID
	sc.Name = tempCommon.Name

	switch sc.Type {
	case SchemaChangeTypeModifyProperty:
		sc.SchemaChangeModifyPropertyPayload = &SchemaChangeModifyPropertyPayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifyPropertyPayload)
	case SchemaChangeTypeAddField:
		sc.SchemaChangeAddFieldPayload = &SchemaChangeAddFieldPayload{}
		return utils.FromJSON(data, sc.SchemaChangeAddFieldPayload)
	case SchemaChangeTypeRemoveField:
		return nil
	case SchemaChangeTypeModifyField:
		sc.SchemaChangeModifyFieldPayload = &SchemaChangeModifyFieldPayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifyFieldPayload)
	case SchemaChangeTypeAddIndex:
		sc.SchemaChangeAddIndexPayload = &SchemaChangeAddIndexPayload{}
		return utils.FromJSON(data, sc.SchemaChangeAddIndexPayload)
	case SchemaChangeTypeRemoveIndex:
		return nil
	case SchemaChangeTypeModifyIndex:
		sc.SchemaChangeModifyIndexPayload = &SchemaChangeModifyIndexPayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifyIndexPayload)
	case SchemaChangeTypeAddConstraint:
		sc.SchemaChangeAddConstraintPayload = &SchemaChangeAddConstraintPayload{}
		return utils.FromJSON(data, sc.SchemaChangeAddConstraintPayload)
	case SchemaChangeTypeRemoveConstraint:
		return nil
	case SchemaChangeTypeModifyConstraint:
		sc.SchemaChangeModifyConstraintPayload = &SchemaChangeModifyConstraintPayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifyConstraintPayload)
	case SchemaChangeTypeAddSchema:
		sc.SchemaChangeAddSchemaPayload = &SchemaChangeAddSchemaPayload{}
		return utils.FromJSON(data, sc.SchemaChangeAddSchemaPayload)
	case SchemaChangeTypeRemoveSchema:
		return nil
	case SchemaChangeTypeModifySchema:
		sc.SchemaChangeModifySchemaPayload = &SchemaChangeModifySchemaPayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifySchemaPayload)
	case SchemaChangeTypeModifySchemaReference:
		sc.SchemaChangeModifySchemaReferencePayload = &SchemaChangeModifySchemaReferencePayload{}
		return utils.FromJSON(data, sc.SchemaChangeModifySchemaReferencePayload)
	default:
		return newUnknownSchemaChangeTypeError(sc.Type)
	}
}

func (sc SchemaChange) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	m["type"] = sc.Type
	if sc.ID != nil && *sc.ID != "" {
		m["id"] = *sc.ID
	}
	if sc.Name != nil && *sc.Name != "" {
		m["name"] = *sc.Name
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
	case SchemaChangeTypeAddSchema:
		if sc.SchemaChangeAddSchemaPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeAddSchemaPayload)
		}
	case SchemaChangeTypeModifySchema:
		if sc.SchemaChangeModifySchemaPayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifySchemaPayload)
		}
	case SchemaChangeTypeModifySchemaReference:
		if sc.SchemaChangeModifySchemaReferencePayload != nil {
			payloadBytes, err = json.Marshal(sc.SchemaChangeModifySchemaReferencePayload)
		}
	case SchemaChangeTypeRemoveField, SchemaChangeTypeRemoveIndex, SchemaChangeTypeRemoveConstraint, SchemaChangeTypeRemoveSchema:
		return json.Marshal(m)
	default:
		return json.Marshal(m)
	}

	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("schema.SchemaChange.MarshalJSON").WithCause(err)
	}

	if payloadBytes != nil {
		var payloadMap map[string]any
		if err := utils.FromJSON(payloadBytes, &payloadMap); err != nil {
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
	Constraints SchemaConstraint `json:"constraints,omitempty"`
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
	Unset []string `json:"unset,omitempty"`
}

func (fd *PartialFieldDefinition) UnmarshalJSON(data []byte) error {
	type Alias PartialFieldDefinition

	var temp struct {
		Type   *FieldType      `json:"type"`
		Schema json.RawMessage `json:"schema,omitempty"`
		Unset  []string        `json:"unset,omitempty"`
		*Alias
	}

	temp.Alias = (*Alias)(fd)
	if err := utils.FromJSON(data, &temp); err != nil {
		return err
	}

	*fd = PartialFieldDefinition(*temp.Alias)

	if temp.Type != nil {
		fd.Type = temp.Type
		// Normalize deprecated "dynamic" to "record"
		if *fd.Type == FieldTypeDynamic {
			*fd.Type = FieldTypeRecord
		}
	}
	if temp.Unset != nil {
		fd.Unset = temp.Unset
	}

	if temp.Schema != nil && temp.Type != nil {
		handled := false
		switch *temp.Type {
		case FieldTypeObject, FieldTypeArray, FieldTypeRecord, FieldTypeDynamic:
			var singleSchema NestedSchemaReference
			if err := utils.FromJSON(temp.Schema, &singleSchema); err == nil {
				if singleSchema.ID != "" { // Check ID similar to FieldDefinition
					fd.Schema = singleSchema
					handled = true
				}
			}
		case FieldTypeUnion:
			var multiSchema []NestedSchemaReference
			if err := utils.FromJSON(temp.Schema, &multiSchema); err == nil {
				fd.Schema = multiSchema
				handled = true
			}
		}

		if !handled {
			if *temp.Type != FieldTypeObject && *temp.Type != FieldTypeArray && *temp.Type != FieldTypeRecord && *temp.Type != FieldTypeDynamic && *temp.Type != FieldTypeUnion {
				return ErrFieldTypeCannotHaveSchemaReference.WithOperation("schema.PartialFieldDefinition.UnmarshalJSON").
					WithMessage(fmt.Sprintf("field of type '%s' cannot have a 'schema' reference", *temp.Type))
			}
			var genericSchema any
			if err := utils.FromJSON(temp.Schema, &genericSchema); err != nil {
				return ErrFailedToUnmarshalSchema.WithCause(err).WithOperation("schema.PartialFieldDefinition.UnmarshalJSON")
			}
			fd.Schema = genericSchema
		}
	}

	if fd.ItemsType != nil && fd.Type != nil && *fd.Type != FieldTypeArray && *fd.Type != FieldTypeSet {
		return ErrFieldTypeCannotHaveItemsType.WithOperation("schema.PartialFieldDefinition.UnmarshalJSON").
			WithMessage(fmt.Sprintf("field of type '%s' cannot have an 'itemsType'", *fd.Type))
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
	Unset       []string               `json:"unset,omitempty"`
}

// PartialConstraint represents a partial definition of a constraint, used for modifications.
type PartialConstraint struct {
	Name         *string                 `json:"name,omitempty"`
	Operator     *common.LogicalOperator `json:"operator,omitempty"`
	Predicate    *string                 `json:"predicate,omitempty"`
	Field        *string                 `json:"field,omitempty"`
	Fields       []string                `json:"fields,omitempty"`
	Parameters   any                     `json:"parameters,omitempty"`
	Description  *string                 `json:"description,omitempty"`
	ErrorMessage *string                 `json:"errorMessage,omitempty"`
	Rules        []ConstraintRule        `json:"rules"`
	Unset        []string                `json:"unset,omitempty"`
}

// PartialNestedSchemaDefinition represents a partial definition of a nested schema, used for modifications.
type PartialNestedSchemaDefinition struct {
	Name        *string                     `json:"name,omitempty"`
	Description *string                     `json:"description,omitempty"`
	Indexes     []IndexDefinition           `json:"indexes,omitempty"`
	Metadata    map[string]any              `json:"metadata,omitempty"`
	Concrete    *bool                       `json:"concrete,omitempty"`
	Fields      any                         `json:"fields,omitempty"`
	Type        *FieldType                  `json:"type,omitempty"`
	Constraints SchemaConstraint `json:"constraints,omitempty"`
	Default     any                         `json:"default,omitempty"`
	Schema      any                         `json:"schema,omitempty"`
	ItemsType   *FieldType                  `json:"itemsType,omitempty"`
}

// TransformFunction defines a function for transforming data from one schema version to another.
type TransformFunction[Initial, Next any] func(data Initial) (Next, error)

// DataTransform represents a pair of transformations for bidirectional data migration.
type DataTransform[Initial, Next any] struct {
	Forward  TransformFunction[Initial, Next] `json:"-"`
	Backward TransformFunction[Next, Initial] `json:"-"`
}

// MigrationVersion represents the version transition for a migration.
type MigrationVersion struct {
	Source string  `json:"source"`
	Target *string `json:"target,omitempty"`
}

// Migration defines a single migration, consisting of schema changes and data transformations.
type Migration struct {
	ID          string           `json:"id"`
	Version     MigrationVersion `json:"version"`
	Changes     []SchemaChange   `json:"changes"`
	Description string           `json:"description"`
	Status      string           `json:"status,omitempty"`
	Rollback    []SchemaChange   `json:"rollback,omitempty"`
	Transform   string           `json:"transform"`
	CreatedAt   string           `json:"createdAt"`
	Checksum    string           `json:"checksum"`
}

// InputHint provides hints for UI generation or tooling.
type InputHint map[string]any

// SchemaHint provides hints for the schema as a whole.
type SchemaHint map[string]any

// ValidationResult represents the result of a validation operation.
type ValidationResult struct {
	Valid  bool           `json:"valid"`
	Issues []common.Issue `json:"issues"`
}

func newUnknownSchemaChangeTypeError(changeType SchemaChangeType) error {
	return ErrUnknownSchemaChangeType.WithOperation("schema.newUnknownSchemaChangeTypeError").
		WithMessage(fmt.Sprintf("unknown schema change type: %s", changeType))
}

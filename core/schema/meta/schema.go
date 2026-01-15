package meta

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// MetaSchema is the schema that describes the structure of Schema itself
var MetaSchema = definition.Schema{
	BaseSchema: definition.BaseSchema{
		Name:        "Schema",
		Description: "Meta-schema defining the structure of schema definitions",
		Fields: map[definition.FieldId]definition.Field{
			"name": {
				Name:        "name",
				Description: "The name of the schema",
				Required:    true,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			"description": {
				Name:        "description",
				Description: "Optional description of the schema",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			"version": {
				Name:        "version",
				Description: "Schema version",
				Required:    true,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			"fields": {
				Name:        "fields",
				Description: "Map of field definitions",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Field"}),
				},
			},
			"indexes": {
				Name:        "indexes",
				Description: "Map of index definitions",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Index"}),
				},
			},
			"constraints": {
				Name:        "constraints",
				Description: "Map of constraint definitions",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Constraint"}),
				},
			},
			"metadata": {
				Name:        "metadata",
				Description: "Arbitrary metadata as key-value pairs",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeRecord,
				},
			},
			"schemas": {
				Name:        "schemas",
				Description: "Map of nested schema definitions",
				Required:    false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "NestedSchema"}),
				},
			},
		},
		Indexes:     map[definition.IndexId]definition.Index{},
		Constraints: map[definition.ConstraintId]definition.Constraint{},
		Metadata:    map[string]any{},
	},
	Version: common.Version{},
	Schemas: map[definition.SchemaId]definition.NestedSchema{
		// Field definition
		"Field": {
			BaseSchema: definition.BaseSchema{
				Name:        "Field",
				Description: "Defines a field within a schema",
				Fields: map[definition.FieldId]definition.Field{
					"name": {
						Name:        "name",
						Description: "The name of the field",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description of the field",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"type": {
						Name:        "type",
						Description: "The type of the field",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "definition.FieldTypeEnum"}),
						},
					},
					"default": {
						Name:        "default",
						Description: "Default value for the field",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
					"schema": {
						Name:        "schema",
						Description: "Schema reference for complex types (single SchemaReference or array for unions)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "SchemaReference"}, {ID: "SchemaReferenceArray"}}),
						},
					},
					"required": {
						Name:        "required",
						Description: "Whether the field is required",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					"deprecated": {
						Name:        "deprecated",
						Description: "Whether the field is deprecated",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					"unique": {
						Name:        "unique",
						Description: "Whether the field value must be unique",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
				},
			},
		},

		// NestedSchema definition - represents either object schemas or element type schemas
		"NestedSchema": {
			BaseSchema: definition.BaseSchema{
				Name:        "NestedSchema",
				Description: "Defines a nested schema - uses BaseSchema fields for objects, FieldProperties for array/enum elements",
				Fields: map[definition.FieldId]definition.Field{
					"name": {
						Name:        "name",
						Description: "The name of the nested schema",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"fields": {
						Name:        "fields",
						Description: "Map of field definitions (for object types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Field"}),
						},
					},
					"indexes": {
						Name:        "indexes",
						Description: "Map of index definitions (for object types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Index"}),
						},
					},
					"constraints": {
						Name:        "constraints",
						Description: "Map of constraint definitions (for object types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Constraint"}),
						},
					},
					"metadata": {
						Name:        "metadata",
						Description: "Arbitrary metadata (for object types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeRecord,
						},
					},
					"type": {
						Name:        "type",
						Description: "The element type (for array/set/enum element types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "definition.FieldTypeEnum"}),
						},
					},
					"default": {
						Name:        "default",
						Description: "Default value (for array/set/enum element types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
					"values": {
						Name:        "values",
						Description: "Allowed values (for enum element types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "UnknownArray"}),
						},
					},
					"schema": {
						Name:        "schema",
						Description: "Schema reference (for complex array/set element types only)",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "SchemaReference"}, {ID: "SchemaReferenceArray"}}),
						},
					},
				},
			},
		},

		// Index definition
		"Index": {
			BaseSchema: definition.BaseSchema{
				Name:        "Index",
				Description: "Defines an index for querying",
				Fields: map[definition.FieldId]definition.Field{
					"name": {
						Name:        "name",
						Description: "The name of the index",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"type": {
						Name:        "type",
						Description: "The type of index",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "IndexTypeEnum"}),
						},
					},
					"fields": {
						Name:        "fields",
						Description: "Fields included in the index",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "StringArray"}),
						},
					},
					"unique": {
						Name:        "unique",
						Description: "Whether the index enforces uniqueness",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					"order": {
						Name:        "order",
						Description: "Sort order",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "IndexOrderEnum"}),
						},
					},
					"condition": {
						Name:        "condition",
						Description: "Optional condition for partial indexes",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "IndexCondition"}, {ID: "IndexConditionGroup"}}),
						},
					},
				},
			},
		},

		// IndexCondition definition
		"IndexCondition": {
			BaseSchema: definition.BaseSchema{
				Name:        "IndexCondition",
				Description: "A single condition for a partial index",
				Fields: map[definition.FieldId]definition.Field{
					"field": {
						Name:        "field",
						Description: "The field to check",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"operator": {
						Name:        "operator",
						Description: "Comparison operator",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "ComparisonOperatorEnum"}),
						},
					},
					"value": {
						Name:        "value",
						Description: "The value to compare against",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
				},
			},
		},

		// IndexConditionGroup definition
		"IndexConditionGroup": {
			BaseSchema: definition.BaseSchema{
				Name:        "IndexConditionGroup",
				Description: "A group of conditions combined with a logical operator",
				Fields: map[definition.FieldId]definition.Field{
					"operator": {
						Name:        "operator",
						Description: "Logical operator (AND, OR, NOT)",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "LogicalOperatorEnum"}),
						},
					},
					"conditions": {
						Name:        "conditions",
						Description: "Array of conditions (each can be IndexCondition or IndexConditionGroup)",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "IndexConditionUnion"}),
						},
					},
				},
			},
		},

		// IndexConditionUnion - for array elements
		"IndexConditionUnion": {
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeUnion,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "IndexCondition"}, {ID: "IndexConditionGroup"}}),
			},
		},

		// Constraint definition - composite of metadata + union of rule/group
		"Constraint": {
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeComposite,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "ConstraintMetadata"}, {ID: "ConstraintUnion"}}),
			},
		},

		// ConstraintMetadata - the metadata fields always present
		"ConstraintMetadata": {
			BaseSchema: definition.BaseSchema{
				Name:        "ConstraintMetadata",
				Description: "Metadata fields present in all constraints",
				Fields: map[definition.FieldId]definition.Field{
					"name": {
						Name:        "name",
						Description: "The name of the constraint",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
				},
			},
		},

		// ConstraintUnion - for the union part
		"ConstraintUnion": {
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeUnion,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: "ConstraintRule"}, {ID: "ConstraintGroup"}}),
			},
		},

		// ConstraintRule definition
		"ConstraintRule": {
			BaseSchema: definition.BaseSchema{
				Name:        "ConstraintRule",
				Description: "A single validation rule with a predicate",
				Fields: map[definition.FieldId]definition.Field{
					"fields": {
						Name:        "fields",
						Description: "Fields this rule applies to",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "StringArray"}),
						},
					},
					"predicate": {
						Name:        "predicate",
						Description: "The predicate function name",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"parameters": {
						Name:        "parameters",
						Description: "Parameters for the predicate",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
				},
			},
		},

		// ConstraintGroup definition
		"ConstraintGroup": {
			BaseSchema: definition.BaseSchema{
				Name:        "ConstraintGroup",
				Description: "A group of constraints combined with a logical operator",
				Fields: map[definition.FieldId]definition.Field{
					"operator": {
						Name:        "operator",
						Description: "Logical operator (AND, OR, NOT)",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "LogicalOperatorEnum"}),
						},
					},
					"rules": {
						Name:        "rules",
						Description: "Array of constraint rules or groups",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "ConstraintUnion"}),
						},
					},
				},
			},
		},

		// SchemaReference definition
		"SchemaReference": {
			BaseSchema: definition.BaseSchema{
				Name:        "SchemaReference",
				Description: "A reference to a nested schema",
				Fields: map[definition.FieldId]definition.Field{
					"id": {
						Name:        "id",
						Description: "The ID of the referenced schema",
						Required:    true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					"indexes": {
						Name:        "indexes",
						Description: "Additional indexes for this reference",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Index"}),
						},
					},
					"constraints": {
						Name:        "constraints",
						Description: "Additional constraints for this reference",
						Required:    false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "Constraint"}),
						},
					},
				},
			},
		},

		// Array types
		"SchemaReferenceArray": {
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeObject,
				Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "SchemaReference"}),
			},
		},

		"StringArray": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
		},

		"UnknownArray": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeUnknown,
			},
		},

		// Enum definitions
		"definition.FieldTypeEnum": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Values: []definition.LiteralValue{
					mustNewLiteralValue("unknown"),
					mustNewLiteralValue("string"),
					mustNewLiteralValue("number"),
					mustNewLiteralValue("integer"),
					mustNewLiteralValue("decimal"),
					mustNewLiteralValue("boolean"),
					mustNewLiteralValue("array"),
					mustNewLiteralValue("set"),
					mustNewLiteralValue("enum"),
					mustNewLiteralValue("object"),
					mustNewLiteralValue("record"),
					mustNewLiteralValue("union"),
					mustNewLiteralValue("composite"),
					mustNewLiteralValue("geometry"),
				},
			},
		},

		"IndexTypeEnum": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Values: []definition.LiteralValue{
					mustNewLiteralValue("normal"),
					mustNewLiteralValue("unique"),
					mustNewLiteralValue("primary"),
					mustNewLiteralValue("spatial"),
					mustNewLiteralValue("fulltext"),
				},
			},
		},

		"LogicalOperatorEnum": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Values: []definition.LiteralValue{
					mustNewLiteralValue("and"),
					mustNewLiteralValue("or"),
					mustNewLiteralValue("not"),
					mustNewLiteralValue("nor"),
					mustNewLiteralValue("xor"),
					mustNewLiteralValue("nand"),
					mustNewLiteralValue("xnor"),
				},
			},
		},

		"IndexOrderEnum": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Values: []definition.LiteralValue{
					mustNewLiteralValue("asc"),
					mustNewLiteralValue("desc"),
				},
			},
		},

		"ComparisonOperatorEnum": {
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Values: []definition.LiteralValue{
					mustNewLiteralValue("eq"),
					mustNewLiteralValue("neq"),
					mustNewLiteralValue("lt"),
					mustNewLiteralValue("lte"),
					mustNewLiteralValue("gt"),
					mustNewLiteralValue("gte"),
					mustNewLiteralValue("in"),
					mustNewLiteralValue("nin"),
					mustNewLiteralValue("contains"),
					mustNewLiteralValue("ncontains"),
					mustNewLiteralValue("exists"),
					mustNewLiteralValue("nexists"),
				},
			},
		},
	},
}

func mustNewLiteralValue[T definition.LiteralValueType](value T) definition.LiteralValue {
	val, err := definition.NewLiteralValue(value)
	if err != nil {
		panic(err)
	}
	return val
}


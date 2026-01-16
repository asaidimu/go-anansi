package definition

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)

// MetaSchema is the schema that describes the structure of Schema itself
var MetaSchema = Schema{
	Version: *common.MustNewVersion("1.0.0"),
	BaseSchema: BaseSchema{
		Name:        "Schema",
		Description: "Meta-schema defining the structure of schema definitions",
		Fields: map[FieldId]Field{
			"name": {
				Name:        "name",
				Description: "The name of the schema",
				Required:    true,
				FieldProperties: FieldProperties{
					Type: FieldTypeString,
				},
			},
			"description": {
				Name:        "description",
				Description: "Optional description of the schema",
				Required:    false,
				FieldProperties: FieldProperties{
					Type: FieldTypeString,
				},
			},
			"version": {
				Name:        "version",
				Description: "Schema version",
				Required:    true,
				FieldProperties: FieldProperties{
					Type: FieldTypeString,
				},
			},
			"fields": {
				Name:        "fields",
				Description: "Map of field definitions",
				Required:    false,
				FieldProperties: FieldProperties{
					Type:   FieldTypeRecord,
					Schema: NewSchemaReference(SchemaReference{ID: "Field"}),
				},
			},
			"indexes": {
				Name:        "indexes",
				Description: "Map of index definitions",
				Required:    false,
				FieldProperties: FieldProperties{
					Type:   FieldTypeRecord,
					Schema: NewSchemaReference(SchemaReference{ID: "Index"}),
				},
			},
			"constraints": {
				Name:        "constraints",
				Description: "Map of constraint definitions",
				Required:    false,
				FieldProperties: FieldProperties{
					Type:   FieldTypeRecord,
					Schema: NewSchemaReference(SchemaReference{ID: "Constraint"}),
				},
			},
			"metadata": {
				Name:        "metadata",
				Description: "Arbitrary metadata as key-value pairs",
				Required:    false,
				FieldProperties: FieldProperties{
					Type: FieldTypeRecord,
				},
			},
			"schemas": {
				Name:        "schemas",
				Description: "Map of nested schema definitions",
				Required:    false,
				FieldProperties: FieldProperties{
					Type:   FieldTypeRecord,
					Schema: NewSchemaReference(SchemaReference{ID: "NestedSchema"}),
				},
			},
		},
		Indexes:     nil,
		Constraints: nil,
		Metadata:    nil,
	},
	Schemas: map[SchemaId]NestedSchema{
		// Field definition
		"Field": {
			BaseSchema: BaseSchema{
				Name:        "Field",
				Description: "Defines a field within a schema",
				Fields: map[FieldId]Field{
					"name": {
						Name:        "name",
						Description: "The name of the field",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description of the field",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"type": {
						Name:        "type",
						Description: "The type of the field",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "FieldTypeEnum"}),
						},
					},
					"default": {
						Name:        "default",
						Description: "Default value for the field",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeUnknown,
						},
					},
					"schema": {
						Name:        "schema",
						Description: "Schema reference for complex types (single SchemaReference or array for unions/composites)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeUnion,
							Schema: NewSchemaReference([]SchemaReference{{ID: "SchemaReference"}, {ID: "SchemaReferenceArray"}}),
						},
					},
					"required": {
						Name:        "required",
						Description: "Whether the field is required",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeBoolean,
						},
					},
					"deprecated": {
						Name:        "deprecated",
						Description: "Whether the field is deprecated",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeBoolean,
						},
					},
					"unique": {
						Name:        "unique",
						Description: "Whether the field value must be unique",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeBoolean,
						},
					},
				},
				Constraints: nil,
			},
		},

		// NestedSchema definition - represents either object schemas or element type schemas
		"NestedSchema": {
			BaseSchema: BaseSchema{
				Name:        "NestedSchema",
				Description: "Defines a nested schema - uses BaseSchema fields for objects, FieldProperties for array/enum elements",
				Fields: map[FieldId]Field{
					"name": {
						Name:        "name",
						Description: "The name of the nested schema",
						Required:    true, // FIXED: Changed from false to true
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"fields": {
						Name:        "fields",
						Description: "Map of field definitions (for object types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeRecord,
							Schema: NewSchemaReference(SchemaReference{ID: "Field"}),
						},
					},
					"indexes": {
						Name:        "indexes",
						Description: "Map of index definitions (for object types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeRecord,
							Schema: NewSchemaReference(SchemaReference{ID: "Index"}),
						},
					},
					"constraints": {
						Name:        "constraints",
						Description: "Map of constraint definitions (for object types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeRecord,
							Schema: NewSchemaReference(SchemaReference{ID: "Constraint"}),
						},
					},
					"metadata": {
						Name:        "metadata",
						Description: "Arbitrary metadata (for object types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeRecord,
						},
					},
					"type": {
						Name:        "type",
						Description: "The element type (for array/set/enum element types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "FieldTypeEnum"}),
						},
					},
					"default": {
						Name:        "default",
						Description: "Default value (for array/set/enum element types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeUnknown,
						},
					},
					"values": {
						Name:        "values",
						Description: "Allowed values (for enum element types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeArray,
							Schema: NewSchemaReference(SchemaReference{ID: "UnknownArray"}),
						},
					},
					"schema": {
						Name:        "schema",
						Description: "Schema reference (for complex array/set element types only)",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeUnion,
							Schema: NewSchemaReference([]SchemaReference{{ID: "SchemaReference"}, {ID: "SchemaReferenceArray"}}),
						},
					},
				},
				Constraints: nil,
			},
		},

		// Index definition
		"Index": {
			BaseSchema: BaseSchema{
				Name:        "Index",
				Description: "Defines an index for querying",
				Fields: map[FieldId]Field{
					"name": {
						Name:        "name",
						Description: "The name of the index",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"type": {
						Name:        "type",
						Description: "The type of index",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "IndexTypeEnum"}),
						},
					},
					"fields": {
						Name:        "fields",
						Description: "Fields included in the index",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeArray,
							Schema: NewSchemaReference(SchemaReference{ID: "StringArray"}),
						},
					},
					"unique": {
						Name:        "unique",
						Description: "Whether the index enforces uniqueness",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeBoolean,
						},
					},
					"order": {
						Name:        "order",
						Description: "Sort order",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "IndexOrderEnum"}),
						},
					},
					"condition": {
						Name:        "condition",
						Description: "Optional condition for partial indexes",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeUnion,
							Schema: NewSchemaReference([]SchemaReference{{ID: "IndexCondition"}, {ID: "IndexConditionGroup"}}),
						},
					},
				},
			},
		},

		// IndexCondition definition
		"IndexCondition": {
			BaseSchema: BaseSchema{
				Name:        "IndexCondition",
				Description: "A single condition for a partial index",
				Fields: map[FieldId]Field{
					"field": {
						Name:        "field",
						Description: "The field to check",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"operator": {
						Name:        "operator",
						Description: "Comparison operator",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "ComparisonOperatorEnum"}),
						},
					},
					"value": {
						Name:        "value",
						Description: "The value to compare against",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeUnknown,
						},
					},
				},
			},
		},

		// IndexConditionGroup definition
		"IndexConditionGroup": {
			BaseSchema: BaseSchema{
				Name:        "IndexConditionGroup",
				Description: "A group of conditions combined with a logical operator",
				Fields: map[FieldId]Field{
					"operator": {
						Name:        "operator",
						Description: "Logical operator (AND, OR, NOT)",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "LogicalOperatorEnum"}),
						},
					},
					"conditions": {
						Name:        "conditions",
						Description: "Array of conditions (each can be IndexCondition or IndexConditionGroup)",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeArray,
							Schema: NewSchemaReference(SchemaReference{ID: "IndexConditionUnion"}),
						},
					},
				},
			},
		},

		// IndexConditionUnion - for array elements
		"IndexConditionUnion": {
			FieldProperties: FieldProperties{
				Type:   FieldTypeUnion,
				Schema: NewSchemaReference([]SchemaReference{{ID: "IndexCondition"}, {ID: "IndexConditionGroup"}}),
			},
		},

		// Constraint definition - composite of metadata + union of rule/group
		"Constraint": {
			FieldProperties: FieldProperties{
				Type:   FieldTypeComposite,
				Schema: NewSchemaReference([]SchemaReference{{ID: "ConstraintMetadata"}, {ID: "ConstraintUnion"}}),
			},
		},

		// ConstraintMetadata - the metadata fields always present
		"ConstraintMetadata": {
			BaseSchema: BaseSchema{
				Name:        "ConstraintMetadata",
				Description: "Metadata fields present in all constraints",
				Fields: map[FieldId]Field{
					"name": {
						Name:        "name",
						Description: "The name of the constraint",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"description": {
						Name:        "description",
						Description: "Optional description",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
				},
			},
		},

		// ConstraintUnion - for the union part
		"ConstraintUnion": {
			FieldProperties: FieldProperties{
				Type:   FieldTypeUnion,
				Schema: NewSchemaReference([]SchemaReference{{ID: "ConstraintRule"}, {ID: "ConstraintGroup"}}),
			},
		},

		// ConstraintRule definition
		"ConstraintRule": {
			BaseSchema: BaseSchema{
				Name:        "ConstraintRule",
				Description: "A single validation rule with a predicate",
				Fields: map[FieldId]Field{
					"fields": {
						Name:        "fields",
						Description: "Fields this rule applies to",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeArray,
							Schema: NewSchemaReference(SchemaReference{ID: "StringArray"}),
						},
					},
					"predicate": {
						Name:        "predicate",
						Description: "The predicate function name",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"parameters": {
						Name:        "parameters",
						Description: "Parameters for the predicate",
						Required:    false,
						FieldProperties: FieldProperties{
							Type: FieldTypeUnknown,
						},
					},
				},
			},
		},

		// ConstraintGroup definition
		"ConstraintGroup": {
			BaseSchema: BaseSchema{
				Name:        "ConstraintGroup",
				Description: "A group of constraints combined with a logical operator",
				Fields: map[FieldId]Field{
					"operator": {
						Name:        "operator",
						Description: "Logical operator (AND, OR, NOT)",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeEnum,
							Schema: NewSchemaReference(SchemaReference{ID: "LogicalOperatorEnum"}),
						},
					},
					"rules": {
						Name:        "rules",
						Description: "Array of constraint rules or groups",
						Required:    true,
						FieldProperties: FieldProperties{
							Type:   FieldTypeArray,
							Schema: NewSchemaReference(SchemaReference{ID: "ConstraintUnion"}),
						},
					},
				},
			},
		},

		// SchemaReference definition
		"SchemaReference": {
			BaseSchema: BaseSchema{
				Name:        "SchemaReference",
				Description: "A reference to a nested schema",
				Fields: map[FieldId]Field{
					"id": {
						Name:        "id",
						Description: "The ID of the referenced schema",
						Required:    true,
						FieldProperties: FieldProperties{
							Type: FieldTypeString,
						},
					},
					"indexes": {
						Name:        "indexes",
						Description: "Additional indexes for this reference",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeRecord,
							Schema: NewSchemaReference(SchemaReference{ID: "Index"}),
						},
					},
					"constraints": {
						Name:        "constraints",
						Description: "Additional constraints for this reference",
						Required:    false,
						FieldProperties: FieldProperties{
							Type:   FieldTypeRecord,
							Schema: NewSchemaReference(SchemaReference{ID: "Constraint"}),
						},
					},
				},
			},
		},

		// Array types
		"SchemaReferenceArray": {
			FieldProperties: FieldProperties{
				Type:   FieldTypeArray,
				Schema: NewSchemaReference(SchemaReference{ID: "SchemaReference"}),
			},
		},

		"StringArray": {
			FieldProperties: FieldProperties{
				Type: FieldTypeString,
			},
		},

		"UnknownArray": {
			FieldProperties: FieldProperties{
				Type: FieldTypeUnknown,
			},
		},

		// Enum definitions
		"FieldTypeEnum": {
			FieldProperties: FieldProperties{
				Type: FieldTypeString,
			},
					Values: []LiteralValue{
						MustNewLiteralValue("unknown"),
						MustNewLiteralValue("string"),
						MustNewLiteralValue("number"),
						MustNewLiteralValue("integer"),
						MustNewLiteralValue("decimal"),
						MustNewLiteralValue("boolean"),
						MustNewLiteralValue("array"),
						MustNewLiteralValue("set"),
						MustNewLiteralValue("enum"),
						MustNewLiteralValue("object"),
						MustNewLiteralValue("record"),
						MustNewLiteralValue("union"),
						MustNewLiteralValue("composite"),
						MustNewLiteralValue("geometry"),
					},
				},
			
				"IndexTypeEnum": {
					FieldProperties: FieldProperties{
						Type: FieldTypeString,
					},
					Values: []LiteralValue{
						MustNewLiteralValue("normal"),
						MustNewLiteralValue("unique"),
						MustNewLiteralValue("primary"),
						MustNewLiteralValue("spatial"),
						MustNewLiteralValue("fulltext"),
					},
				},
			
				"LogicalOperatorEnum": {
					FieldProperties: FieldProperties{
						Type: FieldTypeString,
					},
					Values: []LiteralValue{
						MustNewLiteralValue("and"),
						MustNewLiteralValue("or"),
						MustNewLiteralValue("not"),
						MustNewLiteralValue("nor"),
						MustNewLiteralValue("xor"),
						MustNewLiteralValue("nand"),
						MustNewLiteralValue("xnor"),
					},
				},
			
				"IndexOrderEnum": {
					FieldProperties: FieldProperties{
						Type: FieldTypeString,
					},
					Values: []LiteralValue{
						MustNewLiteralValue("asc"),
						MustNewLiteralValue("desc"),
					},
				},
			
				"ComparisonOperatorEnum": {
					FieldProperties: FieldProperties{
						Type: FieldTypeString,
					},
					Values: []LiteralValue{
						MustNewLiteralValue("eq"),
						MustNewLiteralValue("neq"),
						MustNewLiteralValue("lt"),
						MustNewLiteralValue("lte"),
						MustNewLiteralValue("gt"),
						MustNewLiteralValue("gte"),
						MustNewLiteralValue("in"),
						MustNewLiteralValue("nin"),
						MustNewLiteralValue("contains"),
						MustNewLiteralValue("ncontains"),
						MustNewLiteralValue("exists"),
						MustNewLiteralValue("nexists"),
					},		},
	},
}

func MustNewLiteralValue[T LiteralValueType](value T) LiteralValue {
	val, err := NewLiteralValue(value)
	if err != nil {
		panic(err)
	}
	return val
}

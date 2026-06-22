package meta

import (
	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// MetaSchema is the schema that describes the structure of Schema itself
var MetaSchema = definition.Schema{
	Version: common.MustNewVersion("1.0.0"),
	BaseSchema: definition.BaseSchema{
		Name:        "Schema",
		Description: "Meta-schema defining the structure of schema definitions",
		Fields: map[definition.FieldId]definition.Field{
			FieldIDName: {
				Name:     "name",
				Required: true,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			FieldIDDescription: {
				Name:     "description",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			FieldIDVersion: {
				Name:     "version",
				Required: true,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
			FieldIDFields: {
				Name:     "fields",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDField}),
				},
			},
			FieldIDIndexes: {
				Name:     "indexes",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndex}),
				},
			},
			FieldIDConstraints: {
				Name:     "constraints",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDConstraint}),
				},
			},
			FieldIDMetadata: {
				Name:     "metadata",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeRecord,
				},
			},
			FieldIDSchemas: {
				Name:     "schemas",
				Required: false,
				FieldProperties: definition.FieldProperties{
					Type:   definition.FieldTypeRecord,
					Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDNestedSchema}),
				},
			},
		},
		Constraints: map[definition.ConstraintId]definition.Constraint{
			ConstraintGlobalFieldIdUniqueness:   MetaSchemaConstraints[ConstraintGlobalFieldIdUniqueness],
			ConstraintSchemaReferenceIntegrity:  MetaSchemaConstraints[ConstraintSchemaReferenceIntegrity],
			ConstraintSchemaNameRequired:        MetaSchemaConstraints[ConstraintSchemaNameRequired],
			ConstraintNestedSchemaModeExclusive: MetaSchemaConstraints[ConstraintNestedSchemaModeExclusive],
		},
	},
	Schemas: map[definition.SchemaId]definition.NestedSchema{
		// -----------------------------------------------------------------
		// Field definition
		// -----------------------------------------------------------------
		SchemaIDField: {
			BaseSchema: definition.BaseSchema{
				Name: "Field",
				Fields: map[definition.FieldId]definition.Field{
					FieldFieldIDName: {
						Name:     "name",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					FieldFieldIDDescription: {
						Name:     "description",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					FieldFieldIDType: {
						Name:     "type",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDFieldTypeEnum, Type: definition.FieldTypeString}),
						},
					},
					FieldFieldIDDefault: {
						Name:     "default",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
					FieldFieldIDSchema: {
						Name:     "schema",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{
								{ID: SchemaIDSchemaReference},      // single named ref
								{ID: SchemaIDSchemaReferenceArray}, // array of named refs
								{ID: SchemaIDInlineTypeDescriptor}, // inline type descriptor
							}),
						},
					},
					FieldFieldIDRequired: {
						Name:     "required",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					FieldFieldIDDeprecated: {
						Name:     "deprecated",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					FieldFieldIDUnique: {
						Name:     "unique",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintPrimitivesNoSchema:                      MetaSchemaConstraints[ConstraintPrimitivesNoSchema],
					ConstraintEnumFieldsValid:                         MetaSchemaConstraints[ConstraintEnumFieldsValid],
					ConstraintArraysRequireSchema:                     MetaSchemaConstraints[ConstraintArraysRequireSchema],
					ConstraintObjectsRequireSchema:                    MetaSchemaConstraints[ConstraintObjectsRequireSchema],
					ConstraintUnionsRequireMultipleSchemas:            MetaSchemaConstraints[ConstraintUnionsRequireMultipleSchemas],
					ConstraintCompositesRequireMultipleSchemas:        MetaSchemaConstraints[ConstraintCompositesRequireMultipleSchemas],
					ConstraintDefaultValueTypeMatch:                   MetaSchemaConstraints[ConstraintDefaultValueTypeMatch],
					ConstraintObjectReferencedSchemaHasFields:         MetaSchemaConstraints[ConstraintObjectReferencedSchemaHasFields],
					ConstraintCompositeReferencedSchemasMustBeObjects: MetaSchemaConstraints[ConstraintCompositeReferencedSchemasMustBeObjects],
					ConstraintSchemaReferenceFormCorrect:              MetaSchemaConstraints[ConstraintSchemaReferenceFormCorrect],
					ConstraintRecordsAllowOptionalSchema:              MetaSchemaConstraints[ConstraintRecordsAllowOptionalSchema],
					ConstraintInlineTypeDescriptorValid:               MetaSchemaConstraints[ConstraintInlineTypeDescriptorValid],
				},
			},
		},

		// -----------------------------------------------------------------
		// NestedSchema
		// -----------------------------------------------------------------
		SchemaIDNestedSchema: {
			BaseSchema: definition.BaseSchema{
				Name: "NestedSchema",
				Fields: map[definition.FieldId]definition.Field{
					NestedSchemaFieldIDName: {
						Name:     "name",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					NestedSchemaFieldIDDescription: {
						Name:     "description",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					NestedSchemaFieldIDFields: {
						Name:     "fields",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDField}),
						},
					},
					NestedSchemaFieldIDIndexes: {
						Name:     "indexes",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndex}),
						},
					},
					NestedSchemaFieldIDConstraints: {
						Name:     "constraints",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDConstraint}),
						},
					},
					NestedSchemaFieldIDMetadata: {
						Name:     "metadata",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeRecord,
						},
					},
					NestedSchemaFieldIDType: {
						Name:     "type",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDFieldTypeEnum}),
						},
					},
					NestedSchemaFieldIDDefault: {
						Name:     "default",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
					NestedSchemaFieldIDValues: {
						Name:     "values",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDUnknown}),
						},
					},
					NestedSchemaFieldIDSchema: {
						Name:     "schema",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{
								{ID: SchemaIDSchemaReference},
								{ID: SchemaIDSchemaReferenceArray},
								{ID: SchemaIDInlineTypeDescriptor}, // inline type descriptor
							}),
						},
					},
					NestedSchemaFieldIDConcrete: {
						Name:     "concrete",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintNestedSchemaModeExclusive: MetaSchemaConstraints[ConstraintNestedSchemaModeExclusive],
				},
			},
		},

		// -----------------------------------------------------------------
		// Index
		// -----------------------------------------------------------------
		SchemaIDIndex: {
			BaseSchema: definition.BaseSchema{
				Name: "Index",
				Fields: map[definition.FieldId]definition.Field{
					IndexFieldIDName: {
						Name:     "name",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					IndexFieldIDDescription: {
						Name:     "description",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					IndexFieldIDType: {
						Name:     "type",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndexTypeEnum}),
						},
					},
					IndexFieldIDFields: {
						Name:     "fields",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDString}),
						},
					},
					IndexFieldIDUnique: {
						Name:     "unique",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeBoolean,
						},
					},
					IndexFieldIDOrder: {
						Name:     "order",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndexOrderEnum}),
						},
					},
					IndexFieldIDCondition: {
						Name:     "condition",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnion,
							Schema: definition.NewSchemaReference([]definition.SchemaReference{
								{ID: SchemaIDIndexCondition},
								{ID: SchemaIDIndexConditionGroup},
							}),
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintIndexFieldsNotEmpty:          MetaSchemaConstraints[ConstraintIndexFieldsNotEmpty],
					ConstraintIndexFieldsExist:             MetaSchemaConstraints[ConstraintIndexFieldsExist],
					ConstraintSpatialIndexOnGeometryField:  MetaSchemaConstraints[ConstraintSpatialIndexOnGeometryField],
					ConstraintIndexConditionValueTypeMatch: MetaSchemaConstraints[ConstraintIndexConditionValueTypeMatch],
				},
			},
		},

		// -----------------------------------------------------------------
		// IndexCondition
		// -----------------------------------------------------------------
		SchemaIDIndexCondition: {
			BaseSchema: definition.BaseSchema{
				Name: "IndexCondition",
				Fields: map[definition.FieldId]definition.Field{
					IndexConditionFieldIDField: {
						Name:     "field",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					IndexConditionFieldIDOperator: {
						Name:     "operator",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDComparisonOperatorEnum}),
						},
					},
					IndexConditionFieldIDValue: {
						Name:     "value",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintIndexConditionValueTypeMatch: MetaSchemaConstraints[ConstraintIndexConditionValueTypeMatch],
					ConstraintIndexConditionExclusiveType:  MetaSchemaConstraints[ConstraintIndexConditionExclusiveType],
				},
			},
		},

		// -----------------------------------------------------------------
		// IndexConditionGroup
		// -----------------------------------------------------------------
		SchemaIDIndexConditionGroup: {
			BaseSchema: definition.BaseSchema{
				Name: "IndexConditionGroup",
				Fields: map[definition.FieldId]definition.Field{
					IndexConditionGroupFieldIDOperator: {
						Name:     "operator",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDLogicalOperatorEnum}),
						},
					},
					IndexConditionGroupFieldIDConditions: {
						Name:     "conditions",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndexConditionUnion}),
						},
					},
				},
			},
		},

		// -----------------------------------------------------------------
		// IndexConditionUnion
		// -----------------------------------------------------------------
		SchemaIDIndexConditionUnion: {
			BaseSchema: definition.BaseSchema{Name: "IndexConditionUnion"},
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeUnion,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: SchemaIDIndexCondition}, {ID: SchemaIDIndexConditionGroup}}),
			},
		},

		// -----------------------------------------------------------------
		// Constraint (composite)
		// -----------------------------------------------------------------
		SchemaIDConstraint: {
			BaseSchema: definition.BaseSchema{Name: "Constraint"},
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeComposite,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: SchemaIDConstraintMetadata}, {ID: SchemaIDConstraintUnion}}),
			},
		},

		SchemaIDConstraintMetadata: {
			BaseSchema: definition.BaseSchema{
				Name: "ConstraintMetadata",
				Fields: map[definition.FieldId]definition.Field{
					ConstraintMetadataFieldIDName: {
						Name:     "name",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					ConstraintMetadataFieldIDDescription: {
						Name:     "description",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
				},
			},
		},

		SchemaIDConstraintUnion: {
			BaseSchema: definition.BaseSchema{Name: "ConstraintUnion",
				Constraints: map[definition.ConstraintId]definition.Constraint{},
			},
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeUnion,
				Schema: definition.NewSchemaReference([]definition.SchemaReference{{ID: SchemaIDConstraintRule}, {ID: SchemaIDConstraintGroup}}),
			},
		},

		SchemaIDConstraintRule: {
			BaseSchema: definition.BaseSchema{
				Name: "ConstraintRule",
				Fields: map[definition.FieldId]definition.Field{
					ConstraintRuleFieldIDFields: {
						Name:     "fields",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDString}),
						},
					},
					ConstraintRuleFieldIDPredicate: {
						Name:     "predicate",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					ConstraintRuleFieldIDParameters: {
						Name:     "parameters",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeUnknown,
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintConstraintTypeExclusive:         MetaSchemaConstraints[ConstraintConstraintTypeExclusive],
					ConstraintConstraintRuleRequiresPredicate: MetaSchemaConstraints[ConstraintConstraintRuleRequiresPredicate],
					ConstraintConstraintFieldsExist:           MetaSchemaConstraints[ConstraintConstraintFieldsExist],
				},
			},
		},

		SchemaIDConstraintGroup: {
			BaseSchema: definition.BaseSchema{
				Name: "ConstraintGroup",
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintConstraintTypeExclusive: MetaSchemaConstraints[ConstraintConstraintTypeExclusive],
				},
				Fields: map[definition.FieldId]definition.Field{
					ConstraintGroupFieldIDOperator: {
						Name:     "operator",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDLogicalOperatorEnum}),
						},
					},
					ConstraintGroupFieldIDRules: {
						Name:     "rules",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDConstraintUnion}),
						},
					},
				},
			},
		},

		// -----------------------------------------------------------------
		// SchemaReference
		// -----------------------------------------------------------------
		SchemaIDSchemaReference: {
			BaseSchema: definition.BaseSchema{
				Name: "SchemaReference",
				Fields: map[definition.FieldId]definition.Field{
					SchemaReferenceFieldIDID: {
						Name:     "id",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
					SchemaReferenceFieldIDIndexes: {
						Name:     "indexes",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDIndex}),
						},
					},
					SchemaReferenceFieldIDConstraints: {
						Name:     "constraints",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeRecord,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDConstraint}),
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{
					ConstraintSchemaReferenceIdRequired: MetaSchemaConstraints[ConstraintSchemaReferenceIdRequired],
				},
			},
		},

		SchemaIDSchemaReferenceArray: {
			BaseSchema: definition.BaseSchema{Name: "SchemaReferenceArray"},
			FieldProperties: definition.FieldProperties{
				Type:   definition.FieldTypeArray,
				Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDSchemaReference}),
			},
		},

		SchemaIDString: {
			BaseSchema: definition.BaseSchema{Name: "String"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
		},

		SchemaIDUnknown: {
			BaseSchema: definition.BaseSchema{Name: "Unknown"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeUnknown,
			},
		},

		// -----------------------------------------------------------------
		// InlineTypeDescriptor
		// -----------------------------------------------------------------
		SchemaIDInlineTypeDescriptor: {
			BaseSchema: definition.BaseSchema{
				Name: "InlineTypeDescriptor",
				Fields: map[definition.FieldId]definition.Field{
					InlineTypeDescriptorFieldIDType: {
						Name:     "type",
						Required: true,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeEnum,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDInlineTypeEnum}),
						},
					},
					InlineTypeDescriptorFieldIDValues: {
						Name:     "values",
						Required: false,
						FieldProperties: definition.FieldProperties{
							Type:   definition.FieldTypeArray,
							Schema: definition.NewSchemaReference(definition.SchemaReference{ID: SchemaIDUnknown}),
						},
					},
				},
				Constraints: map[definition.ConstraintId]definition.Constraint{},
			},
		},

		SchemaIDInlineTypeEnum: {
			BaseSchema: definition.BaseSchema{Name: "InlineTypeEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
			Values: []definition.LiteralValue{
				mustNewLiteralValue("string"),
				mustNewLiteralValue("number"),
				mustNewLiteralValue("integer"),
				mustNewLiteralValue("decimal"),
				mustNewLiteralValue("boolean"),
				mustNewLiteralValue("bytes"),
				mustNewLiteralValue("unknown"),
				mustNewLiteralValue("record"),
			},
		},

		// -----------------------------------------------------------------
		// Enums
		// -----------------------------------------------------------------
		SchemaIDFieldTypeEnum: {
			BaseSchema: definition.BaseSchema{Name: "FieldTypeEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
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
				mustNewLiteralValue("bytes"),
			},
		},

		SchemaIDIndexTypeEnum: {
			BaseSchema: definition.BaseSchema{Name: "IndexTypeEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
			Values: []definition.LiteralValue{
				mustNewLiteralValue("normal"),
				mustNewLiteralValue("unique"),
				mustNewLiteralValue("primary"),
				mustNewLiteralValue("spatial"),
				mustNewLiteralValue("fulltext"),
			},
		},

		SchemaIDLogicalOperatorEnum: {
			BaseSchema: definition.BaseSchema{Name: "LogicalOperatorEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
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

		SchemaIDIndexOrderEnum: {
			BaseSchema: definition.BaseSchema{Name: "IndexOrderEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
			Values: []definition.LiteralValue{
				mustNewLiteralValue("asc"),
				mustNewLiteralValue("desc"),
			},
		},

		SchemaIDComparisonOperatorEnum: {
			BaseSchema: definition.BaseSchema{Name: "ComparisonOperatorEnum"},
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
			},
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
}

func mustNewLiteralValue[T definition.LiteralValueType](value T) definition.LiteralValue {
	val, err := definition.NewLiteralValue(value)
	if err != nil {
		panic(err)
	}
	return val
}

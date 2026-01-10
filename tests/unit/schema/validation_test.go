package schema

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

// createMockRootSchema creates a mock SchemaDefinition for testing purposes
func createMockRootSchema(fields map[string]*schema.FieldDefinition, nestedSchemas map[string]*schema.NestedSchemaDefinition) *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:          "mockRootSchema",
		Version:       "1.0.0",
		Fields:        fields,
		NestedSchemas: nestedSchemas,
	}
}

// createMockNestedSchema creates a mock NestedSchemaDefinition for testing purposes
func createMockNestedSchemaWithConditionalSet(name string, baseFields map[string]*schema.FieldDefinition, cfsID string, cfs schema.ConditionalFieldSet) *schema.NestedSchemaDefinition {
	nsd := &schema.NestedSchemaDefinition{
		Name: name,
		Fields: &schema.NestedSchemaFields{
			FieldsMap: baseFields,
			FieldSets: map[string]schema.ConditionalFieldSet{
				cfsID: cfs,
			},
		},
	}
	return nsd
}

// createMockNestedSchemaSimple creates a simple mock NestedSchemaDefinition without conditional sets
func createMockNestedSchemaSimple(name string, fields map[string]*schema.FieldDefinition) *schema.NestedSchemaDefinition {
	return &schema.NestedSchemaDefinition{
		Name: name,
		Fields: &schema.NestedSchemaFields{
			FieldsMap: fields,
		},
	}
}


func TestValidateConditionalFieldSetThroughValidateAll(t *testing.T) {
	// Base mock nested schemas and root fields for various test cases
	baseRootFields := map[string]*schema.FieldDefinition{
		"rootField1_id": {Name: "rootField1", Type: schema.FieldTypeString},
	}

	baseNestedSchemas := map[string]*schema.NestedSchemaDefinition{
		"referencedNsd": createMockNestedSchemaSimple("referencedNsd", map[string]*schema.FieldDefinition{"refField_id": {Name: "refField", Type: schema.FieldTypeString}}),
	}

	tests := []struct {
		name             string
		rootSchema       *schema.SchemaDefinition
		expectedErrCodes []string
	}{
		{
			name: "Valid ConditionalFieldSet - No When, local fields only",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil, // No base fields in NSD
						"set1",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldA_id": {Name: "fieldA", Type: schema.FieldTypeString},
							},
						},
					),
				},
			),
			expectedErrCodes: []string{},
		},
		{
			name: "Valid ConditionalFieldSet - With When, local Nsd field",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{"nsdStatus_id": {Name: "nsdStatus", Type: schema.FieldTypeString}}, // Base field in NSD
						"set2",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldB_id": {Name: "fieldB", Type: schema.FieldTypeInteger},
							},
							When: &schema.FieldInclusionCondition{
								Field: "nsdStatus_id", // References a base field in 'myNested' NSD
								Value: "active",
							},
						},
					),
				},
			),
			expectedErrCodes: []string{},
		},
		{
			name: "Empty Fields Map in ConditionalFieldSet",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil,
						"set3",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalFieldsEmpty.Code},
		},
		{
			name: "Invalid FieldDefinition within ConditionalFieldSet Fields (empty FieldID)",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil,
						"set4",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"": {Name: "invalidField", Type: schema.FieldTypeString}, // Empty field ID
							},
						},
					),
				},
			),
			expectedErrCodes: []string{fmt.Sprintf("%s", schema.ErrFieldIDEmpty.Code)}, // This error code is defined as ErrFieldIDEmpty.Code in core/schema/errors.go
		},
		{
			name: "When.Field is empty",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{"nsdStatus_id": {Name: "nsdStatus", Type: schema.FieldTypeString}},
						"set5",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldC_id": {Name: "fieldC", Type: schema.FieldTypeBoolean},
							},
							When: &schema.FieldInclusionCondition{
								Field: "",
								Value: true,
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalWhenFieldEmpty.Code},
		},
		{
			name: "When.Field is not a valid identifier",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{"nsdStatus_id": {Name: "nsdStatus", Type: schema.FieldTypeString}},
						"set6",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldD_id": {Name: "fieldD", Type: schema.FieldTypeNumber},
							},
							When: &schema.FieldInclusionCondition{
								Field: "invalid-id", // Invalid field ID
								Value: 10.5,
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalWhenFieldInvalidIdentifier.Code},
		},
		{
			name: "When.Field references a non-existent local Nsd field",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{"nsdStatus_id": {Name: "nsdStatus", Type: schema.FieldTypeString}},
						"set7",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldE_id": {Name: "fieldE", Type: schema.FieldTypeString},
							},
							When: &schema.FieldInclusionCondition{
								Field: "nonExistentField_id", // Not in 'myNestedNsd' base fields
								Value: "value",
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalWhenFieldNotFound.Code},
		},
		{
			name: "When.Field references a field within the conditional set itself (circular)",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil,
						"set8",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"status_id": {Name: "status", Type: schema.FieldTypeString}, // Same ID as When.Field
								"fieldF_id": {Name: "fieldF", Type: schema.FieldTypeString},
							},
							When: &schema.FieldInclusionCondition{
								Field: "status_id",
								Value: "pending",
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalWhenFieldSelfReference.Code},
		},
		{
			name: "When.Value type incompatibility with local Nsd field",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{"amount_id": {Name: "amount", Type: schema.FieldTypeNumber}}, // Base field in NSD
						"set9",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"fieldG_id": {Name: "fieldG", Type: schema.FieldTypeString},
							},
							When: &schema.FieldInclusionCondition{
								Field: "amount_id",      // Nsd field type Number
								Value: "notANumber", // Incompatible string value
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalWhenValueTypeMismatch.Code},
		},
		{
			name: "Conditional field redefines base field in Nsd",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						map[string]*schema.FieldDefinition{
							"nsdStatus_id":       {Name: "nsdStatus", Type: schema.FieldTypeString}, // Base field
							"someOtherNsdField_id": {Name: "someOtherNsdField", Type: schema.FieldTypeString},
						},
						"set10",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"nsdStatus_id": {Name: "nsdStatus", Type: schema.FieldTypeString}, // Collides with base field
							},
							When: &schema.FieldInclusionCondition{
								Field: "someOtherNsdField_id",
								Value: "value",
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrConditionalFieldRedefinesBaseField.Code},
		},
		{
			name: "Conditional field with nested schema reference (valid)",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil,
						"set11",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"complexField_id": {
									Name: "complexField",
									Type: schema.FieldTypeObject,
									Schema: schema.NestedSchemaReference{
										ID: "referencedNsd", // References a nested schema in rootSchema.NestedSchemas
									},
								},
							},
						},
					),
					// Ensure referencedNsd is in the root's NestedSchemas
					"referencedNsd": baseNestedSchemas["referencedNsd"],
				},
			),
			expectedErrCodes: []string{},
		},
		{
			name: "Conditional field with nested schema reference (non-existent)",
			rootSchema: createMockRootSchema(
				baseRootFields,
				map[string]*schema.NestedSchemaDefinition{
					"myNested": createMockNestedSchemaWithConditionalSet(
						"myNested",
						nil,
						"set12",
						schema.ConditionalFieldSet{
							Fields: map[string]*schema.FieldDefinition{
								"badComplexField_id": {
									Name: "badComplexField",
									Type: schema.FieldTypeObject,
									Schema: schema.NestedSchemaReference{
										ID: "nonExistentNestedSchemaRef", // Non-existent reference
									},
								},
							},
						},
					),
				},
			),
			expectedErrCodes: []string{schema.ErrUnknownNestedSchemaReference.Code},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := tt.rootSchema.ValidateAll()

			if len(tt.expectedErrCodes) == 0 {
				assert.Empty(t, issues, "Expected no issues")
			} else {
				assert.NotEmpty(t, issues, "Expected issues")
				actualErrCodes := make([]string, len(issues))
				for i, issue := range issues {
					actualErrCodes[i] = issue.Code
				}
				assert.ElementsMatch(t, tt.expectedErrCodes, actualErrCodes, "Expected and actual error codes should match")
			}
		})
	}
}

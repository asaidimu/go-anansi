package schema_test

import (
	"embed"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

var schemasFS embed.FS

func TestSchemaDefinition_JsonSchema(t *testing.T) {

	t.Run("should validate json", func(t *testing.T) {

		// Helper function to validate a JSON string
		validateJSON := func(t *testing.T, jsonStr string, shouldPass bool) {
			_, err := schema.From([]byte(jsonStr))
			if shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		}

		// Valid test cases
		t.Run("valid - minimal schema", func(t *testing.T) {
			jsonStr := `{
				"name": "Minimal",
				"version": "1.0.0",
				"fields": {}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - basic fields", func(t *testing.T) {
			jsonStr := `{
				"name": "Carts",
				"version": "1.0.0",
				"fields": {
					"user_id": {
						"name": "user_id",
						"type": "string",
						"required": true
					},
					"product_ids": {
						"name": "product_ids",
						"type": "array",
						"itemsType": "string",
						"required": true
					},
					"quantity": {
						"name": "quantity",
						"type": "integer",
						"required": true
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with description and metadata", func(t *testing.T) {
			jsonStr := `{
				"name": "Users",
				"version": "1.0.0",
				"description": "User schema",
				"metadata": {"key": "value"},
				"fields": {}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with concrete", func(t *testing.T) {
			jsonStr := `{
				"name": "Products",
				"version": "1.0.0",
				"concrete": true,
				"fields": {}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with registry", func(t *testing.T) {
			jsonStr := `{
				"name": "Orders",
				"version": "1.0.0",
				"fields": {},
				"registry": {
					"schemas": {
						"nested": {
							"name": "Nested",
							"fields": {
								"field1": {
									"name": "field1",
									"type": "string"
								}
							}
						}
					},
					"constraints": {
						"constraint1": {
							"name": "constraint1",
							"predicate": "true"
						}
					},
					"indexes": {
						"index1": {
							"name": "index1",
							"fields": ["field1"],
							"type": "normal"
						}
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with indexes", func(t *testing.T) {
			jsonStr := `{
				"name": "Items",
				"version": "1.0.0",
				"fields": {},
				"indexes": [
					{
						"name": "idx1",
						"fields": ["user_id"],
						"type": "unique"
					}
				]
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with constraints", func(t *testing.T) {
			jsonStr := `{
				"name": "Payments",
				"version": "1.0.0",
				"fields": {},
				"constraints": [
					{
						"name": "const1",
						"predicate": "amount > 0"
					}
				]
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with migrations", func(t *testing.T) {
			jsonStr := `{
				"name": "MigrationsTest",
				"version": "1.0.0",
				"fields": {},
				"migrations": [
					{
						"id": "mig1",
						"version": {"source": "1.0.0"},
						"changes": [],
						"transform": "transformFunc",
						"createdAt": "2023-01-01T00:00:00Z",
						"checksum": "abc123"
					}
				]
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with hints", func(t *testing.T) {
			jsonStr := `{
				"name": "HintsTest",
				"version": "1.0.0",
				"fields": {},
				"hints": {"hint1": "value"}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with deprecated dependencies", func(t *testing.T) {
			jsonStr := `{
				"name": "DepsTest",
				"version": "1.0.0",
				"fields": {},
				"dependencies": ["dep1", "dep2"]
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - with deprecated nestedSchemas", func(t *testing.T) {
			jsonStr := `{
				"name": "NestedTest",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {
					"nested1": {
						"name": "nested1",
						"type": "string"
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - field with enum values", func(t *testing.T) {
			jsonStr := `{
				"name": "EnumTest",
				"version": "1.0.0",
				"fields": {
					"status": {
						"name": "status",
						"type": "enum",
						"values": ["active", "inactive"]
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - field with object schema", func(t *testing.T) {
			jsonStr := `{
				"name": "ObjectTest",
				"version": "1.0.0",
				"fields": {
					"details": {
						"name": "details",
						"type": "object",
						"schema": {"id": "schema1"}
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - complex nested schema", func(t *testing.T) {
			jsonStr := `{
				"name": "ComplexNested",
				"version": "1.0.0",
				"fields": {},
				"registry": {
					"schemas": {
						"nested": {
							"name": "Nested",
							"type": "object",
							"schema": {"id": "subschema"}
						}
					}
				}
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - constraint group", func(t *testing.T) {
			jsonStr := `{
				"name": "ConstraintGroup",
				"version": "1.0.0",
				"fields": {},
				"constraints": [
					{
						"name": "group1",
						"operator": "and",
						"rules": [
							{"name": "rule1", "predicate": "true"},
							{"id": "ref1"}
						]
					}
				]
			}`
			validateJSON(t, jsonStr, true)
		})

		t.Run("valid - partial index", func(t *testing.T) {
			jsonStr := `{
				"name": "PartialIndex",
				"version": "1.0.0",
				"fields": {},
				"indexes": [
					{
						"name": "partialIdx",
						"fields": ["field1"],
						"type": "normal",
						"partial": {
							"operator": "and",
							"field": "status",
							"value": "active"
						}
					}
				]
			}`
			validateJSON(t, jsonStr, true)
		})

		// Invalid test cases
		t.Run("invalid - missing name", func(t *testing.T) {
			jsonStr := `{
				"version": "1.0.0",
				"fields": {}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - missing version", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"fields": {}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - missing fields", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0"
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - additional property", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"extra": "property"
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - name not string", func(t *testing.T) {
			jsonStr := `{
				"name": 123,
				"version": "1.0.0",
				"fields": {}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - fields not object", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": "notobject"
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field missing name", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {
					"user_id": {
						"type": "string"
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field missing type", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {
					"user_id": {
						"name": "user_id"
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field invalid type", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {
					"user_id": {
						"name": "user_id",
						"type": "invalidtype"
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field additional property", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {
					"user_id": {
						"name": "user_id",
						"type": "string",
						"extra": "prop"
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - registry with additional property", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"registry": {
					"extra": "prop"
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - index missing name", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"indexes": [
					{
						"fields": ["user_id"],
						"type": "normal"
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - constraint missing predicate", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"constraints": [
					{
						"name": "const1"
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - migration missing id", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"migrations": [
					{
						"version": {"source": "1.0.0"},
						"changes": [],
						"transform": "func",
						"createdAt": "2023-01-01T00:00:00Z",
						"checksum": "abc"
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - nested schema missing name", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"nestedSchemas": {
					"nested": {
						"fields": {}
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field schema missing id", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {
					"obj": {
						"name": "obj",
						"type": "object",
						"schema": {}
					}
				}
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - partial index invalid operator", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"indexes": [
					{
						"name": "idx",
						"fields": ["f1"],
						"type": "normal",
						"partial": {
							"operator": "invalid",
							"field": "status"
						}
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - schema change invalid type", func(t *testing.T) {
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"migrations": [
					{
						"id": "mig1",
						"version": {"source": "1.0.0"},
						"changes": [
							{"type": "invalidchange"}
						],
						"transform": "func",
						"createdAt": "2023-01-01T00:00:00Z",
						"checksum": "abc"
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})

		t.Run("invalid - field patch with invalid type", func(t *testing.T) {
			// Note: This tests indirectly through modifyField in schemaChange, but since schemaChange uses fieldPatch
			jsonStr := `{
				"name": "Invalid",
				"version": "1.0.0",
				"fields": {},
				"migrations": [
					{
						"id": "mig1",
						"version": {"source": "1.0.0"},
						"changes": [
							{
								"type": "modifyField",
								"id": "field1",
								"changes": {
									"type": "invalidtype"
								}
							}
						],
						"transform": "func",
						"createdAt": "2023-01-01T00:00:00Z",
						"checksum": "abc"
					}
				]
			}`
			validateJSON(t, jsonStr, false)
		})
	})
}

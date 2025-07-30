package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

func TestConstraintPropagation(t *testing.T) {
	trueBool := true
	falseBool := false
	stringType := schema.FieldTypeString
	objectType := schema.FieldTypeObject
	integerType := schema.FieldTypeInteger

	// Custom predicate for testing
	fmap := schema.FunctionMap{
		"isPositive": func(params schema.PredicateParams[any]) bool {
			if params.Field != nil {
				dataMap, ok := params.Data.(map[string]any)
				if ok {
					if val, exists := dataMap[*params.Field]; exists {
						if intVal, ok := val.(int); ok {
							return intVal > 0
						}
					}
				}
				return false
			}
			if val, ok := params.Data.(int); ok {
				return val > 0
			}
			return false
		},
		"startsWithA": func(params schema.PredicateParams[any]) bool {
			if params.Field != nil {
				dataMap, ok := params.Data.(map[string]any)
				if ok {
					if val, exists := dataMap[*params.Field]; exists {
						if strVal, ok := val.(string); ok {
							return len(strVal) > 0 && strVal[0] == 'A'
						}
					}
				}
				return false
			}
			if val, ok := params.Data.(string); ok {
				return len(val) > 0 && val[0] == 'A'
			}
			return false
		},
		"isLongEnough": func(params schema.PredicateParams[any]) bool {
			if params.Field != nil {
				if dataMap, ok := params.Data.(map[string]any); ok {
					if val, exists := dataMap[*params.Field]; exists {
						if strVal, ok := val.(string); ok {
							return len(strVal) >= 5
						}
					}
				}
				return false
			}
			if val, ok := params.Data.(string); ok {
				return len(val) >= 5
			}
			return false
		},
		"hasEvenLength": func(params schema.PredicateParams[any]) bool {
			if params.Field != nil {
				if dataMap, ok := params.Data.(map[string]any); ok {
					if val, exists := dataMap[*params.Field]; exists {
						if strVal, ok := val.(string); ok {
							return len(strVal)%2 == 0
						}
					}
				}
				return false
			}
			if val, ok := params.Data.(string); ok {
				return len(val)%2 == 0
			}
			return false
		},
		"isGlobalValid": func(params schema.PredicateParams[any]) bool {
			if dataMap, ok := params.Data.(map[string]any); ok {
				if globalVal, exists := dataMap["globalCheckField"]; exists {
					return globalVal == "globalValue"
				}
			}
			return false
		},
	}

	// --- 1. Field-Level Constraints ---
	t.Run("Field-Level Constraint - Pass", func(t *testing.T) {
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"age": {
					Name: "age", Type: schema.FieldTypeInteger,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "positiveAge", Predicate: "isPositive"},
					},
				},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{"age": 10}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Field-Level Constraint - Fail", func(t *testing.T) {
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"age": {
					Name: "age", Type: schema.FieldTypeInteger,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "positiveAge", Predicate: "isPositive"},
					},
				},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{"age": -5}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "age", issues[0].Path)
		assert.Equal(t, "Constraint 'positiveAge' failed", issues[0].Message)
	})

	// --- 2. Nested Schema Reference Constraints ---
	t.Run("Nested Schema Reference Constraint - Pass", func(t *testing.T) {
		addressSchema := &schema.NestedSchemaDefinition{
			Name: "address", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"street": {Name: "street", Type: schema.FieldTypeString},
				"city":   {Name: "city", Type: schema.FieldTypeString},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"homeAddress": {
					Name: "homeAddress", Type: schema.FieldTypeObject,
					Schema: schema.NestedSchemaReference{
						ID: "address",
						Constraints: schema.SchemaConstraint[schema.FieldType]{
							schema.Constraint[schema.FieldType]{Name: "cityStartsWithA", Predicate: "startsWithA", Field: &[]string{"city"}[0]},
						},
					},
				},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"address": addressSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"homeAddress": map[string]any{"street": "123 Main St", "city": "Anytown"},
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Nested Schema Reference Constraint - Fail", func(t *testing.T) {
		addressSchema := &schema.NestedSchemaDefinition{
			Name: "address", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"street": {Name: "street", Type: schema.FieldTypeString},
				"city":   {Name: "city", Type: schema.FieldTypeString},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"homeAddress": {
					Name: "homeAddress", Type: schema.FieldTypeObject,
					Schema: schema.NestedSchemaReference{
						ID: "address",
						Constraints: schema.SchemaConstraint[schema.FieldType]{
							schema.Constraint[schema.FieldType]{Name: "cityStartsWithA", Predicate: "startsWithA", Field: &[]string{"city"}[0]},
						},
					},
				},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"address": addressSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"homeAddress": map[string]any{"street": "123 Main St", "city": "Zebulon"},
		}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "homeAddress.city", issues[0].Path)
		assert.Equal(t, "Constraint 'cityStartsWithA' failed", issues[0].Message)
	})

	// --- 3. Nested Schema Definition Constraints (Literal Type) ---
	t.Run("Field Constraint (Literal) - Fail", func(t *testing.T) {
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"tag": {
					Name: "tag",
					Type: schema.FieldTypeString,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
					},
				},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{"tag": "sho"}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "tag", issues[0].Path)
		assert.Equal(t, "Constraint 'isLongEnough' failed", issues[0].Message)
	})

	// --- 4. Constraints on Fields within a Structured Nested Schema Definition ---
	t.Run("Structured Nested Schema Field Constraint - Pass", func(t *testing.T) {
		personSchema := &schema.NestedSchemaDefinition{
			Name: "person", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"firstName": {Name: "firstName", Type: schema.FieldTypeString, Required: &trueBool},
				"lastName": {
					Name: "lastName", Type: schema.FieldTypeString,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "hasEvenLength", Predicate: "hasEvenLength"},
					},
				},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"user": {Name: "user", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "person"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"person": personSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"user": map[string]any{"firstName": "John", "lastName": "Doee"},
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Structured Nested Schema Field Constraint - Fail", func(t *testing.T) {
		personSchema := &schema.NestedSchemaDefinition{
			Name: "person", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"firstName": {Name: "firstName", Type: schema.FieldTypeString, Required: &trueBool},
				"lastName": {
					Name: "lastName", Type: schema.FieldTypeString,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "hasEvenLength", Predicate: "hasEvenLength"},
					},
				},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"user": {Name: "user", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "person"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"person": personSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"user": map[string]any{"firstName": "Jane", "lastName": "Smit"},
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	// --- 5. Schema-Level Constraints ---
	t.Run("Schema-Level Constraint - Pass", func(t *testing.T) {
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"globalCheckField": {Name: "globalCheckField", Type: schema.FieldTypeString},
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "globaldation", Predicate: "isGlobalValid"},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{"globalCheckField": "globalValue"}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Schema-Level Constraint - Fail", func(t *testing.T) {
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"globalCheckField": {Name: "globalCheckField", Type: schema.FieldTypeString},
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "globaldation", Predicate: "isGlobalValid"},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{"globalCheckField": "wrongValue"}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "", issues[0].Path) // Schema-level constraints have empty path
		assert.Equal(t, "Constraint 'globaldation' failed", issues[0].Message)
	})

	// --- 3. Nested Schema Definition Constraints (Structured Type) ---
	// This test will initially fail if NestedSchemaDefinition.Constraints are not applied to structured types.
	t.Run("Nested Schema Definition Constraint (Structured) - Fail", func(t *testing.T) {
		constrainedObjectSchema := &schema.NestedSchemaDefinition{
			Name: "constrainedObject", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"id":   {Name: "id", Type: schema.FieldTypeString, Required: &trueBool},
				"data": {Name: "data", Type: schema.FieldTypeString},
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "idStartsWithA", Predicate: "startsWithA", Field: &[]string{"id"}[0]},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"item": {Name: "item", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "constrainedObject"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"constrainedObject": constrainedObjectSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"item": map[string]any{"id": "badId", "data": "some data"},
		}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "item.id", issues[0].Path)
		assert.Equal(t, "Constraint 'idStartsWithA' failed", issues[0].Message)
	})

	// --- Combined Scenarios ---
	t.Run("Combined Constraints - Pass", func(t *testing.T) {
		// Schema: user -> account (object) -> permissions (array of objects)
		// Field-level: username (isLongEnough)
		// Nested Ref: account (cityStartsWithA)
		// Nested Def (structured): permission (resource isLongEnough)
		// Schema-level: isGlobalValid

		permissionSchema := &schema.NestedSchemaDefinition{
			Name: "permission", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"resource": {Name: "resource", Type: schema.FieldTypeString, Required: &trueBool,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "resourceLongEnough", Predicate: "isLongEnough"},
					},
				},
				"level": {Name: "level", Type: schema.FieldTypeString, Required: &trueBool},
			},
		}
		accountSchema := &schema.NestedSchemaDefinition{
			Name: "account", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"name":        {Name: "name", Type: schema.FieldTypeString},
				"city":        {Name: "city", Type: schema.FieldTypeString},
				"permissions": {Name: "permissions", Type: schema.FieldTypeArray, ItemsType: &objectType, Schema: schema.NestedSchemaReference{ID: "permission"}},
			},
		}

		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"username": {Name: "username", Type: schema.FieldTypeString, Required: &trueBool,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "usernameLongEnough", Predicate: "isLongEnough"},
					},
				},
				"account": {
					Name: "account", Type: schema.FieldTypeObject,
					Schema: schema.NestedSchemaReference{
						ID: "account",
						Constraints: schema.SchemaConstraint[schema.FieldType]{
							schema.Constraint[schema.FieldType]{Name: "accountCityStartsWithA", Predicate: "startsWithA", Field: &[]string{"city"}[0]},
						},
					},
				},
				"globalCheckField": {Name: "globalCheckField", Type: schema.FieldTypeString},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{
				"permission": permissionSchema,
				"account":    accountSchema,
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "globalCheck", Predicate: "isGlobalValid"},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"username": "longusername",
			"account": map[string]any{
				"name": "admin",
				"city": "Anytown",
				"permissions": []any{
					map[string]any{"resource": "users", "level": "admin"},
					map[string]any{"resource": "posts", "level": "write"},
				},
			},
			"globalCheckField": "globalValue",
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Combined Constraints - Fail", func(t *testing.T) {
		// Schema: user -> account (object) -> permissions (array of objects)
		// Field-level: username (isLongEnough)
		// Nested Ref: account (cityStartsWithA)
		// Nested Def (structured): permission (resource isLongEnough)
		// Schema-level: isGlobalValid

		permissionSchema := &schema.NestedSchemaDefinition{
			Name: "permission", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"resource": {Name: "resource", Type: schema.FieldTypeString, Required: &trueBool,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "resourceLongEnough", Predicate: "isLongEnough"},
					},
				},
				"level": {Name: "level", Type: schema.FieldTypeString, Required: &trueBool},
			},
		}
		accountSchema := &schema.NestedSchemaDefinition{
			Name: "account", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"name":        {Name: "name", Type: schema.FieldTypeString},
				"city":        {Name: "city", Type: schema.FieldTypeString},
				"permissions": {Name: "permissions", Type: schema.FieldTypeArray, ItemsType: &objectType, Schema: schema.NestedSchemaReference{ID: "permission"}},
			},
		}

		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"username": {Name: "username", Type: schema.FieldTypeString, Required: &trueBool,
					Constraints: schema.SchemaConstraint[schema.FieldType]{
						schema.Constraint[schema.FieldType]{Name: "usernameLongEnough", Predicate: "isLongEnough"},
					},
				},
				"account": {
					Name: "account", Type: schema.FieldTypeObject,
					Schema: schema.NestedSchemaReference{
						ID: "account",
						Constraints: schema.SchemaConstraint[schema.FieldType]{
							schema.Constraint[schema.FieldType]{Name: "accountCityStartsWithA", Predicate: "startsWithA", Field: &[]string{"city"}[0]},
						},
					},
				},
				"globalCheckField": {Name: "globalCheckField", Type: schema.FieldTypeString},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{
				"permission": permissionSchema,
				"account":    accountSchema,
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "globalCheck", Predicate: "isGlobalValid"},
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"username": "ben", // Fails usernameLongEnough
			"account": map[string]any{
				"name": "admin",
				"city": "Zebulon", // Fails accountCityStartsWithA
				"permissions": []any{
					map[string]any{"resource": "usr", "level": "admin"}, // Fails resourceLongEnough
				},
			},
			"globalCheckField": "wrongValue", // Fails globalCheck
		}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 4)
		// Check specific errors
		assert.Contains(t, issues, schema.Issue{Code: "CONSTRAINT_VIOLATION", Path: "username", Message: "Constraint 'usernameLongEnough' failed"})
		assert.Contains(t, issues, schema.Issue{Code: "CONSTRAINT_VIOLATION", Path: "account.city", Message: "Constraint 'accountCityStartsWithA' failed"})
		assert.Contains(t, issues, schema.Issue{Code: "CONSTRAINT_VIOLATION", Path: "account.permissions[0].resource", Message: "Constraint 'resourceLongEnough' failed"})
		assert.Contains(t, issues, schema.Issue{Code: "CONSTRAINT_VIOLATION", Path: "", Message: "Constraint 'globalCheck' failed"})
	})

	// --- Nested Schema Definition Constraints (Structured Type) - Pass ---
	t.Run("Nested Schema Definition Constraint (Structured) - Pass", func(t *testing.T) {
		constrainedObjectSchema := &schema.NestedSchemaDefinition{
			Name: "constrainedObject", IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"id":   {Name: "id", Type: schema.FieldTypeString, Required: &trueBool},
				"data": {Name: "data", Type: schema.FieldTypeString},
			},
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "idStartsWithA", Predicate: "startsWithA", Field: &[]string{"id"}[0]},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"item": {Name: "item", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "constrainedObject"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"constrainedObject": constrainedObjectSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"item": map[string]any{"id": "A1234", "data": "some data"},
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	// --- 6. Constraints on Conditional Fields (When Cases) ---
	t.Run("Conditional Field Constraint - Pass", func(t *testing.T) {
		conditionalSchema := &schema.NestedSchemaDefinition{
			Name: "conditionalSchema", IsStructured: &trueBool,
			StructuredFieldsArray: []struct {
				Fields map[string]*schema.FieldDefinition `json:"fields"`
				When   *schema.FieldInclusionCondition    `json:"when,omitempty"`
			}{
				{
					Fields: map[string]*schema.FieldDefinition{
						"type": {Name: "type", Type: schema.FieldTypeString},
					},
				},
				{
					When: &schema.FieldInclusionCondition{Field: "type", Value: "email"},
					Fields: map[string]*schema.FieldDefinition{
						"emailAddress": {
							Name: "emailAddress", Type: schema.FieldTypeString,
							Constraints: schema.SchemaConstraint[schema.FieldType]{
								schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
							},
						},
					},
				},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"contact": {Name: "contact", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "conditionalSchema"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"conditionalSchema": conditionalSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)

		data := map[string]any{
			"contact": map[string]any{
				"type":         "email", // does not have a when, is included
				"emailAddress": "test@example.com", // Long enough, should be included because type is email
			},
		}
		issues, ok := validator.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Conditional Field Constraint - Fail", func(t *testing.T) {
		conditionalSchema := &schema.NestedSchemaDefinition{
			Name: "conditionalSchema", IsStructured: &trueBool,
			StructuredFieldsArray: []struct {
				Fields map[string]*schema.FieldDefinition `json:"fields"`
				When   *schema.FieldInclusionCondition    `json:"when,omitempty"`
			}{
				{
					Fields: map[string]*schema.FieldDefinition{
						"type": {Name: "type", Type: schema.FieldTypeString},
					},
				},
				{
					When: &schema.FieldInclusionCondition{Field: "type", Value: "email"},
					Fields: map[string]*schema.FieldDefinition{
						"emailAddress": {
							Name: "emailAddress", Type: schema.FieldTypeString,
							Constraints: schema.SchemaConstraint[schema.FieldType]{
								schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
							},
						},
					},
				},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"contact": {Name: "contact", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "conditionalSchema"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"conditionalSchema": conditionalSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)

		data := map[string]any{
			"contact": map[string]any{
				"type":         "email",
				"emailAddress": "two", // Not long enough, should be 5 or more
			},
		}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "contact.emailAddress", issues[0].Path)
		assert.Equal(t, "Constraint 'isLongEnough' failed", issues[0].Message)
	})

	t.Run("Conditional Field Constraint - Applied When Condition Not Met", func(t *testing.T) {
		conditionalSchema := &schema.NestedSchemaDefinition{
			Name: "conditionalSchema", IsStructured: &trueBool,
			StructuredFieldsArray: []struct {
				Fields map[string]*schema.FieldDefinition `json:"fields"`
				When   *schema.FieldInclusionCondition    `json:"when,omitempty"`
			}{
				{
					Fields: map[string]*schema.FieldDefinition{
						"type": {Name: "type", Type: schema.FieldTypeString},
					},
				},
				{
					When: &schema.FieldInclusionCondition{Field: "type", Value: "email"},
					Fields: map[string]*schema.FieldDefinition{
						"emailAddress": {
							Name: "emailAddress", Type: schema.FieldTypeString,
							Constraints: schema.SchemaConstraint[schema.FieldType]{
								schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
							},
						},
					},
				},
			},
		}
		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"contact": {Name: "contact", Type: schema.FieldTypeObject, Schema: schema.NestedSchemaReference{ID: "conditionalSchema"}},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{"conditionalSchema": conditionalSchema},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)

		data := map[string]any{
			"contact": map[string]any{
				"type":         "phone", // Condition not met
				"emailAddress": "short", // Constraint will be applied, expects type to be email
			},
		}
		issues, ok := validator.Validate(data, false)
		assert.False(t, ok) // Should not pass because constraint is applied
		require.Len(t, issues, 2)
		assert.Equal(t, "CONDITIONAL_FIELD_PRESENT", issues[0].Code)
		assert.Equal(t, "UNEXPECTED_FIELD", issues[1].Code)
	})

	// --- 7. Union Field with Literal Nested Schema Constraints ---
	t.Run("Union Field with Literal Nested Schema Constraint - Pass", func(t *testing.T) {
		constrainedStringType := &schema.NestedSchemaDefinition{
			Name: "constrainedString", IsStructured: &falseBool, Type: &stringType,
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
			},
		}
		constrainedIntegerType := &schema.NestedSchemaDefinition{
			Name: "constrainedInteger", IsStructured: &falseBool, Type: &integerType,
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "isPositive", Predicate: "isPositive"},
			},
		}

		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"value": {
					Name: "value", Type: schema.FieldTypeUnion,
					Schema: []schema.NestedSchemaReference{
						{ID: "constrainedString"},
						{ID: "constrainedInteger"},
					},
				},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{
				"constrainedString":  constrainedStringType,
				"constrainedInteger": constrainedIntegerType,
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)

		// Test case 1: String that passes constraint
		data1 := map[string]any{"value": "long_string"}
		issues1, ok1 := validator.Validate(data1, false)
		assert.True(t, ok1)
		assert.Empty(t, issues1)

		// Test case 2: Integer that passes constraint
		data2 := map[string]any{"value": 100}
		issues2, ok2 := validator.Validate(data2, false)
		assert.True(t, ok2)
		assert.Empty(t, issues2)
	})

	t.Run("Union Field with Literal Nested Schema Constraint - Fail", func(t *testing.T) {
		constrainedStringType := &schema.NestedSchemaDefinition{
			Name: "constrainedString", IsStructured: &falseBool, Type: &stringType,
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "isLongEnough", Predicate: "isLongEnough"},
			},
		}
		constrainedIntegerType := &schema.NestedSchemaDefinition{
			Name: "constrainedInteger", IsStructured: &falseBool, Type: &integerType,
			Constraints: schema.SchemaConstraint[schema.FieldType]{
				schema.Constraint[schema.FieldType]{Name: "isPositive", Predicate: "isPositive"},
			},
		}

		schemaDef := &schema.SchemaDefinition{
			Name: "test", Version: "1.0",
			Fields: map[string]*schema.FieldDefinition{
				"value": {
					Name: "value", Type: schema.FieldTypeUnion,
					Schema: []schema.NestedSchemaReference{
						{ID: "constrainedString"},
						{ID: "constrainedInteger"},
					},
				},
			},
			NestedSchemas: map[string]*schema.NestedSchemaDefinition{
				"constrainedString":  constrainedStringType,
				"constrainedInteger": constrainedIntegerType,
			},
		}
		validator, err := schema.NewDocumentValidator(schemaDef, &fmap)
		require.NoError(t, err)

		// Test case 1: String that fails constraint
		data1 := map[string]any{"value": "two"}
		issues1, ok1 := validator.Validate(data1, false)
		assert.False(t, ok1)
		require.Len(t, issues1, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues1[0].Code)
		assert.Equal(t, "value", issues1[0].Path)
		assert.Equal(t, "Constraint 'isLongEnough' failed", issues1[0].Message)

		// Test case 2: Integer that fails constraint
		data2 := map[string]any{"value": -5}
		issues2, ok2 := validator.Validate(data2, false)
		assert.False(t, ok2)
		require.Len(t, issues2, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues2[0].Code)
		assert.Equal(t, "value", issues2[0].Path)
		assert.Equal(t, "Constraint 'isPositive' failed", issues2[0].Message)
	})
}

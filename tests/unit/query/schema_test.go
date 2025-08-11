package query_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func float64p(f float64) *float64 {
	return &f
}

func TestSchemaFromQuery(t *testing.T) {
	// Define base schemas for testing
	userSchema := &schema.SchemaDefinition{
		Name:    "users",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: schema.FieldTypeString},
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"email":   {Name: "email", Type: schema.FieldTypeString},
			"age":     {Name: "age", Type: schema.FieldTypeInteger},
			"isAdmin": {Name: "isAdmin", Type: schema.FieldTypeBoolean},
		},
	}

	orderSchema := &schema.SchemaDefinition{
		Name:    "orders",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":       {Name: "id", Type: schema.FieldTypeString},
			"userId":   {Name: "userId", Type: schema.FieldTypeString},
			"amount":   {Name: "amount", Type: schema.FieldTypeNumber},
			"quantity": {Name: "quantity", Type: schema.FieldTypeInteger},
		},
	}

	t.Run("SimpleSelect", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("users")
		q := qb.Build()
		q.Target.Schema = userSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)
		assert.Len(t, resultSchema.Fields, 5)
		assert.Contains(t, resultSchema.Fields, "name")
	})

	t.Run("SelectWithProjection", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("users").Select().Include("id", "name").Exclude("email").End()
		q := qb.Build()
		q.Target.Schema = userSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)
		assert.Len(t, resultSchema.Fields, 2)
		assert.Contains(t, resultSchema.Fields, "id")
		assert.Contains(t, resultSchema.Fields, "name")
		assert.NotContains(t, resultSchema.Fields, "email")
	})

	t.Run("SelectWithJoin", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("users").
			InnerJoin("orders").
			Alias("user_orders").
			On(query.QueryFilter{
				Condition: &query.FilterCondition{
					Field:    "users.id",
					Operator: query.ComparisonOperatorEq,
					Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "user_orders.userId"}},
				},
			}).
			Schema(orderSchema).
			End()

		q := qb.Build()
		q.Target.Schema = userSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)
		assert.Len(t, resultSchema.Fields, 6) // 5 from users + 1 for the joined orders
		assert.Contains(t, resultSchema.Fields, "user_orders")

		joinedField := resultSchema.Fields["user_orders"]
		assert.Equal(t, schema.FieldTypeObject, joinedField.Type)
		assert.NotNil(t, resultSchema.NestedSchemas)

		nestedSchemaRef, ok := joinedField.Schema.(schema.NestedSchemaReference)
		require.True(t, ok)
		nestedSchema, exists := resultSchema.NestedSchemas[nestedSchemaRef.ID]
		require.True(t, exists)
		assert.Len(t, nestedSchema.StructuredFieldsMap, 4)
		assert.Contains(t, nestedSchema.StructuredFieldsMap, "amount")
	})

	t.Run("SelectWithAggregation", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("orders").
			Sum("amount", "total_revenue").
			Avg("quantity", "avg_quantity")

		q := qb.Build()
		q.Target.Schema = orderSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)
		assert.Len(t, resultSchema.Fields, 2)
		assert.Contains(t, resultSchema.Fields, "total_revenue")
		assert.Contains(t, resultSchema.Fields, "avg_quantity")

		totalRevenueField := resultSchema.Fields["total_revenue"]
		assert.Equal(t, schema.FieldTypeNumber, totalRevenueField.Type)

		avgQuantityField := resultSchema.Fields["avg_quantity"]
		assert.Equal(t, schema.FieldTypeNumber, avgQuantityField.Type)
	})

	t.Run("SelectWithGroupBy", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("orders").
			GroupBy("userId").
			WithFilter(query.QueryFilter{
				Condition: &query.FilterCondition{
					Field:    "amount",
					Operator: query.ComparisonOperatorGt,
					Value:    query.FilterValue{NumberVal: float64p(100)},
				},
			}).
			End()

		q := qb.Build()
		q.Target.Schema = orderSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)
		assert.Len(t, resultSchema.Fields, 1)
		assert.Contains(t, resultSchema.Fields, "aggregation_results")
	})

	t.Run("SchemaMatchesExpectedFormat", func(t *testing.T) {
		qb := query.NewQueryBuilder()
		qb.From("users").
			Select().
			Include("id", "name").
			End()
		q := qb.Build()
		q.Target.Schema = userSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)

		expectedSchema := &schema.SchemaDefinition{
			Name:        "users_projected_result",
			Version:     "1.0.0",
			Description: "Generated schema for query result",
			Fields: map[string]*schema.FieldDefinition{
				"id":   {Name: "id", Type: schema.FieldTypeString},
				"name": {Name: "name", Type: schema.FieldTypeString},
			},
		}

		assert.Equal(t, expectedSchema.Name, resultSchema.Name)
		assert.Equal(t, expectedSchema.Version, resultSchema.Version)
		assert.Equal(t, expectedSchema.Description, resultSchema.Description)
		assert.Equal(t, len(expectedSchema.Fields), len(resultSchema.Fields))
		for name, field := range expectedSchema.Fields {
			resultField, ok := resultSchema.Fields[name]
			require.True(t, ok)
			assert.Equal(t, field.Name, resultField.Name)
			assert.Equal(t, field.Type, resultField.Type)
		}
	})

	t.Run("SelectWithNestedJoins", func(t *testing.T) {
		contactSchema := &schema.SchemaDefinition{
			Name: "contacts",
			Fields: map[string]*schema.FieldDefinition{
				"id":      {Name: "id", Type: schema.FieldTypeString},
				"phone":   {Name: "phone", Type: schema.FieldTypeString},
				"address": {Name: "address", Type: schema.FieldTypeString},
			},
		}

		profileSchema := &schema.SchemaDefinition{
			Name: "profiles",
			Fields: map[string]*schema.FieldDefinition{
				"id":         {Name: "id", Type: schema.FieldTypeString},
				"userId":     {Name: "userId", Type: schema.FieldTypeString},
				"contactId":  {Name: "contactId", Type: schema.FieldTypeString},
				"department": {Name: "department", Type: schema.FieldTypeString},
			},
		}

		qb := query.NewQueryBuilder()
		qb.From("users_001").
			Alias("users").
			InnerJoin("profiles_001").
			Alias("profile").
			On(query.QueryFilter{
				Condition: &query.FilterCondition{
					Field:    "users.id",
					Operator: query.ComparisonOperatorEq,
					Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "profile.userId"}},
				},
			}).
			Schema(profileSchema).
			End().
			InnerJoin("contacts").
			Alias("profile.contact").
			On(query.QueryFilter{
				Condition: &query.FilterCondition{
					Field:    "profile.contactId",
					Operator: query.ComparisonOperatorEq,
					Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "profile.contact.id"}},
				},
			}).
			Schema(contactSchema).
			End()

		q := qb.Build()
		q.Target.Schema = userSchema

		resultSchema, err := query.SchemaFromQuery(&q, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultSchema)

		// 5 from users + 1 for profile join
		assert.Len(t, resultSchema.Fields, 6)
		assert.Contains(t, resultSchema.Fields, "profile")

		profileField := resultSchema.Fields["profile"]
		assert.Equal(t, schema.FieldTypeObject, profileField.Type)

		profileSchemaRef, ok := profileField.Schema.(schema.NestedSchemaReference)
		require.True(t, ok)
		nestedProfileSchema, exists := resultSchema.FindNestedSchema(profileSchemaRef.ID)
		require.True(t, exists)

		// 4 from profiles + 1 for contact join
		assert.Len(t, nestedProfileSchema.StructuredFieldsMap, 5)
		assert.Contains(t, nestedProfileSchema.StructuredFieldsMap, "contact")

		contactField := nestedProfileSchema.StructuredFieldsMap["contact"]
		assert.Equal(t, schema.FieldTypeObject, contactField.Type)

		contactSchemaRef, ok := contactField.Schema.(schema.NestedSchemaReference)
		require.True(t, ok)
		nestedContactSchema, exists := resultSchema.FindNestedSchema(contactSchemaRef.ID)
		require.True(t, exists)

		assert.Len(t, nestedContactSchema.StructuredFieldsMap, 3)
		assert.Contains(t, nestedContactSchema.StructuredFieldsMap, "phone")

		// Validate the schema with a sample document
		doc := common.Document{
			"id":      "user-123",
			"name":    "John Doe",
			"email":   "john.doe@example.com",
			"age":     30,
			"isAdmin": false,
			"profile": common.Document{
				"id":         "profile-456",
				"userId":     "user-123",
				"contactId":  "contact-789",
				"department": "Engineering",
				"contact": common.Document{
					"id":      "contact-789",
					"phone":   "123-456-7890",
					"address": "123 Main St",
				},
			},
		}

		validator, err := schema.NewDocumentValidator(resultSchema, nil)
		require.NoError(t, err)

		issues, ok := validator.Validate(doc, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})
}

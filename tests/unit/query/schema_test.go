package query_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func float64p(f float64) *float64 {
	return &f
}

func TestSchemaFromQuery(t *testing.T) {
	// Define base schemas for testing
	userSchema := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":    {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":   {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":     {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"isAdmin": {Name: "isAdmin", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
		},
	}

	orderSchema := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":       {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"userId":   {Name: "userId", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"amount":   {Name: "amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"quantity": {Name: "quantity", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("name"))
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("id"))
		assert.Contains(t, resultSchema.Fields, definition.FieldId("name"))
		assert.NotContains(t, resultSchema.Fields, definition.FieldId("email"))
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("user_orders"))

		joinedField := resultSchema.Fields["user_orders"]
		assert.Equal(t, definition.FieldTypeObject, joinedField.Type)
		assert.NotNil(t, resultSchema.Schemas)

		nestedSchemaRef, err := definition.FieldSchemaAs[definition.SchemaReference](joinedField.Schema)
		require.NoError(t, err)
		nestedSchema, exists := resultSchema.Schemas[nestedSchemaRef.ID]
		require.True(t, exists)
		assert.Len(t, nestedSchema.Fields, 4)
		assert.Contains(t, nestedSchema.Fields, definition.FieldId("amount"))
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("total_revenue"))
		assert.Contains(t, resultSchema.Fields, definition.FieldId("avg_quantity"))

		totalRevenueField := resultSchema.Fields["total_revenue"]
		assert.Equal(t, definition.FieldTypeNumber, totalRevenueField.Type)

		avgQuantityField := resultSchema.Fields["avg_quantity"]
		assert.Equal(t, definition.FieldTypeNumber, avgQuantityField.Type)
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("aggregation_results"))
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

		expectedSchema := &definition.Schema{
			Version: common.MustNewVersion("1.0.0"),
			BaseSchema: definition.BaseSchema{
				Name:        "users_projected_result",
				Description: "Generated schema for query result",
				Fields: map[definition.FieldId]definition.Field{
					"id":   {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
			},
		}

		assert.Equal(t, expectedSchema.Name, resultSchema.Name)
		assert.Equal(t, expectedSchema.Version.String(), resultSchema.Version.String())
		assert.Equal(t, expectedSchema.Description, resultSchema.Description)
		assert.Equal(t, len(expectedSchema.Fields), len(resultSchema.Fields))
		for id, field := range expectedSchema.Fields {
			resultField, ok := resultSchema.Fields[id]
			require.True(t, ok)
			assert.Equal(t, field.Name, resultField.Name)
			assert.Equal(t, field.Type, resultField.Type)
		}
	})

	t.Run("SelectWithNestedJoins", func(t *testing.T) {
		contactSchema := &definition.Schema{
			Version: common.MustNewVersion("1.0.0"),
			BaseSchema: definition.BaseSchema{
				Name: "contacts",
				Fields: map[definition.FieldId]definition.Field{
					"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"phone":   {Name: "phone", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"address": {Name: "address", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
			},
		}

		profileSchema := &definition.Schema{
			Version: common.MustNewVersion("1.0.0"),
			BaseSchema: definition.BaseSchema{
				Name: "profiles",
				Fields: map[definition.FieldId]definition.Field{
					"id":         {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"userId":     {Name: "userId", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"contactId":  {Name: "contactId", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					"department": {Name: "department", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
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
		assert.Contains(t, resultSchema.Fields, definition.FieldId("profile"))

		profileField := resultSchema.Fields["profile"]
		assert.Equal(t, definition.FieldTypeObject, profileField.Type)

		profileSchemaRef, err := definition.FieldSchemaAs[definition.SchemaReference](profileField.Schema)
		require.NoError(t, err)
		nestedProfileSchema, exists := resultSchema.Schemas[profileSchemaRef.ID]
		require.True(t, exists)

		// 4 from profiles + 1 for contact join
		assert.Len(t, nestedProfileSchema.Fields, 5)
		assert.Contains(t, nestedProfileSchema.Fields, definition.FieldId("contact"))

		contactField := nestedProfileSchema.Fields["contact"]
		assert.Equal(t, definition.FieldTypeObject, contactField.Type)

		contactSchemaRef, err := definition.FieldSchemaAs[definition.SchemaReference](contactField.Schema)
		require.NoError(t, err)
		nestedContactSchema, exists := resultSchema.Schemas[contactSchemaRef.ID]
		require.True(t, exists)

		assert.Len(t, nestedContactSchema.Fields, 3)
		assert.Contains(t, nestedContactSchema.Fields, definition.FieldId("phone"))

		// Validate the schema with a sample document
		doc := map[string]any{
			"id":      "user-123",
			"name":    "John Doe",
			"email":   "john.doe@example.com",
			"age":     30,
			"isAdmin": false,
			"profile": map[string]any{
				"id":         "profile-456",
				"userId":     "user-123",
				"contactId":  "contact-789",
				"department": "Engineering",
				"contact": map[string]any{
					"id":      "contact-789",
					"phone":   "123-456-7890",
					"address": "123 Main St",
				},
			},
		}

		v, err := definition.NewDocumentValidator(resultSchema, nil)
		require.NoError(t, err)
		issues, ok := v.Validate(doc)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})
}

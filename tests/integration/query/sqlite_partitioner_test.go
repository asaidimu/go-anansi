package query_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/query"
	sqlite "github.com/asaidimu/go-anansi/v8/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryPartitioner_Partition_Basic(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	partitioner := query.NewQueryPartitioner(capabilities)

	// A simple query with a supported filter
	dsl := query.NewQueryBuilder().
		From("users").
		Where("age").Gt(30).
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	// The entire query should be in the dbQuery
	assert.NotNil(t, dbQuery)
	assert.NotNil(t, dbQuery.Filters)
	assert.Nil(t, postQuery.Filters)
	assert.Empty(t, postQuery.Joins)
	assert.Empty(t, postQuery.Aggregations)
	assert.Empty(t, postQuery.Sort)
	assert.Nil(t, postQuery.Pagination)
}

func TestQueryPartitioner_Partition_UnsupportedFilter(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	// Let's pretend 'regex' is not supported
	delete(capabilities.SupportedComparisonOperators, query.ComparisonOperatorContains)
	partitioner := query.NewQueryPartitioner(capabilities)

	dsl := query.NewQueryBuilder().
		From("users").
		Where("name").Contains("^A").
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	// The filter should be in the postQuery
	assert.NotNil(t, postQuery)
	assert.NotNil(t, postQuery.Filters)
	assert.Nil(t, dbQuery.Filters)
}

func TestQueryPartitioner_Partition_MixedFilters_AND(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	delete(capabilities.SupportedComparisonOperators, query.ComparisonOperatorContains)
	partitioner := query.NewQueryPartitioner(capabilities)

	dsl := query.NewQueryBuilder().
		From("users").
		WhereGroup(common.LogicalAnd).
		Where("age").Gt(30).
		Where("name").Contains("A").
		End().
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	// The 'age' filter should be in dbQuery, 'name' filter in postQuery
	assert.NotNil(t, dbQuery.Filters)
	assert.NotNil(t, postQuery.Filters)
	assert.Equal(t, common.LogicalAnd, dbQuery.Filters.Group.Operator)
	assert.Len(t, dbQuery.Filters.Group.Conditions, 1)
	assert.Equal(t, "age", dbQuery.Filters.Group.Conditions[0].Condition.Field)
	assert.Len(t, postQuery.Filters.Group.Conditions, 1)
	assert.Equal(t, "name", postQuery.Filters.Group.Conditions[0].Condition.Field)
}

func TestQueryPartitioner_Partition_MixedFilters_OR(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	delete(capabilities.SupportedComparisonOperators, query.ComparisonOperatorContains)
	partitioner := query.NewQueryPartitioner(capabilities)

	dsl := query.NewQueryBuilder().
		From("users").
		WhereGroup(common.LogicalOr).
		Where("age").Gt(30).
		Where("name").Contains("A").
		End().
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	// The entire filter group should be in postQuery
	assert.Nil(t, dbQuery.Filters)
	assert.NotNil(t, postQuery.Filters)
	assert.Equal(t, common.LogicalOr, postQuery.Filters.Group.Operator)
	assert.Len(t, postQuery.Filters.Group.Conditions, 2)
}

func TestQueryPartitioner_Partition_UnsupportedJoin(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	// SQLite doesn't support RIGHT JOIN
	delete(capabilities.SupportedJoinTypes, query.JoinTypeRight)
	partitioner := query.NewQueryPartitioner(capabilities)

	dsl := query.NewQueryBuilder().
		From("users").
		RightJoin("profiles").On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "users.id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "profiles.user_id"}},
		},
	}).
		End().
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	assert.Empty(t, dbQuery.Joins)
	assert.Len(t, postQuery.Joins, 1)
	assert.Equal(t, query.JoinTypeRight, postQuery.Joins[0].Type)
}

func TestQueryPartitioner_Partition_ProjectionAugmentation(t *testing.T) {
	factory := sqlite.NewSQLiteFactory(nil)
	capabilities := factory.Capabilities()
	delete(capabilities.SupportedComparisonOperators, query.ComparisonOperatorContains)
	partitioner := query.NewQueryPartitioner(capabilities)

	dsl := query.NewQueryBuilder().
		From("users").
		Select().Include("id").End().
		Where("name").Contains("A").
		Build()

	dbQuery, postQuery, err := partitioner.Partition(&dsl)
	require.NoError(t, err)

	// postQuery needs 'name', so dbQuery's projection should be augmented
	assert.NotNil(t, dbQuery.Projection)
	assert.Len(t, dbQuery.Projection.Include, 2) // id and name

	foundId := false
	foundName := false
	for _, f := range dbQuery.Projection.Include {
		if f.Name == "id" {
			foundId = true
		}
		if f.Name == "name" {
			foundName = true
		}
	}
	assert.True(t, foundId)
	assert.True(t, foundName)

	assert.NotNil(t, postQuery.Filters)
}

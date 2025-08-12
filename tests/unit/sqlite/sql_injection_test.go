package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqlite_query "github.com/asaidimu/go-anansi/v6/sqlite/query"
)

func TestSQLInjection_FieldName(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory()

	// Malicious field name
	maliciousFieldName := "id; DROP TABLE users; --" // Added space after ;
	q := &query.Query{
		Target: &query.QueryTarget{Name: "users"},
		Projection: &query.ProjectionConfiguration{
			Include: []query.ProjectionField{
				{Name: maliciousFieldName},
			},
		},
	}

	_, err := factory.Build(q, native.StmtSelect, nil)
	assert.Error(t, err, "Expected an error for malicious field name")
	assert.Contains(t, err.Error(), "unsupported field reference", "Expected error to indicate unsupported field reference")
}

func TestSQLInjection_JSONPath(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory()

	// Malicious JSON path
	maliciousJSONPath := "profile.name'); DROP TABLE users;--"
	q := &query.Query{
		Target: &query.QueryTarget{Name: "users"},
		Projection: &query.ProjectionConfiguration{
			Include: []query.ProjectionField{
				{Name: "data." + maliciousJSONPath},
			},
		},
	}

	// Add a schema that defines 'data' as an object type
	userSchema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"data": {Name: "data", Type: schema.FieldTypeObject},
		},
	}
	q.Target.Schema = userSchema

	_, err := factory.Build(q, native.StmtSelect, nil)
	assert.Error(t, err, "Expected an error for malicious JSON path")
	assert.Contains(t, err.Error(), "unsupported field reference", "Expected error to indicate unsupported field reference")
}

func TestSQLInjection_FilterValue(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory()

	// Malicious filter value
	maliciousValue := "1 OR 1=1; DROP TABLE users;--"
	q := &query.Query{
		Target: &query.QueryTarget{Name: "users"},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: &maliciousValue},
			},
		},
	}

	_, err := factory.Build(q, native.StmtSelect, nil)
	assert.Error(t, err, "Expected an error for malicious filter value")
	assert.Contains(t, err.Error(), "unsupported filter value", "Expected error to indicate unsupported filter value")
}

func TestSQLInjection_TextSearch(t *testing.T) {
	factory := sqlite_query.NewSQLiteFactory()

	// Malicious text search query
	maliciousSearchQuery := "search_term' OR 1=1; DROP TABLE users;--"
	q := &query.Query{
		Target: &query.QueryTarget{Name: "users"},
		Filters: &query.QueryFilter{
			TextSearchQuery: &query.TextSearchQuery{
				Query:  maliciousSearchQuery,
				Fields: []string{"name"},
			},
		},
	}

	_, err := factory.Build(q, native.StmtSelect, nil)
	assert.Error(t, err, "Expected an error for malicious text search query")
	assert.Contains(t, err.Error(), "unsupported text search type", "Expected error to indicate unsupported text search type")
}
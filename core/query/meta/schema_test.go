package meta_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/query/meta"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	metaschema "github.com/asaidimu/go-anansi/v8/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestQuerySchema_Loads(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err, "QuerySchema should load without error")
	require.NotNil(t, s, "QuerySchema should not be nil")
	require.Equal(t, "QueryDefinition", s.Name, "Schema name should be QueryDefinition")
}

func TestQuerySchema_JSONValid(t *testing.T) {
	raw := meta.QuerySchemaJSON()
	require.True(t, len(raw) > 0, "QuerySchema JSON should not be empty")

	var parsed any
	err := json.Unmarshal(raw, &parsed)
	require.NoError(t, err, "QuerySchema JSON should be valid JSON")
}

func TestQuerySchema_AsMapMatchesJSON(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)

	asMap := s.AsMap()
	require.NotNil(t, asMap, "AsMap should not be nil")

	raw := meta.QuerySchemaJSON()
	var rawMap map[string]any
	err = json.Unmarshal(raw, &rawMap)
	require.NoError(t, err)

	require.Equal(t, rawMap, asMap, "AsMap should match the unmarshalled JSON")
}

func TestQuerySchema_ToJSONValid(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)

	jsonBytes := s.ToJSON()
	require.True(t, len(jsonBytes) > 0, "ToJSON output should not be empty")

	var parsed any
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err, "ToJSON should produce valid JSON")
}

func TestQuerySchema_ToJSONMatchesOriginal(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)

	toJSON := s.ToJSON()

	var expected, actual any
	err = json.Unmarshal(meta.QuerySchemaJSON(), &expected)
	require.NoError(t, err)

	err = json.Unmarshal(toJSON, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual, "Re-serialized schema should match the original")
}

func TestQuerySchema_ValidAgainstMetaSchema(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)
	require.NotNil(t, s)

	vd := metaschema.DevelopmentSchemaValidator()

	asMap := s.AsMap()
	issues, ok := vd.Validate(asMap)

	if !ok {
		t.Logf("QuerySchema validation failed with %d issues:", len(issues))
		for _, issue := range issues {
			t.Logf("  [%s] %s (path: %s)", issue.Code, issue.Message, issue.Path)
		}
	}

	require.True(t, ok, "QuerySchema should be valid against MetaSchema")
	require.Empty(t, issues, "QuerySchema should have no validation issues against MetaSchema")
}

func TestQuerySchema_ValidateSampleQueries(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)
	require.NotNil(t, s)

	vd, err := definition.NewDocumentValidator(s, nil)
	require.NoError(t, err, "Should be able to create a validator from QuerySchema")

	tests := []struct {
		name    string
		query   string
		wantOK  bool
	}{
		{
			name:   "empty query",
			query:  `{}`,
			wantOK: true,
		},
		{
			name:   "simple filter condition",
			query:  `{"filters": {"field": "name", "operator": "eq", "value": "Alice"}}`,
			wantOK: true,
		},
		{
			name:   "filter with number value",
			query:  `{"filters": {"field": "price", "operator": "gt", "value": 100}}`,
			wantOK: true,
		},
		{
			name:   "filter group with AND",
			query:  `{"filters": {"operator": "and", "conditions": [{"field": "status", "operator": "eq", "value": "active"}, {"field": "age", "operator": "gt", "value": 21}]}}`,
			wantOK: true,
		},
		{
			name:   "text search",
			query:  `{"filters": {"query": "hello world", "fields": ["title"], "type": "contains"}}`,
			wantOK: true,
		},
		{
			name:   "sort and pagination",
			query:  `{"sort": [{"field": "createdAt", "direction": "desc"}], "pagination": {"type": "offset", "limit": 10, "offset": 0}}`,
			wantOK: true,
		},
		{
			name:   "projection with include",
			query:  `{"projection": {"include": [{"name": "id"}, {"name": "email"}]}}`,
			wantOK: true,
		},
		{
			name:   "distinct boolean",
			query:  `{"distinct": true}`,
			wantOK: true,
		},
		{
			name:   "distinct fields",
			query:  `{"distinct": {"fields": ["category"]}}`,
			wantOK: true,
		},
		{
			name:   "aggregation",
			query:  `{"aggregations": [{"type": "count", "field": "id", "alias": "total"}]}`,
			wantOK: true,
		},
		{
			name:   "join",
			query:  `{"joins": [{"type": "inner", "target": {"name": "orders"}, "on": {"field": "id", "operator": "eq", "value": {"type": "field", "field": "order_id"}}}]}`,
			wantOK: true,
		},
		{
			name:   "invalid operator",
			query:  `{"filters": {"field": "name", "operator": "invalid_op", "value": "test"}}`,
			wantOK: false,
		},
		{
			name:   "invalid sort direction",
			query:  `{"sort": [{"field": "name", "direction": "invalid"}]}`,
			wantOK: false,
		},
		{
			name:   "invalid pagination type",
			query:  `{"pagination": {"type": "unknown", "limit": 10}}`,
			wantOK: false,
		},
		{
			name:   "missing required field in condition",
			query:  `{"filters": {"operator": "eq", "value": "test"}}`,
			wantOK: false,
		},
		{
			name:   "nested projection",
			query:  `{"projection": {"include": [{"name": "address", "nested": {"include": [{"name": "city"}]}}]}}`,
			wantOK: true,
		},
		{
			name:   "computed field",
			query:  `{"projection": {"computed": [{"type": "computed", "expression": {"function": "CONCAT", "arguments": ["a", "b"]}, "alias": "combined"}]}}`,
			wantOK: true,
		},
		{
			name:   "case expression",
			query:  `{"projection": {"computed": [{"type": "case", "conditions": [{"when": {"field": "status", "operator": "eq", "value": "active"}, "then": "yes"}], "else": "no", "alias": "is_active"}]}}`,
			wantOK: true,
		},
		{
			name:   "subquery",
			query:  `{"filters": {"field": "id", "operator": "in", "value": {"type": "subquery", "query": {"filters": {"field": "status", "operator": "eq", "value": "active"}}}}}`,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]any
			data, err := decodeJSONNumber([]byte(tt.query))
			require.NoError(t, err, "test query should be valid JSON")

			issues, ok := vd.Validate(data)
			if !ok {
				t.Logf("Validation issues for %q:", tt.name)
				for _, issue := range issues {
					t.Logf("  [%s] %s (path: %s)", issue.Code, issue.Message, issue.Path)
				}
			}
			require.Equal(t, tt.wantOK, ok, "Validation result for %q", tt.name)
		})
	}
}

func TestQuerySchema_ContainsExpectedSchemas(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)

	expectedSchemas := []string{
		"schema_query_target", "schema_query_filter", "schema_filter_condition", "schema_filter_group",
		"schema_text_search_query", "schema_pagination_offset", "schema_pagination_cursor",
		"schema_sort_configuration", "schema_projection_configuration", "schema_projection_field",
		"schema_projection_computed_item", "schema_computed_field_expression", "schema_case_expression",
		"schema_join_configuration", "schema_aggregation_configuration",
		"schema_query_distinct_config_bool", "schema_query_distinct_config_fields",
		"schema_query_union", "schema_raw_query", "schema_field_reference", "schema_function_call",
		"schema_subquery_value", "schema_subquery_inner_query",

		"enum_logical_operator", "enum_comparison_operator",
		"enum_sort_direction", "enum_join_type", "enum_aggregation_type",
		"enum_union_type", "enum_text_search_type", "enum_text_operator",
		"enum_pagination_type", "enum_field_ref_type",
		"enum_subquery_type", "enum_computed_type", "enum_case_type",
	}

	for _, name := range expectedSchemas {
		_, ok := s.Schemas[definition.SchemaId(name)]
		require.True(t, ok, "Schema should contain %q", name)
	}
}

func TestQuerySchema_ContainsExpectedFields(t *testing.T) {
	s, err := meta.QuerySchema()
	require.NoError(t, err)

	expectedFields := []string{
		"target", "filters", "projection", "sort", "limit",
		"pagination", "joins", "distinct", "aggregations",
		"union", "hints", "raw",
	}

	for _, name := range expectedFields {
		_, found := s.FindField(name)
		require.NotNil(t, found, "Root schema should contain field %q", name)
	}
}

// decodeJSONNumber decodes raw JSON into a map, converting json.Number
// values to int64 or float64 so that integer fields validate correctly.
// Go's json.Unmarshal into map[string]any produces float64 for all
// JSON numbers, which fails integer type checks.
func decodeJSONNumber(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return nil, err
	}
	return convertNumbers(v).(map[string]any), nil
}

func convertNumbers(v any) any {
	switch vv := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(vv.String(), 10, 64); err == nil {
			return i
		}
		f, _ := strconv.ParseFloat(vv.String(), 64)
		return f
	case map[string]any:
		for k, v := range vv {
			vv[k] = convertNumbers(v)
		}
	case []any:
		for i, v := range vv {
			vv[i] = convertNumbers(v)
		}
	}
	return v
}

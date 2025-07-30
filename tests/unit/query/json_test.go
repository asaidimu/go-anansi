package query_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
)

// Helper functions to get pointers to primitive types
func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

// TestProjectionComputedItem_UnmarshalJSON tests unmarshalling of ProjectionComputedItem
func TestProjectionComputedItem_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    query.ProjectionComputedItem
		wantErr bool
	}{
		{
			name:    "Unmarshal ComputedFieldExpression",
			jsonStr: ` { "type": "computed", "expression": { "function": "CONCAT", "arguments": [ { "type": "field", "field": "firstName" }, { "type": "field", "field": "lastName" } ] }, "alias": "fullName" }`,
			want: query.ProjectionComputedItem{
				ComputedFieldExpression: &query.ComputedFieldExpression{
					Type: "computed",
					Expression: &query.FunctionCall{
						Function: "CONCAT",
						Arguments: []query.FilterValue{
							{FieldRefVal: &query.FieldReference{
								Type:  "field",
								Field: "firstName",
							}},
							{FieldRefVal: &query.FieldReference{
								Type:  "field",
								Field: "lastName",
							}},
						},
					},
					Alias: "fullName",
				},
			},
			wantErr: false,
		},
		{
			name:    "Unmarshal ComputedFieldExpression With Literal",
			jsonStr: ` { "type": "computed", "expression": { "function": "PREFIX_STRING", "arguments": [ "User-", { "type": "field", "field": "userId" } ] }, "alias": "prefixedUserId" } `,
			want: query.ProjectionComputedItem{
				ComputedFieldExpression: &query.ComputedFieldExpression{
					Type: "computed",
					Expression: &query.FunctionCall{
						Function: "PREFIX_STRING",
						Arguments: []query.FilterValue{
							{
								StringVal: stringPtr("User-"),
							},
							{FieldRefVal: &query.FieldReference{
								Type:  "field",
								Field: "userId",
							}},
						},
					},
					Alias: "prefixedUserId",
				},
			},
			wantErr: false,
		},

		{
			name:    "Unmarshal CaseExpression",
			jsonStr: `{"type": "case", "conditions": [{"when": {"field": "status", "operator": "eq", "value": "active"}, "then": "Active User"}], "else": "Inactive User", "alias": "user_status"}`,
			want: query.ProjectionComputedItem{
				CaseExpression: &query.CaseExpression{
					Type: "case",
					Conditions: []query.CaseCondition{
						{
							When: query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "status",
									Operator: "eq",
									Value:    query.FilterValue{StringVal: stringPtr("active")},
								},
							},
							Then: query.FilterValue{StringVal: stringPtr("Active User")},
						},
					},
					Else:  query.FilterValue{StringVal: stringPtr("Inactive User")},
					Alias: "user_status",
				},
			},
			wantErr: false,
		},
		{
			name:    "Unmarshal Invalid Type",
			jsonStr: `{"type": "unknown", "field": "value"}`,
			want:    query.ProjectionComputedItem{},
			wantErr: true,
		},
		{
			name:    "Unmarshal Missing Type",
			jsonStr: `{"field": "value"}`,
			want:    query.ProjectionComputedItem{},
			wantErr: true,
		},
		{
			name:    "Unmarshal Malformed JSON",
			jsonStr: `{"type": "computed", "expression": "invalid"}`, // Expression should be an object
			want:    query.ProjectionComputedItem{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got query.ProjectionComputedItem
			err := json.Unmarshal([]byte(tt.jsonStr), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnmarshalJSON() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestProjectionComputedItem_MarshalJSON tests marshalling of ProjectionComputedItem
func TestProjectionComputedItem_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    query.ProjectionComputedItem
		wantJson string
		wantErr  bool
	}{
		{
			name: "Marshal ComputedFieldExpression",
			input: query.ProjectionComputedItem{
				ComputedFieldExpression: &query.ComputedFieldExpression{
					Type: "computed",
					Expression: &query.FunctionCall{
						Function: "SUM",
						Arguments: []query.FilterValue{
							{StringVal: stringPtr("price")},
						},
					},
					Alias: "total_price",
				},
			},
			wantJson: `{"type":"computed","expression":{"function":"SUM","arguments":["price"]},"alias":"total_price"}`,
			wantErr:  false,
		},
		{
			name: "Marshal CaseExpression",
			input: query.ProjectionComputedItem{
				CaseExpression: &query.CaseExpression{
					Type: "case",
					Conditions: []query.CaseCondition{
						{
							When: query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "status",
									Operator: "eq",
									Value:    query.FilterValue{StringVal: stringPtr("active")},
								},
							},
							Then: query.FilterValue{StringVal: stringPtr("Active User")},
						},
					},
					Else:  query.FilterValue{StringVal: stringPtr("Inactive User")},
					Alias: "user_status",
				},
			},
			wantJson: `{"type":"case","conditions":[{"when":{"field":"status","operator":"eq","value":"active"},"then":"Active User"}],"else":"Inactive User","alias":"user_status"}`,
			wantErr:  false,
		},
		{
			name:     "Marshal Empty ProjectionComputedItem",
			input:    query.ProjectionComputedItem{},
			wantJson: `{}`, // Our MarshalJSON outputs an empty object for an empty struct
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(gotBytes) != tt.wantJson {
				t.Errorf("MarshalJSON() got = %s, want %s", string(gotBytes), tt.wantJson)
			}
		})
	}
}

// TestPaginationOptions_UnmarshalJSON tests unmarshalling of PaginationOptions
func TestPaginationOptions_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    query.PaginationOptions
		wantErr bool
	}{
		{
			name:    "Unmarshal Offset Pagination",
			jsonStr: `{"type": "offset", "limit": 10, "offset": 20}`,
			want: query.PaginationOptions{
				Type:   "offset",
				Limit:  10,
				Offset: intPtr(20),
			},
			wantErr: false,
		},
		{
			name:    "Unmarshal Offset Pagination without offset",
			jsonStr: `{"type": "offset", "limit": 10}`,
			want: query.PaginationOptions{
				Type:   "offset",
				Limit:  10,
				Offset: nil, // Should be nil if not present
			},
			wantErr: false,
		},
		
		{
			name:    "Unmarshal Unknown Type",
			jsonStr: `{"type": "unknown", "limit": 10}`,
			want:    query.PaginationOptions{},
			wantErr: true,
		},
		{
			name:    "Unmarshal Missing Type",
			jsonStr: `{"limit": 10}`,
			want:    query.PaginationOptions{},
			wantErr: true, // Our UnmarshalJSON expects a 'type' field to be present
		},
		{
			name:    "Unmarshal Malformed JSON",
			jsonStr: `{"type": "offset", "limit": "invalid"}`, // Limit should be int
			want:    query.PaginationOptions{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got query.PaginationOptions
			err := json.Unmarshal([]byte(tt.jsonStr), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnmarshalJSON() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestPaginationOptions_MarshalJSON tests marshalling of PaginationOptions
func TestPaginationOptions_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    query.PaginationOptions
		wantJson string
		wantErr  bool
	}{
		{
			name: "Marshal Offset Pagination",
			input: query.PaginationOptions{
				Type:   "offset",
				Limit:  10,
				Offset: intPtr(20),
			},
			wantJson: `{"type":"offset","limit":10,"offset":20}`,
			wantErr:  false,
		},
		{
			name: "Marshal Offset Pagination without offset",
			input: query.PaginationOptions{
				Type:  "offset",
				Limit: 10,
			},
			wantJson: `{"type":"offset","limit":10}`,
			wantErr:  false,
		},
		
		{
			name: "Marshal Unknown Type",
			input: query.PaginationOptions{
				Type:  "unknown",
				Limit: 10,
			},
			wantJson: "", // Error case, so no specific JSON expected
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(gotBytes) != tt.wantJson {
				t.Errorf("MarshalJSON() got = %s, want %s", string(gotBytes), tt.wantJson)
			}
		})
	}
}

// TestQueryDistinctConfig_UnmarshalJSON tests unmarshalling of QueryDistinctConfig
func TestQueryDistinctConfig_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    query.QueryDistinctConfig
		wantErr bool
	}{
		{
			name:    "Unmarshal Boolean True",
			jsonStr: `true`,
			want:    query.QueryDistinctConfig{IsDistinct: boolPtr(true)},
			wantErr: false,
		},
		{
			name:    "Unmarshal Boolean False", // Although our MarshalJSON outputs null for false, Unmarshal should handle it
			jsonStr: `false`,
			want:    query.QueryDistinctConfig{IsDistinct: boolPtr(false)},
			wantErr: false,
		},
		{
			name:    "Unmarshal Fields Array",
			jsonStr: `{"fields": ["id", "name"]}`,
			want:    query.QueryDistinctConfig{Fields: []string{"id", "name"}},
			wantErr: false,
		},
		{
			name:    "Unmarshal Empty Fields Array",
			jsonStr: `{"fields": []}`,
			want:    query.QueryDistinctConfig{}, // Should result in an empty config
			wantErr: false,
		},
		{
			name:    "Unmarshal Null",
			jsonStr: `null`,
			want:    query.QueryDistinctConfig{}, // Should result in an empty config
			wantErr: false,
		},
		{
			name:    "Unmarshal Empty Object",
			jsonStr: `{}`,
			want:    query.QueryDistinctConfig{}, // Should result in an empty config
			wantErr: false,
		},
		{
			name:    "Unmarshal Invalid Type",
			jsonStr: `123`, // Neither boolean nor object
			want:    query.QueryDistinctConfig{},
			wantErr: true,
		},
		{
			name:    "Unmarshal Malformed Fields Object",
			jsonStr: `{"fields": "invalid"}`, // Fields should be an array
			want:    query.QueryDistinctConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got query.QueryDistinctConfig
			err := json.Unmarshal([]byte(tt.jsonStr), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnmarshalJSON() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestQueryDistinctConfig_MarshalJSON tests marshalling of QueryDistinctConfig
func TestQueryDistinctConfig_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    query.QueryDistinctConfig
		wantJson string
		wantErr  bool
	}{
		{
			name:     "Marshal Boolean True",
			input:    query.QueryDistinctConfig{IsDistinct: boolPtr(true)},
			wantJson: `true`,
			wantErr:  false,
		},
		{
			name:     "Marshal Fields Array",
			input:    query.QueryDistinctConfig{Fields: []string{"id", "name"}},
			wantJson: `{"fields":["id","name"]}`,
			wantErr:  false,
		},
		{
			name:     "Marshal Empty Config (should be null)",
			input:    query.QueryDistinctConfig{},
			wantJson: `null`,
			wantErr:  false,
		},
		{
			name:     "Marshal Boolean False (should be null based on current MarshalJSON logic)",
			input:    query.QueryDistinctConfig{IsDistinct: boolPtr(false)},
			wantJson: `null`,
			wantErr:  false,
		},
		{
			name:     "Marshal Empty Fields Array (should be null)",
			input:    query.QueryDistinctConfig{Fields: []string{}},
			wantJson: `{"fields":[]}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := json.Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(gotBytes) != tt.wantJson {
				t.Errorf("MarshalJSON() got = %s, want %s", string(gotBytes), tt.wantJson)
			}
		})
	}
}
func TestQuery_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    query.Query
		wantJson string
		wantErr  bool
	}{
		{
			name:     "Empty Query",
			input:    query.Query{},
			wantJson: `{}`, // An empty struct should marshal to an empty JSON object
			wantErr:  false,
		},
		{
			name: "Simple Filter Query",
			input: query.Query{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "name",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{StringVal: stringPtr("Alice")},
					},
				},
			},
			wantJson: `{"filters":{"field":"name","operator":"eq","value":"Alice"}}`,
			wantErr:  false,
		},
		{
			name: "Query with Sort and Offset Pagination",
			input: query.Query{
				Sort: []query.SortConfiguration{
					{Field: "createdAt", Direction: query.SortDirectionDesc},
				},
				Pagination: &query.PaginationOptions{
					Type:   "offset",
					Limit:  10,
					Offset: intPtr(5),
				},
			},
			wantJson: `{"sort":[{"field":"createdAt","direction":"desc"}],"pagination":{"type":"offset","limit":10,"offset":5}}`,
			wantErr:  false,
		},
		
		{
			name: "Query with Basic Projection (Include)",
			input: query.Query{
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{
						{Name: "id"},
						{Name: "email"},
					},
				},
			},
			wantJson: `{"projection":{"include":[{"name":"id"},{"name":"email"}]}}`,
			wantErr:  false,
		},
		{
			name: "Query with Nested Projection",
			input: query.Query{
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{
						{Name: "user"},
						{
							Name: "address",
							Nested: &query.ProjectionConfiguration{
								Include: []query.ProjectionField{{Name: "street"}, {Name: "city"}},
							},
						},
					},
				},
			},
			wantJson: `{"projection":{"include":[{"name":"user"},{"name":"address","nested":{"include":[{"name":"street"},{"name":"city"}]}}]}}`,
			wantErr:  false,
		},
		{
			name: "Query with Computed Field Projection",
			input: query.Query{
				Projection: &query.ProjectionConfiguration{
					Computed: []query.ProjectionComputedItem{
						{
							ComputedFieldExpression: &query.ComputedFieldExpression{
								Type: "computed",
								Expression: &query.FunctionCall{
									Function: "CONCAT",
									Arguments: []query.FilterValue{
										{FieldRefVal: &query.FieldReference{Type: "field", Field: "firstName"}},
										{StringVal: stringPtr(" ")},
										{FieldRefVal: &query.FieldReference{Type: "field", Field: "lastName"}},
									},
								},
								Alias: "fullName",
							},
						},
					},
				},
			},
			wantJson: `{"projection":{"computed":[{"type":"computed","expression":{"function":"CONCAT","arguments":[{"type":"field","field":"firstName"}," ",{"type":"field","field":"lastName"}]},"alias":"fullName"}]}}`,
			wantErr:  false,
		},
		{
			name: "Query with Case Expression Projection",
			input: query.Query{
				Projection: &query.ProjectionConfiguration{
					Computed: []query.ProjectionComputedItem{
						{
							CaseExpression: &query.CaseExpression{
								Type: "case",
								Conditions: []query.CaseCondition{
									{
										When: query.QueryFilter{
											Condition: &query.FilterCondition{
												Field:    "status",
												Operator: query.ComparisonOperatorEq,
												Value:    query.FilterValue{StringVal: stringPtr("active")},
											},
										},
										Then: query.FilterValue{StringVal: stringPtr("Active User")},
									},
								},
								Else:  query.FilterValue{StringVal: stringPtr("Inactive User")},
								Alias: "user_status",
							},
						},
					},
				},
			},
			wantJson: `{"projection":{"computed":[{"type":"case","conditions":[{"when":{"field":"status","operator":"eq","value":"active"},"then":"Active User"}],"else":"Inactive User","alias":"user_status"}]}}`,
			wantErr:  false,
		},
		{
			name: "Query with Distinct (boolean)",
			input: query.Query{
				Distinct: &query.QueryDistinctConfig{
					IsDistinct: boolPtr(true),
				},
			},
			wantJson: `{"distinct":true}`,
			wantErr:  false,
		},
		{
			name: "Query with Distinct (fields)",
			input: query.Query{
				Distinct: &query.QueryDistinctConfig{
					Fields: []string{"category", "product"},
				},
			},
			wantJson: `{"distinct":{"fields":["category","product"]}}`,
			wantErr:  false,
		},
		{
			name: "Query with Aggregation (Count)",
			input: query.Query{
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeCount,
						Field: "id",
						Alias: stringPtr("totalRecords"),
					},
				},
			},
			wantJson: `{"aggregations":[{"type":"count","field":"id","alias":"totalRecords"}]}`,
			wantErr:  false,
		},
		{
			name: "Query with Aggregation (Sum with Group and Filter)",
			input: query.Query{
				Aggregations: []query.AggregationConfiguration{
					{
						Type:   query.AggregationTypeSum,
						Field:  "amount",
						Alias:  stringPtr("totalAmount"),
						Groups: []string{"product_id", "region"},
						Filter: &query.QueryFilter{
							Condition: &query.FilterCondition{
								Field:    "status",
								Operator: query.ComparisonOperatorEq,
								Value:    query.FilterValue{StringVal: stringPtr("completed")},
							},
						},
					},
				},
			},
			wantJson: `{"aggregations":[{"type":"sum","field":"amount","alias":"totalAmount","groups":["product_id","region"],"filter":{"field":"status","operator":"eq","value":"completed"}}]}`,
			wantErr:  false,
		},
		{
			name: "Full Query Marshal and Unmarshal",
			input: query.Query{
				Filters: &query.QueryFilter{
					Group: &query.FilterGroup{
						Operator: query.LogicalOperatorAnd,
						Conditions: []query.QueryFilter{
							{
								Condition: &query.FilterCondition{
									Field:    "status",
									Operator: query.ComparisonOperatorEq,
									Value:    query.FilterValue{StringVal: stringPtr("active")},
								},
							},
							{
								Condition: &query.FilterCondition{
									Field:    "age",
									Operator: query.ComparisonOperatorGt,
									Value:    query.FilterValue{NumberVal: floatPtr(30.0)},
								},
							},
							{
								TextSearchQuery: &query.TextSearchQuery{
									Query:  "golang programming",
									Fields: []string{"title", "description"},
									Type:   query.TextSearchTypeContains,
								},
							},
						},
					},
				},
				Sort: []query.SortConfiguration{
					{Field: "createdAt", Direction: query.SortDirectionDesc},
					{Field: "name", Direction: query.SortDirectionAsc},
				},
				Pagination: &query.PaginationOptions{
					Type:   "offset",
					Limit:  10,
					Offset: intPtr(0),
				},
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{
						{Name: "id"},
						{Name: "name"},
						{
							Name: "address",
							Nested: &query.ProjectionConfiguration{
								Include: []query.ProjectionField{{Name: "city"}},
							},
						},
					},
					Computed: []query.ProjectionComputedItem{
						{
							ComputedFieldExpression: &query.ComputedFieldExpression{
								Type: "computed",
								Expression: &query.FunctionCall{
									Function: "CONCAT",
									Arguments: []query.FilterValue{
										{StringVal: stringPtr("Mr./Ms. ")},
										{FieldRefVal: &query.FieldReference{Type: "field", Field: "lastName"}},
									},
								},
								Alias: "salutationName",
							},
						},
					},
				},
				Joins: []query.JoinConfiguration{
					{
						Type:   query.JoinTypeInner,
						Target: "orders",
						On: &query.QueryFilter{
							Condition: &query.FilterCondition{
								Field:    "users.id",
								Operator: query.ComparisonOperatorEq,
								Value: query.FilterValue{
									FieldRefVal: &query.FieldReference{Type: "field", Field: "orders.user_id"},
								},
							},
						},
						Alias: stringPtr("userOrders"),
						Projection: &query.ProjectionConfiguration{
							Include: []query.ProjectionField{
								{Name: "orderId"},
								{Name: "amount"},
							},
						},
					},
				},
				Distinct: &query.QueryDistinctConfig{
					Fields: []string{"category", "item"},
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeCount,
						Field: "id",
						Alias: stringPtr("totalUsers"),
					},
					{
						Type:   query.AggregationTypeSum,
						Field:  "price",
						Alias:  stringPtr("totalSales"),
						Groups: []string{"product"},
						Filter: &query.QueryFilter{
							Condition: &query.FilterCondition{
								Field:    "region",
								Operator: query.ComparisonOperatorEq,
								Value:    query.FilterValue{StringVal: stringPtr("north")},
							},
						},
					},
				},
			},
			wantJson: `{
				"filters": {
						"operator": "and",
						"conditions": [
							{
									"field": "status",
									"operator": "eq",
									"value": "active"
							},
							{
									"field": "age",
									"operator": "gt",
									"value": 30
							},
							{
									"query": "golang programming",
									"fields": ["title", "description"],
									"type": "contains"
							}
						]
				},
				"sort": [
					{"field": "createdAt", "direction": "desc"},
					{"field": "name", "direction": "asc"}
				],
				"pagination": {
					"type": "offset",
					"limit": 10,
					"offset": 0
				},
				"projection": {
					"include": [
						{"name": "id"},
						{"name": "name"},
						{"name": "address", "nested": {"include": [{"name": "city"}]}}
					],
					"computed": [
						{
							"type": "computed",
							"expression": {
								"function": "CONCAT",
								"arguments": [
									"Mr./Ms. ",
									{"type": "field", "field": "lastName"}
								]
							},
							"alias": "salutationName"
						}
					]
				},
				"joins": [
					{
						"type": "inner",
						"target": "orders",
						"on": {
								"field": "users.id",
								"operator": "eq",
								"value": {"type": "field", "field": "orders.user_id"}
						}
						,"alias": "userOrders",
						"projection": {
							"include": [
								{"name": "orderId"},
								{"name": "amount"}
							]
						}
					}
				],
				"distinct": {
					"fields": ["category", "item"]
				},
				"aggregations": [
					{
						"type": "count",
						"field": "id",
						"alias": "totalUsers"
					},
					{
						"type": "sum",
						"field": "price",
						"alias": "totalSales",
						"groups": ["product"],
						"filter": {
								"field": "region",
								"operator": "eq",
								"value": "north"
						}
					}
				]
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// --- Test Marshal ---
			gotBytes, err := json.MarshalIndent(tt.input, "", "  ") // Use MarshalIndent for pretty output for comparison
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Normalize JSON strings for comparison
			var gotMap, wantMap map[string]any
			err = json.Unmarshal(gotBytes, &gotMap)
			if err != nil {
				t.Fatalf("Failed to unmarshal marshaled JSON: %v", err)
			}
			err = json.Unmarshal([]byte(tt.wantJson), &wantMap)
			if err != nil {
				t.Fatalf("Failed to unmarshal wanted JSON: %v", err)
			}

			if !reflect.DeepEqual(gotMap, wantMap) {
				t.Errorf("Marshal got JSON = %s, want %s", string(gotBytes), tt.wantJson)
			}

			// --- Test Unmarshal ---
			var gotQuery query.Query
			unmarshalErr := json.Unmarshal([]byte(tt.wantJson), &gotQuery)
			if (unmarshalErr != nil) != tt.wantErr {
				t.Errorf("Unmarshal error = %v, wantErr %v", unmarshalErr, tt.wantErr)
				return
			}

			// For unmarshal, we compare the unmarshaled struct to the original input struct
			// This implicitly tests if the unmarshaled JSON correctly reconstructs the Go struct.
			if !reflect.DeepEqual(gotQuery, tt.input) {
				t.Errorf("Unmarshal got struct = %+v, want %+v", gotQuery, tt.input)
				// To help debug, marshal the unmarshaled struct back to JSON
				reMarshalBytes, _ := json.MarshalIndent(gotQuery, "", "  ")
				t.Errorf("Unmarshal got JSON (re-marshaled) = %s", string(reMarshalBytes))
			}
		})
	}
}

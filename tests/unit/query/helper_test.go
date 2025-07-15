package query_test

import (
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query" // Assuming this is your module path
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

var testRecords = []schema.Document{
	{"id": 1, "name": "Alice", "age": 30, "active": true, "address": map[string]any{"city": "New York"}},
	{"id": 2, "name": "Bob", "age": 25, "active": false, "address": map[string]any{"city": "Los Angeles"}},
	{"id": 3, "name": "Charlie", "age": 35, "active": true, "address": map[string]any{"city": "New York"}},
	{"id": 4, "name": "David", "age": 30, "active": true, "address": map[string]any{"city": "Chicago"}},
	{"id": 5, "name": "Eve", "age": 25, "active": false, "address": nil},
}

func TestNewQueryHelper(t *testing.T) {
	t.Run("valid query", func(t *testing.T) {
		q := &query.QueryDSL{}
		_, err := query.NewQueryHelper(q, nil, nil, nil)
		if err != nil {
			t.Errorf("expected no error, but got %v", err)
		}
	})

	t.Run("nil query", func(t *testing.T) {
		_, err := query.NewQueryHelper(nil, nil, nil, nil)
		if err == nil {
			t.Error("expected an error for nil query, but got none")
		}
	})

	t.Run("invalid query", func(t *testing.T) {
		q := &query.QueryDSL{
			Sort: []query.SortConfiguration{
				{Field: "name", Direction: "invalid"},
			},
		}
		_, err := query.NewQueryHelper(q, nil, nil, nil)
		if err == nil {
			t.Error("expected an error for invalid sort direction, but got none")
		}
	})
}

func TestFilter(t *testing.T) {
	testCases := []struct {
		name        string
		query       *query.QueryDSL
		expectedIDs []int
		expectError bool
	}{
		{
			name: "equal operator",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{Field: "age", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{NumberVal: floatPtr(30)}},
				},
			},
			expectedIDs: []int{1, 4},
		},
		{
			name: "nested field equal",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{Field: "address.city", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{StringVal: stringPtr("New York")}},
				},
			},
			expectedIDs: []int{1, 3},
		},
		{
			name: "AND group",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Group: &query.FilterGroup{
						Operator: "and",
						Conditions: []query.QueryFilter{
							{Condition: &query.FilterCondition{Field: "age", Operator: query.ComparisonOperatorGte, Value: query.FilterValue{NumberVal: floatPtr(30)}}},
							{Condition: &query.FilterCondition{Field: "active", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{BoolVal: boolPtr(true)}}},
						},
					},
				},
			},
			expectedIDs: []int{1, 3, 4},
		},
		{
			name: "OR group",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Group: &query.FilterGroup{
						Operator: "or",
						Conditions: []query.QueryFilter{
							{Condition: &query.FilterCondition{Field: "age", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{NumberVal: floatPtr(25)}}},
							{Condition: &query.FilterCondition{Field: "active", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{BoolVal: boolPtr(true)}}},
						},
					},
				},
			},
			expectedIDs: []int{1, 2, 3, 4, 5},
		},
		{
			name: "IN operator",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{Field: "name", Operator: query.ComparisonOperatorIn, Value: query.FilterValue{
						ArrayVal: []query.FilterValue{
							{StringVal: stringPtr("Alice")},
							{StringVal: stringPtr("Eve")},
						},
					},
					},
				},
			},
			expectedIDs: []int{1, 5},
		},
		{
			name: "NOT operator",
			query: &query.QueryDSL{
				Filters: &query.QueryFilter{
					Group: &query.FilterGroup{
						Operator: "not",
						Conditions: []query.QueryFilter{
							{Condition: &query.FilterCondition{Field: "active", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{BoolVal: boolPtr(true)}}},
						},
					},
				},
			},
			expectedIDs: []int{2, 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := query.NewQueryHelper(tc.query, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create query helper: %v", err)
			}

			filtered, err := helper.Filter(testRecords)
			if (err != nil) != tc.expectError {
				t.Fatalf("expected error: %v, got: %v", tc.expectError, err)
			}

			var resultIDs []int
			for _, r := range filtered {
				resultIDs = append(resultIDs, r["id"].(int))
			}

			if !reflect.DeepEqual(resultIDs, tc.expectedIDs) {
				t.Errorf("expected IDs %v, but got %v", tc.expectedIDs, resultIDs)
			}
		})
	}
}

func TestSort(t *testing.T) {
	testCases := []struct {
		name        string
		query       *query.QueryDSL
		expectedIDs []int
	}{
		{
			name: "sort by age ascending",
			query: &query.QueryDSL{
				Sort: []query.SortConfiguration{
					{Field: "age", Direction: query.SortDirectionAsc},
				},
			},
			expectedIDs: []int{2, 5, 1, 4, 3},
		},
		{
			name: "sort by age descending, then name ascending",
			query: &query.QueryDSL{
				Sort: []query.SortConfiguration{
					{Field: "age", Direction: query.SortDirectionDesc},
					{Field: "name", Direction: query.SortDirectionAsc},
				},
			},
			expectedIDs: []int{3, 1, 4, 2, 5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := query.NewQueryHelper(tc.query, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create query helper: %v", err)
			}

			sorted, err := helper.Sort(testRecords)
			if err != nil {
				t.Fatalf("sort failed: %v", err)
			}

			var resultIDs []int
			for _, r := range sorted {
				resultIDs = append(resultIDs, r["id"].(int))
			}

			if !reflect.DeepEqual(resultIDs, tc.expectedIDs) {
				t.Errorf("expected IDs %v, but got %v", tc.expectedIDs, resultIDs)
			}
		})
	}
}

func TestPaginate(t *testing.T) {
	offset := 2
	cursor := "2"

	testCases := []struct {
		name               string
		query              *query.QueryDSL
		expectedIDs        []int
		expectedTotal      *int
		expectedNextCursor *string
	}{
		{
			name: "offset pagination",
			query: &query.QueryDSL{
				Pagination: &query.PaginationOptions{Type: "offset", Limit: 2, Offset: &offset},
			},
			expectedIDs:   []int{3, 4},
			expectedTotal: func(i int) *int { return &i }(5),
		},
		{
			name: "cursor pagination",
			query: &query.QueryDSL{
				Pagination: &query.PaginationOptions{Type: "cursor", Limit: 2, Cursor: &cursor},
			},
			expectedIDs:        []int{3, 4},
			expectedNextCursor: func(s string) *string { return &s }("4"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := query.NewQueryHelper(tc.query, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create query helper: %v", err)
			}

			paginated, result, err := helper.Paginate(testRecords)
			if err != nil {
				t.Fatalf("pagination failed: %v", err)
			}

			var resultIDs []int
			for _, r := range paginated {
				resultIDs = append(resultIDs, r["id"].(int))
			}

			if !reflect.DeepEqual(resultIDs, tc.expectedIDs) {
				t.Errorf("expected IDs %v, but got %v", tc.expectedIDs, resultIDs)
			}

			if tc.expectedTotal != nil && (result.Total == nil || *result.Total != *tc.expectedTotal) {
				t.Errorf("expected total %v, got %v", *tc.expectedTotal, result.Total)
			}

			if tc.expectedNextCursor != nil && (result.Cursor == nil || *result.Cursor != *tc.expectedNextCursor) {
				t.Errorf("expected next cursor %v, got %v", *tc.expectedNextCursor, result.Cursor)
			}
		})
	}
}

func TestProject(t *testing.T) {
	testCases := []struct {
		name            string
		query           *query.QueryDSL
		expectedRecords []schema.Document
	}{
		{
			name: "include fields",
			query: &query.QueryDSL{
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{{Name: "name"}, {Name: "age"}},
				},
			},
			expectedRecords: []schema.Document{
				{"name": "Alice", "age": 30},
				{"name": "Bob", "age": 25},
				{"name": "Charlie", "age": 35},
				{"name": "David", "age": 30},
				{"name": "Eve", "age": 25},
			},
		},
		{
			name: "exclude fields",
			query: &query.QueryDSL{
				Projection: &query.ProjectionConfiguration{
					Exclude: []query.ProjectionField{{Name: "active"}, {Name: "address"}},
				},
			},
			expectedRecords: []schema.Document{
				{"id": 1, "name": "Alice", "age": 30},
				{"id": 2, "name": "Bob", "age": 25},
				{"id": 3, "name": "Charlie", "age": 35},
				{"id": 4, "name": "David", "age": 30},
				{"id": 5, "name": "Eve", "age": 25},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := query.NewQueryHelper(tc.query, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create query helper: %v", err)
			}

			projected, err := helper.Project(testRecords)
			if err != nil {
				t.Fatalf("projection failed: %v", err)
			}

			if !reflect.DeepEqual(projected, tc.expectedRecords) {
				t.Errorf("expected records %v, but got %v", tc.expectedRecords, projected)
			}
		})
	}
}

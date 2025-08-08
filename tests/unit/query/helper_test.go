package query_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query" // Assuming this is your module path
)

var testRecords = []common.Document{
	{"id": 1, "name": "Alice", "age": 30, "active": true, "address": map[string]any{"city": "New York"}},
	{"id": 2, "name": "Bob", "age": 25, "active": false, "address": map[string]any{"city": "Los Angeles"}},
	{"id": 3, "name": "Charlie", "age": 35, "active": true, "address": map[string]any{"city": "New York"}},
	{"id": 4, "name": "David", "age": 30, "active": true, "address": map[string]any{"city": "Chicago"}},
	{"id": 5, "name": "Eve", "age": 25, "active": false, "address": nil},
}

func TestNewQueryHelper(t *testing.T) {
	t.Run("valid query", func(t *testing.T) {
		q := &query.Query{}
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
		q := &query.Query{
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
		query       *query.Query
		expectedIDs []int
		expectError bool
	}{
		{
			name: "equal operator",
			query: &query.Query{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{Field: "age", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{NumberVal: floatPtr(30)}},
				},
			},
			expectedIDs: []int{1, 4},
		},
		{
			name: "nested field equal",
			query: &query.Query{
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{Field: "address.city", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{StringVal: stringPtr("New York")}},
				},
			},
			expectedIDs: []int{1, 3},
		},
		{
			name: "AND group",
			query: &query.Query{
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
			query: &query.Query{
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
			query: &query.Query{
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
			query: &query.Query{
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
		query       *query.Query
		expectedIDs []int
	}{
		{
			name: "sort by age ascending",
			query: &query.Query{
				Sort: []query.SortConfiguration{
					{Field: "age", Direction: query.SortDirectionAsc},
				},
			},
			expectedIDs: []int{2, 5, 1, 4, 3},
		},
		{
			name: "sort by age descending, then name ascending",
			query: &query.Query{
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


	testCases := []struct {
		name               string
		query              *query.Query
		expectedIDs        []int
		expectedTotal      *int
	}{
		{
			name: "offset pagination",
			query: &query.Query{
				Pagination: &query.PaginationOptions{Type: "offset", Limit: 2, Offset: &offset},
			},
			expectedIDs:   []int{3, 4},
			expectedTotal: func(i int) *int { return &i }(5),
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


		})
	}
}

func TestProject(t *testing.T) {
	testCases := []struct {
		name            string
		query           *query.Query
		expectedRecords []common.Document
	}{
		{
			name: "include fields",
			query: &query.Query{
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{{Name: "name"}, {Name: "age"}},
				},
			},
			expectedRecords: []common.Document{
				{"name": "Alice", "age": 30},
				{"name": "Bob", "age": 25},
				{"name": "Charlie", "age": 35},
				{"name": "David", "age": 30},
				{"name": "Eve", "age": 25},
			},
		},
		{
			name: "exclude fields",
			query: &query.Query{
				Projection: &query.ProjectionConfiguration{
					Exclude: []query.ProjectionField{{Name: "active"}, {Name: "address"}},
				},
			},
			expectedRecords: []common.Document{
				{"id": 1, "name": "Alice", "age": 30},
				{"id": 2, "name": "Bob", "age": 25},
				{"id": 3, "name": "Charlie", "age": 35},
				{"id": 4, "name": "David", "age": 30},
				{"id": 5, "name": "Eve", "age": 25},
			},
		},
		{
			name: "include nested field",
			query: &query.Query{
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{{Name: "address.city"}},
				},
			},
			expectedRecords: []common.Document{
				{"address": map[string]any{"city": "New York"}},
				{"address": map[string]any{"city": "Los Angeles"}},
				{"address": map[string]any{"city": "New York"}},
				{"address": map[string]any{"city": "Chicago"}},
				{},
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

func TestJoin(t *testing.T) {
	// Sample data for joins
	users := []common.Document{
		{"id": 1, "name": "Alice", "city_id": 101},
		{"id": 2, "name": "Bob", "city_id": 102},
		{"id": 3, "name": "Charlie", "city_id": 101},
		{"id": 4, "name": "David", "city_id": 103},
		{"id": 5, "name": "Eve", "city_id": 999}, // No matching city
	}

	cities := []common.Document{
		{"id": 101, "name": "New York", "country": "USA"},
		{"id": 102, "name": "Los Angeles", "country": "USA"},
		{"id": 103, "name": "London", "country": "UK"},
	}

	testCases := []struct {
		name            string
		leftRecords     []common.Document
		rightRecords    []common.Document
		joinConfig      *query.JoinConfiguration
		expectedRecords []common.Document
		expectError     bool
	}{
		{
			name:         "Inner Join - users and cities on city_id",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: query.JoinTypeInner,
				Target: query.QueryTarget{
					Name: "city",
				},
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectedRecords: []common.Document{
				{"user": common.Document{"id": 1, "name": "Alice", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 3, "name": "Charlie", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 2, "name": "Bob", "city_id": 102}, "city": common.Document{"id": 102, "name": "Los Angeles", "country": "USA"}},
				{"user": common.Document{"id": 4, "name": "David", "city_id": 103}, "city": common.Document{"id": 103, "name": "London", "country": "UK"}},
			},
		},
		{
			name:         "Left Join - users and cities on city_id",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: query.JoinTypeLeft,
				Target: query.QueryTarget{
					Name: "city",
				},
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectedRecords: []common.Document{
				{"user": common.Document{"id": 1, "name": "Alice", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 3, "name": "Charlie", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 2, "name": "Bob", "city_id": 102}, "city": common.Document{"id": 102, "name": "Los Angeles", "country": "USA"}},
				{"user": common.Document{"id": 4, "name": "David", "city_id": 103}, "city": common.Document{"id": 103, "name": "London", "country": "UK"}},
				{"user": common.Document{"id": 5, "name": "Eve", "city_id": 999}, "city": nil},
			},
		},
		{
			name:         "Right Join - users and cities on city_id",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: query.JoinTypeRight,
				Target: query.QueryTarget{
					Name: "city",
				},
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectedRecords: []common.Document{
				{"user": common.Document{"id": 1, "name": "Alice", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 3, "name": "Charlie", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 2, "name": "Bob", "city_id": 102}, "city": common.Document{"id": 102, "name": "Los Angeles", "country": "USA"}},
				{"user": common.Document{"id": 4, "name": "David", "city_id": 103}, "city": common.Document{"id": 103, "name": "London", "country": "UK"}},
			},
		},
		{
			name:         "Full Join - users and cities on city_id",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: query.JoinTypeFull,
				Target: query.QueryTarget{
					Name: "city",
				},
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectedRecords: []common.Document{
				{"user": common.Document{"id": 1, "name": "Alice", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 3, "name": "Charlie", "city_id": 101}, "city": common.Document{"id": 101, "name": "New York", "country": "USA"}},
				{"user": common.Document{"id": 2, "name": "Bob", "city_id": 102}, "city": common.Document{"id": 102, "name": "Los Angeles", "country": "USA"}},
				{"user": common.Document{"id": 4, "name": "David", "city_id": 103}, "city": common.Document{"id": 103, "name": "London", "country": "UK"}},
				{"user": common.Document{"id": 5, "name": "Eve", "city_id": 999}, "city": nil},
			},
		},
		/* {
			name:         "Inner Join with Projection",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type:   query.JoinTypeInner,
				Target: "city",
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
				Projection: &query.ProjectionConfiguration{
					Include: []query.ProjectionField{
						{Name: "user.id"},
						{Name: "user.name"},
						{Name: "city.country"},
					},
				},
			},
			expectedRecords: []common.Document{
				{"user": common.Document{"id": 1, "name": "Alice"}, "city": common.Document{"country": "USA"}},
				{"user": common.Document{"id": 3, "name": "Charlie"}, "city": common.Document{"country": "USA"}},
				{"user": common.Document{"id": 2, "name": "Bob"}, "city": common.Document{"country": "USA"}},
				{"user": common.Document{"id": 4, "name": "David"}, "city": common.Document{"country": "UK"}},
			},
		},
		{
			name:         "Invalid Join Configuration - Missing Target",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: query.JoinTypeInner,
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectError: true,
		},
		{
			name:         "Invalid Join Configuration - Missing On",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type:   query.JoinTypeInner,
				Target: "city",
			},
			expectError: true,
		}, */
		{
			name:         "Invalid Join Configuration - Unsupported Type",
			leftRecords:  users,
			rightRecords: cities,
			joinConfig: &query.JoinConfiguration{
				Type: "unsupported",
				Target: query.QueryTarget{
					Name: "city",
				},
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "user.city_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "city.id"}},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := query.NewQueryHelper(&query.Query{
				Target: &query.QueryTarget{
					Name: "user",
				},
			}, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create query helper: %v", err)
			}

			// For join tests, the "collection" parameter in Join is the name given to the left records
			// within the combined document. Let's use "user" for consistency with the test cases.
			joinedRecords, err := helper.Join(tc.leftRecords, tc.rightRecords, tc.joinConfig)

			if (err != nil) != tc.expectError {
				t.Errorf("expected error: %v, got: %v", tc.expectError, err)
				return
			}
			if tc.expectError {
				return
			}

			// Deep equality comparison might be tricky with map[string]any due to order or nil vs empty map.
			// Sort both expected and actual results if order is not guaranteed by the join type.
			// For these specific test cases, the order is predictable based on the input slices.
			sortedJoins := sortDocumentsByUserID(joinedRecords)
			sortedExpected := sortDocumentsByUserID(tc.expectedRecords)
		    if !reflect.DeepEqual(sortedJoins, sortedExpected) {
				t.Errorf("expected records %v, but got %v", sortedExpected, sortedJoins)
			}
		})
	}
}

// --- utils ---

func sortDocumentsByUserID(docs []common.Document) []common.Document {
	sorted := make([]common.Document, len(docs))
	copy(sorted, docs)
	sort.Slice(sorted, func(i, j int) bool {
		userI, okI := sorted[i]["user"].(common.Document)
		userJ, okJ := sorted[j]["user"].(common.Document)
		if okI && okJ {
			idI, _ := userI["id"].(int)
			idJ, _ := userJ["id"].(int)
			return idI < idJ
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return i < j
	})
	return sorted
}

package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstraint_MarshalJSON_Rule(t *testing.T) {
	tests := []struct {
		name     string
		constraint definition.Constraint
		expected string
	}{
		{
			name: "Rule with Parameters and Fields",
			constraint: definition.Constraint{
				Name:        "test_constraint",
				Description: "description",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Fields:     []definition.FieldName{"field1", "field2"},
					Predicate:  "is_valid",
				}),
			},
			expected: `{
				"name": "test_constraint",
				"description": "description",
				"fields": ["field1", "field2"],
				"predicate": "is_valid"
			}`,
		},
		{
			name: "Rule with null Parameters, should omit",
			constraint: definition.Constraint{
				Name:        "test_constraint_no_params",
				Description: "description",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Fields:     []definition.FieldName{"field1"},
					Predicate:  "is_valid",
					Parameters: definition.NewNullLiteral(), // Use NewNullLiteral for null
				}),
			},
			expected: `{
				"name": "test_constraint_no_params",
				"description": "description",
				"fields": ["field1"],
				"predicate": "is_valid"
			}`,
		},
		{
			name: "Rule with no Fields, should omit",
			constraint: definition.Constraint{
				Name:        "test_constraint_no_fields",
				Description: "description",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Predicate:  "is_valid",
				}),
			},
			expected: `{
				"name": "test_constraint_no_fields",
				"description": "description",
				"predicate": "is_valid"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.constraint)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestConstraint_MarshalJSON_Group(t *testing.T) {
	tests := []struct {
		name     string
		constraint definition.Constraint
		expected string
	}{
		{
			name: "Group with one rule",
			constraint: definition.Constraint{
				Name:        "test_group_constraint",
				Description: "group description",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
					Operator: common.LogicalAnd,
					Rules: []definition.ConstraintUnion{
						definition.NewConstrainUnion(&definition.ConstraintRule{
							Fields:     []definition.FieldName{"field3"},
							Predicate:  "is_present",
							Parameters: definition.NewNullLiteral(), // Use NewNullLiteral
						}),
					},
				}),
			},
			expected: `{
				"name": "test_group_constraint",
				"description": "group description",
				"operator": "and",
				"rules": [
					{
						"fields": ["field3"],
						"predicate": "is_present"
					}
				]
			}`,
		},
		{
			name: "Group with multiple rules and nested group",
			constraint: definition.Constraint{
				Name:        "complex_group",
				Description: "A complex group of constraints",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
					Operator: common.LogicalOr,
					Rules: []definition.ConstraintUnion{
						definition.NewConstrainUnion(&definition.ConstraintRule{
							Fields:    []definition.FieldName{"fieldA"},
							Predicate: "is_valid_A",
						}),
						definition.NewConstrainUnion(&definition.ConstraintGroup{
							Operator: common.LogicalAnd,
							Rules: []definition.ConstraintUnion{
								definition.NewConstrainUnion(&definition.ConstraintRule{
									Fields:    []definition.FieldName{"fieldB"},
									Predicate: "is_valid_B",
								}),
								definition.NewConstrainUnion(&definition.ConstraintRule{
									Fields:     []definition.FieldName{"fieldC"},
									Predicate:  "has_value",
									Parameters: func() definition.LiteralValue { lv, _ := definition.NewLiteralValue(int64(10)); return lv }(), // Use int64
								}),
							},
						}),
					},
				}),
			},
			expected: `{
				"name": "complex_group",
				"description": "A complex group of constraints",
				"operator": "or",
				"rules": [
					{
						"fields": ["fieldA"],
						"predicate": "is_valid_A"
					},
					{
						"operator": "and",
						"rules": [
							{
								"fields": ["fieldB"],
								"predicate": "is_valid_B"
							},
							{
								"fields": ["fieldC"],
								"predicate": "has_value",
								"parameters": 10
							}
						]
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.constraint)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestConstraint_UnmarshalJSON_Rule(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected definition.Constraint
		wantErr  bool
	}{
		{
			name:    "Rule with Fields and Predicate",
			jsonStr: `{ "name": "unmarshal_rule", "description": "unmarshal rule desc", "fields": ["fieldA"], "predicate": "is_ok" }`,
			expected: definition.Constraint{
				Name:        "unmarshal_rule",
				Description: "unmarshal rule desc",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Fields:    []definition.FieldName{"fieldA"},
					Predicate: "is_ok",
				}),
			},
			wantErr: false,
		},
		{
			name:    "Rule with Parameters",
			jsonStr: `{ "name": "unmarshal_rule_params", "predicate": "is_ok_param", "parameters": "some_value" }`,
			expected: definition.Constraint{
				Name: "unmarshal_rule_params",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Predicate:  "is_ok_param",
					Parameters: func() definition.LiteralValue { lv, _ := definition.NewLiteralValue("some_value"); return lv }(),
				}),
			},
			wantErr: false,
		},
		{
			name:    "Rule with empty Parameters (explicit null)",
			jsonStr: `{ "name": "unmarshal_rule_empty_params", "predicate": "is_ok_empty_param", "parameters": null }`,
			expected: definition.Constraint{
				Name: "unmarshal_rule_empty_params",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
					Predicate:  "is_ok_empty_param",
					Parameters: definition.NewNullLiteral(),
				}),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var constraint definition.Constraint
			err := json.Unmarshal([]byte(tt.jsonStr), &constraint)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Name, constraint.Name)
			assert.Equal(t, tt.expected.Description, constraint.Description)
			assert.Equal(t, tt.expected.ConstraintUnion.Kind(), constraint.ConstraintUnion.Kind())

			expectedRule, err := definition.ConstraintAs[*definition.ConstraintRule](tt.expected.ConstraintUnion)
			require.NoError(t, err)
			actualRule, err := definition.ConstraintAs[*definition.ConstraintRule](constraint.ConstraintUnion)
			require.NoError(t, err)

			assert.Equal(t, expectedRule.Fields, actualRule.Fields)
			assert.Equal(t, expectedRule.Predicate, actualRule.Predicate)
			assert.Equal(t, expectedRule.Parameters.Value(), actualRule.Parameters.Value())
		})
	}
}

func TestConstraint_UnmarshalJSON_Group(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected definition.Constraint
		wantErr  bool
	}{
		{
			name:    "Group with one rule",
			jsonStr: `{ "name": "unmarshal_group", "description": "unmarshal group desc", "operator": "or", "rules": [ { "fields": ["fieldB"], "predicate": "is_not_empty" } ] }`,
			expected: definition.Constraint{
				Name:        "unmarshal_group",
				Description: "unmarshal group desc",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
					Operator: common.LogicalOr,
					Rules: []definition.ConstraintUnion{
						definition.NewConstrainUnion(&definition.ConstraintRule{
							Fields:    []definition.FieldName{"fieldB"},
							Predicate: "is_not_empty",
						}),
					},
				}),
			},
			wantErr: false,
		},
		{
			name:    "Group with nested group",
			jsonStr: `{ "name": "unmarshal_nested_group", "operator": "and", "rules": [ { "predicate": "rule1" }, { "operator": "or", "rules": [ { "predicate": "rule2" } ] } ] }`,
			expected: definition.Constraint{
				Name: "unmarshal_nested_group",
				ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
					Operator: common.LogicalAnd,
					Rules: []definition.ConstraintUnion{
						definition.NewConstrainUnion(&definition.ConstraintRule{
							Predicate: "rule1",
						}),
						definition.NewConstrainUnion(&definition.ConstraintGroup{
							Operator: common.LogicalOr,
							Rules: []definition.ConstraintUnion{
								definition.NewConstrainUnion(&definition.ConstraintRule{
									Predicate: "rule2",
								}),
							},
						}),
					},
				}),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var constraint definition.Constraint
			err := json.Unmarshal([]byte(tt.jsonStr), &constraint)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Name, constraint.Name)
			assert.Equal(t, tt.expected.Description, constraint.Description)
			assert.Equal(t, tt.expected.ConstraintUnion.Kind(), constraint.ConstraintUnion.Kind())

			expectedGroup, err := definition.ConstraintAs[*definition.ConstraintGroup](tt.expected.ConstraintUnion)
			require.NoError(t, err)
			actualGroup, err := definition.ConstraintAs[*definition.ConstraintGroup](constraint.ConstraintUnion)
			require.NoError(t, err)

			assert.Equal(t, expectedGroup.Operator, actualGroup.Operator)
			require.Len(t, actualGroup.Rules, len(expectedGroup.Rules))

			// Deep compare rules - simplified for now, could be recursive
			// This check assumes simple rules or groups at the top level
			for i := range actualGroup.Rules {
				actualRule := actualGroup.Rules[i]
				expectedRule := expectedGroup.Rules[i]

				if expectedRule.Kind() == definition.ConstraintKindRule {
					er, eErr := definition.ConstraintAs[*definition.ConstraintRule](expectedRule)
					ar, aErr := definition.ConstraintAs[*definition.ConstraintRule](actualRule)
					require.NoError(t, eErr)
					require.NoError(t, aErr)
					assert.Equal(t, er.Fields, ar.Fields)
					assert.Equal(t, er.Predicate, ar.Predicate)
					assert.Equal(t, er.Parameters.Value(), ar.Parameters.Value())
				} else if expectedRule.Kind() == definition.ConstraintKindGroup {
					eg, eErr := definition.ConstraintAs[*definition.ConstraintGroup](expectedRule)
					ag, aErr := definition.ConstraintAs[*definition.ConstraintGroup](actualRule)
					require.NoError(t, eErr)
					require.NoError(t, aErr)
					assert.Equal(t, eg.Operator, ag.Operator)
					// Further deep comparison for nested groups is needed here for full coverage
				}
			}
		})
	}
}

func TestConstraintUnion_UnmarshalJSON_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{
			name:    "Malformed JSON",
			jsonStr: `{"name": "invalid", "description": "desc", "fields": ["f1"], "predicate": "p1", "parameters":`,
			wantErr: true,
		},
		{
			name:    "Neither operator nor predicate present",
			jsonStr: `{"name": "invalid", "description": "desc"}`, // Missing predicate and operator
			wantErr: true,
		},
		{
			name:    "Both operator and predicate present (invalid)",
			jsonStr: `{"name": "invalid", "predicate": "p1", "operator": "and"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var constraint definition.Constraint
			err := json.Unmarshal([]byte(tt.jsonStr), &constraint)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConstraintUnion_ValueAndKind(t *testing.T) {
	// For LiteralValue type, it should be passed as string, int64, float64, etc.
	// definition.NewLiteralValue("some_rule") is effectively a LiteralValue with a string.
	// So, the PredicateName (which is a string) is correctly derived from LiteralValue.String()
	// This test focuses on the ConstraintUnion and ConstraintAs behavior.

	rule := &definition.ConstraintRule{Predicate: "some_rule"}
	group := &definition.ConstraintGroup{Operator: common.LogicalAnd}

	cuRule := definition.NewConstrainUnion(rule)
	assert.Equal(t, definition.ConstraintKindRule, cuRule.Kind())
	payloadRule, err := definition.ConstraintAs[*definition.ConstraintRule](cuRule)
	require.NoError(t, err)
	assert.Equal(t, rule, payloadRule) // Direct comparison of pointers for NewConstrainUnion

	cuGroup := definition.NewConstrainUnion(group)
	assert.Equal(t, definition.ConstraintKindGroup, cuGroup.Kind())
	assert.Equal(t, group, cuGroup.Value()) // Value() returns the underlying payload

	cuNil := definition.NewConstrainUnion[*definition.ConstraintRule](nil)
	assert.Equal(t, definition.ConstraintKind(0), cuNil.Kind()) // Default zero value for byte
	assert.Nil(t, cuNil.Value())
}

func TestConstraintAs(t *testing.T) {
	rule := &definition.ConstraintRule{Predicate: "check"}
	group := &definition.ConstraintGroup{Operator: common.LogicalOr}

	cuRule := definition.NewConstrainUnion(rule)
	actualRule, err := definition.ConstraintAs[*definition.ConstraintRule](cuRule)
	require.NoError(t, err)
	assert.Equal(t, rule, actualRule)

	// Error case: trying to cast a rule as a group
	_, err = definition.ConstraintAs[*definition.ConstraintGroup](cuRule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type mismatch")

	cuGroup := definition.NewConstrainUnion(group)
	actualGroup, err := definition.ConstraintAs[*definition.ConstraintGroup](cuGroup)
	require.NoError(t, err)
	assert.Equal(t, group, actualGroup)

	// Error case: trying to cast a group as a rule
	_, err = definition.ConstraintAs[*definition.ConstraintRule](cuGroup)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type mismatch")
}

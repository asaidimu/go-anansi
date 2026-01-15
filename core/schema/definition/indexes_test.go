package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexConditionUnion_MarshalJSON_Condition(t *testing.T) {
	strVal := "some_value"
	lv := MustNewLiteralValueStrict(strVal)
	icu := definition.NewIndexConditionUnion(&definition.IndexCondition{
		Field:    "field1",
		Operator: common.Equal,
		Value:    lv, // lv is already a LiteralValue
	})

	data, err := json.Marshal(icu)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.JSONEq(t, `"field1"`, string(raw["field"]))
	assert.JSONEq(t, `"eq"`, string(raw["operator"]))
	assert.JSONEq(t, `"some_value"`, string(raw["value"]))
}

func TestIndexConditionUnion_MarshalJSON_Group(t *testing.T) {
	icu := definition.NewIndexConditionUnion(&definition.IndexConditionGroup{
		Operator: common.LogicalAnd,
		Conditions: []definition.IndexConditionUnion{
			definition.NewIndexConditionUnion(&definition.IndexCondition{
				Field:    "field2",
				Operator: common.GreaterThan,
			}),
		},
	})

	data, err := json.Marshal(icu)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.JSONEq(t, `"and"`, string(raw["operator"]))
	assert.Contains(t, string(raw["conditions"]), `"field":"field2"`)
}

func TestIndexConditionUnion_UnmarshalJSON_Condition(t *testing.T) {
	jsonStr := `{
		"field": "fieldA",
		"operator": "neq",
		"value": "another_value"
	}`

	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte(jsonStr), &icu)
	require.NoError(t, err)

	cond, errA := definition.IndexConditionAs[*definition.IndexCondition](icu)
	require.NoError(t, errA)
	require.NotNil(t, cond)

	assert.Equal(t, definition.FieldId("fieldA"), cond.Field)
	assert.Equal(t, common.NotEqual, cond.Operator)

	val, errB := definition.LiteralValueAs[string](cond.Value)
	require.NoError(t, errB)
	assert.Equal(t, "another_value", val)
}

func TestIndexConditionUnion_UnmarshalJSON_Group(t *testing.T) {
	jsonStr := `{
		"operator": "or",
		"conditions": [
			{
				"field": "fieldB",
				"operator": "lt",
				"value": 100
			}
		]
	}`

	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte(jsonStr), &icu)
	require.NoError(t, err)

	group, errG := definition.IndexConditionAs[*definition.IndexConditionGroup](icu)
	require.NoError(t, errG)
	require.NotNil(t, group)

	assert.Equal(t, common.LogicalOr, group.Operator)
	require.Len(t, group.Conditions, 1)

	// Nested condition
	nestedIcu := group.Conditions[0]
	cond, errC := definition.IndexConditionAs[*definition.IndexCondition](nestedIcu)
	require.NoError(t, errC)
	require.NotNil(t, cond)

	assert.Equal(t, definition.FieldId("fieldB"), cond.Field)
	assert.Equal(t, common.LessThan, cond.Operator)

	val, errV := definition.LiteralValueAs[int64](cond.Value)
	require.NoError(t, errV)
	assert.Equal(t, int64(100), val)
}

func TestIndexConditionUnion_Helpers(t *testing.T) {
	// Test NewIndexCondition
	lv := MustNewLiteralValueStrict("value")
	condIcu := definition.NewIndexConditionUnion(&definition.IndexCondition{
		Field:    "field",
		Operator: common.Equal,
		Value:    lv,
	})

	cond, err := definition.IndexConditionAs[*definition.IndexCondition](condIcu)
	require.NoError(t, err)
	require.NotNil(t, cond)

	assert.True(t, condIcu.IsCondition())
	assert.False(t, condIcu.IsConditionGroup())
	assert.False(t, condIcu.IsZero())

	// Test NewIndexConditionGroup
	groupIcu := definition.NewIndexConditionUnion(&definition.IndexConditionGroup{
		Operator: common.LogicalAnd,
		Conditions: []definition.IndexConditionUnion{condIcu},
	})

	group, errG := definition.IndexConditionAs[*definition.IndexConditionGroup](groupIcu)
	require.NoError(t, errG)
	require.NotNil(t, group)

	assert.False(t, groupIcu.IsCondition())
	assert.True(t, groupIcu.IsConditionGroup())
	assert.False(t, groupIcu.IsZero())

	// Test IsZero
	zero := definition.IndexConditionUnion{}
	assert.True(t, zero.IsZero())
}

func TestIndexConditionUnion_MarshalJSON_Zero(t *testing.T) {
	icu := definition.IndexConditionUnion{}
	data, err := json.Marshal(icu)
	require.NoError(t, err)
	assert.JSONEq(t, `null`, string(data))
}

func TestIndexConditionUnion_UnmarshalJSON_Null(t *testing.T) {
	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte("null"), &icu)
	require.NoError(t, err)
	assert.True(t, icu.IsZero())
}

func TestIndexConditionUnion_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte("{"), &icu)
	require.Error(t, err)
}

func TestIndexConditionUnion_UnmarshalJSON_NeitherFieldNorConditions(t *testing.T) {
	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte(`{"foo": "bar"}`), &icu)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IndexConditionUnion")
}

func TestIndexConditionUnion_UnmarshalJSON_InvalidGroup(t *testing.T) {
	jsonStr := `{"conditions": "not_an_array"}`
	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte(jsonStr), &icu)
	require.Error(t, err)
}

func TestIndexConditionUnion_UnmarshalJSON_InvalidCondition(t *testing.T) {
	jsonStr := `{"field": 123}`
	var icu definition.IndexConditionUnion
	err := json.Unmarshal([]byte(jsonStr), &icu)
	require.Error(t, err)
}




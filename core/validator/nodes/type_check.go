package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// TypeCheckNode validates that a field's value matches its defined FieldType.
type TypeCheckNode struct {
	BaseNode[any]
	FieldDef *schema.FieldDefinition
}

// Execute performs the validation for TypeCheckNode.
func (n *TypeCheckNode) Execute(ctx *types.ValidationContext) *types.NodeResult {
	return n.execute(ctx, true, func(data any, valueType reflect.Type) *types.NodeResult {
		issues := make([]types.Issue, 0)
		ok := true
		switch n.FieldDef.Type {
		case schema.FieldTypeString:
			ok = valueType.Kind() == reflect.String
		case schema.FieldTypeNumber, schema.FieldTypeDecimal:
			// Go's JSON unmarshaling converts all numbers to float64 by default.
			// So we check for float64, float32, or various int types.
			switch data.(type) {
			case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
				ok = true
			default:
				ok = false
			}
		case schema.FieldTypeInteger:
			switch data.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
				ok = true
			default:
				ok = false
			}
		case schema.FieldTypeBoolean:
			ok = valueType.Kind() == reflect.Bool
		case schema.FieldTypeArray, schema.FieldTypeSet:
			ok = valueType.Kind() == reflect.Slice || valueType.Kind() == reflect.Array
		case schema.FieldTypeObject, schema.FieldTypeRecord:
			ok = valueType.Kind() == reflect.Map
		case schema.FieldTypeUnion, schema.FieldTypeEnum:
			// Union and Enum types require further semantic validation/matching,
			// the base type check might pass for any underlying type that matches
			// one of the union/enum members. We assume here that these types
			// don't have a direct "base type" validation at this level
			// beyond what their members would imply.
			// For Enum, the value should be a string (or whatever the enum type is defined as)
			// For Union, it could be anything allowed by its member types.
			// We'll let specific EnumValidationNode and UnionValidationNode handle the details.
			return &types.NodeResult{Success: true} // Defer specific validation
		default:
			// Unknown type, consider it a failure for safety or ignore depending on strictness
			issues = append(issues, types.Issue{
				Code:    "UNKNOWN_FIELD_TYPE",
				Message: fmt.Sprintf("Unsupported field type '%s' defined for field '%s'", n.FieldDef.Type, n.FieldDef.Name),
				Path:    n.path,
			})
			return &types.NodeResult{Success: false, Issues: issues}
		}

		if !ok {
			issues = append(issues, types.Issue{
				Code:    "TYPE_MISMATCH",
				Message: fmt.Sprintf("Expected type '%s' for field '%s', got '%T'", n.FieldDef.Type, n.FieldDef.Name, data),
				Path:    n.path,
			})
		}

		return &types.NodeResult{
			Success: len(issues) == 0,
			Issues:  issues,
		}

	})

}

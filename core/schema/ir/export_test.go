package ir

import "encoding/json"

// Export unexported members for testing.
var (
	PackDescriptor            = packDescriptor
	EnumElemTypeToArrayDataType = enumElemTypeToArrayDataType
	DescriptorToEnumDocumentKey = descriptorToEnumDocumentKey
	InferEnumElemType         = inferEnumElemType
	FieldTypeToDataType       = fieldTypeToDataType
	BuildFieldIndex           = buildFieldIndex
	BuildSchemaIndex          = buildSchemaIndex
	GetDefaultFromStore       = getDefaultFromStore
	SchemaOffsetRange         = schemaOffsetRange
	AddressSpaceMax           = addressSpaceMax
	Validate                  = func(src []byte) []CompileError {
		var s sourceSchema
		if err := json.Unmarshal(src, &s); err != nil {
			return []CompileError{{Pass: PassParse, Message: err.Error()}}
		}
		return validateSource(&s)
	}
)

type FieldIndexInternal = fieldIndex
type SchemaIndexInternal = schemaIndex

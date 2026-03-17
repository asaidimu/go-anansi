package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// Export unexported members for testing.
var (
	PackDescriptor            = packDescriptor
	EnumElemTypeToArrayDataType = enumElemTypeToArrayDataType
	DescriptorToEnumDataPoint = descriptorToEnumDataPoint
	InferEnumElemType         = inferEnumElemType
	FieldTypeToDataType       = fieldTypeToDataType
	BuildFieldIndex           = buildFieldIndex
	BuildSchemaIndex          = buildSchemaIndex
	GetDefaultFromStore       = getDefaultFromStore
)

type SourceSchemaInternal = sourceSchema
type FieldIndexInternal = fieldIndex
type SchemaIndexInternal = schemaIndex

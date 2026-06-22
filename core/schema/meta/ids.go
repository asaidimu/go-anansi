package meta

import "github.com/asaidimu/go-anansi/v7/core/schema/definition"

// -----------------------------------------------------------------------------
// Constraint IDs
// Each constant is a stable UUIDv7, frozen at generation time.
// Never regenerate — these are persisted in the registry and used for migrations.
// -----------------------------------------------------------------------------

const (
	ConstraintPrimitivesNoSchema                      definition.ConstraintId = "019d7775-6563-7c55-a6f3-ac8f087d89d1"
	ConstraintEnumFieldsValid                      definition.ConstraintId = "019d7775-6563-7605-8bfb-c27365b73581"
	ConstraintArraysRequireSchema                     definition.ConstraintId = "019d7775-6563-74fe-a5ae-4eef62a128d9"
	ConstraintObjectsRequireSchema                    definition.ConstraintId = "019d7775-6563-7080-ae67-bad5cd864f07"
	ConstraintUnionsRequireMultipleSchemas             definition.ConstraintId = "019d7775-6563-7e83-92c4-0258a8fc0d3a"
	ConstraintCompositesRequireMultipleSchemas         definition.ConstraintId = "019d7775-6563-73ce-aef1-86943b54a1ca"
	ConstraintNestedSchemaModeExclusive               definition.ConstraintId = "019d7775-6563-7554-bfae-d6ad7774d5ac"
	ConstraintConstraintTypeExclusive                 definition.ConstraintId = "019d7775-6563-7028-b96f-bc6e0abefc15"
	ConstraintConstraintRuleRequiresPredicate         definition.ConstraintId = "019d7775-6563-752b-bf9d-bf8936472250"
	ConstraintIndexConditionExclusiveType             definition.ConstraintId = "019d7775-6563-713e-8355-df812c6bb8c9"
	ConstraintSchemaNameRequired                      definition.ConstraintId = "019d7775-6563-7f89-8368-395733efec86"
	ConstraintFieldNameRequired                       definition.ConstraintId = "019d7775-6563-78cc-a95e-1ecc6fdfd639"
	ConstraintIndexFieldsNotEmpty                     definition.ConstraintId = "019d7775-6564-7285-a734-f7eed15d25e4"
	ConstraintSchemaReferenceIdRequired               definition.ConstraintId = "019d7775-6564-79f2-9d64-d6b8c2696deb"
	ConstraintRecordsAllowOptionalSchema              definition.ConstraintId = "019d7775-6564-77ca-94bc-30a07da76c3d"
	ConstraintSchemaReferenceIntegrity                definition.ConstraintId = "019d7775-6564-7a5d-bb7e-d2e9e804123f"
	ConstraintDefaultValueTypeMatch                   definition.ConstraintId = "019d7775-6564-7c60-9f9f-aeeabe05c1b5"
	ConstraintIndexFieldsExist                        definition.ConstraintId = "019d7775-6564-7c2d-b06d-82971c0b1fa9"
	ConstraintConstraintFieldsExist                   definition.ConstraintId = "019d7775-6564-7fc2-bbdb-0fc59e567f2b"
	ConstraintIndexConditionValueTypeMatch            definition.ConstraintId = "019d7775-6564-712b-8c53-9fa93809f219"
	ConstraintCompositeReferencedSchemasMustBeObjects definition.ConstraintId = "019d7775-6564-75ce-a807-02396e0b8394"
	ConstraintObjectReferencedSchemaHasFields         definition.ConstraintId = "019d7775-6564-719d-9948-67dd885e7632"
	ConstraintSpatialIndexOnGeometryField             definition.ConstraintId = "019d7775-6564-7660-adc8-3f1ab037d695"
	ConstraintGlobalFieldIdUniqueness                 definition.ConstraintId = "019d7775-6564-7565-b53c-49667d23c237"
	ConstraintInlineTypeDescriptorValid               definition.ConstraintId = "019d7775-6564-7768-8995-72c282cfe69f"
	ConstraintSchemaReferenceFormCorrect              definition.ConstraintId = "019d7775-6564-75bc-abf5-6ddd81b7d2d4"
)

// -----------------------------------------------------------------------------
// Top-level Schema field IDs
// -----------------------------------------------------------------------------

const (
	FieldIDName        definition.FieldId = "019d7775-6565-7c1f-a6f7-aeb5c4eb35e4"
	FieldIDDescription definition.FieldId = "019d7775-6565-7545-9e25-fc471f57bcfb"
	FieldIDVersion     definition.FieldId = "019d7775-6565-7bdc-a9d4-776b1f163346"
	FieldIDFields      definition.FieldId = "019d7775-6565-7755-904d-2d0cdf0e1b86"
	FieldIDIndexes     definition.FieldId = "019d7775-6565-73d6-a7be-130b3c1e93c8"
	FieldIDConstraints definition.FieldId = "019d7775-6565-722f-9a5a-2810f537a61c"
	FieldIDMetadata    definition.FieldId = "019d7775-6565-74ed-8a52-12027f3c577d"
	FieldIDSchemas     definition.FieldId = "019d7775-6565-7fe8-86f6-57a8a305a314"
)

// -----------------------------------------------------------------------------
// Nested schema IDs (keys in MetaSchema.Schemas)
// -----------------------------------------------------------------------------

const (
	SchemaIDField                  definition.SchemaId = "019d7775-6565-7f15-b0f4-27a4f40cbf5c"
	SchemaIDNestedSchema           definition.SchemaId = "019d7775-6565-7db3-9bb0-be9fc50550bd"
	SchemaIDIndex                  definition.SchemaId = "019d7775-6565-74f6-9c72-9e35d3d16b0a"
	SchemaIDIndexCondition         definition.SchemaId = "019d7775-6565-71ef-a08f-cdcdb116f947"
	SchemaIDIndexConditionGroup    definition.SchemaId = "019d7775-6565-7d6a-a531-47e7ea838bc4"
	SchemaIDIndexConditionUnion    definition.SchemaId = "019d7775-6565-74a3-a6e0-881a0148fc59"
	SchemaIDConstraint             definition.SchemaId = "019d7775-6565-7ab6-9c65-ab60632aeca8"
	SchemaIDConstraintMetadata     definition.SchemaId = "019d7775-6565-7068-9cb9-28460e0368f0"
	SchemaIDConstraintUnion        definition.SchemaId = "019d7775-6565-794d-8aba-bdc5e3cbe97e"
	SchemaIDConstraintRule         definition.SchemaId = "019d7775-6565-703f-95cf-e7511d33c91b"
	SchemaIDConstraintGroup        definition.SchemaId = "019d7775-6565-7ea4-a820-e8bcdbc690bc"
	SchemaIDSchemaReference        definition.SchemaId = "019d7775-6565-7f2a-956b-35b40caa8ec7"
	SchemaIDSchemaReferenceArray   definition.SchemaId = "019d7775-6565-71bc-84f3-044dc089d50c"
	SchemaIDString                 definition.SchemaId = "019d7775-6566-7a63-9e10-5d11c8eb607c"
	SchemaIDUnknown                definition.SchemaId = "019d7775-6566-7da1-b24a-67bd81dd2b0e"
	SchemaIDInlineTypeDescriptor   definition.SchemaId = "019d7775-6566-7786-8769-c13eb5a883d9"
	SchemaIDInlineTypeEnum         definition.SchemaId = "019d7775-6566-7e05-ab1b-6f768f459927"
	SchemaIDFieldTypeEnum          definition.SchemaId = "019d7775-6566-765a-b142-157d1d6367b7"
	SchemaIDIndexTypeEnum          definition.SchemaId = "019d7775-6566-7df4-bc09-67956251c4a2"
	SchemaIDLogicalOperatorEnum    definition.SchemaId = "019d7775-6566-7e3b-860a-c4aa4adf5d6d"
	SchemaIDIndexOrderEnum         definition.SchemaId = "019d7775-6566-7f2f-a009-0af144502aac"
	SchemaIDComparisonOperatorEnum definition.SchemaId = "019d7775-6566-7a70-8b56-e55704397fc2"
)

// -----------------------------------------------------------------------------
// Field schema: Field nested schema field IDs
// -----------------------------------------------------------------------------

const (
	FieldFieldIDName        definition.FieldId = "019d7775-6566-7a4c-877d-262c18adb028"
	FieldFieldIDDescription definition.FieldId = "019d7775-6566-7b93-aa4c-b1e3869512d8"
	FieldFieldIDType        definition.FieldId = "019d7775-6566-727e-b4ff-4e6e8b800808"
	FieldFieldIDDefault     definition.FieldId = "019d7775-6566-7ed5-9fb3-33a67b90dbbe"
	FieldFieldIDSchema      definition.FieldId = "019d7775-6566-79e9-a60b-1c15e2154580"
	FieldFieldIDRequired    definition.FieldId = "019d7775-6566-722b-8132-d5d8c4c9b2b6"
	FieldFieldIDDeprecated  definition.FieldId = "019d7775-6566-7367-9664-adb25994bf41"
	FieldFieldIDUnique      definition.FieldId = "019d7775-6566-7fc8-82c6-735838cdfe21"
)

// -----------------------------------------------------------------------------
// NestedSchema nested schema field IDs
// -----------------------------------------------------------------------------

const (
	NestedSchemaFieldIDName        definition.FieldId = "019d7775-6566-748e-b099-50c76a5b79c2"
	NestedSchemaFieldIDDescription definition.FieldId = "019d7775-6566-74df-a8df-038cc26af034"
	NestedSchemaFieldIDFields      definition.FieldId = "019d7775-6566-77a1-9b36-bbd57f927d11"
	NestedSchemaFieldIDIndexes     definition.FieldId = "019d7775-6567-7f25-b7e2-acc72435964f"
	NestedSchemaFieldIDConstraints definition.FieldId = "019d7775-6567-750b-a618-5bcdc4e5d04a"
	NestedSchemaFieldIDMetadata    definition.FieldId = "019d7775-6567-748e-bcad-76068cc4739f"
	NestedSchemaFieldIDType        definition.FieldId = "019d7775-6567-7931-b2d6-fb52039c28b5"
	NestedSchemaFieldIDDefault     definition.FieldId = "019d7775-6567-75c4-b399-eb4764617462"
	NestedSchemaFieldIDValues      definition.FieldId = "019d7775-6567-7742-90ae-a96178ae505d"
	NestedSchemaFieldIDSchema      definition.FieldId = "019d7775-6567-7709-a7e3-ed75f833da15"
	NestedSchemaFieldIDConcrete    definition.FieldId = "019d7775-6567-7022-b7fb-3975ccae2b72"
)

// -----------------------------------------------------------------------------
// Index nested schema field IDs
// -----------------------------------------------------------------------------

const (
	IndexFieldIDName        definition.FieldId = "019d7775-6567-7390-96c6-8154ccc6d1ad"
	IndexFieldIDDescription definition.FieldId = "019d7775-6567-7568-98c8-76d64b745bc7"
	IndexFieldIDType        definition.FieldId = "019d7775-6567-7e59-a2f4-ffc431e24fdd"
	IndexFieldIDFields      definition.FieldId = "019d7775-6567-787d-921c-fa28f53cedcd"
	IndexFieldIDUnique      definition.FieldId = "019d7775-6567-7a3d-b154-1199406b7851"
	IndexFieldIDOrder       definition.FieldId = "019d7775-6567-76c9-869a-c4e25c154583"
	IndexFieldIDCondition   definition.FieldId = "019d7775-6567-7bc2-b870-6e1ae4faa558"
)

// -----------------------------------------------------------------------------
// IndexCondition nested schema field IDs
// -----------------------------------------------------------------------------

const (
	IndexConditionFieldIDField    definition.FieldId = "019d7775-6567-7466-a873-e7866e9d031f"
	IndexConditionFieldIDOperator definition.FieldId = "019d7775-6567-75ef-8e82-347da30f06fd"
	IndexConditionFieldIDValue    definition.FieldId = "019d7775-6567-7526-9704-d19fb4eb8d8d"
)

// -----------------------------------------------------------------------------
// IndexConditionGroup nested schema field IDs
// -----------------------------------------------------------------------------

const (
	IndexConditionGroupFieldIDOperator    definition.FieldId = "019d7775-6567-7b4e-893f-ee7afb0c4be1"
	IndexConditionGroupFieldIDConditions  definition.FieldId = "019d7775-6567-7af2-817f-bc5b8732c198"
)

// -----------------------------------------------------------------------------
// ConstraintMetadata nested schema field IDs
// -----------------------------------------------------------------------------

const (
	ConstraintMetadataFieldIDName        definition.FieldId = "019d7775-6567-7017-9068-5bfcd1f9fef0"
	ConstraintMetadataFieldIDDescription definition.FieldId = "019d7775-6568-7b0d-ad28-391874b5e3de"
)

// -----------------------------------------------------------------------------
// ConstraintRule nested schema field IDs
// -----------------------------------------------------------------------------

const (
	ConstraintRuleFieldIDFields     definition.FieldId = "019d7775-6568-7d8b-a230-328fe5d352e8"
	ConstraintRuleFieldIDPredicate  definition.FieldId = "019d7775-6568-7ad6-9e45-7fdbdef3fc7e"
	ConstraintRuleFieldIDParameters definition.FieldId = "019d7775-6568-7221-a4de-7e82d6c9f6bc"
)

// -----------------------------------------------------------------------------
// ConstraintGroup nested schema field IDs
// -----------------------------------------------------------------------------

const (
	ConstraintGroupFieldIDOperator definition.FieldId = "019d7775-6568-7ebf-a1c3-829de95d9216"
	ConstraintGroupFieldIDRules    definition.FieldId = "019d7775-6568-76d9-a21c-fb289c36cc70"
)

// -----------------------------------------------------------------------------
// SchemaReference nested schema field IDs
// -----------------------------------------------------------------------------

const (
	SchemaReferenceFieldIDID          definition.FieldId = "019d7775-6568-762c-932e-89b85f08a951"
	SchemaReferenceFieldIDIndexes     definition.FieldId = "019d7775-6568-77eb-bd9d-d874a2354fd5"
	SchemaReferenceFieldIDConstraints definition.FieldId = "019d7775-6568-7ed9-99b3-90cca439ee2e"
)

// -----------------------------------------------------------------------------
// InlineTypeDescriptor nested schema field IDs
// -----------------------------------------------------------------------------

const (
	InlineTypeDescriptorFieldIDType   definition.FieldId = "019d7775-6568-713f-867f-93dde123ccfd"
	InlineTypeDescriptorFieldIDValues definition.FieldId = "019d7775-6568-7ae5-95b7-56c97722c732"
)

-   **Test Suite: `MigrationApplier` (`applier.go`)**
    *   **Description:** Tests the `MigrationApplier`'s ability to correctly apply schema changes to a `SchemaDefinition`, including various types of modifications, additions, and removals, respecting `StrictMode` and performing necessary validations.
    *   **Test Case: Successful `ApplyMigration`**
        *   Description and purpose: Verify that a valid migration with multiple changes is applied correctly, producing a new schema with the target version, and the source schema remains unchanged.
        *   Involved components: `NewMigrationApplier`, `ApplyMigration`, `deepCloneSchema`, `applyChanges`.
        *   Necessary assertions: No error returned; `target.Version` matches `migration.Version.Target`; `source` and `target` are different objects (deep clone verified); `source` schema content is unchanged; `target` schema content reflects all applied changes.
    *   **Test Case: `ApplyMigration` with No Changes**
        *   Description and purpose: Verify that if a migration contains no changes, the resulting schema is identical to the source, but with an updated version.
        *   Involved components: `ApplyMigration`.
        *   Necessary assertions: No error returned; `target.Version` matches `migration.Version.Target`; `target` schema content is otherwise identical to `source`.
    *   **Test Case: `ApplyMigration` - `StrictMode` prevents adding existing field**
        *   Description and purpose: Ensure that with `StrictMode` enabled, attempting to add a field that already exists returns an error.
        *   Involved components: `NewMigrationApplier` (with `StrictMode: true`), `applyAddField`.
        *   Necessary assertions: `common.SystemError` with `ERR_FIELD_ALREADY_EXISTS` returned; schema remains unchanged.
    *   **Test Case: `ApplyMigration` - `StrictMode` ignores adding existing field**
        *   Description and purpose: Ensure that with `StrictMode` disabled, attempting to add an existing field does not return an error and the field definition might be updated (or stay the same based on payload).
        *   Involved components: `NewMigrationApplier` (with `StrictMode: false`), `applyAddField`.
        *   Necessary assertions: No error returned; schema content is updated (if new field definition is different) or remains consistent.
    *   **Test Case: `ApplyMigration` - `StrictMode` prevents removing non-existent field**
        *   Description and purpose: Ensure that with `StrictMode` enabled, attempting to remove a field that does not exist returns an error.
        *   Involved components: `NewMigrationApplier` (with `StrictMode: true`), `applyRemoveField`.
        *   Necessary assertions: `common.SystemError` with `ERR_FIELD_NOT_FOUND` returned; schema remains unchanged.
    *   **Test Case: `ApplyMigration` - `StrictMode` ignores removing non-existent field**
        *   Description and purpose: Ensure that with `StrictMode` disabled, attempting to remove a non-existent field does not return an error and the schema remains unchanged.
        *   Involved components: `NewMigrationApplier` (with `StrictMode: false`), `applyRemoveField`.
        *   Necessary assertions: No error returned; schema remains unchanged.
    *   **Test Case: `ApplyMigration` - Version Mismatch**
        *   Description and purpose: Verify that `ApplyMigration` returns an error if the source schema's version does not match the migration's source version.
        *   Involved components: `validateMigration`.
        *   Necessary assertions: `common.SystemError` with `ERR_MIGRATION_VERSION_MISMATCH` returned.
    *   **Test Case: `ApplyMigration` - Nil Target Version**
        *   Description and purpose: Verify that `ApplyMigration` returns an error if the migration's target version is nil.
        *   Involved components: `validateMigration`.
        *   Necessary assertions: `common.SystemError` with `ERR_MIGRATION_INVALID_TARGET_VERSION` returned.
    *   **Test Case: `ApplyMigration` - Validating Resulting Schema (Indices)**
        *   Description and purpose: Ensure that if `ValidateResult` is true, an error is returned if an index references a non-existent field in the resulting schema.
        *   Involved components: `NewMigrationApplier` (with `ValidateResult: true`), `validateResultingSchema`.
        *   Necessary assertions: `common.SystemError` with `ERR_INDEX_REFERENCES_NON_EXISTENT_FIELD` returned.
    *   **Test Case: `ApplyMigration` - Validating Resulting Schema (Constraints)**
        *   Description and purpose: Ensure that if `ValidateResult` is true, an error is returned if a constraint references a non-existent field in the resulting schema.
        *   Involved components: `NewMigrationApplier` (with `ValidateResult: true`), `validateResultingSchema`.
        *   Necessary assertions: `common.SystemError` with `ERR_CONSTRAINT_REFERENCES_NON_EXISTENT_FIELD` returned.
    *   **Test Case: `applyModifyProperty` - Description Change**
        *   Description and purpose: Verify a schema's description can be modified.
        *   Involved components: `applyModifyProperty`.
        *   Necessary assertions: `target.Description` matches new value.
    *   **Test Case: `applyModifyProperty` - Metadata Change**
        *   Description and purpose: Verify a schema's metadata can be modified.
        *   Involved components: `applyModifyProperty`.
        *   Necessary assertions: `target.Metadata` matches new value.
    *   **Test Case: `applyModifyProperty` - Unknown Property**
        *   Description and purpose: Ensure an error is returned for an attempt to modify an unknown property.
        *   Involved components: `applyModifyProperty`.
        *   Necessary assertions: `common.SystemError` with `ERR_UNKNOWN_PROPERTY` returned.
    *   **Test Case: `applyAddField` - Successful Addition**
        *   Description and purpose: Verify a new field is successfully added to the schema.
        *   Involved components: `applyAddField`, `deepCloneFieldDefinition`.
        *   Necessary assertions: Field exists in `target.Fields` with correct definition.
    *   **Test Case: `applyRemoveField` - Successful Removal**
        *   Description and purpose: Verify a field is successfully removed, and related indexes/constraints are cleaned up.
        *   Involved components: `applyRemoveField`, `filterIndexesByField`, `filterConstraintsByField`.
        *   Necessary assertions: Field is absent from `target.Fields`; indexes/constraints referencing the field are removed.
    *   **Test Case: `applyRemoveField` - Removal of non-existent field (StrictMode disabled)**
        *   Description and purpose: Verify that removing a non-existent field does not error when `StrictMode` is disabled.
        *   Involved components: `applyRemoveField` (StrictMode false).
        *   Necessary assertions: No error. Schema remains unchanged.
    *   **Test Case: `applyModifyField` - Successful Modification**
        *   Description and purpose: Verify an existing field's properties are correctly updated using `applyPartialFieldChanges`.
        *   Involved components: `applyModifyField`, `applyPartialFieldChanges`.
        *   Necessary assertions: Field properties in `target.Fields` reflect partial changes.
    *   **Test Case: `applyAddIndex` - Successful Addition**
        *   Description and purpose: Verify a new index is successfully added.
        *   Involved components: `applyAddIndex`, `deepCloneIndexDefinition`.
        *   Necessary assertions: Index exists in `target.Indexes`.
    *   **Test Case: `applyAddIndex` - Add existing index (StrictMode disabled)**
        *   Description and purpose: Verify that adding an existing index replaces it when `StrictMode` is disabled.
        *   Involved components: `applyAddIndex` (StrictMode false), `removeIndexByName`.
        *   Necessary assertions: No error. Old index is replaced by the new one.
    *   **Test Case: `applyRemoveIndex` - Successful Removal**
        *   Description and purpose: Verify an index is successfully removed.
        *   Involved components: `applyRemoveIndex`, `removeIndexByName`, `removeIndexFromList`.
        *   Necessary assertions: Index is absent from `target.Indexes`.
    *   **Test Case: `applyModifyIndex` - Successful Modification**
        *   Description and purpose: Verify an existing index's properties are correctly updated using `applyPartialIndexChanges`.
        *   Involved components: `applyModifyIndex`, `applyPartialIndexChanges`.
        *   Necessary assertions: Index properties in `target.Indexes` reflect partial changes.
    *   **Test Case: `applyAddConstraint` - Successful Addition**
        *   Description and purpose: Verify a new constraint is successfully added.
        *   Involved components: `applyAddConstraint`, `deepCloneConstraintRule`.
        *   Necessary assertions: Constraint exists in `target.Constraints`.
    *   **Test Case: `applyAddConstraint` - Add existing constraint (StrictMode disabled)**
        *   Description and purpose: Verify that adding an existing constraint replaces it when `StrictMode` is disabled.
        *   Involved components: `applyAddConstraint` (StrictMode false), `removeConstraintByName`.
        *   Necessary assertions: No error. Old constraint is replaced by the new one.
    *   **Test Case: `applyRemoveConstraint` - Successful Removal (Simple Name)**
        *   Description and purpose: Verify a simple constraint is successfully removed.
        *   Involved components: `applyRemoveConstraint`, `removeConstraintByName`, `removeConstraintByNameParts`.
        *   Necessary assertions: Constraint is absent from `target.Constraints`.
    *   **Test Case: `applyRemoveConstraint` - Successful Removal (Hierarchical Name)**
        *   Description and purpose: Verify a nested constraint within a group is successfully removed using a hierarchical name.
        *   Involved components: `applyRemoveConstraint`, `removeConstraintByName`, `removeConstraintByNameParts`.
        *   Necessary assertions: Nested constraint is absent from `target.Constraints`'s groups.
    *   **Test Case: `applyModifyConstraint` - Successful Modification (Simple Constraint)**
        *   Description and purpose: Verify an existing simple constraint's properties are correctly updated.
        *   Involved components: `applyModifyConstraint`, `findConstraintByNameParts`, `applyPartialConstraintChanges`.
        *   Necessary assertions: Constraint properties in `target.Constraints` reflect partial changes.
    *   **Test Case: `applyModifyConstraint` - Successful Modification (Constraint Group Operator)**
        *   Description and purpose: Verify an existing constraint group's operator is correctly updated.
        *   Involved components: `applyModifyConstraint`, `findConstraintByNameParts`, `applyPartialConstraintChanges`.
        *   Necessary assertions: Constraint group operator reflects partial changes.
    *   **Test Case: `applyModifyConstraint` - Constraint Not Found**
        *   Description and purpose: Verify an error is returned if attempting to modify a non-existent constraint.
        *   Involved components: `applyModifyConstraint`.
        *   Necessary assertions: `common.SystemError` with `ERR_CONSTRAINT_NOT_FOUND` returned.
    *   **Test Case: `applyAddSchema` - Successful Addition**
        *   Description and purpose: Verify a new nested schema is successfully added.
        *   Involved components: `applyAddSchema`, `deepCloneNestedSchemaDefinition`.
        *   Necessary assertions: Nested schema exists in `target.NestedSchemas`.
    *   **Test Case: `applyRemoveSchema` - Successful Removal**
        *   Description and purpose: Verify a nested schema is successfully removed.
        *   Involved components: `applyRemoveSchema`.
        *   Necessary assertions: Nested schema is absent from `target.NestedSchemas`.
    *   **Test Case: `applyModifySchema` - Successful Modification of Nested Schema**
        *   Description and purpose: Verify changes to a nested schema are correctly applied.
        *   Involved components: `applyModifySchema`, `nestedSchemaToTempSchema`, `tempSchemaToNestedSchema`, `applyChange` (recursively).
        *   Necessary assertions: Nested schema's properties reflect the applied changes.
    *   **Test Case: `applyModifySchemaReference` - Successful Modification of Field's Schema Reference**
        *   Description and purpose: Verify changes to a field's `NestedSchemaReference` (e.g., indexes or constraints within it) are correctly applied.
        *   Involved components: `applyModifySchemaReference`, `applyChange` (recursively).
        *   Necessary assertions: Field's `Schema` (NestedSchemaReference) reflects the applied changes to its indexes/constraints.
    *   **Test Case: `applyModifySchemaReference` - Field Not Found**
        *   Description and purpose: Verify an error is returned if the target field for schema reference modification is not found.
        *   Involved components: `applyModifySchemaReference`.
        *   Necessary assertions: `common.NewSystemError("ERR_FIELD_NOT_FOUND")` returned.
    *   **Test Case: `applyModifySchemaReference` - Invalid Schema Reference Type**
        *   Description and purpose: Verify an error is returned if the field's schema is not a `NestedSchemaReference`.
        *   Involved components: `applyModifySchemaReference`.
        *   Necessary assertions: `common.NewSystemError("ERR_INVALID_SCHEMA_REFERENCE_TYPE")` returned.
    *   **Test Case: `deepCloneSchema` - Error on Marshal**
        *   Description and purpose: Verify error handling during JSON marshalling for cloning.
        *   Involved components: `deepCloneSchema`.
        *   Necessary assertions: `common.NewSystemError("ERR_SCHEMA_MARSHAL")` returned.
    *   **Test Case: `filterIndexesByField` - No matching indexes**
        *   Description and purpose: Ensure filtering by field name returns original indexes if no match.
        *   Involved components: `filterIndexesByField`.
        *   Necessary assertions: Returns original slice unmodified.
    *   **Test Case: `filterConstraintsByField` - No matching constraints**
        *   Description and purpose: Ensure filtering by field name returns original constraints if no match.
        *   Involved components: `filterConstraintsByField`.
        *   Necessary assertions: Returns original slice unmodified.
    *   **Test Case: `validateConstraintFields` - No fields to validate**
        *   Description and purpose: Ensure validation passes if there are no constraints or fields.
        *   Involved components: `validateConstraintFields`.
        *   Necessary assertions: No error.
    *   **Test Case: `convertToStringPtr` - Various types**
        *   Description and purpose: Test the helper function `convertToStringPtr` with `nil`, `*string`, `string` and other types.
        *   Involved components: `convertToStringPtr`.
        *   Necessary assertions: Returns `nil` or correct `*string` value.

-   **Test Suite: `DefaultMigrationEngine` (`engine.go`)**
    *   **Description:** Tests the core migration engine's ability to diff schemas, apply migrations, patch schemas with JSON patches, and plan migration strategies.
    *   **Test Case: `Diff` - Successful Generation**
        *   Description and purpose: Verify `Diff` correctly generates a migration object when changes exist.
        *   Involved components: `NewDefaultMigrationEngine`, `Diff`, `MigrationGenerator`.
        *   Necessary assertions: No error; returned `migration` is not nil and contains expected changes and version info.
    *   **Test Case: `Diff` - No Changes**
        *   Description and purpose: Verify `Diff` returns nil migration when no structural changes are detected.
        *   Involved components: `NewDefaultMigrationEngine`, `Diff`.
        *   Necessary assertions: No error; returned `migration` is nil.
    *   **Test Case: `Apply` - Successful Application**
        *   Description and purpose: Verify `Apply` successfully projects a migration onto a base schema.
        *   Involved components: `NewDefaultMigrationEngine`, `Apply`, `MigrationApplier`.
        *   Necessary assertions: No error; returned schema reflects the applied migration.
    *   **Test Case: `Apply` - Error Propagation**
        *   Description and purpose: Verify `Apply` propagates errors from the underlying `MigrationApplier`.
        *   Involved components: `Apply`.
        *   Necessary assertions: Error returned is a `common.SystemError`.
    *   **Test Case: `Patch` - Successful Application of JSON Patches**
        *   Description and purpose: Verify `Patch` applies valid JSON Patches to a schema definition.
        *   Involved components: `NewDefaultMigrationEngine`, `Patch`, `scjson.NewPatcher`, `cleanupEmptyCollections`.
        *   Necessary assertions: No error; returned schema reflects the JSON patch operations.
    *   **Test Case: `Patch` - Invalid JSON Patches**
        *   Description and purpose: Verify `Patch` returns an error for invalid JSON Patch operations.
        *   Involved components: `Patch`.
        *   Necessary assertions: `common.SystemError` with `ERR_JSON_PATCH_APPLY` returned.
    *   **Test Case: `Patch` - `cleanupEmptyCollections` effect**
        *   Description and purpose: Verify that `cleanupEmptyCollections` correctly removes empty slices/maps to match `omitempty`.
        *   Involved components: `Patch`, `cleanupEmptyCollections`.
        *   Necessary assertions: Patched schema (when marshalled back to JSON) does not contain empty arrays/maps for fields with `omitempty` tag.
    *   **Test Case: `Transform` - Not Implemented Error**
        *   Description and purpose: Verify `Transform` correctly returns the "not implemented" error.
        *   Involved components: `Transform`.
        *   Necessary assertions: `common.SystemError` with `ERR_NOT_IMPLEMENTED` returned.
    *   **Test Case: `Plan` - `InPlace` Strategy**
        *   Description and purpose: Verify `Plan` correctly determines `MigrationStrategyInPlace` for safe changes.
        *   Involved components: `NewDefaultMigrationEngine`, `Plan`, `determineStrategy`.
        *   Necessary assertions: `plan.Strategy` is `MigrationStrategyInPlace`; `plan.Changes` contains only in-place executable changes.
    *   **Test Case: `Plan` - `BlueGreen` Strategy**
        *   Description and purpose: Verify `Plan` correctly determines `MigrationStrategyBlueGreen` for destructive changes.
        *   Involved components: `NewDefaultMigrationEngine`, `Plan`, `determineStrategy`.
        *   Necessary assertions: `plan.Strategy` is `MigrationStrategyBlueGreen`; `plan.Changes` is empty (as all changes require blue-green).
    *   **Test Case: `Plan` - Nil Source Schema Input**
        *   Description and purpose: Verify `Plan` returns an error if `sourceSchema` is nil.
        *   Involved components: `Plan`, `validatePlanInputs`.
        *   Necessary assertions: `common.SystemError` with `ERR_PLAN_NIL_SOURCE_SCHEMA` returned.
    *   **Test Case: `isAddFieldDestructive` - Required Field without Default**
        *   Description and purpose: Verify adding a required field without a default value is considered destructive.
        *   Involved components: `isAddFieldDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isAddFieldDestructive` - Required Field with Default**
        *   Description and purpose: Verify adding a required field with a default value is not destructive.
        *   Involved components: `isAddFieldDestructive`.
        *   Necessary assertions: Returns `false`.
    *   **Test Case: `isModifyFieldDestructive` - Type Change**
        *   Description and purpose: Verify changing a field's type is destructive.
        *   Involved components: `isModifyFieldDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyFieldDestructive` - Renaming Field**
        *   Description and purpose: Verify renaming a field is destructive.
        *   Involved components: `isModifyFieldDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyFieldDestructive` - Making Field Required without Default**
        *   Description and purpose: Verify making an optional field required without providing a default is destructive.
        *   Involved components: `isModifyFieldDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyFieldDestructive` - Changing ItemsType**
        *   Description and purpose: Verify changing the `ItemsType` of an array/set field is destructive.
        *   Involved components: `isModifyFieldDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyFieldDestructive` - Removing Enum Values**
        *   Description and purpose: Verify removing existing enum values from a field is destructive.
        *   Involved components: `isModifyFieldDestructive`, `hasRemovedEnumValues`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyIndexDestructive` - Changing to Primary Key Type**
        *   Description and purpose: Verify changing an index to a primary key type is destructive.
        *   Involved components: `isModifyIndexDestructive`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifyIndexDestructive` - Modifying Primary Key Fields**
        *   Description and purpose: Verify modifying the fields of a primary key index is destructive.
        *   Involved components: `isModifyIndexDestructive`, `isPrimaryKeyName`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isModifySchemaDestructive` - Nested Destructive Change**
        *   Description and purpose: Verify that a destructive change within a nested schema causes the parent `ModifySchema` change to be considered destructive.
        *   Involved components: `isModifySchemaDestructive` (recursive call to `requiresBlueGreen`).
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isSchemaSafeChange` - NestedSchemaReference with identical content**
        *   Description and purpose: Verify `compareNestedSchemaRef` returns true for structurally identical references.
        *   Involved components: `isSchemaSafeChange`, `compareNestedSchemaRef`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isSchemaSafeChange` - NestedSchemaReference with different content**
        *   Description and purpose: Verify `compareNestedSchemaRef` returns false for structurally different references (e.g., different indexes).
        *   Involved components: `isSchemaSafeChange`, `compareNestedSchemaRef`.
        *   Necessary assertions: Returns `false`.
    *   **Test Case: `hasRemovedEnumValues` - No Values Removed**
        *   Description and purpose: Verify `hasRemovedEnumValues` returns false when no enum values have been removed.
        *   Involved components: `hasRemovedEnumValues`.
        *   Necessary assertions: Returns `false`.
    *   **Test Case: `hasRemovedEnumValues` - Values Removed**
        *   Description and purpose: Verify `hasRemovedEnumValues` returns true when enum values have been removed.
        *   Involved components: `hasRemovedEnumValues`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `filterInPlaceChanges` - Mixed Changes**
        *   Description and purpose: Verify `filterInPlaceChanges` correctly separates in-place from blue-green changes.
        *   Involved components: `filterInPlaceChanges`, `requiresBlueGreen`, `isDriverActionRequired`.
        *   Necessary assertions: Returned slice contains only expected in-place changes.
    *   **Test Case: `isDriverActionRequired` - Index Operation**
        *   Description and purpose: Verify index operations (`AddIndex`, `RemoveIndex`, `ModifyIndex`) are marked as driver actions.
        *   Involved components: `isDriverActionRequired`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isDriverActionRequired` - Field Operation**
        *   Description and purpose: Verify field operations (`AddField`, `ModifyField`) are marked as driver actions.
        *   Involved components: `isDriverActionRequired`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `isDriverActionRequired` - Constraint Operation**
        *   Description and purpose: Verify constraint operations (`AddConstraint`, `RemoveConstraint`, `ModifyConstraint`) are *not* marked as driver actions (as they are WASM-enforced).
        *   Involved components: `isDriverActionRequired`.
        *   Necessary assertions: Returns `false`.
    *   **Test Case: `cleanupEmptyCollections` - Removes empty slice/map fields**
        *   Description and purpose: Verify `cleanupEmptyCollections` removes fields like `indexes`, `nestedSchemas`, `constraints` if their values are empty slices or maps.
        *   Involved components: `cleanupEmptyCollections`.
        *   Necessary assertions: Input map has specified keys removed if empty.
    *   **Test Case: `isPrimaryKeyName` - Standard Primary Key Names**
        *   Description and purpose: Verify common primary key names are recognized.
        *   Involved components: `isPrimaryKeyName`.
        *   Necessary assertions: Returns `true` for "primary", "pk", "id_primary_key", etc.
    *   **Test Case: `isPrimaryKeyName` - Non-Primary Key Names**
        *   Description and purpose: Verify non-primary key names are not recognized.
        *   Involved components: `isPrimaryKeyName`.
        *   Necessary assertions: Returns `false` for "id", "user_id", etc.

-   **Test Suite: `MigrationGenerator` (`migrator.go`)**
    *   **Description:** Tests the `MigrationGenerator`'s ability to create accurate schema migrations between two `SchemaDefinition`s, including rollback generation and checksum computation.
    *   **Test Case: `Generate` - No Changes Detected**
        *   Description and purpose: Verify that `Generate` returns a `nil` migration when `oldSchema` and `newSchema` are identical.
        *   Involved components: `NewMigrationGenerator`, `Generate`, `collectAllChanges`.
        *   Necessary assertions: No error returned; `migration` is `nil`.
    *   **Test Case: `Generate` - Single Field Added**
        *   Description and purpose: Verify `Generate` creates a migration with a single `AddField` change.
        *   Involved components: `Generate`, `compareFields`.
        *   Necessary assertions: `migration.Changes` contains one `AddField` change; `migration.Version.Target` is a minor bump.
    *   **Test Case: `Generate` - Single Field Removed**
        *   Description and purpose: Verify `Generate` creates a migration with a single `RemoveField` change.
        *   Involved components: `Generate`, `compareFields`.
        *   Necessary assertions: `migration.Changes` contains one `RemoveField` change; `migration.Version.Target` is a major bump.
    *   **Test Case: `Generate` - Field Modified**
        *   Description and purpose: Verify `Generate` creates a migration with a `ModifyField` change.
        *   Involved components: `Generate`, `compareFields`, `processFieldModification`.
        *   Necessary assertions: `migration.Changes` contains one `ModifyField` change with correct partial updates.
    *   **Test Case: `Generate` - Schema Name Mismatch**
        *   Description and purpose: Verify `validateSchemas` returns an error if schema names differ.
        *   Involved components: `Generate`, `validateSchemas`.
        *   Necessary assertions: `common.SystemError` with `ERR_SCHEMA_NAME_MISMATCH` returned.
    *   **Test Case: `Generate` - Rollback Generated for AddField**
        *   Description and purpose: Verify a rollback migration is generated for an `AddField` operation.
        *   Involved components: `NewMigrationGenerator` (with `GenerateRollback: true`), `Generate`, `rollbackAddField`.
        *   Necessary assertions: `migration.Rollback` contains a `RemoveField` change for the added field.
    *   **Test Case: `Generate` - Rollback Generated for RemoveField**
        *   Description and purpose: Verify a rollback migration is generated for a `RemoveField` operation.
        *   Involved components: `NewMigrationGenerator` (with `GenerateRollback: true`), `Generate`, `rollbackRemoveField`.
        *   Necessary assertions: `migration.Rollback` contains an `AddField` change restoring the removed field.
    *   **Test Case: `Generate` - Checksum Generated**
        *   Description and purpose: Verify a checksum is generated and attached to the migration.
        *   Involved components: `Generate`, `addChecksum`, `generateChecksum`.
        *   Necessary assertions: `migration.Checksum` is a non-empty string.
    *   **Test Case: `compareProperties` - Description Change**
        *   Description and purpose: Verify `compareProperties` detects description changes.
        *   Involved components: `compareProperties`.
        *   Necessary assertions: Returns a `ModifyProperty` change for "description".
    *   **Test Case: `compareProperties` - Metadata Change**
        *   Description and purpose: Verify `compareProperties` detects metadata changes.
        *   Involved components: `compareProperties`.
        *   Necessary assertions: Returns a `ModifyProperty` change for "metadata".
    *   **Test Case: `compareProperties` - Ignore Metadata**
        *   Description and purpose: Verify `compareProperties` ignores metadata changes when `IgnoreMetadata` is true.
        *   Involved components: `NewMigrationGenerator` (with `IgnoreMetadata: true`), `compareProperties`.
        *   Necessary assertions: No `ModifyProperty` change for "metadata" is returned.
    *   **Test Case: `compareFieldDefinitions` - Type Change**
        *   Description and purpose: Verify a `PartialFieldDefinition` is generated with `Type` change.
        *   Involved components: `compareFieldDefinitions`.
        *   Necessary assertions: `partial.Type` is set.
    *   **Test Case: `compareFieldDefinitions` - Required Added**
        *   Description and purpose: Verify `PartialFieldDefinition` for `Required` becoming true.
        *   Involved components: `compareFieldDefinitions`.
        *   Necessary assertions: `partial.Required` is set to true.
    *   **Test Case: `compareFieldDefinitions` - Required Removed (Unset)**
        *   Description and purpose: Verify `PartialFieldDefinition` for `Required` becoming false/nil (via `Unset`).
        *   Involved components: `compareFieldDefinitions`.
        *   Necessary assertions: `partial.Unset` contains "required".
    *   **Test Case: `compareFieldDefinitions` - Schema Reference Change**
        *   Description and purpose: Verify changes within a `NestedSchemaReference` on a field are detected and wrapped in `ModifySchemaReference`.
        *   Involved components: `compareFieldSchemaProperty`, `compareSchemaReferenceChanges`.
        *   Necessary assertions: Returns a `ModifySchemaReference` change.
    *   **Test Case: `compareIndexDefinitions` - Unique Status Change**
        *   Description and purpose: Verify a `PartialIndexDefinition` is generated for `Unique` status change.
        *   Involved components: `compareIndexDefinitions`.
        *   Necessary assertions: `partial.Unique` is set.
    *   **Test Case: `compareIndexDefinitions` - Fields Change**
        *   Description and purpose: Verify a `PartialIndexDefinition` is generated for `Fields` change.
        *   Involved components: `compareIndexDefinitions`.
        *   Necessary assertions: `partial.Fields` is set.
    *   **Test Case: `compareConstraints` - Simple Constraint Added**
        *   Description and purpose: Verify adding a new simple constraint.
        *   Involved components: `compareConstraints`.
        *   Necessary assertions: Returns an `AddConstraint` change.
    *   **Test Case: `compareConstraints` - Simple Constraint Removed**
        *   Description and purpose: Verify removing a simple constraint.
        *   Involved components: `compareConstraints`.
        *   Necessary assertions: Returns a `RemoveConstraint` change.
    *   **Test Case: `compareConstraints` - Constraint Modified**
        *   Description and purpose: Verify modifying a simple constraint.
        *   Involved components: `compareConstraints`, `compareConstraintDefinitions`.
        *   Necessary assertions: Returns a `ModifyConstraint` change.
    *   **Test Case: `compareConstraintGroupChanges` - Operator Change**
        *   Description and purpose: Verify operator change in a constraint group is detected.
        *   Involved components: `compareConstraintGroupChanges`.
        *   Necessary assertions: Returns a `ModifyConstraint` change for the operator.
    *   **Test Case: `compareConstraintGroupRules` - Nested Rule Added (Hierarchical Name)**
        *   Description and purpose: Verify adding a rule within a constraint group using a hierarchical name.
        *   Involved components: `compareConstraintGroupRules`.
        *   Necessary assertions: Returns an `AddConstraint` change with the correct hierarchical `Name`.
    *   **Test Case: `compareNested` - Nested Schema Added**
        *   Description and purpose: Verify adding a new nested schema.
        *   Involved components: `compareNested`.
        *   Necessary assertions: Returns an `AddSchema` change.
    *   **Test Case: `compareNested` - Nested Schema Removed**
        *   Description and purpose: Verify removing a nested schema.
        *   Involved components: `compareNested`.
        *   Necessary assertions: Returns a `RemoveSchema` change.
    *   **Test Case: `compareNested` - Nested Schema Modified (Fields)**
        *   Description and purpose: Verify changes within a nested schema (e.g., field additions) are captured.
        *   Involved components: `compareNested`, `compareNestedSchemas`, `compareFields`.
        *   Necessary assertions: Returns a `ModifySchema` change containing nested field changes.
    *   **Test Case: `GenerateMigrationSequence` - Valid Sequence**
        *   Description and purpose: Verify that `GenerateMigrationSequence` correctly creates a series of migrations from multiple schema versions.
        *   Involved components: `GenerateMigrationSequence`, `sortSchemasByVersion`.
        *   Necessary assertions: Returns a slice of migrations matching the differences between sorted schemas.
    *   **Test Case: `GenerateMigrationSequence` - Insufficient Schemas**
        *   Description and purpose: Verify `GenerateMigrationSequence` returns an error if fewer than 2 schemas are provided.
        *   Involved components: `GenerateMigrationSequence`.
        *   Necessary assertions: `common.SystemError` with `ERR_INSUFFICIENT_SCHEMAS` returned.
    *   **Test Case: `rollbackModifyField` - Full Reversal**
        *   Description and purpose: Verify `rollbackModifyField` generates changes to revert all partial modifications.
        *   Involved components: `rollbackModifyField`, `compareFieldDefinitions`.
        *   Necessary assertions: `rollback.SchemaChangeModifyFieldPayload.Changes` correctly reverses old field to new field.
    *   **Test Case: `buildConstraintMap` - Mixed Constraints and Groups**
        *   Description and purpose: Verify `buildConstraintMap` correctly maps both simple constraints and constraint groups by name.
        *   Involved components: `buildConstraintMap`.
        *   Necessary assertions: Map contains entries for both constraint types.
    *   **Test Case: `stringSliceEqual` - Equal Slices**
        *   Description and purpose: Verify helper correctly identifies equal string slices.
        *   Involved components: `stringSliceEqual`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `stringSliceEqual` - Different Order**
        *   Description and purpose: Verify helper correctly identifies different order in string slices as unequal.
        *   Involved components: `stringSliceEqual`.
        *   Necessary assertions: Returns `false`.
    *   **Test Case: `stringPtrEqual` - Both Nil**
        *   Description and purpose: Verify helper returns true when both string pointers are nil.
        *   Involved components: `stringPtrEqual`.
        *   Necessary assertions: Returns `true`.

-   **Test Suite: `PatchConverter` (`patch.go`)**
    *   **Description:** Tests the `PatchConverter`'s ability to accurately translate `schema.SchemaChange` objects into RFC6902 JSON Patch operations.
    *   **Test Case: `Convert` - Unknown Change Type**
        *   Description and purpose: Verify `Convert` returns an error for an unrecognized `SchemaChangeType`.
        *   Involved components: `Convert`.
        *   Necessary assertions: `common.SystemError` with `ERR_CREATE_MIGRATION_PATCH` returned.
    *   **Test Case: `convertModifyProperty` - Replace Description**
        *   Description and purpose: Verify `ModifyProperty` for description creates a `replace` patch.
        *   Involved components: `convertModifyProperty`.
        *   Necessary assertions: Patch `Op` is "replace", `Path` is "/description", `Value` is new description.
    *   **Test Case: `convertAddField` - Add New Field**
        *   Description and purpose: Verify `AddField` creates an `add` patch to `/fields/{fieldName}`.
        *   Involved components: `convertAddField`.
        *   Necessary assertions: Patch `Op` is "add", `Path` is "/fields/{fieldName}", `Value` is field definition.
    *   **Test Case: `convertRemoveField` - Remove Existing Field**
        *   Description and purpose: Verify `RemoveField` creates a `remove` patch for `/fields/{fieldName}`.
        *   Involved components: `convertRemoveField`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` is "/fields/{fieldName}".
    *   **Test Case: `convertModifyField` - Replace Type**
        *   Description and purpose: Verify `ModifyField` for changing a field's type creates a `replace` patch.
        *   Involved components: `convertModifyField`, `createReplacePatch`.
        *   Necessary assertions: Patch `Op` is "replace", `Path` ends with "/type".
    *   **Test Case: `convertModifyField` - Add Required (previously nil)**
        *   Description and purpose: Verify `ModifyField` for setting `Required` to true (from nil) creates an `add` patch.
        *   Involved components: `convertModifyField`, `handleOptionalFieldProperty`.
        *   Necessary assertions: Patch `Op` is "add", `Path` ends with "/required".
    *   **Test Case: `convertModifyField` - Unset Description (previously existing)**
        *   Description and purpose: Verify `ModifyField` with `Unset: "description"` creates a `remove` patch.
        *   Involved components: `convertModifyField`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` ends with "/description".
    *   **Test Case: `convertModifyField` - Unset Description (previously nil)**
        *   Description and purpose: Verify `ModifyField` with `Unset: "description"` does *not* create a `remove` patch if description was already nil.
        *   Involved components: `convertModifyField`.
        *   Necessary assertions: No `remove` patch for "description" is generated.
    *   **Test Case: `convertAddIndex` - Add First Index**
        *   Description and purpose: Verify `AddIndex` correctly creates an `add` patch for `/indexes` array initialization and then for `/indexes/-`.
        *   Involved components: `convertAddIndex`.
        *   Necessary assertions: Two patches: first `add` to `/indexes` with empty array, second `add` to `/indexes/-` with index definition.
    *   **Test Case: `convertAddIndex` - Add Subsequent Index**
        *   Description and purpose: Verify `AddIndex` creates a single `add` patch to `/indexes/-` when the array already exists.
        *   Involved components: `convertAddIndex`.
        *   Necessary assertions: One patch: `add` to `/indexes/-`.
    *   **Test Case: `convertRemoveIndex` - Remove Existing Index**
        *   Description and purpose: Verify `RemoveIndex` creates a `remove` patch for `/indexes/{index}`.
        *   Involved components: `convertRemoveIndex`, `findIndexByName`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` is "/indexes/{idx}".
    *   **Test Case: `convertRemoveIndex` - Index Not Found**
        *   Description and purpose: Verify `RemoveIndex` returns an error if index is not found.
        *   Involved components: `convertRemoveIndex`.
        *   Necessary assertions: `common.SystemError` with `ERR_CREATE_MIGRATION_PATCH` (index not found) returned.
    *   **Test Case: `convertModifyIndex` - Replace Index Type**
        *   Description and purpose: Verify `ModifyIndex` for changing index type creates a `replace` patch.
        *   Involved components: `convertModifyIndex`.
        *   Necessary assertions: Patch `Op` is "replace", `Path` ends with "/type".
    *   **Test Case: `convertModifyIndex` - Unset Index Fields**
        *   Description and purpose: Verify `ModifyIndex` with `Unset: "fields"` creates a `remove` patch.
        *   Involved components: `convertModifyIndex`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` ends with "/fields".
    *   **Test Case: `convertAddConstraint` - Add First Constraint**
        *   Description and purpose: Verify `AddConstraint` creates `add` patches for `/constraints` array initialization and then `/constraints/-`.
        *   Involved components: `convertAddConstraint`.
        *   Necessary assertions: Two patches: first `add` to `/constraints` with empty array, second `add` to `/constraints/-` with constraint.
    *   **Test Case: `convertRemoveConstraint` - Remove Simple Constraint**
        *   Description and purpose: Verify `RemoveConstraint` for a simple constraint creates a `remove` patch.
        *   Involved components: `convertRemoveConstraint`, `findConstraintPath`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` is "/constraints/{idx}".
    *   **Test Case: `convertRemoveConstraint` - Remove Hierarchical Constraint**
        *   Description and purpose: Verify `RemoveConstraint` for a nested constraint uses the correct hierarchical path.
        *   Involved components: `convertRemoveConstraint`, `findConstraintPath`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` is "/constraints/{groupIdx}/rules/{constraintIdx}".
    *   **Test Case: `convertModifyConstraint` - Replace Predicate**
        *   Description and purpose: Verify `ModifyConstraint` for changing predicate creates a `replace` patch.
        *   Involved components: `convertModifyConstraint`.
        *   Necessary assertions: Patch `Op` is "replace", `Path` ends with "/predicate".
    *   **Test Case: `convertModifyConstraint` - Unset Field**
        *   Description and purpose: Verify `ModifyConstraint` with `Unset: "field"` creates a `remove` patch.
        *   Involved components: `convertModifyConstraint`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` ends with "/field".
    *   **Test Case: `convertModifyConstraint` - Replace Constraint Group Operator (after FIX)**
        *   Description and purpose: Verify `ModifyConstraint` correctly replaces the operator of a constraint group (assuming FIX is implemented to handle this type correctly).
        *   Involved components: `convertModifyConstraint`.
        *   Necessary assertions: Patch `Op` is "replace", `Path` ends with "/operator".
    *   **Test Case: `convertAddSchema` - Add New Nested Schema**
        *   Description and purpose: Verify `AddSchema` creates an `add` patch for `/nestedSchemas/{id}`.
        *   Involved components: `convertAddSchema`.
        *   Necessary assertions: Patch `Op` is "add", `Path` is "/nestedSchemas/{id}".
    *   **Test Case: `convertRemoveSchema` - Remove Existing Nested Schema**
        *   Description and purpose: Verify `RemoveSchema` creates a `remove` patch for `/nestedSchemas/{id}`.
        *   Involved components: `convertRemoveSchema`.
        *   Necessary assertions: Patch `Op` is "remove", `Path` is "/nestedSchemas/{id}".
    *   **Test Case: `convertModifySchema` - Modify Field in Nested Schema**
        *   Description and purpose: Verify changes to a nested schema's field are correctly converted and paths are prefixed.
        *   Involved components: `convertModifySchema`, recursive `Convert`.
        *   Necessary assertions: Patches have `Path` prefixed with `/nestedSchemas/{id}/fields/{fieldName}`.
    *   **Test Case: `convertModifySchemaReference` - Modify Index in Field's Schema Reference**
        *   Description and purpose: Verify changes to an index within a field's `NestedSchemaReference` are converted and paths are prefixed.
        *   Involved components: `convertModifySchemaReference`, recursive `Convert`.
        *   Necessary assertions: Patches have `Path` prefixed with `/fields/{fieldName}/schema/indexes/{idx}`.
    *   **Test Case: `handleOptionalFieldProperty` - Add (Old nil, New non-nil)**
        *   Description and purpose: Verify an `add` patch is generated when an optional field becomes non-nil.
        *   Involved components: `handleOptionalFieldProperty`, `isNilOrEmpty`, `dereferenceValue`.
        *   Necessary assertions: Returns a slice with one `add` patch.
    *   **Test Case: `handleOptionalFieldProperty` - Replace (Old non-nil, New non-nil)**
        *   Description and purpose: Verify a `replace` patch is generated when an optional field changes value.
        *   Involved components: `handleOptionalFieldProperty`.
        *   Necessary assertions: Returns a slice with one `replace` patch.
    *   **Test Case: `handleOptionalFieldProperty` - No Change (Both nil)**
        *   Description and purpose: Verify no patch is generated when both old and new values are nil.
        *   Involved components: `handleOptionalFieldProperty`.
        *   Necessary assertions: Returns an empty slice.
    *   **Test Case: `findConstraintPath` - Simple Constraint**
        *   Description and purpose: Verify path for a top-level simple constraint is found.
        *   Involved components: `findConstraintPath`, `findConstraintPathRecursive`.
        *   Necessary assertions: Returns correct path like "/constraints/0".
    *   **Test Case: `findConstraintPath` - Nested Constraint**
        *   Description and purpose: Verify path for a constraint within a group is found using hierarchical name.
        *   Involved components: `findConstraintPath`, `findConstraintPathRecursive`.
        *   Necessary assertions: Returns correct path like "/constraints/0/rules/1".
    *   **Test Case: `parseHierarchicalName` - Single Part Name**
        *   Description and purpose: Verify single part name is parsed correctly.
        *   Involved components: `parseHierarchicalName`.
        *   Necessary assertions: Returns a slice with one element.
    *   **Test Case: `parseHierarchicalName` - Multi-Part Name**
        *   Description and purpose: Verify multi-part name is parsed correctly.
        *   Involved components: `parseHierarchicalName`.
        *   Necessary assertions: Returns a slice with multiple elements.

-   **Test Suite: `VersioningUtil` (`version.go`)**
    *   **Description:** Tests `VersioningUtil`'s logic for calculating the next semantic version based on the impact of schema changes.
    *   **Test Case: `CalculateNextVersion` - No Changes Provided**
        *   Description and purpose: Verify that an error is returned if `CalculateNextVersion` is called with an empty list of changes.
        *   Involved components: `NewVersioningUtil`, `CalculateNextVersion`.
        *   Necessary assertions: `common.SystemError` with `ERR_NO_CHANGES` returned.
    *   **Test Case: `CalculateNextVersion` - Invalid Current Version**
        *   Description and purpose: Verify an error is returned for an invalid current version string.
        *   Involved components: `CalculateNextVersion`.
        *   Necessary assertions: `common.SystemError` returned.
    *   **Test Case: `CalculateNextVersion` - Highest Impact is PATCH**
        *   Description and purpose: Verify the version is correctly bumped to a PATCH version when only patch-level changes are present.
        *   Involved components: `CalculateNextVersion`, `calculateEffectiveVersion`.
        *   Necessary assertions: Returns `X.Y.Z+1` format.
    *   **Test Case: `CalculateNextVersion` - Highest Impact is MINOR**
        *   Description and purpose: Verify the version is correctly bumped to a MINOR version when minor-level changes are present (and no major).
        *   Involved components: `CalculateNextVersion`, `calculateEffectiveVersion`.
        *   Necessary assertions: Returns `X.Y+1.0` format.
    *   **Test Case: `CalculateNextVersion` - Highest Impact is MAJOR**
        *   Description and purpose: Verify the version is correctly bumped to a MAJOR version when major-level changes are present.
        *   Involved components: `CalculateNextVersion`, `calculateEffectiveVersion`.
        *   Necessary assertions: Returns `X+1.0.0` format.
    *   **Test Case: `calculateEffectiveVersion` - Early Exit on Major**
        *   Description and purpose: Verify `calculateEffectiveVersion` returns "major" immediately if a major change is encountered early in the list.
        *   Involved components: `calculateEffectiveVersion`, `getChangeImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `getChangeImpact` - `RemoveField` (MAJOR)**
        *   Description and purpose: Verify `RemoveField` is classified as "major".
        *   Involved components: `getChangeImpact`, `handleRemoveField`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `getChangeImpact` - `AddField` Required without Default (MAJOR)**
        *   Description and purpose: Verify `AddField` for a required field without a default is "major".
        *   Involved components: `getChangeImpact`, `handleAddField`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `getChangeImpact` - `AddField` Optional or with Default (MINOR)**
        *   Description and purpose: Verify `AddField` for an optional field or a required field with default is "minor".
        *   Involved components: `getChangeImpact`, `handleAddField`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `getChangeImpact` - `AddConstraint` (MAJOR)**
        *   Description and purpose: Verify `AddConstraint` is "major".
        *   Involved components: `getChangeImpact`, `handleAddConstraint`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `getChangeImpact` - `RemoveConstraint` (MINOR)**
        *   Description and purpose: Verify `RemoveConstraint` is "minor".
        *   Involved components: `getChangeImpact`, `handleRemoveConstraint`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `getChangeImpact` - `AddIndex` Unique (MAJOR)**
        *   Description and purpose: Verify `AddIndex` for a unique index is "major".
        *   Involved components: `getChangeImpact`, `handleAddIndex`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `getChangeImpact` - `AddIndex` Non-Unique (PATCH)**
        *   Description and purpose: Verify `AddIndex` for a non-unique index is "patch".
        *   Involved components: `getChangeImpact`, `handleAddIndex`.
        *   Necessary assertions: Returns "patch".
    *   **Test Case: `getChangeImpact` - `ModifyProperty` (PATCH)**
        *   Description and purpose: Verify `ModifyProperty` is "patch".
        *   Involved components: `getChangeImpact`, `handleModifyProperty`.
        *   Necessary assertions: Returns "patch".
    *   **Test Case: `determineModifyFieldImpact` - Type Change (MAJOR)**
        *   Description and purpose: Verify `Type` change results in "major".
        *   Involved components: `determineModifyFieldImpact`, `checkMajorFieldChanges`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyFieldImpact` - Required Added (MAJOR)**
        *   Description and purpose: Verify `Required` changing from false/nil to true results in "major".
        *   Involved components: `determineModifyFieldImpact`, `checkMajorFieldChanges`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyFieldImpact` - Required Removed (MINOR)**
        *   Description and purpose: Verify `Required` changing from true to false/nil results in "minor".
        *   Involved components: `determineModifyFieldImpact`, `checkMinorFieldChanges`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `determineModifyFieldImpact` - Enum Value Removed (MAJOR)**
        *   Description and purpose: Verify removal of an enum value results in "major".
        *   Involved components: `determineModifyFieldImpact`, `checkMajorFieldChanges`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyFieldImpact` - Enum Value Added (MINOR)**
        *   Description and purpose: Verify addition of an enum value results in "minor".
        *   Involved components: `determineModifyFieldImpact`, `checkMinorFieldChanges`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `determineModifyIndexImpact` - Unique Added (MAJOR)**
        *   Description and purpose: Verify an index becoming unique results in "major".
        *   Involved components: `determineModifyIndexImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyIndexImpact` - Unique Removed (MINOR)**
        *   Description and purpose: Verify an index becoming non-unique results in "minor".
        *   Involved components: `determineModifyIndexImpact`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `determineModifyIndexImpact` - Fields Changed (MAJOR)**
        *   Description and purpose: Verify changing index fields results in "major".
        *   Involved components: `determineModifyIndexImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyConstraintImpact` - Simple Constraint Predicate Change (MAJOR)**
        *   Description and purpose: Verify predicate change in a simple constraint results in "major".
        *   Involved components: `determineModifyConstraintImpact`, `determineSimpleConstraintImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineModifyConstraintImpact` - Group Operator Change (AND -> OR) (MINOR)**
        *   Description and purpose: Verify changing group operator from AND to OR results in "minor".
        *   Involved components: `determineModifyConstraintImpact`, `determineConstraintGroupImpact`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `determineModifyConstraintImpact` - Group Operator Change (OR -> AND) (MAJOR)**
        *   Description and purpose: Verify changing group operator from OR to AND results in "major".
        *   Involved components: `determineModifyConstraintImpact`, `determineConstraintGroupImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineConstraintChangesImpact` - Constraint Added (MAJOR)**
        *   Description and purpose: Verify adding a constraint to a list results in "major".
        *   Involved components: `determineConstraintChangesImpact`.
        *   Necessary assertions: Returns "major".
    *   **Test Case: `determineConstraintChangesImpact` - Constraint Removed (MINOR)**
        *   Description and purpose: Verify removing a constraint from a list results in "minor".
        *   Involved components: `determineConstraintChangesImpact`.
        *   Necessary assertions: Returns "minor".
    *   **Test Case: `determineConstraintChangesImpact` - Constraint Modified (Propagate Impact)**
        *   Description and purpose: Verify modification of a constraint propagates its impact.
        *   Involved components: `determineConstraintChangesImpact`, `compareConstraintRules`.
        *   Necessary assertions: Returns the highest impact from the modification.
    *   **Test Case: `findConstraintRuleByName` - Top-level Simple Constraint**
        *   Description and purpose: Verify a top-level simple constraint is found by name.
        *   Involved components: `findConstraintRuleByName`.
        *   Necessary assertions: Returns the correct `ConstraintRule`.
    *   **Test Case: `findConstraintRuleByName` - Top-level Constraint Group**
        *   Description and purpose: Verify a top-level constraint group is found by name.
        *   Involved components: `findConstraintRuleByName`.
        *   Necessary assertions: Returns the correct `ConstraintRule`.
    *   **Test Case: `findConstraintRuleByName` - Constraint Not Found**
        *   Description and purpose: Verify an error is returned if a constraint is not found.
        *   Involved components: `findConstraintRuleByName`.
        *   Necessary assertions: `common.SystemError` with `ERR_SCHEMA_CONSTRAINT_NOT_FOUND` returned.
    *   **Test Case: `containsString` - String Present**
        *   Description and purpose: Verify `containsString` returns true when the string is in the slice.
        *   Involved components: `containsString`.
        *   Necessary assertions: Returns `true`.
    *   **Test Case: `containsString` - String Absent**
        *   Description and purpose: Verify `containsString` returns false when the string is not in the slice.
        *   Involved components: `containsString`.
        *   Necessary assertions: Returns `false`.
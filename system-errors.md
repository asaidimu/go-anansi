
## ERR_SCHEMA_VALIDATOR_SCHEMA_VALIDATION_FAILED
This error indicates that the schema provided to the document validator failed its own internal validation checks during the validator's creation. This means the schema itself is malformed or semantically incorrect.

**Error Chain**: `core.schema` -> (underlying schema validation error)

**Packages**: `core/schema`
**Methods**: `schema.NewDocumentValidator`
**Severity**: High (indicates a critical issue with the schema definition)

---

## ERR_SCHEMA_VALIDATOR_CIRCULAR_DEPENDENCY
This error occurs when a circular dependency is detected within the validation graph constructed from the schema. Circular dependencies can lead to infinite loops or incorrect validation results.

**Error Chain**: `core.schema`

**Packages**: `core/schema`
**Methods**: `schema.NewDocumentValidator`
**Severity**: High (indicates a structural problem in the schema's validation logic)

---

## ERR_SCHEMA_VALIDATOR_CREATION_FAILED
This error indicates a failure to create a nested document validator. This typically occurs when validating complex types like arrays, records, or unions, where sub-validators are dynamically created based on nested schemas.

**Error Chain**: `core.schema` -> (underlying validator creation error)

**Packages**: `core/schema`
**Methods**: `schema.ArrayValidationNode.validateArrayItem`, `schema.RecordValidationNode.validateRecordItem`, `schema.UnionValidationNode.tryUnionSchema`
**Severity**: High (indicates a failure in dynamically setting up validation for complex data structures)

---
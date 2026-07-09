package meta

import "github.com/asaidimu/go-anansi/v8/core/schema/definition"

// DevelopmentSchemaValidator returns a DocumentValidator for MetaSchema with the
// UUIDv7 constraint removed. Useful in development and tests that use
// non-UUIDv7 identifiers.
func DevelopmentSchemaValidator() *definition.DocumentValidator {
	clone := MetaSchema
	clone.Constraints = make(map[definition.ConstraintId]definition.Constraint, len(MetaSchema.Constraints)-1)
	for id, c := range MetaSchema.Constraints {
		if id != ConstraintFieldIdMustBeUUIDv7 {
			clone.Constraints[id] = c
		}
	}
	vd, err := definition.NewDocumentValidator(&clone, MetaSchemaPredicates)
	if err != nil {
		panic(err)
	}
	return vd
}

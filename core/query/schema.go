package query

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// SchemaFromQueryOptions provides configuration options for schema generation
type SchemaFromQueryOptions struct {
	// FunctionTypeMap maps function names to their return types
	// If not provided, function calls will default to FieldTypeUnknown
	FunctionTypeMap map[string]definition.FieldType

	// DefaultAggregationTypes maps aggregation types to their expected return types
	// If not provided, aggregation types will default to FieldTypeUnknown
	DefaultAggregationTypes map[AggregationType]definition.FieldType
}

// DefaultSchemaFromQueryOptions returns sensible defaults for schema generation
func DefaultSchemaFromQueryOptions() *SchemaFromQueryOptions {
	return &SchemaFromQueryOptions{
		FunctionTypeMap: make(map[string]definition.FieldType),
		DefaultAggregationTypes: map[AggregationType]definition.FieldType{
			AggregationTypeCount: definition.FieldTypeInteger,
			AggregationTypeSum:   definition.FieldTypeNumber,
			AggregationTypeAvg:   definition.FieldTypeNumber,
			AggregationTypeMin:   definition.FieldTypeUnknown,
			AggregationTypeMax:   definition.FieldTypeUnknown,
		},
	}
}

// SchemaFromQuery generates a schema definition for the expected result of a query
func SchemaFromQuery(q *Query, options *SchemaFromQueryOptions) (*definition.Schema, error) {
	if options == nil {
		options = DefaultSchemaFromQueryOptions()
	}

	// Validate that we have a target schema
	if q.Target == nil || q.Target.Schema == nil {
		return nil, common.NewSystemError("ERR_QUERY_SCHEMA_TARGET_REQUIRED", "query target schema is required").WithOperation("SchemaFromQuery").WithCause(errors.New("query target schema is required"))
	}

	// Handle aggregation queries - they have a completely different result structure
	if len(q.Aggregations) > 0 {
		return generateAggregationResultSchema(q, options)
	}

	// Start with the base schema from the target
	resultSchema := q.Target.Schema.DeepCopy()

	// Apply joins - this adds nested schemas for joined collections
	if err := applyJoinsToSchema(resultSchema, q.Joins); err != nil {
		return nil, common.NewSystemError("ERR_QUERY_SCHEMA_APPLY_JOINS_FAILED", "failed to apply joins to schema").WithOperation("SchemaFromQuery").WithCause(err)
	}

	// Apply projections - this filters and transforms fields
	if q.Projection != nil {
		if err := applyProjectionToSchema(resultSchema, q.Projection, q.Target.Schema, options); err != nil {
			return nil, common.NewSystemError("ERR_QUERY_SCHEMA_APPLY_PROJECTION_FAILED", "failed to apply projection to schema").WithOperation("SchemaFromQuery").WithCause(err)
		}
	}

	// Update schema metadata
	resultSchema.Name = generateResultSchemaName(q)
	resultSchema.Description = "Generated schema for query result"

	return resultSchema, nil
}

// generateAggregationResultSchema creates a schema for aggregation query results
func generateAggregationResultSchema(q *Query, options *SchemaFromQueryOptions) (*definition.Schema, error) {
	resultSchema := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:        generateResultSchemaName(q) + "_aggregated",
			Description: "Generated schema for aggregation query result",
			Fields:      make(map[definition.FieldId]definition.Field),
		},
	}

	// For aggregations, the result structure depends on grouping
	hasGrouping := false
	// Check if any aggregation has groups
	for _, agg := range q.Aggregations {
		if len(agg.Groups) > 0 {
			hasGrouping = true
			break
		}
	}

	if hasGrouping {
		// Result will be: { "groupValue": { "agg1": value, "agg2": value } }
		// We need to create a dynamic structure based on group fields

		// Create a nested schema for the aggregation values
		aggregationValueSchema := definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name:        "AggregationValues",
				Description: "Schema for aggregated values",
				Fields:      make(map[definition.FieldId]definition.Field),
			},
		}

		// Add fields for each aggregation
		for _, agg := range q.Aggregations {
			fieldName := agg.Field
			if agg.Alias != nil {
				fieldName = *agg.Alias
			} else {
				fieldName = fmt.Sprintf("%s_%s", string(agg.Type), agg.Field)
			}

			fieldType := inferAggregationFieldType(agg, q.Target.Schema, options)
			aggregationValueSchema.Fields[definition.FieldId(fieldName)] = definition.Field{
				Name:        definition.FieldName(fieldName),
				Description: fmt.Sprintf("%s aggregation on field %s", string(agg.Type), agg.Field),
				FieldProperties: definition.FieldProperties{
					Type: fieldType,
				},
			}
		}

		// Add the nested schema to result schema
		if resultSchema.Schemas == nil {
			resultSchema.Schemas = make(map[definition.SchemaId]definition.NestedSchema)
		}
		resultSchema.Schemas["AggregationValues"] = aggregationValueSchema

		// The result is a record type with dynamic keys pointing to aggregation values
		resultSchema.Fields["aggregation_results"] = definition.Field{
			Name:        "aggregation_results",
			Description: "Grouped aggregation results",
			Required:    true,
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeRecord,
				Schema: definition.NewSchemaReference(definition.SchemaReference{
					ID: "AggregationValues",
				}),
			},
		}
	} else {
		// Simple aggregation without grouping - flat object with aggregation results
		for _, agg := range q.Aggregations {
			fieldName := agg.Field
			if agg.Alias != nil {
				fieldName = *agg.Alias
			} else {
				fieldName = fmt.Sprintf("%s_%s", string(agg.Type), agg.Field)
			}

			fieldType := inferAggregationFieldType(agg, q.Target.Schema, options)
			resultSchema.Fields[definition.FieldId(fieldName)] = definition.Field{
				Name:        definition.FieldName(fieldName),
				Description: fmt.Sprintf("%s aggregation on field %s", string(agg.Type), agg.Field),
				FieldProperties: definition.FieldProperties{
					Type: fieldType,
				},
			}
		}
	}

	return resultSchema, nil
}

func applyJoinsToSchema(resultSchema *definition.Schema, joins []JoinConfiguration) error {
	for _, join := range joins {
		if join.Target.Schema == nil {
			return common.NewSystemError("ERR_QUERY_SCHEMA_JOIN_TARGET_REQUIRED", fmt.Sprintf("join target schema is required for %s", join.Target.Name)).WithOperation("applyJoinsToSchema").WithCause(errors.New("join target schema is required"))
		}

		// Determine the field name for the joined data
		fieldName := join.Target.Name
		if join.Target.Alias != nil {
			fieldName = *join.Target.Alias
		}

		// Handle nested joins
		parts := strings.Split(fieldName, ".")

		// In definition.Schema, nested schemas are global to the schema, not nested within each other.
		// However, fields can point to them.

		currentFields := resultSchema.Fields
		var currentBase *definition.BaseSchema = &resultSchema.BaseSchema

		for i, part := range parts {
			if i < len(parts)-1 {
				// This is a nested part, so traverse the schema
				fieldId := definition.FieldId(part)
				field, exists := currentFields[fieldId]
				if !exists {
					return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_JOIN_FIELD_NOT_FOUND", fmt.Sprintf("nested join field '%s' not found in schema", part)).WithOperation("applyJoinsToSchema").WithCause(errors.New("nested join field not found"))
				}

				if field.Type != definition.FieldTypeObject {
					return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_JOIN_FIELD_NOT_OBJECT", fmt.Sprintf("nested join field '%s' is not an object", part)).WithOperation("applyJoinsToSchema").WithCause(errors.New("nested join field is not an object"))
				}

				if field.Schema.IsZero() || !field.Schema.IsSingle() {
					return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_JOIN_NO_SCHEMA_REF", fmt.Sprintf("nested join field '%s' does not have a nested schema reference", part)).WithOperation("applyJoinsToSchema").WithCause(errors.New("nested join field does not have a nested schema reference"))
				}

				nestedRef, _ := definition.FieldSchemaAs[definition.SchemaReference](field.Schema)
				nestedSchema, exists := resultSchema.Schemas[nestedRef.ID]
				if !exists {
					return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_SCHEMA_NOT_FOUND", fmt.Sprintf("nested schema '%s' not found for nested join", nestedRef.ID)).WithOperation("applyJoinsToSchema").WithCause(errors.New("nested schema not found for nested join"))
				}

				currentFields = nestedSchema.Fields
				currentBase = &nestedSchema.BaseSchema

			} else {
				// This is the last part, so add the join field here
				if err := addJoinField(resultSchema, currentBase, part, join); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func addJoinField(rootSchema *definition.Schema, targetBase *definition.BaseSchema, fieldName string, join JoinConfiguration) error {
	// Clone the joined schema
	joinedSchema := join.Target.Schema.DeepCopy()

	// Apply projection to joined schema if specified
	if join.Projection != nil {
		if err := applyProjectionToSchema(joinedSchema, join.Projection, join.Target.Schema, nil); err != nil {
			return common.NewSystemError("ERR_QUERY_SCHEMA_APPLY_PROJECTION_JOINED_FAILED", fmt.Sprintf("failed to apply projection to joined schema %s", fieldName)).WithOperation("addJoinField").WithCause(err)
		}
	}

	// Create nested schema definition
	nestedSchemaName := fmt.Sprintf("%s_joined", fieldName)
	nestedSchema := definition.NestedSchema{
		BaseSchema: definition.BaseSchema{
			Name:        nestedSchemaName,
			Description: fmt.Sprintf("Joined data from %s", join.Target.Name),
			Fields:      joinedSchema.Fields,
			Indexes:     joinedSchema.Indexes,
			Constraints: joinedSchema.Constraints,
			Metadata:    joinedSchema.Metadata,
		},
	}

	// Add to nested schemas of the root schema
	if rootSchema.Schemas == nil {
		rootSchema.Schemas = make(map[definition.SchemaId]definition.NestedSchema)
	}
	rootSchema.Schemas[definition.SchemaId(nestedSchemaName)] = nestedSchema

	// Add field to target base schema pointing to nested schema
	joinFieldType := definition.FieldTypeObject

	targetBase.Fields[definition.FieldId(fieldName)] = definition.Field{
		Name:        definition.FieldName(fieldName),
		Description: fmt.Sprintf("Data joined from %s collection", join.Target.Name),
		Required:    join.Type == JoinTypeInner, // Inner joins guarantee presence
		FieldProperties: definition.FieldProperties{
			Type: joinFieldType,
			Schema: definition.NewSchemaReference(definition.SchemaReference{
				ID: definition.SchemaId(nestedSchemaName),
			}),
		},
	}

	return nil
}

// applyProjectionToSchema modifies the schema based on projection configuration
func applyProjectionToSchema(resultSchema *definition.Schema, projection *ProjectionConfiguration, originalSchema *definition.Schema, options *SchemaFromQueryOptions) error {
	// Handle exclusions first
	if len(projection.Exclude) > 0 {
		for _, exclude := range projection.Exclude {
			fieldId := definition.FieldId(exclude.Name)

			// Handle nested exclusions
			if exclude.Nested != nil {
				if field, exists := resultSchema.Fields[fieldId]; exists {
					// Apply nested exclusion to the field's schema
					if err := applyNestedProjection(resultSchema, &field, exclude.Nested, originalSchema); err != nil {
						return common.NewSystemError("ERR_QUERY_SCHEMA_RECURSIVE_NESTED_EXCLUSION_FAILED", fmt.Sprintf("%s to field %s", err.Error(), exclude.Name)).WithOperation("applyProjectionToSchema").WithCause(err)
					}
					resultSchema.Fields[fieldId] = field
				}
			} else {
				delete(resultSchema.Fields, fieldId)
			}
		}
	}

	// Handle inclusions (if specified, only included fields remain)
	if len(projection.Include) > 0 {
		newFields := make(map[definition.FieldId]definition.Field)

		for _, include := range projection.Include {
			fieldName := include.Name
			if include.Alias != nil {
				fieldName = *include.Alias
			}

			oldFieldId := definition.FieldId(include.Name)
			if originalField, exists := originalSchema.Fields[oldFieldId]; exists {
				// Clone the field (already cloned by DeepCopy of resultSchema, but let's be safe if we want a fresh one)
				newField := originalField // Struct copy is enough since we will modify it
				newField.Name = definition.FieldName(fieldName)

				// Handle nested projections
				if include.Nested != nil {
					if err := applyNestedProjection(resultSchema, &newField, include.Nested, originalSchema); err != nil {
						return common.NewSystemError("ERR_QUERY_SCHEMA_RECURSIVE_NESTED_PROJECTION_FAILED", fmt.Sprintf("%s to field %s", err.Error(), include.Name)).WithOperation("applyProjectionToSchema").WithCause(err)
					}
				}

				newFields[definition.FieldId(fieldName)] = newField
			}
		}

		resultSchema.Fields = newFields
	}

	// Handle computed fields
	if len(projection.Computed) > 0 && options != nil {
		for _, computed := range projection.Computed {
			var fieldDef *definition.Field

			if computed.ComputedFieldExpression != nil {
				fieldDef = createComputedFieldDefinition(computed.ComputedFieldExpression, options)
			} else if computed.CaseExpression != nil {
				fieldDef = createCaseFieldDefinition(computed.CaseExpression)
			}

			if fieldDef != nil {
				resultSchema.Fields[definition.FieldId(fieldDef.Name)] = *fieldDef
			}
		}
	}

	return nil
}

// createComputedFieldDefinition creates a field definition for a computed field
func createComputedFieldDefinition(expr *ComputedFieldExpression, options *SchemaFromQueryOptions) *definition.Field {
	fieldType := definition.FieldTypeUnknown

	if expr.Expression != nil {
		if fnType, exists := options.FunctionTypeMap[expr.Expression.Function]; exists {
			fieldType = fnType
		}
	}

	return &definition.Field{
		Name:        definition.FieldName(expr.Alias),
		Description: fmt.Sprintf("Computed field using function %s", expr.Expression.Function),
		FieldProperties: definition.FieldProperties{
			Type: fieldType,
		},
	}
}

// createCaseFieldDefinition creates a field definition for a case expression
func createCaseFieldDefinition(expr *CaseExpression) *definition.Field {
	return &definition.Field{
		Name:        definition.FieldName(expr.Alias),
		Description: "Case expression result",
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeUnknown,
		},
	}
}

// inferAggregationFieldType determines the appropriate field type for an aggregation result
func inferAggregationFieldType(agg AggregationConfiguration, targetSchema *definition.Schema, options *SchemaFromQueryOptions) definition.FieldType {
	// For MIN and MAX, try to use the original field type
	if agg.Type == AggregationTypeMin || agg.Type == AggregationTypeMax {
		if field, exists := targetSchema.Fields[definition.FieldId(agg.Field)]; exists {
			// For numeric types, return the same type
			switch field.Type {
			case definition.FieldTypeNumber, definition.FieldTypeInteger, definition.FieldTypeDecimal:
				return field.Type
			default:
				return definition.FieldTypeUnknown
			}
		}
	}

	// Use default aggregation type mapping
	if fieldType, exists := options.DefaultAggregationTypes[agg.Type]; exists {
		return fieldType
	}

	return definition.FieldTypeUnknown
}

// generateResultSchemaName creates a descriptive name for the result schema
func generateResultSchemaName(q *Query) string {
	if q.Target != nil && q.Target.Schema != nil {
		baseName := q.Target.Schema.Name

		// Add suffixes based on query operations
		var suffixes []string

		if len(q.Joins) > 0 {
			suffixes = append(suffixes, "joined")
		}

		if q.Projection != nil && (len(q.Projection.Include) > 0 || len(q.Projection.Exclude) > 0) {
			suffixes = append(suffixes, "projected")
		}

		if len(q.Aggregations) > 0 {
			suffixes = append(suffixes, "aggregated")
		}

		if len(suffixes) > 0 {
			return fmt.Sprintf("%s_%s_result", baseName, strings.Join(suffixes, "_"))
		}

		return fmt.Sprintf("%s_result", baseName)
	}

	return "query_result"
}

// applyNestedProjection applies projection to nested field schemas
func applyNestedProjection(rootSchema *definition.Schema, field *definition.Field, projection *ProjectionConfiguration, originalSchema *definition.Schema) error {
	// Only handle fields that can have nested schemas
	switch field.Type {
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeRecord:
		// Continue processing
	default:
		return common.NewSystemError("ERR_QUERY_SCHEMA_CANNOT_APPLY_NESTED_PROJECTION", fmt.Sprintf("cannot apply nested projection to field of type %s", field.Type.String())).WithOperation("applyNestedProjection").WithCause(errors.New("cannot apply nested projection to field of this type"))
	}

	// Handle nested schema references
	if field.Schema.IsZero() {
		return nil
	}

	if field.Schema.IsSingle() {
		nestedRef, _ := definition.FieldSchemaAs[definition.SchemaReference](field.Schema)
		return applyProjectionToSchemaReference(rootSchema, field, nestedRef, projection, originalSchema)
	}

	if field.Schema.IsMultiple() {
		nestedRefs, _ := definition.FieldSchemaAs[[]definition.SchemaReference](field.Schema)
		return applyProjectionToSchemaReferenceSlice(rootSchema, field, nestedRefs, projection, originalSchema)
	}

	return common.NewSystemError("ERR_QUERY_SCHEMA_UNSUPPORTED_NESTED_SCHEMA_FORMAT", fmt.Sprintf("unsupported nested schema format for field %s", field.Name)).WithOperation("applyNestedProjection").WithCause(errors.New("unsupported nested schema format"))
}

// applyProjectionToSchemaReference handles projection for a single nested schema reference
func applyProjectionToSchemaReference(rootSchema *definition.Schema, field *definition.Field, nestedRef definition.SchemaReference, projection *ProjectionConfiguration, originalSchema *definition.Schema) error {
	// Find the referenced nested schema
	originalNestedSchema, exists := originalSchema.Schemas[nestedRef.ID]
	if !exists {
		return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_SCHEMA_REF_NOT_FOUND", fmt.Sprintf("nested schema reference %s not found", nestedRef.ID)).WithOperation("applyProjectionToSchemaReference").WithCause(errors.New("nested schema reference not found"))
	}

	// Deep copy the nested schema for modification
	modifiedNestedSchema := originalNestedSchema // Struct copy is enough for BaseSchema if we deep copy fields later
	modifiedNestedSchema.Fields = make(map[definition.FieldId]definition.Field, len(originalNestedSchema.Fields))
	for id, f := range originalNestedSchema.Fields {
		modifiedNestedSchema.Fields[id] = f // We'll project these below
	}

	// Apply projection to the nested schema's fields
	if err := applyProjectionToFieldsMap(rootSchema, modifiedNestedSchema.Fields, projection, originalSchema); err != nil {
		return common.NewSystemError("ERR_QUERY_SCHEMA_APPLY_PROJECTION_NESTED_FAILED", fmt.Sprintf("failed to apply projection to nested schema %s", nestedRef.ID)).WithOperation("applyProjectionToSchemaReference").WithCause(err)
	}

	// Create a new nested schema ID to avoid conflicts
	projectedSchemaID := generateProjectedSchemaID(string(nestedRef.ID), projection)

	// Update both the schema name and register it with the new ID
	modifiedNestedSchema.Name = projectedSchemaID

	// Ensure the root schema has a Schemas map
	if rootSchema.Schemas == nil {
		rootSchema.Schemas = make(map[definition.SchemaId]definition.NestedSchema)
	}

	// Register the modified nested schema
	rootSchema.Schemas[definition.SchemaId(projectedSchemaID)] = modifiedNestedSchema

	// Update the field's schema reference to use the new schema name/ID
	updatedRef := definition.SchemaReference{
		ID:          definition.SchemaId(projectedSchemaID),
		Constraints: nestedRef.Constraints, // Preserve existing constraints
		Indexes:     nestedRef.Indexes,     // Preserve existing indexes
	}
	field.Schema = definition.NewSchemaReference(updatedRef)

	return nil
}

// applyProjectionToSchemaReferenceSlice handles projection for union types with multiple schema references
func applyProjectionToSchemaReferenceSlice(rootSchema *definition.Schema, field *definition.Field, nestedRefs []definition.SchemaReference, projection *ProjectionConfiguration, originalSchema *definition.Schema) error {
	projectedRefs := make([]definition.SchemaReference, 0, len(nestedRefs))

	for _, nestedRef := range nestedRefs {
		// Find the referenced nested schema
		originalNestedSchema, exists := originalSchema.Schemas[nestedRef.ID]
		if !exists {
			return common.NewSystemError("ERR_QUERY_SCHEMA_NESTED_SCHEMA_REF_NOT_FOUND_UNION", fmt.Sprintf("nested schema reference %s not found in union type", nestedRef.ID)).WithOperation("applyProjectionToSchemaReferenceSlice").WithCause(errors.New("nested schema reference not found in union type"))
		}

		// Clone the nested schema for modification
		modifiedNestedSchema := originalNestedSchema
		modifiedNestedSchema.Fields = make(map[definition.FieldId]definition.Field, len(originalNestedSchema.Fields))
		for id, f := range originalNestedSchema.Fields {
			modifiedNestedSchema.Fields[id] = f
		}

		// Apply projection
		if err := applyProjectionToFieldsMap(rootSchema, modifiedNestedSchema.Fields, projection, originalSchema); err != nil {
			return common.NewSystemError("ERR_QUERY_SCHEMA_APPLY_PROJECTION_NESTED_UNION_FAILED", fmt.Sprintf("failed to apply projection to nested schema %s in union", nestedRef.ID)).WithOperation("applyProjectionToSchemaReferenceSlice").WithCause(err)
		}

		// Create a new nested schema ID for the projected version
		projectedSchemaID := generateProjectedSchemaID(string(nestedRef.ID), projection)

		// Update both the schema name and register it with the new ID
		modifiedNestedSchema.Name = projectedSchemaID

		// Ensure the root schema has a Schemas map
		if rootSchema.Schemas == nil {
			rootSchema.Schemas = make(map[definition.SchemaId]definition.NestedSchema)
		}

		// Register the modified nested schema
		rootSchema.Schemas[definition.SchemaId(projectedSchemaID)] = modifiedNestedSchema

		// Create updated reference
		updatedRef := definition.SchemaReference{
			ID:          definition.SchemaId(projectedSchemaID),
			Constraints: nestedRef.Constraints, // Preserve existing constraints
			Indexes:     nestedRef.Indexes,     // Preserve existing indexes
		}
		projectedRefs = append(projectedRefs, updatedRef)
	}

	// Update the field's schema reference slice
	field.Schema = definition.NewSchemaReference(projectedRefs)
	return nil
}

// applyProjectionToFieldsMap applies projection to a map of field definitions
func applyProjectionToFieldsMap(rootSchema *definition.Schema, fields map[definition.FieldId]definition.Field, projection *ProjectionConfiguration, originalSchema *definition.Schema) error {
	// Handle exclusions first
	if len(projection.Exclude) > 0 {
		for _, exclude := range projection.Exclude {
			fieldId := definition.FieldId(exclude.Name)
			if exclude.Nested != nil {
				// Apply nested projection before excluding the field
				if field, exists := fields[fieldId]; exists {
					if err := applyNestedProjection(rootSchema, &field, exclude.Nested, originalSchema); err != nil {
						return common.NewSystemError("ERR_QUERY_SCHEMA_RECURSIVE_NESTED_EXCLUSION_FAILED", fmt.Sprintf("failed to apply recursive nested exclusion to field %s", exclude.Name)).WithOperation("applyProjectionToFieldsMap").WithCause(err)
					}
					fields[fieldId] = field
				}
			} else {
				// Simple exclusion - remove the field
				delete(fields, fieldId)
			}
		}
	}

	// Handle inclusions (if specified, only included fields remain)
	if len(projection.Include) > 0 {
		newFields := make(map[definition.FieldId]definition.Field)

		for _, include := range projection.Include {
			fieldName := include.Name
			if include.Alias != nil {
				fieldName = *include.Alias
			}

			oldFieldId := definition.FieldId(include.Name)
			if originalField, exists := fields[oldFieldId]; exists {
				// Clone the field
				newField := originalField
				newField.Name = definition.FieldName(fieldName)

				// Handle recursive nested projections
				if include.Nested != nil {
					if err := applyNestedProjection(rootSchema, &newField, include.Nested, originalSchema); err != nil {
						return common.NewSystemError("ERR_QUERY_SCHEMA_RECURSIVE_NESTED_PROJECTION_FAILED", fmt.Sprintf("failed to apply recursive nested projection to field %s", include.Name)).WithOperation("applyProjectionToFieldsMap").WithCause(err)
					}
				}

				newFields[definition.FieldId(fieldName)] = newField
			} else {
				// Field doesn't exist in the original schema
				return common.NewSystemError("ERR_QUERY_SCHEMA_PROJECTION_INCLUDE_FIELD_NOT_EXIST", fmt.Sprintf("field %s specified in projection include does not exist", include.Name)).WithOperation("applyProjectionToFieldsMap").WithCause(errors.New("field specified in projection include does not exist"))
			}
		}

		// Replace the original fields map with the projected fields
		for k := range fields {
			delete(fields, k)
		}
		maps.Copy(fields, newFields)
	}

	// Handle computed fields in nested context
	if len(projection.Computed) > 0 {
		for _, computed := range projection.Computed {
			var fieldDef *definition.Field

			if computed.ComputedFieldExpression != nil {
				fieldDef = createComputedFieldDefinitionForNested(computed.ComputedFieldExpression)
			} else if computed.CaseExpression != nil {
				fieldDef = createCaseFieldDefinitionForNested(computed.CaseExpression)
			}

			if fieldDef != nil {
				// Check for field name conflicts
				if _, exists := fields[definition.FieldId(fieldDef.Name)]; exists {
					return common.NewSystemError("ERR_QUERY_SCHEMA_COMPUTED_FIELD_CONFLICT", fmt.Sprintf("computed field %s conflicts with existing field", fieldDef.Name)).WithOperation("applyProjectionToFieldsMap").WithCause(errors.New("computed field conflicts with existing field"))
				}
				fields[definition.FieldId(fieldDef.Name)] = *fieldDef
			}
		}
	}

	return nil
}

// createComputedFieldDefinitionForNested creates a field definition for a computed field in nested context
func createComputedFieldDefinitionForNested(expr *ComputedFieldExpression) *definition.Field {
	return &definition.Field{
		Name:        definition.FieldName(expr.Alias),
		Description: fmt.Sprintf("Computed field using function %s", expr.Expression.Function),
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeUnknown,
		},
	}
}

// createCaseFieldDefinitionForNested creates a field definition for a case expression in nested context
func createCaseFieldDefinitionForNested(expr *CaseExpression) *definition.Field {
	return &definition.Field{
		Name:        definition.FieldName(expr.Alias),
		Description: "Case expression result",
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeUnknown,
		},
	}
}

// generateProjectedSchemaID creates a unique ID for a projected nested schema
func generateProjectedSchemaID(originalID string, projection *ProjectionConfiguration) string {
	// Create a deterministic hash based on projection configuration
	var parts []string
	parts = append(parts, originalID)

	if len(projection.Include) > 0 {
		parts = append(parts, "inc")
		for _, inc := range projection.Include {
			parts = append(parts, inc.Name)
			if inc.Alias != nil {
				parts = append(parts, *inc.Alias)
			}
		}
	}

	if len(projection.Exclude) > 0 {
		parts = append(parts, "exc")
		for _, exc := range projection.Exclude {
			parts = append(parts, exc.Name)
		}
	}

	if len(projection.Computed) > 0 {
		parts = append(parts, "comp")
		for _, comp := range projection.Computed {
			if comp.ComputedFieldExpression != nil {
				parts = append(parts, comp.ComputedFieldExpression.Alias)
			} else if comp.CaseExpression != nil {
				parts = append(parts, comp.CaseExpression.Alias)
			}
		}
	}

	// Create a hash-like suffix to ensure uniqueness while keeping it readable
	suffix := fmt.Sprintf("%x", len(strings.Join(parts, "_")))
	return fmt.Sprintf("%s_projected_%s", originalID, suffix)
}

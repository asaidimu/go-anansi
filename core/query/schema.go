package query

import (
	"maps"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

const (
	FieldTypeUnknown schema.FieldType = "unknown"
)

// SchemaFromQueryOptions provides configuration options for schema generation
type SchemaFromQueryOptions struct {
	// FunctionTypeMap maps function names to their return types
	// If not provided, function calls will default to FieldTypeUnknown
	FunctionTypeMap map[string]schema.FieldType

	// DefaultAggregationTypes maps aggregation types to their expected return types
	DefaultAggregationTypes map[AggregationType]schema.FieldType
}

// DefaultSchemaFromQueryOptions returns sensible defaults for schema generation
func DefaultSchemaFromQueryOptions() *SchemaFromQueryOptions {
	return &SchemaFromQueryOptions{
		FunctionTypeMap: make(map[string]schema.FieldType),
		DefaultAggregationTypes: map[AggregationType]schema.FieldType{
			AggregationTypeCount: schema.FieldTypeInteger,
			AggregationTypeSum:   schema.FieldTypeNumber,
			AggregationTypeAvg:   schema.FieldTypeNumber,
			AggregationTypeMin:   FieldTypeUnknown, // Depends on the field type
			AggregationTypeMax:   FieldTypeUnknown, // Depends on the field type
		},
	}
}

// SchemaFromQuery generates a schema definition for the expected result of a query
func SchemaFromQuery(q *Query, options *SchemaFromQueryOptions) (*schema.SchemaDefinition, error) {
	if options == nil {
		options = DefaultSchemaFromQueryOptions()
	}

	// Validate that we have a target schema
	if q.Target == nil || q.Target.Schema == nil {
		return nil, fmt.Errorf("query target schema is required")
	}

	// Handle aggregation queries - they have a completely different result structure
	if len(q.Aggregations) > 0 {
		return generateAggregationResultSchema(q, options)
	}

	// Start with the base schema from the target
	resultSchema := cloneSchemaDefinition(q.Target.Schema)

	// Apply joins - this adds nested schemas for joined collections
	if err := applyJoinsToSchema(resultSchema, q.Joins); err != nil {
		return nil, fmt.Errorf("failed to apply joins to schema: %w", err)
	}

	// Apply projections - this filters and transforms fields
	if q.Projection != nil {
		if err := applyProjectionToSchema(resultSchema, q.Projection, q.Target.Schema, options); err != nil {
			return nil, fmt.Errorf("failed to apply projection to schema: %w", err)
		}
	}

	// Update schema metadata
	resultSchema.Name = generateResultSchemaName(q)
	resultSchema.Description = "Generated schema for query result"

	return resultSchema, nil
}

// generateAggregationResultSchema creates a schema for aggregation query results
func generateAggregationResultSchema(q *Query, options *SchemaFromQueryOptions) (*schema.SchemaDefinition, error) {
	resultSchema := &schema.SchemaDefinition{
		Name:        generateResultSchemaName(q) + "_aggregated",
		Version:     "1.0.0",
		Description: "Generated schema for aggregation query result",
		Fields:      make(map[string]*schema.FieldDefinition),
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

		// The top-level will be an object where keys are group values
		// and values are objects containing aggregation results

		// Create a nested schema for the aggregation values
		aggregationValueSchema := &schema.NestedSchemaDefinition{
			Name:         "AggregationValues",
			Description:  utils.StringPtr("Schema for aggregated values"),
			IsStructured: utils.BoolPtr(true),
			StructuredFieldsMap: make(map[string]*schema.FieldDefinition),
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
			aggregationValueSchema.StructuredFieldsMap[fieldName] = &schema.FieldDefinition{
				Name:        fieldName,
				Type:        fieldType,
				Required:    utils.BoolPtr(false),
				Description: utils.StringPtr(fmt.Sprintf("%s aggregation on field %s", string(agg.Type), agg.Field)),
			}
		}

		// Add the nested schema to result schema
		if resultSchema.NestedSchemas == nil {
			resultSchema.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
		}
		resultSchema.NestedSchemas["AggregationValues"] = aggregationValueSchema

		// The result is a record type with dynamic keys pointing to aggregation values
		resultSchema.Fields["aggregation_results"] = &schema.FieldDefinition{
			Name:        "aggregation_results",
			Type:        schema.FieldTypeRecord,
			Required:    utils.BoolPtr(true),
			Description: utils.StringPtr("Grouped aggregation results"),
			Schema: schema.NestedSchemaReference{
				ID: "AggregationValues",
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
			resultSchema.Fields[fieldName] = &schema.FieldDefinition{
				Name:        fieldName,
				Type:        fieldType,
				Required:    utils.BoolPtr(false),
				Description: utils.StringPtr(fmt.Sprintf("%s aggregation on field %s", string(agg.Type), agg.Field)),
			}
		}
	}

	return resultSchema, nil
}

func applyJoinsToSchema(resultSchema *schema.SchemaDefinition, joins []JoinConfiguration) error {
	for _, join := range joins {
		if join.Target.Schema == nil {
			return fmt.Errorf("join target schema is required for %s", join.Target.Name)
		}

		// Determine the field name for the joined data
		fieldName := join.Target.Name
		if join.Target.Alias != nil {
			fieldName = *join.Target.Alias
		}

		// Handle nested joins
		parts := strings.Split(fieldName, ".")
		currentSchema := resultSchema

		for i, part := range parts {
			if i < len(parts)-1 {
				// This is a nested part, so traverse the schema
				field, exists := currentSchema.Fields[part]
				if !exists {
					return fmt.Errorf("nested join field '%s' not found in schema", part)
				}

				if field.Type != schema.FieldTypeObject {
					return fmt.Errorf("nested join field '%s' is not an object", part)
				}

				nestedSchemaRef, ok := field.Schema.(schema.NestedSchemaReference)
				if !ok {
					return fmt.Errorf("nested join field '%s' does not have a nested schema reference", part)
				}

				nestedSchema, exists := currentSchema.NestedSchemas[nestedSchemaRef.ID]
				if !exists {
					return fmt.Errorf("nested schema '%s' not found for nested join", nestedSchemaRef.ID)
				}

				currentSchema = &schema.SchemaDefinition{
					Name:          nestedSchema.Name,
					NestedSchemas: currentSchema.NestedSchemas,
					Fields:        nestedSchema.StructuredFieldsMap,
				}

			} else {
				// This is the last part, so add the join field here
				if err := addJoinField(currentSchema, part, join); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func addJoinField(targetSchema *schema.SchemaDefinition, fieldName string, join JoinConfiguration) error {
	// Clone the joined schema
	joinedSchema := cloneSchemaDefinition(join.Target.Schema)

	// Apply projection to joined schema if specified
	if join.Projection != nil {
		if err := applyProjectionToSchema(joinedSchema, join.Projection, join.Target.Schema, nil); err != nil {
			return fmt.Errorf("failed to apply projection to joined schema %s: %w", fieldName, err)
		}
	}

	// Create nested schema definition
	nestedSchemaName := fmt.Sprintf("%s_joined", fieldName)
	nestedSchema := &schema.NestedSchemaDefinition{
		Name:                nestedSchemaName,
		Description:         utils.StringPtr(fmt.Sprintf("Joined data from %s", join.Target.Name)),
		IsStructured:        utils.BoolPtr(true),
		StructuredFieldsMap: joinedSchema.Fields,
	}

	// Add to nested schemas
	if targetSchema.NestedSchemas == nil {
		targetSchema.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
	}
	targetSchema.NestedSchemas[nestedSchemaName] = nestedSchema

	// Add field to main schema pointing to nested schema
	joinFieldType := schema.FieldTypeObject
	if join.Type == JoinTypeLeft || join.Type == JoinTypeRight || join.Type == JoinTypeFull {
		// Outer joins can result in null values
	}

	targetSchema.Fields[fieldName] = &schema.FieldDefinition{
		Name:        fieldName,
		Type:        joinFieldType,
		Required:    utils.BoolPtr(join.Type == JoinTypeInner), // Inner joins guarantee presence
		Description: utils.StringPtr(fmt.Sprintf("Data joined from %s collection", join.Target.Name)),
		Schema: schema.NestedSchemaReference{
			ID: nestedSchemaName,
		},
	}

	return nil
}

// applyProjectionToSchema modifies the schema based on projection configuration
func applyProjectionToSchema(resultSchema *schema.SchemaDefinition, projection *ProjectionConfiguration, originalSchema *schema.SchemaDefinition, options *SchemaFromQueryOptions) error {
	// Handle exclusions first
	if len(projection.Exclude) > 0 {
		for _, exclude := range projection.Exclude {
			delete(resultSchema.Fields, exclude.Name)

			// Handle nested exclusions
			if exclude.Nested != nil {
				if field, exists := resultSchema.Fields[exclude.Name]; exists {
					// Apply nested exclusion to the field's schema
					if err := applyNestedProjection(field, exclude.Nested, originalSchema); err != nil {
						return fmt.Errorf("failed to apply nested exclusion to field %s: %w", exclude.Name, err)
					}
				}
			}
		}
	}

	// Handle inclusions (if specified, only included fields remain)
	if len(projection.Include) > 0 {
		newFields := make(map[string]*schema.FieldDefinition)

		for _, include := range projection.Include {
			fieldName := include.Name
			if include.Alias != nil {
				fieldName = *include.Alias
			}

			if originalField, exists := originalSchema.Fields[include.Name]; exists {
				// Clone the field
				newField := cloneFieldDefinition(originalField)
				newField.Name = fieldName

				// Handle nested projections
				if include.Nested != nil {
					if err := applyNestedProjection(newField, include.Nested, originalSchema); err != nil {
						return fmt.Errorf("failed to apply nested projection to field %s: %w", include.Name, err)
					}
				}

				newFields[fieldName] = newField
			}
		}

		resultSchema.Fields = newFields
	}

	// Handle computed fields
	if len(projection.Computed) > 0 && options != nil {
		for _, computed := range projection.Computed {
			var fieldDef *schema.FieldDefinition

			if computed.ComputedFieldExpression != nil {
				fieldDef = createComputedFieldDefinition(computed.ComputedFieldExpression, options)
			} else if computed.CaseExpression != nil {
				fieldDef = createCaseFieldDefinition(computed.CaseExpression)
			}

			if fieldDef != nil {
				resultSchema.Fields[fieldDef.Name] = fieldDef
			}
		}
	}

	return nil
}

// createComputedFieldDefinition creates a field definition for a computed field
func createComputedFieldDefinition(expr *ComputedFieldExpression, options *SchemaFromQueryOptions) *schema.FieldDefinition {
	fieldType := FieldTypeUnknown

	if expr.Expression != nil {
		if fnType, exists := options.FunctionTypeMap[expr.Expression.Function]; exists {
			fieldType = fnType
		}
	}

	return &schema.FieldDefinition{
		Name:        expr.Alias,
		Type:        fieldType,
		Required:    utils.BoolPtr(false),
		Description: utils.StringPtr(fmt.Sprintf("Computed field using function %s", expr.Expression.Function)),
	}
}

// createCaseFieldDefinition creates a field definition for a case expression
func createCaseFieldDefinition(expr *CaseExpression) *schema.FieldDefinition {
	// Case expressions can return different types - we'll use unknown for simplicity
	// In a more sophisticated implementation, you could analyze the THEN clauses
	// to infer a common type
	return &schema.FieldDefinition{
		Name:        expr.Alias,
		Type:        FieldTypeUnknown,
		Required:    utils.BoolPtr(false),
		Description: utils.StringPtr("Case expression result"),
	}
}

// inferAggregationFieldType determines the appropriate field type for an aggregation result
func inferAggregationFieldType(agg AggregationConfiguration, targetSchema *schema.SchemaDefinition, options *SchemaFromQueryOptions) schema.FieldType {
	// For MIN and MAX, try to use the original field type
	if agg.Type == AggregationTypeMin || agg.Type == AggregationTypeMax {
		if field, exists := targetSchema.Fields[agg.Field]; exists {
			// For numeric types, return the same type
			switch field.Type {
			case schema.FieldTypeNumber, schema.FieldTypeInteger, schema.FieldTypeDecimal:
				return field.Type
			default:
				return FieldTypeUnknown
			}
		}
	}

	// Use default aggregation type mapping
	if fieldType, exists := options.DefaultAggregationTypes[agg.Type]; exists {
		return fieldType
	}

	return FieldTypeUnknown
}

// cloneSchemaDefinition creates a deep copy of a schema definition
func cloneSchemaDefinition(original *schema.SchemaDefinition) *schema.SchemaDefinition {
	clone := &schema.SchemaDefinition{
		Name:        original.Name,
		Version:     original.Version,
		Description: original.Description,
		Fields:      make(map[string]*schema.FieldDefinition),
		Indexes:     make([]schema.IndexDefinition, len(original.Indexes)),
		Constraints: make(schema.SchemaConstraint[schema.FieldType], len(original.Constraints)),
	}

	// Clone fields
	for name, field := range original.Fields {
		clone.Fields[name] = cloneFieldDefinition(field)
	}

	// Clone indexes
	copy(clone.Indexes, original.Indexes)

	// Clone constraints
	copy(clone.Constraints, original.Constraints)

	// Clone nested schemas if they exist
	if original.NestedSchemas != nil {
		clone.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
		for name, nestedSchema := range original.NestedSchemas {
			clone.NestedSchemas[name] = cloneNestedSchemaDefinition(nestedSchema)
		}
	}

	// Clone metadata if it exists
	if original.Metadata != nil {
		clone.Metadata = make(map[string]any)
		maps.Copy(clone.Metadata, original.Metadata)
	}

	return clone
}

// cloneFieldDefinition creates a deep copy of a field definition
func cloneFieldDefinition(original *schema.FieldDefinition) *schema.FieldDefinition {
	clone := &schema.FieldDefinition{
		Name:        original.Name,
		Type:        original.Type,
		Default:     original.Default,
		Schema:      original.Schema,
		Values:      make([]any, len(original.Values)),
		Constraints: make(schema.SchemaConstraint[schema.FieldType], len(original.Constraints)),
	}

	// Clone pointer fields
	if original.Required != nil {
		clone.Required = utils.BoolPtr(*original.Required)
	}
	if original.ItemsType != nil {
		itemsType := *original.ItemsType
		clone.ItemsType = &itemsType
	}
	if original.Deprecated != nil {
		clone.Deprecated = utils.BoolPtr(*original.Deprecated)
	}
	if original.Description != nil {
		clone.Description = utils.StringPtr(*original.Description)
	}
	if original.Unique != nil {
		clone.Unique = utils.BoolPtr(*original.Unique)
	}

	// Clone values slice
	copy(clone.Values, original.Values)

	// Clone constraints
	copy(clone.Constraints, original.Constraints)

	return clone
}

// cloneNestedSchemaDefinition creates a deep copy of a nested schema definition
func cloneNestedSchemaDefinition(original *schema.NestedSchemaDefinition) *schema.NestedSchemaDefinition {
	clone := &schema.NestedSchemaDefinition{
		Name:        original.Name,
		IsStructured: original.IsStructured,
		Schema:      original.Schema,
		Default:     original.Default,
	}

	// Clone pointer fields
	if original.Description != nil {
		clone.Description = utils.StringPtr(*original.Description)
	}
	if original.Concrete != nil {
		clone.Concrete = utils.BoolPtr(*original.Concrete)
	}
	if original.Type != nil {
		fieldType := *original.Type
		clone.Type = &fieldType
	}
	if original.ItemsType != nil {
		itemsType := *original.ItemsType
		clone.ItemsType = &itemsType
	}

	// Clone structured fields map
	if original.StructuredFieldsMap != nil {
		clone.StructuredFieldsMap = make(map[string]*schema.FieldDefinition)
		for name, field := range original.StructuredFieldsMap {
			clone.StructuredFieldsMap[name] = cloneFieldDefinition(field)
		}
	}

	// Clone structured fields array
	if original.StructuredFieldsArray != nil {
		clone.StructuredFieldsArray = make([]struct {
			Fields map[string]*schema.FieldDefinition `json:"fields"`
			When   *schema.FieldInclusionCondition    `json:"when,omitempty"`
		}, len(original.StructuredFieldsArray))

		for i, structuredFields := range original.StructuredFieldsArray {
			clone.StructuredFieldsArray[i].When = structuredFields.When
			if structuredFields.Fields != nil {
				clone.StructuredFieldsArray[i].Fields = make(map[string]*schema.FieldDefinition)
				for name, field := range structuredFields.Fields {
					clone.StructuredFieldsArray[i].Fields[name] = cloneFieldDefinition(field)
				}
			}
		}
	}

	// Clone indexes
	if original.Indexes != nil {
		clone.Indexes = make([]schema.IndexDefinition, len(original.Indexes))
		copy(clone.Indexes, original.Indexes)
	}

	// Clone constraints
	if original.Constraints != nil {
		clone.Constraints = make(schema.SchemaConstraint[schema.FieldType], len(original.Constraints))
		copy(clone.Constraints, original.Constraints)
	}

	// Clone metadata
	if original.Metadata != nil {
		clone.Metadata = make(map[string]any)
		maps.Copy(clone.Metadata, original.Metadata)
	}

	return clone
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
func applyNestedProjection(field *schema.FieldDefinition, projection *ProjectionConfiguration, originalSchema *schema.SchemaDefinition) error {
	// Only handle fields that can have nested schemas
	switch field.Type {
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeRecord:
		// Continue processing
	default:
		return fmt.Errorf("cannot apply nested projection to field of type %s", field.Type)
	}

	// Handle nested schema references
	if nestedRef, ok := field.Schema.(schema.NestedSchemaReference); ok {
		return applyProjectionToSchemaReference(field, nestedRef, projection, originalSchema)
	}

	// Handle inline nested field definitions (map[string]*FieldDefinition)
	if nestedFields, ok := field.Schema.(map[string]*schema.FieldDefinition); ok {
		return applyProjectionToInlineFields(nestedFields, projection, originalSchema)
	}

	// Handle slice of nested schema references (for union types)
	if nestedRefs, ok := field.Schema.([]schema.NestedSchemaReference); ok {
		return applyProjectionToSchemaReferenceSlice(field, nestedRefs, projection, originalSchema)
	}

	return fmt.Errorf("unsupported nested schema format for field %s", field.Name)
}

// applyProjectionToSchemaReference handles projection for a single nested schema reference
func applyProjectionToSchemaReference(field *schema.FieldDefinition, nestedRef schema.NestedSchemaReference, projection *ProjectionConfiguration, originalSchema *schema.SchemaDefinition) error {
	// Find the referenced nested schema using the provided method
	originalNestedSchema, exists := originalSchema.FindNestedSchema(nestedRef.ID)
	if !exists {
		return fmt.Errorf("nested schema reference %s not found", nestedRef.ID)
	}

	// Clone the nested schema for modification
	modifiedNestedSchema := cloneNestedSchemaDefinition(originalNestedSchema)

	// Apply projection to the nested schema's fields
	if originalNestedSchema.IsStructured != nil && *originalNestedSchema.IsStructured {
		if err := applyProjectionToNestedSchema(modifiedNestedSchema, projection, originalSchema); err != nil {
			return fmt.Errorf("failed to apply projection to nested schema %s: %w", nestedRef.ID, err)
		}
	} else {
		// For non-structured nested schemas, we can't apply field-level projections
		return fmt.Errorf("cannot apply field projection to non-structured nested schema %s", nestedRef.ID)
	}

	// Create a new nested schema ID to avoid conflicts
	projectedSchemaID := generateProjectedSchemaID(nestedRef.ID, projection)

	// Update both the schema name and register it with the new ID
	modifiedNestedSchema.Name = projectedSchemaID

	// Ensure the parent schema has a NestedSchemas map
	if originalSchema.NestedSchemas == nil {
		originalSchema.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
	}

	// Register the modified nested schema using the same ID as the name
	originalSchema.NestedSchemas[projectedSchemaID] = modifiedNestedSchema

	// Update the field's schema reference to use the new schema name/ID
	updatedRef := schema.NestedSchemaReference{
		ID:          projectedSchemaID, // This must match NestedSchemaDefinition.Name
		Constraints: nestedRef.Constraints, // Preserve existing constraints
		Indexes:     nestedRef.Indexes,     // Preserve existing indexes
	}
	field.Schema = updatedRef

	return nil
}

// applyProjectionToSchemaReferenceSlice handles projection for union types with multiple schema references
func applyProjectionToSchemaReferenceSlice(field *schema.FieldDefinition, nestedRefs []schema.NestedSchemaReference, projection *ProjectionConfiguration, originalSchema *schema.SchemaDefinition) error {
	projectedRefs := make([]schema.NestedSchemaReference, 0, len(nestedRefs))

	for _, nestedRef := range nestedRefs {
		// Find the referenced nested schema
		originalNestedSchema, exists := originalSchema.FindNestedSchema(nestedRef.ID)
		if !exists {
			return fmt.Errorf("nested schema reference %s not found in union type", nestedRef.ID)
		}

		// Clone the nested schema for modification
		modifiedNestedSchema := cloneNestedSchemaDefinition(originalNestedSchema)

		// Apply projection if the nested schema is structured
		if originalNestedSchema.IsStructured != nil && *originalNestedSchema.IsStructured {
			if err := applyProjectionToNestedSchema(modifiedNestedSchema, projection, originalSchema); err != nil {
				return fmt.Errorf("failed to apply projection to nested schema %s in union: %w", nestedRef.ID, err)
			}
		}

		// Create a new nested schema ID for the projected version
		projectedSchemaID := generateProjectedSchemaID(nestedRef.ID, projection)

		// Update both the schema name and register it with the new ID
		modifiedNestedSchema.Name = projectedSchemaID

		// Ensure the parent schema has a NestedSchemas map
		if originalSchema.NestedSchemas == nil {
			originalSchema.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
		}

		// Register the modified nested schema using the same ID as the name
		originalSchema.NestedSchemas[projectedSchemaID] = modifiedNestedSchema

		// Create updated reference with ID matching the schema name
		updatedRef := schema.NestedSchemaReference{
			ID:          projectedSchemaID, // This must match NestedSchemaDefinition.Name
			Constraints: nestedRef.Constraints,
			Indexes:     nestedRef.Indexes,
		}
		projectedRefs = append(projectedRefs, updatedRef)
	}

	// Update the field's schema reference slice
	field.Schema = projectedRefs
	return nil
}

// applyProjectionToNestedSchema applies projection configuration to a nested schema definition
func applyProjectionToNestedSchema(nestedSchema *schema.NestedSchemaDefinition, projection *ProjectionConfiguration, parentSchema *schema.SchemaDefinition) error {
	// Handle structured fields map
	if nestedSchema.StructuredFieldsMap != nil {
		return applyProjectionToFieldsMap(nestedSchema.StructuredFieldsMap, projection, parentSchema)
	}

	// Handle structured fields array (conditional fields)
	if nestedSchema.StructuredFieldsArray != nil {
		for i := range nestedSchema.StructuredFieldsArray {
			if nestedSchema.StructuredFieldsArray[i].Fields != nil {
				if err := applyProjectionToFieldsMap(nestedSchema.StructuredFieldsArray[i].Fields, projection, parentSchema); err != nil {
					return fmt.Errorf("failed to apply projection to conditional fields array at index %d: %w", i, err)
				}
			}
		}
		return nil
	}

	return fmt.Errorf("nested schema has no structured fields to project")
}

// applyProjectionToFieldsMap applies projection to a map of field definitions
func applyProjectionToFieldsMap(fields map[string]*schema.FieldDefinition, projection *ProjectionConfiguration, parentSchema *schema.SchemaDefinition) error {
	// Handle exclusions first
	if len(projection.Exclude) > 0 {
		for _, exclude := range projection.Exclude {
			if exclude.Nested != nil {
				// Apply nested projection before excluding the field
				if field, exists := fields[exclude.Name]; exists {
					if err := applyNestedProjection(field, exclude.Nested, parentSchema); err != nil {
						return fmt.Errorf("failed to apply recursive nested exclusion to field %s: %w", exclude.Name, err)
					}
				}
			} else {
				// Simple exclusion - remove the field
				delete(fields, exclude.Name)
			}
		}
	}

	// Handle inclusions (if specified, only included fields remain)
	if len(projection.Include) > 0 {
		newFields := make(map[string]*schema.FieldDefinition)

		for _, include := range projection.Include {
			fieldName := include.Name
			if include.Alias != nil {
				fieldName = *include.Alias
			}

			if originalField, exists := fields[include.Name]; exists {
				// Clone the field to avoid modifying the original
				newField := cloneFieldDefinition(originalField)
				newField.Name = fieldName

				// Handle recursive nested projections
				if include.Nested != nil {
					if err := applyNestedProjection(newField, include.Nested, parentSchema); err != nil {
						return fmt.Errorf("failed to apply recursive nested projection to field %s: %w", include.Name, err)
					}
				}

				newFields[fieldName] = newField
			} else {
				// Field doesn't exist in the original schema
				return fmt.Errorf("field %s specified in projection include does not exist", include.Name)
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
			var fieldDef *schema.FieldDefinition

			if computed.ComputedFieldExpression != nil {
				fieldDef = createComputedFieldDefinitionForNested(computed.ComputedFieldExpression)
			} else if computed.CaseExpression != nil {
				fieldDef = createCaseFieldDefinitionForNested(computed.CaseExpression)
			}

			if fieldDef != nil {
				// Check for field name conflicts
				if _, exists := fields[fieldDef.Name]; exists {
					return fmt.Errorf("computed field %s conflicts with existing field", fieldDef.Name)
				}
				fields[fieldDef.Name] = fieldDef
			}
		}
	}

	return nil
}

// applyProjectionToInlineFields applies projection to inline nested field definitions
func applyProjectionToInlineFields(fields map[string]*schema.FieldDefinition, projection *ProjectionConfiguration, parentSchema *schema.SchemaDefinition) error {
	return applyProjectionToFieldsMap(fields, projection, parentSchema)
}

// createComputedFieldDefinitionForNested creates a field definition for a computed field in nested context
func createComputedFieldDefinitionForNested(expr *ComputedFieldExpression) *schema.FieldDefinition {
	// In nested context, we default to unknown type since we don't have access to function type maps
	return &schema.FieldDefinition{
		Name:        expr.Alias,
		Type:        FieldTypeUnknown,
		Required:    utils.BoolPtr(false),
		Description: utils.StringPtr(fmt.Sprintf("Computed field using function %s", expr.Expression.Function)),
	}
}

// createCaseFieldDefinitionForNested creates a field definition for a case expression in nested context
func createCaseFieldDefinitionForNested(expr *CaseExpression) *schema.FieldDefinition {
	return &schema.FieldDefinition{
		Name:        expr.Alias,
		Type:        FieldTypeUnknown,
		Required:    utils.BoolPtr(false),
		Description: utils.StringPtr("Case expression result"),
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

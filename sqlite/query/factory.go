package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"go.uber.org/zap"
)

// sqliteFactory builds SQLite query components with support for nested scopes.
type sqliteFactory struct {
	// Local state for this scope
	aliases map[string]string
	schemas map[string]*definition.Schema

	// Shared state across all scopes (for parameter numbering)
	globalParamCounter *int

	// Parent scope for correlated subqueries
	parent *sqliteFactory

	// Depth tracking to prevent infinite recursion
	depth int
	logger      *zap.Logger
}

// newSQLiteFactory creates a new root-level factory.
func newSQLiteFactory(logger *zap.Logger) *sqliteFactory {
	counter := 0
	if logger == nil {
		logger = zap.NewNop()
	}
	return &sqliteFactory{
		aliases:            make(map[string]string),
		schemas:            make(map[string]*definition.Schema),
		globalParamCounter: &counter,
		parent:             nil,
		depth:              0,
		logger: logger,
	}
}

// createChildScope creates a new child scope for subqueries.
// The child shares the global parameter counter but has its own alias/schema maps.
func (f *sqliteFactory) createChildScope() *sqliteFactory {
	return &sqliteFactory{
		aliases:            make(map[string]string),
		schemas:            make(map[string]*definition.Schema),
		globalParamCounter: f.globalParamCounter,
		parent:             f,
		depth:              f.depth + 1,
	}
}

// nextParam returns the next parameter placeholder (e.g., "$1", "$2").
// This is shared across all scopes to ensure unique parameter numbering.
func (f *sqliteFactory) nextParam() string {
	*f.globalParamCounter++
	return fmt.Sprintf("$%d", *f.globalParamCounter)
}

// addAlias registers an alias in the current scope.
func (f *sqliteFactory) addAlias(original, alias string) {
	f.aliases[original] = alias
}

// addSchema registers a schema in the current scope.
func (f *sqliteFactory) addSchema(name string, schemaDef *definition.Schema) {
	f.schemas[name] = schemaDef
}

// resolveFieldReference resolves a field reference, checking parent scopes for correlated subqueries.
func (f *sqliteFactory) resolveFieldReference(fieldRef string, schemas map[string]*definition.Schema, qualify ...bool) (string, error) {
	if !isValidIdentifier(fieldRef) {
		return "", ErrSelectUnsupportedFieldReference.WithCause(fmt.Errorf("unsupported field reference: %s", fieldRef))
	}

	parts := strings.Split(fieldRef, ".")

	// Case 1: Single field (e.g., "id", "name")
	if len(parts) == 1 {
		// Try to resolve in current scope first
		resolved, err := f.resolveInCurrentScope(parts[0], schemas, qualify...)
		if err == nil {
			return resolved, nil
		}

		// For correlated subqueries, try parent scope
		if f.parent != nil {
			return f.parent.resolveFieldReference(fieldRef, f.parent.schemas, qualify...)
		}

		// If qualification not requested, just quote the identifier
		if qualify == nil || !qualify[0] {
			return quoteIdentifier(parts[0]), nil
		}

		return "", fmt.Errorf("field %s not found in any scope", fieldRef)
	}

	// Case 2: Multi-part field reference (e.g., "table.field", "_metadata_.version")
	if len(parts) > 1 {
		// First, check if the first part is a table/alias in our schemas
		if schemaDef, ok := schemas[parts[0]]; ok {
			// This is a table.field reference
			if _, fieldDef := schemaDef.FindField(parts[1]); fieldDef != nil && fieldDef.Type.IsComplex() {
				// This is a JSON field - use json_extract for nested access
				if len(parts) > 2 {
					jsonPath := "$." + strings.Join(parts[2:], ".")
					return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0])+"."+quoteIdentifier(parts[1]), jsonPath), nil
				}
			}
			// Regular table.field reference
			quotedParts := make([]string, len(parts))
			for i, part := range parts {
				quotedParts[i] = quoteIdentifier(part)
			}
			return strings.Join(quotedParts, "."), nil
		}

		// Try parent scope for correlated subqueries
		if f.parent != nil {
			if _, ok := f.parent.schemas[parts[0]]; ok {
				return f.parent.resolveFieldReference(fieldRef, f.parent.schemas, qualify...)
			}
		}

		// Case 3: Check if the first part is a field in any of our schemas (for single table contexts)
		// This handles cases like "_metadata_.version" when there's only one table
		for _, schemaDef := range schemas {
			if _, fieldDef := schemaDef.FindField(parts[0]); fieldDef != nil && fieldDef.Type.IsComplex() {
				// This is a JSON field - use json_extract for nested access
				jsonPath := "$." + strings.Join(parts[1:], ".")
				return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0]), jsonPath), nil
			}
		}

		// Check parent scope for JSON fields
		if f.parent != nil {
			for _, schemaDef := range f.parent.schemas {
				if _, fieldDef := schemaDef.FindField(parts[0]); fieldDef != nil && fieldDef.Type.IsComplex() {
					jsonPath := "$." + strings.Join(parts[1:], ".")
					return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0]), jsonPath), nil
				}
			}
		}
	}

	// Default: quote all parts and join with dots
	quotedParts := make([]string, len(parts))
	for i, part := range parts {
		quotedParts[i] = quoteIdentifier(part)
	}
	return strings.Join(quotedParts, "."), nil
}

// resolveInCurrentScope attempts to resolve a field in the current scope only.
func (f *sqliteFactory) resolveInCurrentScope(fieldName string, schemas map[string]*definition.Schema, qualify ...bool) (string, error) {
	if qualify != nil && qualify[0] {
		for alias, schemaDef := range schemas {
			if _, fieldDef := schemaDef.FindField(fieldName); fieldDef != nil {
				// Return table-qualified name: "users"."id"
				return fmt.Sprintf("%s.%s", quoteIdentifier(alias), quoteIdentifier(fieldName)), nil
			}
		}
	}
	return quoteIdentifier(fieldName), nil
}

// maxSubqueryDepth prevents infinite recursion
const maxSubqueryDepth = 10

// checkDepth validates that we haven't exceeded maximum nesting depth.
func (f *sqliteFactory) checkDepth() error {
	if f.depth > maxSubqueryDepth {
		return fmt.Errorf("maximum subquery nesting depth (%d) exceeded", maxSubqueryDepth)
	}
	return nil
}

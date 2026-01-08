package schema

// ============================================================================
// RESULT TYPES
// ============================================================================

// FieldEntry represents a field with its ID and definition
type FieldEntry struct {
	ID         string
	Definition *FieldDefinition
}

// IndexWithPosition represents an index with its array position
type IndexWithPosition struct {
	Index    *IndexDefinition
	Position int
}

// ConstraintLocation describes where a constraint is located in the schema
type ConstraintLocation struct {
	JSONPath string // e.g., "/constraints/2/rules/0"
	Position int    // Position in top-level array
	Depth    int    // Nesting depth (0 = top level)
	Parent   string // Parent group name, if any
}

// ConstraintWithLocation represents a constraint with its location info
type ConstraintWithLocation struct {
	Rule     *ConstraintRule
	Location *ConstraintLocation
}

// FieldComparison contains detailed comparison results between two fields
type FieldComparison struct {
	Exists            bool
	Identical         bool
	NameDifferent     bool
	TypeDifferent     bool
	RequiredDifferent bool
	UniqueDifferent   bool
	SchemaDifferent   bool
	Differences       []string // Human-readable list of differences
}

// IndexRemovalResult contains information about removed indexes
type IndexRemovalResult struct {
	RemovedNames []string
	Count        int
}

// ConstraintRemovalResult contains information about removed constraints
type ConstraintRemovalResult struct {
	RemovedPaths []string
	RemovedNames []string
	Count        int
}

// NestedSchemaRemovalResult contains information about removed nested schemas
type NestedSchemaRemovalResult struct {
	RemovedIDs []string
	Count      int
}

// CleanupResult contains results from cleaning up all orphaned references
type CleanupResult struct {
	Indexes       *IndexRemovalResult
	Constraints   *ConstraintRemovalResult
	NestedSchemas *NestedSchemaRemovalResult
}

// ValidationError represents a validation error in the schema
type ValidationError struct {
	Path    string // JSON path to the problematic element
	Type    string // "index", "constraint", "field", etc.
	Name    string // Name of the element
	Message string // Description of the problem
}

// FieldDiff represents differences in fields between two schemas
type FieldDiff struct {
	Added    map[string]*FieldDefinition
	Removed  map[string]*FieldDefinition
	Modified map[string]*FieldComparison
}

// IndexDiff represents differences in indexes between two schemas
type IndexDiff struct {
	Added    []*IndexDefinition
	Removed  []*IndexDefinition
	Modified map[string]*IndexDefinition
}

// ConstraintDiff represents differences in constraints between two schemas
type ConstraintDiff struct {
	Added    []ConstraintRule
	Removed  []ConstraintRule
	Modified map[string]*ConstraintRule
}

// SchemaStats contains comprehensive statistics about a schema
type SchemaStats struct {
	FieldCount                int
	RequiredFieldCount        int
	OptionalFieldCount        int
	UniqueFieldCount          int
	DeprecatedFieldCount      int
	IndexCount                int
	UniqueIndexCount          int
	PartialIndexCount         int
	ConstraintCount           int
	TotalConstraintCount      int // Including nested
	NestedSchemaCount         int
	OrphanedIndexCount        int
	OrphanedConstraintCount   int
	OrphanedNestedSchemaCount int
	MaxFieldDepth             int
	FieldTypeDistribution     map[FieldType]int
}

// ============================================================================
// FUNCTION TYPES
// ============================================================================

// SchemaProvider provides nested schemas when adding fields
type SchemaProvider func(s *SchemaDefinition) (*NestedSchemaDefinition, []*NestedSchemaDefinition)

// FieldUpdater is a function that modifies a field
type FieldUpdater func(*FieldDefinition) error

// IndexUpdater is a function that modifies an index
type IndexUpdater func(*IndexDefinition) error

// ConstraintUpdater is a function that modifies a constraint
type ConstraintUpdater func(*ConstraintRule) error

// NestedSchemaUpdater is a function that modifies a nested schema
type NestedSchemaUpdater func(*NestedSchemaDefinition) error

// FieldPredicate is a function that tests a field
type FieldPredicate func(id string, field *FieldDefinition) bool

// IndexPredicate is a function that tests an index
type IndexPredicate func(index *IndexDefinition) bool

// ConstraintPredicate is a function that tests a constraint
type ConstraintPredicate func(constraint *ConstraintRule) bool

// FieldMapper is a function that transforms a field
type FieldMapper func(id string, field *FieldDefinition) *FieldDefinition

// FieldVisitor is a function that visits a field during iteration
type FieldVisitor func(id string, field *FieldDefinition) error

// IndexVisitor is a function that visits an index during iteration
type IndexVisitor func(idx int, index *IndexDefinition) error

// ConstraintVisitor is a function that visits a constraint during iteration
type ConstraintVisitor func(idx int, constraint *ConstraintRule) error

// ConstraintWalker is a function that walks constraints recursively
type ConstraintWalker func(constraint *ConstraintRule, depth int) error

// NestedSchemaVisitor is a function that visits a nested schema during iteration
type NestedSchemaVisitor func(id string, schema *NestedSchemaDefinition) error



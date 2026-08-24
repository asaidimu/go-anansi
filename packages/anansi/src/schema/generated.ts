// AUTO-GENERATED from the Anansi meta-schema (core/schema/meta/schema.json)
// via `go run ./cmd/anansi codegen typescript` — do not edit by hand.
// Regenerate and re-commit whenever the meta schema changes.

export type ComparisonOperator = "eq" | "neq" | "lt" | "lte" | "gt" | "gte" | "in" | "nin" | "contains" | "ncontains" | "exists" | "nexists";

export type Constraint = ConstraintMetadata & ConstraintUnion;

export interface ConstraintGroup {
  operator: LogicalOperatorEnum;
  rules: ConstraintUnion[];
}

export interface ConstraintMetadata {
  description?: string;
  name: string;
}

export interface ConstraintRule {
  fields?: String[];
  parameters?: unknown;
  predicate: string;
}

export type ConstraintUnion = ConstraintRule | ConstraintGroup;

export interface FieldDefinition {
  default?: unknown;
  deprecated?: boolean;
  description?: string;
  metadata?: unknown;
  name: string;
  nullable?: boolean;
  required?: boolean;
  schema?: SchemaReference | SchemaReferenceArray | InlineTypeDescriptor;
  type: FieldType;
  unique?: boolean;
}

export type FieldType = "unknown" | "string" | "number" | "integer" | "decimal" | "boolean" | "array" | "enum" | "object" | "record" | "union" | "composite" | "geometry" | "bytes";

export interface IndexCondition {
  field: string;
  operator: ComparisonOperator;
  value: unknown;
}

export interface IndexConditionGroup {
  conditions: IndexConditionUnion[];
  operator: LogicalOperatorEnum;
}

export type IndexConditionUnion = IndexCondition | IndexConditionGroup;

export interface IndexDefinition {
  condition?: IndexCondition | IndexConditionGroup;
  description?: string;
  fields: String[];
  name: string;
  order?: IndexOrder;
  type: IndexType;
  unique?: boolean;
}

export type IndexOrder = "asc" | "desc";

export type IndexType = "normal" | "unique" | "primary" | "spatial" | "fulltext";

export interface InlineTypeDescriptor {
  type: InlineTypeKind;
  values?: Unknown[];
}

export type InlineTypeKind = "string" | "number" | "integer" | "decimal" | "boolean" | "bytes" | "unknown" | "record";

export type LogicalOperatorEnum = "and" | "or" | "not" | "nor" | "xor" | "nand" | "xnor";

export interface NestedSchemaDefinition {
  concrete?: boolean;
  constraints?: Record<string, Constraint>;
  default?: unknown;
  description?: string;
  fields?: Record<string, FieldDefinition>;
  indexes?: Record<string, IndexDefinition>;
  metadata?: unknown;
  name: string;
  schema?: SchemaReference | SchemaReferenceArray | InlineTypeDescriptor;
  type?: FieldType;
  values?: Unknown[];
}

export interface SchemaReference {
  constraints?: Record<string, Constraint>;
  id: string;
  indexes?: Record<string, IndexDefinition>;
}

export type SchemaReferenceArray = SchemaReference[];

export type String = string;

export type Unknown = unknown;

/** Meta-schema defining the structure of schema definitions */
export interface SchemaDefinition {
  constraints?: Record<string, Constraint>;
  description?: string;
  fields?: Record<string, FieldDefinition>;
  indexes?: Record<string, IndexDefinition>;
  metadata?: unknown;
  name: string;
  schemas?: Record<string, NestedSchemaDefinition>;
  version: string;
}

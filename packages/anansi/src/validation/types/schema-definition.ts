/**
 * Schema API Reference
 *
 * This document outlines the schema definition for modeling data structures,
 * supporting atomic schemas with reusable nested definitions, flexible constraints,
 * and indexing. The types are aligned with the canonical meta‑schema (schema.json).
 */

import type { LogicalOperator } from "@asaidimu/query";
import type { InputHint, SchemaHint } from "./hints";

// ============================================================================
// Primitives & Enums
// ============================================================================

/**
 * Basic field types supported by the schema system.
 * Matches the `FieldTypeEnum` in the meta‑schema.
 */
export type FieldType =
  | "unknown"
  | "string"
  | "number"
  | "integer"
  | "decimal"
  | "boolean"
  | "array"
  | "set"
  | "enum"
  | "object"
  | "record"
  | "union"
  | "composite"
  | "geometry"
  | "bytes";

/**
 * Index types for optimizing different query patterns.
 * Matches the `IndexTypeEnum` in the meta‑schema.
 */
export type IndexType =
  | "normal"
  | "unique"
  | "primary"
  | "spatial"
  | "fulltext";

/**
 * Logical operators used in constraint groups and index condition groups.
 * Matches the `LogicalOperatorEnum` in the meta‑schema.
 */
export type LogicalOperatorEnum =
  | "and"
  | "or"
  | "not"
  | "nor"
  | "xor"
  | "nand"
  | "xnor";

/**
 * Comparison operators for index conditions.
 * Matches the `ComparisonOperatorEnum` in the meta‑schema.
 */
export type ComparisonOperator =
  | "eq"
  | "neq"
  | "lt"
  | "lte"
  | "gt"
  | "gte"
  | "in"
  | "nin"
  | "contains"
  | "ncontains"
  | "exists"
  | "nexists";

/**
 * Inline type descriptor kinds.
 * Matches the `InlineTypeEnum` in the meta‑schema.
 */
export type InlineTypeKind =
  | "string"
  | "number"
  | "integer"
  | "decimal"
  | "boolean"
  | "bytes"
  | "unknown"
  | "record";

/**
 * Sort order for index fields.
 * Matches the `IndexOrderEnum` in the meta‑schema.
 */
export type IndexOrder = "asc" | "desc";

// ============================================================================
// Metadata
// ============================================================================

/**
 * Shared metadata properties for most schema components.
 */
export interface BaseMetadata extends Record<string, any> {
  description?: string;
  /** Arbitrary key‑value pairs for implementation‑specific metadata. */
  metadata?: Record<string, unknown>;
}

/**
 * Metadata for components that also have a human‑readable name.
 */
export interface NamedMetadata extends BaseMetadata {
  name: string;
}

// ============================================================================
// References
// ============================================================================

/**
 * A reference to a schema component (schema, constraint, index) by its unique ID.
 */
export interface ResourceReference {
  id: string;
}

/**
 * Reference to another schema (e.g., for `object`, `array`, `union` types).
 * May include local overrides for indexes and constraints.
 */
export interface SchemaReference extends BaseMetadata {
  id: string;
  indexes?: Record<string, IndexDefinition>;
  constraints?: Record<string, Constraint | ConstraintGroup>;
}

/**
 * Inline type descriptor – used when a simple type is defined directly
 * without a separate schema reference.
 */
export interface InlineTypeDescriptor extends BaseMetadata {
  type: InlineTypeKind;
  values?: Array<string | number>; // For enum‑like inline definitions
}

// ============================================================================
// Fields
// ============================================================================

/**
 * Defines a field within a schema.
 * Matches the `Field` schema in the meta‑schema.
 */
export interface FieldDefinition<T = unknown> extends NamedMetadata {
  type: FieldType;
  required?: boolean;
  nullable?: boolean;
  deprecated?: boolean;
  unique?: boolean;
  default?: T;
  /**
   * The `schema` property is used for complex types:
   * - For `array`/`set`: a single `SchemaReference` pointing to the item schema.
   * - For `object`: a single `SchemaReference` pointing to the object schema.
   * - For `union`: an array of `SchemaReference`s.
   * - For `enum`: an `InlineTypeDescriptor` (or `values` on the field itself).
   * - For primitive overrides: an `InlineTypeDescriptor`.
   */
  schema?: SchemaReference | SchemaReference[] | InlineTypeDescriptor;
  // Legacy alias – deprecated in favour of `schema`
  /** @deprecated Use `schema` instead. */
  nestedSchema?: SchemaReference;
  hint?: {
    input: InputHint;
  };
}

// ============================================================================
// Indexes
// ============================================================================

/**
 * A single index condition (leaf node).
 * Matches the `IndexCondition` schema in the meta‑schema.
 */
export interface IndexCondition extends BaseMetadata {
  operator: ComparisonOperator;
  field: string;
  value: unknown;
}

/**
 * A group of index conditions combined with a logical operator.
 * Matches the `IndexConditionGroup` schema in the meta‑schema.
 */
export interface IndexConditionGroup extends BaseMetadata {
  operator: LogicalOperatorEnum;
  conditions: Array<IndexCondition | IndexConditionGroup>;
}

/**
 * Defines an index for optimizing queries or enforcing uniqueness.
 * Matches the `Index` schema in the meta‑schema.
 */
export interface IndexDefinition extends NamedMetadata {
  unique?: boolean;
  type: IndexType;
  fields: string[];
  order?: IndexOrder;
  condition?: IndexCondition | IndexConditionGroup;
}

// ============================================================================
// Constraints
// ============================================================================

/**
 * A predicate‑based constraint rule.
 * Matches the `ConstraintRule` schema in the meta‑schema.
 */
export interface ConstraintRule extends NamedMetadata {
  predicate: string;
  parameters?: unknown;
  fields?: string[];
}

/**
 * A group of constraints combined with a logical operator.
 * Matches the `ConstraintGroup` schema in the meta‑schema.
 */
export interface ConstraintGroup extends NamedMetadata {
  operator: LogicalOperatorEnum;
  rules: Array<ConstraintRule | ConstraintGroup>;
}

/**
 * A constraint can be either a single rule or a logical group of rules.
 * Matches the `Constraint` composite in the meta‑schema.
 */
export type Constraint = ConstraintRule | ConstraintGroup;

// ============================================================================
// Nested Schemas
// ============================================================================

/**
 * A reusable nested schema definition.
 * Matches the `NestedSchema` schema in the meta‑schema.
 *
 * A nested schema can be either:
 * - An **object schema** (with `fields`, and optionally `indexes`, `constraints`),
 * - A **primitive type alias** (with `type` and optionally `default`, `values`, `schema`).
 */
export type NestedSchemaDefinition<T = unknown> = NamedMetadata & {
  indexes?: Record<string, IndexDefinition>;
  constraints?: Record<string, Constraint>;
} & (
    | {
        /**
         * Object schema: defines the fields of the nested object.
         * The `fields` record maps field IDs to `FieldDefinition`s.
         */
        fields: Record<string, FieldDefinition>;
      }
    | {
        /**
         * Primitive type alias.
         * The `type` must not be `"object"``.
         */
        type: Exclude<FieldType, "object">;
        default?: T;
        values?: Array<string | number>; // For `enum` type
        schema?: SchemaReference | SchemaReference[]; // For `array`,`set`,`union`,`composite` or `record` base type
      }
  );

// ============================================================================
// Top‑Level Schema
// ============================================================================

/**
 * Defines a complete schema.
 * Matches the top‑level `Schema` structure in the meta‑schema.
 */
export interface SchemaDefinition extends NamedMetadata {
  version: string;
  /** Map of field IDs to field definitions. */
  fields: Record<string, FieldDefinition>;
  /** Map of index IDs to index definitions. */
  indexes?: Record<string, IndexDefinition>;
  /** Map of constraint IDs to constraint definitions. */
  constraints?: Record<string, Constraint>;
  /** Map of nested schema IDs to nested schema definitions. */
  schemas?: Record<string, NestedSchemaDefinition>;

  // === Extensions (not part of the core meta‑schema but supported by the tooling) ===
  /**
   * Optional data migration definitions.
   * @extension
   */
  migrations?: Array<Migration>;
  /**
   * UI/input hints.
   * @extension
   */
  hints?: SchemaHint;
  /**
   * Mock data generator.
   * @extension
   */
  mock?: <T>(
    /** Lazy-typed: @faker-js/faker is a dev-only peer; kept out of bundle. */
    faker: unknown,
  ) => Generator<T, void, unknown>;
  /**
   * Domain dependencies.
   * @extension
   */
  dependencies?: string[];
  /** @deprecated Use `schemas` instead. */
  nestedSchemas?: Record<string, NestedSchemaDefinition>;
  /** @deprecated Use top‑level `indexes`, `constraints`, `schemas` directly. */
  registry?: {
    schemas?: Record<string, NestedSchemaDefinition>;
    constraints?: Record<string, Constraint>;
    indexes?: Record<string, IndexDefinition>;
  };
}

// ============================================================================
// Migrations (extensions)
// ============================================================================

/**
 * Implements Partial Update Semantics:
 * - `undefined`: No change to the property.
 * - `null`: Clear/remove the property (if allowed by NonNullable).
 * - `value`: Set the property to the provided value.
 */
export type Patch<T, NonNullable extends keyof T = never> = {
  [K in keyof T]?: K extends NonNullable ? T[K] : T[K] | null;
};

export type FieldPatch = Patch<FieldDefinition, "name" | "type">;
export type ConstraintPatch = Patch<Constraint, "name">;
export type IndexPatch = Patch<IndexDefinition, "name">;
export type SchemaReferencePatch = Patch<SchemaReference, "id">;

export type SchemaChange =
  | { type: "modifyProperty"; id: keyof NamedMetadata | "version"; value: any }
  | { type: "addField"; id: string; definition: FieldDefinition }
  | { type: "removeField"; id: string }
  | { type: "modifyField"; id: string; changes: FieldPatch }
  | { type: "addIndex"; id: string; definition: IndexDefinition }
  | { type: "removeIndex"; id: string }
  | { type: "modifyIndex"; id: string; changes: IndexPatch }
  | { type: "addConstraint"; id: string; constraint: Constraint }
  | { type: "removeConstraint"; id: string }
  | { type: "modifyConstraint"; id: string; changes: ConstraintPatch }
  | { type: "addSchema"; id: string; definition: NestedSchemaDefinition }
  | { type: "removeSchema"; id: string }
  | { type: "modifySchema"; id: string; changes: Array<SchemaChange> };

export type TransformFunction<Initial, Next> = (
  ctx: any,
  data: Initial,
) => Next | Promise<Next>;

export interface DataTransform<Initial, Next> {
  forward: TransformFunction<Initial, Next>;
  backward: TransformFunction<Next, Initial>;
}

export interface Migration<Initial = any, Next = any> {
  id: string;
  version: { source: string; target?: string };
  changes: SchemaChange[];
  description: string;
  rollback?: SchemaChange[];
  transform: string | DataTransform<Initial, Next>;
  createdAt: string;
  dependencies?: string[];
  checksum: string;
}

// Re-export LogicalOperator from the external package for convenience
export type { LogicalOperator };

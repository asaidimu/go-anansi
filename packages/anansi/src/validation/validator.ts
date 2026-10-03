/**
 * validator.ts
 *
 * Public API for the schema document validator.
 *
 * Usage:
 *
 *   const validator = DocumentValidator.create(schema, predicateMap);
 *   const issues    = await validator.validate(document);
 *   const partial   = await validator.validatePartial(document);
 *   const loose     = await validator.validateLoose(document);
 */

import type { SchemaDefinition } from "../schema/generated.ts";
import type { Issue, PredicateMap, ValidationConfig } from "./validator-types.ts";
import { defaultValidationConfig } from "./validator-types.ts";
import { ValidationGraph, BuildContext } from "./graph.ts";

export type { Issue, PredicateMap, ValidationConfig };
export { defaultValidationConfig };
export type { SchemaDefinition };

// ---------------------------------------------------------------------------
// DocumentValidator
// ---------------------------------------------------------------------------

export class DocumentValidator {
  private readonly fmap: PredicateMap;
  private readonly graph: ValidationGraph;
  private readonly config: ValidationConfig;

  protected constructor(
    graph: ValidationGraph,
    fmap: PredicateMap,
    config: ValidationConfig,
  ) {
    this.graph = graph;
    this.fmap = fmap;
    this.config = config;
  }

  /**
   * Full validation: reports missing required fields, unexpected fields,
   * type mismatches, and constraint violations.
   *
   * Accepts `unknown` (Go parity: `Validate(doc any)`) — reporting bad
   * shapes is this type's job, not the caller's.
   */
  async validate(document: unknown): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document as Record<string, unknown>,
      "strict",
      this.config.maxDepth,
      document as Record<string, unknown>,
    );
  }

  /**
   * Partial-strict validation: suppresses REQUIRED_FIELD_MISSING issues.
   * Use when validating partial updates (e.g. PATCH payloads).
   */
  async validatePartial(document: unknown): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document as Record<string, unknown>,
      "partialStrict",
      this.config.maxDepth,
      document as Record<string, unknown>,
    );
  }

  /**
   * Loose validation: suppresses both REQUIRED_FIELD_MISSING and
   * UNEXPECTED_FIELD issues.  Validates only present fields for type and
   * constraints.
   */
  async validateLoose(document: unknown): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document as Record<string, unknown>,
      "loose",
      this.config.maxDepth,
      document as Record<string, unknown>,
    );
  }

  // ---------------------------------------------------------------------------
  // Factory
  // ---------------------------------------------------------------------------

  /**
   * Builds a DocumentValidator from a SchemaDefinition.
   *
   * This compiles the schema into a validation DAG exactly once; subsequent
   * calls to `validate()` / `validatePartial()` / `validateLoose()` traverse
   * the pre-compiled graph and are fast. Construction is synchronous (Go
   * parity); only traversal is async, because predicates may be.
   *
   * Throws if the schema contains structural errors that prevent graph construction
   * (e.g. dangling schema references, composite parts with unsupported types).
   */
  public static create(
    schema: SchemaDefinition,
    fmap: PredicateMap,
    config: ValidationConfig = defaultValidationConfig(),
  ): DocumentValidator {
    const graph = new ValidationGraph();
    const buildCtx = new BuildContext();
    const addedConstraints = new Map<string, boolean>();

    graph.buildFromSchema(
      schema,
      "",
      [],
      addedConstraints,
      null,
      {},
      schema,
      buildCtx,
      false,
      false,
    );

    graph.finalize();

    return new DocumentValidator(graph, fmap, config);
  }
}

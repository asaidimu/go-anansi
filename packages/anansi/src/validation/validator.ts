/**
 * validator.ts
 *
 * Public API for the schema document validator.
 *
 * Usage:
 *
 *   const validator = await newDocumentValidator(schema, predicateMap);
 *   const issues    = await validator.validate(document);
 *   const partial   = await validator.validatePartial(document);
 *   const loose     = await validator.validateLoose(document);
 */

import type { SchemaDefinition } from "./types/schema-definition";
import type { Issue, PredicateMap, ValidationConfig } from "./types/validator";
import { defaultValidationConfig } from "./types/validator";
import { ValidationGraph, BuildContext } from "./graph";

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
   */
  async validate(document: Record<string, unknown>): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document,
      "strict",
      this.config.maxDepth,
      document,
    );
  }

  /**
   * Partial-strict validation: suppresses REQUIRED_FIELD_MISSING issues.
   * Use when validating partial updates (e.g. PATCH payloads).
   */
  async validatePartial(document: Record<string, unknown>): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document,
      "partialStrict",
      this.config.maxDepth,
      document,
    );
  }

  /**
   * Loose validation: suppresses both REQUIRED_FIELD_MISSING and
   * UNEXPECTED_FIELD issues.  Validates only present fields for type and
   * constraints.
   */
  async validateLoose(document: Record<string, unknown>): Promise<Issue[]> {
    return this.graph.traverse(
      this.fmap,
      document,
      "loose",
      this.config.maxDepth,
      document,
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
   * the pre-compiled graph and are fast.
   *
   * Throws if the schema contains structural errors that prevent graph construction
   * (e.g. dangling schema references, composite parts with unsupported types).
   */
  public static async create(
    schema: SchemaDefinition,
    fmap: PredicateMap,
    config: ValidationConfig = defaultValidationConfig(),
  ) {
    const graph = new ValidationGraph();
    const buildCtx = new BuildContext();
    const addedConstraints = new Map<string, boolean>();

    await graph.buildFromSchema(
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

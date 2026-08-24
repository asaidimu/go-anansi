/**
 * graph.ts
 *
 * ValidationGraph: DAG construction, topological sort, and traversal.
 * Mirrors validator.go's ValidationGraph, buildContext, and all build* methods.
 */

import {
  type ConstraintScope,
  type ConstraintSpecificity,
  type EffectiveConstraint,
  type Issue,
  type PredicateMap,
  type SchemaConstraintMap,
  type ValidationContext,
  type ValidationMode,
  type ValidatorConstraint,
  type ValidatorConstraintGroup,
  FieldSchemaReference,
  SpecificityNestedSchema,
  SpecificitySchemaReference,
  SpecificityTopLevel,
  toConstraintMap,
  toValidatorConstraint,
} from "./types/validator";

import {
  buildPathAndParts,
  getNodeValue,
  resolveConstraintFieldPaths,
} from "./utils";

import type {
  FieldDefinition,
  FieldType,
  NestedSchemaDefinition,
  SchemaDefinition,
  SchemaReference,
} from "./types/schema-definition";

import {
  type ValidationNode,
  ArrayValidationNode,
  ConstraintGroupNode,
  ConstraintNode,
  EnumValidationNode,
  GeometryValidationNode,
  NestedSchemaNode,
  RecordValidationNode,
  RecursionMarkerNode,
  RequiredFieldNode,
  SetValidationNode,
  TypeCheckNode,
  UnexpectedFieldsNode,
  UnionValidationNode,
} from "./nodes";

// ---------------------------------------------------------------------------
// DFS state constants
// ---------------------------------------------------------------------------
const DFS_UNVISITED = 0;
const DFS_VISITING = 1;
const DFS_VISITED = 2;

// ---------------------------------------------------------------------------
// ConstraintRegistry — applies override rules based on specificity
// ---------------------------------------------------------------------------

class ConstraintRegistry {
  private readonly constraints: Map<string, EffectiveConstraint> = new Map();

  add(
    name: string,
    constraint: ValidatorConstraint,
    specificity: ConstraintSpecificity,
    basePath: string,
    scope: ConstraintScope,
  ): void {
    const existing = this.constraints.get(name);
    // Higher or equal specificity wins (last-write for same specificity).
    if (!existing || specificity >= existing.specificity) {
      this.constraints.set(name, { constraint, specificity, basePath, scope });
    }
  }

  getEffective(): EffectiveConstraint[] {
    return Array.from(this.constraints.values());
  }
}

function newConstraintRegistry(): ConstraintRegistry {
  return new ConstraintRegistry();
}

// ---------------------------------------------------------------------------
// BuildContext — recursion tracking and sub-graph caching
// ---------------------------------------------------------------------------

class BuildContext {
  /** Ref-count of schemas currently being built. */
  private readonly buildingSchemas: Map<string, number> = new Map();
  /** Cache of pre-built recursive graphs keyed by schemaID + constraint hash. */
  private readonly recursiveGraphCache: Map<string, ValidationGraph> =
    new Map();

  isRecursive(schemaID: string): boolean {
    return (this.buildingSchemas.get(schemaID) ?? 0) > 0;
  }

  markBuilding(schemaID: string): void {
    this.buildingSchemas.set(
      schemaID,
      (this.buildingSchemas.get(schemaID) ?? 0) + 1,
    );
  }

  unmarkBuilding(schemaID: string): void {
    const count = (this.buildingSchemas.get(schemaID) ?? 0) - 1;
    if (count <= 0) {
      this.buildingSchemas.delete(schemaID);
    } else {
      this.buildingSchemas.set(schemaID, count);
    }
  }

  makeGraphCacheKey(
    schemaID: string,
    constraints: SchemaConstraintMap,
  ): string {
    const ids = Object.keys(constraints).sort();
    if (ids.length === 0) return schemaID;
    return `${schemaID}:${ids.join(",")}`;
  }

  async getOrBuildRecursiveGraph(
    schemaID: string,
    schemaDef: NestedSchemaDefinition,
    instanceConstraints: SchemaConstraintMap,
    topLevelSchema: SchemaDefinition,
  ): Promise<ValidationGraph> {
    const cacheKey = this.makeGraphCacheKey(schemaID, instanceConstraints);
    const cached = this.recursiveGraphCache.get(cacheKey);
    if (cached) return cached;

    const graph = new ValidationGraph();
    // Optimistically cache to handle self-references within this graph.
    this.recursiveGraphCache.set(cacheKey, graph);

    this.markBuilding(schemaID);
    try {
      // Build a synthetic Schema from the nested schema definition.
      const effectiveSchema = nestedDefToSchemaDefinition(
        schemaDef,
        topLevelSchema,
      );

      await graph.buildFromSchema(
        effectiveSchema,
        "",
        [],
        new Map<string, boolean>(),
        schemaDef,
        instanceConstraints,
        topLevelSchema,
        this,
        false,
        false,
      );

      graph.finalize();
    } catch (err) {
      this.recursiveGraphCache.delete(cacheKey);
      throw err;
    } finally {
      this.unmarkBuilding(schemaID);
    }

    return graph;
  }
}

// ---------------------------------------------------------------------------
// Helpers for converting NestedSchemaDefinition → SchemaDefinition-like
// ---------------------------------------------------------------------------

/**
 * Wraps a `NestedSchemaDefinition` into a minimal `SchemaDefinition`-compatible
 * object that `buildFromSchema` can consume.  Mirrors Go's `&Schema{BaseSchema: ...}`.
 */
function nestedDefToSchemaDefinition(
  nsd: NestedSchemaDefinition,
  topLevel: SchemaDefinition,
): SchemaDefinition {
  const fields = "fields" in nsd ? nsd.fields : {};
  const constraints = nsd.constraints ?? {};
  const result: SchemaDefinition = {
    name: nsd.name,
    version: "0",
    fields: fields as Record<string, FieldDefinition>,
    constraints: constraints as Record<string, any>,
  };
  if (nsd.description !== undefined) result.description = nsd.description;
  if (topLevel.schemas !== undefined) result.schemas = topLevel.schemas;
  return result;
}

/**
 * Returns the effective FieldType for a NestedSchemaDefinition.
 * Mirrors Go's `getNestedSchemaEffectiveType`.
 */
function getNestedSchemaEffectiveType(nsd: NestedSchemaDefinition): FieldType {
  if ("type" in nsd && nsd.type) return nsd.type as FieldType;
  if ("fields" in nsd && nsd.fields && Object.keys(nsd.fields).length > 0)
    return "object";
  return "unknown";
}

/**
 * Returns true if the FieldType is a "complex" type whose structural validation
 * is handled by a dedicated node rather than a TypeCheckNode.
 * Mirrors Go's `FieldType.IsComplex()`.
 */
function isComplexType(t: FieldType): boolean {
  switch (t) {
    case "object":
    case "array":
    case "set":
    case "record":
    case "union":
    case "composite":
    case "enum":
    case "geometry":
      return true;
    default:
      return false;
  }
}

// ---------------------------------------------------------------------------
// Constraint collection helpers
// ---------------------------------------------------------------------------

/**
 * Recursively collects all field names referenced by any ConstraintRule
 * within a ValidatorConstraintGroup.
 */
function collectGroupFieldNames(g: ValidatorConstraintGroup): string[] {
  const names: string[] = [];
  for (const rule of g.rules) {
    if (rule.kind === "rule") {
      names.push(...rule.fields);
    } else if (rule.kind === "group") {
      names.push(...collectGroupFieldNames(rule));
    }
  }
  return names;
}

/**
 * Extracts top-level constraints that apply to `fieldPath` or any sub-path.
 * Mirrors Go's `getTopLevelConstraintsForPath`.
 */
function getTopLevelConstraintsForPath(
  topLevelSchema: SchemaDefinition,
  fieldPath: string,
): SchemaConstraintMap {
  const result: SchemaConstraintMap = {};
  const rawConstraints = topLevelSchema.constraints ?? {};

  const fieldMatches = (fieldName: string): boolean =>
    fieldName === fieldPath || fieldName.startsWith(fieldPath + ".");

  for (const [id, rawC] of Object.entries(rawConstraints)) {
    const c = toValidatorConstraint(rawC as any);

    if (c.kind === "rule") {
      if (c.fields.some(fieldMatches)) {
        result[id] = c;
      }
    } else if (c.kind === "group") {
      const groupFields = collectGroupFieldNames(c);
      if (groupFields.some(fieldMatches)) {
        result[id] = c;
      }
    }
  }

  return result;
}

/**
 * Merges constraints from all three specificity levels and returns the
 * effective set after applying override rules.
 * Mirrors Go's `collectConstraints`.
 */
function collectConstraints(
  nestedSchemaConstraints: SchemaConstraintMap,
  schemaRefConstraints: SchemaConstraintMap,
  topLevelConstraints: SchemaConstraintMap,
  basePath: string,
): EffectiveConstraint[] {
  const registry = newConstraintRegistry();
  const scope: ConstraintScope = basePath === "" ? "global" : "recursive";

  for (const [, c] of Object.entries(nestedSchemaConstraints)) {
    registry.add(c.name, c, SpecificityNestedSchema, basePath, scope);
  }
  for (const [, c] of Object.entries(schemaRefConstraints)) {
    registry.add(c.name, c, SpecificitySchemaReference, basePath, scope);
  }
  for (const [, c] of Object.entries(topLevelConstraints)) {
    registry.add(c.name, c, SpecificityTopLevel, basePath, scope);
  }

  return registry.getEffective();
}

// ---------------------------------------------------------------------------
// Enum value lookup builder
// ---------------------------------------------------------------------------

function buildEnumLookup(values: Array<string | number>): {
  lookup: Map<unknown, true>;
  complex: unknown[];
} {
  const lookup = new Map<unknown, true>();
  const complex: unknown[] = [];
  for (const v of values) {
    if (v === null || v === undefined) continue;
    if (
      typeof v === "string" ||
      typeof v === "number" ||
      typeof v === "boolean"
    ) {
      lookup.set(v, true);
    } else {
      complex.push(v);
    }
  }
  return { lookup, complex };
}

// ---------------------------------------------------------------------------
// Composite vocabulary helper
// ---------------------------------------------------------------------------

/**
 * Collects the union of all top-level field names reachable from every
 * composite part.  Mirrors Go's `collectCompositeVocabulary`.
 */
function collectCompositeVocabulary(
  refs: SchemaReference[],
  topLevelSchema: SchemaDefinition,
): Set<string> {
  const vocab = new Set<string>();

  function walk(nsd: NestedSchemaDefinition): void {
    const effectiveType = getNestedSchemaEffectiveType(nsd);
    if (effectiveType === "object" || effectiveType === "unknown") {
      if ("fields" in nsd && nsd.fields) {
        for (const fieldDef of Object.values(nsd.fields) as any) {
          vocab.add(fieldDef.name);
        }
      }
    } else if (effectiveType === "union" || effectiveType === "composite") {
      if (!("schema" in nsd) || !nsd.schema) return;
      const schemaRef = FieldSchemaReference.fromFieldSchema(nsd.schema as any);
      if (!schemaRef.isMultiple()) return;
      for (const vRef of schemaRef.asMultiple()) {
        const variantDef = topLevelSchema.schemas?.[vRef.id];
        if (variantDef) walk(variantDef);
      }
    }
  }

  for (const ref of refs) {
    const nsd = topLevelSchema.schemas?.[ref.id];
    if (nsd) walk(nsd);
  }

  return vocab;
}

// ---------------------------------------------------------------------------
// ValidationGraph
// ---------------------------------------------------------------------------

/** Internal tracking node used only during graph construction. */
interface TrackNode {
  id: number;
  deps: number[];
}

export class ValidationGraph {
  private nodes: Map<number, ValidationNode> = new Map();
  private dependencies: Map<number, number[]> = new Map();
  private visitedState: Map<number, number> = new Map();
  private executionOrder: number[] = [];
  private nextNodeID = 1;
  /** Paths explicitly marked nullable:true (null is a legal value). */
  readonly nullablePaths: Set<string> = new Set();

  buildNodeID(): number {
    return this.nextNodeID++;
  }

  addNode(node: ValidationNode): void {
    const id = node.getID();
    if (this.nodes.has(id)) return;
    this.nodes.set(id, node);
    this.dependencies.set(id, node.getDependencies());
  }

  // ── Node factory methods ──────────────────────────────────────────────────

  private createUnexpectedFieldsNode(
    path: string,
    pathParts: string[],
    expectedFields: Set<string>,
    deps: number[] = [],
  ): UnexpectedFieldsNode {
    return new UnexpectedFieldsNode(
      this.buildNodeID(),
      path,
      pathParts,
      expectedFields,
      deps,
    );
  }

  private createRequiredFieldNode(
    fieldPath: string,
    fieldPathParts: string[],
    fieldName: string,
    parentPath: string,
    parentPathParts: string[],
    deps: number[] = [],
  ): RequiredFieldNode {
    return new RequiredFieldNode(
      this.buildNodeID(),
      fieldPath,
      fieldPathParts,
      fieldName,
      parentPath,
      parentPathParts,
      deps,
    );
  }

  private createTypeCheckNode(
    fieldPath: string,
    fieldPathParts: string[],
    expected: FieldType,
    deps: number[] = [],
  ): TypeCheckNode {
    return new TypeCheckNode(
      this.buildNodeID(),
      fieldPath,
      fieldPathParts,
      expected,
      deps,
    );
  }

  private createCompletionNode(
    path: string,
    pathParts: string[],
    deps: number[],
  ): NestedSchemaNode {
    return new NestedSchemaNode(this.buildNodeID(), path, pathParts, deps);
  }

  // ── Topological sort ─────────────────────────────────────────────────────

  finalize(): void {
    this.visitedState = new Map();
    const order: number[] = [];
    let hasCycle = false;

    const visit = (nodeID: number): void => {
      if (hasCycle) return;

      const state = this.visitedState.get(nodeID) ?? DFS_UNVISITED;
      if (state === DFS_VISITING) {
        hasCycle = true;
        return;
      }
      if (state === DFS_VISITED) return;

      this.visitedState.set(nodeID, DFS_VISITING);
      for (const depID of this.dependencies.get(nodeID) ?? []) {
        visit(depID);
      }
      this.visitedState.set(nodeID, DFS_VISITED);
      order.push(nodeID);
    };

    for (const nodeID of this.nodes.keys()) {
      if ((this.visitedState.get(nodeID) ?? DFS_UNVISITED) === DFS_UNVISITED) {
        visit(nodeID);
        if (hasCycle) {
          throw new Error(
            `Circular dependency detected in validation graph at node ${nodeID}`,
          );
        }
      }
    }

    this.executionOrder = order;
  }

  // ── Traversal ─────────────────────────────────────────────────────────────

  async traverse(
    fmap: PredicateMap,
    document: Record<string, unknown>,
    mode: ValidationMode,
    maxDepth: number,
    originalRoot: Record<string, unknown>,
  ): Promise<Issue[]> {
    const ctx: ValidationContext = {
      originalRoot,
      rootData: document,
      data: document,
      functionMap: fmap,
      maxDepth,
      mode,
      visited: new Map(),
      issues: [],
    };

    for (const nodeID of this.executionOrder) {
      const node = this.nodes.get(nodeID)!;
      const deps = this.dependencies.get(nodeID) ?? [];

      // Propagate skip: if any dependency failed, skip this node.
      let shouldSkip = false;
      for (const depID of deps) {
        const depResult = ctx.visited.get(depID);
        if (depResult !== undefined && !depResult) {
          ctx.visited.set(nodeID, true); // mark as skipped (success=true, won't emit issues)
          shouldSkip = true;
          break;
        }
      }
      if (shouldSkip) continue;

      const nodePathParts = node.getPathParts();
      const nodePath = node.getPath();

      const [val, keyExists] = getNodeValue(ctx, nodePathParts);
      const isRequiredNode = node instanceof RequiredFieldNode;

      const isNullable = this.nullablePaths.has(nodePath);

      // Absent/undefined values: nothing to validate for optional fields
      // (required-ness is owned by RequiredFieldNode, which still executes).
      if (!keyExists || val === undefined) {
        if (!isRequiredNode) {
          ctx.visited.set(nodeID, true);
          continue;
        }
      }

      // Explicit null:
      //   - satisfies a required field only when the field is nullable;
      //   - skips type/constraint nodes when nullable;
      //   - otherwise falls through so the type check reports the mismatch.
      if (val === null) {
        if (isRequiredNode) continue; // presence satisfies "required"
        if (isNullable) {
          ctx.visited.set(nodeID, true);
          continue;
        }
        // not nullable → let the type node reject null (TYPE_MISMATCH)
      }

      const result = await node.execute(ctx);
      ctx.visited.set(nodeID, result.success);

      if (result && !result.skipped && result.issues.length > 0) {
        for (const issue of result.issues) {
          switch (mode) {
            case "strict":
              ctx.issues.push(issue);
              break;
            case "partialStrict":
              if (issue.code !== "REQUIRED_FIELD_MISSING") {
                ctx.issues.push(issue);
              }
              break;
            case "loose":
              if (
                issue.code !== "REQUIRED_FIELD_MISSING" &&
                issue.code !== "UNEXPECTED_FIELD"
              ) {
                ctx.issues.push(issue);
              }
              break;
          }
        }
      }
    }

    return ctx.issues;
  }

  // ── Main build entry point ─────────────────────────────────────────────────

  /**
   * Builds the validation graph from a SchemaDefinition.
   * Returns the root TrackNodes (for wiring dependencies).
   * Mirrors Go's `graph.buildFromSchema`.
   */
  async buildFromSchema(
    schema: SchemaDefinition,
    basePath: string,
    baseParts: string[],
    addedConstraints: Map<string, boolean>,
    nsd: NestedSchemaDefinition | null,
    schemaRefConstraints: SchemaConstraintMap,
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    skipUnexpectedCheck: boolean,
    skipUnexpectedForObjects: boolean,
  ): Promise<TrackNode[]> {
    const rootNodes: TrackNode[] = [];

    // Determine the field set to process.
    const fieldsToProcess =
      nsd && "fields" in nsd && nsd.fields ? nsd.fields : schema.fields;

    // 1. Unexpected-fields guard
    let unexpectedNode: UnexpectedFieldsNode | null = null;
    if (!skipUnexpectedCheck) {
      const expectedFields = new Set<string>(
        Object.values(fieldsToProcess).map((f: any) => f.name),
      );
      unexpectedNode = this.createUnexpectedFieldsNode(
        basePath,
        baseParts,
        expectedFields,
      );
      this.addNode(unexpectedNode);
      rootNodes.push({ id: unexpectedNode.getID(), deps: [] });
    }

    const baseDepsForFields: number[] = [];

    // 2. Field nodes
    const allFieldNodes: TrackNode[] = [];
    for (const fieldDef of Object.values(fieldsToProcess)) {
      const fieldNodes = await this.buildFieldNodes(
        fieldDef as FieldDefinition,
        basePath,
        baseParts,
        baseDepsForFields,
        schema,
        addedConstraints,
        topLevelSchema,
        buildCtx,
        skipUnexpectedForObjects,
      );
      allFieldNodes.push(...fieldNodes);
    }

    // 3. Constraints
    const nestedConstraints: SchemaConstraintMap = nsd
      ? toConstraintMap(nsd.constraints as any)
      : toConstraintMap(schema.constraints as any);

    const topLevelConstraints: SchemaConstraintMap =
      topLevelSchema && basePath !== ""
        ? getTopLevelConstraintsForPath(topLevelSchema, basePath)
        : {};

    const effectiveConstraints = collectConstraints(
      nestedConstraints,
      schemaRefConstraints,
      topLevelConstraints,
      basePath,
    );

    const constraintDeps = allFieldNodes.map((n) => n.id);
    const schemaConstraintIDs = this.buildFromEffectiveConstraints(
      effectiveConstraints,
      baseParts,
      constraintDeps,
      addedConstraints,
    );

    // 4. Completion node (synchronization barrier after all constraints)
    if (schemaConstraintIDs.length > 0) {
      const completionNode = this.createCompletionNode(
        basePath,
        baseParts,
        schemaConstraintIDs,
      );
      this.addNode(completionNode);
      rootNodes.push({ id: completionNode.getID(), deps: schemaConstraintIDs });
    }

    return rootNodes;
  }

  // ── Field node builders ───────────────────────────────────────────────────

  private async buildFieldNodes(
    fieldDef: FieldDefinition,
    basePath: string,
    baseParts: string[],
    baseDeps: number[],
    sc: SchemaDefinition,
    addedConstraints: Map<string, boolean>,
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    skipUnexpectedForObjects: boolean,
  ): Promise<TrackNode[]> {
    const [fieldPath, fieldPathParts] = buildPathAndParts(
      basePath,
      baseParts,
      fieldDef.name,
    );

    // Track nullable paths: `nullable:true` means null is a legal value and
    // type/constraint nodes are skipped for it. Absence of the flag defaults
    // to NOT nullable — null on such fields surfaces as a type error.
    if (fieldDef.nullable !== false) {
      this.nullablePaths.add(fieldPath);
    }

    let currentDeps = baseDeps;
    const nodes: TrackNode[] = [];

    // Required check
    if (fieldDef.required) {
      const reqNode = this.createRequiredFieldNode(
        fieldPath,
        fieldPathParts,
        fieldDef.name,
        basePath,
        baseParts,
        currentDeps,
      );
      this.addNode(reqNode);
      currentDeps = [reqNode.getID()];
      nodes.push({ id: reqNode.getID(), deps: currentDeps });
    }

    // Type check (only for non-complex types)
    if (!isComplexType(fieldDef.type)) {
      const typeNode = this.createTypeCheckNode(
        fieldPath,
        fieldPathParts,
        fieldDef.type,
        currentDeps,
      );
      this.addNode(typeNode);
      currentDeps = [typeNode.getID()];
      nodes.push({ id: typeNode.getID(), deps: currentDeps });
    }

    // Type-specific structural nodes
    const typeSpecificNodes = await this.buildFieldTypeNodes(
      fieldDef,
      fieldPath,
      fieldPathParts,
      currentDeps,
      sc,
      addedConstraints,
      topLevelSchema,
      buildCtx,
      skipUnexpectedForObjects,
    );
    nodes.push(...typeSpecificNodes);

    return nodes;
  }

  private async buildFieldTypeNodes(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    currentDeps: number[],
    sc: SchemaDefinition,
    addedConstraints: Map<string, boolean>,
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    skipUnexpectedForObjects: boolean,
  ): Promise<TrackNode[]> {
    const nodes: TrackNode[] = [];
    let node: ValidationNode | null = null;

    switch (fieldDef.type) {
      case "enum": {
        node = await this.buildEnumNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
        );
        break;
      }
      case "array": {
        node = await this.buildContainerNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
          buildCtx,
          "array",
          "array items",
        );
        break;
      }
      case "set": {
        const containerNode = await this.buildContainerNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
          buildCtx,
          "array",
          "set elements",
        );
        const itemGraph =
          containerNode instanceof ArrayValidationNode
            ? containerNode.graph
            : null;
        node = new SetValidationNode(
          this.buildNodeID(),
          fieldPath,
          fieldPathParts,
          itemGraph,
          currentDeps,
        );
        break;
      }
      case "record": {
        node = await this.buildContainerNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
          buildCtx,
          "record",
          "record items",
        );
        break;
      }
      case "union": {
        node = await this.buildUnionNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
          buildCtx,
        );
        break;
      }
      case "composite": {
        const compositeNodes = await this.buildCompositeNode(
          fieldDef,
          fieldPath,
          fieldPathParts,
          currentDeps,
          topLevelSchema,
          buildCtx,
        );
        nodes.push(...compositeNodes);
        return nodes;
      }
      case "geometry": {
        node = new GeometryValidationNode(
          this.buildNodeID(),
          fieldPath,
          fieldPathParts,
          currentDeps,
        );
        break;
      }
      case "object": {
        const objectNodes = await this.buildObjectFieldNodes(
          fieldDef,
          fieldPath,
          fieldPathParts,
          sc,
          addedConstraints,
          topLevelSchema,
          buildCtx,
          skipUnexpectedForObjects,
        );
        nodes.push(...objectNodes);
        return nodes;
      }
      default:
        return nodes;
    }

    if (node) {
      this.addNode(node);
      nodes.push({ id: node.getID(), deps: node.getDependencies() });
    }

    return nodes;
  }

  // ── Object field nodes ────────────────────────────────────────────────────

  private async buildObjectFieldNodes(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    _: SchemaDefinition,
    addedConstraints: Map<string, boolean>,
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    skipUnexpected: boolean,
  ): Promise<TrackNode[]> {
    const nodes: TrackNode[] = [];
    const fsr = FieldSchemaReference.fromFieldSchema(fieldDef.schema);

    if (fsr.isZero()) {
      throw new Error(
        `FieldSchemaReference is zero/uninitialized at path '${fieldPath}'`,
      );
    }
    if (!fsr.isSingle()) {
      throw new Error(
        `Object field must use a single schema reference at path '${fieldPath}'`,
      );
    }

    const schemaRef = fsr.asSingle();
    const nestedDef = topLevelSchema.schemas?.[schemaRef.id];
    if (!nestedDef) {
      throw new Error(
        `Nested schema '${schemaRef.id}' not found for field '${fieldDef.name}' at path '${fieldPath}'`,
      );
    }

    // Recursive reference detection
    if (buildCtx.isRecursive(schemaRef.id)) {
      const instanceConstraints = toConstraintMap(schemaRef.constraints as any);
      const recursiveGraph = await buildCtx.getOrBuildRecursiveGraph(
        schemaRef.id,
        nestedDef,
        instanceConstraints,
        topLevelSchema,
      );

      const markerNode = new RecursionMarkerNode(
        this.buildNodeID(),
        fieldPath,
        fieldPathParts,
        recursiveGraph,
        schemaRef.id,
      );
      this.addNode(markerNode);
      nodes.push({ id: markerNode.getID(), deps: [] });
      return nodes;
    }

    // Non-recursive: build inline
    const effectiveSchema = nestedDefToSchemaDefinition(
      nestedDef,
      topLevelSchema,
    );

    buildCtx.markBuilding(schemaRef.id);
    try {
      const nestedNodes = await this.buildFromSchema(
        effectiveSchema,
        fieldPath,
        fieldPathParts,
        addedConstraints,
        nestedDef,
        toConstraintMap(schemaRef.constraints as any),
        topLevelSchema,
        buildCtx,
        skipUnexpected,
        false,
      );

      const markerNode = new NestedSchemaNode(
        this.buildNodeID(),
        fieldPath,
        fieldPathParts,
        nestedNodes.map((n) => n.id),
      );
      this.addNode(markerNode);
      nodes.push({
        id: markerNode.getID(),
        deps: nestedNodes.map((n) => n.id),
      });
    } finally {
      buildCtx.unmarkBuilding(schemaRef.id);
    }

    return nodes;
  }

  // ── Container node builder (shared for array + record) ───────────────────

  /**
   * Mirrors Go's `buildContainerNode`.
   * Returns an ArrayValidationNode or RecordValidationNode depending on `itemKind`.
   */
  private async buildContainerNode(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    deps: number[],
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    itemKind: "array" | "record",
    errContext: string,
  ): Promise<ArrayValidationNode | RecordValidationNode> {
    const makeNode = (subGraph: ValidationGraph | null) => {
      if (itemKind === "array") {
        return new ArrayValidationNode(
          this.buildNodeID(),
          fieldPath,
          fieldPathParts,
          subGraph,
          deps,
        );
      }
      return new RecordValidationNode(
        this.buildNodeID(),
        fieldPath,
        fieldPathParts,
        subGraph,
        deps,
      );
    };

    const fsr = FieldSchemaReference.fromFieldSchema(fieldDef.schema);

    // No schema reference → untyped container
    if (fsr.isZero()) return makeNode(null);

    // ── Named schema (non-empty id) ──────────────────────────────────────
    if (fsr.isSingle()) {
      const schemaRef = fsr.asSingle();
      if (schemaRef.id !== "") {
        const nestedDef = topLevelSchema.schemas?.[schemaRef.id];
        if (!nestedDef) {
          throw new Error(
            `Nested schema '${schemaRef.id}' not found for ${errContext} at path '${fieldPath}'`,
          );
        }

        const effectiveType = getNestedSchemaEffectiveType(nestedDef);
        const skipUnexpected =
          effectiveType === "composite" || effectiveType === "union";

        // Determine what FieldSchemaReference to forward to the temp root field.
        let tempRootFieldSchema: FieldSchemaReference;
        if (effectiveType === "object") {
          tempRootFieldSchema = FieldSchemaReference.fromSingle({
            id: schemaRef.id,
          });
        } else if ("schema" in nestedDef && nestedDef.schema) {
          tempRootFieldSchema = FieldSchemaReference.fromFieldSchema(
            nestedDef.schema as any,
          );
        } else {
          tempRootFieldSchema = FieldSchemaReference.zero();
        }

        const rawSchema = tempRootFieldSchema.isZero()
          ? undefined
          : tempRootFieldSchema.isSingle()
            ? tempRootFieldSchema.asSingle()
            : tempRootFieldSchema.isMultiple()
              ? tempRootFieldSchema.asMultiple()
              : tempRootFieldSchema.asInline();

        const tempRootField: FieldDefinition = {
          name: "item",
          type: effectiveType as FieldType,
        };
        if (rawSchema !== undefined) tempRootField.schema = rawSchema;
        const rawDefault =
          "default" in nestedDef ? (nestedDef as any).default : undefined;
        if (rawDefault !== undefined)
          (tempRootField as any).default = rawDefault;

        const subGraph = await this.createSubGraph(
          "item",
          tempRootField,
          fieldPath,
          topLevelSchema,
          buildCtx,
          skipUnexpected,
          false,
        );

        return makeNode(subGraph);
      }
    }

    // ── Inline descriptor (empty id, has `type`) ─────────────────────────
    if (fsr.isInline()) {
      const inline = fsr.asInline();
      if (!inline.type) {
        throw new Error(
          `Inline descriptor missing 'type' field at path '${fieldPath}'`,
        );
      }

      const subGraph = new ValidationGraph();
      const addedConstraints = new Map<string, boolean>();
      const itemType = inline.type as FieldType;

      if (itemType === "record") {
        // Inline "record" → any map[string]any, no further fields.
        const syntheticSchema: SchemaDefinition = {
          name: "__inline_record",
          version: "0",
          fields: {},
        };
        if (topLevelSchema.schemas !== undefined)
          syntheticSchema.schemas = topLevelSchema.schemas;
        await subGraph.buildFromSchema(
          syntheticSchema,
          "",
          [],
          addedConstraints,
          null,
          {},
          topLevelSchema,
          buildCtx,
          true,
          false,
        );
      } else {
        // Primitive type → just a TypeCheckNode
        const typeNode = subGraph.createTypeCheckNode(
          "item",
          ["item"],
          itemType,
        );
        subGraph.addNode(typeNode);
      }

      subGraph.finalize();
      return makeNode(subGraph);
    }

    throw new Error(
      `Invalid schema reference for ${errContext} at path '${fieldPath}'`,
    );
  }

  // ── Enum node builder ─────────────────────────────────────────────────────

  private async buildEnumNode(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    currentDeps: number[],
    topLevelSchema: SchemaDefinition,
  ): Promise<EnumValidationNode> {
    const fsr = FieldSchemaReference.fromFieldSchema(fieldDef.schema);
    if (fsr.isZero()) {
      throw new Error(
        `Enum field must have schema reference at path '${fieldPath}'`,
      );
    }

    const refs: SchemaReference[] = fsr.isSingle()
      ? [fsr.asSingle()]
      : fsr.isMultiple()
        ? fsr.asMultiple()
        : [];

    if (refs.length === 0 && !fsr.isInline()) {
      throw new Error(
        `Enum schema reference must be single, multiple, or inline at path '${fieldPath}'`,
      );
    }

    // Handle inline enum descriptor (Rule 21 — inline with values)
    if (fsr.isInline()) {
      const inline = fsr.asInline();
      if (!inline.type) {
        throw new Error(
          `Inline enum descriptor missing 'type' at path '${fieldPath}'`,
        );
      }
      if (!inline.values || inline.values.length === 0) {
        throw new Error(
          `Inline enum descriptor missing 'values' at path '${fieldPath}'`,
        );
      }
      const { lookup, complex } = buildEnumLookup(inline.values);
      const expectNumeric =
        inline.type === "number" ||
        inline.type === "integer" ||
        inline.type === "decimal";
      return new EnumValidationNode(
        this.buildNodeID(),
        fieldPath,
        fieldPathParts,
        lookup,
        complex,
        expectNumeric,
        currentDeps,
      );
    }

    // Named schema refs
    let mergedLookup: Map<unknown, true> = new Map();
    let mergedComplex: unknown[] = [];
    let enumType: FieldType | null = null;

    for (const ref of refs) {
      let lookup: Map<unknown, true>;
      let complex: unknown[];
      let typ: FieldType;

      if (ref.id !== "") {
        const nestedSchema = topLevelSchema.schemas?.[ref.id];
        if (!nestedSchema) {
          throw new Error(
            `Enum schema '${ref.id}' not found at path '${fieldPath}'`,
          );
        }
        if (
          !("values" in nestedSchema) ||
          !(nestedSchema as any).values?.length
        ) {
          throw new Error(
            `Enum schema '${ref.id}' has no values defined at path '${fieldPath}'`,
          );
        }
        typ = (nestedSchema as any).type as FieldType;
        ({ lookup, complex } = buildEnumLookup((nestedSchema as any).values));
      } else {
        // Inline ref in an array (shouldn't happen per Rule 21, but guard it)
        throw new Error(
          `Inline descriptor in named-reference array is not permitted at path '${fieldPath}'`,
        );
      }

      // Merge
      for (const [k] of lookup) mergedLookup.set(k, true);
      mergedComplex = mergedComplex.concat(complex);
      if (!enumType) enumType = typ;
    }

    const expectNumeric =
      enumType === "number" || enumType === "integer" || enumType === "decimal";

    return new EnumValidationNode(
      this.buildNodeID(),
      fieldPath,
      fieldPathParts,
      mergedLookup,
      mergedComplex,
      expectNumeric,
      currentDeps,
    );
  }

  // ── Union node builder ────────────────────────────────────────────────────

  private async buildUnionNode(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    deps: number[],
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
  ): Promise<UnionValidationNode> {
    const fsr = FieldSchemaReference.fromFieldSchema(fieldDef.schema);
    if (fsr.isZero() || !fsr.isMultiple()) {
      throw new Error(
        `Union field must reference multiple schemas at path '${fieldPath}'`,
      );
    }

    const refs = fsr.asMultiple();
    const graphs: ValidationGraph[] = [];

    for (const ref of refs) {
      const nestedDef = topLevelSchema.schemas?.[ref.id];
      if (!nestedDef) {
        throw new Error(
          `Nested schema '${ref.id}' not found for union at path '${fieldPath}'`,
        );
      }

      let tempRootField: FieldDefinition;
      if (
        "fields" in nestedDef &&
        nestedDef.fields &&
        Object.keys(nestedDef.fields).length > 0
      ) {
        tempRootField = {
          name: "root",
          type: "object",
          schema: { id: ref.id },
        };
      } else {
        tempRootField = {
          name: "root",
          type: getNestedSchemaEffectiveType(nestedDef) as FieldType,
          schema: "schema" in nestedDef ? (nestedDef as any).schema : undefined,
        };
      }

      const unionGraph = await this.createSubGraph(
        "root",
        tempRootField,
        fieldPath,
        topLevelSchema,
        buildCtx,
        true,
        false,
      );
      graphs.push(unionGraph);
    }

    return new UnionValidationNode(
      this.buildNodeID(),
      fieldPath,
      fieldPathParts,
      graphs,
      deps,
    );
  }

  // ── Composite node builder ────────────────────────────────────────────────

  private async buildCompositeNode(
    fieldDef: FieldDefinition,
    fieldPath: string,
    fieldPathParts: string[],
    _: number[],
    topLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
  ): Promise<TrackNode[]> {
    const fsr = FieldSchemaReference.fromFieldSchema(fieldDef.schema);
    if (fsr.isZero() || !fsr.isMultiple()) {
      throw new Error(
        `Composite field must reference multiple schemas at path '${fieldPath}'`,
      );
    }

    const refs = fsr.asMultiple();

    // 1. Unified UnexpectedFieldsNode for the merged vocabulary
    const vocab = collectCompositeVocabulary(refs, topLevelSchema);
    const unexpectedNode = this.createUnexpectedFieldsNode(
      fieldPath,
      fieldPathParts,
      vocab,
    );
    this.addNode(unexpectedNode);

    const baseDeps = [unexpectedNode.getID()];
    const allPartNodes: TrackNode[] = [];

    for (const ref of refs) {
      const nestedDef = topLevelSchema.schemas?.[ref.id];
      if (!nestedDef) {
        throw new Error(
          `Nested schema '${ref.id}' not found for composite at path '${fieldPath}'`,
        );
      }

      const effectiveType = getNestedSchemaEffectiveType(nestedDef);

      if (effectiveType === "object" || effectiveType === "unknown") {
        // Object part: inline directly, skip its own unexpected-fields node.
        const effectiveSchema = nestedDefToSchemaDefinition(
          nestedDef,
          topLevelSchema,
        );

        buildCtx.markBuilding(ref.id);
        let partNodes: TrackNode[];
        try {
          partNodes = await this.buildFromSchema(
            effectiveSchema,
            fieldPath,
            fieldPathParts,
            new Map<string, boolean>(),
            nestedDef,
            toConstraintMap(ref.constraints as any),
            topLevelSchema,
            buildCtx,
            true, // skip unexpected — unified node handles it
            true,
          );
        } finally {
          buildCtx.unmarkBuilding(ref.id);
        }

        // Re-wire: inject the shared UnexpectedFieldsNode as a dependency.
        for (const pn of partNodes) {
          if (!pn.deps.includes(unexpectedNode.getID())) {
            pn.deps = [unexpectedNode.getID(), ...pn.deps];
            this.dependencies.set(pn.id, pn.deps);
          }
        }

        allPartNodes.push(...partNodes);
      } else if (effectiveType === "union") {
        // Union part: build per-variant sub-graphs and wrap in a UnionValidationNode.
        if (!("schema" in nestedDef) || !(nestedDef as any).schema) {
          throw new Error(
            `Union part '${ref.id}' in composite must reference multiple schemas at path '${fieldPath}'`,
          );
        }

        const variantFsr = FieldSchemaReference.fromFieldSchema(
          (nestedDef as any).schema,
        );
        if (!variantFsr.isMultiple()) {
          throw new Error(
            `Union part '${ref.id}' in composite must reference multiple schemas at path '${fieldPath}'`,
          );
        }

        const variantRefs = variantFsr.asMultiple();
        const variantGraphs: ValidationGraph[] = [];

        for (const vRef of variantRefs) {
          const variantDef = topLevelSchema.schemas?.[vRef.id];
          if (!variantDef) {
            throw new Error(
              `Union variant schema '${vRef.id}' not found at path '${fieldPath}'`,
            );
          }

          let tempRootField: FieldDefinition;
          if (
            "fields" in variantDef &&
            variantDef.fields &&
            Object.keys(variantDef.fields).length > 0
          ) {
            tempRootField = {
              name: "root",
              type: "object",
              schema: { id: vRef.id },
            };
          } else {
            tempRootField = {
              name: "root",
              type: getNestedSchemaEffectiveType(variantDef) as FieldType,
              schema:
                "schema" in variantDef ? (variantDef as any).schema : undefined,
            };
          }

          const vGraph = await this.createSubGraph(
            "root",
            tempRootField,
            fieldPath,
            topLevelSchema,
            buildCtx,
            true,
            true, // skipUnexpectedForObjects — composite's unified node is sole boundary
          );
          variantGraphs.push(vGraph);
        }

        const memberNode = new UnionValidationNode(
          this.buildNodeID(),
          fieldPath,
          fieldPathParts,
          variantGraphs,
          baseDeps,
        );
        this.addNode(memberNode);
        allPartNodes.push({ id: memberNode.getID(), deps: baseDeps });
      } else {
        throw new Error(
          `Composite part '${ref.id}' has unsupported effective type '${effectiveType}'; composite parts must be object or union schemas at path '${fieldPath}'`,
        );
      }
    }

    const result: TrackNode[] = [
      { id: unexpectedNode.getID(), deps: [] },
      ...allPartNodes,
    ];
    return result;
  }

  // ── Constraint builders ───────────────────────────────────────────────────

  private buildFromEffectiveConstraints(
    effectiveConstraints: EffectiveConstraint[],
    baseParts: string[],
    deps: number[],
    addedConstraints: Map<string, boolean>,
  ): number[] {
    const ruleDepIDs: number[] = [];
    for (const ec of effectiveConstraints) {
      const ids = this.buildFromConstraintRuleWithScope(
        ec.constraint,
        ec.basePath,
        baseParts,
        deps,
        addedConstraints,
        ec.scope,
      );
      ruleDepIDs.push(...ids);
    }
    return ruleDepIDs;
  }

  private buildFromConstraintRuleWithScope(
    rule: ValidatorConstraint,
    basePath: string,
    baseParts: string[],
    deps: number[],
    addedConstraints: Map<string, boolean>,
    scope: ConstraintScope,
  ): number[] {
    const dedupKey = `${basePath}:${rule.name}`;
    if (addedConstraints.get(dedupKey)) return [];
    addedConstraints.set(dedupKey, true);

    if (rule.kind === "rule") {
      const [absoluteFieldPaths, absoluteFieldPathParts] =
        resolveConstraintFieldPaths(basePath, baseParts, rule.fields);

      const node = new ConstraintNode(
        this.buildNodeID(),
        basePath,
        baseParts,
        rule,
        absoluteFieldPaths,
        absoluteFieldPathParts,
        basePath,
        baseParts,
        scope,
        deps,
      );
      this.addNode(node);
      return [node.getID()];
    }

    if (rule.kind === "group") {
      const node = new ConstraintGroupNode(
        this.buildNodeID(),
        basePath,
        baseParts,
        rule,
        rule.name,
        scope,
        deps,
      );
      this.addNode(node);
      return [node.getID()];
    }

    return [];
  }

  // ── Sub-graph factory ─────────────────────────────────────────────────────

  /**
   * Creates a standalone ValidationGraph for a single field, used by array /
   * record / union / composite nodes.
   * Mirrors Go's `graph.createSubGraph`.
   */
  async createSubGraph(
    rootFieldName: string,
    rootFieldDef: FieldDefinition,
    basePath: string,
    originalTopLevelSchema: SchemaDefinition,
    buildCtx: BuildContext,
    skipUnexpectedCheck: boolean,
    skipUnexpectedForObjects: boolean,
  ): Promise<ValidationGraph> {
    const tempSchema: SchemaDefinition = {
      name: `subgraph_${basePath}`,
      version: "0",
      fields: { [rootFieldName]: rootFieldDef },
    };
    if (originalTopLevelSchema.schemas !== undefined)
      tempSchema.schemas = originalTopLevelSchema.schemas;

    const subGraph = new ValidationGraph();
    const addedConstraints = new Map<string, boolean>();

    await subGraph.buildFromSchema(
      tempSchema,
      "",
      [],
      addedConstraints,
      null,
      {},
      originalTopLevelSchema,
      buildCtx,
      skipUnexpectedCheck,
      skipUnexpectedForObjects,
    );

    subGraph.finalize();
    return subGraph;
  }
}

// ---------------------------------------------------------------------------
// Public factory
// ---------------------------------------------------------------------------

export { BuildContext };

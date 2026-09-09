/**
 * nodes.ts
 *
 * All ValidationNode class definitions and their Execute implementations.
 * Mirrors every node type in validator.go faithfully.
 */

import type { ValidationGraph } from "./graph";
import type { FieldType } from "./types";
import {
  type ConstraintScope,
  type Issue,
  type NodeResult,
  SKIPPED,
  SUCCESS,
  type ValidationContext,
  type ValidatorConstraint,
  type ValidatorConstraintGroup,
  type ValidatorConstraintRule,
  failNode,
  createIssue,
} from "./types/validator";
import {
  buildPath,
  deepEqual,
  getMapStringAny,
  getNodeValue,
  getPathDepth,
  isDecimal,
  isInteger,
  isNumber,
  isSafeComparable,
  resolveConstraintFieldPaths,
} from "./utils";

// ---------------------------------------------------------------------------
// ValidationNode interface
// ---------------------------------------------------------------------------

export interface ValidationNode {
  execute(ctx: ValidationContext): Promise<NodeResult>;
  getDependencies(): number[];
  getID(): number;
  getPath(): string;
  getPathParts(): string[];
}

// ---------------------------------------------------------------------------
// BaseNode — shared fields
// ---------------------------------------------------------------------------

export class BaseNode {
  readonly id: number;
  readonly path: string;
  readonly pathParts: string[];
  readonly deps: number[];

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    deps: number[] = [],
  ) {
    this.id = id;
    this.path = path;
    this.pathParts = pathParts;
    this.deps = deps;
  }

  getID(): number {
    return this.id;
  }
  getPath(): string {
    return this.path;
  }
  getPathParts(): string[] {
    return this.pathParts;
  }
  getDependencies(): number[] {
    return this.deps;
  }
}

// ---------------------------------------------------------------------------
// UnexpectedFieldsNode
// ---------------------------------------------------------------------------

export class UnexpectedFieldsNode extends BaseNode implements ValidationNode {
  private readonly expectedFields: Set<string>;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    expectedFields: Set<string>,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.expectedFields = expectedFields;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [currentData, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists) return SUCCESS;

    const dataMap = getMapStringAny(currentData);
    if (!dataMap) {
      return failNode([
        createIssue(
          "TYPE_MISMATCH",
          "Expected object for unexpected field check",
          this.path,
        ),
      ]);
    }

    const issues: Issue[] = [];
    for (const key of Object.keys(dataMap)) {
      if (!this.expectedFields.has(key)) {
        issues.push(
          createIssue(
            "UNEXPECTED_FIELD",
            `Unexpected field '${key}'`,
            buildPath(this.path, key),
          ),
        );
      }
    }

    if (issues.length > 0) {
      return failNode(issues);
    }
    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// RequiredFieldNode
// ---------------------------------------------------------------------------

export class RequiredFieldNode extends BaseNode implements ValidationNode {
  private readonly fieldName: string;
  private readonly parentPath: string;
  private readonly parentPathParts: string[];

  constructor(
    id: number,
    fieldPath: string,
    fieldPathParts: string[],
    fieldName: string,
    parentPath: string,
    parentPathParts: string[],
    deps: number[] = [],
  ) {
    super(id, fieldPath, fieldPathParts, deps);
    this.fieldName = fieldName;
    this.parentPath = parentPath;
    this.parentPathParts = parentPathParts;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [parentData, exists] = getNodeValue(ctx, this.parentPathParts);

    if (!exists) {
      if (this.parentPath !== "") return SUCCESS;
    }

    if (parentData === null || parentData === undefined) {
      return SKIPPED;
    }

    const dataMap = getMapStringAny(parentData);
    if (!dataMap) {
      return failNode([
        createIssue(
          "INVALID_DATA_STRUCTURE",
          "Cannot check for required fields on non-object parent",
          this.parentPath,
        ),
      ]);
    }

    if (!(this.fieldName in dataMap) || dataMap[this.fieldName] === undefined) {
      return failNode([
        createIssue(
          "REQUIRED_FIELD_MISSING",
          `Required field '${this.fieldName}' is missing`,
          this.path,
        ),
      ]);
    }

    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// TypeCheckNode
// ---------------------------------------------------------------------------

export class TypeCheckNode extends BaseNode implements ValidationNode {
  private readonly expected: FieldType;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    expected: FieldType,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.expected = expected;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    switch (this.expected) {
      case "string":
        if (typeof value === "string") return SUCCESS;
        break;
      case "boolean":
        if (typeof value === "boolean") return SUCCESS;
        break;
      case "number":
        if (isNumber(value)) return SUCCESS;
        break;
      case "integer":
        if (isInteger(value)) return SUCCESS;
        break;
      case "decimal":
        if (isDecimal(value)) return SUCCESS;
        break;
      case "bytes":
        if (
          value instanceof Uint8Array ||
          (Array.isArray(value) && value.every((b) => typeof b === "number"))
        ) {
          return SUCCESS;
        }
        break;
      case "record":
        if (getMapStringAny(value) !== null) return SUCCESS;
        break;
      case "unknown":
        return SUCCESS;
      default:
        // Complex types (object, array, set, etc.) are validated by their own nodes.
        return SUCCESS;
    }

    return failNode([
      createIssue(
        "TYPE_MISMATCH",
        `Expected ${this.expected}, got ${typeof value} (${JSON.stringify(value)})`,
        this.path,
      ),
    ]);
  }
}

// ---------------------------------------------------------------------------
// EnumValidationNode
// ---------------------------------------------------------------------------

export class EnumValidationNode extends BaseNode implements ValidationNode {
  /** Fast O(1) lookup for primitives. */
  private readonly lookup: Map<unknown, true>;
  /** Fallback list for complex (object / array) enum values. */
  private readonly complex: unknown[];
  /** Whether the enum type is numeric (affects deepEqual mode). */
  private readonly expectNumeric: boolean;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    lookup: Map<unknown, true>,
    complex: unknown[],
    expectNumeric: boolean,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.lookup = lookup;
    this.complex = complex;
    this.expectNumeric = expectNumeric;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    if (
      typeof value !== "string" &&
      typeof value !== "number" &&
      typeof value !== "boolean"
    ) {
      return failNode([
        createIssue(
          "TYPE_MISMATCH",
          `Expected scalar for enum, got ${typeof value}`,
          this.path,
        ),
      ]);
    }

    if (this.lookup.has(value)) return SUCCESS;

    for (const allowed of this.complex) {
      if (deepEqual(value, allowed, this.expectNumeric)) return SUCCESS;
    }

    return failNode([
      createIssue(
        "ENUM_VIOLATION",
        `Value ${JSON.stringify(value)} is not in the allowed list`,
        this.path,
      ),
    ]);
  }
}

// ---------------------------------------------------------------------------
// ArrayValidationNode
// ---------------------------------------------------------------------------

export class ArrayValidationNode extends BaseNode implements ValidationNode {
  readonly graph: ValidationGraph | null;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    graph: ValidationGraph | null,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.graph = graph;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    if (!Array.isArray(value)) {
      return failNode([
        createIssue("ARRAY_TYPE_MISMATCH", "Expected array", this.path),
      ]);
    }

    const currentDepth = getPathDepth(this.pathParts);
    if (currentDepth >= ctx.maxDepth) {
      return failNode([
        createIssue(
          "MAX_DEPTH_EXCEEDED",
          `Maximum nesting depth of ${ctx.maxDepth} exceeded`,
          this.path,
        ),
      ]);
    }

    if (!this.graph) return SUCCESS;

    const allIssues: Issue[] = [];
    const remainingDepth = ctx.maxDepth - currentDepth;

    for (let i = 0; i < value.length; i++) {
      const item = value[i];
      const itemPath = `${this.path}[${i}]`;
      const itemIssues = await this.graph.traverse(
        ctx.functionMap,
        { item },
        ctx.mode,
        remainingDepth,
        ctx.originalRoot,
      );
      for (const issue of itemIssues) {
        allIssues.push({
          ...issue,
          path: issue.path.startsWith("item")
            ? issue.path.replace("item", itemPath)
            : issue.path,
        });
      }
    }

    return allIssues.length > 0 ? failNode(allIssues) : SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// RecordValidationNode
// ---------------------------------------------------------------------------

export class RecordValidationNode extends BaseNode implements ValidationNode {
  readonly graph: ValidationGraph | null;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    graph: ValidationGraph | null,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.graph = graph;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    const recordMap = getMapStringAny(value);
    if (!recordMap) {
      return failNode([
        createIssue(
          "OBJECT_TYPE_MISMATCH",
          `Expected object for record, got ${typeof value}`,
          this.path,
        ),
      ]);
    }

    const currentDepth = getPathDepth(this.pathParts);
    if (currentDepth >= ctx.maxDepth) {
      return failNode([
        createIssue(
          "MAX_DEPTH_EXCEEDED",
          `Maximum nesting depth of ${ctx.maxDepth} exceeded`,
          this.path,
        ),
      ]);
    }

    if (!this.graph) return SUCCESS;

    const allIssues: Issue[] = [];
    const remainingDepth = ctx.maxDepth - currentDepth;

    for (const [key, item] of Object.entries(recordMap)) {
      const itemPath = buildPath(this.path, key);
      const itemIssues = await this.graph.traverse(
        ctx.functionMap,
        { item },
        ctx.mode,
        remainingDepth,
        ctx.originalRoot,
      );
      for (const issue of itemIssues) {
        allIssues.push({
          ...issue,
          path: issue.path.startsWith("item")
            ? issue.path.replace("item", itemPath)
            : issue.path,
        });
      }
    }

    return allIssues.length > 0 ? failNode(allIssues) : SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// SetValidationNode
// ---------------------------------------------------------------------------

export class SetValidationNode extends BaseNode implements ValidationNode {
  private readonly itemGraph: ValidationGraph | null;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    itemGraph: ValidationGraph | null,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.itemGraph = itemGraph;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    if (!Array.isArray(value)) {
      return failNode([
        createIssue(
          "SET_TYPE_MISMATCH",
          "Expected array for set validation",
          this.path,
        ),
      ]);
    }

    // Validate element types if an itemGraph is present
    if (this.itemGraph) {
      const currentDepth = getPathDepth(this.pathParts);
      if (currentDepth >= ctx.maxDepth) {
        return failNode([
          createIssue(
            "MAX_DEPTH_EXCEEDED",
            `Maximum nesting depth of ${ctx.maxDepth} exceeded`,
            this.path,
          ),
        ]);
      }
      const remainingDepth = ctx.maxDepth - currentDepth;
      const allIssues: Issue[] = [];

      for (let i = 0; i < value.length; i++) {
        const itemPath = `${this.path}[${i}]`;
        const itemIssues = await this.itemGraph.traverse(
          ctx.functionMap,
          { item: value[i] },
          ctx.mode,
          remainingDepth,
          ctx.originalRoot,
        );
        for (const issue of itemIssues) {
          allIssues.push({
            ...issue,
            path: issue.path.startsWith("item")
              ? issue.path.replace("item", itemPath)
              : issue.path,
          });
        }
      }

      if (allIssues.length > 0) return failNode(allIssues);
    }

    // Uniqueness check
    if (value.length <= 1) return SUCCESS;

    const seenComparable = new Map<unknown, number>();
    const seenComplex: Array<{ val: unknown; index: number }> = [];

    for (let i = 0; i < value.length; i++) {
      const item = value[i];

      if (isSafeComparable(item)) {
        const firstIdx = seenComparable.get(item);
        if (firstIdx !== undefined) {
          return failNode([
            createIssue(
              "SET_DUPLICATE",
              `Duplicate value at index ${i} (first seen at index ${firstIdx})`,
              this.path,
            ),
          ]);
        }
        seenComparable.set(item, i);
      } else {
        const dup = seenComplex.find((prev) =>
          deepEqual(item, prev.val, false),
        );
        if (dup) {
          return failNode([
            createIssue(
              "SET_DUPLICATE",
              `Duplicate value at index ${i} (first seen at index ${dup.index})`,
              this.path,
            ),
          ]);
        }
        seenComplex.push({ val: item, index: i });
      }
    }

    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// GeometryValidationNode
// ---------------------------------------------------------------------------

export class GeometryValidationNode extends BaseNode implements ValidationNode {
  constructor(
    id: number,
    path: string,
    pathParts: string[],
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists) return SUCCESS;

    if (!Array.isArray(value)) {
      return failNode([
        createIssue(
          "GEOMETRY_TYPE_MISMATCH",
          "Geometry must be an array of coordinate arrays",
          this.path,
        ),
      ]);
    }

    for (let i = 0; i < value.length; i++) {
      const inner = value[i];
      if (!Array.isArray(inner)) {
        return failNode([
          createIssue(
            "GEOMETRY_TYPE_MISMATCH",
            `Geometry inner element at index ${i} must be an array`,
            `${this.path}[${i}]`,
          ),
        ]);
      }
      for (let j = 0; j < inner.length; j++) {
        if (!isNumber(inner[j])) {
          return failNode([
            createIssue(
              "GEOMETRY_TYPE_MISMATCH",
              `Geometry element at [${i}][${j}] is not a valid number`,
              `${this.path}[${i}][${j}]`,
            ),
          ]);
        }
      }
    }

    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// NestedSchemaNode — DAG synchronization barrier (no-op execute)
// ---------------------------------------------------------------------------

export class NestedSchemaNode extends BaseNode implements ValidationNode {
  constructor(
    id: number,
    path: string,
    pathParts: string[],
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
  }

  async execute(_ctx: ValidationContext): Promise<NodeResult> {
    // Intentional no-op. This node exists solely as a dependency barrier in
    // the DAG so that constraint nodes that depend on an entire nested schema
    // being validated first are correctly ordered.
    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// RecursionMarkerNode
// ---------------------------------------------------------------------------

export class RecursionMarkerNode extends BaseNode implements ValidationNode {
  private readonly validationGraph: ValidationGraph;
  private readonly schemaName: string;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    validationGraph: ValidationGraph,
    schemaName: string,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.validationGraph = validationGraph;
    this.schemaName = schemaName;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    const mapValue = getMapStringAny(value);
    if (!mapValue) {
      return failNode([
        createIssue(
          "TYPE_MISMATCH",
          `Expected object for recursive schema ${this.schemaName}, got ${typeof value}`,
          this.path,
        ),
      ]);
    }

    const currentDepth = getPathDepth(this.pathParts);
    if (currentDepth >= ctx.maxDepth) {
      return failNode([
        createIssue(
          "MAX_DEPTH_EXCEEDED",
          `Recursive schema '${this.schemaName}' exceeds maximum depth of ${ctx.maxDepth}`,
          this.path,
        ),
      ]);
    }

    // Traverse the cached recursive graph with instance constraints already baked in.
    const issues = await this.validationGraph.traverse(
      ctx.functionMap,
      mapValue,
      ctx.mode,
      ctx.maxDepth,
      ctx.originalRoot,
    );

    // Rewrite paths from subgraph context back to parent path.
    const rewritten = issues.map((issue) => ({
      ...issue,
      path: issue.path === "" ? this.path : buildPath(this.path, issue.path),
    }));

    return rewritten.length > 0 ? failNode(rewritten) : SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// UnionValidationNode
// ---------------------------------------------------------------------------

export class UnionValidationNode extends BaseNode implements ValidationNode {
  private readonly graphs: ValidationGraph[];

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    graphs: ValidationGraph[],
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.graphs = graphs;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const [value, exists] = getNodeValue(ctx, this.pathParts);
    if (!exists || value === undefined) return SUCCESS;

    const currentDepth = getPathDepth(this.pathParts);
    if (currentDepth >= ctx.maxDepth) {
      return failNode([
        createIssue(
          "MAX_DEPTH_EXCEEDED",
          `Maximum nesting depth of ${ctx.maxDepth} exceeded`,
          this.path,
        ),
      ]);
    }

    // Evaluate all variants concurrently — they are independent of each other.
    const variantResults = await Promise.all(
      this.graphs.map(async (graph, idx) => {
        const wrapped: Record<string, unknown> = { root: value };
        const issues = await graph.traverse(
          ctx.functionMap,
          wrapped,
          ctx.mode,
          ctx.maxDepth,
          ctx.originalRoot,
        );

        // Rewrite "root" prefix back to this node's path and tag with variant index.
        const rewritten = issues.map((issue) => ({
          ...issue,
          path: issue.path.startsWith("root")
            ? issue.path.replace("root", this.path)
            : issue.path,
          message: `[variant ${idx}] ${issue.message}`,
        }));

        // If the graph reported failure but produced no issues, synthesize one.
        const ok = rewritten.length === 0;
        if (!ok && rewritten.length === 0) {
          rewritten.push(
            createIssue(
              "INTERNAL_ERROR",
              `Variant ${idx} failed without reporting any issues`,
              this.path,
            ),
          );
        }

        return { ok, issues: rewritten };
      }),
    );

    const succeeded = variantResults.some((r) => r.ok);
    if (succeeded) return SUCCESS;

    const syntheticIssue = createIssue(
      "UNION_MISMATCH",
      `Value at '${this.path}' did not match any variant of the union`,
      this.path,
    );

    const allIssues = [
      syntheticIssue,
      ...variantResults.flatMap((r) => r.issues),
    ];
    return failNode(allIssues);
  }
}

// ---------------------------------------------------------------------------
// ConstraintNode
// ---------------------------------------------------------------------------

export class ConstraintNode extends BaseNode implements ValidationNode {
  private readonly constraint: ValidatorConstraint;
  private readonly fieldPaths: string[];
  private readonly fieldPathParts: string[][];
  private readonly constraintPath: string;
  private readonly constraintPathParts: string[];
  private readonly scope: ConstraintScope;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    constraint: ValidatorConstraint,
    fieldPaths: string[],
    fieldPathParts: string[][],
    constraintPath: string,
    constraintPathParts: string[],
    scope: ConstraintScope,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.constraint = constraint;
    this.fieldPaths = fieldPaths;
    this.fieldPathParts = fieldPathParts;
    this.constraintPath = constraintPath;
    this.constraintPathParts = constraintPathParts;
    this.scope = scope;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    if (this.constraint.kind !== "rule") {
      return failNode([
        createIssue(
          "INTERNAL_ERROR",
          `ConstraintNode '${this.constraint.name}': expected a rule, got a group`,
          this.constraintPath,
        ),
      ]);
    }

    switch (this.scope) {
      case "global":
        return this.executeGlobalConstraint(ctx, this.constraint);
      case "recursive":
        return this.executeRecursiveConstraint(ctx, this.constraint);
    }
  }

  private executeGlobalConstraint(
    ctx: ValidationContext,
    rule: ValidatorConstraintRule,
  ): Promise<NodeResult> {
    return evaluateWithPresenceCheck(
      ctx,
      rule.name,
      this.constraintPath,
      this.fieldPaths,
      this.fieldPathParts,
      () => {
        const globalCtx: ValidationContext = { ...ctx, data: ctx.rootData };
        return runConstraintPredicate(globalCtx, rule, this.constraintPath);
      },
    );
  }

  private executeRecursiveConstraint(
    ctx: ValidationContext,
    rule: ValidatorConstraintRule,
  ): Promise<NodeResult> {
    const [instanceData, exists] = getNodeValue(ctx, this.constraintPathParts);
    if (!exists) return Promise.resolve(SKIPPED);

    // Convert absolute field paths to paths relative to the constraint's base.
    const relativeFieldPaths = this.fieldPaths.map((absPath) => {
      if (
        this.constraintPath !== "" &&
        absPath.startsWith(this.constraintPath + ".")
      ) {
        return absPath.slice(this.constraintPath.length + 1);
      }
      return absPath;
    });
    const relativeFieldPathParts = this.fieldPathParts.map((absParts, i) => {
      const absPath = this.fieldPaths[i] ?? "";
      if (
        this.constraintPath !== "" &&
        absPath.startsWith(this.constraintPath + ".")
      ) {
        return absParts.slice(this.constraintPathParts.length);
      }
      return absParts;
    });

    return evaluateWithPresenceCheck(
      ctx,
      rule.name,
      this.constraintPath,
      relativeFieldPaths,
      relativeFieldPathParts,
      () => {
        const instanceCtx: ValidationContext = { ...ctx, data: instanceData };
        return runConstraintPredicate(instanceCtx, rule, this.constraintPath);
      },
    );
  }
}

// ---------------------------------------------------------------------------
// ConstraintGroupNode
// ---------------------------------------------------------------------------

export class ConstraintGroupNode extends BaseNode implements ValidationNode {
  private readonly group: ValidatorConstraintGroup;
  readonly name: string;

  constructor(
    id: number,
    path: string,
    pathParts: string[],
    group: ValidatorConstraintGroup,
    name: string,
    _: ConstraintScope,
    deps: number[] = [],
  ) {
    super(id, path, pathParts, deps);
    this.group = group;
    this.name = name;
  }

  async execute(ctx: ValidationContext): Promise<NodeResult> {
    const { allPaths, allParts } = collectGroupFieldPaths(
      this.group,
      this.path,
      this.pathParts,
    );

    return evaluateWithPresenceCheck(
      ctx,
      this.name,
      this.path,
      allPaths,
      allParts,
      () => this.executeGroup(ctx, this.group, this.path, this.name),
    );
  }

  private async executeGroup(
    ctx: ValidationContext,
    group: ValidatorConstraintGroup,
    path: string,
    name: string,
  ): Promise<NodeResult> {
    const results: boolean[] = [];
    const memberIssues: Issue[] = [];

    for (const rule of group.rules) {
      let res: NodeResult;

      if (rule.kind === "rule") {
        res = await runConstraintPredicate(ctx, rule, path);
      } else if (rule.kind === "group") {
        res = await this.executeGroup(ctx, rule, path, rule.name);
      } else {
        return failNode([
          createIssue(
            "INTERNAL_ERROR",
            `Unknown constraint kind in group '${name}'`,
            path,
          ),
        ]);
      }

      results.push(res.success);
      if (!res.success) {
        memberIssues.push(...res.issues);
      }
    }

    const ok = evaluateLogicalOperator(group.operator, results);
    if (!ok) {
      return failNode([
        createIssue(
          "CONSTRAINT_GROUP_VIOLATION",
          `Constraint group '${name}' failed`,
          path,
        ),
        ...memberIssues,
      ]);
    }

    return SUCCESS;
  }
}

// ---------------------------------------------------------------------------
// Shared constraint helpers
// ---------------------------------------------------------------------------

/**
 * Evaluates a logical operator against a list of boolean results.
 * Mirrors Go's `group.Operator.Evaluate(results)`.
 */
function evaluateLogicalOperator(
  operator: string,
  results: boolean[],
): boolean {
  switch (operator) {
    case "and":
      return results.every(Boolean);
    case "or":
      return results.some(Boolean);
    case "not":
      return results.length === 1 && !results[0];
    case "nor":
      return results.every((r) => !r);
    case "xor":
      return results.filter(Boolean).length === 1;
    case "nand":
      return !results.every(Boolean);
    case "xnor":
      return results.filter(Boolean).length !== 1;
    default:
      return false;
  }
}

/**
 * Collects all field paths (absolute) referenced by every ConstraintRule
 * within a group at any nesting depth.
 */
function collectGroupFieldPaths(
  group: ValidatorConstraintGroup,
  basePath: string,
  baseParts: string[],
): { allPaths: string[]; allParts: string[][] } {
  const pathMap = new Map<string, string[]>();

  function walk(g: ValidatorConstraintGroup): void {
    for (const rule of g.rules) {
      if (rule.kind === "rule") {
        const [paths, parts] = resolveConstraintFieldPaths(
          basePath,
          baseParts,
          rule.fields,
        );
        for (let i = 0; i < paths.length; i++) {
          const p = paths[i]!;
          const pt = parts[i]!;
          pathMap.set(p, pt);
        }
      } else if (rule.kind === "group") {
        walk(rule);
      }
    }
  }

  walk(group);

  const allPaths = Array.from(pathMap.keys());
  const allParts = allPaths.map((p) => pathMap.get(p)!);
  return { allPaths, allParts };
}

/**
 * Presence-check gate that wraps any constraint executor.
 * Mirrors Go's `evaluateWithPresenceCheck`.
 */
async function evaluateWithPresenceCheck(
  ctx: ValidationContext,
  constraintName: string,
  constraintPath: string,
  requiredFields: string[],
  requiredFieldParts: string[][],
  executor: () => Promise<NodeResult> | NodeResult,
): Promise<NodeResult> {
  const presentFields: string[] = [];
  const missingFields: string[] = [];

  for (let i = 0; i < requiredFields.length; i++) {
    const fieldParts = requiredFieldParts[i] ?? [];
    const fieldPath = requiredFields[i] ?? "";
    const [, exists] = getNodeValue(ctx, fieldParts);
    if (exists) {
      presentFields.push(fieldPath);
    } else {
      missingFields.push(fieldPath);
    }
  }

  // All fields present — run the constraint normally.
  if (missingFields.length === 0) {
    return executor();
  }

  // No fields present at all.
  if (presentFields.length === 0) {
    switch (ctx.mode) {
      case "strict":
        return failNode([
          createIssue(
            "CONSTRAINT_INCOMPLETE",
            `Constraint '${constraintName}' cannot be evaluated: missing required fields ${JSON.stringify(missingFields)}`,
            constraintPath,
          ),
        ]);
      case "partialStrict":
      case "loose":
        return SKIPPED;
    }
  }

  // Some fields present, some missing — partial update scenario.
  switch (ctx.mode) {
    case "strict":
      return failNode([
        createIssue(
          "CONSTRAINT_INCOMPLETE",
          `Constraint '${constraintName}' cannot be evaluated: missing required fields ${JSON.stringify(missingFields)}`,
          constraintPath,
        ),
      ]);
    case "partialStrict":
      return failNode([
        createIssue(
          "CONSTRAINT_PARTIAL_UPDATE",
          `Constraint '${constraintName}' couples fields ${JSON.stringify(requiredFields)}. Cannot update only ${JSON.stringify(presentFields)} - all coupled fields must be updated together`,
          constraintPath,
        ),
      ]);
    case "loose":
      return SKIPPED;
  }
}

/**
 * Invokes a predicate function from the FunctionMap and wraps the result
 * in a NodeResult.  Mirrors Go's `runConstraintPredicate`.
 */
async function runConstraintPredicate(
  ctx: ValidationContext,
  rule: ValidatorConstraintRule,
  contextPath: string,
): Promise<NodeResult> {
  const predicateFunc = ctx.functionMap[rule.predicate];
  if (!predicateFunc) {
    return failNode([
      createIssue(
        "MISSING_PREDICATE",
        `Predicate '${rule.predicate}' not found`,
        contextPath,
      ),
    ]);
  }

  const rawIssues = await predicateFunc({
    root: ctx.originalRoot,
    data: ctx.data,
    keys: rule.fields,
    parameters: rule.parameters,
  });

  if (rawIssues.length > 0) {
    const issues = rawIssues.map((issue) => ({
      ...issue,
      path:
        issue.path === "" ? contextPath : buildPath(contextPath, issue.path),
    }));
    return failNode(issues);
  }

  return SUCCESS;
}

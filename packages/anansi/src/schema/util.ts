/**
 * util.ts — SchemaUtil: schema-bound document helpers.
 *
 * hydrate / defaults / empty / pick / required operate on plain documents;
 * validator / validate / standardValidator build (and memoize) the compiled
 * validation graph exactly once per argument set.
 */

import type {
  FieldDefinition,
  InlineTypeDescriptor,
  NestedSchemaDefinition,
  SchemaDefinition,
  SchemaReference,
} from "./generated.ts";
import { parseSchema } from "./types.ts";
import { AnansiCodes, anansiError, isAnansiError } from "../errors.ts";
import { DocumentValidator } from "../validation/validator.ts";
import { StandardDocumentValidator } from "../validation/standard-schema.ts";
import type {
  Issue,
  PredicateMap,
  ValidationConfig,
} from "../validation/validator-types.ts";

type Document = Record<string, unknown>;
type Fields = NonNullable<SchemaDefinition["fields"]>;

/** Stable identity for the empty predicate map so `validator()` memoization hits. */
const NO_PREDICATES: PredicateMap = {};

function isPlainObject(v: unknown): v is Document {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

function parse(data: string | SchemaDefinition | Record<string, unknown>): SchemaDefinition {
  try {
    return parseSchema(data);
  } catch (e) {
    if (isAnansiError(e)) throw e;
    throw anansiError(
      AnansiCodes.SCH.SCHEMA_NAME_MISSING,
      `anansi: schema parse failed: ${errorMessage(e)}`,
      { cause: e },
    );
  }
}

function namedRef(
  field: FieldDefinition,
): SchemaReference | InlineTypeDescriptor | undefined {
  const ref = field.schema;
  if (!ref || Array.isArray(ref)) return undefined;
  return ref;
}

function objectFieldId(field: FieldDefinition): string | undefined {
  if (field.type !== "object") return undefined;
  const ref = namedRef(field);
  if (!ref || !("id" in ref) || typeof ref.id !== "string") return undefined;
  return ref.id;
}

function nestedFor(root: SchemaDefinition, field: FieldDefinition): NestedSchemaDefinition | undefined {
  const id = objectFieldId(field);
  return id !== undefined ? root.schemas?.[id] : undefined;
}

function cloneDefault(value: unknown): unknown {
  return isPlainObject(value) || Array.isArray(value) ? structuredClone(value) : value;
}

function hydrateFields(root: SchemaDefinition, fields: Fields, src: Document): Document {
  const out: Document = { ...src };
  for (const [key, field] of Object.entries(fields)) {
    let value = out[key];
    if (value === undefined && field.default !== undefined) {
      value = cloneDefault(field.default);
      out[key] = value;
    }
    if (isPlainObject(value)) {
      const nested = nestedFor(root, field);
      if (nested?.fields) out[key] = hydrateFields(root, nested.fields, value);
    }
  }
  return out;
}

function enumValues(
  root: SchemaDefinition,
  field: FieldDefinition,
): unknown[] | undefined {
  const ref = namedRef(field);
  if (!ref) return undefined;
  if ("values" in ref && Array.isArray(ref.values)) return ref.values;
  if ("id" in ref && typeof ref.id === "string") {
    return root.schemas?.[ref.id]?.values;
  }
  return undefined;
}

function emptyFields(root: SchemaDefinition, fields: Fields): Document {
  const out: Document = {};
  for (const [key, field] of Object.entries(fields)) {
    if (field.default !== undefined) continue;
    out[key] = zeroValue(root, field);
  }
  return out;
}

function variantZero(root: SchemaDefinition, nested: NestedSchemaDefinition): unknown {
  if (nested.fields && Object.keys(nested.fields).length > 0) {
    return emptyFields(root, nested.fields);
  }
  if (Array.isArray(nested.values) && nested.values.length > 0) {
    return nested.values[0];
  }
  return null;
}

function zeroValue(root: SchemaDefinition, field: FieldDefinition): unknown {
  switch (field.type) {
    case "string":
      return "";
    case "number":
    case "integer":
    case "decimal":
      return 0;
    case "boolean":
      return false;
    case "bytes":
      return new Uint8Array(0);
    case "array":
      return [];
    case "record":
      return {};
    case "enum": {
      const values = enumValues(root, field);
      return values && values.length > 0 ? values[0] : null;
    }
    case "object": {
      const nested = nestedFor(root, field);
      return nested?.fields ? emptyFields(root, nested.fields) : {};
    }
    case "union":
    case "composite": {
      const ref = field.schema;
      if (Array.isArray(ref) && ref.length > 0) {
        const nested = root.schemas?.[ref[0].id];
        if (nested) return variantZero(root, nested);
      }
      return null;
    }
    case "geometry":
      return null;
    case "unknown":
    default:
      return null;
  }
}

export class SchemaUtil {
  readonly schema: SchemaDefinition;

  private cache?: {
    fmap: PredicateMap;
    config?: ValidationConfig;
    validator: DocumentValidator;
  };

  constructor(data: string | SchemaDefinition | Record<string, unknown>) {
    this.schema = parse(data);
  }

  /**
   * Fill absent (`undefined`) keys from `FieldDefinition.default`, then
   * recurse into nested objects (after defaulting). Explicit `null` is
   * preserved, arrays are left untouched, extras pass through, and `input`
   * is never mutated.
   */
  hydrate(input?: unknown): Document {
    return SchemaUtil.hydrate(this.schema, input);
  }

  /** Static variant — works without an instance; `schema` may be JSON text. */
  static hydrate(
    schema: SchemaDefinition | string,
    input?: unknown,
  ): Document {
    const s = typeof schema === "string" ? parse(schema) : schema;
    const src = isPlainObject(input) ? input : {};
    return hydrateFields(s, s.fields ?? {}, src);
  }

  /** `hydrate({})` — a document with every schema default applied. */
  defaults(): Document {
    return SchemaUtil.hydrate(this.schema, {});
  }

  /**
   * A document with one key per field: type-appropriate zero values
   * (`""` / `0` / `false` / `[]` / `{}` / `Uint8Array(0)`, enum → first
   * value, object → nested zeros, unknown/geometry → `null`), with
   * `default`s winning over zeros where present.
   */
  empty(): Document {
    const zero: Document = {};
    for (const [key, field] of Object.entries(this.schema.fields ?? {})) {
      if (field.default !== undefined) continue;
      zero[key] = zeroValue(this.schema, field);
    }
    return this.hydrate(zero);
  }

  /** Subset of `document` for the given top-level keys; missing keys are omitted. */
  pick(document: unknown, keys: readonly string[]): Document {
    const src = isPlainObject(document) ? document : {};
    const out: Document = {};
    for (const key of keys) {
      if (Object.prototype.hasOwnProperty.call(src, key)) out[key] = src[key];
    }
    return out;
  }

  /** Dotted paths (recursive through object refs) of fields with `required: true`. */
  required(): string[] {
    const out: string[] = [];
    const walk = (fields: Fields, prefix: string): void => {
      for (const [key, field] of Object.entries(fields)) {
        const path = prefix === "" ? key : `${prefix}.${key}`;
        if (field.required) out.push(path);
        const nested = nestedFor(this.schema, field);
        if (nested?.fields) walk(nested.fields, path);
      }
    };
    walk(this.schema.fields ?? {}, "");
    return out;
  }

  /**
   * The compiled `DocumentValidator` for this schema. Calls with identical
   * arguments share one memoized instance; a failed build throws `SCH-006`
   * with the underlying error as `cause`.
   */
  validator(
    predicateMap: PredicateMap = NO_PREDICATES,
    config?: ValidationConfig,
  ): DocumentValidator {
    const cached = this.cache;
    if (cached && cached.fmap === predicateMap && cached.config === config) {
      return cached.validator;
    }
    const validator = this.buildValidator(predicateMap, config);
    this.cache = { fmap: predicateMap, config, validator };
    return validator;
  }

  /** Full strict validation of `document` against this schema. */
  async validate(
    document: unknown,
    predicateMap?: PredicateMap,
    config?: ValidationConfig,
  ): Promise<Issue[]> {
    const validator = this.validator(predicateMap, config);
    return validator.validate(document);
  }

  /** A Standard Schema V1 wrapper around this schema's validator. */
  standardValidator(
    predicateMap?: PredicateMap,
    config?: ValidationConfig,
  ): StandardDocumentValidator {
    return new StandardDocumentValidator(this.validator(predicateMap, config), this.schema);
  }

  private buildValidator(
    fmap: PredicateMap,
    config?: ValidationConfig,
  ): DocumentValidator {
    try {
      return DocumentValidator.create(this.schema, fmap, config);
    } catch (e) {
      throw anansiError(
        AnansiCodes.SCH.VALIDATOR_BUILD_FAILED,
        `anansi: failed to build validator: ${errorMessage(e)}`,
        { cause: e },
      );
    }
  }
}

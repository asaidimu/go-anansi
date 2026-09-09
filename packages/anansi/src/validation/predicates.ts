import type { Issue, PredicateMap } from "./types/validator";
import { createIssue } from "./types/validator";
import { isInteger, isNumber } from "./utils";

const primitiveTypes = new Set([
  "string",
  "number",
  "integer",
  "decimal",
  "boolean",
  "geometry",
  "bytes",
  "unknown",
]);

const collectionTypes = new Set(["array", "set"]);

const baseSchemaIndicators = ["fields"];
const fieldPropsIndicators = ["type", "default", "values", "schema"];

const numericTypes = new Set(["number", "integer", "decimal", "string"]);

function getFieldType(field: Record<string, any>): [string, boolean] {
  const t = field["type"];
  if (t === undefined || t === null) return ["", false];
  return [String(t), true];
}

function isPrimitiveType(t: string): boolean {
  return primitiveTypes.has(t);
}

function isNumericType(t: string): boolean {
  return numericTypes.has(t);
}

function isCollectionType(t: string): boolean {
  return collectionTypes.has(t);
}

function getSchemaByID(
  root: Record<string, any>,
  id: string,
): Record<string, any> | null {
  const schemas = root["schemas"] as Record<string, any> | undefined;
  if (!schemas) return null;
  return schemas[id] || null;
}

function isObjectLikeSchema(schema: Record<string, any>): boolean {
  // Has non-empty fields
  if (schema["fields"]) {
    const fmap = schema["fields"] as Record<string, any>;
    if (Object.keys(fmap).length > 0) return true;
  }
  // Or type is object/record
  const t = schema["type"];
  if (t === "object" || t === "record") return true;
  return false;
}

function getSchemaReferenceObjects(
  field: Record<string, any>,
): [any[], boolean] {
  const schemaVal = field["schema"];
  if (schemaVal === undefined || schemaVal === null) return [[], false];
  if (Array.isArray(schemaVal)) return [schemaVal, true];
  return [[schemaVal], false];
}

// ---------------------------------------------------------------------------
// Enum validation helpers (mirrors Go)
// ---------------------------------------------------------------------------

function validateNamedEnumSchema(
  root: Record<string, any>,
  schemaID: string,
  fieldPath: string,
): Issue[] {
  const schemaMap = getSchemaByID(root, schemaID);
  if (!schemaMap) {
    return [
      createIssue(
        "ENUM_NAMED_SCHEMA_NOT_FOUND",
        `Enum referenced schema '${schemaID}' does not exist`,
        fieldPath,
      ),
    ];
  }

  const issues: Issue[] = [];
  const typeVal = schemaMap["type"];
  if (typeVal === undefined || typeVal === null) {
    issues.push(
      createIssue(
        "ENUM_NAMED_SCHEMA_NO_TYPE",
        `Enum referenced schema '${schemaID}' must have a 'type' (string or numeric)`,
        fieldPath,
      ),
    );
    return issues;
  }

  const typeStr = String(typeVal);
  if (!(typeStr === "string" || isNumericType(typeStr))) {
    issues.push(
      createIssue(
        "ENUM_NAMED_SCHEMA_INVALID_TYPE",
        `Enum referenced schema '${schemaID}' type must be string or numeric, got '${typeVal}'`,
        fieldPath,
      ),
    );
    return issues;
  }

  const valuesVal = schemaMap["values"];
  if (valuesVal === undefined || valuesVal === null) {
    issues.push(
      createIssue(
        "ENUM_NAMED_SCHEMA_MISSING_VALUES",
        `Enum schema '${schemaID}' must have a 'values' array`,
        fieldPath,
      ),
    );
    return issues;
  }

  const valuesArr = valuesVal as any[];
  if (!Array.isArray(valuesArr) || valuesArr.length === 0) {
    issues.push(
      createIssue(
        "ENUM_NAMED_SCHEMA_EMPTY_VALUES",
        `Enum schema '${schemaID}' 'values' must be a non-empty array`,
        fieldPath,
      ),
    );
    return issues;
  }

  const isStringEnum = typeStr === "string";
  valuesArr.forEach((val, i) => {
    if (val === null || val === undefined) return;
    if (isStringEnum) {
      if (typeof val !== "string") {
        issues.push(
          createIssue(
            "ENUM_NAMED_VALUE_TYPE_MISMATCH",
            `Enum schema '${schemaID}': value at index ${i} must be string, got ${typeof val}`,
            `${fieldPath} (values[${i}])`,
          ),
        );
      }
    } else if (isNumericType(typeStr)) {
      if (!isNumber(val)) {
        issues.push(
          createIssue(
            "ENUM_NAMED_VALUE_TYPE_MISMATCH",
            `Enum schema '${schemaID}': value at index ${i} must be numeric (type '${typeStr}'), got ${typeof val}`,
            `${fieldPath} (values[${i}])`,
          ),
        );
      }
    }
  });

  baseSchemaIndicators.forEach((key) => {
    const val = schemaMap[key];
    if (val !== undefined && val !== null) {
      const m = val as Record<string, any>;
      if (Object.keys(m).length > 0) {
        issues.push(
          createIssue(
            "ENUM_NAMED_SCHEMA_HAS_FIELDS",
            `Enum referenced schema '${schemaID}' must not have '${key}' (must be a simple value schema)`,
            fieldPath,
          ),
        );
      }
    }
  });

  return issues;
}

function validateInlineEnumDescriptor(
  refMap: Record<string, any>,
  fieldPath: string,
): Issue[] {
  const issues: Issue[] = [];
  const typeVal = refMap["type"];
  if (typeVal === undefined || typeVal === null) {
    issues.push(
      createIssue(
        "ENUM_INLINE_NO_TYPE",
        "Inline enum descriptor must have a 'type' (string or numeric)",
        fieldPath,
      ),
    );
    return issues;
  }

  const typeStr = String(typeVal);
  if (!(typeStr === "string" || isNumericType(typeStr))) {
    issues.push(
      createIssue(
        "ENUM_INLINE_INVALID_TYPE",
        `Inline enum descriptor type must be string or numeric, got '${typeVal}'`,
        fieldPath,
      ),
    );
    return issues;
  }

  const valuesVal = refMap["values"];
  if (valuesVal === undefined || valuesVal === null) {
    issues.push(
      createIssue(
        "ENUM_INLINE_MISSING_VALUES",
        "Inline enum descriptor must have a 'values' array",
        fieldPath,
      ),
    );
    return issues;
  }

  const valuesArr = valuesVal as any[];
  if (!Array.isArray(valuesArr) || valuesArr.length === 0) {
    issues.push(
      createIssue(
        "ENUM_INLINE_EMPTY_VALUES",
        "Inline enum descriptor 'values' must be a non-empty array",
        fieldPath,
      ),
    );
    return issues;
  }

  const isStringEnum = typeStr === "string";
  valuesArr.forEach((val, i) => {
    if (val === null || val === undefined) return;
    if (isStringEnum) {
      if (typeof val !== "string") {
        issues.push(
          createIssue(
            "ENUM_INLINE_VALUE_TYPE_MISMATCH",
            `Inline enum value at index ${i} must be string, got ${typeof val}`,
            `${fieldPath}.values[${i}]`,
          ),
        );
      }
    } else if (isNumericType(typeStr)) {
      if (!isNumber(val)) {
        issues.push(
          createIssue(
            "ENUM_INLINE_VALUE_TYPE_MISMATCH",
            `Inline enum value at index ${i} must be numeric (type '${typeStr}'), got ${typeof val}`,
            `${fieldPath}.values[${i}]`,
          ),
        );
      }
    }
  });

  return issues;
}

function validateEnumReference(
  ref: any,
  fieldPath: string,
  root: Record<string, any>,
): Issue[] {
  if (typeof ref !== "object" || ref === null) {
    return [
      createIssue(
        "ENUM_REF_INVALID",
        "Enum schema reference must be an object",
        fieldPath,
      ),
    ];
  }

  const refMap = ref as Record<string, any>;
  const hasID = "id" in refMap;
  const hasType = "type" in refMap;

  if (hasID && hasType) {
    return [
      createIssue(
        "ENUM_REF_AMBIGUOUS",
        "Enum schema reference cannot have both 'id' (named) and 'type' (inline)",
        fieldPath,
      ),
    ];
  }

  if (hasID) {
    const idVal = refMap["id"];
    if (idVal === undefined || idVal === null || idVal === "") {
      return [
        createIssue(
          "ENUM_NAMED_REF_INVALID_ID",
          "Named enum reference 'id' must be a non-empty string",
          fieldPath,
        ),
      ];
    }
    return validateNamedEnumSchema(root, String(idVal), fieldPath);
  }

  if (hasType) {
    return validateInlineEnumDescriptor(refMap, fieldPath);
  }

  return [
    createIssue(
      "ENUM_REF_NO_ID_OR_TYPE",
      "Enum schema reference must have either 'id' (named) or 'type' (inline)",
      fieldPath,
    ),
  ];
}

// ---------------------------------------------------------------------------
// Predicate Map (Parity with Go)
// ---------------------------------------------------------------------------

/**
 * Resolves a dotted field PATH (e.g. "customer.email") or a bare field NAME
 * against the schema definition, descending through named-object references
 * and inline object descriptors. Index/constraint field references are
 * paths into the document shape — never map keys of root.fields (which are
 * field IDs). Drift fix vs the utils snapshot; mirrors Go meta predicates'
 * name-based lookup while adding nested-path support.
 */
function resolveFieldByPath(
  root: Record<string, any>,
  path: string,
): { def: Record<string, any> } | null {
  const segs = path.split(".");
  let scopeFields: Record<string, any> | null =
    root["fields"] && typeof root["fields"] === "object"
      ? (root["fields"] as Record<string, any>)
      : null;

  let def: Record<string, any> | null = null;
  for (let i = 0; i < segs.length; i++) {
    if (!scopeFields) return null;
    let found: Record<string, any> | null = null;
    for (const v of Object.values(scopeFields)) {
      if (v && typeof v === "object" && (v as Record<string, any>).name === segs[i]) {
        found = v as Record<string, any>;
        break;
      }
    }
    if (!found) return null;
    def = found;
    if (i < segs.length - 1) {
      const ref = found["schema"];
      if (ref && typeof ref === "object" && !Array.isArray(ref)) {
        if (typeof (ref as Record<string, unknown>).id === "string") {
          const id = (ref as Record<string, unknown>).id as string;
          const nested = (root["schemas"] as Record<string, any>)?.[id];
          scopeFields =
            nested && typeof nested === "object"
              ? ((nested as Record<string, any>).fields as Record<string, any>) ?? null
              : null;
        } else if ((ref as Record<string, any>).fields) {
          scopeFields = (ref as Record<string, any>).fields as Record<string, any>;
        } else {
          scopeFields = null;
        }
      } else {
        scopeFields = null;
      }
    }
  }
  return def ? { def } : null;
}

export const metaSchemaPredicateMap: PredicateMap = {
  primitives_prohibit_schema: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const [typeStr, hasType] = getFieldType(data);
    const schemaVal = data["schema"];
    if (
      hasType &&
      isPrimitiveType(typeStr) &&
      schemaVal !== undefined &&
      schemaVal !== null
    ) {
      return [
        createIssue(
          "PRIMITIVE_HAS_SCHEMA",
          `Primitive type '${typeStr}' cannot have a schema reference`,
          "",
        ),
      ];
    }
    return [];
  },

  enum_fields_valid: (params) => {
    const field = params.data as Record<string, any>;
    if (typeof field !== "object" || field === null) return [];

    const [typ, hasType] = getFieldType(field);
    if (!hasType || typ !== "enum") return [];

    const schemaVal = field["schema"];
    if (schemaVal === undefined || schemaVal === null) {
      return [
        createIssue(
          "ENUM_MISSING_SCHEMA",
          "Enum field must have a schema reference",
          "",
        ),
      ];
    }

    const [refs, isArray] = getSchemaReferenceObjects(field);
    if (refs.length === 0) {
      return [
        createIssue(
          "ENUM_NO_VALID_REFS",
          "Enum field has no valid schema references",
          "",
        ),
      ];
    }

    const allIssues: Issue[] = [];
    refs.forEach((ref, i) => {
      const refPath = isArray ? `schema[${i}]` : "schema";
      allIssues.push(...validateEnumReference(ref, refPath, params.root));
    });
    return allIssues;
  },

  composite_requires_multiple_schemas: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const minSchemas = 2;
    const [typeStr, hasType] = getFieldType(data);
    const schemaVal = data["schema"];
    if (hasType && typeStr === "composite") {
      if (schemaVal === undefined || schemaVal === null) {
        return [
          createIssue(
            "COMPOSITE_MISSING_SCHEMA",
            "Composite type must have schema references",
            "",
          ),
        ];
      }
      if (Array.isArray(schemaVal)) {
        if (schemaVal.length < minSchemas) {
          return [
            createIssue(
              "COMPOSITE_INSUFFICIENT_SCHEMAS",
              `Composite must have at least ${minSchemas} schema references`,
              "",
            ),
          ];
        }
      } else {
        return [
          createIssue(
            "COMPOSITE_SCHEMA_NOT_ARRAY",
            "Composite 'schema' must be an array of references",
            "",
          ),
        ];
      }
    }
    return [];
  },

  composite_referenced_schemas_must_be_objects: (params) => {
    const fieldData = params.data as Record<string, any>;
    if (typeof fieldData !== "object" || fieldData === null) return [];

    const [typ, hasType] = getFieldType(fieldData);
    if (!hasType || typ !== "composite") return [];

    const schemaVal = fieldData["schema"];
    if (!Array.isArray(schemaVal) || schemaVal.length === 0) return [];

    const issues: Issue[] = [];
    const root = params.root;

    const validateSchemaIsObjectLikeRecursive = (
      schemaID: string,
      fieldPath: string,
    ): Issue[] => {
      const schemaMap = getSchemaByID(root, schemaID);
      if (!schemaMap) return [];

      const currentIssues: Issue[] = [];
      const t = schemaMap["type"];
      if (t !== undefined && t !== null) {
        const ts = String(t);
        if (ts === "composite") return [];
        if (ts === "union") {
          const unionSchemaVal = schemaMap["schema"];
          if (Array.isArray(unionSchemaVal)) {
            unionSchemaVal.forEach((ref, i) => {
              if (typeof ref === "object" && ref !== null && "id" in ref) {
                currentIssues.push(
                  ...validateSchemaIsObjectLikeRecursive(
                    String(ref.id),
                    `${fieldPath}.schema[${i}]`,
                  ),
                );
              }
            });
          }
          return currentIssues;
        }
      }

      if (!isObjectLikeSchema(schemaMap)) {
        currentIssues.push(
          createIssue(
            "COMPOSITE_REF_NOT_OBJECT",
            `Composite schema '${schemaID}' must effectively represent an object type (has fields, or type 'object'/'record')`,
            fieldPath,
          ),
        );
      }
      return currentIssues;
    };

    schemaVal.forEach((ref, i) => {
      if (typeof ref === "object" && ref !== null && "id" in ref) {
        issues.push(
          ...validateSchemaIsObjectLikeRecursive(
            String(ref.id),
            `schema[${i}]`,
          ),
        );
      }
    });
    return issues;
  },

  object_requires_schema: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const [typeStr, hasType] = getFieldType(data);
    const schemaVal = data["schema"];
    if (hasType && typeStr === "object") {
      if (schemaVal === undefined || schemaVal === null) {
        return [
          createIssue(
            "OBJECT_MISSING_SCHEMA",
            "Object type must have a schema reference",
            "",
          ),
        ];
      }
    }
    return [];
  },

  object_referenced_schema_has_fields: (params) => {
    const root = params.root;
    const fieldData = params.data as Record<string, any>;
    if (typeof fieldData !== "object" || fieldData === null) return [];

    const [typ, hasType] = getFieldType(fieldData);
    if (!hasType || typ !== "object") return [];

    const schemaVal = fieldData["schema"];
    if (
      typeof schemaVal !== "object" ||
      schemaVal === null ||
      Array.isArray(schemaVal)
    )
      return [];

    const schemaID = (schemaVal as Record<string, any>)["id"];
    if (!schemaID) return [];

    const schemaMap = getSchemaByID(root, String(schemaID));
    if (!schemaMap) return [];

    const fields = schemaMap["fields"];
    if (fields === undefined || fields === null) {
      return [
        createIssue(
          "OBJECT_REF_NO_FIELDS",
          `Object field references schema '${schemaID}' which has no 'fields' (must be Schema mode)`,
          "",
        ),
      ];
    }
    if (typeof fields !== "object" || Object.keys(fields).length === 0) {
      return [
        createIssue(
          "OBJECT_REF_EMPTY_FIELDS",
          `Object field references schema '${schemaID}' which has empty fields`,
          "",
        ),
      ];
    }
    return [];
  },

  
spatial_index_on_geometry_field: (params) => {
    const root = params.root;
    const indexData = params.data as Record<string, any>;
    if (typeof indexData !== "object" || indexData === null) return [];

    if (indexData["type"] !== "spatial") return [];

    const fieldsVal = indexData["fields"];
    if (!Array.isArray(fieldsVal)) return [];

    for (const f of fieldsVal) {
      const fieldPath = String(f);
      // Index field references are PATHS/NAMES, not root.fields map keys.
      const resolved = resolveFieldByPath(root, fieldPath);
      if (!resolved) continue;

      const fieldType = resolved.def["type"];
      if (fieldType !== "geometry") {
        return [
          createIssue(
            "SPATIAL_INDEX_NON_GEOMETRY",
            `Spatial index can only reference geometry fields, but field '${fieldPath}' has type '${fieldType}'`,
            "",
          ),
        ];
      }
    }
    return [];
  },

  index_condition_value_matches_field_type: (params) => {
    const root = params.root;
    const condition = params.data as Record<string, any>;
    if (typeof condition !== "object" || condition === null) return [];

    const fieldPath = condition["field"];
    if (fieldPath === undefined || fieldPath === null) return [];

    const value = condition["value"];
    // Index condition field references are PATHS/NAMES, not map keys.
    const resolved = resolveFieldByPath(root, String(fieldPath));
    if (!resolved) return [];

    const fieldType = resolved.def["type"] as string | undefined;

    let ok = true;
    switch (fieldType) {
      case "string":
        if (typeof value !== "string" && value !== null) ok = false;
        break;
      case "boolean":
        if (typeof value !== "boolean" && value !== null) ok = false;
        break;
      case "integer":
        if (!isInteger(value) && value !== null) ok = false;
        break;
      case "number":
      case "decimal":
        if (!isNumber(value) && value !== null) ok = false;
        break;
    }

    if (!ok) {
      return [
        createIssue(
          "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
          `Value for field '${fieldPath}' must be ${fieldType}`,
          "",
        ),
      ];
    }
    return [];
  },

  schema_reference_exists: (params) => {
    const root = params.data as Record<string, any>;
    if (typeof root !== "object" || root === null) return [];

    const schemasMap = (root["schemas"] || {}) as Record<string, any>;
    const issues: Issue[] = [];

    // Helper to validate a single SchemaReference object
    const validateReference = (ref: any, path: string) => {
      if (ref && typeof ref === "object" && "id" in ref) {
        const idVal = ref.id;
        if (typeof idVal === "string" && !schemasMap[idVal]) {
          issues.push(
            createIssue(
              "SCHEMA_REFERENCE_NOT_FOUND",
              `Referenced schema '${idVal}' does not exist`,
              path,
            ),
          );
        }
      }
    };

    const walk = (data: any, path: string) => {
      if (typeof data !== "object" || data === null) return;

      // 1. If we are at a 'schema' property (on a field or constraint)
      if (data.hasOwnProperty("schema")) {
        const schemaVal = data["schema"];
        if (Array.isArray(schemaVal)) {
          // Handle Union/Array of references
          schemaVal.forEach((item, i) =>
            validateReference(item, `${path}.schema[${i}]`),
          );
        } else {
          // Handle single reference
          validateReference(schemaVal, `${path}.schema`);
        }
      }

      // 2. Recursively descend into children, but ONLY if they are structures
      // that might contain further 'schema' definitions (like 'fields' or 'schemas')
      for (const [key, value] of Object.entries(data)) {
        if (typeof value === "object" && value !== null) {
          walk(value, path ? `${path}.${key}` : key);
        }
      }
    };

    walk(root, "");
    return issues;
  },

  default_matches_type: (params) => {
    const fieldData = params.data as Record<string, any>;
    if (typeof fieldData !== "object" || fieldData === null) return [];

    const typeVal = fieldData["type"];
    const defaultVal = fieldData["default"];
    if (
      typeVal === undefined ||
      defaultVal === undefined ||
      defaultVal === null
    )
      return [];

    const typeStr = String(typeVal);
    let errMsg = "";
    switch (typeStr) {
      case "string":
        if (typeof defaultVal !== "string") errMsg = "must be string";
        break;
      case "boolean":
        if (typeof defaultVal !== "boolean") errMsg = "must be boolean";
        break;
      case "integer":
        if (!isInteger(defaultVal)) errMsg = "must be integer";
        break;
      case "number":
      case "decimal":
        if (!isNumber(defaultVal)) errMsg = "must be numeric";
        break;
      case "array":
      case "set":
        if (!Array.isArray(defaultVal)) errMsg = "must be array";
        break;
      case "object":
      case "record":
        if (typeof defaultVal !== "object" || Array.isArray(defaultVal))
          errMsg = "must be object";
        break;
      case "geometry":
        if (!Array.isArray(defaultVal))
          errMsg = "must be array of coordinate arrays";
        break;
    }

    if (errMsg) {
      return [
        createIssue(
          "DEFAULT_VALUE_TYPE_MISMATCH",
          `Default value ${JSON.stringify(defaultVal)} ${errMsg} for type ${typeStr}`,
          "",
        ),
      ];
    }
    return [];
  },

  global_field_id_uniqueness: (params) => {
    const root = params.data as Record<string, any>;
    if (typeof root !== "object" || root === null) return [];

    const seen = new Map<string, string>();
    const issues: Issue[] = [];

    const walkFields = (fieldsMap: any, path: string) => {
      if (typeof fieldsMap !== "object" || fieldsMap === null) return;
      for (const id of Object.keys(fieldsMap)) {
        const existingPath = seen.get(id);
        if (existingPath) {
          issues.push(
            createIssue(
              "DUPLICATE_FIELD_ID",
              `Field ID '${id}' is not unique; already used at ${existingPath}`,
              `${path}.${id}`,
            ),
          );
        } else {
          seen.set(id, `${path}.${id}`);
        }
      }
    };

    // Check root fields
    if (root["fields"]) walkFields(root["fields"], "fields");

    // Check nested schemas
    const schemas = root["schemas"] as Record<string, any>;
    if (schemas && typeof schemas === "object") {
      for (const [schemaID, schemaVal] of Object.entries(schemas)) {
        if (typeof schemaVal === "object" && schemaVal !== null) {
          const sMap = schemaVal as Record<string, any>;
          if (sMap["fields"]) {
            walkFields(sMap["fields"], `schemas.${schemaID}.fields`);
          }
        }
      }
    }
    return issues;
  },

  constraint_fields_exist: (params) => {
    const root = params.root as Record<string, any>;
    const issues: Issue[] = [];

    const resolveFieldPath = (
      path: string,
      contextFields: Record<string, any>,
    ): boolean => {
      const parts = path.split(".");
      let current = contextFields;
      for (let i = 0; i < parts.length; i++) {
        const part = parts[i]!;
        const field = current[part];
        if (field === undefined) return false;
        if (i === parts.length - 1) return true;

        const fieldMap = field as Record<string, any>;
        if (fieldMap["type"] === "object") {
          const schemaRef = fieldMap["schema"] as Record<string, any>;
          if (schemaRef && schemaRef["id"]) {
            const nestedSchema = getSchemaByID(root, String(schemaRef["id"]));
            if (nestedSchema && nestedSchema["fields"]) {
              current = nestedSchema["fields"] as Record<string, any>;
              continue;
            }
          }
        }
        return false;
      }
      return false;
    };

    const checkConstraints = (
      constraints: Record<string, any>,
      contextFields: Record<string, any>,
      basePath: string,
    ) => {
      for (const [constraintID, constraintData] of Object.entries(
        constraints,
      )) {
        if (typeof constraintData !== "object" || constraintData === null)
          continue;
        const cMap = constraintData as Record<string, any>;
        if (cMap["predicate"] && cMap["fields"]) {
          const fieldsArray = cMap["fields"];
          if (Array.isArray(fieldsArray)) {
            fieldsArray.forEach((fieldPath, i) => {
              if (typeof fieldPath === "string") {
                if (!resolveFieldPath(fieldPath, contextFields)) {
                  issues.push(
                    createIssue(
                      "CONSTRAINT_FIELD_NOT_FOUND",
                      `Constraint '${constraintID}' references non-existent field path '${fieldPath}'`,
                      `${basePath}constraints.${constraintID}.fields[${i}]`,
                    ),
                  );
                }
              }
            });
          }
        }
      }
    };

    const rootFields = root["fields"] as Record<string, any>;
    const rootConstraints = root["constraints"] as Record<string, any>;
    if (rootConstraints && rootFields)
      checkConstraints(rootConstraints, rootFields, "");

    const schemas = root["schemas"] as Record<string, any>;
    if (schemas) {
      for (const [schemaID, schemaData] of Object.entries(schemas)) {
        if (typeof schemaData === "object" && schemaData !== null) {
          const sMap = schemaData as Record<string, any>;
          const sConstraints = sMap["constraints"] as Record<string, any>;
          const sFields = sMap["fields"] as Record<string, any>;
          if (sConstraints && sFields)
            checkConstraints(sConstraints, sFields, `schemas.${schemaID}.`);
        }
      }
    }
    return issues;
  },

  inline_type_descriptor_valid: (params) => {
    const field = params.data as Record<string, any>;
    if (typeof field !== "object" || field === null) return [];

    const schemaVal = field["schema"] as Record<string, any>;
    if (!schemaVal || typeof schemaVal !== "object" || Array.isArray(schemaVal))
      return [];

    const fieldTypeVal = field["type"];
    const inlineTypeVal = schemaVal["type"];
    if (fieldTypeVal === undefined || inlineTypeVal === undefined) return [];

    const allowedToHaveInline = new Set(["record", "array", "set", "enum"]);
    const fieldTypeStr = String(fieldTypeVal);
    if (!allowedToHaveInline.has(fieldTypeStr)) {
      return [
        createIssue(
          "INLINE_DESCRIPTOR_NOT_ALLOWED_FOR_FIELD_TYPE",
          `Inline type descriptors are only allowed for fields of type record, array, set, or enum, but got '${fieldTypeStr}'`,
          "",
        ),
      ];
    }

    const inlineTypeStr = String(inlineTypeVal);
    const allowedInline = new Set([
      "string",
      "number",
      "integer",
      "decimal",
      "boolean",
      "bytes",
      "unknown",
      "record",
    ]);
    if (!allowedInline.has(inlineTypeStr)) {
      return [
        createIssue(
          "INLINE_DESCRIPTOR_INVALID_TYPE",
          `Inline descriptor type '${inlineTypeStr}' is not allowed (only primitives and 'record')`,
          "",
        ),
      ];
    }

    const valuesVal = schemaVal["values"];
    if (valuesVal !== undefined && valuesVal !== null) {
      if (inlineTypeStr !== "string" && !isNumericType(inlineTypeStr)) {
        return [
          createIssue(
            "INLINE_DESCRIPTOR_VALUES_WITHOUT_ENUM",
            "'values' can only be used with string or numeric inline types",
            "",
          ),
        ];
      }
      if (!Array.isArray(valuesVal) || valuesVal.length === 0) {
        return [
          createIssue(
            "INLINE_DESCRIPTOR_VALUES_NOT_ARRAY",
            "'values' must be a non‑empty array",
            "",
          ),
        ];
      }
    }
    return [];
  },

  schema_reference_form_correct: (params) => {
    const fieldData = params.data as Record<string, any>;
    if (typeof fieldData !== "object" || fieldData === null) return [];

    const [typeStr, hasType] = getFieldType(fieldData);
    if (!hasType) return [];

    const schemaVal = fieldData["schema"];
    if (schemaVal === undefined || schemaVal === null) return [];

    const classifyRef = (refMap: Record<string, any>) => {
      return { isNamed: "id" in refMap, isInline: "type" in refMap };
    };

    if (Array.isArray(schemaVal)) {
      if (schemaVal.length === 0) {
        return [
          createIssue(
            "SCHEMA_REF_FORM_INVALID",
            `Field type '${typeStr}' requires at least one reference`,
            "",
          ),
        ];
      }
      if (
        typeStr !== "union" &&
        typeStr !== "composite" &&
        typeStr !== "enum"
      ) {
        return [
          createIssue(
            "SCHEMA_REF_FORM_INVALID",
            `Field type '${typeStr}' cannot use an array of references`,
            "",
          ),
        ];
      }
      for (let i = 0; i < schemaVal.length; i++) {
        const item = schemaVal[i];
        if (typeof item !== "object" || item === null) {
          return [
            createIssue(
              "SCHEMA_REF_FORM_INVALID",
              `Element ${i} of schema array is not an object`,
              "",
            ),
          ];
        }
        const { isNamed, isInline } = classifyRef(item as Record<string, any>);
        if (typeStr === "enum") {
          if (!isNamed && !isInline) {
            return [
              createIssue(
                "SCHEMA_REF_FORM_INVALID",
                `Enum array element ${i} must be a named reference or inline descriptor`,
                "",
              ),
            ];
          }
        } else {
          if (!isNamed) {
            return [
              createIssue(
                "SCHEMA_REF_FORM_INVALID",
                `Field type '${typeStr}' array element ${i} must be a named schema reference (with 'id')`,
                "",
              ),
            ];
          }
        }
      }
    } else if (typeof schemaVal === "object") {
      const { isNamed, isInline } = classifyRef(
        schemaVal as Record<string, any>,
      );
      switch (typeStr) {
        case "array":
        case "set":
        case "record":
        case "enum":
          if (!isNamed && !isInline) {
            return [
              createIssue(
                "SCHEMA_REF_FORM_INVALID",
                `Field type '${typeStr}' requires a single named reference or an inline descriptor`,
                "",
              ),
            ];
          }
          break;
        case "object":
          if (!isNamed) {
            return [
              createIssue(
                "SCHEMA_REF_FORM_INVALID",
                "Object field requires a single named schema reference",
                "",
              ),
            ];
          }
          break;
        case "union":
        case "composite":
          return [
            createIssue(
              "SCHEMA_REF_FORM_INVALID",
              `Field type '${typeStr}' requires an array of references`,
              "",
            ),
          ];
      }
    }
    return [];
  },

  collection_requires_schema: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const [typeStr, hasType] = getFieldType(data);
    if (!hasType) return [];

    const schemaVal = data["schema"];
    if (
      isCollectionType(typeStr) &&
      (schemaVal === undefined || schemaVal === null)
    ) {
      return [
        createIssue(
          "COLLECTION_MISSING_SCHEMA",
          `Collection type '${typeStr}' must have a schema reference`,
          "",
        ),
      ];
    }
    return [];
  },

  union_requires_multiple_schemas: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const minSchemas = 2;
    const [typeStr, hasType] = getFieldType(data);
    if (!hasType || typeStr !== "union") return [];

    const schemaVal = data["schema"];
    if (schemaVal === undefined || schemaVal === null) {
      return [
        createIssue(
          "UNION_MISSING_SCHEMA",
          "Union type must have schema references",
          "",
        ),
      ];
    }
    if (!Array.isArray(schemaVal)) {
      return [
        createIssue(
          "UNION_SCHEMA_NOT_ARRAY",
          "Union type schema must be an array of schema references",
          "",
        ),
      ];
    }
    if (schemaVal.length < minSchemas) {
      return [
        createIssue(
          "UNION_INSUFFICIENT_SCHEMAS",
          `Union type must have at least ${minSchemas} schema references, got ${schemaVal.length}`,
          "",
        ),
      ];
    }
    return [];
  },

  record_schema_cardinality: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const [typeStr, hasType] = getFieldType(data);
    if (!hasType || typeStr !== "record") return [];

    const schemaVal = data["schema"];
    if (Array.isArray(schemaVal)) {
      return [
        createIssue(
          "RECORD_SCHEMA_ARRAY",
          "Record type must have zero or one schema reference, not an array",
          "",
        ),
      ];
    }
    return [];
  },

  nested_schema_exclusive_mode: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    let hasBaseSchema = false;
    for (const key of baseSchemaIndicators) {
      const val = data[key];
      if (val !== undefined && val !== null) {
        if (
          typeof val === "object" &&
          Object.keys(val as Record<string, any>).length > 0
        ) {
          hasBaseSchema = true;
          break;
        }
      }
    }

    let hasFieldProps = false;
    for (const key of fieldPropsIndicators) {
      const val = data[key];
      if (val !== undefined && val !== null) {
        hasFieldProps = true;
        break;
      }
    }

    if (hasBaseSchema && hasFieldProps) {
      return [
        createIssue(
          "NESTED_SCHEMA_MIXED_MODE",
          "NestedSchema cannot mix BaseSchema fields (fields/indexes/constraints) with FieldProperties (type/values/schema)",
          "",
        ),
      ];
    }
    if (!hasBaseSchema && !hasFieldProps) {
      return [
        createIssue(
          "NESTED_SCHEMA_NO_MODE",
          "NestedSchema must have either BaseSchema fields or FieldProperties, not neither",
          "",
        ),
      ];
    }
    return [];
  },

  constraint_type_exclusive: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const name = String(data["name"] || "UNNAMED");
    const hasRule = !!(data["predicate"] && String(data["predicate"]) !== "");
    const hasGroup = !!(
      data["operator"] &&
      Array.isArray(data["rules"]) &&
      data["rules"].length > 0
    );

    if (hasRule && hasGroup) {
      return [
        createIssue(
          "CONSTRAINT_MIXED_TYPE",
          "Constraint cannot have both predicate (rule) and operator+rules (group)",
          name,
        ),
      ];
    }
    if (!hasRule && !hasGroup) {
      return [
        createIssue(
          "CONSTRAINT_NO_TYPE",
          "Constraint must have either predicate (rule) or operator+rules (group)",
          name,
        ),
      ];
    }
    return [];
  },

  constraint_rule_requires_predicate: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const val = data["predicate"];
    if (val === undefined)
      return [
        createIssue(
          "REQUIRED_FIELD_MISSING",
          "Required field 'predicate' is missing",
          "predicate",
        ),
      ];
    if (val === null)
      return [
        createIssue(
          "REQUIRED_FIELD_NULL",
          "Required field 'predicate' cannot be null",
          "predicate",
        ),
      ];
    if (typeof val !== "string")
      return [
        createIssue(
          "REQUIRED_FIELD_WRONG_TYPE",
          "Required field 'predicate' must be a string",
          "predicate",
        ),
      ];
    if (val.trim() === "")
      return [
        createIssue(
          "REQUIRED_FIELD_EMPTY",
          "Required field 'predicate' cannot be empty",
          "predicate",
        ),
      ];
    return [];
  },

  index_condition_type_exclusive: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const hasSingle = !!(
      data["field"] &&
      data["operator"] &&
      data["value"] !== undefined
    );
    const hasGroup = !!(
      data["operator"] &&
      Array.isArray(data["conditions"]) &&
      data["conditions"].length > 0
    );

    if (hasSingle && hasGroup) {
      return [
        createIssue(
          "INDEX_CONDITION_MIXED_TYPE",
          "IndexCondition cannot have both single condition fields and group fields",
          "",
        ),
      ];
    }
    if (!hasSingle && !hasGroup) {
      return [
        createIssue(
          "INDEX_CONDITION_NO_TYPE",
          "IndexCondition must be either a single condition or a group",
          "",
        ),
      ];
    }
    return [];
  },

  schema_name_required: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const val = data["name"];
    if (val === undefined || val === null || String(val).trim() === "") {
      return [
        createIssue("SCHEMA_NAME_MISSING", "Schema name is required", "name"),
      ];
    }
    return [];
  },

  field_name_required: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const val = data["name"];
    if (val === undefined || val === null || String(val).trim() === "") {
      return [
        createIssue("FIELD_NAME_MISSING", "Field name is required", "name"),
      ];
    }
    return [];
  },

  index_fields_not_empty: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const fields = data["fields"];
    if (!Array.isArray(fields) || fields.length === 0) {
      return [
        createIssue(
          "INDEX_FIELDS_EMPTY",
          "Index must reference at least one field",
          "fields",
        ),
      ];
    }
    return [];
  },

  schema_reference_id_required: (params) => {
    const data = params.data as Record<string, any>;
    if (typeof data !== "object" || data === null) return [];

    const val = data["id"];
    if (val === undefined || val === null || String(val).trim() === "") {
      return [
        createIssue(
          "SCHEMA_REFERENCE_ID_MISSING",
          "SchemaReference ID is required",
          "id",
        ),
      ];
    }
    return [];
  },

  index_fields_reference_valid: (params) => {
    // TODO: Check that declared paths can be reached from the root by
    // following the path
    return [];

    // const root = params.root;
    // const data = params.data as Record<string, any>;
    // if (typeof data !== "object" || data === null) return [];
    //
    // const schemaFields = root["fields"] as Record<string, any>;
    // if (!schemaFields) return [];
    //
    // const fieldsArray = data["fields"];
    // if (!Array.isArray(fieldsArray)) return [];
    //
    // const name = String(data["name"] || "UNNAMED");
    // const issues: Issue[] = [];
    // fieldsArray.forEach((fieldID, i) => {
    //   if (!schemaFields[String(fieldID)]) {
    //     issues.push(
    //       createIssue(
    //         "INDEX_FIELD_NOT_FOUND",
    //         `Index '${name}' references non-existent field '${fieldID}'`,
    //         `fields[${i}]`,
    //       ),
    //     );
    //   }
    // });
    // return issues;
  },
};

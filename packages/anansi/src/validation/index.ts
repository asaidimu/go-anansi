// Document + schema validation, adopted from @asaidimu/utils/src/schema.
export {
  DocumentValidator,
  type Issue,
  type PredicateMap,
  type ValidationConfig,
  defaultValidationConfig,
} from "./validator.ts";
export { SchemaValidator } from "./schema-validator.ts";
export { metaSchemaPredicateMap } from "./predicates.ts";
export type { SchemaDefinition as ValidatedSchemaDefinition } from "./types/schema-definition.ts";

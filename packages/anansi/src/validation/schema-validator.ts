import { DocumentValidator } from "./validator.ts";
import schema from "./schema.json";
import { metaSchemaPredicateMap } from "./predicates.ts";
import type { Issue } from "./validator-types.ts";
import type { SchemaDefinition } from "../schema/generated.ts";
import { parseSchema } from "../schema/types.ts";

/**
 * A dedicated validator for ensuring schema definitions themselves
 * conform to the meta-schema rules.
 */
export class SchemaValidator {
  private static validator: DocumentValidator | null = null;

  /**
   * Validates a schema definition against the meta-schema rules.
   */
  static async validate(schemaDef: SchemaDefinition): Promise<Issue[]> {
    if (!this.validator) {
      this.validator = DocumentValidator.create(
        parseSchema(schema),
        metaSchemaPredicateMap,
      );
    }
    return await this.validator.validate(schemaDef);
  }
}

import { DocumentValidator } from "./validator";
import schema from "./schema.json";
import { metaSchemaPredicateMap } from "./predicates";
import type { SchemaDefinition } from "./types/schema-definition";
import type { Issue } from "./types/validator";

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
      this.validator = await DocumentValidator.create(
        schema as any,
        metaSchemaPredicateMap,
      );
    }
    return await this.validator.validate(schemaDef);
  }
}

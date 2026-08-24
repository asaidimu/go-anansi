import type { StandardSchemaV1 } from "@standard-schema/spec";
import { DocumentValidator } from "./validator";
import type { SchemaDefinition } from "./types/schema-definition";
import type { PredicateMap, ValidationConfig } from "./types/validator";

/**
 * A Standard Schema V1 compatible wrapper for the DocumentValidator.
 */
export class StandardDocumentValidator<T = unknown> implements StandardSchemaV1<
  unknown,
  T
> {
  readonly "~standard": StandardSchemaV1.Props<unknown, T>;

  constructor(
    private readonly validator: DocumentValidator,
    readonly schema: SchemaDefinition,
  ) {
    this["~standard"] = {
      version: 1,
      vendor: "asaidimu-utils-schema",
      validate: async (value: unknown): Promise<StandardSchemaV1.Result<T>> => {
        const issues = await this.validator.validate(value as any);
        if (issues.length === 0) {
          return { value: value as T };
        }

        return {
          issues: issues.map((issue) => ({
            message: issue.message,
            path: issue.path ? issue.path.split(".") : [],
          })),
        };
      },
    };
  }

  /**
   * Factory method to create a StandardDocumentValidator.
   */
  static async create<T = unknown>(
    schema: SchemaDefinition,
    predicateMap: PredicateMap = {},
    config?: ValidationConfig,
  ): Promise<StandardDocumentValidator<T>> {
    const validator = await DocumentValidator.create(
      schema,
      predicateMap,
      config,
    );
    return new StandardDocumentValidator<T>(validator, schema);
  }
}

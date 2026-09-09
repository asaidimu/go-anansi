import { describe, it, expect } from "bun:test";
import {
  DocumentValidator,
  metaSchemaPredicateMap,
} from "../src/validation/index.ts";

// Parity with core/schema/definition/validator_record_test.go: an array
// declared with the inline descriptor { "type": "record" } must accept plain
// object items and reject everything else.
describe("inline record array", () => {
  const build = () =>
    DocumentValidator.create(
      {
        name: "RecordDoc",
        version: "1.0.0",
        fields: {
          tags: { name: "tags", type: "array", schema: { type: "record" } },
        },
      } as never,
      metaSchemaPredicateMap,
    );

  it("accepts map items", async () => {
    const v = await build();
    const issues = await v.validate({
      tags: [{ key: "env", value: "prod" }, {}],
    });
    expect(issues).toEqual([]);
  });

  it("rejects non-map items", async () => {
    const v = await build();
    const issues = await v.validate({ tags: ["not-a-record"] });
    expect(issues.length).toBe(1);
    expect(issues[0]?.code).toBe("TYPE_MISMATCH");
    expect(issues[0]?.path).toBe("tags[0]");
  });
});

// Shared fixture: the schema both sides agree on, plus the sample request
// documents. Kept byte-identical in main.go via the same JSON string.
export const schemaJSON = `{
  "version": "1.0.0",
  "name": "orders",
  "fields": {
    "f01": { "name": "order_id", "type": "string", "required": true },
    "f02": { "name": "total",    "type": "number" },
    "f03": { "name": "paid",     "type": "boolean" },
    "f04": { "name": "receipt",  "type": "bytes" },
    "f05": { "name": "tags",     "type": "array", "schema": { "type": "string" } },
    "f06": { "name": "customer", "type": "object", "schema": { "id": "customer" } },
    "f07": { "name": "lines",    "type": "array",  "schema": { "id": "line" } }
  },
  "schemas": {
    "customer": {
      "name": "customer",
      "fields": {
        "c1": { "name": "email", "type": "string" },
        "c2": { "name": "tier",  "type": "string" }
      }
    },
    "line": {
      "name": "line",
      "fields": {
        "l1": { "name": "sku", "type": "string" },
        "l2": { "name": "qty", "type": "integer" }
      }
    }
  }
}`;

export const order = {
  order_id: "ORD-2026-0001",
  total: 129.99,
  paid: true,
  receipt: "cmVjaXB0LWJ5dGVz",
  tags: ["priority", "gift"],
  customer: { email: "ada@example.com", tier: "gold" },
  lines: [
    { sku: "WIDGET-1", qty: 2 },
    { sku: "SPROCKET-9", qty: 1 },
  ],
};

export const expectedResponse = [
  { order_id: "ORD-2026-0001", total: 129.99, paid: true },
  { order_id: "ORD-2026-0002", total: 42.5 },
];

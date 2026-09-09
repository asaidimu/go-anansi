---
title: "Projections"
description: "Declare projections in schema metadata, generate them as DTOs, read/write field subsets through ReadAs, CreateFrom, UpdateFrom. Required overrides and custom tags."
---

# Projections

Projections are declared under `metadata.projections.<Name>.fields` in a
schema. Each entry describes a field subset and optional constraint/tag
overrides:

```json
{
  "metadata": {
    "projections": {
      "ProductSummary": { "fields": { "include": ["name", "price"] } },
      "ProductCreate":  {
        "fields": { "include": ["name", "price", "stock"] },
        "required": ["name", "price"]
      },
      "ProductUpdate": {
        "fields": { "exclude": ["_id_", "_metadata_"] }
      }
    }
  }
}
```

## What you get

Codegen emits one struct per projection, embedding `document.DocumentModel`:

```go
type ProductSummary struct {
    document.DocumentModel
    Name  string  `anansi:"name,required=true"  json:"name"`
    Price float64 `anansi:"price,required=true" json:"price"`
}

type ProductCreate struct {
    document.DocumentModel
    Name  string  `anansi:"name,required=true"  json:"name"`
    Price float64 `anansi:"price,required=true" json:"price"`
    Stock int64   `anansi:"stock,required=false" json:"stock,omitempty"`
}
```

These structs satisfy `data.DocumentModelProvider` and are valid type
arguments for the shape methods on `*collection.ModelCollection[P]`.

## Read, create, update by shape

The caller picks the operation and the shape at the call site — no
per-projection accessors are generated:

```go
// Read as a summary shape (only name/price bound)
q := query.NewQueryBuilder().Where("name").Eq("Laptop").Build()
summaries, err := productsModel.ReadAs[*products.ProductSummary](ctx, &q)
if err != nil {
    log.Fatalf("read as: %v", err)
}
for _, s := range summaries {
    fmt.Printf("%s = $%.2f\n", s.Name, s.Price)
}

// Create from a create shape
created, err := productsModel.CreateFrom[*products.ProductCreate](ctx,
    &products.ProductCreate{Name: "Mouse", Price: 25.00, Stock: 200})
if err != nil {
    log.Fatalf("create from: %v", err)
}

// Update from an update shape (partial; system fields untouched)
updated, err := productsModel.UpdateFrom[*products.ProductUpdate](ctx,
    created.ID, &products.ProductUpdate{Stock: 45})
```

This is why Anansi requires Go 1.27: shape methods are generic
(`ReadAs[R]`, `CreateFrom[R]`, `UpdateFrom[R]`) on a concrete type. Generic
methods on concrete types weren't expressible before Go 1.27.

## The projection keys

| Key | Meaning |
| --- | --- |
| `include` / `exclude` | Membership. No `include` ⇒ all root fields minus `exclude`. |
| `required` / `optional` | Override `required` for this projection. Drives value-vs-pointer type and `anansi:"…,required=…"`. |
| `tags` | Custom struct tags per field, with `{prop}` placeholders: `{name}`, `{type}`, `{required}`, `{nullable}`, `{default}`, `{goName}`. |

### Custom tag example

```json
{
  "ProductApi": {
    "fields": { "include": ["name", "price"] },
    "tags": {
      "name":  "validate:\"required\" db:\"name\"",
      "price": "validate:\"required,gt=0\" db:\"price\""
    }
  }
}
```

Generates:

```go
type ProductApi struct {
    document.DocumentModel
    Name  string  `anansi:"name,required=true"  json:"name"     validate:"required" db:"name"`
    Price float64 `anansi:"price,required=true" json:"price"    validate:"required,gt=0" db:"price"`
}
```

## Fail-fast validation

Codegen validates projections **fail-fast**. You'll see these as `ERR_*`
codes with diagnostics:

- Unknown fields in `include` / `exclude` → error.
- `include` ∩ `exclude` non-empty → error.
- `required` ∩ `optional` non-empty → error.
- Tag references to fields outside the final set → error.

Run `anansi schema validate schemas/products.schema.json` to surface these
before codegen.

## Why this design

Projection methods are *generic* (`ReadAs[R]`, `CreateFrom[R]`,
`UpdateFrom[R]`), so adding a new projection doesn't require regenerating
per-shape accessors. You declare the shape in the schema, run codegen, and
pick the operation at the call site.

The alternative — per-shape methods like `ReadProductSummary`,
`CreateProductCreate` — would balloon the API surface and break every time
you added a projection. Generics keep the surface flat.

## Next

[Schema change workflow →](/tutorial/schema-change-workflow) — evolve the
schema over time with the canonical edit → migrate generate → codegen loop.

> **Reference:** the full projection spec, including the tag-grammar and
> the validation rule set, is in
> [Codegen modes](/reference/codegen-modes).

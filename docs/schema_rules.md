# Schema Versioning Model

This document outlines the model for how changes to a schema should impact its semantic version (`MAJOR.MINOR.PATCH`).

## Core Principle

Schema versioning tracks **structural compatibility** — changes to the shape of data, the types of fields, and the consistency rules enforced at the storage layer. It does not track validation logic applied above the structural layer. See [Constraints as a Separate Validation Layer](#constraints-as-a-separate-validation-layer).

---

## PATCH Version Changes
*Backward-compatible, non-functional changes.*
- Changes that do not affect the data shape, validation rules, or consistency guarantees.
- Examples: updating `description` on a field, schema, or index; adding or modifying `metadata`.

## MINOR Version Changes
*Backward-compatible functional additions or relaxations.*
- Additions that existing data and applications can safely ignore, or relaxations that make the schema less strict.
- Examples: adding a new optional field; deprecating a field; adding a value to an `enum`; removing a `unique` index.

## MAJOR Version Changes
*Semantically breaking changes.*
- Any change that could cause existing data to become invalid against the new schema.
- Examples: removing a field; changing a field's type; making an optional field required; removing an enum value.

---

## Changes to a Field

**PATCH**

- **`description`**: Purely metadata; no effect on validation or storage.

**MINOR**

- **`default`**: Adding or changing a default value — backward-compatible fallback for new data.
- **`deprecated`**: Setting `deprecated: true` or reversing it — a developer signal; the field remains functional.
- **`required`**: `true` → `false` — makes the field less strict; all existing data remains valid.
- **`unique`**: `true` → `false` — removes a uniqueness rule; existing data remains valid.
- **`values`** (enum): Adding a new value — extends the allowed set without invalidating existing data.

**MAJOR**

- **`name`**: Renaming a field — effectively a removal and addition; breaks all integrations referencing the field by name.
- **`default`**: Removing a default value — breaks write-path clients that rely on the persistence layer to apply it.
- **`type`**: Changing the field's data type — existing data will likely be invalid against the new type.
- **`required`**: `false` → `true` — existing records without this field become invalid.
- **`unique`**: `false` → `true` — existing duplicate data violates the new uniqueness rule.
- **`values`** (enum): Removing a value — existing records using that value become invalid.

---

## Changes to `field.schema`

`field.schema` expresses the element type, value type, or structural reference for complex fields. It can take three forms: a named ref (`id`), an inline descriptor (`type`/`values`), or an array of named refs (union/composite). Version impact is determined by the **resolved semantic change**, not the structural form used to express it.

### Switching between named ref and inline

Changing from a named ref to an inline (or vice versa) where the **resolved type is identical** is a **PATCH** — the data contract is unchanged. A diff tool must recurse into the referenced schema to confirm equivalence; the structural form change alone is not the trigger.

If the resolved types differ, the change is evaluated as a type change — typically **MAJOR**.

### Single-reference fields (`object`, `record`, `array`, `set`)

- **Changing the referenced schema** (named ref `id` swap, or inline `type` change): evaluate the resolved difference — semantically equivalent is **PATCH**; shape change is **MAJOR**.
- **Adding a schema reference where there was none** (e.g. untyped `record` → typed): **MAJOR** — imposes new structural validation on previously unconstrained data.
- **Removing a schema reference** (typed → untyped): **MAJOR** — removes structural validation, fundamentally altering the contract.
- **Changing `values` on an inline enum**:
  - Adding a value: **MINOR**
  - Removing a value: **MAJOR**

### Union and composite fields (array of named refs)

- **Adding a named ref to the array**: **MINOR** — expands the set of valid shapes; existing data remains valid.
- **Removing a named ref from the array**: **MAJOR** — data conforming to the removed shape becomes invalid.
- **Changing the `id` of an entry**: evaluate as a remove + add against the resolved shapes — compatible is **MINOR**; incompatible is **MAJOR**.
- **Reordering entries**: **PATCH** — order has no semantic meaning for union/composite validation.

### Index augmentation on a schema reference

Changes to `indexes` attached to a `field.schema` reference follow the same rules as top-level index changes defined below.

---

## Changes to Indexes

Version impact is scoped to changes that affect data consistency — primarily uniqueness. Performance-only changes (non-unique indexes) are always PATCH.

**MAJOR**

1. **Adding a new `unique` index** — imposes a new consistency rule that existing non-unique data may violate.
2. **Making an existing index `unique`** (`unique: false` → `true`) — same as adding a unique index.
3. **Changing the `fields` of an existing `unique` index** — alters the scope of the uniqueness constraint.
4. **Changing the `condition` of an existing `unique` index** — changes which data the consistency rule applies to.

**MINOR**

1. **Removing an existing `unique` index** — removes a consistency rule; schema becomes less restrictive.
2. **Making a `unique` index non-unique** (`unique: true` → `false`) — same as removing a unique index.

**PATCH**

1. **Adding or removing a non-unique index** — performance optimization; no consistency impact.
2. **Any change to a non-unique index** (fields, order, type, condition) — performance only.
3. **Changing the `description` or `name` of any index** — metadata; no effect on consistency rules.

---

## Changes to Top-Level Schema Properties

- **`description`**: **PATCH** — metadata only.
- **`metadata`**: **PATCH** — metadata only.
- **`concrete`**: **MAJOR** — changes whether the schema maps to a physical collection; affects storage and query behavior.

---

## Changes to the `fields` Collection

- **Adding an optional field** (not required, or has a `default`): **MINOR** — backward-compatible; existing data remains valid.
- **Adding a required field without a `default`**: **MAJOR** — existing records lack this field and become invalid.
- **Removing a field**: **MAJOR** — existing data carrying this field no longer conforms to the schema.

---

## Nested Schema Changes

Nested schemas do not have independent versions. The version calculation for a top-level schema is recursive — the diff process descends into nested schema definitions and applies the same rules. The highest impact level found anywhere in the structure (`MAJOR` > `MINOR` > `PATCH`) determines the final version bump for the parent schema.

**MAJOR** if the nested schema has:
- A required field added, or any field removed.
- Its own `type` changed (e.g. `string` → `number` for a Type mode schema).
- Any MAJOR change to its `fields` or `indexes`.

**MINOR** if no MAJOR changes and the nested schema has:
- An optional field added, or any MINOR change to its `fields` or `indexes`.

**PATCH** if only:
- `description` or `metadata` changed.
- Any property whose isolated impact is PATCH-only.

---

## Constraints as a Separate Validation Layer

Constraints are not part of the structural schema and are excluded from schema versioning.

A schema defines the **shape** of data — types, field presence, collection structure, and uniqueness rules. Constraints define **rules over data** that already conforms to that shape. These are orthogonal concerns. A constraint predicate may reference business logic, regulatory rules, or domain-specific conditions that evolve independently of the data model entirely — a change in tax legislation can render previously valid integers invalid without any change to the schema definition.

Attempting to encode constraint changes in schema versions would couple structural versioning to a dynamic, externally-driven validation layer — producing version noise that carries no meaningful signal about data model compatibility.

**Constraints should be versioned and managed independently**, under whatever model suits the domain. The schema version communicates nothing about constraint compatibility, and consumers that depend on specific constraint behavior must manage that contract separately.

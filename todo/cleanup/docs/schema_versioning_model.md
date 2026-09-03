# Schema Versioning Model

This document outlines a proposed model for how changes to a schema should impact its semantic version (`MAJOR.MINOR.PATCH`).

## PATCH Version Changes
*Backward-compatible bug fixes or non-functional changes.*
*   **What:** Changes that don't affect the data shape or validation rules.
*   **Examples:**
    *   Updating `description` on a field, schema, or index.
    *   Adding or modifying `metadata` or `hint` properties.

## MINOR Version Changes
*Adding backward-compatible functionality.*
*   **What:** Additions that existing applications can safely ignore.
*   **Examples:**
    *   Adding a new field that is **not required** (i.e., it's optional or has a `default` value).
    *   Deprecating a field by setting `deprecated: true`.
    *   Adding a new value to an `enum`.

## MAJOR Version Changes
*Incompatible (breaking) API changes.*
*   **What:** Any change that could cause existing data to become invalid or require applications to be updated.
*   **Examples:**
    *   **Removing** a field.
    *   **Renaming** a field.
    *   Changing a field's `type` (e.g., from `string` to `number`).
    *   Making an optional field **`required`**.
    *   Adding a new `required` field without a `default` value.
    *   Making a constraint more restrictive (e.g., reducing a `maxLength`).
    *   Removing a value from an `enum`.

### Changes to a Field:

**PATCH Changes (Non-breaking, No Functional Impact)**

*   **`description`**: Modifying the text description.
    *   *Reason*: Purely metadata, no effect on data storage or validation.
*   **`hint`**: Changing UI hints.
    *   *Reason*: Metadata for external tools, does not affect the schema's contract.

**MINOR Changes (Backward-Compatible Additions/Changes)**

*   **`default`**: Adding or changing a default value.
    *   *Reason*: This is a non-breaking, backward-compatible addition that provides a graceful fallback for new data.
*   **`deprecated`**: Setting `deprecated: true` or changing it back to `false`.
    *   *Reason*: A signal for developers; the field remains functional.
*   **`required`**: Changing from `true` to `false`.
    *   *Reason*: Makes the field *less* strict, so all existing data remains valid.
*   **`unique`**: Changing from `true` to `false`.
    *   *Reason*: Removes a constraint, making the schema less strict.
*   **`values`** (for `enum` type): Adding a new value to the list.
    *   *Reason*: Extends the set of allowed values without invalidating existing ones.

**MAJOR Changes (Breaking/Incompatible Changes)**

*   **`name`**: Renaming a field.
    *   *Reason*: Effectively a field removal and addition; breaks all client integrations.
*   **`default`**: Removing a default value from a field.
    *   *Reason*: This is a breaking change for write-path clients who rely on the persistence layer to apply the default.
*   **`type`**: Changing the field's data type (e.g., from `string` to `integer`).
    *   *Reason*: Existing data will likely be invalid.
*   **`itemsType`** (for `array` type): Changing the data type of elements in an array.
    *   *Reason*: Same as changing the `type` of a simple field.
*   **`required`**: Changing from `false` to `true`.
    *   *Reason*: Existing records without this field become invalid.
*   **`unique`**: Changing from `false` to `true`.
    *   *Reason*: Can fail if existing data contains duplicates, representing a new, strict database rule.
*   **`values`** (for `enum` type): Removing a value from the list.
    *   *Reason*: Existing records using that value become invalid.

### Changes to `field.schema` property:

This property defines the structure of nested objects, records, arrays of objects, or unions.

**For Types with a Single Schema Reference (`object`, `record`, `array` of objects):**
When `field.schema` refers to a single `NestedSchemaReference` (via its `id`):

*   **Changing the referenced `id`**: Swapping the `NestedSchemaReference` for a new one (i.e., changing its `id` to point to a different schema).
    *   **Impact: MAJOR**
    *   *Reason:* This fundamentally redefines the field's expected data structure, breaking existing integrations.
*   **Adding a `NestedSchemaReference` where there was none**: Defining a schema for a field that was previously unstructured (e.g., from `any` to a specific object).
    *   **Impact: MAJOR**
    *   *Reason:* Imposes new, strict validation rules, potentially invalidating existing data that did not conform.
*   **Removing a `NestedSchemaReference`**: Making a structured field unstructured.
    *   **Impact: MAJOR**
    *   *Reason:* Removes structural validation, fundamentally altering the contract and potentially changing data validity.

**For `union` Types (Array of Schema References):**
When `field.schema` refers to an array of `NestedSchemaReference` objects (for union types):

*   **Adding a `NestedSchemaReference` to the array**:
    *   **Impact: MINOR**
    *   *Reason:* Expands the set of valid shapes for the field. Existing data remains valid.
*   **Removing a `NestedSchemaReference` from the array**:
    *   **Impact: MAJOR**
    *   *Reason:* Data conforming to the removed shape becomes invalid.
*   **Changing the `id` of a `NestedSchemaReference` within the array**:
    *   **Impact: MAJOR**
    *   *Reason:* Equivalent to a "remove" and "add," breaking compatibility for data matching the old reference.
*   **Reordering the `NestedSchemaReference` objects in the array**:
    *   **Impact: PATCH**
    *   *Reason:* The order typically has no semantic meaning for validation; a non-functional change.

---

### The Versioning Model for Constraints

Given that all predicates are treated as "black boxes" and the `name` field is a unique identifier (to be renamed to `id`):

**MAJOR Changes (Breaking)**

A **MAJOR** version bump is required if you:

1.  **Add a new constraint.**
    *   *Reason:* Imposes a new rule that could invalidate existing data.
2.  **Change the `name` (the identifier) of an existing constraint.**
    *   *Reason:* This is equivalent to removing the old constraint and adding a new one, breaking any logic that references the constraint by its ID.
3.  **Change the `predicate` of an existing constraint.**
    *   *Reason:* The fundamental validation logic is changing in an unpredictable way.
4.  **Change the `parameters` of an existing constraint.**
    *   *Reason:* The inputs to the validation logic are changing, with an unpredictable effect.
5.  **Change the `field` or `fields` a constraint applies to.**
    *   *Reason:* The scope of the validation is changing, which is a fundamental modification.

**MINOR Changes (Backward-Compatible)**

A **MINOR** version bump is required if you:

1.  **Remove an existing constraint.**
    *   *Reason:* This makes the schema *less* restrictive. All previously valid data remains valid. It is a non-breaking, functional change.

**PATCH Changes (Non-Functional)**

A **PATCH** version bump is required if you:

1.  **Change the `description` of an existing constraint.**
    *   *Reason:* Purely a metadata update with no effect on validation.
2.  **Change the `errorMessage` of an existing constraint.**
    *   *Reason:* Changes the output for failed validation but not the logic itself. This is considered a non-functional update.

---

### The Versioning Model for Indexes (Consistency-Focused)

Under this model, only changes that affect data consistency—primarily uniqueness—will trigger a MINOR or MAJOR version bump. All other changes are considered non-functional from the schema's perspective.

**MAJOR Changes (Breaking Consistency Rules)**

A **MAJOR** version bump is required if you:

1.  **Add a new `unique` index.**
    *   *Reason:* Imposes a new consistency rule that could invalidate existing non-unique data.
2.  **Make an existing index `unique`** (changing `unique` from `false` to `true`).
    *   *Reason:* Same as adding a new `unique` index.
3.  **Change the `fields` of an existing `unique` index.**
    *   *Reason:* This alters the nature of the uniqueness constraint, effectively creating a new consistency rule.
4.  **Change the `partial` filter of an existing `unique` index.**
    *   *Reason:* This changes the scope of the consistency rule, which we must treat as a breaking change.

**MINOR Changes (Relaxing Consistency Rules)**

A **MINOR** version bump is required if you:

1.  **Remove an existing `unique` index.**
    *   *Reason:* Removes a consistency rule, making the schema less restrictive.
2.  **Make a unique index non-unique** (changing `unique` from `true` to `false`).
    *   *Reason:* Same as removing a unique index.

**PATCH Changes (No Impact on Consistency)**

A **PATCH** version bump is appropriate for all other changes, including:

1.  **Adding or removing a non-unique index.**
    *   *Reason:* This is a performance optimization with no effect on data consistency.
2.  **Any change to a non-unique index** (e.g., changing its `fields`, `order`, `type`, or `partial` filter).
    *   *Reason:* Performance optimization only.
3.  **Changing the `description` or `name` of any index.**
    *   *Reason:* Metadata or identifier changes that do not alter the consistency rules themselves.

---

### Changes to Top-Level `SchemaDefinition` Properties

*   **`description`**: Modifying the overall description of the schema.
    *   **Impact: PATCH**
    *   *Reason:* Purely metadata; does not affect data shape, validation, or consistency.
*   **`metadata`**: Adding, removing, or modifying key-value pairs in the schema's metadata.
    *   **Impact: PATCH**
    *   *Reason:* Purely metadata for external context or tooling; does not affect consistency or data shape.
*   **`hint`**: Adding, removing, or modifying schema-level hints.
    *   **Impact: PATCH**
    *   *Reason:* Purely metadata for UI generation or tooling; does not affect consistency or data shape.

---

### Changes to the `fields` Collection

This covers adding or removing entire fields from the top-level `SchemaDefinition.Fields` map.

*   **Adding a new field:**
    *   If the new field is **not required** (i.e., optional or has a `default` value).
        *   **Impact: MINOR**
        *   *Reason:* This is a backward-compatible addition of functionality; existing data remains valid.
    *   If the new field is **required** and has **no `default` value**.
        *   **Impact: MAJOR**
        *   *Reason:* Existing records lack this field and will become invalid against the new schema.
*   **Removing an existing field:**
    *   **Impact: MAJOR**
    *   *Reason:* Existing data contains this field, making it invalid or incomplete against the new schema. This is a breaking change.

---

### Semantics for `NestedSchemaDefinition` Changes

Since nested schemas do not have their own independent versions, the version calculation for a top-level `SchemaDefinition` must be recursive. The comparison process "dives into" nested schema definitions and applies the same versioning rules to them as it would to a top-level schema. The highest level of change (`MAJOR` > `MINOR` > `PATCH`) found anywhere in the entire structure determines the final version bump for the parent `SchemaDefinition`.

A change to a nested schema results in a `MAJOR`, `MINOR`, or `PATCH` impact, which then "bubbles up" to the parent schema.

**MAJOR Impact (Breaking Change)**

The change is `MAJOR` if you modify the nested schema by:

*   Adding a required field to, or removing any field from, its `fields` collection.
*   Changing the `type` of the nested schema itself (if it's a typed schema, e.g., from `string` to `number`).
*   Making **any `MAJOR` change to one of its `fields`**, as defined in the "Changes to a Field" model.
*   Making **any `MAJOR` change to its `constraints`**, as defined in the "Versioning Model for Constraints".
*   Making **any `MAJOR` change to its `indexes`**, as defined in the "Versioning Model for Indexes".

**MINOR Impact (Backward-Compatible Change)**

The change is `MINOR` if there are no `MAJOR` changes and you modify the nested schema by:

*   Making **any `MINOR` change to one of its `fields`** (e.g., adding a new optional field).
*   Making **any `MINOR` change to its `constraints`** (i.e., removing a constraint).
*   Making **any `MINOR` change to its `indexes`** (i.e., removing a `unique` index).

**PATCH Impact (Non-Functional Change)**

The change is `PATCH` if there are no `MAJOR` or `MINOR` changes and you only modify:

*   The nested schema's `description` or `metadata`.
*   Any property that results in a `PATCH`-only impact (e.g., changing the `description` of a field, constraint, or index within it; adding a non-unique index).
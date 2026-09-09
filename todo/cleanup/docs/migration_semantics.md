# Migration Semantics

This document defines key concepts related to data modeling and schema evolution.

## Data Model
The data model is the conceptual and logical definition of a system’s data. The data model is independent of representation and storage. It captures meaning, not form.

## Schema
A schema is a formal structural specification of data. A schema describes the shape of data, not necessarily where or how it is stored.

## Migration
A migration is a versioned change to the data model. It represents a transition from one data model version to another and is expressed as a sequence of changes to the schema.

---

### Kinds of Data Model Changes Expressed in Schema (Fields)

#### 1. Adding a New Attribute to the Model

This change signifies that the data model now needs to capture a new piece of information that was not previously part of its definition.

*   **Conceptual Change:** The data model is extended to include a new characteristic or property for the entity it describes.
*   **Schema Expression:** This is expressed in the schema by adding a new `FieldDefinition` to the schema's collection of fields.
*   **`SchemaChange` Type:** `addField`

#### 2. Removing an Existing Attribute from the Model

This change indicates that a previously tracked piece of information is no longer relevant or necessary for the data model.

*   **Conceptual Change:** The data model is simplified by eliminating an existing characteristic or property.
*   **Schema Expression:** This is expressed in the schema by removing an existing `FieldDefinition` from the schema's collection of fields.
*   **`SchemaChange` Type:** `removeField`

#### 3. Modifying an Existing Attribute of the Model

This is a complex category that covers any change to the characteristics of an existing attribute. To avoid ambiguity, modifications are expressed explicitly using `set` and `unset` operations within the `modifyField` change type. The `deprecateField` type also serves as a specific kind of modification.

##### 3.1 Setting a Property 
This is used to change the value of one or more properties of a field.

*   **Conceptual Change:** The attribute `fname` is now conceptually referred to as `firstName`.
*   **Schema Expression:** The `name` property of the `FieldDefinition` is altered.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyField",
      "id": "fname",
      "changes": {
         "name": "firstName" 
      }
    }
    ```

*   **Conceptual Change:** The `email` attribute is now mandatory.
*   **Schema Expression:** The `required` property of the `FieldDefinition` is set to `true`.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyField",
      "id": "email",
      "changes": {
        "required": true 
      }
    }
    ```

##### 3.2 Unsetting a Property (`unset`)
This is used to explicitly remove an optional property from a field, such as a `default` value.

*   **Conceptual Change:** The `status` field should no longer have a default value of 'pending'.
*   **Schema Expression:** The `default` property is removed from the `status` field definition.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyField",
      "id": "status",
      "changes": {
        "unset": ["default"]
      }
    }
    ```

##### 3.3 Signaling Deprecation
This indicates that an attribute is considered obsolete and will be removed in a future version of the data model. It uses a dedicated, semantic change type.

*   **Conceptual Change:** The `legacy_id` attribute is being phased out and should no longer be used by new implementations.
*   **Schema Expression:** The `deprecated` property of the `FieldDefinition` is set to `true`.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "deprecateField",
      "id": "legacy_id"
    }
    ```

---
### Modifications for Complex Field Types

#### 1. Enumerations (Enums)

Enums define a field whose value must be one of a predefined set. This set can either be a list of literal values or a choice between different schema types.

##### 1.1 Literal Value Enums (using `values` array)

This involves changing the allowed set of simple string or integer values for a field.

*   **Conceptual Change:** The data model for `OrderStatus` needs to be updated. It must now allow a new "RETURNED" status, and the old "ARCHIVED" status is no longer valid.
*   **Schema Expression:** The `values` array within the `FieldDefinition` is modified.
*   **Example:**
    *   **Old Field Definition:**
        ```json
        {
          "name": "orderStatus",
          "type": "enum",
          "values": ["PENDING", "SHIPPED", "DELIVERED", "ARCHIVED"]
        }
        ```
    *   **New Field Definition:**
        ```json
        {
          "name": "orderStatus",
          "type": "enum",
          "values": ["PENDING", "SHIPPED", "DELIVERED", "RETURNED"]
        }
        ```
    *   **Resulting `SchemaChange`:**
        ```json
        {
          "type": "modifyField",
          "id": "orderStatus",
          "changes": {
              "values": ["PENDING", "SHIPPED", "DELIVERED", "RETURNED"]
          }
        }
        ```

##### 1.2 Schema-Referenced Enums (Union of Schemas using `schema` property)

This conceptual "enum" allows a field to hold data that conforms to one of several predefined nested schema structures. This is typically achieved using the `FieldTypeUnion` type.

*   **Conceptual Change:** A `PaymentMethod` can now also be a 'GiftCard', in addition to 'CreditCard' or 'PayPal'.
*   **Schema Expression:** The `schema` property (which holds an array of `NestedSchemaReference` objects) within the `FieldDefinition` is modified.
*   **Example:**
    *   **Old Field Definition:**
        ```json
        {
          "name": "paymentMethod",
          "type": "union",
          "schema": [
            { "id": "CreditCardSchema" },
            { "id": "PayPalSchema" }
          ]
        }
        ```
    *   **New Field Definition:**
        ```json
        {
          "name": "paymentMethod",
          "type": "union",
          "schema": [
            { "id": "CreditCardSchema" },
            { "id": "PayPalSchema" },
            { "id": "GiftCardSchema" }
          ]
        }
        ```
    *   **Resulting `SchemaChange`:**
        ```json
        {
          "type": "modifyField",
          "id": "paymentMethod",
          "changes": {
              "schema": [
                { "id": "CreditCardSchema" },
                { "id": "PayPalSchema" },
                { "id": "GiftCardSchema" }
              ]
          }
        }
        ```

#### 2. Arrays

This involves changing the type of items that a list is expected to hold, or changing a field from a scalar to an array.

*   **Conceptual Change:** A product's list of `tag_ids` should now store numbers instead of strings for better database indexing.
*   **Schema Expression:** The `itemsType` property of the array's `FieldDefinition` is altered.
*   **Example:**
    *   **Old Field Definition:**
        ```json
        { "name": "tag_ids", "type": "array", "itemsType": "string" }
        ```
    *   **New Field Definition:**
        ```json
        { "name": "tag_ids", "type": "array", "itemsType": "integer" }
        ```
    *   **Resulting `SchemaChange`:**
        ```json
        {
          "type": "modifyField",
          "id": "tag_ids",
          "changes": {
             "itemsType": "integer" 
          }
        }
        ```

#### 3. Objects

This typically involves changing a simple field into a structured object, or changing the structure that an object field points to.

*   **Conceptual Change:** A `user`'s `address`, which was a single string, now needs to be a structured object with separate `street` and `city` attributes.
*   **Schema Expression:** The field's `type` is changed to `object`, and the `schema` property is added to reference a `NestedSchemaDefinition`.
*   **Example:**
    *   **Old Field Definition:**
        ```json
        { "name": "address", "type": "string" }
        ```
    *   **New Field Definition:**
        ```json
        {
          "name": "address",
          "type": "object",
          "schema": { "id": "AddressV2" }
        }
        ```
    *   **Resulting `SchemaChange`:**
        ```json
        {
          "type": "modifyField",
          "id": "address",
          "changes": {
              "type": "object",
              "schema": { "id": "AddressV2" }
          }
        }
        ```

#### 4. Modifying Field-Specific Schema References

When a field's `type` is `object`, `array`, `record`, or `union`, it references one or more `NestedSchemaReference` objects via its `schema` property. While `modifyField` with can replace the entire `NestedSchemaReference`, it doesn't allow for granular modifications (e.g., adding a single constraint or index) without overwriting the whole reference.

To address this, the `modifySchemaReference` change type allows for applying a list of `SchemaChange` objects to a specific `NestedSchemaReference` that is part of a `FieldDefinition`. This is particularly useful for adding or removing constraints and indexes that are specific to a field's usage of a schema, without affecting the global definition of that schema.

*   **Conceptual Change:** "For the `shipping_address` field specifically, we need to add a constraint that its `country` attribute must be 'USA', without affecting other usages of the `Address` schema."
*   **Schema Expression:** A `modifySchemaReference` change targets the `shipping_address` field and applies an `addConstraint` change to its embedded `NestedSchemaReference`.
*   **`SchemaChange` Type:** `modifySchemaReference`
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifySchemaReference",
      "field": "shipping_address",
      "id": "Address", // Refers to the ID of the NestedSchemaReference within shipping_address
      "changes": [
        {
          "type": "addConstraint",
          "constraint": {
            "name": "shipping_must_be_usa",
            "predicate": "equals",
            "field": "country",
            "parameters": { "value": "USA" }
          }
        }
      ]
    }
    ```

---
### Kinds of Data Model Changes Expressed in Schema (Indexes)

Based on the philosophy that schema evolution is primarily concerned with data consistency, not performance.

#### 1. Adding a Performance Index

This change does not alter the logical data model but provides hints to the underlying storage for optimization.

*   **Conceptual Change:** "We need to improve the performance of queries that filter by the `email` attribute."
*   **Schema Expression:** A new, **non-unique** `IndexDefinition` is added to the schema's `indexes` list.
*   **`SchemaChange` Type:** `addIndex`
*   **Example:**
    ```json
    {
      "type": "addIndex",
      "definition": {
        "name": "idx_email",
        "fields": ["email"],
        "type": "normal"
      }
    }
    ```

#### 2. Removing a Performance Index

This change removes a performance hint that is no longer deemed necessary. It also has no effect on the logical data model.

*   **Conceptual Change:** "The performance index on `last_login_date` is no longer providing benefits."
*   **Schema Expression:** A **non-unique** `IndexDefinition` is removed from the schema's `indexes` list.
*   **`SchemaChange` Type:** `removeIndex`
*   **Example:**
    ```json
    {
      "type": "removeIndex",
      "name": "idx_last_login_date"
    }
    ```

#### 3. Modifying an Existing Index

This alters the properties of an existing index. To avoid ambiguity, modifications are expressed explicitly using direct assignment or `unset` operations within the `modifyIndex` change type.

##### 3.1 Setting Index Properties (`set`)
This is used to change the value of one or more properties of an index.

*   **Conceptual Change:** "The `idx_order_lines` index should now be unique and cover both `order_id` and `line_item_id`."
*   **Schema Expression:** The `unique` and `fields` properties of the `IndexDefinition` are altered.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyIndex",
      "name": "idx_order_lines",
      "changes": {
          "unique": true,
          "fields": ["order_id", "line_item_id"]
      }
    }
    ```

##### 3.2 Unsetting Index Properties (`unset`)
This is used to explicitly remove an optional property from an index, such as its partial filter.

*   **Conceptual Change:** "The partial filter on the `idx_active_users` index is no longer needed."
*   **Schema Expression:** The `partial` property is removed from the `IndexDefinition`.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyIndex",
      "name": "idx_active_users",
      "changes": {
        "unset": ["partial"]
      }
    }
    ```

---
### Relationship Between Indexes and Fields

There are two primary ways a field definition implies an index:

#### 1. The `unique` Property on a Field Definition

When the `unique` property is set on a `FieldDefinition`, it directly modifies that field's attributes to enforce a uniqueness constraint on its values.

*   **Conceptual Change:** "The `email` attribute for this field must be unique across all records within the data model."
*   **Schema Expression:** This is a direct modification of the `FieldDefinition` itself, where the `unique` boolean property is set.
*   **Resulting `SchemaChange`:** This change is directly expressed using the `modifyField` type.
    ```json
    {
      "type": "modifyField",
      "id": "email",
      "changes": {
        "set": { "unique": true }
      }
    }
    ```

#### 2. Indexes Defined Within a Field's Schema Reference

When a field's `type` is `object` or `array`, it references a `NestedSchemaDefinition` via its `schema` property. The `NestedSchemaReference` within that property has an `indexes` array, allowing you to define indexes that are specific to that field's usage of the nested schema.

*   **Conceptual Change:** "For the `billing_address` field specifically, we need to add an index on the `zip_code`."
*   **Schema Expression:** The `indexes` array within the `NestedSchemaReference` object is modified.
*   **Resulting `SchemaChange`:** This change is a direct modification of the `billing_address` field, updating its `schema` property.
    ```json
    {
      "type": "modifyField",
      "id": "billing_address",
      "changes": {
        "set": {
          "schema": {
            "id": "Address",
            "indexes": [
              {
                "index": {
                  "name": "idx_billing_zip",
                  "fields": ["zip_code"],
                  "type": "normal"
                }
              }
            ]
          }
        }
      }
    }
    ```

---
### Kinds of Data Model Changes Expressed in Schema (Constraints)

This covers the lifecycle of a business rule or validation semantic within the data model.

#### 1. Adding a New Constraint

This introduces a new business rule to the data model.

*   **Conceptual Change:** "A `username` must now be at least 3 characters long."
*   **Schema Expression:** A new `Constraint` object is added to a `constraints` list within the schema.
*   **`SchemaChange` Type:** `addConstraint`
*   **Example:**
    ```json
    {
      "type": "addConstraint",
      "constraint": {
        "name": "username_min_length",
        "predicate": "minLength",
        "field": "username",
        "parameters": { "value": 3 },
        "errorMessage": "Username must be at least 3 characters."
      }
    }
    ```

#### 2. Removing an Existing Constraint

This represents a relaxation of the data model's rules.

*   **Conceptual Change:** "We no longer require `usernames` to adhere to a specific regex pattern."
*   **Schema Expression:** An existing `Constraint` is removed from a `constraints` list, identified by its unique `name`.
*   **`SchemaChange` Type:** `removeConstraint`
*   **Example:**
    ```json
    {
      "type": "removeConstraint",
      "name": "username_regex_pattern"
    }
    ```

#### 3. Modifying an Existing Constraint

This alters the properties of an existing rule. To avoid ambiguity, modifications are expressed explicitly using `set` and `unset` operations within the `modifyConstraint` change type.

##### 3.1 Setting Constraint Properties (`set`)
This is used to change the value of one or more properties of a constraint.

*   **Conceptual Change:** "The `username_min_length` constraint should now have a parameter of `5`."
*   **Schema Expression:** The `parameters` of an existing `Constraint` object are changed.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyConstraint",
      "name": "username_min_length",
      "changes": {
        "set": {
          "parameters": { "value": 5 }
        }
      }
    }
    ```

##### 3.2 Unsetting Constraint Properties (`unset`)
This is used to explicitly remove an optional property from a constraint, such as a custom error message.

*   **Conceptual Change:** "The `password_match` constraint should no longer use a custom error message, and should revert to the default."
*   **Schema Expression:** The `errorMessage` property is removed from the `Constraint` definition.
*   **Example `SchemaChange`:**
    ```json
    {
      "type": "modifyConstraint",
      "name": "password_match",
      "changes": {
        "unset": ["errorMessage"]
      }
    }
    ```

---
### Relationship Between Constraints and Fields

This explains how constraints can be applied either directly to a field or at a higher level to target specific fields.

#### 1. Constraints Defined Directly on a Field

A `FieldDefinition` has a `constraints` property, which is a list of rules that apply only to that specific field.

*   **Conceptual Change:** "The `age` field must contain a value greater than or equal to 18."
*   **Schema Expression:** A `Constraint` is added to the `constraints` array *inside* the `age` field's definition.
*   **Example:**
    *   **New Field Definition:**
        ```json
        {
          "name": "age",
          "type": "integer",
          "constraints": [
            {
              "constraint": {
                "name": "age_gte_18",
                "predicate": "gte",
                "parameters": { "value": 18 }
              }
            }
          ]
        }
        ```
*   **Resulting `SchemaChange`:** This is expressed as a **`modifyField`** operation because you are changing the definition of the `age` field itself.
    ```json
    {
      "type": "modifyField",
      "id": "age",
      "changes": {
          "constraints": [
            {
              "constraint": {
                "name": "age_gte_18",
                "predicate": "gte",
                "parameters": { "value": 18 }
              }
            }
          ]
      }
    }
    ```

#### 2. Constraints Defined at the Schema Level that Target Fields

A constraint can be defined at the top level of the schema and then target one or more fields using its own `field` or `fields` properties. This is often used for cross-field validation.

*   **Conceptual Change:** "The `password_confirmation` field must match the value of the `password` field."
*   **Schema Expression:** A single `Constraint` is added to the top-level `constraints` list of the schema.
*   **Example:**
    ```json
    // In the top-level SchemaDefinition
    "constraints": [
      {
        "constraint": {
          "name": "password_must_match",
          "predicate": "fieldsEqual",
          "fields": ["password", "password_confirmation"]
        }
      }
    ]
    ```
*   **Resulting `SchemaChange`:** This is expressed as an **`addConstraint`** operation on the schema.
    ```json
    {
      "type": "addConstraint",
      "constraint": {
        "name": "password_must_match",
        "predicate": "fieldsEqual",
        "fields": ["password", "password_confirmation"]
      }
    }
    ```
---
### Constraints Defined Within a Field's Schema Reference

This describes how constraints can be applied to a nested schema specifically at the point where it is referenced by a parent field.

*   **Conceptual Change:** "For a `shipping_address` specifically, the `country` attribute inside the address must be 'USA'. This rule should not apply to a `billing_address`."
*   **Schema Expression:** A new constraint is added to the `constraints` array *inside the `schema` property* of the parent field definition.
*   **Example:**
    *   **Old Field Definition:**
        ```json
        {
          "name": "shipping_address",
          "type": "object",
          "schema": { "id": "Address" }
        }
        ```
    *   **New Field Definition:**
        ```json
        {
          "name": "shipping_address",
          "type": "object",
          "schema": {
            "id": "Address",
            "constraints": [
              {
                "constraint": {
                  "name": "shipping_must_be_usa",
                  "predicate": "equals",
                  "field": "country",
                  "parameters": { "value": "USA" }
                }
              }
            ]
          }
        }
        ```
*   **Resulting `SchemaChange`:** This is expressed as a **`modifyField`** operation on the `shipping_address` field, as you are directly changing that field's definition.
    ```json
    {
      "type": "modifyField",
      "id": "shipping_address",
      "changes": {
        "set": {
          "schema": {
            "id": "Address",
            "constraints": [
              {
                "constraint": {
                  "name": "shipping_must_be_usa",
                  "predicate": "equals",
                  "field": "country",
                  "parameters": { "value": "USA" }
                }
              }
            ]
          }
        }
      }
    }
    ```

---
### Modifying Nested Schemas

The logic for modifying a nested schema is perfectly analogous to modifying a top-level schema. A `NestedSchemaDefinition` is effectively a complete schema designed to be reusable. Changes to it are wrapped in one of three dedicated `SchemaChange` types.

#### 1. Adding a New Nested Schema

This introduces a new, reusable component to your data model's "library."

*   **Conceptual Change:** "We need a new, standard `Address` component that can be used in various parts of our data model."
*   **`SchemaChange` Type:** `addSchema`
*   **Example:**
    ```json
    {
      "type": "addSchema",
      "id": "Address",
      "definition": {
        "name": "Address",
        "fields": {
          "street": { "type": "string" },
          "city": { "type": "string" }
        }
      }
    }
    ```

#### 2. Removing a Nested Schema

This removes a reusable component from your data model's "library."

*   **Conceptual Change:** "The old `LegacyAddress` component is no longer used and should be removed."
*   **`SchemaChange` Type:** `removeSchema`
*   **Example:**
    ```json
    {
      "type": "removeSchema",
      "id": "LegacyAddress"
    }
    ```

#### 3. Modifying a Nested Schema

This alters an existing nested schema. The `modifySchema` change type identifies *which* nested schema to alter, and its `changes` property contains a list of the familiar `SchemaChange` objects to apply *to that nested schema*.

*   **Conceptual Change:** "The standard `Address` component needs a new, optional `country` field."
*   **`SchemaChange` Type:** `modifySchema`
*   **Example:**
    ```json
    {
      "type": "modifySchema",
      "id": "Address",
      "changes": [
        {
          "type": "addField",
          "id": "country",
          "definition": {
            "name": "country",
            "type": "string",
            "required": false
          }
        }
      ]
    }
    ```

---
### Modifying Schema Properties

The `modifyProperty` change type is used to alter the simple, top-level properties of a schema itself, such as its `description` or `metadata`.

#### 1. Modifying a Top-Level Schema's Properties

*   **Conceptual Change:** "The description of the entire `User` schema is outdated."
*   **Schema Expression:** The `description` property at the top level of the `SchemaDefinition` is changed.
*   **`SchemaChange` Type:** `modifyProperty`
*   **Example:**
    ```json
    {
      "type": "modifyProperty",
      "id": "description",
      "value": "Represents a customer in the e-commerce system."
    }
    ```

#### 2. Modifying a Nested Schema's Properties

This is expressed by nesting a `modifyProperty` change inside a `modifySchema` change.

*   **Conceptual Change:** "The description for the reusable `Address` component needs clarification."
*   **Schema Expression:** The `description` property within the `Address` `NestedSchemaDefinition` is changed.
*   **`SchemaChange` Type:** A `modifyProperty` change nested within a `modifySchema` change.
*   **Example:**
    ```json
    {
      "type": "modifySchema",
      "id": "Address",
      "changes": [
        {
          "type": "modifyProperty",
          "id": "description",
          "value": "A reusable component for storing street and city information."
        }
      ]
    }
    ```

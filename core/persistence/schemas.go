// Package persistence provides the internal schema management for the persistence layer.
// This includes the schema for the `_schemas` collection, which is used to store the
// definitions of all other collections.
package persistence

import (
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// SCHEMA_COLLECTION_NAME is the constant name for the internal collection that
// stores the schema definitions for all other collections.
const SCHEMA_COLLECTION_NAME = "_schemas"

// SchemaRecord represents the structure of a document in the `_schemas` collection.
// Each record holds the definition of a single collection's schema.
type NameRecord struct {
	Logical  string `json:"logical"`  // The name of the collection this schema defines.
	Physical string `json:"physical"` // The name of the collection this schema defines.

}
type SchemaRecord struct {
	Name        NameRecord              `json:"name"`                  // The name of the collection this schema defines.
	Description string                  `json:"description,omitempty"` // A human-readable description of the schema.
	Version     string                  `json:"version"`               // The version of the schema.
	Schema      schema.SchemaDefinition `json:"schema"`                // The full schema definition, stored as a raw JSON message.
}

// schemasCollectionSchema is the JSON definition for the `_schemas` collection itself.
// This schema is used to create and validate the collection that stores all other schemas.
var schemasCollectionSchema = []byte(`
{
  "name": "_schemas",
  "version": "1.0.0",
  "description": "Stores schema definitions for all collections in the database.",
  "fields": {
    "identifiers": {
      "name": "name",
      "type": "object",
      "required": true,
      "description": "Contains identifying information for the collection.",
      "schema": {
        "id": "CollectionIdentifiersSchema"
      }
    },
    "version": {
      "name": "version",
      "type": "string",
      "required": true,
      "description": "The version of the schema."
    },
    "description": {
      "name": "description",
      "type": "string",
      "description": "A description of the schema."
    },
    "schema": {
      "name": "schema",
      "type": "record",
      "required": true,
      "description": "The full schema definition as a JSON object."
    }
  },
  "nestedSchemas": {
    "CollectionIdentifiersSchema": {
      "name": "CollectionIdentifiersSchema",
      "description": "Defines the structure for collection identifying information.",
      "fields": {
        "logical": {
          "name": "logical",
          "type": "string",
          "required": true,
          "description": "The logical name of the collection this schema defines."
        },
        "physical": {
          "name": "physical",
          "type": "string",
          "required": false,
          "description": "The physical name of the collection in the database."
        }
      }
    }
  },
  "indexes": [
    {
      "name": "name_index",
      "fields": ["name.logical"],
      "type": "normal",
      "description": "Index on schema name for quick lookup."
    },
    {
      "name": "version_index",
      "fields": ["version"],
      "type": "normal",
      "description": "Index on schema version for quick lookup."
    },
    {
      "name": "name_version_unique",
      "fields": ["name.logical", "version"],
      "type": "unique",
      "description": "Ensures unique combination of schema name and version."
    }
  ]
}
`)

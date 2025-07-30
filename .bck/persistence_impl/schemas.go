// Package persistence provides the internal schema management for the persistence layer.
// This includes the schema for the `_schemas` collection, which is used to store the
// definitions of all other collections.
package persistence

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// REGISTRY_COLLECTION_NAME is the constant name for the internal collection that
// stores the schema definitions for all other collections.
const REGISTRY_COLLECTION_NAME = "_schemas_"

// CollectionVersionRecord represents a historical version of a collection's schema and its physical name.
type CollectionVersionRecord struct {
	Version  string                  `json:"version"`  // The version of the schema.
	Physical string                  `json:"physical"` // The physical name of the collection in the database for this version.
	Schema   schema.SchemaDefinition `json:"schema"`   // The full schema definition for this version.
}

type SchemaRecord struct {
	Name          string                             `json:"name"`                  // The name of the collection this schema defines.
	Description   string                             `json:"description,omitempty"` // A human-readable description of the schema.
	ActiveVersion string                             `json:"activeVersion"`         // The current active version of the schema, pointing to an entry in 'Versions'.
	Versions      map[string]CollectionVersionRecord `json:"versions,omitempty"`    // A map of all schema versions, keyed by version string.
	Version       int64                              `json:"_version_"`             // The name of the collection this schema defines.
}

var VersionFieldDefinition = schema.FieldDefinition{
	Name:        schema.VersionFieldName,
	Type:        schema.FieldTypeString,
	Required:    utils.BoolPtr(true),
	Description: utils.StringPtr("The row version"),
}

// registryCollectionSchemaJson is the JSON definition for the `_schemas` collection itself.
// This schema is used to create and validate the collection that stores all other schemas.

var RegistryCollectionSchemaJson = fmt.Appendf(nil, `
{
  "name": "%s",
  "version": "1.0.0",
  "description": "Stores schema definitions for all collections in the database.",
  "fields": {
    "9154fa68-edd1-4c58-8e6f-c05f6d591214": {
      "name": "name",
      "type": "string",
      "description": "The logical name of the schema."
    },
    "a425c61b-1f20-4049-868e-7f1ef805cfb5": {
      "name": "description",
      "type": "string",
      "description": "A description of the schema."
    },
    "d107ad5a-b888-44c5-99dc-46f822eb84d4": {
      "name": "version",
      "type": "string",
      "required": true,
      "description": "The current active version of the schema."
    },
    "3f0575db-a46d-4894-b6d1-19ed82a627da": {
      "name": "versions",
      "type": "record",
  	  "schema": {
        "id": "SchemaVersions"
  	  },
      "required": false,
      "description": "A list of legacy schemas, their physical name & their corresponding schema."
    },
  },
  "nestedSchemas": {
    "SchemaVersions": {
      "name": "SchemaVersions",
      "description": "A list of legacy schemas, their physical name & their corresponding schema.",
      "fields": {
        "ad90c274-3dc1-4025-b532-bf1f5de459ac": {
          "name": "physical",
          "type": "string",
          "required": false,
          "description": "The physical name of the collection in the database."
        },
    	"bbccaf92-9107-40f4-93d2-96c1ca7d09d6": {
    	  "name": "schema",
    	  "type": "record",
    	  "required": true,
    	  "description": "The full schema definition as a JSON object."
    	}
      }
    }
  },
  "indexes": [
    {
      "name": "name_index",
      "fields": ["name"],
      "type": "primary",
      "description": "Index on schema name for quick lookup."
    }
  ]
}
`, REGISTRY_COLLECTION_NAME)

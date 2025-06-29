// Package persistence provides the internal schema management for the persistence layer.
// This includes the schema for the `_schemas` collection, which is used to store the
// definitions of all other collections.
package persistence

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v5/core/schema"
)

// SCHEMA_COLLECTION_NAME is the constant name for the internal collection that
// stores the schema definitions for all other collections.
const SCHEMA_COLLECTION_NAME = "_schemas"

// SchemaRecord represents the structure of a document in the `_schemas` collection.
// Each record holds the definition of a single collection's schema.
type SchemaRecord struct {
	Name        string          `json:"name"`                  // The name of the collection this schema defines.
	Description string          `json:"description,omitempty"` // A human-readable description of the schema.
	Version     string          `json:"version"`               // The version of the schema.
	Schema      json.RawMessage `json:"schema"`                // The full schema definition, stored as a raw JSON message.
}

// schemasCollectionSchema is the JSON definition for the `_schemas` collection itself.
// This schema is used to create and validate the collection that stores all other schemas.
var schemasCollectionSchema = []byte(`
{
  "name": "_schemas",
  "version": "1.0.0",
  "description": "Stores schema definitions for all collections in the database.",
  "fields": {
    "name": {
      "name": "name",
      "type": "string",
      "required": true,
      "description": "The name of the collection this schema defines."
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
  "indexes": [
    {
      "name": "id_primary_key",
      "fields": ["name"],
      "type": "primary"
    },
    {
      "name": "name_index",
      "fields": ["name"],
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
      "fields": ["name", "version"],
      "type": "unique",
      "description": "Ensures unique combination of schema name and version."
    }
  ]
}`)

// mapToSchemaRecord converts a generic schema.Document (map[string]any) into a
// structured SchemaRecord. This is done by marshaling the map to JSON and then
// unmarshaling it into the SchemaRecord struct.
func mapToSchemaRecord(data schema.Document) (*SchemaRecord, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to JSON: %w", err)
	}

	var record SchemaRecord
	if err := json.Unmarshal(jsonBytes, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to SchemaRecord: %w", err)
	}

	return &record, nil
}

// schemaRecordToMap converts a SchemaRecord struct into a generic schema.Document
// (map[string]any). This is useful for when the data needs to be passed to a
// method that expects a generic document.
func schemaRecordToMap(record *SchemaRecord) (map[string]any, error) {
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SchemaRecord to JSON: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to map[string]any: %w", err)
	}

	return data, nil
}

// schemaToRawJSON marshals a schema.SchemaDefinition into a raw JSON message.
// This is used to store the schema definition within a SchemaRecord.
func schemaToRawJSON(s *schema.SchemaDefinition) ([]byte, error) {
	jsonBytes, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SchemaDefinition to JSON: %w", err)
	}
	return jsonBytes, nil
}


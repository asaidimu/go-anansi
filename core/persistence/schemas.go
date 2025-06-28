package persistence

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/core"
)

const SCHEMA_COLLECTION_NAME = "_schemas"

type SchemaRecord struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Version     string          `json:"version"`
	Schema      json.RawMessage `json:"schema"`
}

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

func mapToSchemaRecord(data core.Document) (*SchemaRecord, error) {
	// 1. Marshal the map[string]any into JSON bytes
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to JSON: %w", err)
	}

	// 2. Unmarshal the JSON bytes into the SchemaRecord struct
	var record SchemaRecord
	err = json.Unmarshal(jsonBytes, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to SchemaRecord: %w", err)
	}

	return &record, nil
}

func schemaRecordToMap(record *SchemaRecord) (map[string]any, error) {
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SchemaRecord to JSON: %w", err)
	}
	var data map[string]any
	err = json.Unmarshal(jsonBytes, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to map[string]any: %w", err)
	}
	return data, nil
}

func schemaToRawJson(schema *core.SchemaDefinition) ([]byte, error) {
	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SchemaDefinition to JSON: %w", err)
	}
	return jsonBytes, nil
}


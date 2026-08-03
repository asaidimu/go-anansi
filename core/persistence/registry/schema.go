package registry

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// REGISTRY_COLLECTION_NAME is the constant name for the internal collection that
// stores the schema definitions for all other collections.
const REGISTRY_COLLECTION_NAME = "_schemas_"

var RegistryCollectionSchemaJson = fmt.Sprintf(`
{
  "name": "%s",
  "version": "1.0.0",
  "description": "Stores schema definitions for all collections in the database.",
  "fields": {
    "019f4066-0000-7000-8000-000000000001": {
      "name": "name",
      "type": "string",
      "description": "The logical name of the schema."
    },
    "019f4066-0000-7000-8000-000000000002": {
      "name": "description",
      "type": "string",
      "description": "A description of the schema."
    },
    "019f4066-0000-7000-8000-000000000003": {
      "name": "version",
      "type": "string",
      "required": true,
      "description": "The current active version of the schema."
    },
    "019f4066-0000-7000-8000-000000000004": {
      "name": "versions",
      "type": "record",
   	  "schema": {
        "id": "019f4066-0000-7000-8000-000000000005"
   	  },
      "required": false,
      "description": "A list of legacy schemas, their physical name & their corresponding schema."
    }
  },
  "schemas": {
    "019f4066-0000-7000-8000-000000000005": {
      "name": "SchemaVersions",
      "description": "A list of legacy schemas, their physical name & their corresponding schema.",
      "fields": {
        "019f4066-0000-7000-8000-000000000006": {
          "name": "physical",
          "type": "string",
          "required": false,
          "description": "The physical name of the collection in the database."
        },
   	    "019f4066-0000-7000-8000-000000000007": {
          "name": "schema",
          "type": "record",
          "required": true,
          "description": "The full schema definition as a JSON object."
        }
      }
    }
  },
  "indexes": {
    "019f4066-0000-7000-8000-000000000008": {
      "name": "name_index",
      "fields": ["name"],
      "type": "normal",
      "description": "Index on schema name for quick lookup."
    }
  }
}
`, REGISTRY_COLLECTION_NAME)

func RegistrySchema() *definition.Schema {
	def, err := definition.FromJSON([]byte(RegistryCollectionSchemaJson))
	if err != nil {
		// This should ideally not happen as the JSON is hardcoded and controlled.
		// If it does, it indicates a critical internal error.
		panic(fmt.Sprintf("failed to unmarshal registry schema: %v", err))
	}

	return MustEnrichSchema(def)
}

package registry

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// REGISTRY_COLLECTION_NAME is the constant name for the internal collection that
// stores the schema definitions for all other collections.
const REGISTRY_COLLECTION_NAME = "_schemas_"

var RegistryCollectionSchemaJson = fmt.Sprintf(`
{
  "name": "%%s",
  "version": "1.0.0",
  "description": "Stores schema definitions for all collections in the database.",
  "fields": {
    "f1": {
      "name": "name",
      "type": "string",
      "description": "The logical name of the schema."
    },
    "f2": {
      "name": "description",
      "type": "string",
      "description": "A description of the schema."
    },
    "f3": {
      "name": "version",
      "type": "string",
      "required": true,
      "description": "The current active version of the schema."
    },
    "f4": {
      "name": "versions",
      "type": "record",
  	  "schema": {
        "id": "s1"
  	  },
      "required": false,
      "description": "A list of legacy schemas, their physical name & their corresponding schema."
    }
  },
  "schemas": {
    "s1": {
      "name": "SchemaVersions",
      "description": "A list of legacy schemas, their physical name & their corresponding schema.",
      "fields": {
        "sf1": {
          "name": "physical",
          "type": "string",
          "required": false,
          "description": "The physical name of the collection in the database."
        },
    	"sf2": {
    	  "name": "schema",
    	  "type": "record",
    	  "required": true,
    	  "description": "The full schema definition as a JSON object."
    	}
      }
    }
  },
  "indexes": {
    "ni1": {
      "name": "name_index",
      "fields": ["f1"],
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
		panic(fmt.Sprintf("failed to unmarshal registry schema: %%v", err))
	}

	return MustEnrichSchema(def)
}

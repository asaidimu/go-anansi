package registry

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
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
    "019f2c3f-37cd-77c1-acac-9f6ade24120d": {
      "name": "name",
      "type": "string",
      "description": "The logical name of the schema."
    },
    "019f2c3f-37cd-77f9-b5f4-42b7f3906d83": {
      "name": "description",
      "type": "string",
      "description": "A description of the schema."
    },
    "019f2c3f-37cd-7801-a648-b7d414d50b62": {
      "name": "version",
      "type": "string",
      "required": true,
      "description": "The current active version of the schema."
    },
    "019f2c3f-37cd-7808-a681-97b38228f6b5": {
      "name": "versions",
      "type": "record",
   	  "schema": {
        "id": "019f2c3f-37cd-7810-b000-b7a83b05b848"
   	  },
      "required": false,
      "description": "A list of legacy schemas, their physical name & their corresponding schema."
    }
  },
  "schemas": {
    "019f2c3f-37cd-7810-b000-b7a83b05b848": {
      "name": "SchemaVersions",
      "description": "A list of legacy schemas, their physical name & their corresponding schema.",
      "fields": {
        "019f2c3f-37cd-7816-b965-fd3d3f676215": {
          "name": "physical",
          "type": "string",
          "required": false,
          "description": "The physical name of the collection in the database."
        },
   	    "019f2c3f-37cd-781d-8fd2-6792d1fc83ad": {
          "name": "schema",
          "type": "record",
          "required": true,
          "description": "The full schema definition as a JSON object."
        }
      }
    }
  },
  "indexes": {
    "019f2c3f-37cd-7823-b029-4f9781c97ae3": {
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

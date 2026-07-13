package meta

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

//go:embed schema.json
var schemaJSON []byte

var (
	querySchema     *definition.Schema
	querySchemaOnce sync.Once
	querySchemaErr  error
)

// QuerySchema is the meta-schema that describes the structure of all valid queries.
// It can be used with definition.NewDocumentValidator to validate query JSON payloads.
func QuerySchema() (*definition.Schema, error) {
	querySchemaOnce.Do(func() {
		var s definition.Schema
		if err := json.Unmarshal(schemaJSON, &s); err != nil {
			querySchemaErr = err
			return
		}
		querySchema = &s
	})
	return querySchema, querySchemaErr
}

// MustQuerySchema returns the QuerySchema, panicking on error.
func MustQuerySchema() *definition.Schema {
	s, err := QuerySchema()
	if err != nil {
		panic(err)
	}
	return s
}

// QuerySchemaJSON returns the raw JSON bytes of the query meta-schema.
func QuerySchemaJSON() []byte {
	return schemaJSON
}

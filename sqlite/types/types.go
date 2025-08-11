package types

import "github.com/asaidimu/go-anansi/v6/core/schema"

type SQLitePayload struct {
	Schema *schema.SchemaDefinition
	SQL    string
	Params []any
}

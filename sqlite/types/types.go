package types

import "github.com/asaidimu/go-anansi/v6/core/schema/definition"

type SQLitePayload struct {
	Schema *definition.Schema
	SQL    string
	Params []any
}

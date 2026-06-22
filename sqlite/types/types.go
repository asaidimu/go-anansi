package types

import "github.com/asaidimu/go-anansi/v7/core/schema/definition"

type SQLitePayload struct {
	Schema *definition.Schema
	SQL    string
	Params []any
}

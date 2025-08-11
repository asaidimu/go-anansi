package query

import (
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

type SQLNode interface {
	Value() (string, []any, error)
}

type SQLStatement interface {
	SQLNode
	StatementType() native.StatementType
}

type updateTree struct {
	target      SQLNode
	assignments SQLNode
	filters     SQLNode
}

type deleteTree struct {
	target  SQLNode
	filters SQLNode
}

type insertTree struct {
	target SQLNode
	values SQLNode
}

type selectTree struct {
	projection SQLNode
	target     SQLNode
	joins      SQLNode
	filters    SQLNode
	groupBy    SQLNode
	having     SQLNode
	orderBy    SQLNode
	limit      SQLNode
}



type createTableTree struct {
	schema *schema.SchemaDefinition
}



type dropTableTree struct {
	schema *schema.SchemaDefinition
}

type createIndexTree struct {
	schema *schema.SchemaDefinition
	index  *schema.IndexDefinition
}



type dropIndexTree struct {
	index *schema.IndexDefinition
}
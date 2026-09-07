package query

import (
        "github.com/asaidimu/go-anansi/v8/core/query/native"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
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
        schema *definition.Schema
}



type dropTableTree struct {
        name   string
        schema *definition.Schema
}

type createIndexTree struct {
        collection string
        index  *definition.Index
}

type dropIndexTree struct {
        collection string
        index *definition.Index
}

// createFTSTree emits CREATE VIRTUAL TABLE … USING FTS5 plus sync triggers
// for an IndexTypeFullText index on a collection. See fts.go.
type createFTSTree struct {
        collection string
        schema     *definition.Schema
        index      definition.Index
}

// dropFTSTree emits DROP TABLE / DROP TRIGGER for an FTS5 virtual table
// and its sync triggers. See fts.go.
type dropFTSTree struct {
        collection string
        schema     *definition.Schema
        index      definition.Index
}

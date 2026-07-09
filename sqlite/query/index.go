package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)


func (f *sqliteFactory) buildCreateIndexTree(q *query.Query, extra any) (SQLNode, error) {
	index, ok := extra.(*definition.Index)
	if !ok {
		// Try value type if pointer fails
		if idxVal, ok := extra.(definition.Index); ok {
			index = &idxVal
		} else {
			return nil, ErrIndexExtraNotIndexDefinition
		}
	}
	return &createIndexTree{collection: q.Target.Name, index: index}, nil
}

func (t *createIndexTree) Value() (string, []any, error) {
	if len(t.collection) == 0 {
		return "", nil, ErrIndexSchemaNotDefined
	}
	if t.index == nil {
		return "", nil, ErrIndexIndexNotDefined
	}

	collection := t.collection
	index := t.index

	var sb strings.Builder
	sb.WriteString("CREATE ")
	if index.Unique || index.Type == definition.IndexTypeUnique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX IF NOT EXISTS ")
	indexName := index.Name
	if indexName == "" {
		unquotedTableName := strings.Trim(collection, `"`)
		stringFields := make([]string, len(index.Fields))
		for i, fn := range index.Fields {
			stringFields[i] = string(fn)
		}
		indexName = fmt.Sprintf("idx_%s_%s", unquotedTableName, strings.Join(stringFields, "_"))
	}
	sb.WriteString(quoteIdentifier(indexName))
	sb.WriteString(fmt.Sprintf(" ON %s (", collection))

	var fieldParts []string
	for _, field := range index.Fields {
		colName := string(field)
		part := ""
		if strings.Contains(colName, ".") {
			tablePart := colName[:strings.Index(colName, ".")]
			fieldPart := colName[strings.Index(colName, ".")+1:]
			jsonPath := "$." + strings.ReplaceAll(fieldPart, ".", ".")
			part = fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(tablePart), jsonPath)
		} else {
			part = quoteIdentifier(colName)
		}
		if index.Order != "" && strings.ToUpper(index.Order) == "DESC" {
			part += " DESC"
		}
		fieldParts = append(fieldParts, part)
	}
	sb.WriteString(strings.Join(fieldParts, ", ") + ")")
	sb.WriteString(";")
	return sb.String(), nil, nil
}

func (f *sqliteFactory) buildDropIndexTree(_ *query.Query, extra any) (SQLNode, error) {
	index, ok := extra.(*definition.Index)
	if !ok {
		// Try value type if pointer fails
		if idxVal, ok := extra.(definition.Index); ok {
			index = &idxVal
		} else {
			return nil, ErrIndexExtraNotIndexDefinition
		}
	}
	return &dropIndexTree{index: index}, nil
}

func (t *dropIndexTree) Value() (string, []any, error) {
	if t.index == nil {
		return "", nil, ErrIndexIndexNotDefined
	}

	return fmt.Sprintf("DROP INDEX IF EXISTS %s;", t.index.Name), nil, nil
}

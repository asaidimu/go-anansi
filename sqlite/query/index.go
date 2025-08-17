package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)


func (f *sqliteFactory) buildCreateIndexTree(q *query.Query, extra any) (SQLNode, error) {
	index, ok := extra.(schema.IndexDefinition)
	if !ok {
		return nil, fmt.Errorf("extra is not an IndexDefinition")
	}
	return &createIndexTree{collection: q.Target.Name, index: &index}, nil
}

func (t *createIndexTree) Value() (string, []any, error) {
	if len(t.collection) == 0 {
		return "", nil, fmt.Errorf("schema is not defined for create index tree")
	}
	if t.index == nil {
		return "", nil, fmt.Errorf("index is not defined for create index tree")
	}

	collection := t.collection
	index := t.index

	var sb strings.Builder
	sb.WriteString("CREATE ")
	if (index.Unique != nil && *index.Unique) || index.Type == schema.IndexTypeUnique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX IF NOT EXISTS ")
	indexName := index.Name
	if indexName == "" {
		unquotedTableName := strings.Trim(collection, `"`)
		indexName = fmt.Sprintf("idx_%s_%s", unquotedTableName, strings.Join(index.Fields, "_"))
	}
	sb.WriteString(quoteIdentifier(indexName))
	sb.WriteString(fmt.Sprintf(" ON %s (", collection))

	var fieldParts []string
	for _, field := range index.Fields {
		part := ""
		if strings.Contains(field, ".") {
			jsonPath := "$." + strings.ReplaceAll(field, ".", ".")
			part = fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(field[:strings.Index(field, ".")]), jsonPath)
		} else {
			part = quoteIdentifier(field)
		}
		if index.Order != nil && strings.ToUpper(*index.Order) == "DESC" {
			part += " DESC"
		}
		fieldParts = append(fieldParts, part)
	}
	sb.WriteString(strings.Join(fieldParts, ", ") + ")")
	sb.WriteString(";")
	return sb.String(), nil, nil
}

func (f *sqliteFactory) buildDropIndexTree(_ *query.Query, extra any) (SQLNode, error) {
	index, ok := extra.(schema.IndexDefinition)
	if !ok {
		return nil, fmt.Errorf("extra is not an IndexDefinition")
	}
	return &dropIndexTree{index: &index}, nil
}

func (t *dropIndexTree) Value() (string, []any, error) {
	if t.index == nil {
		return "", nil, fmt.Errorf("index is not defined for drop index tree")
	}

	return fmt.Sprintf("DROP INDEX IF EXISTS %s;", t.index.Name), nil, nil
}

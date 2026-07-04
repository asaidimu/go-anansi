package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func paginationSchema(name string) *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: name,
			Fields: map[definition.FieldId]definition.Field{
				"name": {
					Name: "name",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"value": {
					Name: "value",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeNumber,
					},
				},
				"even": {
					Name: "even",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeBoolean,
					},
				},
			},
		},
	}
}

func setupPersistence(t *testing.T) base.Persistence {
	t.Helper()
	testutils.ConfigureDocumentFactory()
	p, err := persistence.NewPersistence(ephemeral.NewEphemeral(), nil, zap.NewNop(), nil)
	require.NoError(t, err)
	return p
}

func TestPagination_SQLite_Offset_NoFilters_WithPagination(t *testing.T) {
	ctx := context.Background()
	collection, cleanup := testutils.SetupCollectionTest(t)
	defer cleanup()

	docs := make([]*data.Document, 10)
	for i := range 10 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "age": i})
	}
	_, err := collection.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().Limit(3).Offset(0).Build()

	result, err := collection.Read(ctx, &q)
	require.NoError(t, err)

	t.Logf("PaginationInfo: Number=%d, Size=%d, Count=%d, Total=%d, Pages=%d",
		result.PaginationInfo.Number,
		result.PaginationInfo.Size,
		result.PaginationInfo.Count,
		result.PaginationInfo.Total,
		result.PaginationInfo.Pages,
	)

	assert.Len(t, result.Data, 3)
	assert.Equal(t, 3, result.Count)
	require.NotNil(t, result.PaginationInfo)
	assert.Equal(t, 10, result.PaginationInfo.Total, "Total should be the total number of matching records (10)")
	assert.True(t, result.PaginationInfo.Pages > 0, "Pages should be > 0, got %d", result.PaginationInfo.Pages)
}

func TestPagination_Offset_ReturnsCorrectTotal(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 10)
	for i := range 10 {
		docs[i] = data.MustNewDocument(map[string]any{
			"name":  "item",
			"value": i,
		})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	offset := 3
	q := query.NewQueryBuilder().Limit(3).Offset(offset).Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 3, "should return 3 documents on this page")
	assert.Equal(t, 3, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 10, *result.Total, "Total should be all matching records")
	assertPaginationInfo(t, result.PaginationInfo, 2, 3, 3, 10, 4)
	for i, doc := range result.Data {
		val, err := doc.GetInt("value")
		require.NoError(t, err)
		assert.Equal(t, i+offset, val, "page should start at offset 3")
	}
}

func TestPagination_Offset_FirstPage(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 10)
	for i := range 10 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "value": i})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().Limit(4).Offset(0).Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 4)
	assert.Equal(t, 4, result.Count)
	require.NotNil(t, result.Total)
	assert.Equal(t, 10, *result.Total)
	assertPaginationInfo(t, result.PaginationInfo, 1, 4, 4, 10, 3)
	for i, doc := range result.Data {
		val, _ := doc.GetInt("value")
		assert.Equal(t, i, val)
	}
}

func TestPagination_Offset_SecondPage(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 10)
	for i := range 10 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "value": i})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().Limit(4).Offset(4).Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 4)
	assert.Equal(t, 4, result.Count)
	require.NotNil(t, result.Total)
	assert.Equal(t, 10, *result.Total)
	assertPaginationInfo(t, result.PaginationInfo, 2, 4, 4, 10, 3)
	for i, doc := range result.Data {
		val, _ := doc.GetInt("value")
		assert.Equal(t, i+4, val)
	}
}

func assertPaginationInfo(t *testing.T, pi *query.PaginationInfo, number, size, count, total, pages int) {
	t.Helper()
	require.NotNil(t, pi)
	assert.Equal(t, number, pi.Number, "page number")
	assert.Equal(t, size, pi.Size, "page size")
	assert.Equal(t, count, pi.Count, "page count")
	assert.Equal(t, total, pi.Total, "total items")
	assert.Equal(t, pages, pi.Pages, "total pages")
}

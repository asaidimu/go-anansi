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

func TestPagination_Offset_LastPagePartial(t *testing.T) {
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

	q := query.NewQueryBuilder().Limit(4).Offset(8).Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 2, "only 2 records remain on the last page")
	assert.Equal(t, 2, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 10, *result.Total)
	assertPaginationInfo(t, result.PaginationInfo, 3, 4, 2, 10, 3)
	assert.Equal(t, 8, mustGetInt(t, result.Data[0], "value"))
	assert.Equal(t, 9, mustGetInt(t, result.Data[1], "value"))
}

func TestPagination_Offset_BeyondRange(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 5)
	for i := range 5 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "value": i})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().Limit(3).Offset(10).Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 0, "offset beyond range should return empty set")
	assert.Equal(t, 0, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 5, *result.Total, "Total should still reflect total matching records")
	assertPaginationInfo(t, result.PaginationInfo, 4, 3, 0, 5, 2)
}

func TestPagination_Offset_NoPaginationReturnsAll(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 7)
	for i := range 7 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "value": i})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.Query{}
	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 7, "no pagination should return all documents")
	assert.Equal(t, 7, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 7, *result.Total)
	assertPaginationInfo(t, result.PaginationInfo, 1, 7, 7, 7, 1)
}

func TestPagination_Offset_WithFilter(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 10)
	for i := range 10 {
		docs[i] = data.MustNewDocument(map[string]any{
			"name":  "item",
			"value": i,
			"even":  i%2 == 0,
		})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().
		Where("even").Eq(true).
		Limit(2).
		Offset(0).
		Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 2, "page size is 2")
	assert.Equal(t, 2, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 5, *result.Total, "Total should reflect filtered matching records (5 even values)")
	assertPaginationInfo(t, result.PaginationInfo, 1, 2, 2, 5, 3)
}

func TestPagination_Offset_WithSort(t *testing.T) {
	ctx := context.Background()
	p := setupPersistence(t)

	coll, err := p.CreateCollection(ctx, paginationSchema("items"))
	require.NoError(t, err)

	docs := make([]*data.Document, 5)
	for i := range 5 {
		docs[i] = data.MustNewDocument(map[string]any{"name": "item", "value": i})
	}
	_, err = coll.CreateMany(ctx, docs)
	require.NoError(t, err)

	q := query.NewQueryBuilder().
		OrderBy("value", query.SortDirectionDesc).
		Build()

	result, err := coll.Read(ctx, &q)
	require.NoError(t, err)

	assert.Len(t, result.Data, 5)
	assert.Equal(t, 5, result.Count, "Count should match len(Data)")
	require.NotNil(t, result.Total)
	assert.Equal(t, 5, *result.Total)
	assertPaginationInfo(t, result.PaginationInfo, 1, 5, 5, 5, 1)
	assert.Equal(t, 4, mustGetInt(t, result.Data[0], "value"), "desc sort: highest first")
	assert.Equal(t, 3, mustGetInt(t, result.Data[1], "value"))
	assert.Equal(t, 2, mustGetInt(t, result.Data[2], "value"))
	assert.Equal(t, 1, mustGetInt(t, result.Data[3], "value"))
	assert.Equal(t, 0, mustGetInt(t, result.Data[4], "value"))
}

func mustGetInt(t *testing.T, doc *data.Document, field string) int {
	t.Helper()
	val, err := doc.GetInt(field)
	require.NoError(t, err)
	return val
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

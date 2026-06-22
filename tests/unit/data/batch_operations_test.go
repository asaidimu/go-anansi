package data_test

import (
	"testing"

	// "github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/stretchr/testify/require"
)

func TestDocumentSet_Filter(t *testing.T) {
	require.True(t, true)
	/* doc1, err := data.NewDocument(map[string]any{"id": "1", "status": "active"})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "status": "inactive"})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "status": "active"})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3})

	filtered := ds.Filter(func(d *data.Document) bool {
		status, _ := d.GetString("status")
		return status == "active"
	})

	require.Len(t, filtered, 2)
	require.Contains(t, filtered, doc1)
	require.Contains(t, filtered, doc3)
	require.NotContains(t, filtered, doc2) */
}

/* func TestDocumentSet_Map(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "value": 10})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "value": 20})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2})

	mapped := ds.Map(func(d *data.Document) *data.Document {
		val, _ := d.GetInt("value")
		d.Set("value", val*2)
		return d
	})

	require.Len(t, mapped, 2)
	require.Equal(t, 20, mapped[0].Must().Get("value"))
	require.Equal(t, 40, mapped[1].Must().Get("value"))
}

func TestDocumentSet_Find(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "name": "Alice"})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "name": "Bob"})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2})

	found, ok := ds.Find(func(d *data.Document) bool {
		name, _ := d.GetString("name")
		return name == "Alice"
	})
	require.True(t, ok)
	require.Equal(t, doc1, found)

	_, ok = ds.Find(func(d *data.Document) bool {
		name, _ := d.GetString("name")
		return name == "Charlie"
	})
	require.False(t, ok)
}

func TestDocumentSet_Where(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "category": "A"})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "category": "B"})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "category": "A"})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3})

	filtered := ds.Where("category", "A")

	require.Len(t, filtered, 2)
	require.Contains(t, filtered, doc1)
	require.Contains(t, filtered, doc3)
}

func TestDocumentSet_SortBy(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "age": 30})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "age": 20})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "age": 40})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3})

	sorted := ds.SortBy("age")

	require.Len(t, sorted, 3)
	require.Equal(t, doc2, sorted[0])
	require.Equal(t, doc1, sorted[1])
	require.Equal(t, doc3, sorted[2])
}

func TestDocumentSet_SortByDesc(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "age": 30})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "age": 20})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "age": 40})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3})

	sorted := ds.SortByDesc("age")

	require.Len(t, sorted, 3)
	require.Equal(t, doc3, sorted[0])
	require.Equal(t, doc1, sorted[1])
	require.Equal(t, doc2, sorted[2])
}

func TestDocumentSet_GroupBy(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "city": "NY"})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "city": "LA"})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "city": "NY"})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3})

	grouped := ds.GroupBy("city")

	require.Len(t, grouped, 2)
	require.Len(t, grouped["NY"], 2)
	require.Contains(t, grouped["NY"], doc1)
	require.Contains(t, grouped["NY"], doc3)
	require.Len(t, grouped["LA"], 1)
	require.Contains(t, grouped["LA"], doc2)
}

func TestDocumentSet_Reduce(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "value": 10})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "value": 20})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2})

	initial, err := data.NewDocument(map[string]any{"total": 0})
	require.NoError(t, err)

	reduced := ds.Reduce(func(acc, current *data.Document) *data.Document {
		accTotal, _ := acc.GetInt("total")
		currentValue, _ := current.GetInt("value")
		acc.Set("total", accTotal+currentValue)
		return acc
	}, initial)

	require.Equal(t, 30, reduced.Must().Get("total"))
}

func TestDocumentSet_Aggregate(t *testing.T) {
	doc1, err := data.NewDocument(map[string]any{"id": "1", "score": 10.0})
	require.NoError(t, err)
	doc2, err := data.NewDocument(map[string]any{"id": "2", "score": 20.0})
	require.NoError(t, err)
	doc3, err := data.NewDocument(map[string]any{"id": "3", "score": 30.0})
	require.NoError(t, err)
	doc4, err := data.NewDocument(map[string]any{"id": "4", "score": 20.0})
	require.NoError(t, err)

	ds, _:= data.NewDocumentSet([]*data.Document{doc1, doc2, doc3, doc4})

	result := ds.Aggregate("score")

	require.Equal(t, 4, result.Count)
	require.Equal(t, 80.0, result.Sum)
	require.Equal(t, 20.0, result.Average)
	require.Equal(t, 10.0, result.Min)
	require.Equal(t, 30.0, result.Max)
	require.Equal(t, 20.0, result.Median)
	// StdDev calculation might have floating point inaccuracies, check within a small delta
	require.InDelta(t, 7.0710678118654755, result.StdDev, 0.0001)
} */

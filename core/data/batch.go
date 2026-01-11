package data

import (
	"context"
	"math"
	"reflect"
	"sort"
)

// DocumentSet represents a collection of documents with batch operations.
type DocumentSet []*Document

// NewDocumentSet creates a new DocumentSet from a variety of slice types.
// It intelligently converts []map[string]any, []any, and []Document into a
// consistent DocumentSet. It accepts an optional context that is passed down
// during the creation of each new Document, allowing for contextual metadata injection.
func NewDocumentSet(v any, ctx ...context.Context) (DocumentSet, bool) {
	switch val := v.(type) {
	case DocumentSet:
		return val, true
	case []*Document:
		return DocumentSet(val), true
	case []Document:
		docs := make([]*Document, len(val))
		for i := range val {
			docs[i] = &val[i]
		}
		return DocumentSet(docs), true
	case []any:
		docs := make(DocumentSet, 0, len(val))
		for _, item := range val {
			if doc, ok := DocumentFrom(item, ctx...); ok {
				docs = append(docs, doc)
			} else {
				return nil, false
			}
		}
		return docs, true
	case []map[string]any:
		docs := make(DocumentSet, len(val))
		for i, m := range val {
			newDoc, err := NewDocument(m, ctx...)
			if err != nil {
				return nil, false
			}
			docs[i] = newDoc
		}
		return docs, true
	default:
		return nil, false
	}
}

// Filter applies a filter to all documents in the set.
func (ds DocumentSet) Filter(predicate func(*Document) bool) DocumentSet {
	result := make(DocumentSet, 0)
	for _, doc := range ds {
		if predicate(doc) {
			result = append(result, doc)
		}
	}
	return result
}

// Map applies a transformation to all documents in the set.
func (ds DocumentSet) Map(transformer func(*Document) *Document) DocumentSet {
	result := make(DocumentSet, len(ds))
	for i, doc := range ds {
		result[i] = transformer(doc)
	}
	return result
}

// Find returns the first document matching the predicate.
func (ds DocumentSet) Find(predicate func(*Document) bool) (*Document, bool) {
	for _, doc := range ds {
		if predicate(doc) {
			return doc, true
		}
	}
	return nil, false
}

// Where returns documents where the specified key equals the value.
func (ds DocumentSet) Where(key string, value any) DocumentSet {
	return ds.Filter(func(d *Document) bool {
		val, err := d.Get(key)
		if err != nil {
			return false
		}
		return reflect.DeepEqual(val, value)
	})
}

// SortBy sorts documents by a key in ascending order.
func (ds DocumentSet) SortBy(key string) DocumentSet {
	result := make(DocumentSet, len(ds))
	copy(result, ds)

	sort.Slice(result, func(i, j int) bool {
		val1, err1 := result[i].Get(key)
		val2, err2 := result[j].Get(key)

		if err1 != nil && err2 != nil {
			return false
		}
		if err1 != nil {
			return true
		}
		if err2 != nil {
			return false
		}

		return compareValues(val1, val2) < 0
	})

	return result
}

// SortByDesc sorts documents by a key in descending order.
func (ds DocumentSet) SortByDesc(key string) DocumentSet {
	result := ds.SortBy(key)
	// Reverse the slice
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// GroupBy groups documents by a key value.
func (ds DocumentSet) GroupBy(key string) map[string]DocumentSet {
	groups := make(map[string]DocumentSet)

	for _, doc := range ds {
		val, err := doc.GetString(key)
		if err != nil {
			val = "undefined"
		}

		if _, exists := groups[val]; !exists {
			groups[val] = make(DocumentSet, 0)
		}
		groups[val] = append(groups[val], doc)
	}

	return groups
}

// Reduce applies a reducer function to all documents.
func (ds DocumentSet) Reduce(reducer func(acc, current *Document) *Document, initial *Document) *Document {
	result := initial.Clone()
	for _, doc := range ds {
		result = reducer(result, doc)
	}
	return result
}

// Aggregate performs common aggregation operations.
func (ds DocumentSet) Aggregate(key string) AggregationResult {
	var sum float64
	var count int
	var min, max float64
	var values []float64

	for _, doc := range ds {
		if val, err := doc.GetFloat64(key); err == nil {
			values = append(values, val)
			sum += val
			count++

			if count == 1 {
				min = val
				max = val
			} else {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
			}
		}
	}

	result := AggregationResult{
		Count: count,
		Sum:   sum,
		Min:   min,
		Max:   max,
	}

	if count > 0 {
		result.Average = sum / float64(count)

		// Calculate median
		sort.Float64s(values)
		if count%2 == 0 {
			result.Median = (values[count/2-1] + values[count/2]) / 2
		} else {
			result.Median = values[count/2]
		}

		// Calculate standard deviation
		var variance float64
		for _, val := range values {
			diff := val - result.Average
			variance += diff * diff
		}
		result.StdDev = math.Sqrt(variance / float64(count))
	}

	return result
}

// AggregationResult contains statistical aggregation results.
type AggregationResult struct {
	Count   int     `json:"count"`
	Sum     float64 `json:"sum"`
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Median  float64 `json:"median"`
	StdDev  float64 `json:"std_dev"`
}

// ToMaps converts the set of Documents back into a slice of raw maps.
// This is perfect for JSON encoding or legacy API compatibility.
func (ds DocumentSet) ToMaps() []map[string]any {
	result := make([]map[string]any, len(ds))
	for i, doc := range ds {
		result[i] = doc.ToMap()
	}
	return result
}

// Sanitize applies context-aware masking to every document in the set.
func (ds DocumentSet) Sanitize(ctx ...context.Context) (DocumentSet, error) {
	result := make(DocumentSet, len(ds))
	for i, doc := range ds {
		res, err := doc.Sanitize(ctx...)
		if err != nil {
			return nil, err
		}
		result[i] = res
	}

	return result, nil
}

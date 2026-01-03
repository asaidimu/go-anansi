package data

import (
	"context"
	"math"
	"reflect"
	"sort"
)

// DocumentSet represents a collection of documents with batch operations.
type DocumentSet []*Document

// NewDocumentSet creates a new document set.
func NewDocumentSet(docs ...*Document) DocumentSet {
	return DocumentSet(docs)
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
		result[i] = doc.AsMap()
	}
	return result
}

// Sanitize applies context-aware masking to every document in the set.
func (ds DocumentSet) Sanitize(ctx context.Context) DocumentSet {
	result := make(DocumentSet, len(ds))
	for i, doc := range ds {
		result[i] = doc.Sanitize(ctx)
	}
	return result
}

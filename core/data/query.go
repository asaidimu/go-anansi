package data

import (
	"reflect"
	"sort"
	"strings"
)

// Query creates a new fluent query interface
func Query(docs DocumentSet) *FluentQuery {
	return &FluentQuery{
		docs:    docs,
		filters: make([]func(Document) bool, 0),
	}
}

// FluentQuery provides a fluent interface for querying documents
type FluentQuery struct {
	docs    DocumentSet
	filters []func(Document) bool
	sorters []SortCriteria
	limit   int
	offset  int
}

type SortCriteria struct {
	Key  string
	Desc bool
}

// Where adds an equality filter
func (fq *FluentQuery) Where(key string, value any) *FluentQuery {
	fq.filters = append(fq.filters, func(d Document) bool {
		val, err := d.Get(key)
		if err != nil {
			return false
		}
		return reflect.DeepEqual(val, value)
	})
	return fq
}

// WhereFunc adds a custom filter function
func (fq *FluentQuery) WhereFunc(predicate func(Document) bool) *FluentQuery {
	fq.filters = append(fq.filters, predicate)
	return fq
}

// Comparison helpers for fluent queries
type FieldComparison struct {
	query *FluentQuery
	key   string
}

// Where returns a field comparison helper
func (fq *FluentQuery) WhereField(key string) *FieldComparison {
	return &FieldComparison{query: fq, key: key}
}

func (fc *FieldComparison) Equals(value any) *FluentQuery {
	return fc.query.Where(fc.key, value)
}

func (fc *FieldComparison) GreaterThan(value any) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return compareValues(val, value) > 0
	})
	return fc.query
}

func (fc *FieldComparison) LessThan(value any) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return compareValues(val, value) < 0
	})
	return fc.query
}

func (fc *FieldComparison) Contains(substr string) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.GetString(fc.key)
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(val), strings.ToLower(substr))
	})
	return fc.query
}

func (fc *FieldComparison) In(values ...any) *FluentQuery {
	valueSet := make(map[any]bool)
	for _, v := range values {
		valueSet[v] = true
	}

	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return valueSet[val]
	})
	return fc.query
}

// Sorting
func (fq *FluentQuery) SortBy(key string) *FluentQuery {
	fq.sorters = append(fq.sorters, SortCriteria{Key: key, Desc: false})
	return fq
}

func (fq *FluentQuery) SortByDesc(key string) *FluentQuery {
	fq.sorters = append(fq.sorters, SortCriteria{Key: key, Desc: true})
	return fq
}

// Pagination
func (fq *FluentQuery) Limit(n int) *FluentQuery {
	fq.limit = n
	return fq
}

func (fq *FluentQuery) Offset(n int) *FluentQuery {
	fq.offset = n
	return fq
}

func (fq *FluentQuery) Skip(n int) *FluentQuery {
	return fq.Offset(n)
}

// Aggregation helpers
func (fq *FluentQuery) Count() int {
	result := fq.Execute()
	return len(result)
}

func (fq *FluentQuery) First() (Document, bool) {
	result := fq.Limit(1).Execute()
	if len(result) == 0 {
		return nil, false
	}
	return result[0], true
}

func (fq *FluentQuery) Exists() bool {
	return fq.Limit(1).Count() > 0
}

// Execute applies all filters, sorts, and pagination
func (fq *FluentQuery) Execute() DocumentSet {
	result := make(DocumentSet, 0, len(fq.docs))

	// Apply filters
	for _, doc := range fq.docs {
		include := true
		for _, filter := range fq.filters {
			if !filter(doc) {
				include = false
				break
			}
		}
		if include {
			result = append(result, doc)
		}
	}

	// Apply sorting
	if len(fq.sorters) > 0 {
		sort.Slice(result, func(i, j int) bool {
			for _, criteria := range fq.sorters {
				val1, err1 := result[i].Get(criteria.Key)
				val2, err2 := result[j].Get(criteria.Key)

				if err1 != nil && err2 != nil {
					continue
				}
				if err1 != nil {
					return !criteria.Desc
				}
				if err2 != nil {
					return criteria.Desc
				}

				cmp := compareValues(val1, val2)
				if cmp != 0 {
					if criteria.Desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
	}

	// Apply pagination
	if fq.offset > 0 && fq.offset < len(result) {
		result = result[fq.offset:]
	} else if fq.offset >= len(result) {
		return DocumentSet{}
	}

	if fq.limit > 0 && fq.limit < len(result) {
		result = result[:fq.limit]
	}

	return result
}

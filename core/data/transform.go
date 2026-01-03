package data

// DocumentTransform provides a fluent interface for document transformations.
type DocumentTransform struct {
	doc *Document
	ops []func(*Document) *Document
}

// Transform creates a new transformation pipeline.
func (d *Document) Transform() *DocumentTransform {
	return &DocumentTransform{doc: d.Clone(), ops: make([]func(*Document) *Document, 0)}
}

// Map applies a transformation to each value.
func (tp *DocumentTransform) Map(transformer func(key string, value any) any) *DocumentTransform {
	tp.ops = append(tp.ops, func(d *Document) *Document {
		resultData := make(map[string]any)
		for k, v := range d.data {
			resultData[k] = transformer(k, v)
		}
		return &Document{ctx: d.ctx, data: resultData}
	})
	return tp
}

// Filter removes key-value pairs that don't match the predicate.
func (tp *DocumentTransform) Filter(predicate func(key string, value any) bool) *DocumentTransform {
	tp.ops = append(tp.ops, func(d *Document) *Document {
		resultData := make(map[string]any)
		for k, v := range d.data {
			if predicate(k, v) {
				resultData[k] = v
			}
		}
		return &Document{ctx: d.ctx, data: resultData}
	})
	return tp
}

// Pick selects only the specified keys.
func (tp *DocumentTransform) Pick(keys ...string) *DocumentTransform {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	return tp.Filter(func(key string, value any) bool {
		return keySet[key]
	})
}

// Omit excludes the specified keys.
func (tp *DocumentTransform) Omit(keys ...string) *DocumentTransform {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	return tp.Filter(func(key string, value any) bool {
		return !keySet[key]
	})
}

// Execute applies all transformations and returns the result.
func (tp *DocumentTransform) Execute() *Document {
	result := tp.doc
	for _, op := range tp.ops {
		result = op(result)
	}
	return result
}

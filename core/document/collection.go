package document

import (
	"fmt"
)

// Collection is an ordered, pool-aware bag of Documents.
//
// Collection makes no assumptions about schema identity. Two documents from
// entirely different schemas can coexist in the same collection. Whether that
// is meaningful is the caller's concern — the query layer, deserializer, or
// handler that produces the collection is responsible for homogeneity if it
// matters to them.
//
// Ownership model:
//   - A collection returned by NewCollection owns its documents and will return
//     them to the pool on Release.
//   - A collection returned by Filter is a view — it holds pointers to the same
//     documents as the source but does not own them. Release on a view is a no-op
//     on the documents; it only resets the view's own slice.
//   - A collection returned by FilterCopy owns its documents (fresh copies from
//     the pool) and releases them on Release.
type Collection struct {
	docs  []*Document
	pool  *Pool
	owner bool // true if this collection owns its documents
}

// NewCollection constructs an empty owning Collection backed by pool.
// Pass nil pool for collections whose documents are not pool-managed.
func NewCollection(pool *Pool) *Collection {
	return &Collection{
		docs:  make([]*Document, 0, 16),
		pool:  pool,
		owner: true,
	}
}

// Append adds doc to the collection.
func (c *Collection) Append(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("collection: cannot append nil document")
	}
	c.docs = append(c.docs, doc)
	return nil
}

// Len returns the number of documents in the collection.
func (c *Collection) Len() int {
	return len(c.docs)
}

// At returns the document at index i. Panics if i is out of range.
func (c *Collection) At(i int) *Document {
	return c.docs[i]
}

// Each calls f on each document in order. Stops early if f returns false.
func (c *Collection) Each(f func(i int, doc *Document) bool) {
	for i, doc := range c.docs {
		if !f(i, doc) {
			return
		}
	}
}

// Filter returns a view Collection containing only documents for which keep
// returns true. The view does not own its documents — Release resets the view
// slice but does not return documents to the pool.
//
// Use Filter when the source collection outlives the filtered result.
// Use FilterCopy when the source will be released before the result is consumed.
func (c *Collection) Filter(keep func(*Document) bool) *Collection {
	out := &Collection{
		docs:  make([]*Document, 0, len(c.docs)/2),
		pool:  c.pool,
		owner: false,
	}
	for _, doc := range c.docs {
		if keep(doc) {
			out.docs = append(out.docs, doc)
		}
	}
	return out
}

// FilterCopy returns an owning Collection of pool-allocated copies of documents
// for which keep returns true. The caller must Release the returned collection.
//
// Use this when the source collection will be released before the filtered
// result is consumed, or when you need an independent copy for mutation.
func (c *Collection) FilterCopy(keep func(*Document) bool) (*Collection, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("collection: FilterCopy requires a pool")
	}
	out := NewCollection(c.pool)
	for _, doc := range c.docs {
		if !keep(doc) {
			continue
		}
		copied := c.pool.Get()
		if err := copyDocument(doc, copied); err != nil {
			c.pool.Put(copied)
			out.Release()
			return nil, err
		}
		out.docs = append(out.docs, copied)
	}
	return out, nil
}

// Project returns an owning Collection where each document contains only the
// fields whose keys are in keys. Documents are obtained from pool.
// The caller must Release the returned collection.
func (c *Collection) Project(keys []DocumentKey) (*Collection, error) {
	if c.pool == nil {
		return nil, fmt.Errorf("collection: Project requires a pool")
	}
	out := NewCollection(c.pool)
	for _, src := range c.docs {
		dst := c.pool.Get()
		if err := projectDocument(src, dst, keys); err != nil {
			c.pool.Put(dst)
			out.Release()
			return nil, err
		}
		out.docs = append(out.docs, dst)
	}
	return out, nil
}

// Reduce folds all documents into a single accumulator value.
//
// Example — sum total_kes across a sale collection:
//
//	total := c.Reduce(int64(0), func(acc any, doc *Document) any {
//	    v, ok, _ := doc.GetInt(keySaleTotal)
//	    if ok { return acc.(int64) + v }
//	    return acc
//	}).(int64)
func (c *Collection) Reduce(initial any, f func(acc any, doc *Document) any) any {
	acc := initial
	for _, doc := range c.docs {
		acc = f(acc, doc)
	}
	return acc
}

// Release returns all owned documents to the pool and resets the collection.
// On a view collection (owner=false), only the internal slice is reset —
// documents are not touched.
func (c *Collection) Release() {
	if c.owner && c.pool != nil {
		for _, doc := range c.docs {
			c.pool.Put(doc)
		}
	}
	c.docs = c.docs[:0]
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func copyDocument(src, dst *Document) error {
	for k, idx := range src.positions {
		dk := DocumentKey(k)
		if idx < 0 {
			dst.SetNull(dk)
			continue
		}
		if err := copyField(src, dst, dk); err != nil {
			return err
		}
	}
	return nil
}

func projectDocument(src, dst *Document, keys []DocumentKey) error {
	for _, key := range keys {
		if !src.IsSet(key) {
			continue
		}
		if src.IsNull(key) {
			dst.SetNull(key)
			continue
		}
		if err := copyField(src, dst, key); err != nil {
			return err
		}
	}
	return nil
}

func copyField(src, dst *Document, key DocumentKey) error {
	switch key.Type() {
	case TypeInt:
		v, _, err := src.GetInt(key)
		if err != nil {
			return err
		}
		return dst.SetInt(key, v)
	case TypeFloat:
		v, _, err := src.GetFloat(key)
		if err != nil {
			return err
		}
		return dst.SetFloat(key, v)
	case TypeString:
		v, _, err := src.GetString(key)
		if err != nil {
			return err
		}
		return dst.SetString(key, v)
	case TypeBool:
		v, _, err := src.GetBool(key)
		if err != nil {
			return err
		}
		return dst.SetBool(key, v)
	case TypeBytes:
		v, _, err := src.GetBytes(key)
		if err != nil {
			return err
		}
		return dst.SetBytes(key, v)
	case TypeGeometry:
		v, _, err := src.GetGeometry(key)
		if err != nil {
			return err
		}
		return dst.SetGeometry(key, v)
	case TypeRecord:
		v, _, err := src.GetRecord(key)
		if err != nil {
			return err
		}
		return dst.SetRecord(key, v)
	case TypeArrayInt:
		v, _, err := src.GetArrayInt(key)
		if err != nil {
			return err
		}
		return dst.SetArrayInt(key, v)
	case TypeArrayFloat:
		v, _, err := src.GetArrayFloat(key)
		if err != nil {
			return err
		}
		return dst.SetArrayFloat(key, v)
	case TypeArrayString:
		v, _, err := src.GetArrayString(key)
		if err != nil {
			return err
		}
		return dst.SetArrayString(key, v)
	case TypeArrayBool:
		v, _, err := src.GetArrayBool(key)
		if err != nil {
			return err
		}
		return dst.SetArrayBool(key, v)
	case TypeArrayBytes:
		v, _, err := src.GetArrayBytes(key)
		if err != nil {
			return err
		}
		return dst.SetArrayBytes(key, v)
	case TypeArrayObject:
		v, _, err := src.GetArrayObject(key)
		if err != nil {
			return err
		}
		return dst.SetArrayObject(key, v)
	case TypeArrayGeometry:
		v, _, err := src.GetArrayGeometry(key)
		if err != nil {
			return err
		}
		return dst.SetArrayGeometry(key, v)
	case TypeUnknown:
		v, _, err := src.GetUnknown(key)
		if err != nil {
			return err
		}
		return dst.SetUnknown(key, v)
	default:
		return fmt.Errorf("collection: copyField: unhandled type %d", key.Type())
	}
}

package document

import (
	"sync"
	"unsafe"
)

// Pool is a sync.Pool for Documents.
//
// Pool.Put recurses into TypeArrayObject slots before calling Clear, returning
// any child documents embedded in array-object fields back to the pool before
// the parent is cleared. This prevents child documents from leaking when a
// parent is returned.
//
// Usage:
//
//	p := NewPool()
//
//	doc := p.Get(SizeSmall)
//	doc.SetInt(key, 42)
//	// ... fill ...
//	p.Put(doc) // Clear() is called by Put; caller must not use doc after this.
type Pool struct {
	pool sync.Pool
}

// NewPool constructs a Pool with size-appropriate New functions for each tier.
func NewPool() *Pool {
	p := &Pool{}

	p.pool.New = func() any {
		d := &Document{positions: make(map[int64]int32, 0)}
		return d
	}

	return p
}

// Get retrieves a cleared Document sized to the given tier.
func (p *Pool) Get() *Document {
	return p.pool.Get().(*Document)
}

// Put returns a document to the pool.
//
// Before clearing, Put recurses into any TypeArrayObject slots to return child
// documents to the pool. This means the caller must not hold references to
// child documents after calling Put — they are cleared and reused.
//
// Put re-tiers the document by its actual int slice length so that a document
// that grew beyond its original tier is returned to the correct bucket.
func (p *Pool) Put(doc *Document) {
	if doc == nil {
		return
	}

	// Recurse into TypeRecord and TypeArrayObject children before clearing the parent.
	// slot() is not used here because we do not want to allocate a new slice
	// if the type was never initialised.
	if ptr := doc.data[TypeRecord]; ptr != nil {
		children := *(*[]*Document)(ptr)
		for _, child := range children {
			p.Put(child)
		}
	}
	if ptr := doc.data[TypeArrayObject]; ptr != nil {
		children := *(*[][]*Document)(ptr)
		for _, group := range children {
			for _, child := range group {
				p.Put(child) // recursive: handles nested array objects
			}
		}
	}

	doc.Clear()

	p.pool.Put(doc)
}

// Acquire is a convenience wrapper that gets a document, calls f with it,
// then returns it to the pool regardless of whether f returns an error.
// This is the recommended pattern for request handlers.
//
//	err := pool.Acquire(SizeSmall, func(doc *Document) error {
//	    doc.SetInt(keySaleTotal, 935_000)
//	    return handler(doc)
//	})
func (p *Pool) Acquire(f func(*Document) error) error {
	doc := p.Get()
	defer p.Put(doc)
	return f(doc)
}

// Walk is a helper for Walk-based bulk deserialization from a pool.
// It gets a document, exposes its internals to the walker, then calls Put.
// Use this when a deserializer fills a document and immediately hands it off
// to a collection rather than returning it to the caller.
func (p *Pool) Walk(
	walker func(*Document, map[int64]int32, func(DataType, ...int) unsafe.Pointer) error,
) (*Document, error) {
	doc := p.Get()
	var walkErr error
	_, err := doc.Walk(func(positions map[int64]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
		walkErr = walker(doc, positions, slot)
		return nil, walkErr
	})
	if err != nil || walkErr != nil {
		p.Put(doc)
		if walkErr != nil {
			return nil, walkErr
		}
		return nil, err
	}
	return doc, nil
}

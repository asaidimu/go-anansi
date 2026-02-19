package document

// --- Document ---

// Document is a schema-bound, poolable record. DataContainer is embedded directly
// (zero cost vs a named field) and its methods are promoted onto Document.
// Clear() resets only DataContainer state — id and schema are pool-invariant.
type Document struct {
	DataContainer
}

func NewDocument() *Document {
	return &Document{
		DataContainer: *NewDataContainer(),
	}
}

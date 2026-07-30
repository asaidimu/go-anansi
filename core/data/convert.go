package data

// ToDoc converts any struct with anansi tags to a Document using only its
// user-data fields. System fields (_id_, _metadata_) are stripped from the
// output — use NewDocumentFromStruct if you need identity auto-generation
// or transfer system fields explicitly.
//
// This is a lightweight alternative to NewDocumentFromStruct that skips the
// factory pipeline (no UUID generation, no metadata initialization).
func ToDoc(s any) (*Document, error) {
	docData, err := structToMap(s, false)
	if err != nil {
		return nil, err
	}

	delete(docData, DocumentIDField)
	delete(docData, MetadataField)

	return Patch(docData).Document(), nil
}

// FromDoc populates a typed struct from a Document by matching its user data
// keys against the struct's anansi tags. System fields (_id_, _metadata_)
// in the Document are bound to the struct's DocumentModel if present.
func FromDoc[Dst any](doc *Document) (Dst, error) {
	var v Dst
	err := doc.BindTo(&v)
	return v, err
}

// MapTo copies fields from src to dst via Document as the interchange format.
// Fields are matched by anansi tag name. The destination type may embed
// DocumentModel to capture identity; if it does not, system fields are dropped.
func MapTo[Dst any](src any) (Dst, error) {
	doc, err := ToDoc(src)
	if err != nil {
		var zero Dst
		return zero, err
	}
	return FromDoc[Dst](doc)
}

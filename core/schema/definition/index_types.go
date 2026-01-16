package definition

import "encoding/json"

// IndexType represents the type of an index.
type IndexType byte

const (
	IndexTypeNormal IndexType = iota + 1
	IndexTypeUnique
	IndexTypePrimary
	IndexTypeSpatial
	IndexTypeFullText
)

var (
	indexTypeToString = map[IndexType]string{
		IndexTypeNormal:   "normal",
		IndexTypeUnique:   "unique",
		IndexTypePrimary:  "primary",
		IndexTypeSpatial:  "spatial",
		IndexTypeFullText: "fulltext",
	}

	stringToIndexType = map[string]IndexType{
		"normal":   IndexTypeNormal,
		"unique":   IndexTypeUnique,
		"primary":  IndexTypePrimary,
		"spatial":  IndexTypeSpatial,
		"fulltext": IndexTypeFullText,
	}
)

func (t IndexType) String() string {
	if s, ok := indexTypeToString[t]; ok {
		return s
	}
	return "normal"
}

func (t IndexType) MarshalJSON() ([]byte, error) {
	val, err := json.Marshal(t.String())
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err).WithOperation("IndexType.MarshalJSON")
	}
	return val, nil
}

func (t *IndexType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("IndexType.UnmarshalJSON")
	}
	if val, ok := stringToIndexType[s]; ok {
		*t = val
		return nil
	}
	*t = IndexTypeNormal
	return nil
}

func init(){
	_ = IndexTypeNormal
	_ = IndexTypeUnique
	_ = IndexTypePrimary
	_ = IndexTypeSpatial
	_ = IndexTypeFullText
}

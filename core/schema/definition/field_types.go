package definition

import "encoding/json"

type FieldType byte

const (
	FieldTypeUnknown FieldType = iota + 1
	FieldTypeString
	FieldTypeNumber
	FieldTypeInteger
	FieldTypeDecimal
	FieldTypeBoolean
	FieldTypeArray
	FieldTypeSet
	FieldTypeEnum
	FieldTypeObject
	FieldTypeRecord
	FieldTypeUnion
	FieldTypeComposite
	FieldTypeGeometry
	FieldTypeBytes
)

// Internal map for fast lookups
var (
	fieldTypeToString = map[FieldType]string{
		FieldTypeUnknown:   "unknown",
		FieldTypeString:    "string",
		FieldTypeNumber:    "number",
		FieldTypeInteger:   "integer",
		FieldTypeDecimal:   "decimal",
		FieldTypeBoolean:   "boolean",
		FieldTypeArray:     "array",
		FieldTypeSet:       "set",
		FieldTypeEnum:      "enum",
		FieldTypeObject:    "object",
		FieldTypeRecord:    "record",
		FieldTypeUnion:     "union",
		FieldTypeComposite: "composite",
		FieldTypeGeometry:  "geometry",
		FieldTypeBytes:     "bytes",
	}

	stringToFieldType = map[string]FieldType{
		"unknown":   FieldTypeUnknown,
		"string":    FieldTypeString,
		"number":    FieldTypeNumber,
		"integer":   FieldTypeInteger,
		"decimal":   FieldTypeDecimal,
		"boolean":   FieldTypeBoolean,
		"array":     FieldTypeArray,
		"set":       FieldTypeSet,
		"enum":      FieldTypeEnum,
		"object":    FieldTypeObject,
		"record":    FieldTypeRecord,
		"union":     FieldTypeUnion,
		"composite": FieldTypeComposite,
		"geometry":  FieldTypeGeometry,
		"bytes":     FieldTypeBytes,
	}
)

func (t FieldType) String() string {
	if s, ok := fieldTypeToString[t]; ok {
		return s
	}
	return ""
}

func (t FieldType) MarshalJSON() ([]byte, error) {
	val, err := json.Marshal(t.String())
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err).WithOperation("FieldType.MarshalJSON")
	}
	return val, nil
}

func (t *FieldType) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = 0
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("FieldType.UnmarshalJSON")
	}
	if val, ok := stringToFieldType[s]; ok {
		*t = val
		return nil
	}
	*t = FieldTypeUnknown
	return nil
}

func (t FieldType) IsContainer() bool {
	switch t {
	case FieldTypeArray, FieldTypeSet, FieldTypeGeometry,
		FieldTypeRecord, FieldTypeObject, FieldTypeEnum,
		FieldTypeUnion, FieldTypeComposite:
		return true
	default:
		return false
	}
}

func (t FieldType) IsComplex() bool {
	return t.IsContainer()
}

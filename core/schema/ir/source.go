package ir

// source.go defines Go structs that mirror the meta_schema.json source format.
// These types exist solely to receive the result of JSON unmarshalling in
// parse.go. No IR type appears here. All field names use json tags matching
// the exact keys in the source document.

// sourceSchema is the top-level source document.
type sourceSchema struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Version     string                       `json:"version"`
	Concrete    bool                         `json:"concrete"`
	Fields      map[string]sourceField       `json:"fields"`
	Schemas     map[string]sourceNestedSchema `json:"schemas"`
	Indexes     map[string]sourceIndex       `json:"indexes"`
	Constraints map[string]sourceConstraint  `json:"constraints"`
	Metadata    map[string]any               `json:"metadata"`
}

// sourceField is a field definition within a schema.
type sourceField struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Type        string                       `json:"type"`
	Schema      any                          `json:"schema"`
	Required    bool                         `json:"required"`
	Unique      bool                         `json:"unique"`
	Deprecated  bool                         `json:"deprecated"`
	Default     any                          `json:"default"`
}

// sourceFieldRef is a FieldSchema reference — either a named ref (id) or an
// inline type definition (type + optional values). For union/composite fields
// the schema is an array of refs; this is handled by sourceFieldRefList.
//
// json.Unmarshal into any and then type-switch in the resolver handles the
// union between FieldSchema (object) and FieldSchemaArray (array).
type sourceFieldRef struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Values []any  `json:"values"`
}

// sourceNestedSchema is a named nested schema. It is either an object schema
// (has fields) or a type schema (has type). The two forms map to
// NestedSchemaObject and NestedSchemaType in the meta_schema.
type sourceNestedSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Concrete    bool                   `json:"concrete"`
	Values      []any                  `json:"values"`
	Fields      map[string]sourceField `json:"fields"`
	Indexes     map[string]sourceIndex `json:"indexes"`
	Metadata    map[string]any         `json:"metadata"`
	// Schema is the element/variant schema reference for type schemas (array,
	// set, record, union, composite). Raw JSON is preserved for deferred
	// parsing because the shape differs between single-ref and array-of-refs.
	Schema any `json:"schema"`
}

// sourceIndex is an index definition.
type sourceIndex struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Type        string               `json:"type"`
	Order       string               `json:"order"`
	Unique      bool                 `json:"unique"`
	Fields      []string             `json:"fields"`
	Condition   *sourceIndexCondition `json:"condition"`
}

// sourceIndexCondition is either a leaf condition or a group. The actual shape
// is determined at resolution time by inspecting which fields are present.
// A leaf has field+operator+value; a group has operator+conditions.
type sourceIndexCondition struct {
	// Leaf fields
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	// Group fields
	Conditions []*sourceIndexCondition `json:"conditions"`
}

// sourceConstraint is a composite of ConstraintMetadata + ConstraintRule.
// The predicate field distinguishes a leaf rule from a group (which has operator+rules).
type sourceConstraint struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Predicate   string                 `json:"predicate"`
	Fields      []string               `json:"fields"`
	Parameters  any                    `json:"parameters"`
	Operator    string                 `json:"operator"`
	Rules       []*sourceConstraint    `json:"rules"`
}

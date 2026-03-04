// Package ir provides a direct JSON-to-IR compiler that transforms a
// schema definition (schema.json) into a CompiledEntry without constructing any
// intermediate Go definition structs.
//
// The compilation is a two-pass linear algorithm:
//
//	Pass 1 — JSON stream traversal
//	  Assigns schemaIdx and fieldIdx ordinals, resolves field types and kinds,
//	  resolves constraint/index path names to UUID sequences, and collects all
//	  metadata into flat scratch structures. UUIDs are sorted at the end of
//	  Pass 1 to produce the final stable ordinal assignment.
//
//	Pass 2 — scratch traversal
//	  Uses SchemaBuilder to allocate and populate the three contiguous backing
//	  arrays (fields, complex, variants). Builds SchemaSlot sub-slices,
//	  CompiledComplex entries, resolves ResolvedPaths as []FieldDescriptor,
//	  builds []CompiledConstraint and []CompiledIndex, and populates the
//	  values DataContainer keyed by (schemaIdx<<7|fieldIdx).
//
// No JSON is re-read in Pass 2. No definition.Schema is ever constructed.
package ir

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/document"
)

// fieldPos is the compiled (schemaIdx, fieldIdx) ordinal pair for a field.
type fieldPos struct{ schemaIdx, fieldIdx uint8 }

// fieldKey packs (schemaIdx, fieldIdx) into a uint16 for use as a DataPoint id.
// Bits 14-7: schemaIdx (7 bits). Bits 6-0: fieldIdx (7 bits).
func fieldKey(schemaIdx, fieldIdx uint8) int32 {
	return int32(uint16(schemaIdx)<<7 | uint16(fieldIdx))
}

// =============================================================================
// UUID HELPERS
// =============================================================================

// parseUUID parses a canonical UUID string (8-4-4-4-12) into a [16]byte.
func parseUUID(s string) ([16]byte, error) {
	var b [16]byte
	if len(s) != 36 {
		return b, fmt.Errorf("compile: invalid UUID length %d: %q", len(s), s)
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return b, fmt.Errorf("compile: malformed UUID (bad hyphen positions): %q", s)
	}
	hex := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	if len(hex) != 32 {
		return b, fmt.Errorf("compile: malformed UUID (unexpected hex length): %q", s)
	}
	for i := 0; i < 16; i++ {
		hi, ok1 := hexNibble(hex[i*2])
		lo, ok2 := hexNibble(hex[i*2+1])
		if !ok1 || !ok2 {
			return b, fmt.Errorf("compile: invalid hex character in UUID: %q", s)
		}
		b[i] = hi<<4 | lo
	}
	return b, nil
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// =============================================================================
// FIELDTYPE → DATATYPE AND KIND MAPPING
// =============================================================================

// fieldTypeToDataType maps the schema FieldType string to the document DataType.
func fieldTypeToDataType(ft FieldType) document.DataType {
	switch ft {
	case FieldTypeString:
		return document.TypeString
	case FieldTypeInteger, FieldTypeDecimal:
		return document.TypeInt
	case FieldTypeNumber:
		return document.TypeFloat
	case FieldTypeBoolean:
		return document.TypeBool
	case FieldTypeBytes:
		return document.TypeBytes
	case FieldTypeGeometry:
		return document.TypeGeometry
	case FieldTypeEnum:
		return document.TypeInt // stored as ordinal
	case FieldTypeObject, FieldTypeComposite:
		return document.TypeArrayObject
	case FieldTypeRecord:
		return document.TypeRecord
	case FieldTypeUnion:
		return document.TypeUnknown
	default:
		return document.TypeUnknown
	}
}

// arrayElementDataType maps an element FieldType to the TypeArray* variant.
func arrayElementDataType(elemType FieldType) document.DataType {
	switch elemType {
	case FieldTypeString:
		return document.TypeArrayString
	case FieldTypeInteger, FieldTypeDecimal, FieldTypeEnum:
		return document.TypeArrayInt
	case FieldTypeNumber:
		return document.TypeArrayFloat
	case FieldTypeBoolean:
		return document.TypeArrayBool
	case FieldTypeBytes:
		return document.TypeArrayBytes
	case FieldTypeGeometry:
		return document.TypeArrayGeometry
	case FieldTypeObject, FieldTypeComposite, FieldTypeRecord:
		return document.TypeArrayObject
	default:
		return document.TypeArrayUnknown
	}
}

// fieldKindFor determines the FieldKind from a FieldType.
func fieldKindFor(ft FieldType) FieldKind {
	switch ft {
	case FieldTypeObject, FieldTypeRecord:
		return FieldKindObject
	case FieldTypeArray, FieldTypeSet:
		return FieldKindArray
	case FieldTypeUnion, FieldTypeComposite:
		return FieldKindComplex
	default:
		return FieldKindSimple
	}
}

// complexKindFor returns ComplexKind for union and composite types.
// Only meaningful when fieldKindFor returns FieldKindComplex.
func complexKindFor(ft FieldType) ComplexKind {
	if ft == FieldTypeComposite {
		return ComplexComposite
	}
	return ComplexUnion
}

// parseIndexOrder parses the order string from JSON into a typed IndexOrder.
func parseIndexOrder(s string) IndexOrder {
	if s == "desc" {
		return IndexOrderDesc
	}
	return IndexOrderAsc
}

// =============================================================================
// PASS 1 SCRATCH STRUCTURES
// =============================================================================

// scratchField is the flat, pre-sort representation of a single field.
type scratchField struct {
	uuid       [16]byte
	name       string
	schemaUUID [16]byte

	fieldType  FieldType
	dataType   document.DataType
	kind       FieldKind

	required    bool
	deprecated  bool
	unique      bool
	hasDefault  bool
	description string
	terminal    bool // set during cycle detection at end of Pass 1

	// refUUIDs holds the referenced schema UUID(s).
	// FieldKindObject/FieldKindArray: refUUIDs[0] is the single child schema.
	// FieldKindComplex: all entries are variant/constituent schema UUIDs.
	refUUIDs [][16]byte

	defaultVal LiteralValue
}

// scratchSchema is the flat representation of a single nested schema slot.
type scratchSchema struct {
	uuid        [16]byte
	name        string
	description string
	version     string
	concrete    bool
	metadata    map[string]any

	// fieldUUIDs ordered within this schema — becomes fieldIdx after sort.
	fieldUUIDs [][16]byte

	// For type-mode schemas (non-object).
	schemaType FieldType
	refUUIDs   [][16]byte
	values     []LiteralValue // enum-mode only

	// name → UUID for constraint/index path resolution within this schema.
	nameToUUID map[string][16]byte

	// Constraints and indexes defined on this nested schema.
	constraints []scratchConstraint
	indexes     []scratchIndex
}

// unresolvedPath holds a dot-split field name path pending resolution.
type unresolvedPath struct {
	parts []string
}

// scratchConstraint holds a parsed constraint before path resolution.
type scratchConstraint struct {
	uuid        [16]byte
	name        string
	description string
	predicate   PredicateName
	parameters  LiteralValue
	paths       []unresolvedPath
	isGroup     bool
	groupOp     string
	groupRules  []scratchConstraint
}

// scratchIndex holds a parsed index before path resolution.
type scratchIndex struct {
	uuid         [16]byte
	name         string
	description  string
	indexType    IndexType
	unique       bool
	order        string
	fieldPaths   []unresolvedPath
	hasCondition bool
	condition    scratchIndexCondition
}

type scratchIndexCondition struct {
	isGroup   bool
	fieldPath unresolvedPath
	operator  string
	value     LiteralValue
	groupOp   string
	children  []scratchIndexCondition
}

// compilerState is the full Pass 1 accumulator.
type compilerState struct {
	schemaUUIDs [][16]byte
	schemas     map[[16]byte]*scratchSchema
	fields      map[[16]byte]*scratchField
	fieldUUIDs  [][16]byte // global; sorted at end of Pass 1

	rootName        string
	rootDescription string
	rootVersion     string
	rootConcrete    bool
	rootMetadata    map[string]any

	constraints []scratchConstraint
	indexes     []scratchIndex

	adjacency map[[16]byte][][16]byte
}

func newCompilerState() *compilerState {
	return &compilerState{
		schemas:   make(map[[16]byte]*scratchSchema),
		fields:    make(map[[16]byte]*scratchField),
		adjacency: make(map[[16]byte][][16]byte),
	}
}

// =============================================================================
// PASS 1 — JSON STREAM TRAVERSAL
// =============================================================================

func pass1(r io.Reader, cs *compilerState) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("compile: read: %w", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return fmt.Errorf("compile: parse top-level: %w", err)
	}

	if v, ok := top["name"]; ok {
		_ = json.Unmarshal(v, &cs.rootName)
	}
	if v, ok := top["description"]; ok {
		_ = json.Unmarshal(v, &cs.rootDescription)
	}
	if v, ok := top["version"]; ok {
		_ = json.Unmarshal(v, &cs.rootVersion)
	}
	if v, ok := top["concrete"]; ok {
		_ = json.Unmarshal(v, &cs.rootConcrete)
	}
	if v, ok := top["metadata"]; ok {
		_ = json.Unmarshal(v, &cs.rootMetadata)
	}

	// Parse nested schemas first — fields may reference them.
	if v, ok := top["schemas"]; ok {
		if err := pass1Schemas(v, cs); err != nil {
			return err
		}
	}

	if v, ok := top["fields"]; ok {
		if err := pass1Fields(v, cs, [16]byte{}); err != nil {
			return err
		}
	}

	if v, ok := top["constraints"]; ok {
		constraints, err := pass1Constraints(v)
		if err != nil {
			return err
		}
		cs.constraints = constraints
	}

	if v, ok := top["indexes"]; ok {
		indexes, err := pass1Indexes(v)
		if err != nil {
			return err
		}
		cs.indexes = indexes
	}

	return pass1Finalise(cs)
}

func pass1Schemas(raw json.RawMessage, cs *compilerState) error {
	var schemaMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schemaMap); err != nil {
		return fmt.Errorf("compile: parse schemas: %w", err)
	}
	for uuidStr, schemaRaw := range schemaMap {
		uid, err := parseUUID(uuidStr)
		if err != nil {
			return fmt.Errorf("compile: schema key: %w", err)
		}
		ss, err := pass1ParseNestedSchema(schemaRaw, uid, cs)
		if err != nil {
			return fmt.Errorf("compile: schema %s: %w", uuidStr, err)
		}
		cs.schemas[uid] = ss
		cs.schemaUUIDs = append(cs.schemaUUIDs, uid)
	}
	return nil
}

func pass1ParseNestedSchema(raw json.RawMessage, uid [16]byte, cs *compilerState) (*scratchSchema, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse nested schema: %w", err)
	}

	ss := &scratchSchema{
		uuid:       uid,
		nameToUUID: make(map[string][16]byte),
	}

	if v, ok := obj["name"]; ok {
		_ = json.Unmarshal(v, &ss.name)
	}
	if v, ok := obj["description"]; ok {
		_ = json.Unmarshal(v, &ss.description)
	}
	if v, ok := obj["concrete"]; ok {
		_ = json.Unmarshal(v, &ss.concrete)
	}
	if v, ok := obj["metadata"]; ok {
		_ = json.Unmarshal(v, &ss.metadata)
	}

	_, hasFields := obj["fields"]
	_, hasType := obj["type"]

	if hasFields && hasType {
		return nil, fmt.Errorf("nested schema %q: both 'fields' and 'type' present", ss.name)
	}

	if hasFields {
		if err := pass1Fields(obj["fields"], cs, uid); err != nil {
			return nil, err
		}
		var fieldMap map[string]json.RawMessage
		if err := json.Unmarshal(obj["fields"], &fieldMap); err != nil {
			return nil, err
		}
		for uuidStr := range fieldMap {
			fuid, err := parseUUID(uuidStr)
			if err != nil {
				return nil, err
			}
			ss.fieldUUIDs = append(ss.fieldUUIDs, fuid)
		}
		for _, fuid := range ss.fieldUUIDs {
			if sf, ok := cs.fields[fuid]; ok {
				ss.nameToUUID[sf.name] = fuid
			}
		}
		if v, ok := obj["constraints"]; ok {
			constraints, err := pass1Constraints(v)
			if err != nil {
				return nil, err
			}
			ss.constraints = constraints
		}
		if v, ok := obj["indexes"]; ok {
			indexes, err := pass1Indexes(v)
			if err != nil {
				return nil, err
			}
			ss.indexes = indexes
		}
	} else if hasType {
		var typeStr string
		_ = json.Unmarshal(obj["type"], &typeStr)
		ss.schemaType = parseFieldType(typeStr)

		if v, ok := obj["values"]; ok {
			var rawVals []json.RawMessage
			if err := json.Unmarshal(v, &rawVals); err != nil {
				return nil, fmt.Errorf("parse enum values: %w", err)
			}
			for _, rv := range rawVals {
				var lv LiteralValue
				if err := json.Unmarshal(rv, &lv); err != nil {
					return nil, fmt.Errorf("parse enum value: %w", err)
				}
				ss.values = append(ss.values, lv)
			}
		}

		if v, ok := obj["schema"]; ok {
			refs, err := pass1ParseSchemaRef(v)
			if err != nil {
				return nil, err
			}
			ss.refUUIDs = refs
		}
	}

	return ss, nil
}

func pass1Fields(raw json.RawMessage, cs *compilerState, ownerUUID [16]byte) error {
	var fieldMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fieldMap); err != nil {
		return fmt.Errorf("compile: parse fields: %w", err)
	}
	for uuidStr, fieldRaw := range fieldMap {
		uid, err := parseUUID(uuidStr)
		if err != nil {
			return fmt.Errorf("compile: field key: %w", err)
		}
		if _, exists := cs.fields[uid]; exists {
			return fmt.Errorf("compile: duplicate field UUID %s", uuidStr)
		}
		sf, err := pass1ParseField(fieldRaw, uid, ownerUUID, cs)
		if err != nil {
			return fmt.Errorf("compile: field %s: %w", uuidStr, err)
		}
		cs.fields[uid] = sf
		cs.fieldUUIDs = append(cs.fieldUUIDs, uid)
	}
	return nil
}

func pass1ParseField(raw json.RawMessage, uid, ownerUUID [16]byte, cs *compilerState) (*scratchField, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse field: %w", err)
	}

	sf := &scratchField{uuid: uid, schemaUUID: ownerUUID}

	if v, ok := obj["name"]; ok {
		_ = json.Unmarshal(v, &sf.name)
	}
	if v, ok := obj["description"]; ok {
		_ = json.Unmarshal(v, &sf.description)
	}
	if v, ok := obj["required"]; ok {
		_ = json.Unmarshal(v, &sf.required)
	}
	if v, ok := obj["deprecated"]; ok {
		_ = json.Unmarshal(v, &sf.deprecated)
	}
	if v, ok := obj["unique"]; ok {
		_ = json.Unmarshal(v, &sf.unique)
	}

	var typeStr string
	if v, ok := obj["type"]; ok {
		_ = json.Unmarshal(v, &typeStr)
	}
	sf.fieldType = parseFieldType(typeStr)
	sf.kind = fieldKindFor(sf.fieldType)
	sf.dataType = fieldTypeToDataType(sf.fieldType)

	if v, ok := obj["schema"]; ok {
		refs, err := pass1ParseSchemaRef(v)
		if err != nil {
			return nil, fmt.Errorf("field %q schema ref: %w", sf.name, err)
		}
		sf.refUUIDs = refs
	}

	if v, ok := obj["default"]; ok {
		var lv LiteralValue
		if err := json.Unmarshal(v, &lv); err != nil {
			return nil, fmt.Errorf("field %q default: %w", sf.name, err)
		}
		if !lv.IsZero() && !lv.IsNull() {
			sf.defaultVal = lv
			sf.hasDefault = true
		}
	}

	return sf, nil
}

func pass1ParseSchemaRef(raw json.RawMessage) ([][16]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch raw[0] {
	case '{':
		uid, err := pass1ParseSingleRef(raw)
		if err != nil {
			return nil, err
		}
		return [][16]byte{uid}, nil
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("parse schema ref array: %w", err)
		}
		result := make([][16]byte, 0, len(arr))
		for _, item := range arr {
			uid, err := pass1ParseSingleRef(item)
			if err != nil {
				return nil, err
			}
			result = append(result, uid)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unexpected schema ref format: %q", string(raw))
	}
}

func pass1ParseSingleRef(raw json.RawMessage) ([16]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return [16]byte{}, fmt.Errorf("parse schema ref object: %w", err)
	}
	if idRaw, ok := obj["id"]; ok {
		var idStr string
		if err := json.Unmarshal(idRaw, &idStr); err != nil {
			return [16]byte{}, fmt.Errorf("parse schema ref id: %w", err)
		}
		return parseUUID(idStr)
	}
	return [16]byte{}, fmt.Errorf("inline schema references are not supported; use named schema refs")
}

func pass1Constraints(raw json.RawMessage) ([]scratchConstraint, error) {
	var constraintMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &constraintMap); err != nil {
		return nil, fmt.Errorf("parse constraints: %w", err)
	}
	result := make([]scratchConstraint, 0, len(constraintMap))
	for uuidStr, cRaw := range constraintMap {
		sc, err := pass1ParseConstraint(cRaw)
		if err != nil {
			return nil, err
		}
		uid, err := parseUUID(uuidStr)
		if err != nil {
			return nil, fmt.Errorf("constraint key: %w", err)
		}
		sc.uuid = uid
		result = append(result, sc)
	}
	return result, nil
}

// pass1ParseConstraint parses a top-level Constraint — always a composite of
// ConstraintMetadata and ConstraintRule, never a group.
func pass1ParseConstraint(raw json.RawMessage) (scratchConstraint, error) {
	var obj map[string]json.RawMessage
	var sc scratchConstraint
	if err := json.Unmarshal(raw, &obj); err != nil {
		return sc, fmt.Errorf("parse constraint: %w", err)
	}
	if v, ok := obj["name"]; ok {
		_ = json.Unmarshal(v, &sc.name)
	}
	if v, ok := obj["description"]; ok {
		_ = json.Unmarshal(v, &sc.description)
	}

	// Delegate to pass1ParseConstraintUnion to handle both groups and rules.
	union, err := pass1ParseConstraintUnion(raw)
	if err != nil {
		return sc, err
	}
	// Copy the union fields into sc, preserving the name/description already parsed.
	sc.isGroup = union.isGroup
	sc.groupOp = union.groupOp
	sc.groupRules = union.groupRules
	sc.predicate = union.predicate
	sc.parameters = union.parameters
	sc.paths = union.paths

	return sc, nil
}

// pass1ParseConstraintUnion parses one entry inside a constraint group's rules[].
// Each entry is either a ConstraintRule (has predicate) or a ConstraintGroup
// (has operator+rules).
func pass1ParseConstraintUnion(raw json.RawMessage) (scratchConstraint, error) {
	var obj map[string]json.RawMessage
	var sc scratchConstraint
	if err := json.Unmarshal(raw, &obj); err != nil {
		return sc, fmt.Errorf("parse constraint union: %w", err)
	}
	_, hasOperator := obj["operator"]
	_, hasPredicate := obj["predicate"]
	if hasOperator && hasPredicate {
		return sc, fmt.Errorf("constraint union entry cannot have both 'operator' and 'predicate'")
	}
	if hasOperator {
		sc.isGroup = true
		_ = json.Unmarshal(obj["operator"], &sc.groupOp)
		if v, ok := obj["rules"]; ok {
			var rulesRaw []json.RawMessage
			if err := json.Unmarshal(v, &rulesRaw); err != nil {
				return sc, fmt.Errorf("constraint group rules: %w", err)
			}
			for _, rr := range rulesRaw {
				child, err := pass1ParseConstraintUnion(rr)
				if err != nil {
					return sc, err
				}
				sc.groupRules = append(sc.groupRules, child)
			}
		}
		return sc, nil
	}
	if hasPredicate {
		_ = json.Unmarshal(obj["predicate"], &sc.predicate)
		if v, ok := obj["parameters"]; ok {
			_ = json.Unmarshal(v, &sc.parameters)
		}
		if v, ok := obj["fields"]; ok {
			var fieldNames []string
			if err := json.Unmarshal(v, &fieldNames); err != nil {
				return sc, fmt.Errorf("constraint union rule fields: %w", err)
			}
			for _, name := range fieldNames {
				sc.paths = append(sc.paths, unresolvedPath{parts: strings.Split(name, ".")})
			}
		}
		return sc, nil
	}
	return sc, fmt.Errorf("constraint union entry must have either 'operator' or 'predicate'")
}

func pass1Indexes(raw json.RawMessage) ([]scratchIndex, error) {
	var indexMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &indexMap); err != nil {
		return nil, fmt.Errorf("parse indexes: %w", err)
	}
	result := make([]scratchIndex, 0, len(indexMap))
	for uuidStr, iRaw := range indexMap {
		si, err := pass1ParseIndex(iRaw)
		if err != nil {
			return nil, err
		}
		uid, err := parseUUID(uuidStr)
		if err != nil {
			return nil, fmt.Errorf("index key: %w", err)
		}
		si.uuid = uid
		result = append(result, si)
	}
	return result, nil
}

func pass1ParseIndex(raw json.RawMessage) (scratchIndex, error) {
	var obj map[string]json.RawMessage
	var si scratchIndex
	if err := json.Unmarshal(raw, &obj); err != nil {
		return si, fmt.Errorf("parse index: %w", err)
	}
	if v, ok := obj["name"]; ok {
		_ = json.Unmarshal(v, &si.name)
	}
	if v, ok := obj["description"]; ok {
		_ = json.Unmarshal(v, &si.description)
	}
	if v, ok := obj["unique"]; ok {
		_ = json.Unmarshal(v, &si.unique)
	}
	if v, ok := obj["order"]; ok {
		_ = json.Unmarshal(v, &si.order)
	}
	var typeStr string
	if v, ok := obj["type"]; ok {
		_ = json.Unmarshal(v, &typeStr)
	}
	si.indexType = parseIndexType(typeStr)
	if v, ok := obj["fields"]; ok {
		var fieldPaths []string
		if err := json.Unmarshal(v, &fieldPaths); err != nil {
			return si, fmt.Errorf("index %q fields: %w", si.name, err)
		}
		for _, p := range fieldPaths {
			si.fieldPaths = append(si.fieldPaths, unresolvedPath{parts: strings.Split(p, ".")})
		}
	}
	if v, ok := obj["condition"]; ok {
		cond, err := pass1ParseIndexCondition(v)
		if err != nil {
			return si, fmt.Errorf("index %q condition: %w", si.name, err)
		}
		si.hasCondition = true
		si.condition = cond
	}
	return si, nil
}

func pass1ParseIndexCondition(raw json.RawMessage) (scratchIndexCondition, error) {
	var obj map[string]json.RawMessage
	var c scratchIndexCondition
	if err := json.Unmarshal(raw, &obj); err != nil {
		return c, fmt.Errorf("parse index condition: %w", err)
	}
	if _, ok := obj["conditions"]; ok {
		c.isGroup = true
		_ = json.Unmarshal(obj["operator"], &c.groupOp)
		var childRaws []json.RawMessage
		if err := json.Unmarshal(obj["conditions"], &childRaws); err != nil {
			return c, fmt.Errorf("parse index condition group children: %w", err)
		}
		for _, cr := range childRaws {
			child, err := pass1ParseIndexCondition(cr)
			if err != nil {
				return c, err
			}
			c.children = append(c.children, child)
		}
		return c, nil
	}
	var fieldName string
	if v, ok := obj["field"]; ok {
		_ = json.Unmarshal(v, &fieldName)
	}
	c.fieldPath = unresolvedPath{parts: strings.Split(fieldName, ".")}
	if v, ok := obj["operator"]; ok {
		_ = json.Unmarshal(v, &c.operator)
	}
	if v, ok := obj["value"]; ok {
		_ = json.Unmarshal(v, &c.value)
	}
	return c, nil
}

// =============================================================================
// PASS 1 FINALISE — SORT, RESOLVE ARRAY TYPES, DETECT CYCLES
// =============================================================================

func pass1Finalise(cs *compilerState) error {
	// 1. Sort schema UUIDs for stable schemaIdx assignment.
	sort.Slice(cs.schemaUUIDs, func(i, j int) bool {
		a, b := cs.schemaUUIDs[i], cs.schemaUUIDs[j]
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})

	// Sort field UUIDs within each schema slot for stable fieldIdx assignment.
	for _, ss := range cs.schemas {
		sort.Slice(ss.fieldUUIDs, func(i, j int) bool {
			a, b := ss.fieldUUIDs[i], ss.fieldUUIDs[j]
			for k := range a {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
			return false
		})
	}

	// Sort global field UUIDs (used only for deterministic iteration in Pass 2).
	sort.Slice(cs.fieldUUIDs, func(i, j int) bool {
		a, b := cs.fieldUUIDs[i], cs.fieldUUIDs[j]
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})

	// 2. Resolve array/set element DataTypes.
	for _, sf := range cs.fields {
		if sf.fieldType != FieldTypeArray && sf.fieldType != FieldTypeSet {
			continue
		}
		if len(sf.refUUIDs) == 0 {
			sf.dataType = document.TypeArrayUnknown
			continue
		}
		elemSchema, ok := cs.schemas[sf.refUUIDs[0]]
		if !ok {
			sf.dataType = document.TypeArrayUnknown
			continue
		}
		var elemType FieldType
		if elemSchema.schemaType != 0 {
			elemType = elemSchema.schemaType
		} else if len(elemSchema.fieldUUIDs) > 0 {
			elemType = FieldTypeObject
		}
		sf.dataType = arrayElementDataType(elemType)
	}

	// 3. Cycle detection — mark back-edge fields as terminal.
	type edgeKey struct{ from, to [16]byte }
	edgeFields := make(map[edgeKey][]*scratchField)
	for _, sf := range cs.fields {
		for _, refUID := range sf.refUUIDs {
			key := edgeKey{sf.schemaUUID, refUID}
			edgeFields[key] = append(edgeFields[key], sf)
			cs.adjacency[sf.schemaUUID] = append(cs.adjacency[sf.schemaUUID], refUID)
		}
	}
	visited := make(map[[16]byte]int) // 0: unvisited, 1: visiting, 2: visited
	var dfs func(uid [16]byte)
	dfs = func(uid [16]byte) {
		visited[uid] = 1
		for _, child := range cs.adjacency[uid] {
			if visited[child] == 1 {
				// Back-edge detected from 'uid' to 'child'.
				// Mark all fields from 'uid' that point to 'child' as terminal.
				for _, sf := range edgeFields[edgeKey{uid, child}] {
					sf.terminal = true
				}
			} else if visited[child] == 0 {
				dfs(child)
			}
		}
		visited[uid] = 2
	}
	for _, uid := range cs.schemaUUIDs {
		if visited[uid] == 0 {
			dfs(uid)
		}
	}
	// Also check root-level fields for cycles.
	if visited[[16]byte{}] == 0 {
		dfs([16]byte{})
	}

	// 4. Build nameToUUID for root-level fields (schemaUUID == zero).
	rootSchema := &scratchSchema{nameToUUID: make(map[string][16]byte)}
	for _, fuid := range cs.fieldUUIDs {
		sf, ok := cs.fields[fuid]
		if !ok {
			continue
		}
		if sf.schemaUUID == ([16]byte{}) {
			rootSchema.nameToUUID[sf.name] = fuid
		}
	}
	cs.schemas[[16]byte{}] = rootSchema

	return nil
}

// =============================================================================
// PASS 2 — SCRATCH → COMPILED IR
// =============================================================================

func pass2(cs *compilerState) (*CompiledEntry, error) {
	// --- Assign schemaIdx: slot 0 = root, 1..N = nested schemas ---
	totalSchemas := len(cs.schemaUUIDs) + 1
	if totalSchemas > 128 {
		return nil, fmt.Errorf("compile: schema count %d exceeds maximum 128", totalSchemas)
	}

	schemaIdxOf := make(map[[16]byte]uint8, len(cs.schemaUUIDs))
	for i, uid := range cs.schemaUUIDs {
		schemaIdxOf[uid] = uint8(i + 1)
	}

	// --- Assign fieldIdx within each schema slot ---
	// fieldPosOf maps field UUID → (schemaIdx, fieldIdx).
	fieldPosOf := make(map[[16]byte]fieldPos, len(cs.fieldUUIDs))

	// Root fields: schemaIdx=0, fieldIdx by position in sorted root field list.
	rootFieldOrd := uint8(0)
	for _, fuid := range cs.fieldUUIDs {
		sf := cs.fields[fuid]
		if sf.schemaUUID == ([16]byte{}) {
			if rootFieldOrd >= 128 {
				return nil, fmt.Errorf("compile: root schema field count exceeds maximum 128")
			}
			fieldPosOf[fuid] = fieldPos{schemaIdx: 0, fieldIdx: rootFieldOrd}
			rootFieldOrd++
		}
	}

	// Nested schema fields: schemaIdx from schemaIdxOf, fieldIdx by position
	// in each schema's sorted fieldUUIDs slice.
	for _, uid := range cs.schemaUUIDs {
		ss := cs.schemas[uid]
		sIdx := schemaIdxOf[uid]
		for fOrd, fuid := range ss.fieldUUIDs {
			if fOrd >= 128 {
				return nil, fmt.Errorf("compile: schema %q field count %d exceeds maximum 128", ss.name, fOrd+1)
			}
			fieldPosOf[fuid] = fieldPos{schemaIdx: sIdx, fieldIdx: uint8(fOrd)}
		}
	}

	// --- Count backing array sizes for SchemaBuilder ---
	// One pass over all schemas to count total fields, complex entries, variants.
	totalFields := 0
	totalComplex := 0
	totalVariants := 0

	// Root slot counts.
	totalFields += int(rootFieldOrd)
	for _, fuid := range cs.fieldUUIDs {
		sf := cs.fields[fuid]
		if sf.schemaUUID == ([16]byte{}) && sf.kind == FieldKindComplex {
			totalComplex++
			totalVariants += len(sf.refUUIDs)
		}
	}

	// Nested slot counts.
	for _, uid := range cs.schemaUUIDs {
		ss := cs.schemas[uid]
		totalFields += len(ss.fieldUUIDs)
		for _, fuid := range ss.fieldUUIDs {
			sf := cs.fields[fuid]
			if sf.kind == FieldKindComplex {
				totalComplex++
				totalVariants += len(sf.refUUIDs)
			}
		}
	}

	// --- Build all slots using SchemaBuilder ---
	var b SchemaBuilder
	b.Alloc(totalFields, totalComplex, totalVariants)

	schemas := make([]SchemaSlot, totalSchemas)

	// buildSlot constructs one SchemaSlot from an ordered list of field UUIDs
	// and the owning schemaIdx. It appends descriptors and complex entries into
	// the builder and returns a populated SchemaSlot with sub-slice windows.
	buildSlot := func(sIdx uint8, orderedFieldUUIDs [][16]byte) (SchemaSlot, error) {
		fds := make([]FieldDescriptor, 0, len(orderedFieldUUIDs))
		complexEntries := make([]CompiledComplex, 0)
		complexOrdinal := uint8(0) // index into this slot's Complex sub-slice

		for _, fuid := range orderedFieldUUIDs {
			sf := cs.fields[fuid]
			pos := fieldPosOf[fuid]

			// Resolve targetIdx based on Kind.
			var targetIdx uint8
			switch sf.kind {
			case FieldKindObject, FieldKindArray:
				// Single child schema.
				if len(sf.refUUIDs) > 0 {
					childIdx, ok := schemaIdxOf[sf.refUUIDs[0]]
					if !ok {
						return SchemaSlot{}, fmt.Errorf(
							"compile: field %q references unknown schema %x",
							sf.name, sf.refUUIDs[0],
						)
					}
					targetIdx = childIdx
				}
			case FieldKindComplex:
				// Multiple variant/constituent schemas.
				// targetIdx is the ordinal of this field's CompiledComplex within
				// this slot's Complex sub-slice.
				targetIdx = complexOrdinal
				complexOrdinal++

				variantIdxs := make([]uint8, 0, len(sf.refUUIDs))
				for _, refUID := range sf.refUUIDs {
					vIdx, ok := schemaIdxOf[refUID]
					if !ok {
						return SchemaSlot{}, fmt.Errorf(
							"compile: complex field %q references unknown schema %x",
							sf.name, refUID,
						)
					}
					variantIdxs = append(variantIdxs, vIdx)
				}
				variantSlice := b.AppendVariants(variantIdxs)
				complexEntries = append(complexEntries, CompiledComplex{
					Kind:     complexKindFor(sf.fieldType),
					Variants: variantSlice,
				})
			}

			// Build the packed descriptor.
			fd := FieldDescriptor(uint32(sf.dataType)<<28 |
				uint32(sf.kind)<<26 |
				uint32(pos.schemaIdx)<<19 |
				uint32(pos.fieldIdx)<<12)
			if sf.required {
				fd |= FieldDescriptor(fdRequired)
			}
			if sf.hasDefault {
				fd |= FieldDescriptor(fdHasDefault)
			}
			if sf.deprecated {
				fd |= FieldDescriptor(fdDeprecated)
			}
			if sf.unique {
				fd |= FieldDescriptor(fdUnique)
			}
			if sf.terminal {
				fd |= FieldDescriptor(fdTerminal)
			}
			fd |= FieldDescriptor(targetIdx) // bits 6-0

			fds = append(fds, fd)
		}

		fieldSlice := b.AppendFields(fds)
		complexSlice := b.AppendComplex(complexEntries)

		return SchemaSlot{
			Idx:     sIdx,
			Fields:  fieldSlice,
			Complex: complexSlice,
		}, nil
	}

	// Build root slot (schemaIdx=0).
	rootFieldUUIDs := make([][16]byte, 0, rootFieldOrd)
	for _, fuid := range cs.fieldUUIDs {
		if cs.fields[fuid].schemaUUID == ([16]byte{}) {
			rootFieldUUIDs = append(rootFieldUUIDs, fuid)
		}
	}
	rootSlot, err := buildSlot(0, rootFieldUUIDs)
	if err != nil {
		return nil, fmt.Errorf("compile: root slot: %w", err)
	}
	schemas[0] = rootSlot

	// Build nested slots.
	for _, uid := range cs.schemaUUIDs {
		ss := cs.schemas[uid]
		sIdx := schemaIdxOf[uid]
		slot, err := buildSlot(sIdx, ss.fieldUUIDs)
		if err != nil {
			return nil, fmt.Errorf("compile: slot %q: %w", ss.name, err)
		}
		schemas[sIdx] = slot
	}

	// --- Populate values DataContainer ---
	// Keyed by fieldKey(schemaIdx, fieldIdx) — stable across schema versions.
	values := document.NewDataContainer()
	if err := pass2PopulateValues(values, cs, fieldPosOf, schemaIdxOf); err != nil {
		return nil, err
	}

	core := b.Build(schemas, values)

	// --- Resolve constraints ---
	compiledConstraints, err := pass2ResolveConstraints(cs.constraints, cs, fieldPosOf, [16]byte{})
	if err != nil {
		return nil, fmt.Errorf("compile: constraints: %w", err)
	}

	// --- Resolve indexes ---
	compiledIndexes, err := pass2ResolveIndexes(cs.indexes, cs, fieldPosOf, [16]byte{})
	if err != nil {
		return nil, fmt.Errorf("compile: indexes: %w", err)
	}

	// --- Resolve nested schema constraints and indexes ---
	var nestedConstraints map[uint8][]CompiledConstraint
	var nestedIndexes map[uint8][]CompiledIndex

	for _, uid := range cs.schemaUUIDs {
		ss := cs.schemas[uid]
		sIdx := schemaIdxOf[uid]

		if len(ss.constraints) > 0 {
			cc, err := pass2ResolveConstraints(ss.constraints, cs, fieldPosOf, uid)
			if err != nil {
				return nil, fmt.Errorf("compile: nested schema %q constraints: %w", ss.name, err)
			}
			if nestedConstraints == nil {
				nestedConstraints = make(map[uint8][]CompiledConstraint)
			}
			nestedConstraints[sIdx] = cc
		}

		if len(ss.indexes) > 0 {
			ci, err := pass2ResolveIndexes(ss.indexes, cs, fieldPosOf, uid)
			if err != nil {
				return nil, fmt.Errorf("compile: nested schema %q indexes: %w", ss.name, err)
			}
			if nestedIndexes == nil {
				nestedIndexes = make(map[uint8][]CompiledIndex)
			}
			nestedIndexes[sIdx] = ci
		}
	}

	// --- Build CompiledMeta ---
	meta := pass2BuildMeta(cs, schemaIdxOf, fieldPosOf)

	return &CompiledEntry{
		Core:              core,
		Constraints:       compiledConstraints,
		Indexes:           compiledIndexes,
		NestedConstraints: nestedConstraints,
		NestedIndexes:     nestedIndexes,
		Meta:              meta,
	}, nil
}

// pass2PopulateValues writes enum member ordinals and field defaults into the
// values DataContainer, keyed by fieldKey(schemaIdx, fieldIdx).
func pass2PopulateValues(
	dc *document.DataContainer,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	schemaIdxOf map[[16]byte]uint8,
) error {
	for _, fuid := range cs.fieldUUIDs {
		sf := cs.fields[fuid]
		pos, ok := fieldPosOf[fuid]
		if !ok {
			continue
		}
		key := fieldKey(pos.schemaIdx, pos.fieldIdx)

		// Enum: write the ordered set of enum value ordinals as []int64.
		if sf.fieldType == FieldTypeEnum && len(sf.refUUIDs) == 1 {
			enumSchema, ok := cs.schemas[sf.refUUIDs[0]]
			if !ok {
				continue
			}
			dp, err := document.NewDataPoint(document.TypeArrayInt, key)
			if err != nil {
				return fmt.Errorf("compile: enum DataPoint for field %x: %w", fuid, err)
			}
			ordinals := make([]int64, len(enumSchema.values))
			for i := range enumSchema.values {
				ordinals[i] = int64(i)
			}
			if err := dc.AppendArrayInt(dp, ordinals); err != nil {
				return fmt.Errorf("compile: enum values for field %x: %w", fuid, err)
			}
		}

		// Default value.
		if sf.hasDefault {
			if err := pass2WriteDefault(dc, sf.defaultVal, sf.dataType, key); err != nil {
				return fmt.Errorf("compile: default for field %x: %w", fuid, err)
			}
		}
	}
	return nil
}

// pass2WriteDefault writes a LiteralValue default into the values DataContainer.
func pass2WriteDefault(dc *document.DataContainer, lv LiteralValue, dt document.DataType, id int32) error {
	dp, err := document.NewDataPoint(dt, id)
	if err != nil {
		return err
	}
	switch dt {
	case document.TypeString:
		v, err := LiteralValueAs[string](lv)
		if err != nil {
			return err
		}
		return dc.AppendString(dp, v)
	case document.TypeInt:
		v, err := LiteralValueAs[int64](lv)
		if err != nil {
			return err
		}
		return dc.AppendInt(dp, v)
	case document.TypeFloat:
		v, err := LiteralValueAs[float64](lv)
		if err != nil {
			return err
		}
		return dc.AppendFloat(dp, v)
	case document.TypeBool:
		v, err := LiteralValueAs[bool](lv)
		if err != nil {
			return err
		}
		return dc.AppendBool(dp, v)
	}
	return nil
}

// pass2ResolvePath resolves a dot-notation name path to a ResolvedPath
// ([]FieldDescriptor). Resolution walks the nameToUUID scope chain from the
// starting schema outward, following each field's refUUIDs to the next scope.
//
// Each step in the returned path is the full FieldDescriptor for the field at
// that depth — carrying schemaIdx, fieldIdx, Kind, targetIdx, and all flags.
// No string operations occur after this function returns.
func pass2ResolvePath(
	up unresolvedPath,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) (ResolvedPath, error) {
	if len(up.parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	path := make(ResolvedPath, 0, len(up.parts))

	for i, part := range up.parts {
		scopeSchema, ok := cs.schemas[currentSchemaUID]
		if !ok {
			return nil, fmt.Errorf("no schema scope when resolving path segment %q", part)
		}

		fuid, found := scopeSchema.nameToUUID[part]
		if !found {
			return nil, fmt.Errorf("field %q not found in schema scope", part)
		}

		sf, ok := cs.fields[fuid]
		if !ok {
			return nil, fmt.Errorf("field %q has no scratch record", part)
		}

		pos, ok := fieldPosOf[fuid]
		if !ok {
			return nil, fmt.Errorf("field %q has no assigned position", part)
		}

		// Reconstruct the FieldDescriptor for this field from its scratch data.
		// This mirrors the descriptor built in buildSlot — same bits, same logic.
		var targetIdx uint8
		switch sf.kind {
		case FieldKindObject, FieldKindArray:
			if len(sf.refUUIDs) > 0 {
				// We don't have schemaIdxOf in scope here; derive from fieldPosOf
				// by looking up the child schema's first field.
				if childSS, ok := cs.schemas[sf.refUUIDs[0]]; ok {
					if len(childSS.fieldUUIDs) > 0 {
						if childPos, ok := fieldPosOf[childSS.fieldUUIDs[0]]; ok {
							targetIdx = childPos.schemaIdx
						}
					}
				}
			}
		case FieldKindComplex:
			// For path resolution purposes, targetIdx for FieldKindComplex is not
			// needed — the descriptor is used for type/kind information only.
			// The actual ComplexOf lookup uses the slot's Complex sub-slice at
			// runtime, not a path-embedded index.
			targetIdx = 0
		}

		fd := FieldDescriptor(uint32(sf.dataType)<<28 |
			uint32(sf.kind)<<26 |
			uint32(pos.schemaIdx)<<19 |
			uint32(pos.fieldIdx)<<12)
		if sf.required {
			fd |= FieldDescriptor(fdRequired)
		}
		if sf.hasDefault {
			fd |= FieldDescriptor(fdHasDefault)
		}
		if sf.deprecated {
			fd |= FieldDescriptor(fdDeprecated)
		}
		if sf.unique {
			fd |= FieldDescriptor(fdUnique)
		}
		if sf.terminal {
			fd |= FieldDescriptor(fdTerminal)
		}
		fd |= FieldDescriptor(targetIdx)

		path = append(path, fd)

		// Advance scope for the next segment.
		if i < len(up.parts)-1 {
			switch sf.kind {
			case FieldKindObject, FieldKindArray:
				if len(sf.refUUIDs) == 0 {
					return nil, fmt.Errorf(
						"path segment %q has no ref schema; cannot traverse to %q",
						part, up.parts[i+1],
					)
				}
				currentSchemaUID = sf.refUUIDs[0]
			case FieldKindComplex:
				if sf.fieldType == FieldTypeUnion {
					return nil, fmt.Errorf(
						"path segment %q is a union field; union fields may only appear as the terminal step",
						part,
					)
				}
				// Composite: search constituent schemas for a scope containing the next field.
				nextPart := up.parts[i+1]
				found := false
				for _, refUID := range sf.refUUIDs {
					if constSS, ok := cs.schemas[refUID]; ok {
						if _, exists := constSS.nameToUUID[nextPart]; exists {
							currentSchemaUID = refUID
							found = true
							break
						}
					}
				}
				if !found {
					return nil, fmt.Errorf(
						"field %q not found in any constituent of composite field %q",
						nextPart, part,
					)
				}
			default:
				return nil, fmt.Errorf(
					"path segment %q is a simple field; cannot traverse further to %q",
					part, up.parts[i+1],
				)
			}
		}
	}

	return path, nil
}

// pass2ResolveConstraints resolves []scratchConstraint to []CompiledConstraint.
func pass2ResolveConstraints(
	scratch []scratchConstraint,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) ([]CompiledConstraint, error) {
	result := make([]CompiledConstraint, 0, len(scratch))
	for _, sc := range scratch {
		cc, err := pass2ResolveConstraintUnion(sc, cs, fieldPosOf, currentSchemaUID)
		if err != nil {
			return nil, err
		}
		result = append(result, cc)
	}
	return result, nil
}

func pass2ResolveConstraintRule(
	sc scratchConstraint,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) (CompiledConstraint, error) {
	cc := CompiledConstraint{
		Predicate:  string(sc.predicate),
		Parameters: sc.parameters.Value(),
	}
	for _, up := range sc.paths {
		rp, err := pass2ResolvePath(up, cs, fieldPosOf, currentSchemaUID)
		if err != nil {
			return cc, fmt.Errorf("constraint %q path %v: %w", sc.name, up.parts, err)
		}
		cc.Fields = append(cc.Fields, rp)
	}
	return cc, nil
}

func pass2ResolveConstraintUnion(
	sc scratchConstraint,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) (CompiledConstraint, error) {
	if sc.isGroup {
		if len(sc.groupRules) == 0 {
			return CompiledConstraint{}, fmt.Errorf("constraint group with operator %q is empty", sc.groupOp)
		}
		op, ok := common.LogicalOperatorFrom(sc.groupOp)
		if !ok {
			return CompiledConstraint{}, fmt.Errorf("constraint group: unknown logical operator %q", sc.groupOp)
		}
		compiled := CompiledConstraint{IsGroup: true, Op: op}
		for i, child := range sc.groupRules {
			cc, err := pass2ResolveConstraintUnion(child, cs, fieldPosOf, currentSchemaUID)
			if err != nil {
				return CompiledConstraint{}, fmt.Errorf("constraint group child %d: %w", i, err)
			}
			compiled.Children = append(compiled.Children, cc)
		}
		return compiled, nil
	}
	return pass2ResolveConstraintRule(sc, cs, fieldPosOf, currentSchemaUID)
}

// pass2ResolveIndexCondition resolves a scratchIndexCondition to a
// CompiledIndexCondition. The leaf Field is the terminal FieldDescriptor of
// the resolved path — carrying all structural information needed at runtime.
func pass2ResolveIndexCondition(
	c scratchIndexCondition,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) (CompiledIndexCondition, error) {
	if c.isGroup {
		if len(c.children) == 0 {
			return CompiledIndexCondition{}, fmt.Errorf("index condition group is empty")
		}
		op, ok := common.LogicalOperatorFrom(c.groupOp)
		if !ok {
			return CompiledIndexCondition{}, fmt.Errorf("index condition group: unknown logical operator %q", c.groupOp)
		}
		compiled := CompiledIndexCondition{IsGroup: true, Op: op}
		for i, child := range c.children {
			cc, err := pass2ResolveIndexCondition(child, cs, fieldPosOf, currentSchemaUID)
			if err != nil {
				return CompiledIndexCondition{}, fmt.Errorf("index condition group child %d: %w", i, err)
			}
			compiled.Children = append(compiled.Children, cc)
		}
		return compiled, nil
	}

	rp, err := pass2ResolvePath(c.fieldPath, cs, fieldPosOf, currentSchemaUID)
	if err != nil {
		return CompiledIndexCondition{}, fmt.Errorf("index condition field: %w", err)
	}
	if len(rp) == 0 {
		return CompiledIndexCondition{}, fmt.Errorf("index condition resolved to empty path")
	}

	op, err := parseComparisonOperator(c.operator)
	if err != nil {
		return CompiledIndexCondition{}, err
	}

	return CompiledIndexCondition{
		Field:    rp[len(rp)-1], // terminal FieldDescriptor of the resolved path
		Operator: op,
		Value:    c.value,
	}, nil
}

// pass2ResolveIndexes resolves []scratchIndex to []CompiledIndex.
func pass2ResolveIndexes(
	scratch []scratchIndex,
	cs *compilerState,
	fieldPosOf map[[16]byte]fieldPos,
	currentSchemaUID [16]byte,
) ([]CompiledIndex, error) {
	result := make([]CompiledIndex, 0, len(scratch))
	for _, si := range scratch {
		ci := CompiledIndex{
			Type:   si.indexType,
			Unique: si.unique,
			Order:  parseIndexOrder(si.order),
		}
		for _, up := range si.fieldPaths {
			rp, err := pass2ResolvePath(up, cs, fieldPosOf, currentSchemaUID)
			if err != nil {
				return nil, fmt.Errorf("index %q field path %v: %w", si.name, up.parts, err)
			}
			ci.Fields = append(ci.Fields, rp)
		}
		if si.hasCondition {
			cond, err := pass2ResolveIndexCondition(si.condition, cs, fieldPosOf, currentSchemaUID)
			if err != nil {
				return nil, fmt.Errorf("index %q condition: %w", si.name, err)
			}
			ci.Condition = &cond
		}
		result = append(result, ci)
	}
	return result, nil
}

// pass2BuildMeta constructs the cold CompiledMeta from scratch.
// Fields is indexed as Fields[schemaIdx][fieldIdx] — parallel to the
// (schemaIdx, fieldIdx) identity carried by every FieldDescriptor.
func pass2BuildMeta(
	cs *compilerState,
	schemaIdxOf map[[16]byte]uint8,
	fieldPosOf map[[16]byte]fieldPos,
) *CompiledMeta {
	totalSchemas := len(cs.schemaUUIDs) + 1

	meta := &CompiledMeta{
		Fields:      make([][]FieldMeta, totalSchemas),
		Schemas:     make([]SchemaMeta, totalSchemas),
		Constraints: make([]ConstraintMeta, len(cs.constraints)),
		Indexes:     make([]IndexMeta, len(cs.indexes)),
	}

	// Pre-allocate per-schema FieldMeta slices. Size is the number of fields
	// belonging to each schema slot, determined from fieldPosOf.
	fieldCounts := make([]int, totalSchemas)
	for _, fuid := range cs.fieldUUIDs {
		if pos, ok := fieldPosOf[fuid]; ok {
			fieldCounts[pos.schemaIdx]++
		}
	}
	for i := range meta.Fields {
		meta.Fields[i] = make([]FieldMeta, fieldCounts[i])
	}

	// Root schema meta at slot 0.
	meta.Schemas[0] = SchemaMeta{
		Name:        cs.rootName,
		Description: cs.rootDescription,
		Version:     cs.rootVersion,
		Concrete:    cs.rootConcrete,
		Metadata:    cs.rootMetadata,
	}

	// Nested schema meta.
	for _, uid := range cs.schemaUUIDs {
		ss := cs.schemas[uid]
		idx := schemaIdxOf[uid]
		meta.Schemas[idx] = SchemaMeta{
			ID:          uid,
			Name:        ss.name,
			Description: ss.description,
			Concrete:    ss.concrete,
			Metadata:    ss.metadata,
		}
	}

	// Field meta indexed by [schemaIdx][fieldIdx].
	for _, fuid := range cs.fieldUUIDs {
		sf := cs.fields[fuid]
		pos, ok := fieldPosOf[fuid]
		if !ok {
			continue
		}
		meta.Fields[pos.schemaIdx][pos.fieldIdx] = FieldMeta{
			ID:          fuid,
			Name:        sf.name,
			Description: sf.description,
		}
	}

	// Constraint meta parallel to CompiledEntry.Constraints.
	for i, sc := range cs.constraints {
		meta.Constraints[i] = ConstraintMeta{
			ID:          sc.uuid,
			Name:        sc.name,
			Description: sc.description,
		}
	}

	// Index meta parallel to CompiledEntry.Indexes.
	// Order is now on CompiledIndex, not IndexMeta — omitted here.
	for i, si := range cs.indexes {
		meta.Indexes[i] = IndexMeta{
			ID:          si.uuid,
			Name:        si.name,
			Description: si.description,
		}
	}

	return meta
}

// =============================================================================
// PARSING HELPERS
// =============================================================================

func parseFieldType(s string) FieldType {
	switch s {
	case "string":
		return FieldTypeString
	case "number":
		return FieldTypeNumber
	case "integer":
		return FieldTypeInteger
	case "decimal":
		return FieldTypeDecimal
	case "boolean":
		return FieldTypeBoolean
	case "bytes":
		return FieldTypeBytes
	case "array":
		return FieldTypeArray
	case "set":
		return FieldTypeSet
	case "enum":
		return FieldTypeEnum
	case "object":
		return FieldTypeObject
	case "record":
		return FieldTypeRecord
	case "union":
		return FieldTypeUnion
	case "composite":
		return FieldTypeComposite
	case "geometry":
		return FieldTypeGeometry
	default:
		return FieldTypeUnknown
	}
}

func parseIndexType(s string) IndexType {
	switch s {
	case "unique":
		return IndexTypeUnique
	case "primary":
		return IndexTypePrimary
	case "spatial":
		return IndexTypeSpatial
	case "fulltext":
		return IndexTypeFullText
	default:
		return IndexTypeNormal
	}
}

func parseComparisonOperator(s string) (common.ComparisonOperator, error) {
	op, ok := common.ParseComparisonOperator(s)
	if !ok {
		return 0, fmt.Errorf("compile: unknown comparison operator: %q", s)
	}
	return op, nil
}

// =============================================================================
// PUBLIC ENTRY POINT
// =============================================================================

// CompileJSON compiles a schema JSON document directly to a CompiledEntry
// without constructing any intermediate definition.Schema.
//
// The compilation is a two-pass linear algorithm:
//
//	Pass 1 streams the JSON, assigns ordinals, detects cycles, and builds
//	        flat scratch structures. All UUIDs are sorted at the end of Pass 1.
//
//	Pass 2 walks the scratch structures, uses SchemaBuilder to allocate
//	        the three contiguous backing arrays at their exact sizes, populates
//	        SchemaSlot sub-slices, resolves ResolvedPaths as []FieldDescriptor,
//	        builds CompiledConstraint and CompiledIndex trees, and populates
//	        the values DataContainer keyed by fieldKey(schemaIdx, fieldIdx).
//
// The returned CompiledEntry is fully linked and ready for registration.
func CompileJSON(r io.Reader) (*CompiledEntry, error) {
	cs := newCompilerState()

	if err := pass1(r, cs); err != nil {
		return nil, fmt.Errorf("CompileJSON pass1: %w", err)
	}

	entry, err := pass2(cs)
	if err != nil {
		return nil, fmt.Errorf("CompileJSON pass2: %w", err)
	}

	return entry, nil
}

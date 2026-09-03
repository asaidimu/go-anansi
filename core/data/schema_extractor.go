package data

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	creflect "github.com/asaidimu/go-anansi/v8/core/reflect"
)

// @note #cwzt7j todo : Relocate Schema Extraction Logic
//
// The schema extraction logic in this file (e.g., SchemaFrom, ExtractDTOSchemaDirect, etc.)
// is a generic facility for deriving schemas from struct types using struct tags.
// It does not belong in the `data` package, as it is not specific to data-layer concerns.
// It should be moved to a dedicated package
// once the ongoing schema pipeline optimizations are complete.
// Until then, it remains here to avoid breaking dependent code during active development.

// FixedEpochMS is 2026-08-01T00:00:00.000Z in Unix milliseconds. Discovery
// and field ordinals are added to this to produce a monotonically
// increasing UUIDv7 timestamp component. The epoch is set after the system
// field IDs (SystemFieldIDDocumentID/SystemFieldIDMetadata) so deterministic
// DTO schema field IDs always sort after the injected system fields, keeping
// the enriched schema's system fields first.
const FixedEpochMS int64 = 1785542400000

// Thread-safe global schema cache keyed by reflect.Type, tag, and omitSystem flag.
type schemaCacheKey struct {
	t    reflect.Type
	tag  string
	omit bool
}

var schemaCache sync.Map

type cachedSchema struct {
	data []byte
	err  error
}

// SchemaFrom extracts a meta-schema JSON document for any struct type using
// the default "anansi" tag for field names (and all other metadata).
// Results are cached globally per (type, tag="", omit) for zero-allocation subsequent calls.
// If omitSystemField is true, any embedded registered system model (e.g. DocumentModel) is skipped.
func SchemaFrom[T any](omitSystemField ...bool) ([]byte, error) {
	omit := false
	if len(omitSystemField) > 0 {
		omit = omitSystemField[0]
	}
	return SchemaFromWithTag[T]("", omit)
}

// SchemaFromWithTag extracts a meta-schema JSON document for any struct type
// using a custom struct tag for field name/path resolution only.
// The custom tag is used as the primary source for field names (dot‑separated
// paths are allowed), falling back to the "anansi" tag's name if the custom
// tag is absent or empty, and finally to the snake‑cased Go field name.
// All other field metadata (required, nullable, type overrides, defaults, values)
// are taken exclusively from the "anansi" tag.
// Results are cached globally per (type, tag, omit).
func SchemaFromWithTag[T any](tag string, omitSystemField ...bool) ([]byte, error) {
	omit := false
	if len(omitSystemField) > 0 {
		omit = omitSystemField[0]
	}
	var v T
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("root DTO target cannot be nil")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	key := schemaCacheKey{t: t, tag: tag, omit: omit}
	if cached, ok := schemaCache.Load(key); ok {
		res := cached.(cachedSchema)
		return res.data, res.err
	}

	data, err := ExtractDTOSchemaDirectWithTag(v, tag, omit)
	schemaCache.Store(key, cachedSchema{data: data, err: err})
	return data, err
}

// ExtractDTOSchemaDirect streams JSON bytes directly into a buffer without
// intermediate maps or struct serialization. Uses the default "anansi" tag.
// omitSystemField controls whether embedded registered system models (e.g. DocumentModel) are skipped.
//
// Deprecated: Use SchemaFrom instead.
func ExtractDTOSchemaDirect(target any, omitSystemField ...bool) ([]byte, error) {
	omit := false
	if len(omitSystemField) > 0 {
		omit = omitSystemField[0]
	}
	return ExtractDTOSchemaDirectWithTag(target, "", omit)
}

// ExtractDTOSchemaDirectWithTag does the same as ExtractDTOSchemaDirect but
// uses the given custom tag for name/path resolution (see SchemaFromWithTag).
//
// Deprecated: Use SchemaFromWithTag instead.
func ExtractDTOSchemaDirectWithTag(target any, tag string, omitSystemField ...bool) ([]byte, error) {
	if target == nil {
		return nil, fmt.Errorf("root DTO target cannot be nil")
	}
	omit := false
	if len(omitSystemField) > 0 {
		omit = omitSystemField[0]
	}
	e := newDirectExtractor(tag, omit)
	return e.Extract(target)
}

type schemaRegistryEntry struct {
	ID      string
	Ordinal int64
}

type syntheticSchema struct {
	name          string
	schemaID      string
	parentFieldID string
	ordinal       int64
	buf           bytes.Buffer
	fieldCount    int
	isRequired    bool
	childSynOrder []string
}

type directExtractor struct {
	discoveryOrdinal int64
	registry         map[string]*schemaRegistryEntry
	schemas          map[string][]byte
	schemaOrder      []string
	enumValues       map[string][]string
	syntheticSchemas map[string]map[string]*syntheticSchema
	rootSchemaID     string
	rootReferenced   bool
	customTag        string
	omitSystem       bool // if true, skip embedded registered system models (e.g. DocumentModel)
}

func newDirectExtractor(customTag string, omitSystem bool) *directExtractor {
	return &directExtractor{
		discoveryOrdinal: 0,
		registry:         make(map[string]*schemaRegistryEntry),
		schemas:          make(map[string][]byte),
		schemaOrder:      make([]string, 0, 8),
		enumValues:       make(map[string][]string),
		syntheticSchemas: make(map[string]map[string]*syntheticSchema),
		customTag:        customTag,
		omitSystem:       omitSystem,
	}
}

func (e *directExtractor) Extract(target any) ([]byte, error) {
	t := reflect.TypeOf(target)
	if t == nil {
		return nil, fmt.Errorf("root DTO target cannot be nil")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("root DTO target must be a struct, got %v", t.Kind())
	}

	rootTypeKey := getFullyQualifiedName(t)
	rootSchemaID := e.registerType(rootTypeKey)
	e.rootSchemaID = rootSchemaID

	var fieldsBuf bytes.Buffer
	fieldOrdinal := int64(0)
	fieldCount := 0
	if err := e.extractStructFields(t, rootSchemaID, &fieldsBuf, &fieldOrdinal, &fieldCount, nil); err != nil {
		return nil, err
	}

	for _, synMap := range e.syntheticSchemas {
		for _, syn := range synMap {
			var sBuf bytes.Buffer
			sBuf.WriteString("{\n      \"name\": ")
			writeJSONString(&sBuf, syn.name)

			childMap := e.syntheticSchemas[syn.schemaID]
			totalFields := syn.fieldCount + len(childMap)

			if totalFields > 0 {
				sBuf.WriteString(",\n      \"fields\": {\n")
				sBuf.Write(syn.buf.Bytes())

				if len(childMap) > 0 {
					first := syn.fieldCount == 0
					for _, childHead := range syn.childSynOrder {
						childSyn := childMap[childHead]
						if !first {
							sBuf.WriteString(",\n")
						}
						first = false
						sBuf.WriteString("    ")
						writeJSONString(&sBuf, childSyn.parentFieldID)
						sBuf.WriteString(": {\n      \"name\": ")
						writeJSONString(&sBuf, childSyn.name)
						if childSyn.isRequired {
							sBuf.WriteString(",\n      \"required\": true")
						}
						sBuf.WriteString(",\n      \"type\": \"object\",\n      \"schema\": {\"id\": ")
						writeJSONString(&sBuf, childSyn.schemaID)
						sBuf.WriteString("}\n    }")
					}
				}
				sBuf.WriteString("\n      }")
			}
			sBuf.WriteString("\n    }")
			e.schemas[syn.schemaID] = sBuf.Bytes()
		}
	}

	if e.rootReferenced {
		var rootMirror bytes.Buffer
		rootMirror.WriteString("{\n      \"name\": ")
		writeJSONString(&rootMirror, t.Name())
		if fieldCount > 0 {
			rootMirror.WriteString(",\n      \"fields\": {\n")
			rootMirror.Write(fieldsBuf.Bytes())
			rootMirror.WriteString("\n      }")
		}
		rootMirror.WriteString("\n    }")
		e.schemas[e.rootSchemaID] = rootMirror.Bytes()
	}

	var out bytes.Buffer
	out.WriteString("{\n  \"version\": \"1.0.0\",\n  \"name\": ")
	writeJSONString(&out, t.Name())
	if fieldCount > 0 {
		out.WriteString(",\n  \"fields\": {\n")
		out.Write(fieldsBuf.Bytes())
		out.WriteString("\n  }")
	}
	if len(e.schemaOrder) > 0 {
		out.WriteString(",\n  \"schemas\": {\n")
		for i, id := range e.schemaOrder {
			if i > 0 {
				out.WriteString(",\n")
			}
			out.WriteString("    ")
			writeJSONString(&out, id)
			out.WriteString(": ")
			out.Write(e.schemas[id])
		}
		out.WriteString("\n  }")
	}
	out.WriteString("\n}")
	return out.Bytes(), nil
}

func (e *directExtractor) registerType(typeKey string) string {
	if entry, ok := e.registry[typeKey]; ok {
		return entry.ID
	}
	ord := e.discoveryOrdinal
	e.discoveryOrdinal++
	id := generateDeterministicUUIDv7(ord, typeKey)
	e.registry[typeKey] = &schemaRegistryEntry{ID: id, Ordinal: ord}
	return id
}

// extractStructFields extracts all fields from a struct type, recursively flattening
// anonymous structs and handling dotted‑path synthetic schemas. It uses creflect
// to obtain tag metadata in O(1) time per field, with zero allocations for the
// tag value splitting.
func (e *directExtractor) extractStructFields(
	t reflect.Type,
	owningSchemaID string,
	buf *bytes.Buffer,
	fieldOrdinal *int64,
	fieldCount *int,
	skipNames map[string]bool,
) error {
	var localSynOrder []string

	// shadowNames collects the resolved field names declared directly on t
	// (non‑anonymous fields). When a system‑model embed (e.g. DocumentModel)
	// is flattened below, its own _id_/_metadata_ fields are skipped in favor
	// of an outer field that shadows them, so the extracted schema never
	// contains duplicate system fields.
	shadowNames := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			continue
		}
		// Get Anansi tag for this field to check skip and name.
		anansiTag, _ := creflect.FieldTagOf(t, f.Name, AnansiTag)
		parsed := parseSchemaTag(anansiTag)
		if parsed.Skip {
			continue
		}
		// Resolve name (respecting custom tag).
		var customTag creflect.Tag
		if e.customTag != "" {
			customTag, _ = creflect.FieldTagOf(t, f.Name, e.customTag)
		}
		name := resolveFieldName(f, customTag, parsed)
		if name == DocumentIDField || name == MetadataField {
			shadowNames[name] = true
		}
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// ---- 1. Get Anansi tag via creflect (O(1) lookup) ----
		anansiCRefTag, _ := creflect.FieldTagOf(t, field.Name, AnansiTag)
		anansiParsed := parseSchemaTag(anansiCRefTag)
		if anansiParsed.Skip {
			continue
		}

		// ---- 2. Omit system models if requested ----
		if field.Anonymous && e.omitSystem && IsSystemModelType(field.Type) {
			continue // skip the entire system‑model embed
		}

		// ---- 3. Flatten anonymous structs (unless type‑override says otherwise) ----
		if field.Anonymous && field.Type.Kind() == reflect.Struct && anansiParsed.TypeOverride == "" {
			// For system‑model embeds, pass the shadowNames so that _id_/_metadata_ are
			// omitted if they are shadowed by outer fields.
			var skip map[string]bool
			if IsSystemModelType(field.Type) {
				skip = shadowNames
			}
			if err := e.extractStructFields(field.Type, owningSchemaID, buf, fieldOrdinal, fieldCount, skip); err != nil {
				return err
			}
			continue
		}

		// ---- 4. Resolve field name (custom tag first, then Anansi, then snake_case) ----
		var customCRefTag creflect.Tag
		if e.customTag != "" {
			customCRefTag, _ = creflect.FieldTagOf(t, field.Name, e.customTag)
		}
		fieldName := resolveFieldName(field, customCRefTag, anansiParsed)
		if skipNames != nil && skipNames[fieldName] {
			continue
		}

		// ---- 5. Handle dotted‑path fields (synthetic schemas) ----
		if strings.Contains(fieldName, ".") {
			if _, err := e.writePathField(owningSchemaID, fieldOrdinal, &localSynOrder, field, fieldName, anansiParsed); err != nil {
				return err
			}
			continue
		}

		// ---- 6. Standard field (or composite/union) ----
		if anansiParsed.Default != nil && (anansiParsed.TypeOverride == "composite" || anansiParsed.TypeOverride == "union") {
			return fmt.Errorf("field %q: default values are not supported for %s fields", fieldName, anansiParsed.TypeOverride)
		}

		(*fieldOrdinal)++
		fieldID := generateDeterministicUUIDv7(*fieldOrdinal, owningSchemaID+fieldName)
		if *fieldCount > 0 {
			buf.WriteString(",\n")
		}
		*fieldCount++
		buf.WriteString("    ")
		writeJSONString(buf, fieldID)
		buf.WriteString(": ")

		var err error
		switch anansiParsed.TypeOverride {
		case "composite":
			err = e.writeCompositeField(buf, field, fieldName, anansiParsed)
		case "union":
			err = e.writeUnionField(buf, field, fieldName, anansiParsed)
		default:
			err = e.writeStandardField(buf, field, fieldName, anansiParsed)
		}
		if err != nil {
			return err
		}
	}

	// ---- 7. Append synthetic schemas (dotted paths) to the current owning schema ----
	if synMap, ok := e.syntheticSchemas[owningSchemaID]; ok {
		for _, head := range localSynOrder {
			syn := synMap[head]
			if *fieldCount > 0 {
				buf.WriteString(",\n")
			}
			*fieldCount++
			buf.WriteString("    ")
			writeJSONString(buf, syn.parentFieldID)
			buf.WriteString(": {\n      \"name\": ")
			writeJSONString(buf, syn.name)
			if syn.isRequired {
				buf.WriteString(",\n      \"required\": true")
			}
			buf.WriteString(",\n      \"type\": \"object\",\n      \"schema\": {\"id\": ")
			writeJSONString(buf, syn.schemaID)
			buf.WriteString("}\n    }")
		}
	}

	return nil
}

// resolveFieldName resolves a struct field's schema name through the same
// chain used by extraction: custom tag first, then the anansi tag name, then
// the snake-cased Go field name.
func resolveFieldName(
	field reflect.StructField,
	customTag creflect.Tag,
	anansiParsed parsedSchemaTag,
) string {
	// 1. Custom tag takes precedence.
	if !customTag.IsZero() {
		for part := range customTag.ValuesIter() {
			part = strings.TrimSpace(part)
			if part != "" && part != "-" {
				// first token is the name (e.g. "fieldName" from `json:"fieldName,omitempty"`)
				return part
			}
			break // only care about the first token
		}
	}

	// 2. Fall back to the explicit anansi name.
	if anansiParsed.HasName {
		return anansiParsed.Name
	}

	// 3. Default to snake-cased Go field name.
	return toSnakeCase(field.Name)
}

func (e *directExtractor) writePathField(
	owningSchemaID string,
	fieldOrdinal *int64,
	localSynOrder *[]string,
	field reflect.StructField,
	fieldName string,
	tag parsedSchemaTag,
) (bool, error) {
	dotIdx := strings.IndexByte(fieldName, '.')
	head := fieldName[:dotIdx]
	tail := fieldName[dotIdx+1:]

	if e.syntheticSchemas[owningSchemaID] == nil {
		e.syntheticSchemas[owningSchemaID] = make(map[string]*syntheticSchema)
	}
	syn, exists := e.syntheticSchemas[owningSchemaID][head]
	if !exists {
		schemaID := e.registerType(owningSchemaID + "." + head)
		e.schemaOrder = append(e.schemaOrder, schemaID)

		(*fieldOrdinal)++
		parentFieldID := generateDeterministicUUIDv7(*fieldOrdinal, owningSchemaID+head)

		syn = &syntheticSchema{
			name:          head,
			schemaID:      schemaID,
			parentFieldID: parentFieldID,
		}
		e.syntheticSchemas[owningSchemaID][head] = syn
		*localSynOrder = append(*localSynOrder, head)
	}

	var childRequired bool
	var err error

	if strings.Contains(tail, ".") {
		childRequired, err = e.writePathField(syn.schemaID, &syn.ordinal, &syn.childSynOrder, field, tail, tag)
	} else {
		syn.ordinal++
		fieldID := generateDeterministicUUIDv7(syn.ordinal, syn.schemaID+tail)
		if syn.fieldCount > 0 {
			syn.buf.WriteString(",\n")
		}
		syn.fieldCount++
		syn.buf.WriteString("    ")
		writeJSONString(&syn.buf, fieldID)
		syn.buf.WriteString(": ")

		switch tag.TypeOverride {
		case "composite":
			err = e.writeCompositeField(&syn.buf, field, tail, tag)
		case "union":
			err = e.writeUnionField(&syn.buf, field, tail, tag)
		default:
			err = e.writeStandardField(&syn.buf, field, tail, tag)
		}

		childRequired = tag.Required != nil && *tag.Required
	}

	if err != nil {
		return false, err
	}

	if childRequired {
		syn.isRequired = true
	}

	return syn.isRequired, nil
}

func (e *directExtractor) writeStandardField(buf *bytes.Buffer, field reflect.StructField, fieldName string, tag parsedSchemaTag) error {
	fieldType := field.Type
	isPointer := fieldType.Kind() == reflect.Pointer
	if isPointer {
		fieldType = fieldType.Elem()
	}

	required := tag.Required != nil && *tag.Required
	nullable := isPointer
	if tag.Nullable != nil {
		nullable = *tag.Nullable
	}

	buf.WriteString("{\n      \"name\": ")
	writeJSONString(buf, fieldName)
	if required {
		buf.WriteString(",\n      \"required\": true")
	}
	if nullable {
		buf.WriteString(",\n      \"nullable\": true")
	}

	if tag.TypeOverride == "enum" {
		scalarType, refJSON, inlineValues, err := e.resolveEnumSchema(fieldType, tag, field.Name)
		if err != nil {
			return err
		}
		buf.WriteString(",\n      \"type\": \"enum\"")
		buf.WriteString(",\n      \"schema\": ")
		if refJSON != "" {
			buf.WriteString(refJSON)
		} else {
			buf.WriteString("{\n        \"type\": ")
			writeJSONString(buf, scalarType)
			buf.WriteString(",\n        \"values\": [")
			for i, v := range inlineValues {
				if i > 0 {
					buf.WriteString(", ")
				}
				writeJSONString(buf, v)
			}
			buf.WriteString("]\n      }")
		}
		if tag.Default != nil {
			allowedValues := inlineValues
			if refJSON != "" {
				allowedValues = e.enumValues[getFullyQualifiedName(fieldType)]
			}
			if !containsString(allowedValues, *tag.Default) {
				return fmt.Errorf("field %q: default value %q is not one of the declared enum values %v", field.Name, *tag.Default, allowedValues)
			}
			buf.WriteString(",\n      \"default\": ")
			if err := coerceDefaultValue(buf, *tag.Default, scalarType, field.Name); err != nil {
				return err
			}
		}
		buf.WriteString("\n    }")
		return nil
	}

	schemaType, refJSON, err := e.inferSchemaType(fieldType, field.Name, tag)
	if err != nil {
		return err
	}
	buf.WriteString(",\n      \"type\": ")
	writeJSONString(buf, schemaType)
	if refJSON != "" {
		buf.WriteString(",\n      \"schema\": ")
		buf.WriteString(refJSON)
	}
	if tag.Default != nil {
		buf.WriteString(",\n      \"default\": ")
		if err := coerceDefaultValue(buf, *tag.Default, schemaType, field.Name); err != nil {
			return err
		}
	}
	buf.WriteString("\n    }")
	return nil
}

func (e *directExtractor) resolveEnumSchema(fieldType reflect.Type, tag parsedSchemaTag, goFieldName string) (scalarType string, refJSON string, inlineValues []string, err error) {
	scalarType = primitiveKindToSchemaType(fieldType.Kind())
	if scalarType != "string" && scalarType != "integer" && scalarType != "number" {
		return "", "", nil, fmt.Errorf("field %q: enum requires a string, integer, or number underlying type, got %v", goFieldName, fieldType.Kind())
	}

	isNamedType := fieldType.PkgPath() != "" && fieldType.Name() != ""
	if !isNamedType {
		if len(tag.Values) == 0 {
			return "", "", nil, fmt.Errorf("field %q: enum requires values= (underlying type has no shared identity to inherit values from)", goFieldName)
		}
		return scalarType, "", tag.Values, nil
	}

	typeKey := getFullyQualifiedName(fieldType)
	if entry, ok := e.registry[typeKey]; ok {
		return scalarType, fmt.Sprintf("{\"id\": %q}", entry.ID), nil, nil
	}
	if len(tag.Values) == 0 {
		return "", "", nil, fmt.Errorf("field %q: first use of named enum type %s must declare values=", goFieldName, typeKey)
	}

	id := e.registerType(typeKey)
	e.schemaOrder = append(e.schemaOrder, id)
	e.enumValues[typeKey] = tag.Values

	var sBuf bytes.Buffer
	sBuf.WriteString("{\n      \"name\": ")
	writeJSONString(&sBuf, fieldType.Name())
	sBuf.WriteString(",\n      \"type\": ")
	writeJSONString(&sBuf, scalarType)
	sBuf.WriteString(",\n      \"values\": [")
	for i, v := range tag.Values {
		if i > 0 {
			sBuf.WriteString(", ")
		}
		writeJSONString(&sBuf, v)
	}
	sBuf.WriteString("]\n    }")
	e.schemas[id] = sBuf.Bytes()

	return scalarType, fmt.Sprintf("{\"id\": %q}", id), nil, nil
}

func (e *directExtractor) writeCompositeField(buf *bytes.Buffer, field reflect.StructField, fieldName string, tag parsedSchemaTag) error {
	ft := field.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct {
		return fmt.Errorf("composite field %q must be a struct, got %v", field.Name, ft.Kind())
	}
	if ft.NumField() < 2 {
		return fmt.Errorf("composite container %s must embed at least 2 structs, got %d", ft.Name(), ft.NumField())
	}
	buf.WriteString("{\n      \"name\": ")
	writeJSONString(buf, fieldName)
	buf.WriteString(",\n      \"type\": \"composite\"")
	if tag.Required != nil && *tag.Required {
		buf.WriteString(",\n      \"required\": true")
	}
	buf.WriteString(",\n      \"schema\": [")
	for i := 0; i < ft.NumField(); i++ {
		sf := ft.Field(i)
		if !sf.Anonymous {
			return fmt.Errorf("composite container %s contains non-embedded field %q; composite containers may only embed structs", ft.Name(), sf.Name)
		}
		if sf.Type.Kind() == reflect.Pointer {
			return fmt.Errorf("composite container %s embeds pointer field %q; composite members must be embedded by value, not by pointer", ft.Name(), sf.Name)
		}
		if sf.Type.Kind() != reflect.Struct {
			return fmt.Errorf("composite container %s embeds non-struct field %q of kind %v", ft.Name(), sf.Name, sf.Type.Kind())
		}
		subID, err := e.ensureStructRegistered(sf.Type, sf.Name)
		if err != nil {
			return err
		}
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString("{\"id\": ")
		writeJSONString(buf, subID)
		buf.WriteString("}")
	}
	buf.WriteString("]\n    }")
	return nil
}

func (e *directExtractor) writeUnionField(buf *bytes.Buffer, field reflect.StructField, fieldName string, tag parsedSchemaTag) error {
	ft := field.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct {
		return fmt.Errorf("union field %q must be a struct, got %v", field.Name, ft.Kind())
	}
	if ft.NumField() < 2 {
		return fmt.Errorf("union container %s must have at least 2 variants, got %d", ft.Name(), ft.NumField())
	}
	buf.WriteString("{\n      \"name\": ")
	writeJSONString(buf, fieldName)
	buf.WriteString(",\n      \"type\": \"union\"")
	if tag.Required != nil && *tag.Required {
		buf.WriteString(",\n      \"required\": true")
	}
	buf.WriteString(",\n      \"schema\": [")
	for i := 0; i < ft.NumField(); i++ {
		sf := ft.Field(i)
		if sf.Type.Kind() != reflect.Pointer {
			return fmt.Errorf("union variant field %q in %s must be a pointer, got %v", sf.Name, ft.Name(), sf.Type.Kind())
		}
		pointee := sf.Type.Elem()
		var refID string
		var err error
		if pointee.Kind() == reflect.Struct {
			refID, err = e.ensureStructRegistered(pointee, sf.Name)
			if err != nil {
				return err
			}
		} else {
			refID = e.ensurePrimitiveUnionSchemaRegistered(pointee)
		}
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString("{\"id\": ")
		writeJSONString(buf, refID)
		buf.WriteString("}")
	}
	buf.WriteString("]\n    }")
	return nil
}

func (e *directExtractor) ensurePrimitiveUnionSchemaRegistered(t reflect.Type) string {
	bareName := t.Kind().String()
	typeKey := "primitive_" + bareName
	if entry, ok := e.registry[typeKey]; ok {
		return entry.ID
	}
	id := e.registerType(typeKey)
	e.schemaOrder = append(e.schemaOrder, id)
	schemaType := primitiveKindToSchemaType(t.Kind())
	var sBuf bytes.Buffer
	sBuf.WriteString("{\n      \"name\": ")
	writeJSONString(&sBuf, bareName+"_type")
	sBuf.WriteString(",\n      \"type\": ")
	writeJSONString(&sBuf, schemaType)
	sBuf.WriteString("\n    }")
	e.schemas[id] = sBuf.Bytes()
	return id
}

func (e *directExtractor) ensureStructRegistered(t reflect.Type, fieldName string) (string, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	typeKey := getFullyQualifiedName(t)
	if typeKey == "" {
		typeKey = "Anon_" + fieldName
	}
	if entry, ok := e.registry[typeKey]; ok {
		if entry.ID == e.rootSchemaID && !e.rootReferenced {
			e.rootReferenced = true
			e.schemaOrder = append(e.schemaOrder, e.rootSchemaID)
		}
		return entry.ID, nil
	}
	id := e.registerType(typeKey)
	e.schemaOrder = append(e.schemaOrder, id)
	schemaName := t.Name()
	if schemaName == "" {
		schemaName = typeKey
	}
	e.schemas[id] = nil
	var fieldsBuf bytes.Buffer
	fieldOrdinal := int64(0)
	fieldCount := 0
	if err := e.extractStructFields(t, id, &fieldsBuf, &fieldOrdinal, &fieldCount, nil); err != nil {
		return "", err
	}
	var sBuf bytes.Buffer
	sBuf.WriteString("{\n      \"name\": ")
	writeJSONString(&sBuf, schemaName)
	if fieldCount > 0 {
		sBuf.WriteString(",\n      \"fields\": {\n")
		sBuf.Write(fieldsBuf.Bytes())
		sBuf.WriteString("\n      }")
	}
	sBuf.WriteString("\n    }")
	e.schemas[id] = sBuf.Bytes()
	return id, nil
}

func (e *directExtractor) inferSchemaType(t reflect.Type, fieldName string, tag parsedSchemaTag) (string, string, error) {
	if tag.TypeOverride != "" {
		switch tag.TypeOverride {
		case "decimal", "geometry", "bytes", "unknown":
			return tag.TypeOverride, "", nil
		default:
			return "", "", fmt.Errorf("field %q: unsupported type override %q", fieldName, tag.TypeOverride)
		}
	}
	switch t.Kind() {
	case reflect.String:
		return "string", "", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer", "", nil
	case reflect.Float32, reflect.Float64:
		return "number", "", nil
	case reflect.Bool:
		return "boolean", "", nil
	case reflect.Interface:
		return "unknown", "", nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes", "", nil
		}
		if t.Elem().Kind() == reflect.Slice {
			innerElem := t.Elem().Elem().Kind()
			if innerElem == reflect.Float64 || innerElem == reflect.Float32 {
				return "geometry", "", nil
			}
		}
		elemType := t.Elem()
		if elemType.Kind() == reflect.Pointer {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Struct {
			subID, err := e.ensureStructRegistered(elemType, fieldName)
			if err != nil {
				return "", "", err
			}
			return "array", fmt.Sprintf("{\"id\": %q}", subID), nil
		}
		elemSchemaType := primitiveKindToSchemaType(elemType.Kind())
		return "array", fmt.Sprintf("{\"type\": %q}", elemSchemaType), nil
	case reflect.Map:
		if t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.Interface {
			return "record", "", nil
		}
		valType := t.Elem()
		if valType.Kind() == reflect.Pointer {
			valType = valType.Elem()
		}
		if valType.Kind() == reflect.Struct {
			subID, err := e.ensureStructRegistered(valType, fieldName)
			if err != nil {
				return "", "", err
			}
			return "record", fmt.Sprintf("{\"id\": %q}", subID), nil
		}
		valSchemaType := primitiveKindToSchemaType(valType.Kind())
		return "record", fmt.Sprintf("{\"type\": %q}", valSchemaType), nil
	case reflect.Struct:
		subID, err := e.ensureStructRegistered(t, fieldName)
		if err != nil {
			return "", "", err
		}
		return "object", fmt.Sprintf("{\"id\": %q}", subID), nil
	}
	return "unknown", "", nil
}

// Low-level fast JSON formatting helper

const hexDigits = "0123456789abcdef"

func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf.WriteString("\\\"")
		case '\\':
			buf.WriteString("\\\\")
		case '\n':
			buf.WriteString("\\n")
		case '\r':
			buf.WriteString("\\r")
		case '\t':
			buf.WriteString("\\t")
		case '\b':
			buf.WriteString("\\b")
		case '\f':
			buf.WriteString("\\f")
		default:
			if c < 0x20 {
				buf.WriteString("\\u00")
				buf.WriteByte(hexDigits[c>>4])
				buf.WriteByte(hexDigits[c&0xF])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

func coerceDefaultValue(buf *bytes.Buffer, valStr, schemaType, fieldName string) error {
	switch schemaType {
	case "integer":
		if _, err := strconv.ParseInt(valStr, 10, 64); err != nil {
			return fmt.Errorf("field %q: default value %q is not a valid integer: %w", fieldName, valStr, err)
		}
		buf.WriteString(valStr)
	case "number":
		if _, err := strconv.ParseFloat(valStr, 64); err != nil {
			return fmt.Errorf("field %q: default value %q is not a valid number: %w", fieldName, valStr, err)
		}
		buf.WriteString(valStr)
	case "boolean":
		b, err := strconv.ParseBool(valStr)
		if err != nil {
			return fmt.Errorf("field %q: default value %q is not a valid boolean: %w", fieldName, valStr, err)
		}
		if b {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case "string", "decimal":
		writeJSONString(buf, valStr)
	default:
		return fmt.Errorf("field %q: default values are not supported for type %q", fieldName, schemaType)
	}
	return nil
}

func containsString(list []string, target string) bool {
	return slices.Contains(list, target)
}

func primitiveKindToSchemaType(k reflect.Kind) string {
	switch k {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	default:
		return "unknown"
	}
}

func getFullyQualifiedName(t reflect.Type) string {
	if t.PkgPath() == "" {
		return t.Name()
	}
	return t.PkgPath() + "." + t.Name()
}

func generateDeterministicUUIDv7(ordinal int64, seedString string) string {
	ms := FixedEpochMS + ordinal
	hash := sha256.Sum256([]byte(seedString))
	var buf [16]byte
	buf[0] = byte(ms >> 40)
	buf[1] = byte(ms >> 32)
	buf[2] = byte(ms >> 24)
	buf[3] = byte(ms >> 16)
	buf[4] = byte(ms >> 8)
	buf[5] = byte(ms)
	buf[6] = 0x70 | (hash[0] & 0x0F)
	buf[7] = hash[1]
	buf[8] = 0x80 | (hash[2] & 0x3F)
	copy(buf[9:], hash[3:10])
	return uuid.Must(uuid.FromBytes(buf[:])).String()
}

type parsedSchemaTag struct {
	Name         string
	HasName      bool
	Skip         bool
	Required     *bool
	Nullable     *bool
	Default      *string
	TypeOverride string
	Values       []string
}

func parseSchemaTag(tag creflect.Tag) parsedSchemaTag {
	var parsed parsedSchemaTag
	if tag.IsZero() {
		return parsed
	}

	for part := range tag.ValuesIter() {
		part = strings.TrimSpace(part) // safe, slab-backed string header – no new allocation

		if part == "-" {
			parsed.Skip = true
			return parsed
		}

		// First non-empty, non-KV part is the name.
		if !parsed.HasName && !strings.Contains(part, "=") {
			parsed.Name = part
			parsed.HasName = true
			continue
		}

		idx := strings.IndexByte(part, '=')
		if idx == -1 {
			continue // malformed bare flag, ignore
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])

		switch key {
		case "required":
			b := val == "true"
			parsed.Required = &b
		case "nullable":
			b := val == "true"
			parsed.Nullable = &b
		case "default":
			v := val
			parsed.Default = &v
		case "type":
			parsed.TypeOverride = val
		case "values":
			if val != "" {
				parsed.Values = strings.Split(val, "|") // still allocates, but unavoidable if used
			}
		}
	}
	return parsed
}


// Zero-allocation byte scanner for snake_case field name conversion.
func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i > 0 && c >= 'A' && c <= 'Z' {
			prev := s[i-1]
			nextIsLower := i+1 < len(s) && (s[i+1] >= 'a' && s[i+1] <= 'z')
			if (prev >= 'a' && prev <= 'z') || nextIsLower {
				b.WriteByte('_')
			}
		}
		if c >= 'A' && c <= 'Z' {
			b.WriteByte(c + ('a' - 'A'))
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

package data

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// FixedEpochMS is 2026-01-01T00:00:00.000Z in Unix milliseconds. Discovery
// and field ordinals are added to this to produce a monotonically
// increasing UUIDv7 timestamp component, which is what lets consumers
// recover Go declaration order by sorting map keys as strings (see spec.md
// §4). ~285,000 years of ordinal headroom at 1ms resolution.
const FixedEpochMS int64 = 1767225600000

// SchemaFrom extracts a meta-schema JSON document for any struct type.
// The returned JSON describes the struct's full field shape (names, types,
// required/nullable, enums, composites, unions) as declared by anansi tags.
// If the struct embeds DocumentModel, its _id_ and _metadata_ fields are
// included in the output automatically via the standard embedding promotion.
//
// The output is a self-contained contract schema suitable for validation and
// serialization contracts. It is NOT designed to be enriched by the schema
// registry — persistence schemas backing collections should be authored or
// imported independently rather than derived from Go type tags.
func SchemaFrom[T any]() ([]byte, error) {
	var v T
	return ExtractDTOSchemaDirect(v)
}

// ExtractDTOSchemaDirect streams JSON bytes directly into a buffer without
// intermediate maps or struct serialization.
func ExtractDTOSchemaDirect(target any) ([]byte, error) {
	e := newDirectExtractor()
	return e.Extract(target)
}

type schemaRegistryEntry struct {
	ID      string
	Ordinal int64
}

// directExtractor performs a single, single-threaded extraction run. It is
// not safe for concurrent use — each call to Extract creates its own
// extractor, so this is not a concern in practice (concurrent Schema()
// calls for the same type are serialized by schemaComputation.once instead;
// concurrent calls for different types get independent extractors and share
// nothing).
type directExtractor struct {
	discoveryOrdinal int64
	registry         map[string]*schemaRegistryEntry
	schemas          map[string][]byte // Schema ID -> pre-rendered JSON bytes
	schemaOrder      []string          // Schema IDs in discovery order, for deterministic output
	enumValues       map[string][]string
}

func newDirectExtractor() *directExtractor {
	return &directExtractor{
		discoveryOrdinal: 0,
		registry:         make(map[string]*schemaRegistryEntry),
		schemas:          make(map[string][]byte),
		schemaOrder:      make([]string, 0, 8),
		enumValues:       make(map[string][]string),
	}
}

func (e *directExtractor) Extract(target any) ([]byte, error) {
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("root DTO target must be a struct, got %v", t.Kind())
	}

	rootTypeKey := getFullyQualifiedName(t)
	rootSchemaID := e.registerType(rootTypeKey)
	e.schemaOrder = append(e.schemaOrder, rootSchemaID)

	var fieldsBuf bytes.Buffer
	fieldOrdinal := int64(0)
	fieldCount := 0
	if err := e.extractStructFields(t, rootSchemaID, &fieldsBuf, &fieldOrdinal, &fieldCount); err != nil {
		return nil, err
	}

	// Mirror the root schema into the schemas registry too. The root's ID
	// is otherwise only reachable via the top-level document (its "fields"
	// live there, not under "schemas"). If any field anywhere in the type
	// graph references the root type itself — directly or through a cycle
	// — that reference resolves to rootSchemaID, which would otherwise be a
	// dangling ID with no entry in "schemas" to back it. Mirroring costs a
	// small duplication (the root's fields appear twice: once at the top
	// level, once under schemas[rootSchemaID]) but guarantees every "id"
	// reference in the document resolves to something.
	var rootMirror bytes.Buffer
	rootMirror.WriteString("{\n      \"name\": ")
	writeJSONString(&rootMirror, t.Name())
	if fieldCount > 0 {
		rootMirror.WriteString(",\n      \"fields\": {\n")
		rootMirror.Write(fieldsBuf.Bytes())
		rootMirror.WriteString("\n      }")
	}
	rootMirror.WriteString("\n    }")
	e.schemas[rootSchemaID] = rootMirror.Bytes()

	// Stream the root MetaSchema directly to the final output buffer.
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
	id := GenerateDeterministicUUIDv7(ord, typeKey)
	e.registry[typeKey] = &schemaRegistryEntry{ID: id, Ordinal: ord}
	return id
}

func (e *directExtractor) extractStructFields(t reflect.Type, owningSchemaID string, buf *bytes.Buffer, fieldOrdinal *int64, fieldCount *int) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := parseSchemaTag(field.Tag.Get(AnansiTag))
		if tag.Skip {
			continue
		}
		// Pattern A: Default Embeddings (Promote inline)
		if field.Anonymous && field.Type.Kind() == reflect.Struct && tag.TypeOverride == "" {
			if err := e.extractStructFields(field.Type, owningSchemaID, buf, fieldOrdinal, fieldCount); err != nil {
				return err
			}
			continue
		}
		fieldName := field.Name
		if tag.HasName {
			fieldName = tag.Name
		} else {
			fieldName = toSnakeCase(fieldName)
		}
		if tag.Default != nil && (tag.TypeOverride == "composite" || tag.TypeOverride == "union") {
			return fmt.Errorf("field %q: default values are not supported for %s fields", fieldName, tag.TypeOverride)
		}
		(*fieldOrdinal)++
		fieldID := GenerateDeterministicUUIDv7(*fieldOrdinal, owningSchemaID+fieldName)
		if *fieldCount > 0 {
			buf.WriteString(",\n")
		}
		*fieldCount++
		buf.WriteString("    ")
		writeJSONString(buf, fieldID)
		buf.WriteString(": ")
		var err error
		switch tag.TypeOverride {
		case "composite":
			err = e.writeCompositeField(buf, field, fieldName)
		case "union":
			err = e.writeUnionField(buf, field, fieldName)
		default:
			err = e.writeStandardField(buf, field, fieldName, tag)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *directExtractor) writeStandardField(buf *bytes.Buffer, field reflect.StructField, fieldName string, tag parsedSchemaTag) error {
	fieldType := field.Type
	isPointer := fieldType.Kind() == reflect.Pointer
	if isPointer {
		fieldType = fieldType.Elem()
	}
	required := !isPointer
	if tag.Required != nil {
		required = *tag.Required
	}
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

	// Pattern D: Enums (inline or named/shared) — spec.md §7 Pattern D.
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
				// Named/shared enum: validate against the values declared
				// at the type's first registration, not this occurrence's
				// (possibly absent or differing) tag.
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

// resolveEnumSchema implements spec.md §7 Pattern D. Two modes:
//
//   - Inline: fieldType is a bare builtin (e.g. plain `string`, `int`) with
//     no distinct package-qualified identity. tag.Values is rendered
//     directly as an inline descriptor; nothing is registered.
//   - Named/shared: fieldType is a distinct defined Go type (e.g.
//     `type StatusEnum string`, PkgPath != ""). The dedup key is the Go
//     type itself. The first field encountered with this type and
//     type=enum registers a shared schema using *that* field's values=;
//     every subsequent field of the same named type reuses the same
//     schema ID regardless of what (if anything) its own values= says.
//
// Returns (scalarType, refJSON, inlineValues, err). Exactly one of refJSON
// or inlineValues is populated on success.
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

func (e *directExtractor) writeCompositeField(buf *bytes.Buffer, field reflect.StructField, fieldName string) error {
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
	buf.WriteString(",\n      \"required\": true")
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

func (e *directExtractor) writeUnionField(buf *bytes.Buffer, field reflect.StructField, fieldName string) error {
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
	buf.WriteString(",\n      \"required\": true")
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

// ensurePrimitiveUnionSchemaRegistered implements spec.md §3.1: a primitive
// scalar used as a union variant pointee gets a shared, globally-deduplicated
// Type-mode schema keyed by its bare Go kind name, since Form 2
// (SchemaReferenceArray) never permits inline entries.
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
		typeKey = fmt.Sprintf("Anon_%s", fieldName)
	}
	if entry, ok := e.registry[typeKey]; ok {
		return entry.ID, nil
	}
	id := e.registerType(typeKey)
	e.schemaOrder = append(e.schemaOrder, id)
	schemaName := t.Name()
	if schemaName == "" {
		schemaName = typeKey
	}
	// Placeholder prevents re-entrant reprocessing: typeKey is already
	// present in e.registry (via registerType above) before we recurse into
	// its fields below, so a self-referencing field encountered during that
	// recursion resolves to this same id immediately via the registry-hit
	// branch above, rather than recursing again.
	e.schemas[id] = nil
	var fieldsBuf bytes.Buffer
	fieldOrdinal := int64(0)
	fieldCount := 0
	if err := e.extractStructFields(t, id, &fieldsBuf, &fieldOrdinal, &fieldCount); err != nil {
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
			// "enum" is handled upstream in writeStandardField and should
			// never reach here; "union"/"composite" are dispatched before
			// writeStandardField is ever called. Anything else is a typo
			// or an unsupported directive.
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

// Low-level JSON formatting helpers

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
				// Any other control character must be escaped per RFC 8259;
				// raw bytes < 0x20 are not valid inside a JSON string.
				fmt.Fprintf(buf, "\\u%04x", c)
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

// coerceDefaultValue writes the JSON representation of a default value for
// a scalar field type, validating that the raw tag string is actually
// compatible with that type (spec.md §9: "the generator must perform a type
// check and produce a validation error if the default value cannot be
// coerced to the field type"). Defaults are rejected outright for
// structural types (object, array, record, geometry, bytes, unknown) —
// none of them have a single-value default that means anything at the
// schema level.
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
	case "string":
		writeJSONString(buf, valStr)
	case "decimal":
		// Arbitrary-precision: represented as a JSON string so the exact
		// literal survives round-tripping without float64 precision loss.
		writeJSONString(buf, valStr)
	default:
		return fmt.Errorf("field %q: default values are not supported for type %q", fieldName, schemaType)
	}
	return nil
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
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

func GenerateDeterministicUUIDv7(ordinal int64, seedString string) string {
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
	// buf is always exactly 16 bytes, so uuid.FromBytes cannot fail here;
	// uuid.Must documents that invariant instead of silently discarding an
	// error that can never occur.
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

func parseSchemaTag(tag string) parsedSchemaTag {
	var parsed parsedSchemaTag
	if tag == "" {
		return parsed
	}
	if tag == "-" {
		parsed.Skip = true
		return parsed
	}
	parts := strings.Split(tag, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == 0 && !strings.Contains(part, "=") {
			if part == "-" {
				parsed.Skip = true
				return parsed
			}
			parsed.Name = part
			parsed.HasName = true
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		key := strings.TrimSpace(kv[0])
		val := ""
		if len(kv) > 1 {
			val = strings.TrimSpace(kv[1])
		}
		switch key {
		case "required":
			b := val == "true"
			parsed.Required = &b
		case "nullable":
			b := val == "true"
			parsed.Nullable = &b
		case "default":
			parsed.Default = &val
		case "type":
			parsed.TypeOverride = val
		case "values":
			if val != "" {
				parsed.Values = strings.Split(val, "|")
			}
		}
	}
	return parsed
}

func toSnakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || nextIsLower {
				b.WriteRune('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

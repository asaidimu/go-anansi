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
)

// FixedEpochMS is 2026-01-01T00:00:00.000Z in Unix milliseconds. Discovery
// and field ordinals are added to this to produce a monotonically
// increasing UUIDv7 timestamp component.
const FixedEpochMS int64 = 1767225600000

// Thread-safe global schema cache keyed by reflect.Type and tag.
type schemaCacheKey struct {
	t   reflect.Type
	tag string
}
var schemaCache sync.Map

type cachedSchema struct {
	data []byte
	err  error
}

// SchemaFrom extracts a meta-schema JSON document for any struct type using
// the default "anansi" tag for field names (and all other metadata).
// Results are cached globally per type for zero-allocation subsequent calls.
func SchemaFrom[T any]() ([]byte, error) {
	return SchemaFromWithTag[T]("")
}

// SchemaFromWithTag extracts a meta-schema JSON document for any struct type
// using a custom struct tag for field name/path resolution only.
// The custom tag is used as the primary source for field names (dot‑separated
// paths are allowed), falling back to the "anansi" tag's name if the custom
// tag is absent or empty, and finally to the snake‑cased Go field name.
// All other field metadata (required, nullable, type overrides, defaults, values)
// are taken exclusively from the "anansi" tag.
// Results are cached globally per (type, tag) pair.
func SchemaFromWithTag[T any](tag string) ([]byte, error) {
	var v T
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("root DTO target cannot be nil")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	key := schemaCacheKey{t: t, tag: tag}
	if cached, ok := schemaCache.Load(key); ok {
		res := cached.(cachedSchema)
		return res.data, res.err
	}

	data, err := ExtractDTOSchemaDirectWithTag(v, tag)
	schemaCache.Store(key, cachedSchema{data: data, err: err})
	return data, err
}

// ExtractDTOSchemaDirect streams JSON bytes directly into a buffer without
// intermediate maps or struct serialization. Uses the default "anansi" tag.
func ExtractDTOSchemaDirect(target any) ([]byte, error) {
	return ExtractDTOSchemaDirectWithTag(target, "")
}

// ExtractDTOSchemaDirectWithTag does the same as ExtractDTOSchemaDirect but
// uses the given custom tag for name/path resolution (see SchemaFromWithTag).
func ExtractDTOSchemaDirectWithTag(target any, tag string) ([]byte, error) {
	if target == nil {
		return nil, fmt.Errorf("root DTO target cannot be nil")
	}
	e := newDirectExtractor(tag)
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
	customTag        string // the custom tag to use for name/path resolution
}

func newDirectExtractor(customTag string) *directExtractor {
	return &directExtractor{
		discoveryOrdinal: 0,
		registry:         make(map[string]*schemaRegistryEntry),
		schemas:          make(map[string][]byte),
		schemaOrder:      make([]string, 0, 8),
		enumValues:       make(map[string][]string),
		syntheticSchemas: make(map[string]map[string]*syntheticSchema),
		customTag:        customTag,
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
	if err := e.extractStructFields(t, rootSchemaID, &fieldsBuf, &fieldOrdinal, &fieldCount); err != nil {
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

func (e *directExtractor) extractStructFields(t reflect.Type, owningSchemaID string, buf *bytes.Buffer, fieldOrdinal *int64, fieldCount *int) error {
	var localSynOrder []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// First parse the anansi tag to get all metadata (skip, required, etc.)
		anansiTag := parseSchemaTag(field.Tag.Get(AnansiTag))
		if anansiTag.Skip {
			continue
		}

		// If field is embedded (anonymous) and no type override, flatten its fields.
		if field.Anonymous && field.Type.Kind() == reflect.Struct && anansiTag.TypeOverride == "" {
			if err := e.extractStructFields(field.Type, owningSchemaID, buf, fieldOrdinal, fieldCount); err != nil {
				return err
			}
			continue
		}

		// Resolve the field name:
		// 1. If a custom tag is provided, use its first part (before comma) as the name.
		// 2. Else if the anansi tag has an explicit name, use that.
		// 3. Otherwise fall back to snake-cased Go field name.
		fieldName := ""
		if e.customTag != "" {
			ct := field.Tag.Get(e.customTag)
			if ct != "" && ct != "-" {
				// Take the first comma-separated part as the name (ignore options)
				parts := strings.SplitN(ct, ",", 2)
				name := strings.TrimSpace(parts[0])
				if name != "" {
					fieldName = name
				}
			}
		}
		if fieldName == "" && anansiTag.HasName {
			fieldName = anansiTag.Name
		}
		if fieldName == "" {
			fieldName = toSnakeCase(field.Name)
		}

		// Dotted paths are handled by synthetic schemas.
		if strings.Contains(fieldName, ".") {
			if _, err := e.writePathField(owningSchemaID, fieldOrdinal, &localSynOrder, field, fieldName, anansiTag); err != nil {
				return err
			}
			continue
		}

		// For non-path fields, we write a standard field entry.
		if anansiTag.Default != nil && (anansiTag.TypeOverride == "composite" || anansiTag.TypeOverride == "union") {
			return fmt.Errorf("field %q: default values are not supported for %s fields", fieldName, anansiTag.TypeOverride)
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
		switch anansiTag.TypeOverride {
		case "composite":
			err = e.writeCompositeField(buf, field, fieldName, anansiTag)
		case "union":
			err = e.writeUnionField(buf, field, fieldName, anansiTag)
		default:
			err = e.writeStandardField(buf, field, fieldName, anansiTag)
		}
		if err != nil {
			return err
		}
	}

	// Append synthetic schemas (dotted paths) to the current owning schema's fields.
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

func parseSchemaTag(tag string) parsedSchemaTag {
	var parsed parsedSchemaTag
	if tag == "" {
		return parsed
	}
	if tag == "-" {
		parsed.Skip = true
		return parsed
	}

	for i := 0; len(tag) > 0; i++ {
		var part string
		if idx := strings.IndexByte(tag, ','); idx >= 0 {
			part = strings.TrimSpace(tag[:idx])
			tag = tag[idx+1:]
		} else {
			part = strings.TrimSpace(tag)
			tag = ""
		}

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
			v := val
			parsed.Default = &v
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

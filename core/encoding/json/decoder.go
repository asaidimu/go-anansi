package json

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// decodeBytes decodes a serialized bytes payload back to its raw form. The
// serializer emits bytes fields as base64 strings (writeBytes), so the decoder
// must reverse that encoding; strings that are not valid base64 are tolerated
// as raw text so hand-written JSON still loads.
func decodeBytes(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return []byte(s)
	}
	return b
}

// DecodeJSON is an experimental, schema-driven JSON document decoder.
func DecodeJSON(cs *definition.CompiledSchema, data []byte) (*container.DataContainer, error) {
	p := newJSONParser(data)
	doc := container.NewDataContainer()
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, nil); err != nil {
		return nil, err
	}
	p.skipWS()
	if !p.eof() {
		return nil, fmt.Errorf("json: trailing data after JSON document")
	}
	return doc, nil
}

// DecodeJSONInto decodes into a caller-provided document, sourcing
// array-of-object child documents from pool when non-nil.
func DecodeJSONInto(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, pool *container.Pool) error {

	// NOTE: 1/2026-08-07-20:29
	// There is an inherent problem not addressed here:
	// Nested schemas for document arrays and records
	// require their own pools otherwise we loose the benefit of
	// targeted pools. This may grow costly very quickly.
	//
	p := newJSONParser(data)
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, pool); err != nil {
		return err
	}
	p.skipWS()
	if !p.eof() {
		return fmt.Errorf("json: trailing data after JSON document")
	}
	return nil
}

// DecodeJSONUnsafe behaves exactly like DecodeJSON, except that decoded
// string values alias the input buffer directly instead of being copied.
//
// Contract: the caller must guarantee that data is not mutated, reused, or
// returned to a pool for as long as the returned DataContainer — or anything
// derived from it (e.g. a value read back out with GetString) — is still in
// use. Violating this contract will corrupt already-decoded string data or,
// if data is later overwritten, silently return stale/incorrect strings.
// This trades that guarantee for eliminating the string-copy allocations
// that dominate decode's memory profile; only use it on a call path that
// owns data for the full lifetime of the document (e.g. decode → serve →
// discard, with no buffer pooling of the input bytes).
func DecodeJSONUnsafe(cs *definition.CompiledSchema, data []byte) (*container.DataContainer, error) {
	p := newJSONParserUnsafe(data)
	doc := container.NewDataContainer()
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, nil); err != nil {
		return nil, err
	}
	p.skipWS()
	if !p.eof() {
		return nil, fmt.Errorf("json: trailing data after JSON document")
	}
	return doc, nil
}

// DecodeJSONIntoUnsafe is the pooled-document counterpart of
// DecodeJSONUnsafe. See its docstring for the buffer-lifetime contract this
// places on the caller — it applies equally here.
func DecodeJSONIntoUnsafe(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, pool *container.Pool) error {
	p := newJSONParserUnsafe(data)
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, pool); err != nil {
		return err
	}
	p.skipWS()
	if !p.eof() {
		return fmt.Errorf("json: trailing data after JSON document")
	}
	return nil
}

// DecodeJSONField decodes the serialized JSON fragment of a single root-level
// field into doc, at the field's coordinate. It is the lenient (read-back)
// counterpart to full-document decode: absent required fields are tolerated
// and nothing is validated, so stored fragments can be re-materialized even
// when partial. Nested objects decode flattened into doc (their leaves land at
// the deep addresses the schema assigns); array-of-object children are sourced
// from pool when non-nil. data must be exactly what the encoder produced for
// this field (e.g. SerializeJSONPrefix output).
func DecodeJSONField(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, fieldName string, pool *container.Pool) error {
	abs, fieldIdx, ok := findRootField(cs, fieldName)
	if !ok {
		return fmt.Errorf("json: field %q not found", fieldName)
	}
	fd := cs.Descriptors[abs]
	p := newJSONParser(data)
	path := definition.ResolvedPath{definition.NewResolvedStep(0, uint8(fieldIdx))}
	if err := parseValueInto(p, cs, doc, fd, path, pool, false); err != nil {
		return fmt.Errorf("json: decode field %q: %w", fieldName, err)
	}
	p.skipWS()
	if !p.eof() {
		return fmt.Errorf("json: trailing data after JSON document")
	}
	return nil
}

// findRootField resolves a root-level field name to its absolute descriptor
// index and field index within the root schema slot.
func findRootField(cs *definition.CompiledSchema, name string) (abs int, fieldIdx uint16, ok bool) {
	if len(cs.Schemas) == 0 {
		return 0, 0, false
	}
	slot := cs.Schemas[0]
	for j := uint16(0); j < slot.FieldCount; j++ {
		if cs.FieldsMeta[slot.FieldStart+j].Name == name {
			return int(slot.FieldStart) + int(j), j, true
		}
	}
	return 0, 0, false
}

// decodeObject parses one JSON object against one schema slot. Absent
// required fields are validated (full-document decode).
func decodeObject(cs *definition.CompiledSchema, doc *container.DataContainer, schemaIdx uint8, path definition.ResolvedPath, p *jsonParser, pool *container.Pool) error {
	return decodeObjectInto(cs, doc, schemaIdx, path, p, pool, true)
}

// decodeObjectLenient is decodeObject without required-field validation. It is
// used when materializing stored fragments (row read-back) that may be partial
// or structurally absent.
func decodeObjectLenient(cs *definition.CompiledSchema, doc *container.DataContainer, schemaIdx uint8, path definition.ResolvedPath, p *jsonParser, pool *container.Pool) error {
	return decodeObjectInto(cs, doc, schemaIdx, path, p, pool, false)
}

// decodeObjectInto parses one JSON object against one schema slot.
func decodeObjectInto(cs *definition.CompiledSchema, doc *container.DataContainer, schemaIdx uint8, path definition.ResolvedPath, p *jsonParser, pool *container.Pool, validate bool) error {
	if int(schemaIdx) >= len(cs.Schemas) {
		return fmt.Errorf("json: schema slot %d out of range", schemaIdx)
	}
	slot := cs.Schemas[schemaIdx]
	if err := p.expect('{'); err != nil {
		return err
	}

	// Bitmask fast-path to prevent slice allocation for field tracking
	var seenMask uint64
	var seenHeap []bool
	if slot.FieldCount > 64 {
		seenHeap = make([]bool, slot.FieldCount)
	}

	if !p.take('}') {
		for {
			j, err := p.parseSchemaKey(cs, slot)
			if err != nil {
				return err
			}
			if err := p.expect(':'); err != nil {
				return err
			}
			if j < 0 {
				if err := p.skipValue(); err != nil {
					return err
				}
			} else {
				if slot.FieldCount <= 64 {
					seenMask |= (uint64(1) << uint(j))
				} else {
					seenHeap[j] = true
				}

				abs := int(slot.FieldStart) + j
				fd := cs.Descriptors[abs]
				step := definition.NewResolvedStep(schemaIdx, uint8(j))
				fieldPath := appendStep(path, step)
				if err := parseValueInto(p, cs, doc, fd, fieldPath, pool, validate); err != nil {
					return err
				}
			}
			if p.take('}') {
				break
			}
			if !p.take(',') {
				return p.errf("expected ',' or '}' in object, got %q", p.peek())
			}
		}
	}

	// Validate absent required fields. Defaults are deliberately NOT applied:
	// decode is a pure materialization, and default injection is owned by the
	// persistence layer — applying it here would make serialize→decode→serialize
	// round-trips non-idempotent. The lenient form skips validation entirely so
	// stored fragments can be read back without requiring every field.
	if !validate {
		return nil
	}
	for j := uint16(0); j < slot.FieldCount; j++ {
		isSeen := false
		if slot.FieldCount <= 64 {
			isSeen = (seenMask & (uint64(1) << uint(j))) != 0
		} else {
			isSeen = seenHeap[j]
		}

		if isSeen {
			continue
		}

		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		meta := cs.FieldsMeta[abs]
		if fd.Required() {
			return fmt.Errorf("json: missing required field %q", meta.Name)
		}
	}
	return nil
}

func appendStep(path definition.ResolvedPath, step definition.ResolvedStep) definition.ResolvedPath {
	return append(path, step)
}

// findField reports the index of the slot field whose name equals name, or -1.
func findField(cs *definition.CompiledSchema, slot definition.SchemaSlot, name string) int {
	for j := uint16(0); j < slot.FieldCount; j++ {
		if cs.FieldsMeta[slot.FieldStart+j].Name == name {
			return int(j)
		}
	}
	return -1
}

// parseSchemaKey consumes the string literal at the current position and
// returns the index of the matching field in slot, or -1 when the key does not
// belong to the schema. A schema's field names are a fixed set, so the raw
// literal bytes are compared directly against the canonical names the compiled
// schema already holds — no key string is ever allocated. Escaped keys (the
// rare slow path) fall back to the scratch buffer.
func (p *jsonParser) parseSchemaKey(cs *definition.CompiledSchema, slot definition.SchemaSlot) (int, error) {
	p.skipWS()
	if p.eof() || p.data[p.pos] != '"' {
		return -1, p.errf("expected string")
	}
	p.pos++
	start := p.pos

	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '"' {
			j := matchSchemaField(p.data[start:p.pos], cs, slot)
			p.pos++
			return j, nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
		p.pos++
	}

	p.pos = start
	s, err := p.parseStringSlow()
	if err != nil {
		return -1, err
	}
	return findField(cs, slot, s), nil
}

// matchSchemaField reports the index of the slot field whose canonical name
// equals b, or -1. Comparison is byte-wise and allocation-free.
func matchSchemaField(b []byte, cs *definition.CompiledSchema, slot definition.SchemaSlot) int {
	for j := uint16(0); j < slot.FieldCount; j++ {
		name := cs.FieldsMeta[slot.FieldStart+j].Name
		if len(name) == len(b) && equalBytesString(name, b) {
			return int(j)
		}
	}
	return -1
}

func equalBytesString(s string, b []byte) bool {
	for i := range b {
		if s[i] != b[i] {
			return false
		}
	}
	return true
}

func parseValue(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath, pool *container.Pool) error {
	return parseValueInto(p, cs, doc, fd, path, pool, true)
}

func parseValueInto(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath, pool *container.Pool, validate bool) error {
	// A null leaf is absence: consume it and leave the slot untouched. Stored
	// fragments (row read-back) legitimately carry null for unset fields, and
	// re-materializing them must not fail or invent a value.
	p.skipWS()
	if p.peek() == 'n' {
		if err := p.parseNull(); err != nil {
			return err
		}
		return nil
	}
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		childIdx := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeArrayObject:
			return parseArrayObjectInto(p, cs, doc, fd, path, childIdx, pool, validate)

		case container.TypeRecord:
			m, err := p.parseObjectAny()
			if err != nil {
				return err
			}
			return doc.SetRecord(internalKey(fd), m)

		default:
			return decodeObjectInto(cs, doc, childIdx, path, p, pool, validate)
		}
	}
	return parseLeaf(p, cs, doc, fd, path)
}

func parseArrayObject(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath, childIdx uint8, pool *container.Pool) error {
	return parseArrayObjectInto(p, cs, doc, fd, path, childIdx, pool, true)
}

func parseArrayObjectInto(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath, childIdx uint8, pool *container.Pool, validate bool) error {
	// Pre-allocate slice capacity to prevent growslice allocations
	children := make([]*container.DataContainer, 0, 8)
	err := p.parseArray(func() error {
		var child *container.DataContainer
		if pool != nil {
			child = pool.Get()
		} else {
			child = container.NewDataContainer()
		}
		if err := decodeObjectInto(cs, child, childIdx, path, p, pool, validate); err != nil {
			return err
		}
		children = append(children, child)
		return nil
	})
	if err != nil {
		return err
	}
	return doc.SetArrayObject(internalKey(fd), children)
}

func fieldName(cs *definition.CompiledSchema, fd definition.FieldDescriptor) string {
	return cs.FieldPath(fd)
}

func parseLeaf(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath) error {
	key, err := computeLeafKey(cs, fd, path)
	if err != nil {
		return err
	}
	switch fd.DataType() {
	case container.TypeInt:
		n, err := p.parseInteger()
		if err != nil {
			return err
		}
		return doc.SetInt(key, n)
	case container.TypeFloat:
		f, err := p.parseFloat()
		if err != nil {
			return err
		}
		return doc.SetFloat(key, f)
	case container.TypeString:
		s, err := p.parseString()
		if err != nil {
			return err
		}
		return doc.SetString(key, s)
	case container.TypeBool:
		b, err := p.parseBool()
		if err != nil {
			return err
		}
		return doc.SetBool(key, b)
	case container.TypeBytes:
		s, err := p.parseString()
		if err != nil {
			return err
		}
		return doc.SetBytes(key, decodeBytes(s))
	case container.TypeGeometry:
		g, err := parseGeometry(p)
		if err != nil {
			return err
		}
		return doc.SetGeometry(key, g)
	case container.TypeRecord:
		m, err := p.parseObjectAny()
		if err != nil {
			return err
		}
		return doc.SetRecord(key, m)
	case container.TypeArrayInt:
		v, err := parseArrayInt(p)
		if err != nil {
			return err
		}
		return doc.SetArrayInt(key, v)
	case container.TypeArrayFloat:
		v, err := parseArrayFloat(p)
		if err != nil {
			return err
		}
		return doc.SetArrayFloat(key, v)
	case container.TypeArrayString:
		v, err := parseArrayString(p)
		if err != nil {
			return err
		}
		return doc.SetArrayString(key, v)
	case container.TypeArrayBool:
		v, err := parseArrayBool(p)
		if err != nil {
			return err
		}
		return doc.SetArrayBool(key, v)
	case container.TypeArrayBytes:
		v, err := parseArrayBytes(p)
		if err != nil {
			return err
		}
		return doc.SetArrayBytes(key, v)
	case container.TypeArrayGeometry:
		v, err := parseArrayGeometry(p)
		if err != nil {
			return err
		}
		return doc.SetArrayGeometry(key, v)
	case container.TypeArrayUnknown:
		v, err := p.parseArrayAny()
		if err != nil {
			return err
		}
		return doc.SetArrayUnknown(key, v)
	case container.TypeUnknown:
		v, err := p.parseAny()
		if err != nil {
			return err
		}
		return doc.SetUnknown(key, v)
	}
	return fmt.Errorf("json: unsupported data type %d", fd.DataType())
}

// computeLeafKey resolves a leaf field's path to its DataContainerKey via the
// compiled schema's own address space. cs.Address memoizes path→address
// internally, so repeated resolution of the same path (across documents or
// array elements) is a single cached lookup — the compiled schema is the sole
// source of address truth; the codec keeps no parallel cache of its own.
func computeLeafKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, path definition.ResolvedPath) (container.DataContainerKey, error) {
	// FAST-PATH: Root level fields have empty paths; bypass cs.Address computation
	if len(path) == 0 {
		return internalKey(fd), nil
	}
	addr := cs.Address(path)
	if addr == 0 {
		return internalKey(fd), nil
	}
	dp, err := container.NewDataPoint(fd.DataType(), int32(addr))
	if err != nil {
		return 0, fmt.Errorf("json: build data point: %w", err)
	}
	return container.NewDataContainerKey(dp, uint32(fd)), nil
}

func internalKey(fd definition.FieldDescriptor) container.DataContainerKey {
	return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd))
}

func setByType(doc *container.DataContainer, dt container.DataType, key container.DataContainerKey, value any) error {
	switch dt {
	case container.TypeInt:
		n, err := asInt64(value)
		if err != nil {
			return err
		}
		return doc.SetInt(key, n)
	case container.TypeFloat:
		f, err := asFloat64(value)
		if err != nil {
			return err
		}
		return doc.SetFloat(key, f)
	case container.TypeString:
		return doc.SetString(key, asString(value))
	case container.TypeBool:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("json: expected boolean, got %T", value)
		}
		return doc.SetBool(key, b)
	case container.TypeBytes:
		return doc.SetBytes(key, []byte(asString(value)))
	case container.TypeGeometry:
		g, err := asGeometry(value)
		if err != nil {
			return err
		}
		return doc.SetGeometry(key, g)
	case container.TypeRecord:
		m, ok := value.(map[string]any)
		if !ok {
			return typeErr("record", "record", value)
		}
		return doc.SetRecord(key, m)
	case container.TypeArrayInt:
		v, err := asInt64Slice(value)
		if err != nil {
			return err
		}
		return doc.SetArrayInt(key, v)
	case container.TypeArrayFloat:
		v, err := asFloat64Slice(value)
		if err != nil {
			return err
		}
		return doc.SetArrayFloat(key, v)
	case container.TypeArrayString:
		return doc.SetArrayString(key, asStringSlice(value))
	case container.TypeArrayBool:
		v, err := asBoolSlice(value)
		if err != nil {
			return err
		}
		return doc.SetArrayBool(key, v)
	case container.TypeArrayBytes:
		return doc.SetArrayBytes(key, asBytesSlice(value))
	case container.TypeArrayGeometry:
		v, err := asGeometrySlice(value)
		if err != nil {
			return err
		}
		return doc.SetArrayGeometry(key, v)
	case container.TypeArrayUnknown:
		v, ok := value.([]any)
		if !ok {
			return typeErr("array", "array", value)
		}
		return doc.SetArrayUnknown(key, v)
	case container.TypeUnknown:
		return doc.SetUnknown(key, value)
	}
	return fmt.Errorf("json: unsupported data type %d", dt)
}

func typeErr(path, want string, got any) error {
	return fmt.Errorf("json: field %q expects %s, got %T", path, want, got)
}

func asInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	}
	return 0, fmt.Errorf("json: expected integer, got %T", v)
}

func asFloat64(v any) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case int64:
		return float64(t), nil
	case int:
		return float64(t), nil
	case string:
		return strconv.ParseFloat(t, 64)
	}
	return 0, fmt.Errorf("json: expected number, got %T", v)
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asGeometry(v any) ([][]float64, error) {
	outer, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json: expected geometry (array of rings), got %T", v)
	}
	out := make([][]float64, len(outer))
	for i, ring := range outer {
		inner, ok := ring.([]any)
		if !ok {
			return nil, fmt.Errorf("json: geometry ring %d is not an array", i)
		}
		out[i] = make([]float64, len(inner))
		for j, c := range inner {
			f, err := asFloat64(c)
			if err != nil {
				return nil, fmt.Errorf("json: geometry ring %d coord %d: %w", i, j, err)
			}
			out[i][j] = f
		}
	}
	return out, nil
}

func asInt64Slice(v any) ([]int64, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json: expected array, got %T", v)
	}
	out := make([]int64, len(arr))
	for i, e := range arr {
		n, err := asInt64(e)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

func asFloat64Slice(v any) ([]float64, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json: expected array, got %T", v)
	}
	out := make([]float64, len(arr))
	for i, e := range arr {
		f, err := asFloat64(e)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

func asStringSlice(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, len(arr))
	for i, e := range arr {
		out[i] = asString(e)
	}
	return out
}

func asBoolSlice(v any) ([]bool, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json: expected array, got %T", v)
	}
	out := make([]bool, len(arr))
	for i, e := range arr {
		b, ok := e.(bool)
		if !ok {
			return nil, fmt.Errorf("json: array element %d is not a boolean", i)
		}
		out[i] = b
	}
	return out, nil
}

func asBytesSlice(v any) [][]byte {
	arr, _ := v.([]any)
	out := make([][]byte, len(arr))
	for i, e := range arr {
		out[i] = decodeBytes(asString(e))
	}
	return out
}

func asGeometrySlice(v any) ([][][]float64, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json: expected array of geometry, got %T", v)
	}
	out := make([][][]float64, len(arr))
	for i, e := range arr {
		g, err := asGeometry(e)
		if err != nil {
			return nil, err
		}
		out[i] = g
	}
	return out, nil
}

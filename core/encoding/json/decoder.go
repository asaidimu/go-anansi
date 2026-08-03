package json

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// DecodeJSON is an experimental, schema-driven JSON document decoder.
func DecodeJSON(cs *definition.CompiledSchema, data []byte) (*container.DataContainer, error) {
	cache, err := getAddressCache(cs)
	if err != nil {
		return nil, err
	}
	p := newJSONParser(data)
	doc := container.NewDataContainer()
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, nil, cache); err != nil {
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
	cache, err := getAddressCache(cs)
	if err != nil {
		return err
	}
	p := newJSONParser(data)
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, pool, cache); err != nil {
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
	cache, err := getAddressCache(cs)
	if err != nil {
		return nil, err
	}
	p := newJSONParserUnsafe(data)
	doc := container.NewDataContainer()
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, nil, cache); err != nil {
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
	cache, err := getAddressCache(cs)
	if err != nil {
		return err
	}
	p := newJSONParserUnsafe(data)
	path := make(definition.ResolvedPath, 0, 16)
	if err := decodeObject(cs, doc, 0, path, p, pool, cache); err != nil {
		return err
	}
	p.skipWS()
	if !p.eof() {
		return fmt.Errorf("json: trailing data after JSON document")
	}
	return nil
}

// decodeObject parses one JSON object against one schema slot.
func decodeObject(cs *definition.CompiledSchema, doc *container.DataContainer, schemaIdx uint8, path definition.ResolvedPath, p *jsonParser, pool *container.Pool, cache []container.DataContainerKey) error {
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
			name, err := p.parseString()
			if err != nil {
				return err
			}
			if err := p.expect(':'); err != nil {
				return err
			}
			j := findField(cs, slot, name)
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
				if err := parseValue(p, cs, doc, fd, abs, fieldPath, pool, cache); err != nil {
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
	// round-trips non-idempotent.
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

func findField(cs *definition.CompiledSchema, slot definition.SchemaSlot, name string) int {
	for j := uint16(0); j < slot.FieldCount; j++ {
		if cs.FieldsMeta[slot.FieldStart+j].Name == name {
			return int(j)
		}
	}
	return -1
}

func parseValue(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, abs int, path definition.ResolvedPath, pool *container.Pool, cache []container.DataContainerKey) error {
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		childIdx := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeArrayObject:
			return parseArrayObject(p, cs, doc, fd, path, childIdx, pool, cache)

		case container.TypeRecord:
			m, err := p.parseObjectAny()
			if err != nil {
				return err
			}
			return doc.SetRecord(internalKey(fd), m)

		default:
			return decodeObject(cs, doc, childIdx, path, p, pool, cache)
		}
	}
	return parseLeaf(p, cs, doc, fd, abs, path, cache)
}

func parseArrayObject(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, path definition.ResolvedPath, childIdx uint8, pool *container.Pool, cache []container.DataContainerKey) error {
	// Pre-allocate slice capacity to prevent growslice allocations
	children := make([]*container.DataContainer, 0, 8)
	err := p.parseArray(func() error {
		var child *container.DataContainer
		if pool != nil {
			child = pool.Get()
		} else {
			child = container.NewDataContainer()
		}
		if err := decodeObject(cs, child, childIdx, path, p, pool, cache); err != nil {
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

func parseLeaf(p *jsonParser, cs *definition.CompiledSchema, doc *container.DataContainer, fd definition.FieldDescriptor, abs int, path definition.ResolvedPath, cache []container.DataContainerKey) error {
	key, err := leafKey(cs, fd, abs, path, cache)
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
		return doc.SetBytes(key, []byte(s))
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

// addressCacheRegistry memoizes, per *definition.CompiledSchema, the resolved
// container.DataContainerKey for every leaf field descriptor. Because each
// schemaIdx in a CompiledSchema corresponds to exactly one position in the
// schema tree, the path leading to any given leaf field is fixed for the
// lifetime of the schema — so the address it resolves to via cs.Address never
// changes across documents or array elements. Precomputing it once here
// removes a cs.Address (hash + cache-lookup) call from every leaf field of
// every decoded value, which otherwise dominates decode's CPU profile.
var addressCacheRegistry sync.Map // map[*definition.CompiledSchema][]container.DataContainerKey

// getAddressCache returns the memoized per-descriptor key cache for cs,
// building it on first use. If the schema-shaped walk fails (indicating a
// schema construction bug rather than a per-document data problem), the
// error is returned and nothing is cached, so callers fall back to
// per-call computation via computeLeafKey.
func getAddressCache(cs *definition.CompiledSchema) ([]container.DataContainerKey, error) {
	if v, ok := addressCacheRegistry.Load(cs); ok {
		return v.([]container.DataContainerKey), nil
	}
	cache := make([]container.DataContainerKey, len(cs.Descriptors))
	path := make(definition.ResolvedPath, 0, 16)
	if err := buildAddressCache(cs, 0, path, cache); err != nil {
		return nil, err
	}
	actual, _ := addressCacheRegistry.LoadOrStore(cs, cache)
	return actual.([]container.DataContainerKey), nil
}

// buildAddressCache walks the schema tree from schemaIdx, mirroring the
// exact dispatch decisions parseValue makes at decode time (structural vs.
// leaf), and fills cache[abs] with the resolved key for every leaf
// descriptor it visits. Structural fields (nested objects, array-of-object,
// record) use internalKey at decode time instead of leafKey, so they are
// walked purely to reach their descendants and are never written to cache.
func buildAddressCache(cs *definition.CompiledSchema, schemaIdx uint8, path definition.ResolvedPath, cache []container.DataContainerKey) error {
	if int(schemaIdx) >= len(cs.Schemas) {
		return fmt.Errorf("json: build address cache: schema slot %d out of range", schemaIdx)
	}
	slot := cs.Schemas[schemaIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		step := definition.NewResolvedStep(schemaIdx, uint8(j))
		fieldPath := appendStep(path, step)

		if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
			// Matches parseValue's structural branch. TypeRecord terminates
			// there without descending further and without a leafKey lookup;
			// TypeArrayObject and plain nested objects both recurse with the
			// same fieldPath every child/level shares at decode time.
			if fd.DataType() != container.TypeRecord {
				if err := buildAddressCache(cs, fd.ChildSchemaIdx(), fieldPath, cache); err != nil {
					return err
				}
			}
			continue
		}

		key, err := computeLeafKey(cs, fd, fieldPath)
		if err != nil {
			return fmt.Errorf("json: build address cache for schema %d field %d: %w", schemaIdx, j, err)
		}
		cache[abs] = key
	}
	return nil
}

// leafKey returns the DataContainerKey for a leaf field. It expects a
// pre‑computed cache (obtained once per decode call) and uses it directly
// as a slice index. The fallback to computeLeafKey is kept only for safety.
func leafKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, abs int, path definition.ResolvedPath, cache []container.DataContainerKey) (container.DataContainerKey, error) {
	if abs >= 0 && abs < len(cache) {
		return cache[abs], nil
	}
	// Fallback (should never happen for a valid schema)
	return computeLeafKey(cs, fd, path)
}

// computeLeafKey does the actual path-to-address resolution. It is the sole
// place that calls cs.Address, used both as the cache-miss fallback and by
// buildAddressCache to populate the cache.
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
		out[i] = []byte(asString(e))
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

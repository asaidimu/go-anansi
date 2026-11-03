package anansijson

import (
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// TYPE PLAN COMPILER
// ============================================================================
//
// The fast path compiles, once per Go type, a static plan that maps the
// struct's anansi-tagged fields directly to and from JSON bytes: field
// offsets, precomputed escaped key literals, per-field encoders, and the
// schema's required-field flags. Marshal and Unmarshal then run without
// per-call reflection, boxing, path resolution, or container traffic.
//
// The plan refuses (err != nil) any shape the legacy container path handles
// that the fast path does not model exactly — time.Time fields, maps,
// pointer fields, dotted tag paths, geometry overrides, system-model embeds,
// … — and the caller falls back. Byte-for-byte output parity with the legacy
// path is pinned by the differential tests in plan_test.go.

type fieldKind uint8

const (
	kString fieldKind = iota
	kInt              // signed integer, storage width in size
	kUint             // unsigned integer, storage width in size
	kFloat            // float32 or float64
	kBool
	kBytes // []byte
	kObject
	kArrayObject
	kArrayString
	kArrayInt
	kArrayUint
	kArrayFloat
	kArrayBool
)

// maxPlanDepth bounds object nesting; deeper shapes fall back to the legacy
// path (Go value structs cannot recurse without pointers, so this is paranoia,
// not a reachable limit).
const maxPlanDepth = 32

var (
	errRefused = errors.New("anansijson: type not supported by the fast path")
	errDepth   = errors.New("anansijson: struct nesting too deep for the fast path")
)

type fieldNode struct {
	name      string
	keyLit    []byte // `"name":` with JSON escaping applied, precomputed
	kind      fieldKind
	size      uint8   // int/uint/float storage width in bytes
	offset    uintptr // field offset within its struct
	omitEmpty bool

	// object kinds
	child     *objPlan                  // kObject / kArrayObject element plan
	elemSize  uintptr                   // kArrayObject element stride
	elemType  reflect.Type              // element Go type (kArrayObject growth)
	sliceType reflect.Type              // precomputed reflect.SliceOf(elemType)
	emptyHdr  sliceHeader               // shared zero-length, non-nil slice header for []
	growTypes []reflect.Type            // [cap]elem array types for growth caps 4,8,...,1024; index = log2(cap)-2
	zeroFn    func(unsafe.Pointer) bool // kObject: reports all-leaves-zero (IsZero parity)
	required  bool                      // decode: schema marks the field required
}

type objPlan struct {
	fields    []fieldNode // emit order (subtree-min name order, see sortFields)
	byName    map[string]*fieldNode
	required  []int16 // indices into fields of schema-required entries
	nRequired int
	sizeHint  int
}

type planEntry struct {
	root *objPlan
	err  error // non-nil ⇒ use the legacy path for this type
}

var planCache sync.Map // reflect.Type -> *planEntry

// initSliceGrowth precomputes the allocation machinery for a slice field:
// the header of a shared zero-length, non-nil slice (an empty JSON array is
// decoded by copying it in) and the typed array ladder growth allocates
// through. Each doubling is a single reflect.New of a [cap]elem type;
// reflect.MakeSlice would box a second, header-shaped object for its Value
// on every call.
func initSliceGrowth(n *fieldNode, elem reflect.Type) {
	n.sliceType = reflect.SliceOf(elem)
	n.elemSize = elem.Size()
	v := reflect.MakeSlice(n.sliceType, 0, 0)
	n.emptyHdr = sliceHeader{data: v.UnsafePointer(), len: 0, cap: 0}
	for cap := 4; cap <= 1024; cap *= 2 {
		n.growTypes = append(n.growTypes, reflect.ArrayOf(cap, elem))
	}
}

var timeType = reflect.TypeFor[time.Time]()

// planFor returns the cached compiled plan for t (a struct type). A refusal
// is cached too, so unsupported types pay the probe once.
func planFor(t reflect.Type) *planEntry {
	if v, ok := planCache.Load(t); ok {
		return v.(*planEntry)
	}
	e := &planEntry{}
	root, err := buildPlan(t, nil, 0, 0)
	if err != nil {
		e.err = err
	} else {
		e.root = root
	}
	actual, _ := planCache.LoadOrStore(t, e)
	return actual.(*planEntry)
}

// planForDecode builds (or loads) the plan with schema required-flags bound
// in, via the same cached pool the legacy path uses.
func planForDecode(t reflect.Type) (*planEntry, error) {
	key := decodePlanKey{t}
	if v, ok := planCache.Load(key); ok {
		e := v.(*planEntry)
		return e, e.err
	}
	pool, err := binding.PoolForType(t)
	if err != nil {
		e := &planEntry{err: err}
		actual, _ := planCache.LoadOrStore(key, e)
		return actual.(*planEntry), actual.(*planEntry).err
	}
	root, err := buildPlan(t, pool.CS, 0, 0)
	e := &planEntry{err: err}
	if err == nil {
		e.root = root
	}
	actual, _ := planCache.LoadOrStore(key, e)
	return actual.(*planEntry), actual.(*planEntry).err
}

type decodePlanKey struct{ t reflect.Type }

// slotFieldIndex returns the position of name within a schema slot, or -1.
func slotFieldIndex(cs *definition.CompiledSchema, slot definition.SchemaSlot, name string) int {
	for j := uint16(0); j < slot.FieldCount; j++ {
		if cs.FieldsMeta[int(slot.FieldStart)+int(j)].Name == name {
			return int(j)
		}
	}
	return -1
}

// buildPlan compiles t's fields into an objPlan. When cs is non-nil the
// schema slot at slotIdx supplies required flags and per-child slots; the
// binding fields must match it exactly or the plan is refused.
func buildPlan(t reflect.Type, cs *definition.CompiledSchema, slotIdx int, depth int) (*objPlan, error) {
	if depth > maxPlanDepth {
		return nil, errDepth
	}

	specs, err := binding.FieldsForType(t)
	if err != nil {
		return nil, err
	}

	var slot definition.SchemaSlot
	if cs != nil {
		if slotIdx >= len(cs.Schemas) {
			return nil, errRefused
		}
		slot = cs.Schemas[slotIdx]
	}

	p := &objPlan{byName: make(map[string]*fieldNode, len(specs))}
	for i := range specs {
		spec := &specs[i]
		if spec.IsSystemEmbed {
			// The legacy path errors while resolving system-embed fields
			// against the system-free DTO schema; refuse so the fallback
			// reproduces that.
			return nil, errRefused
		}
		if _, dup := p.byName[spec.Name]; dup {
			return nil, errRefused
		}
		if spec.Name == "" || containsByte(spec.Name, '.') {
			// Dotted tag paths restructure the document; the legacy path
			// owns them.
			return nil, errRefused
		}

		n := &fieldNode{
			name:      spec.Name,
			keyLit:    appendKeyLit(nil, spec.Name),
			omitEmpty: spec.OmitEmpty,
		}
		var fd definition.FieldDescriptor
		if cs != nil {
			j := slotFieldIndex(cs, slot, spec.Name)
			if j < 0 {
				return nil, errRefused
			}
			fd = cs.Descriptors[int(slot.FieldStart)+j]
			n.required = fd.Required()
		}

		if err := classifyField(n, spec.Type, cs, fd, depth); err != nil {
			return nil, err
		}

		if n.offset, err = offsetOf(t, spec.Index); err != nil {
			return nil, err
		}

		p.fields = append(p.fields, *n)
		p.byName[spec.Name] = &p.fields[len(p.fields)-1]
		if n.required {
			p.nRequired++
		}
		p.sizeHint += fieldSizeHint(n)
	}

	if cs != nil && p.nRequired > 0 {
		for i := range p.fields {
			if p.fields[i].required {
				p.required = append(p.required, int16(i))
			}
		}
	}

	sortFields(p)
	if err := buildZeroChecks(p); err != nil {
		return nil, err
	}
	return p, nil
}

// classifyField fills in kind/size/child from the field's Go type, refusing
// shapes the fast path does not model. When cs is non-nil, fd carries the
// schema slot of object children for the sub-plan.
//
// Scalar widths deliberately mirror the legacy coercion layer exactly
// (binding.AsInt64/AsFloat64 accept int, int32, int64, uint64, float64):
// other widths error on the container path, so the plan refuses them and the
// fallback reproduces the error.
func classifyField(n *fieldNode, t reflect.Type, cs *definition.CompiledSchema, fd definition.FieldDescriptor, depth int) error {
	switch t.Kind() {
	case reflect.String:
		n.kind, n.size = kString, 0
		return nil
	case reflect.Int, reflect.Int32, reflect.Int64:
		n.kind, n.size = kInt, uint8(t.Size())
		return nil
	case reflect.Uint64:
		n.kind, n.size = kUint, uint8(t.Size())
		return nil
	case reflect.Float64:
		n.kind, n.size = kFloat, uint8(t.Size())
		return nil
	case reflect.Bool:
		n.kind, n.size = kBool, 0
		return nil
	case reflect.Slice:
		return classifySlice(n, t, cs, fd, depth)
	case reflect.Struct:
		if t == timeType {
			return errRefused
		}
		return classifyObject(n, t, cs, childSlotIdx(cs, fd), depth)
	}
	// int8/int16, other uint widths, float32, maps, pointers, interfaces,
	// arrays, … are refused: the legacy coercion errors on them too.
	return errRefused
}

func classifySlice(n *fieldNode, t reflect.Type, cs *definition.CompiledSchema, fd definition.FieldDescriptor, depth int) error {
	elem := t.Elem()
	switch {
	case elem.Kind() == reflect.Uint8:
		// The legacy walk converts []byte to []any and the bytes slot then
		// rejects it — the legacy path errors on bytes fields, so refuse and
		// let the fallback reproduce that.
		return errRefused
	case elem.Kind() == reflect.Struct && elem != timeType:
		n.kind = kArrayObject
		n.elemType = elem
		initSliceGrowth(n, elem)
		var err error
		n.child, err = buildPlan(elem, cs, int(childSlotIdx(cs, fd)), depth+1)
		if err != nil {
			return err
		}
		if n.child.lenFields() == 0 {
			return errRefused
		}
		return nil
	}
	switch elem.Kind() {
	case reflect.String:
		n.kind = kArrayString
	case reflect.Int, reflect.Int32, reflect.Int64:
		n.kind = kArrayInt
	case reflect.Uint64:
		n.kind = kArrayUint
	case reflect.Float64:
		n.kind = kArrayFloat
	case reflect.Bool:
		n.kind = kArrayBool
	default:
		return errRefused
	}
	n.elemType = elem
	initSliceGrowth(n, elem)
	return nil
}

func classifyObject(n *fieldNode, t reflect.Type, cs *definition.CompiledSchema, slotIdx uint8, depth int) error {
	n.kind = kObject
	var err error
	n.child, err = buildPlan(t, cs, int(slotIdx), depth+1)
	if err != nil {
		return err
	}
	if n.child.lenFields() == 0 {
		// An empty object child is omitted entirely by the legacy path
		// (an empty map writes no leaves); refuse so it stays legacy.
		return errRefused
	}
	return nil
}

func childSlotIdx(cs *definition.CompiledSchema, fd definition.FieldDescriptor) uint8 {
	if cs == nil {
		return 0
	}
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		return fd.ChildSchemaIdx()
	}
	return 0
}

// offsetOf accumulates field offsets along a flattened index path. Every
// intermediate hop is an anonymous struct embed (value kind — pointer embeds
// are never flattened by the binder), so plain offset arithmetic matches
// reflect's FieldByIndex exactly.
func offsetOf(t reflect.Type, index []int) (uintptr, error) {
	var off uintptr
	ct := t
	for _, i := range index {
		if ct.Kind() != reflect.Struct {
			return 0, errRefused
		}
		if i < 0 || i >= ct.NumField() {
			return 0, errRefused
		}
		off += ct.Field(i).Offset
		ct = ct.Field(i).Type
	}
	return off, nil
}

// sortFields orders each level by the smallest full dotted name in each
// subtree — the same order the legacy serializer's sort-then-group emit
// produces (direct leaves group with their segment in dotted-sorted
// position).
func sortFields(p *objPlan) {
	sort.SliceStable(p.fields, func(i, j int) bool {
		return subtreeMinName(&p.fields[i]) < subtreeMinName(&p.fields[j])
	})
	// Recurse into children after reordering (indices shifted).
	for i := range p.fields {
		if p.fields[i].child != nil {
			sortFields(p.fields[i].child)
		}
	}
	// Rebuild byName pointers after the sort moved nodes.
	for i := range p.fields {
		p.byName[p.fields[i].name] = &p.fields[i]
	}
}

func subtreeMinName(n *fieldNode) string {
	if n.child == nil || n.kind != kObject {
		return n.name
	}
	min := ""
	for i := range n.child.fields {
		full := n.name + "." + subtreeMinName(&n.child.fields[i])
		if min == "" || full < min {
			min = full
		}
	}
	if min == "" {
		return n.name
	}
	return min
}

// buildZeroChecks wires the all-leaves-zero predicate for object fields that
// carry omitempty, mirroring reflect.Value.IsZero on the struct value.
func buildZeroChecks(p *objPlan) error {
	for i := range p.fields {
		n := &p.fields[i]
		if n.kind == kObject {
			if err := buildZeroChecks(n.child); err != nil {
				return err
			}
			if n.omitEmpty {
				n.zeroFn = zeroCheckFor(n.child)
			}
		}
	}
	return nil
}

// zeroCheckFor builds a closure reporting whether the struct at fp is zero
// according to every leaf plan (slices: nil; strings: ""; numerics: 0;
// bool: false; nested objects: recursive).
func zeroCheckFor(p *objPlan) func(unsafe.Pointer) bool {
	type leafCheck struct {
		off uintptr
		fn  func(unsafe.Pointer) bool
	}
	checks := make([]leafCheck, 0, len(p.fields))
	for i := range p.fields {
		n := &p.fields[i]
		off := n.offset
		switch n.kind {
		case kString:
			checks = append(checks, leafCheck{off, func(fp unsafe.Pointer) bool { return *(*string)(fp) == "" }})
		case kInt, kUint, kFloat:
			fn := zeroScalarCheck(n.kind, n.size)
			checks = append(checks, leafCheck{off, fn})
		case kBool:
			checks = append(checks, leafCheck{off, func(fp unsafe.Pointer) bool { return !*(*bool)(fp) }})
		case kBytes, kArrayString, kArrayInt, kArrayUint, kArrayFloat, kArrayBool, kArrayObject:
			checks = append(checks, leafCheck{off, func(fp unsafe.Pointer) bool { return *(*unsafe.Pointer)(fp) == nil }})
		case kObject:
			if sub := zeroCheckFor(n.child); sub != nil {
				checks = append(checks, leafCheck{off, sub})
			}
		}
	}
	return func(fp unsafe.Pointer) bool {
		for _, c := range checks {
			if !c.fn(unsafe.Pointer(uintptr(fp) + c.off)) {
				return false
			}
		}
		return true
	}
}

func zeroScalarCheck(kind fieldKind, size uint8) func(unsafe.Pointer) bool {
	switch kind {
	case kFloat:
		switch size {
		case 4:
			return func(fp unsafe.Pointer) bool { return *(*float32)(fp) == 0 }
		default:
			return func(fp unsafe.Pointer) bool { return *(*float64)(fp) == 0 }
		}
	case kUint:
		switch size {
		case 1:
			return func(fp unsafe.Pointer) bool { return *(*uint8)(fp) == 0 }
		case 2:
			return func(fp unsafe.Pointer) bool { return *(*uint16)(fp) == 0 }
		case 4:
			return func(fp unsafe.Pointer) bool { return *(*uint32)(fp) == 0 }
		default:
			return func(fp unsafe.Pointer) bool { return *(*uint64)(fp) == 0 }
		}
	default:
		switch size {
		case 1:
			return func(fp unsafe.Pointer) bool { return *(*int8)(fp) == 0 }
		case 2:
			return func(fp unsafe.Pointer) bool { return *(*int16)(fp) == 0 }
		case 4:
			return func(fp unsafe.Pointer) bool { return *(*int32)(fp) == 0 }
		default:
			return func(fp unsafe.Pointer) bool { return *(*int64)(fp) == 0 }
		}
	}
}

func fieldSizeHint(n *fieldNode) int {
	switch n.kind {
	case kString:
		return 48
	case kInt, kUint:
		return 16
	case kFloat:
		return 24
	case kBool:
		return 8
	case kBytes:
		return 32
	case kObject:
		return 2 + n.child.sizeHint
	case kArrayObject:
		return 32 + n.child.sizeHint
	default:
		return 32
	}
}

func (p *objPlan) lenFields() int { return len(p.fields) }

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// appendKeyLit appends the escaped `"name":` key literal.
func appendKeyLit(dst []byte, name string) []byte {
	dst = append(dst, '"')
	dst = appendEscaped(dst, name)
	return append(dst, '"', ':')
}

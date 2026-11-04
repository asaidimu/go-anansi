package definition

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/container"
)

// =============================================================================
// CONTAINER-NATIVE VALIDATION
// =============================================================================
//
// ContainerValidator validates *container.DataContainer values against a
// schema without materializing a map. The map validator (DocumentValidator)
// requires map[string]any input, so container-backed callers must materialize
// the whole document — allocating a map tree and boxing every value — before
// validation can even start, and the graph then re-walks those maps by path
// parts on every node. This validator compiles the schema into a flat plan of
// container keys instead: validating a document is one positions-map probe
// per field (plus one typed read per enum'd field), with zero boxing and zero
// allocations on the happy path.
//
// Coverage. The typed plan natively handles the shapes the container stores
// typed: scalar leaves (int/float/string/bool/bytes), arrays of scalars,
// flattened nested objects (child schemas share the parent's flat address
// space), and arrays of objects (each item is its own container, validated
// against the child plan). Required, explicit-null and enum checks mirror the
// map validator's observable behavior — same issue codes, messages and paths;
// failNullNotAllowed and deepEqual are shared. Structurally impossible checks
// are skipped, not approximated: a typed slot cannot hold a value of the
// wrong type, and a container cannot hold a key the schema does not declare,
// so TYPE_MISMATCH on present values and UNEXPECTED_FIELD cannot fire on this
// lane.
//
// Schemas that use features the flat plan cannot express — record payloads
// (maps), geometry, unions/composites, cross-field constraints, custom
// predicates, enum storage whose value type does not match its slot — fall
// back to the map lane: the validator materializes the document once
// (cvMaterializeSlot, the same semantics as the document layer's ToMap) and
// runs the standard ValidationGraph, so behavior is identical to
// DocumentValidator for those schemas. Compile reports which lane a schema
// took via FastPath.
//
// Modes mirror traverse's post-filter: PartialStrict drops
// REQUIRED_FIELD_MISSING, Loose additionally drops UNEXPECTED_FIELD (which
// the native lane cannot emit anyway).
//
// The differential test suite (container_validator_test.go) pins every issue
// this validator can emit against DocumentValidator run on the materialized
// map for the same container, across all three modes.

// ContainerValidator validates container-backed documents against a compiled
// schema. Construct with NewContainerValidator.
type ContainerValidator struct {
	cs     *CompiledSchema
	fmap   PredicateMap
	config ValidationConfig

	root *cvSlot

	// unsupported is non-empty when the schema uses features the flat plan
	// cannot express (or the plan could reach the configured MaxDepth);
	// validation then goes through the map lane.
	unsupported string
	graph       *ValidationGraph
}

// cvSlot is the compiled plan for one schema slot: the flat leaf checks plus
// the object/array-of-object children rooted at this slot.
type cvSlot struct {
	idx     uint8
	leaves  []cvLeaf
	subs    []cvSub
	maxPath int // deepest leaf path (segments + array nesting) in this subtree
}

// cvLeaf is one typed terminal field: where to probe, what to report, and the
// enum lookup if the field has one this lane can evaluate.
type cvLeaf struct {
	key      container.DataContainerKey
	path     string // dotted path relative to the owning slot
	dt       container.DataType
	required bool
	nullable bool
	enum     *cvEnum
}

// cvSub is a non-terminal object field: a flattened child schema sharing the
// parent container, or an array-of-object field whose items are child
// containers.
type cvSub struct {
	leafKey  container.DataContainerKey // internal key for array-of-object; zero for flattened objects
	isArray  bool
	path     string // dotted path relative to the owning slot
	required bool
	nullable bool
	child    *cvSlot
}

// cvEnum holds one enum's allowed values, typed by the slot's storage so the
// check compares without boxing: string enums keep []string, numeric enums
// []int64 (the compiler stores numeric enum values as int64 regardless of
// whether the declared type was integer or number).
type cvEnum struct {
	strs []string
	ints []int64
}

// NewContainerValidator compiles sc and returns a container-native validator.
// fmap is only consulted on the fallback lane (custom predicates and
// cross-field rules), matching DocumentValidator semantics.
func NewContainerValidator(sc *Schema, fmap PredicateMap) (*ContainerValidator, error) {
	return NewContainerValidatorWithConfig(sc, fmap, DefaultValidationConfig())
}

// NewContainerValidatorWithConfig is NewContainerValidator with explicit
// ValidationConfig.
func NewContainerValidatorWithConfig(sc *Schema, fmap PredicateMap, config ValidationConfig) (*ContainerValidator, error) {
	if sc == nil {
		return nil, ErrInvalidSchema.WithMessage("cannot build a ContainerValidator for a nil schema")
	}

	rs, err := Compile(sc)
	if err != nil {
		return nil, err
	}
	cs, err := Link(rs)
	if err != nil {
		return nil, err
	}

	v := &ContainerValidator{cs: cs, fmap: fmap, config: config}
	b := cvBuilder{cs: cs}
	root, maxPath, err := b.buildSlot(0, nil, "", 0)
	if err != nil {
		return nil, err
	}
	v.root = root
	v.unsupported = b.unsupported

	if v.unsupported != "" || maxPath >= config.MaxDepth {
		v.unsupported = firstNonEmpty(v.unsupported, "schema reaches the configured MaxDepth")
		graph, err := compileValidationGraph(sc)
		if err != nil {
			return nil, err
		}
		v.graph = graph
	}
	return v, nil
}

// FastPath reports whether validation runs fully container-native (no map
// materialization).
func (v *ContainerValidator) FastPath() bool { return v != nil && v.unsupported == "" }

// UnsupportedReason names the schema feature that forced the map lane, or ""
// on the fast path.
func (v *ContainerValidator) UnsupportedReason() string {
	if v == nil {
		return ""
	}
	return v.unsupported
}

// Validate checks c against the schema in strict mode.
func (v *ContainerValidator) Validate(c *container.DataContainer) ([]common.Issue, bool) {
	return v.validate(c, ValidationModeStrict)
}

// ValidatePartial checks c, dropping required-field issues (update payloads).
func (v *ContainerValidator) ValidatePartial(c *container.DataContainer) ([]common.Issue, bool) {
	return v.validate(c, ValidationModePartialStrict)
}

// ValidateLoose checks c, dropping required-field and unexpected-field issues.
func (v *ContainerValidator) ValidateLoose(c *container.DataContainer) ([]common.Issue, bool) {
	return v.validate(c, ValidationModeLoose)
}

func (v *ContainerValidator) validate(c *container.DataContainer, mode ValidationMode) ([]common.Issue, bool) {
	if v == nil {
		return nil, true
	}
	if v.unsupported != "" {
		m := cvMaterializeSlot(v.cs, 0, c)
		return v.graph.traverse(v.fmap, m, mode, v.config.MaxDepth, m)
	}

	var issues []common.Issue
	v.validateSlot(v.root, c, nil, &issues)
	issues = cvFilterMode(issues, mode)
	return issues, len(issues) == 0
}

// cvFilterMode applies traverse's mode post-filter: PartialStrict drops
// REQUIRED_FIELD_MISSING, Loose also drops UNEXPECTED_FIELD.
func cvFilterMode(issues []common.Issue, mode ValidationMode) []common.Issue {
	switch mode {
	case ValidationModePartialStrict:
		kept := issues[:0]
		for _, is := range issues {
			if is.Code == "REQUIRED_FIELD_MISSING" {
				continue
			}
			kept = append(kept, is)
		}
		return kept
	case ValidationModeLoose:
		kept := issues[:0]
		for _, is := range issues {
			if is.Code == "REQUIRED_FIELD_MISSING" || is.Code == "UNEXPECTED_FIELD" {
				continue
			}
			kept = append(kept, is)
		}
		return kept
	}
	return issues
}

// cvPathSeg is one segment of an issue path, chained root→leaf. Segments are
// stack-allocated during validation and only materialized into a string when
// an issue fires, so the happy path does no string work at all.
type cvPathSeg struct {
	prev    *cvPathSeg
	rel     string // flattened-object field path (relative to its slot)
	arrPath string // array-of-object field path (items append [i].)
	idx     int
	isArray bool
}

// cvIssuePath materializes the chain plus a leaf's slot-relative path.
func cvIssuePath(seg *cvPathSeg, rel string) string {
	if seg == nil {
		return rel
	}
	depth, s := 0, seg
	for s != nil {
		depth++
		s = s.prev
	}
	buf := make([]byte, 0, 16*depth+len(rel))
	var write func(*cvPathSeg)
	write = func(sg *cvPathSeg) {
		if sg == nil {
			return
		}
		write(sg.prev)
		if sg.isArray {
			buf = append(buf, sg.arrPath...)
			buf = append(buf, '[')
			buf = appendIntBytes(buf, sg.idx)
			buf = append(buf, ']', '.')
			return
		}
		buf = append(buf, sg.rel...)
		buf = append(buf, '.')
	}
	write(seg)
	buf = append(buf, rel...)
	return string(buf)
}

// appendIntBytes appends the decimal form of n (small non-negative indices).
func appendIntBytes(buf []byte, n int) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return append(buf, digits[i:]...)
}

// validateSlot runs the plan for one slot against the container that holds
// its fields. seg is the issue-path chain (nil at the root).
func (v *ContainerValidator) validateSlot(s *cvSlot, c *container.DataContainer, seg *cvPathSeg, issues *[]common.Issue) {
	for i := range s.leaves {
		l := &s.leaves[i]
		pos, ok := c.Position(l.key)
		if !ok {
			if l.required {
				cvRequiredIssue(cvIssuePath(seg, l.path), lastName(l.path), issues)
			}
			continue
		}
		if pos < 0 {
			// Explicit null: legal only on nullable fields (spec §2.7).
			if !l.nullable {
				*issues = append(*issues, failNullNotAllowed(cvIssuePath(seg, l.path)).Issues...)
			}
			continue
		}
		if l.enum != nil {
			v.checkEnum(l, c, seg, issues)
		}
	}

	for i := range s.subs {
		v.validateSub(&s.subs[i], c, seg, issues)
	}
}

// validateSub validates one object-typed field: either the items of an
// array-of-object field (each a child container) or a flattened child schema
// sharing c's address space.
func (v *ContainerValidator) validateSub(sub *cvSub, c *container.DataContainer, seg *cvPathSeg, issues *[]common.Issue) {
	if sub.isArray {
		pos, ok := c.Position(sub.leafKey)
		if !ok {
			if sub.required {
				cvRequiredIssue(cvIssuePath(seg, sub.path), lastName(sub.path), issues)
			}
			return
		}
		if pos < 0 {
			if !sub.nullable {
				*issues = append(*issues, failNullNotAllowed(cvIssuePath(seg, sub.path)).Issues...)
			}
			return
		}
		children, _, err := c.GetArrayObject(sub.leafKey)
		if err != nil {
			return
		}
		childSeg := cvPathSeg{prev: seg, arrPath: sub.path, isArray: true}
		for i, ch := range children {
			childSeg.idx = i
			v.validateSlot(sub.child, ch, &childSeg, issues)
		}
		return
	}

	// Flattened object: the container has no slot for the object itself, so
	// presence means any descendant slot is set — matching the document
	// layer's anyDescendantPresent and the materializer's omission of empty
	// objects.
	if !cvHasPresence(sub.child, c) {
		if sub.required {
			cvRequiredIssue(cvIssuePath(seg, sub.path), lastName(sub.path), issues)
		}
		return
	}
	childSeg := cvPathSeg{prev: seg, rel: sub.path}
	v.validateSlot(sub.child, c, &childSeg, issues)
}

// cvRequiredIssue appends the map lane's RequiredFieldNode issue.
func cvRequiredIssue(path, name string, issues *[]common.Issue) {
	*issues = append(*issues, common.Issue{
		Code:    "REQUIRED_FIELD_MISSING",
		Message: fmt.Sprintf("Required field '%s' is missing", name),
		Path:    path,
	})
}

// cvHasPresence reports whether any leaf under s is set (value or explicit
// null) in c, recursively through flattened object children.
func cvHasPresence(s *cvSlot, c *container.DataContainer) bool {
	for i := range s.leaves {
		if _, ok := c.Position(s.leaves[i].key); ok {
			return true
		}
	}
	for i := range s.subs {
		sub := &s.subs[i]
		if sub.isArray {
			if _, ok := c.Position(sub.leafKey); ok {
				return true
			}
			continue
		}
		if cvHasPresence(sub.child, c) {
			return true
		}
	}
	return false
}

// checkEnum evaluates a leaf's enum against its typed slot value. The value
// is already of the slot's type, so the map lane's scalar pre-check cannot
// fail here; the comparison is a linear scan over the declared values (enums
// are small), which avoids boxing the slot value into an interface.
func (v *ContainerValidator) checkEnum(l *cvLeaf, c *container.DataContainer, seg *cvPathSeg, issues *[]common.Issue) {
	var violation bool
	switch l.dt {
	case container.TypeString:
		s, ok, _ := c.GetString(l.key)
		if !ok {
			return
		}
		violation = true
		for _, allowed := range l.enum.strs {
			if s == allowed {
				violation = false
				break
			}
		}
	case container.TypeInt:
		n, ok, _ := c.GetInt(l.key)
		if !ok {
			return
		}
		violation = true
		for _, allowed := range l.enum.ints {
			if n == allowed {
				violation = false
				break
			}
		}
	default:
		return
	}

	if !violation {
		return
	}
	var value any
	if l.dt == container.TypeString {
		s, _, _ := c.GetString(l.key)
		value = s
	} else {
		n, _, _ := c.GetInt(l.key)
		value = n
	}
	*issues = append(*issues, common.Issue{
		Code:    "ENUM_VIOLATION",
		Message: fmt.Sprintf("Value %v is not in the allowed list", value),
		Path:    cvIssuePath(seg, l.path),
	})
}

// =============================================================================
// PLAN COMPILATION
// =============================================================================

type cvBuilder struct {
	cs          *CompiledSchema
	unsupported string
}

// buildSlot compiles one schema slot's fields into leaf checks and child
// plans. prefix is the ResolvedPath leading to this slot (nil at the root),
// basePath its dotted form for unsupported-feature messages, and depth the
// number of ancestor path segments (root 0) for MaxDepth accounting: flattened
// object children extend both, array-of-object item slots restart the address
// prefix and paths but still nest one level deeper.
func (b *cvBuilder) buildSlot(slotIdx uint8, prefix ResolvedPath, basePath string, depth int) (*cvSlot, int, error) {
	if int(slotIdx) >= len(b.cs.Schemas) {
		return nil, 0, ErrInvalidSchema.WithMessagef("schema slot %d out of range", slotIdx)
	}
	slot := &b.cs.Schemas[slotIdx]
	s := &cvSlot{idx: slotIdx}
	maxPath := 0

	if len(b.cs.SchemaConstraints[slotIdx]) > 0 || len(b.cs.Constraints) > 0 {
		b.unsupported = firstNonEmpty(b.unsupported, "schema-level cross-field constraints")
	}

	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := b.cs.Descriptors[abs]
		name := b.cs.FieldsMeta[abs].Name
		ft := b.cs.FieldTypes[abs]
		path := name
		if basePath != "" {
			path = basePath + "." + name
		}

		step, _, err := b.cs.ResolveFieldStep(slotIdx, name)
		if err != nil {
			return nil, 0, err
		}
		rp := append(append(ResolvedPath(nil), prefix...), step)

		dp := fd.DataPoint()
		if len(b.cs.Variants[dp]) > 0 {
			b.unsupported = firstNonEmpty(b.unsupported, "union/composite field '"+path+"'")
		}
		if _, ok := b.cs.FieldRefConstraints[dp]; ok {
			b.unsupported = firstNonEmpty(b.unsupported, "field constraint on '"+path+"'")
		}

		switch {
		case fd.Terminal():
			leaf, ok := b.buildLeaf(fd, ft, rp, name)
			if ok {
				s.leaves = append(s.leaves, leaf)
				if depth+1 > maxPath {
					maxPath = depth + 1
				}
			}

		case fd.DataType() == container.TypeArrayObject:
			// Items are child containers, but their fields are addressed by
			// the full path from the root (the decoder stores "items.sku"
			// inside the child container), so the item plan keeps the full
			// address prefix while its issue paths stay item-relative.
			child, childMax, err := b.buildSlot(fd.ChildSchemaIdx(), rp, "", depth+1)
			if err != nil {
				return nil, 0, err
			}
			if childMax > maxPath {
				maxPath = childMax
			}
			s.subs = append(s.subs, cvSub{
				leafKey:  cvInternalKey(fd),
				isArray:  true,
				path:     name,
				required: fd.Required(),
				nullable: fd.Nullable(),
				child:    child,
			})

		case fd.Kind() == KindObject && fd.DataType() != container.TypeRecord && fd.ChildSchemaIdx() != FdNoChild:
			// Flattened object (or composite): children share the parent's
			// flat address space.
			child, childMax, err := b.buildSlot(fd.ChildSchemaIdx(), rp, path, depth+1)
			if err != nil {
				return nil, 0, err
			}
			if childMax > maxPath {
				maxPath = childMax
			}
			s.subs = append(s.subs, cvSub{
				path:     name,
				required: fd.Required(),
				nullable: fd.Nullable(),
				child:    child,
			})

		default:
			// Records (map payloads), unions, geometry and anything else the
			// typed plan cannot read natively validate through the map lane.
			b.unsupported = firstNonEmpty(b.unsupported, "container-typed field '"+path+"'")
		}
	}

	s.maxPath = maxPath
	return s, maxPath, nil
}

// buildLeaf compiles a terminal field; name is the leaf's slot-relative path
// (issue paths are prefixed at validate time). ok is false when the leaf
// validates on the map lane (its plan entry is skipped).
func (b *cvBuilder) buildLeaf(fd FieldDescriptor, ft FieldType, rp ResolvedPath, name string) (cvLeaf, bool) {
	dt := fd.DataType()
	leaf := cvLeaf{
		path:     name,
		dt:       dt,
		required: fd.Required(),
		nullable: fd.Nullable(),
	}

	switch dt {
	case container.TypeString, container.TypeInt, container.TypeFloat, container.TypeBool, container.TypeBytes,
		container.TypeArrayString, container.TypeArrayInt, container.TypeArrayFloat, container.TypeArrayBool, container.TypeArrayBytes:
		// typed leaves this lane reads natively
	default:
		b.unsupported = firstNonEmpty(b.unsupported, "leaf type on '"+name+"'")
		return leaf, false
	}

	addr := b.cs.Address(rp)
	if addr == 0 {
		b.unsupported = firstNonEmpty(b.unsupported, "unaddressable leaf '"+name+"'")
		return leaf, false
	}
	dp, err := container.NewDataPoint(dt, int32(addr))
	if err != nil {
		b.unsupported = firstNonEmpty(b.unsupported, "address overflow on '"+name+"'")
		return leaf, false
	}
	leaf.key = container.NewDataContainerKey(dp, uint32(fd))

	// Enums: native only when the compiled enum storage's value type matches
	// the slot's value type, so lookup comparisons are exact. Numeric enums
	// (integer or number) store int64; string enums store string. Everything
	// else (bool slots, complex/mixed enums, enum'd arrays) falls back to the
	// map lane rather than risk a semantic divergence.
	if ft == FieldTypeEnum && dt == container.TypeString {
		leaf.enum = b.buildEnum(fd, cvEnumString)
	} else if ft == FieldTypeEnum && dt == container.TypeInt {
		leaf.enum = b.buildEnum(fd, cvEnumInt)
	}
	if leaf.enum == nil && cvStoresEnum(b.cs, fd) {
		b.unsupported = firstNonEmpty(b.unsupported, "enum storage mismatch on '"+name+"'")
	}

	return leaf, true
}

type cvEnumKind uint8

const (
	cvEnumString cvEnumKind = iota
	cvEnumInt
)

// buildEnum reads the compiled enum values for fd out of cs.Enums (written by
// setEnumValues, keyed by the field's internal DataPoint) and builds the
// lookup. Returns nil when the field has no enum. A stored enum whose storage
// kind does not match the expected one marks the schema unsupported instead
// of guessing.
func (b *cvBuilder) buildEnum(fd FieldDescriptor, kind cvEnumKind) *cvEnum {
	id := int32(fd.DataPoint()) >> 5
	probe := func(dt container.DataType) ([]any, bool) {
		edp, err := container.NewDataPoint(dt, id)
		if err != nil {
			return nil, false
		}
		ek := container.NewDataContainerKey(edp, 0)
		switch dt {
		case container.TypeArrayString:
			vals, ok, err := b.cs.Enums.GetArrayString(ek)
			if err != nil || !ok {
				return nil, false
			}
			out := make([]any, len(vals))
			for i, v := range vals {
				out[i] = v
			}
			return out, true
		case container.TypeArrayInt:
			vals, ok, err := b.cs.Enums.GetArrayInt(ek)
			if err != nil || !ok {
				return nil, false
			}
			out := make([]any, len(vals))
			for i, v := range vals {
				out[i] = v
			}
			return out, true
		default:
			vals, ok, err := b.cs.Enums.GetArrayUnknown(ek)
			if err != nil || !ok {
				return nil, false
			}
			return vals, true
		}
	}

	expected := container.TypeArrayString
	if kind == cvEnumInt {
		expected = container.TypeArrayInt
	}
	vals, ok := probe(expected)
	if !ok {
		for _, other := range []container.DataType{container.TypeArrayString, container.TypeArrayInt, container.TypeArrayUnknown} {
			if other == expected {
				continue
			}
			if _, found := probe(other); found {
				b.unsupported = firstNonEmpty(b.unsupported, "enum storage mismatch")
				return nil
			}
		}
		return nil
	}

	e := &cvEnum{}
	for _, v := range vals {
		switch tv := v.(type) {
		case string:
			if kind == cvEnumInt {
				b.unsupported = firstNonEmpty(b.unsupported, "mixed enum values")
				return nil
			}
			e.strs = append(e.strs, tv)
		case int64:
			if kind == cvEnumString {
				b.unsupported = firstNonEmpty(b.unsupported, "mixed enum values")
				return nil
			}
			e.ints = append(e.ints, tv)
		default:
			b.unsupported = firstNonEmpty(b.unsupported, "complex enum values")
			return nil
		}
	}
	return e
}

// cvStoresEnum reports whether any enum values are stored for fd.
func cvStoresEnum(cs *CompiledSchema, fd FieldDescriptor) bool {
	id := int32(fd.DataPoint()) >> 5
	for _, dt := range []container.DataType{container.TypeArrayString, container.TypeArrayInt, container.TypeArrayUnknown} {
		edp, err := container.NewDataPoint(dt, id)
		if err != nil {
			continue
		}
		ek := container.NewDataContainerKey(edp, 0)
		switch dt {
		case container.TypeArrayString:
			if _, ok, err := cs.Enums.GetArrayString(ek); err == nil && ok {
				return true
			}
		case container.TypeArrayInt:
			if _, ok, err := cs.Enums.GetArrayInt(ek); err == nil && ok {
				return true
			}
		default:
			if _, ok, err := cs.Enums.GetArrayUnknown(ek); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// cvInternalKey builds the structural key for non-flattened fields (records,
// array-of-object), matching binding.InternalKey.
func cvInternalKey(fd FieldDescriptor) container.DataContainerKey {
	return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd))
}

func lastName(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// =============================================================================
// MAP LANE (fallback): materialization
// =============================================================================

// Materialize rebuilds a container's document as map[string]any — the same
// semantics as the document layer's ToMap (null leaves present with nil
// values, records materialize their stored map, array-of-object fields
// materialize each child container, empty flattened objects omitted). It is
// the map-lane input for schemas that take the fallback lane, and the
// equivalence anchor for the differential tests.
func Materialize(cs *CompiledSchema, c *container.DataContainer) map[string]any {
	return cvMaterializeSlotPrefix(cs, 0, c, nil)
}

// cvMaterializeSlot rebuilds a slot's document as map[string]any for the
// fallback lane. Semantics match the document layer's ToMap: null leaves are
// present with nil values, records materialize their stored map,
// array-of-object fields materialize each child container, and empty
// flattened objects are omitted (indistinguishable from absent in the
// container).
func cvMaterializeSlot(cs *CompiledSchema, slotIdx uint8, c *container.DataContainer) map[string]any {
	return cvMaterializeSlotPrefix(cs, slotIdx, c, nil)
}

func cvMaterializeSlotPrefix(cs *CompiledSchema, slotIdx uint8, c *container.DataContainer, prefix ResolvedPath) map[string]any {
	if c == nil {
		return map[string]any{}
	}
	slot := &cs.Schemas[slotIdx]
	m := make(map[string]any, slot.FieldCount)

	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		name := cs.FieldsMeta[abs].Name
		dt := fd.DataType()

		step, _, err := cs.ResolveFieldStep(slotIdx, name)
		if err != nil {
			continue
		}
		rp := append(append(ResolvedPath(nil), prefix...), step)

		switch {
		case dt == container.TypeRecord:
			k := cvInternalKey(fd)
			if _, ok := c.Position(k); !ok {
				continue
			}
			if c.IsNull(k) {
				m[name] = nil
				continue
			}
			if rec, ok, err := c.GetRecord(k); err == nil && ok {
				m[name] = rec
			}

		case dt == container.TypeArrayObject:
			k := cvInternalKey(fd)
			if _, ok := c.Position(k); !ok {
				continue
			}
			if c.IsNull(k) {
				m[name] = nil
				continue
			}
			children, _, err := c.GetArrayObject(k)
			if err != nil {
				continue
			}
			arr := make([]any, len(children))
			for i, ch := range children {
				arr[i] = cvMaterializeSlotPrefix(cs, fd.ChildSchemaIdx(), ch, rp)
			}
			m[name] = arr

		case fd.Terminal():
			if val, ok := cvGetAny(cs, c, fd, dt, rp); ok {
				m[name] = val
			}

		case fd.Kind() == KindObject && fd.ChildSchemaIdx() != FdNoChild:
			sub := cvMaterializeSlotPrefix(cs, fd.ChildSchemaIdx(), c, rp)
			if len(sub) > 0 {
				m[name] = sub
			}

		default:
			// Unions and other exotic types: read boxed, best effort.
			if val, ok := cvGetAny(cs, c, fd, dt, rp); ok {
				m[name] = val
			}
		}
	}
	return m
}

// cvGetAny reads a terminal slot as any, honoring the null tri-state.
func cvGetAny(cs *CompiledSchema, c *container.DataContainer, fd FieldDescriptor, dt container.DataType, rp ResolvedPath) (any, bool) {
	k := cvTerminalKey(cs, fd, dt, rp)
	switch dt {
	case container.TypeString:
		v, ok, _ := c.GetString(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeInt:
		v, ok, _ := c.GetInt(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeFloat:
		v, ok, _ := c.GetFloat(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeBool:
		v, ok, _ := c.GetBool(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeBytes:
		v, ok, _ := c.GetBytes(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeGeometry:
		v, ok, _ := c.GetGeometry(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayString:
		v, ok, _ := c.GetArrayString(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayInt:
		v, ok, _ := c.GetArrayInt(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayFloat:
		v, ok, _ := c.GetArrayFloat(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayBool:
		v, ok, _ := c.GetArrayBool(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayBytes:
		v, ok, _ := c.GetArrayBytes(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeArrayGeometry:
		v, ok, _ := c.GetArrayGeometry(k)
		return boxedNull(ok, c.IsNull(k), v)
	case container.TypeUnknown:
		v, ok, _ := c.GetUnknown(k)
		return boxedNull(ok, c.IsNull(k), v)
	}
	return nil, false
}

func boxedNull(ok, isNull bool, v any) (any, bool) {
	if !ok {
		return nil, false
	}
	if isNull {
		return nil, true
	}
	return v, true
}

// cvTerminalKey computes the flat user-data key for a terminal descriptor
// from its resolved path.
func cvTerminalKey(cs *CompiledSchema, fd FieldDescriptor, dt container.DataType, rp ResolvedPath) container.DataContainerKey {
	addr := cs.Address(rp)
	if addr == 0 {
		return cvInternalKey(fd)
	}
	dp, err := container.NewDataPoint(dt, int32(addr))
	if err != nil {
		return cvInternalKey(fd)
	}
	return container.NewDataContainerKey(dp, uint32(fd))
}

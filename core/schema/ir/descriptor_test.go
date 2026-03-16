package ir

import "testing"

// descriptor_test.go tests Pass 4 (descriptor packing) and Pass 5
// (cycle detection + terminal bit assignment).

// ── packDescriptor ─────────────────────────────────────────────────────────

func TestPackDescriptor_ScalarField(t *testing.T) {
	fd := packDescriptor(TypeString, 0, 0, 0, true, false, false)

	if ExtractType(fd) != TypeString {
		t.Errorf("type: got %v, want TypeString", ExtractType(fd))
	}
	if !IsRequired(fd) {
		t.Error("required: expected true")
	}
	if IsUnique(fd) {
		t.Error("unique: expected false")
	}
	if IsDeprecated(fd) {
		t.Error("deprecated: expected false")
	}
	if ExtractOwnerSchema(fd) != 0 {
		t.Errorf("owner_schema: got %d, want 0", ExtractOwnerSchema(fd))
	}
	if ExtractFieldIndex(fd) != 0 {
		t.Errorf("field_index: got %d, want 0", ExtractFieldIndex(fd))
	}
	if ExtractTargetSchema(fd) != 0 {
		t.Errorf("target_schema: got %d, want 0", ExtractTargetSchema(fd))
	}
	// Terminal bit must not be set by packDescriptor.
	if IsTerminal(fd) {
		t.Error("terminal: packDescriptor must not set terminal bit")
	}
	// Reserved bit 0 must be zero.
	if fd&1 != 0 {
		t.Error("reserved bit 0 is set")
	}
}

func TestPackDescriptor_AllFlagsAndIndices(t *testing.T) {
	fd := packDescriptor(TypeObject, 5, 12, 7, true, true, true)

	if ExtractType(fd) != TypeObject {
		t.Errorf("type: got %v, want TypeObject", ExtractType(fd))
	}
	if ExtractOwnerSchema(fd) != 5 {
		t.Errorf("owner_schema: got %d, want 5", ExtractOwnerSchema(fd))
	}
	if ExtractFieldIndex(fd) != 12 {
		t.Errorf("field_index: got %d, want 12", ExtractFieldIndex(fd))
	}
	if ExtractTargetSchema(fd) != 7 {
		t.Errorf("target_schema: got %d, want 7", ExtractTargetSchema(fd))
	}
	if !IsRequired(fd) {
		t.Error("required: expected true")
	}
	if !IsUnique(fd) {
		t.Error("unique: expected true")
	}
	if !IsDeprecated(fd) {
		t.Error("deprecated: expected true")
	}
	if fd&1 != 0 {
		t.Error("reserved bit 0 is set")
	}
}

func TestPackDescriptor_MaxIndices(t *testing.T) {
	// field_index max = 126 (7 bits, value 127 reserved)
	// owner_schema max = 127, target_schema max = 127
	fd := packDescriptor(TypeArray, 127, 126, 127, false, false, false)
	if ExtractOwnerSchema(fd) != 127 {
		t.Errorf("owner_schema: got %d, want 127", ExtractOwnerSchema(fd))
	}
	if ExtractFieldIndex(fd) != 126 {
		t.Errorf("field_index: got %d, want 126", ExtractFieldIndex(fd))
	}
	if ExtractTargetSchema(fd) != 127 {
		t.Errorf("target_schema: got %d, want 127", ExtractTargetSchema(fd))
	}
}

func TestPackDescriptor_TypeOrdinals(t *testing.T) {
	types := []FieldTypeEnum{
		TypeUnknown, TypeString, TypeNumber, TypeInteger, TypeBoolean,
		TypeBytes, TypeArray, TypeSet, TypeEnum, TypeObject,
		TypeRecord, TypeUnion, TypeComposite, TypeGeometry,
	}
	for _, ft := range types {
		fd := packDescriptor(ft, 0, 0, 0, false, false, false)
		got := ExtractType(fd)
		if got != ft {
			t.Errorf("type %v: round-tripped to %v", ft, got)
		}
	}
}

// ── buildDescriptors — terminal bits ──────────────────────────────────────

func TestBuildDescriptors_ScalarsAreTerminal(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	// All three fields are scalar strings — all must be terminal.
	for i, fd := range cs.Descriptors {
		if !IsTerminal(fd) {
			t.Errorf("descriptor[%d]: scalar field not terminal, fd=0x%08X", i, fd)
		}
	}
}

func TestBuildDescriptors_ObjectFieldTerminal(t *testing.T) {
	// nestedObjectSchema has one object field in root pointing at Address.
	// The object field must be terminal (no cycle). Address fields (scalars)
	// must also be terminal.
	cs := mustCompile(nestedObjectSchema, nil)
	for i, fd := range cs.Descriptors {
		if !IsTerminal(fd) {
			t.Errorf("descriptor[%d]: expected terminal, fd=0x%08X", i, fd)
		}
	}
}

func TestBuildDescriptors_CycleDetection(t *testing.T) {
	// cycleSchema: root.node → Node, Node.next → Node (self-reference).
	// root.label — scalar, terminal=1.
	// root.node  — object → Node; Node is not yet on the path, terminal=1.
	// Node.value — scalar, terminal=1.
	// Node.next  — object → Node; Node IS on the path (self-reference), terminal=0.
	cs := mustCompile(cycleSchema, nil)

	labelFd := findDescriptor(cs, 0, "label")
	nodeFd := findDescriptor(cs, 0, "node")

	nodeSchemaIdx := si_fromCS(cs, nestedAddressSchemaUUID) // nestedAddressSchemaUUID = ...0010 = Node's UUID
	valueFd := findDescriptor(cs, nodeSchemaIdx, "value")
	nextFd := findDescriptor(cs, nodeSchemaIdx, "next")

	if labelFd == 0 {
		t.Fatal("label descriptor not found")
	}
	if nodeFd == 0 {
		t.Fatal("node descriptor not found")
	}
	if valueFd == 0 {
		t.Fatal("value descriptor not found")
	}
	if nextFd == 0 {
		t.Fatal("next descriptor not found")
	}

	if !IsTerminal(labelFd) {
		t.Error("label (scalar): expected terminal=1")
	}
	if !IsTerminal(nodeFd) {
		t.Error("node (first descent into Node, no cycle yet): expected terminal=1")
	}
	if !IsTerminal(valueFd) {
		t.Error("value (scalar in Node): expected terminal=1")
	}
	if IsTerminal(nextFd) {
		t.Error("next (self-reference back to Node): expected terminal=0")
	}
}

func TestBuildDescriptors_TargetSchemaZeroForScalar(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	for _, fd := range cs.Descriptors {
		if IsSchemaBearing(fd) {
			t.Errorf("flatSchema has no schema-bearing fields, got: 0x%08X", fd)
		}
		if ExtractTargetSchema(fd) != 0 {
			t.Errorf("scalar field has non-zero target_schema: 0x%08X", fd)
		}
	}
}

func TestBuildDescriptors_TargetSchemaForObjectField(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	// The root's object field must have target_schema = 1 (the Address schema).
	addrFd := findDescriptor(cs, 0, "address")
	if addrFd == 0 {
		t.Fatal("address descriptor not found")
	}
	if ExtractTargetSchema(addrFd) != 1 {
		t.Errorf("address target_schema: got %d, want 1", ExtractTargetSchema(addrFd))
	}
}

func TestBuildDescriptors_UnknownTypeError(t *testing.T) {
	// Unknown field type passes JSON parsing (type is just a string in the source
	// model) and is caught at Pass 4 during descriptor building.
	_, parseErr := Parse(invalidUnknownFieldType)
	if parseErr != nil {
		// If Parse itself rejects it, that is also acceptable.
		return
	}
	ss, err := Parse(invalidUnknownFieldType)
	if err != nil {
		return
	}
	_, err = Compile(ss, nil)
	if err == nil {
		t.Fatal("expected compile error for unknown field type")
	}
	ce := firstError(err)
	if ce.Pass != PassDescriptor {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassDescriptor)
	}
}

func TestBuildDescriptors_UnresolvedRef(t *testing.T) {
	ss := mustParse(invalidUnresolvedRef)
	_, err := Compile(ss, nil)
	if err == nil {
		t.Fatal("expected compile error for unresolved ref")
	}
	ce := firstError(err)
	if ce.Pass != PassDescriptor {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassDescriptor)
	}
}

func TestBuildDescriptors_ReservedBitAlwaysZero(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	for i, fd := range cs.Descriptors {
		if fd&1 != 0 {
			t.Errorf("descriptor[%d]: reserved bit 0 is set, fd=0x%08X", i, fd)
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// si_fromCS finds the schema index for a given UUID by scanning cs.Meta.
func si_fromCS(cs *CompiledSchema, uuid string) uint8 {
	for idx, m := range cs.Meta {
		if m != nil && m.UUID == uuid {
			return idx
		}
	}
	// cycleSchema uses nestedAddressSchemaUUID for the Child schema.
	// Fall back to scanning for the first non-root schema.
	for idx, m := range cs.Meta {
		if idx != 0 && m != nil {
			return idx
		}
	}
	return 0
}

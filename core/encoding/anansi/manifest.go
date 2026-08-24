package anansi

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Manifest is the language-agnostic descriptor a client codec consumes in
// place of a compiled schema: the root slot's fields in canonical wire
// order (declaration order, flattened-object children inlined), each with
// its dotted path, wire type tag and the exact uint32 DataPoint the Sparse
// format writes. TypeArrayObject fields additionally carry their child
// fields fully resolved for their mount site — shared sub-schemas appear
// once per mount because addresses (and therefore Sparse DataPoints) are
// mount-dependent.
//
// The wire format itself never appears here; manifests describe structure,
// codecs describe bytes. See clients/ts for the reference consumer.
type Manifest struct {
	Version uint16          `json:"version"`
	Root    []ManifestField `json:"root"`
}

// ManifestField is one wire-addressable field. Child is populated only for
// TypeArrayObject fields and lists each element schema's fields resolved
// under this field's mount path.
type ManifestField struct {
	Name  string          `json:"name"`
	Path  string          `json:"path"`
	T     string          `json:"t"`
	DP    uint32          `json:"dp"`
	Child []ManifestField `json:"child,omitempty"`
}

var manifestTypeNames = map[container.DataType]string{
	container.TypeUnknown:       "unknown",
	container.TypeInt:           "int",
	container.TypeFloat:         "float",
	container.TypeString:        "string",
	container.TypeBool:          "bool",
	container.TypeBytes:         "bytes",
	container.TypeGeometry:      "geometry",
	container.TypeRecord:        "record",
	container.TypeArrayUnknown:  "array_unknown",
	container.TypeArrayInt:      "array_int",
	container.TypeArrayFloat:    "array_float",
	container.TypeArrayString:   "array_string",
	container.TypeArrayBool:     "array_bool",
	container.TypeArrayBytes:    "array_bytes",
	container.TypeArrayObject:   "array_object",
	container.TypeArrayGeometry: "array_geometry",
}

// ExportManifest renders cs as a deterministic JSON manifest for the given
// fullVersion.
func ExportManifest(cs *definition.CompiledSchema, fullVersion uint16) ([]byte, error) {
	if cs == nil || len(cs.Schemas) == 0 {
		return nil, fmt.Errorf("anansi: ExportManifest: nil or empty compiled schema")
	}
	root, err := exportSlot(cs, rootSlot, nil)
	if err != nil {
		return nil, err
	}
	m := Manifest{Version: fullVersion, Root: root}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("anansi: ExportManifest: %w", err)
	}
	return out, nil
}

// exportSlot walks schema slot idx under prefix, emitting fields in the
// canonical wire order used by the packet encoders.
func exportSlot(cs *definition.CompiledSchema, idx uint8, prefix definition.ResolvedPath) ([]ManifestField, error) {
	slot := cs.Schemas[idx]
	out := make([]ManifestField, 0, slot.FieldCount)
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		fieldPath := append(append(definition.ResolvedPath{}, prefix...),
			definition.NewResolvedStep(idx, uint8(j)))

		mf := ManifestField{
			Name: cs.FieldsMeta[abs].Name,
			Path: cs.PathString(fieldPath),
			T:    manifestTypeNames[fd.DataType()],
		}

		switch {
		case fd.Terminal():
			key, err := computeLeafKey(cs, fd, fieldPath)
			if err != nil {
				return nil, fmt.Errorf("anansi: ExportManifest: field %q: %w", mf.Name, err)
			}
			mf.DP = uint32(key.DataPoint())
		case fd.DataType() == container.TypeArrayObject:
			mf.DP = uint32(internalKey(fd).DataPoint())
			child, err := exportSlot(cs, fd.ChildSchemaIdx(), fieldPath)
			if err != nil {
				return nil, err
			}
			mf.Child = child
		case fd.ChildSchemaIdx() == definition.FdNoChild:
			continue
		default:
			// Flattened object/union/composite/recursive field: owns no
			// storage; descendants are emitted in its place.
			children, err := exportSlot(cs, fd.ChildSchemaIdx(), fieldPath)
			if err != nil {
				return nil, err
			}
			out = append(out, children...)
			continue
		}
		out = append(out, mf)
	}
	return out, nil
}

// CompileDumpField mirrors one linked field for cross-language parity
// testing: everything a re-implementation must reproduce bit-exactly.
type CompileDumpField struct {
	Slot        int    `json:"slot"`
	Idx         int    `json:"idx"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	DT          uint8  `json:"dt"`
	Kind        uint8  `json:"kind"`
	Terminal    bool   `json:"terminal"`
	DP          uint32 `json:"dp"`
	Descriptor  uint32 `json:"descriptor"`
	LocalOffset uint32 `json:"localOffset"`
	Address     uint32 `json:"address"`
}

// CompileDumpSlot is one schema slot table entry.
type CompileDumpSlot struct {
	FieldStart int `json:"fieldStart"`
	FieldCount int `json:"fieldCount"`
	Footprint  int `json:"footprint"`
}

// ExportCompileDump renders the linked schema's internals — descriptors,
// offsets, addresses — as deterministic JSON for TypeScript parity tests.
func ExportCompileDump(cs *definition.CompiledSchema) ([]byte, error) {
	if cs == nil {
		return nil, fmt.Errorf("anansi: ExportCompileDump: nil compiled schema")
	}

	slots := make([]CompileDumpSlot, len(cs.Schemas))
	for i, s := range cs.Schemas {
		slots[i] = CompileDumpSlot{FieldStart: int(s.FieldStart), FieldCount: int(s.FieldCount), Footprint: int(s.Footprint)}
	}

	wireDPs, probes := buildWireIndex(cs)

	fields := make([]CompileDumpField, 0, len(cs.FieldsMeta))
	for abs := range cs.FieldsMeta {
		fd := cs.Descriptors[abs]
		f := CompileDumpField{
			Slot:        int(fd.SchemaIdx()),
			Idx:         int(fd.FieldIdx()),
			Path:        cs.FieldsMeta[abs].Path,
			Name:        cs.FieldsMeta[abs].Name,
			DT:          uint8(fd.DataType()),
			Kind:        uint8(fd.Kind()),
			Terminal:    fd.Terminal(),
			DP:          wireDPs[abs],
			Descriptor:  uint32(fd),
			LocalOffset: cs.LocalOffsets[abs],
			Address:     0,
		}
		if fd.Terminal() && fd.SchemaIdx() == rootSlot {
			rp, err := cs.ResolvePath(cs.FieldsMeta[abs].Path)
			if err == nil {
				f.Address = cs.Address(rp)
			}
		}
		fields = append(fields, f)
	}

	dump := struct {
		Version uint16             `json:"version"`
		Slots   []CompileDumpSlot  `json:"slots"`
		Fields  []CompileDumpField `json:"fields"`
		Probes  []CompileDumpProbe `json:"probes"`
	}{Version: uint16(definition.CompiledSchemaFormatVersion), Slots: slots, Fields: fields, Probes: probes}
	out, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CompileDumpProbe is an explicit address probe: full dotted mount path →
// user-data address, exercising multi-step addressing across mounts.
type CompileDumpProbe struct {
	Path  string `json:"path"`
	Addr  uint32 `json:"addr"`
}


// buildWireIndex walks every mount site exactly like collectWireFields,
// producing (a) the canonical Sparse DataPoint per absolute field index and
// (b) full-path user-data address probes for cross-language parity.
func buildWireIndex(cs *definition.CompiledSchema) (map[int]uint32, []CompileDumpProbe) {
	dps := map[int]uint32{}
	probes := []CompileDumpProbe{}

	var walk func(idx uint8, prefix definition.ResolvedPath, dot string)
	walk = func(idx uint8, prefix definition.ResolvedPath, dot string) {
		slot := cs.Schemas[idx]
		for j := uint16(0); j < slot.FieldCount; j++ {
			abs := int(slot.FieldStart) + int(j)
			fd := cs.Descriptors[abs]
			step := definition.NewResolvedStep(idx, uint8(j))
			fp := append(append(definition.ResolvedPath{}, prefix...), step)
			name := cs.FieldsMeta[abs].Name
			full := name
			if dot != "" {
				full = dot + "." + name
			}

			key, err := computeLeafKeyExported(cs, fd, fp)
			if err == nil {
				dps[abs] = uint32(key.DataPoint())
			}

			if fd.Terminal() {
				probes = append(probes, CompileDumpProbe{Path: full, Addr: cs.Address(fp)})
				continue
			}
			if fd.ChildSchemaIdx() == definition.FdNoChild {
				continue
			}
			walk(fd.ChildSchemaIdx(), fp, full)
		}
	}
	walk(rootSlot, nil, "")
	return dps, probes
}

// computeLeafKeyExported bridges the unexported codec helper for the dump
// walk; identical math, exported purely for parity tooling in this package.
func computeLeafKeyExported(cs *definition.CompiledSchema, fd definition.FieldDescriptor, path definition.ResolvedPath) (container.DataContainerKey, error) {
	return computeLeafKey(cs, fd, path)
}

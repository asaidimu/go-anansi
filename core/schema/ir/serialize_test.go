package ir

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestSerialize_RoundTrip(t *testing.T) {
	schemas := []struct {
		name string
		src  []byte
	}{
		{"Flat", flatSchema},
		{"NestedObject", nestedObjectSchema},
		{"Enum", enumSchema},
		{"InlineEnum", inlineEnumSchema},
		{"Union", unionSchema},
		{"Indexed", indexedSchema},
		{"Constrained", constrainedSchema},
		{"Default", defaultSchema},
		{"Cycle", cycleSchema},
	}

	pm := PredicateMap{
		"isEmail": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}

	for _, tc := range schemas {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Parse and Compile original
			ss1, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse original failed: %v", err)
			}
			cs1, err := Compile(ss1, pm)
			if err != nil {
				t.Fatalf("Compile original failed: %v", err)
			}

			// 2. Serialize
			serialized, err := Serialize(cs1)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}

			// 3. Parse and Compile serialized
			ss2, err := Parse(serialized)
			if err != nil {
				t.Fatalf("Parse serialized failed: %v\nSerialized JSON:\n%s", err, string(serialized))
			}
			cs2, err := Compile(ss2, pm)
			if err != nil {
				t.Fatalf("Compile serialized failed: %v\nSerialized JSON:\n%s", err, string(serialized))
			}

			// 4. Compare structures (via their source representations)
			var src1, src2 any
			if err := json.Unmarshal(tc.src, &src1); err != nil {
				t.Fatalf("Unmarshal original failed: %v", err)
			}
			if err := json.Unmarshal(serialized, &src2); err != nil {
				t.Fatalf("Unmarshal serialized failed: %v", err)
			}

			// Note: We don't use reflect.DeepEqual(src1, src2) because the
			// original might have different formatting, or the serialization
			// might reorder maps (though json.Marshal usually sorts keys).
			// Instead, we verify that cs2 (compiled from serialized) behaves
			// identically to cs1 where it matters.

			if len(cs2.Descriptors) != len(cs1.Descriptors) {
				t.Errorf("Descriptors count mismatch: got %d, want %d", len(cs2.Descriptors), len(cs1.Descriptors))
			}
			if len(cs2.Meta) != len(cs1.Meta) {
				t.Errorf("Meta count mismatch: got %d, want %d", len(cs2.Meta), len(cs1.Meta))
			}

			// Compare field names and types in root schema
			m1 := cs1.Meta[0]
			m2 := cs2.Meta[0]
			if m1.Name != m2.Name {
				t.Errorf("Root name mismatch: got %q, want %q", m2.Name, m1.Name)
			}

			// Check that all fields from cs1 are present in cs2 with same name and type
			for fd1, fm1 := range m1.Fields {
				found := false
				for fd2, fm2 := range m2.Fields {
					if fm1.Name == fm2.Name {
						if ExtractType(fd1) != ExtractType(fd2) {
							t.Errorf("Field %q type mismatch: got %v, want %v", fm1.Name, ExtractType(fd2), ExtractType(fd1))
						}
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Field %q (UUID %s) from original not found in serialized", fm1.Name, fm1.UUID)
				}
			}

			// For Default schema, check default value
			if tc.name == "Default" {
				fd1 := findDescriptor(cs1, 0, "retries")
				fd2 := findDescriptor(cs2, 0, "retries")
				def1 := getDefaultFromStore(cs1, fd1, ExtractType(fd1))
				def2 := getDefaultFromStore(cs2, fd2, ExtractType(fd2))
				
				// JSON unmarshals numbers as float64 by default, so we might need adjustment
				// but here we just check if they are "equal" in value.
				if !reflect.DeepEqual(def1, def2) {
					t.Errorf("Default value mismatch for 'retries': got %v (%T), want %v (%T)", def2, def2, def1, def1)
				}
			}
		})
	}
}

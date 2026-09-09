package document

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// NOTE: unexported embeds and tagged unexported fields are deliberately
// absent below. data.StructFieldValues panics on Interface() for those
// (unexported traversal), while walkStructFields skips them — the only
// intentional behavioural delta, and strictly safer.

type walkInner struct {
	City string  `anansi:"city"`
	Zip  *string `anansi:"zip,omitempty"`
}

// WalkPlainEmbed is embedded by value to exercise anonymous-embed
// flattening. It must be exported: traversal through an unexported embed
// yields non-interfaceable values.
type WalkPlainEmbed struct {
	Nick string `anansi:"nick"`
}

type walkProbe struct {
	DocumentModel
	Name     string         `anansi:"name"`
	Age      int            `anansi:"age,omitempty"`
	Active   bool           `anansi:"active"`
	Score    float64        `anansi:"score"`
	Count    uint           `anansi:"count"`
	Tags     []string       `anansi:"tags"`
	Inners   []walkInner    `anansi:"inners"`
	Attrs    map[string]any `anansi:"attrs"`
	Inner    walkInner      `anansi:"inner"`
	InnerPtr *walkInner     `anansi:"inner_ptr"`
	When     time.Time      `anansi:"when"`
	Dotted   string         `anansi:"address.city"`
	Ignored  string         `anansi:"-"`
	NoTag    string
	JSONOnly string `json:"json_only"`
	WalkPlainEmbed
}

func walkStrptr(s string) *string { return &s }

func walkFullProbe() walkProbe {
	return walkProbe{
		DocumentModel: DocumentModel{
			ID:       "doc-1",
			Metadata: map[string]any{"source": "parity", "n": 1},
		},
		Name:     "probe",
		Age:      30,
		Active:   true,
		Score:    9.5,
		Count:    7,
		Tags:     []string{"a", "b"},
		Inners:   []walkInner{{City: "Oslo", Zip: walkStrptr("0001")}, {City: "Lagos"}},
		Attrs:    map[string]any{"k": "v", "nested": map[string]any{"x": 1}},
		Inner:    walkInner{City: "Berlin", Zip: walkStrptr("10115")},
		InnerPtr: &walkInner{City: "Paris"},
		When:     time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		Dotted:   "downtown",
		Ignored:  "skip",
		NoTag:    "skip",
		JSONOnly: "skip",
		WalkPlainEmbed: WalkPlainEmbed{
			Nick: "nick",
		},
	}
}

func requireWalkParity(t *testing.T, in any, partial bool) {
	t.Helper()

	got, gotErr := walkStructFields(in, partial)
	want, wantErr := data.StructFieldValues(in, partial)

	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("partial=%v: error mismatch: got=%v want=%v", partial, gotErr, wantErr)
	}
	if gotErr != nil {
		return
	}
	if len(got) != len(want) {
		t.Fatalf("partial=%v: length mismatch: got=%d want=%d", partial, len(got), len(want))
	}
	for i := range got {
		if got[i].path != want[i].Path {
			t.Fatalf("partial=%v: path mismatch at %d: got=%q want=%q", partial, i, got[i].path, want[i].Path)
		}
		if !reflect.DeepEqual(got[i].value, want[i].Value) {
			t.Fatalf("partial=%v: value mismatch at %q: got=%#v want=%#v", partial, got[i].path, got[i].value, want[i].Value)
		}
	}
}

// TestWalkStructFieldsParity pins behavioural parity between the document
// package's struct walker and the data implementation it replaces, across
// full/partial modes and populated/sparse inputs.
func TestWalkStructFieldsParity(t *testing.T) {
	full := walkFullProbe()

	t.Run("full/non-partial", func(t *testing.T) {
		requireWalkParity(t, full, false)
	})

	t.Run("full/partial", func(t *testing.T) {
		requireWalkParity(t, full, true)
	})

	t.Run("sparse/non-partial", func(t *testing.T) {
		requireWalkParity(t, walkProbe{}, false)
	})

	t.Run("sparse/partial", func(t *testing.T) {
		requireWalkParity(t, walkProbe{}, true)
	})

	t.Run("pointer-input", func(t *testing.T) {
		requireWalkParity(t, &full, false)
		requireWalkParity(t, &full, true)
	})

	t.Run("nil-pointer", func(t *testing.T) {
		got, gotErr := walkStructFields((*walkProbe)(nil), false)
		require.NoError(t, gotErr)
		require.Nil(t, got)

		want, wantErr := data.StructFieldValues((*walkProbe)(nil), false)
		require.NoError(t, wantErr)
		require.Nil(t, want)
	})

	t.Run("non-struct-errors", func(t *testing.T) {
		_, gotErr := walkStructFields("nope", false)
		require.Error(t, gotErr)

		_, wantErr := data.StructFieldValues("nope", false)
		require.Error(t, wantErr)
	})
}

// TestWalkStructFieldsSystemEmbed asserts the partial-mode contract
// directly: embedded identity fields are skipped in partial walks (a
// carried _id_ is only honored when shadowed by an outer field), while
// non-zero user fields are kept.
func TestWalkStructFieldsSystemEmbed(t *testing.T) {
	full := walkFullProbe()

	partial, err := walkStructFields(full, true)
	require.NoError(t, err)
	paths := make(map[string]any, len(partial))
	for _, f := range partial {
		paths[f.path] = f.value
	}
	require.NotContains(t, paths, "_metadata_")
	require.NotContains(t, paths, "_id_")
	require.Contains(t, paths, "name")
	require.Contains(t, paths, "age") // Age=30, non-zero user field

	sparse, err := walkStructFields(walkProbe{}, true)
	require.NoError(t, err)
	require.Empty(t, sparse)

	whole, err := walkStructFields(full, false)
	require.NoError(t, err)
	wholePaths := make(map[string]any, len(whole))
	for _, f := range whole {
		wholePaths[f.path] = f.value
	}
	require.Contains(t, wholePaths, "_id_")
	require.Contains(t, wholePaths, "_metadata_")
}

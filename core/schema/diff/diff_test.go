package diff_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/schema/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MustNewVersion is a test helper to create a common.Version or panic
func MustNewVersion(v string) *common.Version {
	ver, err := common.NewVersion(v)
	if err != nil {
		panic(err)
	}
	return ver
}

// MustNewLiteralValue is a test helper to create a definition.LiteralValue or panic
func MustNewLiteralValue[T definition.LiteralValueType](val T) definition.LiteralValue {
	lv, err := definition.NewLiteralValue(val)
	if err != nil {
		panic(err)
	}
	return lv
}

// ptr is a helper to get a pointer to any value.
func ptr[T any](v T) *T {
	return &v
}

func TestDiff_RootProperties(t *testing.T) {
	testCases := []struct {
		name         string
		fromSchema   definition.Schema
		toSchema     definition.Schema
		expectedDiff *diff.SchemaDiff
	}{
		{
			name: "No change",
			fromSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema"},
			},
			toSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema"},
			},
			expectedDiff: &diff.SchemaDiff{Changes: []diff.SemanticChange{}},
		},
		{
			name: "Version changed",
			fromSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema"},
			},
			toSchema: definition.Schema{
				Version:    *MustNewVersion("1.1.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema"},
			},
			expectedDiff: &diff.SchemaDiff{
				Changes: []diff.SemanticChange{
					{
						Kind:     diff.RootModified,
						EntityId: "",
						Forward: []diff.Operation{
							{
								Type:  diff.OpSet,
								Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaVersion}}},
								Value: "1.1.0",
							},
						},
						Backward: []diff.Operation{
							{
								Type:  diff.OpSet,
								Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaVersion}}},
								Value: "1.0.0",
							},
						},
					},
				},
			},
		},
		{
			name: "Name changed",
			fromSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "OldName"},
			},
			toSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "NewName"},
			},
			expectedDiff: &diff.SchemaDiff{
				Changes: []diff.SemanticChange{
					{
						Kind:     diff.RootModified,
						EntityId: "",
						Forward: []diff.Operation{
							{
								Type:  diff.OpSet,
								Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaName}}},
								Value: "NewName",
							},
						},
						Backward: []diff.Operation{
							{
								Type:  diff.OpSet,
								Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaName}}},
								Value: "OldName",
							},
						},
					},
				},
			},
		},
		{
			name: "Description added",
			fromSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema"},
			},
			toSchema: definition.Schema{
				Version:    *MustNewVersion("1.0.0"),
				BaseSchema: definition.BaseSchema{Name: "TestSchema", Description: "New Description"},
			},
			expectedDiff: &diff.SchemaDiff{
				Changes: []diff.SemanticChange{
					{
						Kind:     diff.RootModified,
						EntityId: "",
						Forward: []diff.Operation{
							{
								Type:  diff.OpSet,
								Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaDescription}}},
								Value: "New Description",
							},
						},
						Backward: []diff.Operation{
							{
								Type: diff.OpRemove,
								Path: diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaDescription}}},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDiff, err := diff.Diff(tc.fromSchema, tc.toSchema)
			require.NoError(t, err)

			// Use json.Marshal/Unmarshal to normalize and compare the diffs
			// This avoids issues with comparing `any` types directly and ensures
			// the underlying values are compared correctly after JSON serialization.
			actualJSON, err := json.Marshal(actualDiff)
			require.NoError(t, err)
			expectedJSON, err := json.Marshal(tc.expectedDiff)
			require.NoError(t, err)

			assert.JSONEq(t, string(expectedJSON), string(actualJSON), "SchemaDiff mismatch")
		})
	}
}

func TestSchemaDiff_MarshalUnmarshalJSON(t *testing.T) {
	expectedDiff := &diff.SchemaDiff{
		Changes: []diff.SemanticChange{
			{
				Kind:     diff.FieldAdded,
				EntityId: "newField",
				Forward: []diff.Operation{
					{
						Type: diff.OpAdd,
						Path: diff.Path{
							Segments: []diff.PathSegment{
								{Type: diff.PathEntity, Key: "newField"},
							},
						},
						Value: map[string]any{
							"name": "newField",
							"type": "string",
						},
					},
				},
				Backward: []diff.Operation{
					{
						Type: diff.OpRemove,
						Path: diff.Path{
							Segments: []diff.PathSegment{
								{Type: diff.PathEntity, Key: "newField"},
							},
						},
					},
				},
			},
			{
				Kind:     diff.RootModified,
				EntityId: "",
				Forward: []diff.Operation{
					{
						Type:  diff.OpSet,
						Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaVersion}}},
						Value: "1.1.0",
					},
				},
				Backward: []diff.Operation{
					{
						Type:  diff.OpSet,
						Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaVersion}}},
						Value: "1.0.0",
					},
				},
			},
			{
				Kind:     diff.MetadataModified,
				EntityId: "author",
				Forward: []diff.Operation{
					{
						Type:  diff.OpSet,
						Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaMetadata}}},
						Key:   ptr("author"), // Use common.Ptr for string pointers
						Value: "newAuthor",
					},
				},
				Backward: []diff.Operation{
					{
						Type:  diff.OpSet,
						Path:  diff.Path{Segments: []diff.PathSegment{{Type: diff.PathSchemaMetadata}}},
						Key:   ptr("author"),
						Value: "oldAuthor",
					},
				},
			},
		},
	}

	// Marshal the diff to JSON
	marshaledData, err := json.MarshalIndent(expectedDiff, "", "  ")
	require.NoError(t, err)

	// Unmarshal the JSON back into a new SchemaDiff object
	var unmarshaledDiff diff.SchemaDiff
	err = json.Unmarshal(marshaledData, &unmarshaledDiff)
	require.NoError(t, err)

	// Compare the unmarshaled object with the original
	// We use Marshal/Unmarshal again to handle 'any' types consistently for comparison
	unmarshaledJSON, err := json.Marshal(unmarshaledDiff)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(expectedDiff)
	require.NoError(t, err)

	assert.JSONEq(t, string(expectedJSON), string(unmarshaledJSON), "Marshal-Unmarshal round-trip mismatch")
}

// generateSchema creates a schema with a specified number of fields.
func generateSchema(numFields int, versionName string) definition.Schema {
	fields := make(map[definition.FieldId]definition.Field)
	for i := range numFields {
		fieldID := definition.FieldId(fmt.Sprintf("field%d", i))
		fields[fieldID] = definition.Field{
			Name:       definition.FieldName(fmt.Sprintf("field%d", i)),
			Required:   i%2 == 0,
			Deprecated: i%3 == 0,
			FieldProperties: definition.FieldProperties{
				Type: definition.FieldTypeString,
				Default: MustNewLiteralValue(fmt.Sprintf("default%d", i)),
			},
		}
	}

	return definition.Schema{
		Version: *MustNewVersion(versionName),
		BaseSchema: definition.BaseSchema{
			Name:   "BenchmarkSchema",
			Fields: fields,
		},
	}
}

var Scmap map[string]any
func BenchmarkDiff(b *testing.B) {
	benchmarks := []struct {
		name      string
		numFields int
		changeType string // "none", "version", "field"
	}{
		{"SmallSchema_NoChange", 10, "none"},
		{"SmallSchema_VersionChange", 10, "version"},
		{"SmallSchema_FieldChange", 10, "field"},
		{"MediumSchema_NoChange", 100, "none"},
		{"MediumSchema_VersionChange", 100, "version"},
		{"MediumSchema_FieldChange", 100, "field"},
		{"LargeSchema_NoChange", 1000, "none"},
		{"LargeSchema_VersionChange", 1000, "version"},
		{"LargeSchema_FieldChange", 1000, "field"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			fromSchema := generateSchema(bm.numFields, "1.0.0")
			toSchema := fromSchema // Start with same schema

			switch bm.changeType {
			case "version":
				toSchema.Version = *MustNewVersion("1.1.0")
			case "field":
				// Modify one field
				if bm.numFields > 0 {
					fieldID := definition.FieldId(fmt.Sprintf("field%d", bm.numFields/2))
					modifiedField := toSchema.BaseSchema.Fields[fieldID]
					modifiedField.Required = !modifiedField.Required
					toSchema.BaseSchema.Fields[fieldID] = modifiedField
				}
			case "none":
				// No changes
			}

			b.ResetTimer()
			for b.Loop() {
				_, err := diff.Diff(fromSchema, toSchema)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

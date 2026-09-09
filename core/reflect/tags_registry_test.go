package reflect

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Tag Construction Helpers
// ============================================================================

// makeTag builds a Tag from a key and value string, packing the handle
// exactly as buildMetadata does.
func makeTag(key, value string) Tag {
	slab := make([]byte, 0, len(key)+len(value))
	kOff := uint16(len(slab))
	slab = append(slab, key...)
	vOff := uint16(len(slab))
	slab = append(slab, value...)

	var h uint64
	h |= uint64(kOff&0x7FFF)
	h |= uint64(uint16(len(key))&0x7FFF) << 15
	h |= uint64(vOff&0x7FFF) << 30
	h |= uint64(uint16(len(value))&0x7FFF) << 45
	h |= uint64(computeValueKind(value, uint16(len(value)))&0x3) << 60

	return Tag{handle: h, slab: slab}
}

// makeEmptyTag returns a zero-value Tag.
func makeEmptyTag() Tag { return Tag{} }

// ============================================================================
// ValueKind Constants
// ============================================================================

func TestValueKind_Constants(t *testing.T) {
	assert.Equal(t, ValueKind(0), KindEmpty)
	assert.Equal(t, ValueKind(1), KindString)
	assert.Equal(t, ValueKind(2), KindSlice)

	// Verify they are distinct
	assert.NotEqual(t, KindEmpty, KindString)
	assert.NotEqual(t, KindString, KindSlice)
	assert.NotEqual(t, KindEmpty, KindSlice)
}

func TestValueKind_IsSequential(t *testing.T) {
	assert.Equal(t, 0, int(KindEmpty))
	assert.Equal(t, 1, int(KindString))
	assert.Equal(t, 2, int(KindSlice))
}

// ============================================================================
// Tag.IsZero
// ============================================================================

func TestTag_IsZero(t *testing.T) {
	t.Run("zero value is zero", func(t *testing.T) {
		assert.True(t, makeEmptyTag().IsZero())
	})

	t.Run("tag with key is not zero", func(t *testing.T) {
		tag := makeTag("json", "name")
		assert.False(t, tag.IsZero())
	})

	t.Run("tag with empty value but key is not zero", func(t *testing.T) {
		tag := makeTag("required", "")
		assert.False(t, tag.IsZero())
	})
}

// ============================================================================
// Tag.Key
// ============================================================================

func TestTag_Key(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{"standard key", "json", "name", "json"},
		{"empty key", "", "value", ""},
		{"long key", "validate", "required", "validate"},
		{"special chars", "db:column", "users", "db:column"},
		{"key only", "flag", "", "flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := makeTag(tt.key, tt.val)
			assert.Equal(t, tt.want, tag.Key())
		})
	}

	t.Run("zero tag returns empty key", func(t *testing.T) {
		assert.Equal(t, "", makeEmptyTag().Key())
	})
}

// ============================================================================
// Tag.ValueKind
// ============================================================================

func TestTag_ValueKind(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want ValueKind
	}{
		{"empty value", "", KindEmpty},
		{"single value", "required", KindString},
		{"comma separated", "a,b,c", KindSlice},
		{"single comma value", "one,two", KindSlice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := makeTag("key", tt.val)
			assert.Equal(t, tt.want, tag.ValueKind())
		})
	}

	t.Run("zero tag returns KindEmpty", func(t *testing.T) {
		assert.Equal(t, KindEmpty, makeEmptyTag().ValueKind())
	})
}

// ============================================================================
// Tag.Value
// ============================================================================

func TestTag_Value(t *testing.T) {
	t.Run("single value returns value and true", func(t *testing.T) {
		tag := makeTag("json", "name")
		v, ok := tag.Value()
		assert.True(t, ok)
		assert.Equal(t, "name", v)
	})

	t.Run("empty value returns false", func(t *testing.T) {
		tag := makeTag("flag", "")
		_, ok := tag.Value()
		assert.False(t, ok)
	})

	t.Run("zero tag returns false", func(t *testing.T) {
		_, ok := makeEmptyTag().Value()
		assert.False(t, ok)
	})

	t.Run("comma separated still returns raw value", func(t *testing.T) {
		tag := makeTag("validate", "min=1,max=100")
		v, ok := tag.Value()
		assert.True(t, ok)
		assert.Equal(t, "min=1,max=100", v)
	})
}

// ============================================================================
// Tag.Values
// ============================================================================

func TestTag_Values(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want []string
	}{
		{"single value", "required", []string{"required"}},
		{"two values", "a,b", []string{"a", "b"}},
		{"three values", "x,y,z", []string{"x", "y", "z"}},
		{"trailing comma", "a,", []string{"a", ""}},
		{"leading comma", ",b", []string{"", "b"}},
		{"empty values in middle", "a,,c", []string{"a", "", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := makeTag("key", tt.val)
			assert.Equal(t, tt.want, tag.Values())
		})
	}

	t.Run("empty value returns nil", func(t *testing.T) {
		tag := makeTag("key", "")
		assert.Nil(t, tag.Values())
	})

	t.Run("zero tag returns nil", func(t *testing.T) {
		assert.Nil(t, makeEmptyTag().Values())
	})
}

// ============================================================================
// Tag.ValuesIter
// ============================================================================

func TestTag_ValuesIter(t *testing.T) {
	t.Run("iterates single value", func(t *testing.T) {
		tag := makeTag("json", "name")
		var got []string
		for v := range tag.ValuesIter() {
			got = append(got, v)
		}
		assert.Equal(t, []string{"name"}, got)
	})

	t.Run("iterates multiple values", func(t *testing.T) {
		tag := makeTag("validate", "min=1,max=100,required")
		var got []string
		for v := range tag.ValuesIter() {
			got = append(got, v)
		}
		assert.Equal(t, []string{"min=1", "max=100", "required"}, got)
	})

	t.Run("early termination", func(t *testing.T) {
		tag := makeTag("validate", "a,b,c")
		count := 0
		for v := range tag.ValuesIter() {
			_ = v
			count++
			if count == 2 {
				break
			}
		}
		assert.Equal(t, 2, count)
	})

	t.Run("empty value yields nothing", func(t *testing.T) {
		tag := makeTag("key", "")
		var got []string
		for v := range tag.ValuesIter() {
			got = append(got, v)
		}
		assert.Nil(t, got)
	})

	t.Run("zero tag yields nothing", func(t *testing.T) {
		var got []string
		for v := range makeEmptyTag().ValuesIter() {
			got = append(got, v)
		}
		assert.Nil(t, got)
	})

	t.Run("matches Values output", func(t *testing.T) {
		tag := makeTag("validate", "a,b,c,d")
		fromValues := tag.Values()
		var fromIter []string
		for v := range tag.ValuesIter() {
			fromIter = append(fromIter, v)
		}
		assert.Equal(t, fromValues, fromIter)
	})
}

// ============================================================================
// Tag.Read
// ============================================================================

type simpleUnmarshaler struct {
	values []string
}

func (u *simpleUnmarshaler) FromValues(values ...string) error {
	u.values = values
	return nil
}

type failingUnmarshaler struct{}

func (u *failingUnmarshaler) FromValues(values ...string) error {
	return errors.New("unmarshal failed")
}

func TestTag_Read(t *testing.T) {
	t.Run("nil target returns error", func(t *testing.T) {
		tag := makeTag("json", "name")
		err := tag.Read(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("single value populates target", func(t *testing.T) {
		tag := makeTag("json", "name")
		var target simpleUnmarshaler
		err := tag.Read(&target)
		require.NoError(t, err)
		assert.Equal(t, []string{"name"}, target.values)
	})

	t.Run("comma separated value splits", func(t *testing.T) {
		tag := makeTag("validate", "min=1,max=100")
		var target simpleUnmarshaler
		err := tag.Read(&target)
		require.NoError(t, err)
		assert.Equal(t, []string{"min=1", "max=100"}, target.values)
	})

	t.Run("empty value passes nil slice", func(t *testing.T) {
		tag := makeTag("flag", "")
		var target simpleUnmarshaler
		err := tag.Read(&target)
		require.NoError(t, err)
		assert.Nil(t, target.values)
	})

	t.Run("zero tag passes nil slice", func(t *testing.T) {
		var target simpleUnmarshaler
		err := makeEmptyTag().Read(&target)
		require.NoError(t, err)
		assert.Nil(t, target.values)
	})

	t.Run("propagates unmarshaler error", func(t *testing.T) {
		tag := makeTag("json", "field")
		var target failingUnmarshaler
		err := tag.Read(&target)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal failed")
	})
}

// ============================================================================
// Tags via Public API - FieldTag
// ============================================================================

type TagAlpha struct {
	Name  string `json:"name" validate:"required,min=1"`
	Email string `json:"email" validate:"required,email"`
}

type TagBeta struct {
	ID   int `json:"id" db:"primary_key"`
	Slug string
}

type TagGamma struct {
	Only string `json:"only"`
}

type TagEmpty struct {
	NoTag string
}

func TestFieldTag(t *testing.T) {
	t.Run("find existing tag", func(t *testing.T) {
		tag, ok := FieldTag[TagAlpha]("Name", "json")
		require.True(t, ok)
		assert.Equal(t, "json", tag.Key())
		v, ok := tag.Value()
		require.True(t, ok)
		assert.Equal(t, "name", v)
	})

	t.Run("find second tag on same field", func(t *testing.T) {
		tag, ok := FieldTag[TagAlpha]("Name", "validate")
		require.True(t, ok)
		assert.Equal(t, "validate", tag.Key())
		assert.Equal(t, KindSlice, tag.ValueKind())
	})

	t.Run("non-existent tag key returns false", func(t *testing.T) {
		_, ok := FieldTag[TagAlpha]("Name", "nonexistent")
		assert.False(t, ok)
	})

	t.Run("non-existent field returns false", func(t *testing.T) {
		_, ok := FieldTag[TagAlpha]("Missing", "json")
		assert.False(t, ok)
	})

	t.Run("struct with no tags on field", func(t *testing.T) {
		_, ok := FieldTag[TagEmpty]("NoTag", "json")
		assert.False(t, ok)
	})

	t.Run("different struct types", func(t *testing.T) {
		tag, ok := FieldTag[TagBeta]("ID", "json")
		require.True(t, ok)
		v, ok := tag.Value()
		require.True(t, ok)
		assert.Equal(t, "id", v)
	})

	t.Run("field without tag struct", func(t *testing.T) {
		_, ok := FieldTag[TagBeta]("Slug", "json")
		assert.False(t, ok)
	})
}

// ============================================================================
// Tags via Public API - FieldTags
// ============================================================================

func TestFieldTags(t *testing.T) {
	t.Run("all tags on a field", func(t *testing.T) {
		var keys []string
		for tag := range FieldTags[TagAlpha]("Name") {
			keys = append(keys, tag.Key())
		}
		assert.Equal(t, []string{"json", "validate"}, keys)
	})

	t.Run("field with no tags", func(t *testing.T) {
		count := 0
		for range FieldTags[TagEmpty]("NoTag") {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("non-existent field", func(t *testing.T) {
		count := 0
		for range FieldTags[TagAlpha]("Missing") {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("single tag field", func(t *testing.T) {
		var tags []Tag
		for tag := range FieldTags[TagGamma]("Only") {
			tags = append(tags, tag)
		}
		require.Len(t, tags, 1)
		assert.Equal(t, "json", tags[0].Key())
	})
}

// ============================================================================
// Tags via Public API - Tags (all field/tag pairs)
// ============================================================================

func TestTags(t *testing.T) {
	type Simple struct {
		A string `json:"a"`
		B int    `db:"b"`
	}

	t.Run("yields all field/tag pairs", func(t *testing.T) {
		var pairs [][2]string
		for name, tag := range Tags[Simple]() {
			pairs = append(pairs, [2]string{name, tag.Key()})
		}
		assert.Contains(t, pairs, [2]string{"A", "json"})
		assert.Contains(t, pairs, [2]string{"B", "db"})
		assert.Len(t, pairs, 2)
	})

	t.Run("struct with multiple tags per field", func(t *testing.T) {
		var pairs [][2]string
		for name, tag := range Tags[TagAlpha]() {
			pairs = append(pairs, [2]string{name, tag.Key()})
		}
		assert.Len(t, pairs, 4) // Name: json+validate, Email: json+validate
	})

	t.Run("struct with no tags", func(t *testing.T) {
		type NoTags struct {
			A string
			B int
		}
		count := 0
		for range Tags[NoTags]() {
			count++
		}
		assert.Equal(t, 0, count)
	})
}

// ============================================================================
// Tags via Public API - KeyTags
// ============================================================================

func TestKeyTags(t *testing.T) {
	t.Run("fields with matching key", func(t *testing.T) {
		var fields []string
		for name, tag := range KeyTags[TagAlpha]("json") {
			_ = tag
			fields = append(fields, name)
		}
		assert.Contains(t, fields, "Name")
		assert.Contains(t, fields, "Email")
		assert.Len(t, fields, 2)
	})

	t.Run("fields with non-matching key", func(t *testing.T) {
		count := 0
		for range KeyTags[TagAlpha]("nonexistent") {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("partial match across fields", func(t *testing.T) {
		type Mixed struct {
			A string `json:"a" db:"col_a"`
			B int    `json:"b"`
			C bool   `db:"col_c"`
		}
		var fields []string
		for name, tag := range KeyTags[Mixed]("db") {
			_ = tag
			fields = append(fields, name)
		}
		assert.Contains(t, fields, "A")
		assert.Contains(t, fields, "C")
		assert.NotContains(t, fields, "B")
	})
}

// ============================================================================
// Read (materialization)
// ============================================================================

type testMaterializer struct {
	Fields map[string][]string
}

func (m *testMaterializer) FromTags(field string, tags []Tag) error {
	if m.Fields == nil {
		m.Fields = make(map[string][]string)
	}
	for _, tag := range tags {
		m.Fields[field] = append(m.Fields[field], tag.Key())
	}
	return nil
}

func TestRead_Materialization(t *testing.T) {
	type Source struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email"`
	}

	t.Run("materializes via TagUnmarshaler", func(t *testing.T) {
		result, err := Parse[Source, testMaterializer]()
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.Fields["Name"], "json")
		assert.Contains(t, result.Fields["Name"], "validate")
		assert.Contains(t, result.Fields["Email"], "json")
	})

	t.Run("result is cached", func(t *testing.T) {
		r1, err := Parse[Source, testMaterializer]()
		require.NoError(t, err)
		r2, err := Parse[Source, testMaterializer]()
		require.NoError(t, err)
		assert.Same(t, r1, r2)
	})
}

type parserResultA struct {
	TagKeys []string
}

type parserResultB struct {
	TagKeys []string
}

func TestRead_WithParser(t *testing.T) {
	type SourceA struct {
		Name string `json:"name"`
	}
	type SourceB struct {
		Name string `json:"name"`
	}

	t.Run("parser is invoked when K does not implement TagUnmarshaler", func(t *testing.T) {
		result, err := Parse[SourceA, parserResultA](func(tags []Tag) (parserResultA, error) {
			var keys []string
			for _, tag := range tags {
				keys = append(keys, tag.Key())
			}
			return parserResultA{TagKeys: keys}, nil
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.TagKeys, "json")
	})

	t.Run("parser error is propagated", func(t *testing.T) {
		_, err := Parse[SourceB, parserResultB](func(tags []Tag) (parserResultB, error) {
			return parserResultB{}, errors.New("parser error")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parser error")
	})

	t.Run("TagUnmarshaler takes precedence over parser", func(t *testing.T) {
		type src2 struct {
			Name string `json:"name"`
		}
		called := false
		result, err := Parse[src2, testMaterializer](func(tags []Tag) (testMaterializer, error) {
			called = true
			return testMaterializer{}, nil
		})
		require.NoError(t, err)
		assert.False(t, called, "parser should not be called when K implements TagUnmarshaler")
		assert.NotNil(t, result)
	})
}

func TestRead_TypeWithoutTagUnmarshaler(t *testing.T) {
	type Source struct {
		Name string `json:"name"`
	}
	type NoImpl struct{}

	_, err := Parse[Source, NoImpl]()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not implement TagUnmarshaler")
}

// ============================================================================
// computeValueKind
// ============================================================================

func TestComputeValueKind(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		valLen  uint16
		want    ValueKind
	}{
		{"empty", "", 0, KindEmpty},
		{"single value", "hello", 5, KindString},
		{"comma separated", "a,b", 3, KindSlice},
		{"multiple commas", "a,b,c", 5, KindSlice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeValueKind(tt.val, tt.valLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
// Tags on structs with various field visibility
// ============================================================================

type tagMixedVisibility struct {
	Exported   string `json:"exported"`
	unexported string `db:"unexported"`
	Anonymous  `json:"anonymous"`
}

type Anonymous struct{}

func TestTags_MixedVisibility(t *testing.T) {
	t.Run("exported fields are included", func(t *testing.T) {
		_, ok := FieldTag[tagMixedVisibility]("Exported", "json")
		assert.True(t, ok)
	})

	t.Run("unexported fields are excluded", func(t *testing.T) {
		_, ok := FieldTag[tagMixedVisibility]("unexported", "db")
		assert.False(t, ok)
	})

	t.Run("anonymous embedded fields are included", func(t *testing.T) {
		_, ok := FieldTag[tagMixedVisibility]("Anonymous", "json")
		assert.True(t, ok)
	})
}

// ============================================================================
// Tag with special characters in value
// ============================================================================

func TestTag_SpecialCharacters(t *testing.T) {
	t.Run("escaped quotes in value", func(t *testing.T) {
		type Special struct {
			Field string `json:"field\"name"`
		}
		tag, ok := FieldTag[Special]("Field", "json")
		require.True(t, ok)
		v, ok := tag.Value()
		require.True(t, ok)
		assert.Equal(t, `field\"name`, v)
	})
}

// ============================================================================
// Tag.Value on slice-kind tags
// ============================================================================

func TestTag_ValueOnSliceKind(t *testing.T) {
	tag := makeTag("validate", "required,min=1,max=100")
	assert.Equal(t, KindSlice, tag.ValueKind())

	v, ok := tag.Value()
	assert.True(t, ok)
	assert.Equal(t, "required,min=1,max=100", v)

	vals := tag.Values()
	assert.Equal(t, []string{"required", "min=1", "max=100"}, vals)
}

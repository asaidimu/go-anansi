package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bindModel mirrors the test schema (document_test.go) for binding tests.
type bindModel struct {
	ID       string   `anansi:"id"`
	Age      int      `anansi:"age"`
	Active   *bool    `anansi:"active"`
	Score    *float64 `anansi:"score"`
	Nickname *string  `anansi:"nickname"`
	Address  *addr    `anansi:"address"`
}

type addr struct {
	Street string `anansi:"street"`
	Zip    int    `anansi:"zip"`
}

// TestBindSlotAbsentPointerStaysNil guards the regression where absent fields
// bound as non-nil pointers to zero values (e.g. a *bool for a field missing
// from the payload). Partial-update paths treat non-nil pointers as present
// and clobber stored data, so absent fields must bind as nil.
func TestBindSlotAbsentPointerStaysNil(t *testing.T) {
	d, err := newTestCollection(t).FromMap(map[string]any{
		"id":  "user-1",
		"age": int64(31),
	})
	require.NoError(t, err)

	var out bindModel
	require.NoError(t, d.BindTo(&out))

	assert.Equal(t, "user-1", out.ID)
	assert.Equal(t, 31, out.Age)
	assert.Nil(t, out.Active, "absent bool field must stay nil")
	assert.Nil(t, out.Score, "absent number field must stay nil")
	assert.Nil(t, out.Nickname, "absent string field must stay nil")
	assert.Nil(t, out.Address, "absent nested object field must stay nil")
}

// TestBindSlotPresentPointerAllocated is the positive counterpart: fields that
// ARE present must still allocate their pointers and bind their values.
func TestBindSlotPresentPointerAllocated(t *testing.T) {
	d, err := newTestCollection(t).FromMap(map[string]any{
		"id":      "user-1",
		"active":  true,
		"score":   float64(9.5),
		"address": map[string]any{"street": "1 Main St", "zip": int64(10001)},
	})
	require.NoError(t, err)

	var out bindModel
	require.NoError(t, d.BindTo(&out))

	require.NotNil(t, out.Active)
	assert.True(t, *out.Active)
	require.NotNil(t, out.Score)
	assert.Equal(t, 9.5, *out.Score)
	require.NotNil(t, out.Address)
	assert.Equal(t, "1 Main St", out.Address.Street)
	assert.Equal(t, 10001, out.Address.Zip)
}

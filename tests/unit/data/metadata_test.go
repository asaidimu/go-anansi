package data_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternalMetadataAccessors(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{"field": "value"})

	// Version
	version, err := doc.Version()
	require.NoError(t, err)
	assert.Equal(t, 1, version)

	// Checksum
	checksum, err := doc.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum)

	// CreatedAt
	createdAt, err := doc.CreatedAt()
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), createdAt, 2*time.Second)

	// UpdatedAt
	updatedAt, err := doc.UpdatedAt()
	require.NoError(t, err)
	assert.Equal(t, createdAt, updatedAt) // Initially they should be the same

	// Test error on doc without metadata
	rawDoc := data.Document{"field": "value"}
	_, err = rawDoc.Version()
	assert.Error(t, err)
}

func TestCustomMetadataAccessors(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{"field": "value"})

	// Set and Get String
	err := doc.SetMetadataValue("customString", "hello world")
	require.NoError(t, err)
	strVal, err := doc.GetMetadataString("customString")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strVal)

	// Set and Get Int
	err = doc.SetMetadataValue("customInt", 123)
	require.NoError(t, err)
	intVal, err := doc.GetMetadataInt("customInt")
	require.NoError(t, err)
	assert.Equal(t, 123, intVal)

	// Set and Get Float
	err = doc.SetMetadataValue("customFloat", 3.14)
	require.NoError(t, err)
	floatVal, err := doc.GetMetadataFloat("customFloat")
	require.NoError(t, err)
	assert.InDelta(t, 3.14, floatVal, 0.001)

	// Set and Get Bool
	err = doc.SetMetadataValue("customBool", true)
	require.NoError(t, err)
	boolVal, err := doc.GetMetadataBool("customBool")
	require.NoError(t, err)
	assert.True(t, boolVal)

	// Set and Get Time
	now := time.Now().UTC().Truncate(time.Millisecond)
	err = doc.SetMetadataValue("customTime", now)
	require.NoError(t, err)
	timeVal, err := doc.GetMetadataTime("customTime")
	require.NoError(t, err)
	assert.Equal(t, now, timeVal)

	// Test Get non-existent key
	_, err = doc.GetMetadataString("nonExistent")
	assert.Error(t, err)
}

func TestSetMetadataValue_Protection(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{})

	// Attempt to overwrite internal keys
	err := doc.SetMetadataValue(data.MetadataVersion, 99)
	assert.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)

	err = doc.SetMetadataValue(data.MetadataChecksum, "abc")
	assert.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)

	err = doc.SetMetadataValue(data.MetadataCreated, time.Now())
	assert.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)

	err = doc.SetMetadataValue(data.MetadataUpdated, time.Now())
	assert.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)

	err = doc.SetMetadataValue(data.MetadataSignature, "sig")
	assert.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrReadOnlyField.Code, sysErr.Code)
}

package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullDefaultValidation(t *testing.T) {
	src := `{"name":"labels","version":"1.0.0","fields":{"project_id":{"name":"project_id","type":"string","required":true,"default":null}},"indexes":{}}`
	issues, err := schema.ValidateSchemaJson([]byte(src))
	require.NoError(t, err)
	for _, i := range issues {
		t.Logf("issue: %v", i)
	}
	assert.Empty(t, issues)
}

func TestNullMetadataValidation(t *testing.T) {
	src := `{"name":"labels","version":"1.0.0","metadata":{"note":null},"fields":{"f1":{"name":"f1","type":"string","metadata":{"x":null}}},"indexes":{}}`
	issues, err := schema.ValidateSchemaJson([]byte(src))
	require.NoError(t, err)
	for _, i := range issues {
		t.Logf("issue: %v", i)
	}
	assert.Empty(t, issues)
}

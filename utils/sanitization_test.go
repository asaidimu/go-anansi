package utils

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/require"
)

func TestSanitizationSchema(t *testing.T) {
	store := &sanitizationStore{
		collectionName: SanitizationPoliciesCollection,
	}
	sc := store.createPolicyCollectionSchema()

	issues, err := schema.ValidateSchema(sc)
	require.NoError(t, err)
	for _, issue := range issues {
		t.Logf("Issue: %s", issue.Message)
	}
	require.NoError(t, err, "Schema should be valid")
}

func TestSanitizationValidator(t *testing.T) {
	store := &sanitizationStore{
		collectionName: SanitizationPoliciesCollection,
	}
	sc := store.createPolicyCollectionSchema()

	_, err := schema.NewDocumentValidator(sc, nil)
	require.NoError(t, err, "Should be able to create a document validator for the sanitization schema")
}

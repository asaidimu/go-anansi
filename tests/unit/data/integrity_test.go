package data_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentHashing(t *testing.T) {

	doc := data.MustNewDocument(map[string]any{
		"field1": "value1",
	})

	// 1. Initial hash and verification
	err := doc.Hash()
	require.NoError(t, err)

	meta := doc.Metadata()
	_, hashExists := meta[data.MetadataChecksum]
	assert.True(t, hashExists, "checksum should exist in metadata after Hash() call")

	ok, err := doc.VerifyHash()
	assert.True(t, ok, "initial hash verification should succeed")
	require.NoError(t, err)

	// 2. Modifying non-metadata field should invalidate metadata hash
	doc.Set("field1","newValue")
	ok, err = doc.VerifyHash()
	assert.False(t, ok, "checksum should still be valid after modifying non-metadata field")

	// 3. Modifying metadata field SHOULD invalidate the hash
	meta[data.MetadataVersion] = 2
	doc.SetMetadata(meta)
	ok, err = doc.VerifyHash()
	assert.False(t, ok, "checksum should be invalid after modifying metadata field")

	// 4. Re-hashing should make it valid again
	err = doc.Hash()
	require.NoError(t, err)
	ok, err = doc.VerifyHash()
	assert.True(t, ok, "hash should be valid again after re-hashing")
}

func TestDocumentSigning(t *testing.T) {

	// Generate a key pair for testing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "failed to generate RSA private key")
	publicKey := &privateKey.PublicKey

	doc := data.MustNewDocument(map[string]any{
		"customer": "test-customer",
		"amount":   100,
	})

	// 1. Initial sign and verify
	err = doc.Sign(privateKey)
	require.NoError(t, err)

	meta := doc.Metadata()
	_, sigExists := meta[data.MetadataSignature]
	assert.True(t, sigExists, "signature should exist in metadata after Sign() call")

	err = doc.Verify(publicKey)
	assert.NoError(t, err, "initial signature verification should succeed")

	// 2. Modifying a body field SHOULD invalidate the signature
	doc.Set("amount",200)
	err = doc.Verify(publicKey)
	assert.Error(t, err, "signature should be invalid after modifying a body field")

	// 3. Modifying a metadata field SHOULD also invalidate the signature
	// First, re-sign the modified document
	err = doc.Sign(privateKey)
	require.NoError(t, err)
	err = doc.Verify(publicKey)
	require.NoError(t, err, "signature should be valid after re-signing")

	// Now, modify the metadata
	meta = doc.Metadata()
	meta[data.MetadataVersion] = 5
	doc.SetMetadata(meta)
	err = doc.Verify(publicKey)
	assert.Error(t, err, "signature should be invalid after modifying a metadata field")

	// 4. Re-signing should make it valid again
	err = doc.Sign(privateKey)
	require.NoError(t, err)
	err = doc.Verify(publicKey)
	assert.NoError(t, err, "signature should be valid again after final re-signing")
}

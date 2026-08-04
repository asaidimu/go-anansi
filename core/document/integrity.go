package document

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
)

// ============================================================================
// CANONICALIZATION
// ============================================================================
//
// The hash/sign/verify routines reproduce data.Document's semantics exactly:
// the document is rendered canonically (sorted keys, normalized numerics) with
// the checksum and signature fields removed from metadata before hashing.

// canonicalize recursively normalizes a value for consistent serialization.
func canonicalize(v any) any {
	switch val := v.(type) {
	case map[string]any:
		newMap := make(map[string]any, len(val))
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			newMap[k] = canonicalize(val[k])
		}
		return newMap
	case []any:
		newSlice := make([]any, len(val))
		for i, item := range val {
			newSlice[i] = canonicalize(item)
		}
		return newSlice
	case float64:
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val
	case int:
		return int64(val)
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case uint:
		return uint64(val)
	case uint8:
		return uint64(val)
	case uint16:
		return uint64(val)
	case uint32:
		return uint64(val)
	case uint64:
		return val
	default:
		return v
	}
}

func canonicalMarshal(v any) ([]byte, error) {
	return json.Marshal(canonicalize(v))
}

// canonicalSkip are the metadata fields excluded from the canonical hash
// input, mirroring the data layer's hash/sign/verify semantics.
var canonicalSkip = map[string]bool{
	data.MetadataField + "." + data.MetadataSignature: true,
	data.MetadataField + "." + data.MetadataChecksum:  true,
}

// canonicalBytes renders the document's canonical JSON for hashing/signing.
// For root container-backed documents it walks the container directly — keys
// sorted, numerics normalized — without materializing intermediate maps.
// Records and views fall back to map canonicalization.
func (d *Document) canonicalBytes() ([]byte, error) {
	if d.isRecord() || len(d.prefix) > 0 {
		return canonicalMarshal(d.canonicalHashInput())
	}
	return cjson.SerializeJSONCanonical(d.cs, d.c, canonicalSkip)
}

// canonicalHashInput returns the canonical document map with signature and
// checksum removed from the metadata block. Used only for record views and
// subtrees; root documents serialize directly from the container instead.
func (d *Document) canonicalHashInput() map[string]any {
	m := d.ToMap()
	if meta, ok := m[data.MetadataField].(map[string]any); ok {
		delete(meta, data.MetadataSignature)
		delete(meta, data.MetadataChecksum)
	}
	return m
}

func (d *Document) calculateHash() (string, error) {
	h := sha256.New()
	if err := d.writeCanonical(h); err != nil {
		return "", common.SystemErrorFrom(data.ErrFailedToMarshalMetadata).
			WithOperation("document.calculateHash").WithCause(err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeCanonical renders the document's canonical JSON into w. Root
// container-backed documents stream straight from the container into the
// writer, avoiding the intermediate byte copy; records and views fall back to
// map canonicalization.
func (d *Document) writeCanonical(w io.Writer) error {
	if d.isRecord() || len(d.prefix) > 0 {
		b, err := canonicalMarshal(d.canonicalHashInput())
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	return cjson.SerializeJSONCanonicalTo(w, d.cs, d.c, canonicalSkip)
}

// ============================================================================
// DATA INTEGRITY
// ============================================================================

// Hash computes and stores the SHA-256 checksum of the document in metadata.
func (d *Document) Hash() error {
	hash, err := d.calculateHash()
	if err != nil {
		return err
	}
	return d.setMetadataValue(data.MetadataChecksum, hash, true)
}

// VerifyHash checks the stored checksum against a freshly computed one.
func (d *Document) VerifyHash() (bool, error) {
	if d == nil || !d.metadataKeySet(data.MetadataChecksum) {
		return false, nil
	}
	providedHash, err := d.GetMetadataValue(data.MetadataChecksum)
	if err != nil {
		return false, err
	}
	p, ok := providedHash.(string)
	if !ok {
		return false, nil
	}
	calculatedHash, err := d.calculateHash()
	if err != nil {
		return false, err
	}
	return p == calculatedHash, nil
}

// Sign computes and stores an RSA signature of the document in metadata.
func (d *Document) Sign(privateKey *rsa.PrivateKey) error {
	canonicalBytes, err := d.canonicalBytes()
	if err != nil {
		return common.SystemErrorFrom(data.ErrSignDocumentMarshalFailed).
			WithOperation("document.Sign").WithCause(err)
	}
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed, nil)
	if err != nil {
		return common.SystemErrorFrom(data.ErrSignDocumentFailed).
			WithOperation("document.Sign").WithCause(err)
	}
	return d.setMetadataValue(data.MetadataSignature, base64.StdEncoding.EncodeToString(signature), true)
}

// Verify checks the stored RSA signature against a public key.
func (d *Document) Verify(publicKey *rsa.PublicKey) error {
	if d == nil {
		return common.SystemErrorFrom(data.ErrNoMetadata).WithOperation("document.Verify")
	}
	sigVal, err := d.GetMetadataValue(data.MetadataSignature)
	if err != nil {
		return common.SystemErrorFrom(data.ErrSignatureInvalid).
			WithOperation("document.Verify").WithMessage("no signature found in metadata")
	}
	sig, ok := sigVal.(string)
	if !ok {
		return common.SystemErrorFrom(data.ErrSignatureInvalid).
			WithOperation("document.Verify").WithMessage("no signature found in metadata")
	}
	signedBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return common.SystemErrorFrom(data.ErrVerifySignatureDecodeFailed).
			WithOperation("document.Verify").WithCause(err)
	}
	canonicalBytes, err := d.canonicalBytes()
	if err != nil {
		return common.SystemErrorFrom(data.ErrVerifyDocumentMarshalFailed).
			WithOperation("document.Verify").WithCause(err)
	}
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	if err := rsa.VerifyPSS(publicKey, crypto.SHA256, hashed, signedBytes, nil); err != nil {
		return common.SystemErrorFrom(data.ErrSignatureVerificationFailed).
			WithOperation("document.Verify").WithCause(err)
	}
	return nil
}

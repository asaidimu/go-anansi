package data

import (
	"context"
	"crypto/rsa"
	"time"
)

// ============================================================================
// Document Interfaces
// ============================================================================

// DocumentReader is the read-only view of a Document.
// Implement it when a consumer only needs to inspect a document
// without mutating it.
type DocumentReader interface {
	// Identity and context
	ID() string
	Context() context.Context

	// Data access
	Get(key string) (any, error)
	GetNested(path string) (any, error)
	GetOr(key string, defaultValue any) any
	MustGet(key string) any
	GetString(keyOrPath string) (string, error)
	GetInt(keyOrPath string) (int, error)
	GetFloat64(keyOrPath string) (float64, error)
	GetBool(keyOrPath string) (bool, error)
	GetTime(keyOrPath string) (time.Time, error)
	GetDocument(keyOrPath string) (Documenter, error)
	GetDocumentArray(keyOrPath string) ([]Documenter, error)
	GetStringArray(keyOrPath string) ([]string, error)
	GetIntArray(keyOrPath string) ([]int, error)
	GetArray(keyOrPath string) ([]any, error)
	Keys() []string
	Values() []any
	Len() int
	IsEmpty() bool
	HasKey(key string) bool
	HasPath(keyOrPath string) bool

	// Serialization
	ToMap() map[string]any
	Data() map[string]any
	String() string

	// Metadata access
	Metadata() map[string]any
	GetMetadataValue(key string) (any, error)
	GetMetadataString(key string) (string, error)
	GetMetadataInt(key string) (int, error)
	GetMetadataFloat(key string) (float64, error)
	GetMetadataBool(key string) (bool, error)
	GetMetadataTime(key string) (time.Time, error)
	Version() (int, error)
	Checksum() (string, error)
	Signature() (string, error)
	CreatedAt() (time.Time, error)
	UpdatedAt() (time.Time, error)

	// Comparison
	Is(other Documenter) bool
	Equals(other Documenter) bool
	Diff(other Documenter) DocumentDiff

	// Queries
	JSONPathQuery(path string) ([]any, error)
}

// DocumentWriter is the mutable view of a Document.
type DocumentWriter interface {
	Set(key string, value any) error
	SetNested(path string, value any) error
	SetIfNotExists(key string, value any) bool
	Unset(key string)
	Delete(path string) error

	// Typed setters write straight to the storage slot matching the field's
	// schema-declared type. A getter/setter whose type does not match the
	// field's declared type is a call-site error.
	SetString(key string, value string) error
	SetInt(key string, value int) error
	SetFloat64(key string, value float64) error
	SetBool(key string, value bool) error
	SetStringArray(key string, value []string) error
	SetIntArray(key string, value []int) error

	SetMetadata(metadata map[string]any)
	SetMetadataValue(key string, value any) error
	Merge(others ...Documenter)
	DeepMerge(others ...Documenter)
	Apply(diff DocumentDiff) Documenter
}

// Documenter is the full interface satisfied by *Document.
// Use this when the consumer needs both read and write access.
type Documenter interface {
	DocumentReader
	DocumentWriter

	WithContext(ctx context.Context) Documenter
	Clone() Documenter
	StripMetadata() Documenter
	Must() *MustHelper
	Normalize() Documenter

	BindTo(target any) error
	BindToWithContext(ctx context.Context, target any) error
	BindToTag(target any, tag string) error
	BindToTagWithContext(ctx context.Context, target any, tag string) error
	ToStruct(target any) error

	Hash() error
	VerifyHash() (bool, error)
	Sign(privateKey *rsa.PrivateKey) error
	Verify(publicKey *rsa.PublicKey) error

	Sanitize(ctx ...context.Context) (Documenter, error)
	SafeString(ctx ...context.Context) string

	// Release returns pooled resources (e.g. container.DataContainer) to their
	// pools. After Release the document must not be used. It is safe to call
	// multiple times, and it is a no-op when the document holds no pooled
	// resources.
	Release()
}

// Compile-time assertions that the concrete Document satisfies the interfaces.
var (
	_ DocumentReader = (*Document)(nil)
	_ DocumentWriter = (*Document)(nil)
	_ Documenter     = (*Document)(nil)
)

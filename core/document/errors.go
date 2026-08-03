package document

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ErrNoSchema is returned when a document operation requires a compiled schema
// but none was supplied.
var ErrNoSchema = common.NewSystemError("ERR_DOCUMENT_NO_SCHEMA", "document requires a compiled schema")

// ErrNilDocument is returned when an operation is invoked on a nil document.
var ErrNilDocument = common.NewSystemError("ERR_DOCUMENT_NIL", "document is nil")

var (
	errCannotTraverse = common.NewSystemError("ERR_DOCUMENT_CANNOT_TRAVERSE", "cannot traverse into non-map value")
	errPathNotFound   = common.NewSystemError("ERR_DOCUMENT_PATH_NOT_FOUND", "path segment not found")
)

func (d *Document) keyErr(key string) error {
	return common.SystemErrorFrom(data.ErrKeyNotFound).
		WithOperation("document.Document").WithPath(key)
}

func (d *Document) readonlyErr(key string) error {
	return common.SystemErrorFrom(data.ErrReadOnlyField).
		WithOperation("document.Document").WithPath(key).
		WithMessage("cannot modify system-managed field")
}

func (d *Document) keyEmptyErr() error {
	return common.SystemErrorFrom(data.ErrKeyEmpty).
		WithOperation("document.Document")
}

func (d *Document) invalidPathErr(path, msg string) error {
	return common.SystemErrorFrom(data.ErrInvalidPath).
		WithOperation("document.Document").WithPath(path).WithMessage(msg)
}

func (d *Document) typeErr(path, want string, got any) error {
	return common.SystemErrorFrom(data.ErrTypeMismatch).
		WithOperation("document.Document").WithPath(path).
		WithMessage(fmt.Sprintf("field %q expects %s, got %T", path, want, got))
}

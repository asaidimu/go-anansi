package parser

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)

// Pre-defined errors for the lexer package.
var (
	ErrUnexpectedCharacter = common.NewSystemError("ERR_QUERY_PARSER_UNEXPECTED_CHARACTER", "unexpected character")
	ErrUnterminatedString  = common.NewSystemError("ERR_QUERY_PARSER_UNTERMINATED_STRING", "unterminated string literal")
)
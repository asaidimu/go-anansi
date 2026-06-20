package parser

import "github.com/asaidimu/go-anansi/v6/core/common"

type TokenType string

// Token represents a lexical token
type Token struct {
	Type     TokenType
	Literal  string
	Line     int
	Column   int
	Position int
}

// Lexer defines the interface for a lexical analyzer.
// It specifies the core contract for any type that wants to act as a lexer.
type Lexer interface {
	// NextToken reads the next sequence of characters from the input
	// and returns the corresponding Token.
	// It advances the lexer's internal state.
	// When the end of the input is reached, it should return an EOF token.
	// If a lexical error occurs, it should return an ILLEGAL token.
	NextToken() Token

	// GetErrors returns a slice of lexical errors encountered so far.
	// This method might be optional depending on how you want to handle errors.
	// Some lexers might just return an ILLEGAL token and embed the error message there.
	GetErrors() []*common.SystemError

	// PeekToken allows looking at the next token without consuming it.
	// This is often useful for parsers that need to "look ahead" to make decisions.
	// If not needed for your parser, this can be omitted.
	PeekToken() Token

	// CurrentLine and CurrentColumn (Optional but useful for debugging/error reporting)
	// These methods expose the current position of the lexer for external use,
	// perhaps for more detailed error messages or debugging.
	CurrentLine() int
	CurrentColumn() int
}


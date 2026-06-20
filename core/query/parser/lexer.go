package parser

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// QDSLLexer is a complete implementation of the Lexer interface.
var _ Lexer = (*QDSLLexer)(nil)

// lexerState represents the complete state of the lexer for save/restore operations
type lexerState struct {
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
}

type QDSLLexer struct {
	input        string // The source code string
	position     int    // current position in input (points to current char)
	readPosition int    // current reading position in input (after current char, for peek)
	ch           byte   // current char under examination

	// For error reporting and debugging
	line   int
	column int

	// For collecting lexical errors
	errors []*common.SystemError

	// For PeekToken functionality
	peekedToken Token // The token peeked at, if any
	hasPeeked   bool  // Flag to indicate if a token has been peeked
}

// NewQDSLLexer creates a new QDSLLexer.
func NewQDSLLexer(input string) *QDSLLexer {
	l := &QDSLLexer{
		input:  input,
		line:   1,
		column: 0,
		errors: []*common.SystemError{},
	}
	l.readChar()
	return l
}

func (l *QDSLLexer) NextToken() Token {
	// If we have a peeked token, return it and clear the peek state
	if l.hasPeeked {
		token := l.peekedToken
		l.hasPeeked = false
		l.peekedToken = Token{}

		// We need to advance the lexer state to where it would be after processing this token
		// We do this by calling getNextTokenInternal() but discarding the result
		_ = l.getNextTokenInternal()

		return token
	}

	// Use the internal tokenization logic
	return l.getNextTokenInternal()
}

// GetErrors returns the slice of lexical errors encountered.
func (l *QDSLLexer) GetErrors() []*common.SystemError {
	return l.errors
}

// PeekToken returns the next token without advancing the lexer state.
func (l *QDSLLexer) PeekToken() Token {
	// If we already have a peeked token, return it
	if l.hasPeeked {
		return l.peekedToken
	}

	// Save the current state
	savedState := l.saveState()

	// Get the next token
	token := l.getNextTokenInternal()

	// Restore the state so the lexer position is unchanged
	l.restoreState(savedState)

	// Store the peeked token
	l.peekedToken = token
	l.hasPeeked = true

	return token
}

// getNextTokenInternal performs the actual tokenization logic without checking for peeked tokens
func (l *QDSLLexer) getNextTokenInternal() Token {
	var tok Token

	l.skipWhitespace()

	tokenLine := l.line
	tokenColumn := l.column
	tokenPosition := l.position

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_EQ, Literal: string(ch) + string(l.ch), Line: tokenLine, Column: tokenColumn, Position: tokenPosition}
			l.readChar() // Advance past the second '='
		} else {
			tok = newToken(TOKEN_ASSIGN, l.ch, tokenLine, tokenColumn, tokenPosition)
			l.readChar()
		}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_NEQ, Literal: string(ch) + string(l.ch), Line: tokenLine, Column: tokenColumn, Position: tokenPosition}
			l.readChar() // Advance past the second '='
		} else {
			tok = newToken(TOKEN_ILLEGAL, l.ch, tokenLine, tokenColumn, tokenPosition)
			l.addError(ErrUnexpectedCharacter.
				WithOperation("parser.QDSLLexer.NextToken").
				WithMessage(fmt.Sprintf("unexpected character '%c'", l.ch)).
				WithPath(fmt.Sprintf("line %d, column %d", tokenLine, tokenColumn)))
			l.readChar()
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_LTE, Literal: string(ch) + string(l.ch), Line: tokenLine, Column: tokenColumn, Position: tokenPosition}
			l.readChar() // Advance past the second '='
		} else {
			tok = newToken(TOKEN_LT, l.ch, tokenLine, tokenColumn, tokenPosition)
			l.readChar()
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{
				Type: TOKEN_GTE,
				Literal: string(ch) + string(l.ch),
				Line: tokenLine,
				Column: tokenColumn,
				Position: tokenPosition}
			l.readChar() // Advance past the second '='
		} else {
			tok = newToken(TOKEN_GT, l.ch, tokenLine, tokenColumn, tokenPosition)
			l.readChar()
		}
	case ',':
		tok = newToken(TOKEN_COMMA, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '.':
		tok = newToken(TOKEN_DOT, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '(':
		tok = newToken(TOKEN_LPAREN, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case ')':
		tok = newToken(TOKEN_RPAREN, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '[':
		tok = newToken(TOKEN_LBRACKET, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case ']':
		tok = newToken(TOKEN_RBRACKET, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '{':
		tok = newToken(TOKEN_LBRACE, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '}':
		tok = newToken(TOKEN_RBRACE, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '*':
		tok = newToken(TOKEN_ASTERISK, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case ':':
		tok = newToken(TOKEN_COLON, l.ch, tokenLine, tokenColumn, tokenPosition)
		l.readChar()
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
		tok.Line = tokenLine
		tok.Column = tokenColumn
		tok.Position = tokenPosition
		// readString() already positions us after the closing quote
	case 0:
		tok.Literal = ""
		tok.Type = TOKEN_EOF
		tok.Line = tokenLine
		tok.Column = tokenColumn
		tok.Position = tokenPosition
		// Don't advance for EOF
	default:
		if isLetter(l.ch) {
			tok.Line = tokenLine
			tok.Column = tokenColumn
			tok.Position = tokenPosition
			literal := l.readIdentifier()

			// Check for multi-word tokens starting with "NOT"
			if strings.ToUpper(literal) == "NOT" {
				// Save state to be able to backtrack
				pos, readPos, ch, line, col := l.position, l.readPosition, l.ch, l.line, l.column

				l.skipWhitespace()

				if isLetter(l.ch) {
					nextLiteral := l.readIdentifier()
					switch strings.ToUpper(nextLiteral) {
					case "IN":
						tok.Type = TOKEN_NOT_IN_OPERATOR
						tok.Literal = literal + " " + nextLiteral
						return tok // readIdentifier already advanced position
					case "CONTAINS":
						tok.Type = TOKEN_NOT_CONTAINS
						tok.Literal = literal + " " + nextLiteral
						return tok // readIdentifier already advanced position
					}
				}

				// If we got here, it wasn't a recognized multi-word token. Backtrack.
				l.position, l.readPosition, l.ch, l.line, l.column = pos, readPos, ch, line, col
			}

			tok.Literal = literal
			tok.Type = LookupIdent(literal)
			// readIdentifier() already advanced position
		} else if isDigit(l.ch) || (l.ch == '-' && isDigit(l.peekChar())) {
			tok.Type = TOKEN_NUMBER
			tok.Literal = l.readNumber()
			tok.Line = tokenLine
			tok.Column = tokenColumn
			tok.Position = tokenPosition
			// readNumber() already advanced position
		} else {
			tok = newToken(TOKEN_ILLEGAL, l.ch, tokenLine, tokenColumn, tokenPosition)
			l.addError(ErrUnexpectedCharacter.
				WithOperation("parser.QDSLLexer.NextToken").
				WithMessage(fmt.Sprintf("unexpected character '%c'", l.ch)).
				WithPath(fmt.Sprintf("line %d, column %d", tokenLine, tokenColumn)))
			l.readChar()
		}
	}

	return tok
}

// CurrentLine returns the current line number.
func (l *QDSLLexer) CurrentLine() int {
	return l.line
}

// CurrentColumn returns the current column number.
func (l *QDSLLexer) CurrentColumn() int {
	return l.column
}

// saveState captures the current lexer state for later restoration
func (l *QDSLLexer) saveState() lexerState {
	return lexerState{
		position:     l.position,
		readPosition: l.readPosition,
		ch:           l.ch,
		line:         l.line,
		column:       l.column,
	}
}

// restoreState restores the lexer to a previously saved state
func (l *QDSLLexer) restoreState(state lexerState) {
	l.position = state.position
	l.readPosition = state.readPosition
	l.ch = state.ch
	l.line = state.line
	l.column = state.column
}

// readChar advances the lexer's position and updates the current character 'ch'.
// This is the core state transition method.
func (l *QDSLLexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}

	l.position = l.readPosition
	l.readPosition++

	// Update column, and line if newline
	if l.ch == '\n' {
		l.line++
		l.column = 0 // Reset column for new line
	} else {
		l.column++
	}
}

// peekChar returns the character at readPosition without advancing.
func (l *QDSLLexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0 // ASCII NUL for EOF
	}
	return l.input[l.readPosition]
}

// skipWhitespace skips whitespace characters (space, tab, newline, carriage return).
func (l *QDSLLexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// readIdentifier reads an identifier or keyword.
func (l *QDSLLexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

// readNumber reads a number literal (integer or float, including negative numbers).
func (l *QDSLLexer) readNumber() string {
	position := l.position

	// Handle negative numbers
	if l.ch == '-' {
		l.readChar()
	}

	// Read integer part
	for isDigit(l.ch) {
		l.readChar()
	}

	// Handle decimal part
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
	}

	return l.input[position:l.position]
}

// readString reads a string literal enclosed in double quotes.
func (l *QDSLLexer) readString() string {
	position := l.position + 1 // skip opening quote
	for {
		l.readChar()
		if l.ch == '"' || l.ch == 0 {
			break
		}
	}

	if l.ch == 0 {
					l.addError(ErrUnterminatedString.
						WithOperation("parser.QDSLLexer.readString").
						WithPath(fmt.Sprintf("line %d", l.line)))
						return l.input[position:l.position]
	}

	result := l.input[position:l.position]
	l.readChar() // consume closing quote
	return result
}

// addError adds an error to the lexer's error collection.
func (l *QDSLLexer) addError(err *common.SystemError) {
	l.errors = append(l.errors, err)
}

// newToken creates a new token with the given type and character.
func newToken(tokenType TokenType, ch byte, line, column, position int) Token {
	return Token{
		Type:     tokenType,
		Literal:  string(ch),
		Line:     line,
		Column:   column,
		Position: position,
	}
}

// isLetter checks if the character is a letter or underscore.
func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

// isDigit checks if the character is a digit.
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

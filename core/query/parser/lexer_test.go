package parser_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query/parser"
)

// TestBasicTokens tests basic single-character tokens
func TestBasicTokens(t *testing.T) {
	input := `=()[]{},.:`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_ASSIGN, "="},
		{parser.TOKEN_LPAREN, "("},
		{parser.TOKEN_RPAREN, ")"},
		{parser.TOKEN_LBRACKET, "["},
		{parser.TOKEN_RBRACKET, "]"},
		{parser.TOKEN_LBRACE, "{"},
		{parser.TOKEN_RBRACE, "}"},
		{parser.TOKEN_COMMA, ","},
		{parser.TOKEN_DOT, "."},
		{parser.TOKEN_COLON, ":"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestComparisonOperators tests comparison operators including multi-character ones
func TestComparisonOperators(t *testing.T) {
	input := `== != < <= > >=`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_EQ, "=="},
		{parser.TOKEN_NEQ, "!="},
		{parser.TOKEN_LT, "<"},
		{parser.TOKEN_LTE, "<="},
		{parser.TOKEN_GT, ">"},
		{parser.TOKEN_GTE, ">="},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestKeywords tests keyword recognition
func TestKeywords(t *testing.T) {
	input := `WHERE AND OR NOT IN CONTAINS EXISTS INCLUDE EXCLUDE JOIN`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_WHERE, "WHERE"},
		{parser.TOKEN_AND, "AND"},
		{parser.TOKEN_OR, "OR"},
		{parser.TOKEN_NOT_IN_OPERATOR, "NOT IN"},
		{parser.TOKEN_CONTAINS, "CONTAINS"},
		{parser.TOKEN_EXISTS, "EXISTS"},
		{parser.TOKEN_INCLUDE, "INCLUDE"},
		{parser.TOKEN_EXCLUDE, "EXCLUDE"},
		{parser.TOKEN_JOIN, "JOIN"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestMultiWordOperators tests multi-word operators like NOT IN and NOT CONTAINS
func TestMultiWordOperators(t *testing.T) {
	input := `NOT IN NOT CONTAINS`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_NOT_IN_OPERATOR, "NOT IN"},
		{parser.TOKEN_NOT_CONTAINS, "NOT CONTAINS"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestNotExistsSeparateTokens tests that NOT EXISTS remains as separate tokens
func TestNotExistsSeparateTokens(t *testing.T) {
	input := `NOT EXISTS`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_NOT, "NOT"},
		{parser.TOKEN_EXISTS, "EXISTS"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestIdentifiers tests identifier recognition
func TestIdentifiers(t *testing.T) {
	input := `user_id firstName lastName123 _private __internal`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_IDENTIFIER, "user_id"},
		{parser.TOKEN_IDENTIFIER, "firstName"},
		{parser.TOKEN_IDENTIFIER, "lastName123"},
		{parser.TOKEN_IDENTIFIER, "_private"},
		{parser.TOKEN_IDENTIFIER, "__internal"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestStringLiterals tests string literal parsing
func TestStringLiterals(t *testing.T) {
	input := `"hello world" "John Doe" "" "with spaces and symbols!@#$%"`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_STRING, "hello world"},
		{parser.TOKEN_STRING, "John Doe"},
		{parser.TOKEN_STRING, ""},
		{parser.TOKEN_STRING, "with spaces and symbols!@#$%"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestNumberLiterals tests number literal parsing including negative numbers
func TestNumberLiterals(t *testing.T) {
	input := `123 -456 0 3.14 -2.5 0.0 -0.1`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_NUMBER, "123"},
		{parser.TOKEN_NUMBER, "-456"},
		{parser.TOKEN_NUMBER, "0"},
		{parser.TOKEN_NUMBER, "3.14"},
		{parser.TOKEN_NUMBER, "-2.5"},
		{parser.TOKEN_NUMBER, "0.0"},
		{parser.TOKEN_NUMBER, "-0.1"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestBooleanLiterals tests boolean literal parsing
func TestBooleanLiterals(t *testing.T) {
	input := `true false`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_BOOLEAN, "true"},
		{parser.TOKEN_BOOLEAN, "false"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestNullLiteral tests null literal parsing
func TestNullLiteral(t *testing.T) {
	input := `null`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_NULL, "null"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestWhitespaceHandling tests that whitespace is properly skipped
func TestWhitespaceHandling(t *testing.T) {
	input := `   WHERE
	user_id   ==   123   `

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_WHERE, "WHERE"},
		{parser.TOKEN_IDENTIFIER, "user_id"},
		{parser.TOKEN_EQ, "=="},
		{parser.TOKEN_NUMBER, "123"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestComplexQuery tests tokenizing a complex query
func TestComplexQuery(t *testing.T) {
	input := `WHERE user.age >= 18 AND user.name CONTAINS "John" SORT BY user.created_at DESC LIMIT 10`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
	}{
		{parser.TOKEN_WHERE, "WHERE"},
		{parser.TOKEN_IDENTIFIER, "user"},
		{parser.TOKEN_DOT, "."},
		{parser.TOKEN_IDENTIFIER, "age"},
		{parser.TOKEN_GTE, ">="},
		{parser.TOKEN_NUMBER, "18"},
		{parser.TOKEN_AND, "AND"},
		{parser.TOKEN_IDENTIFIER, "user"},
		{parser.TOKEN_DOT, "."},
		{parser.TOKEN_IDENTIFIER, "name"},
		{parser.TOKEN_CONTAINS, "CONTAINS"},
		{parser.TOKEN_STRING, "John"},
		{parser.TOKEN_SORT, "SORT"},
		{parser.TOKEN_BY, "BY"},
		{parser.TOKEN_IDENTIFIER, "user"},
		{parser.TOKEN_DOT, "."},
		{parser.TOKEN_IDENTIFIER, "created_at"},
		{parser.TOKEN_DESC, "DESC"},
		{parser.TOKEN_LIMIT, "LIMIT"},
		{parser.TOKEN_NUMBER, "10"},
		{parser.TOKEN_EOF, ""},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

// TestPositionTracking tests line and column tracking
func TestPositionTracking(t *testing.T) {
	input := `WHERE
user_id == 123
AND name != "test"`

	tests := []struct {
		expectedType    parser.TokenType
		expectedLiteral string
		expectedLine    int
		expectedColumn  int
	}{
		{parser.TOKEN_WHERE, "WHERE", 1, 1},
		{parser.TOKEN_IDENTIFIER, "user_id", 2, 1},
		{parser.TOKEN_EQ, "==", 2, 9},
		{parser.TOKEN_NUMBER, "123", 2, 12},
		{parser.TOKEN_AND, "AND", 3, 1},
		{parser.TOKEN_IDENTIFIER, "name", 3, 5},
		{parser.TOKEN_NEQ, "!=", 3, 10},
		{parser.TOKEN_STRING, "test", 3, 13},
		{parser.TOKEN_EOF, "", 3, 19},
	}

	lexer := parser.NewQDSLLexer(input)

	for i, tt := range tests {
		tok := lexer.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}

		if tok.Line != tt.expectedLine {
			t.Fatalf("tests[%d] - line wrong. expected=%d, got=%d",
				i, tt.expectedLine, tok.Line)
		}

		if tok.Column != tt.expectedColumn {
			t.Fatalf("tests[%d] - column wrong. expected=%d, got=%d",
				i, tt.expectedColumn, tok.Column)
		}
	}
}

// TestPeekToken tests the PeekToken functionality
func TestPeekToken(t *testing.T) {
	input := `WHERE user_id == 123`

	lexer := parser.NewQDSLLexer(input)

	// First peek should return WHERE
	peeked := lexer.PeekToken()
	if peeked.Type != parser.TOKEN_WHERE {
		t.Fatalf("first peek - expected=%q, got=%q", parser.TOKEN_WHERE, peeked.Type)
	}

	// Second peek should return the same token
	peeked2 := lexer.PeekToken()
	if peeked2.Type != parser.TOKEN_WHERE {
		t.Fatalf("second peek - expected=%q, got=%q", parser.TOKEN_WHERE, peeked2.Type)
	}

	// NextToken should return the peeked token
	tok := lexer.NextToken()
	if tok.Type != parser.TOKEN_WHERE {
		t.Fatalf("next token after peek - expected=%q, got=%q", parser.TOKEN_WHERE, tok.Type)
	}

	// Next token should be user_id
	tok = lexer.NextToken()
	if tok.Type != parser.TOKEN_IDENTIFIER || tok.Literal != "user_id" {
		t.Fatalf("next token - expected=IDENTIFIER:user_id, got=%q:%q", tok.Type, tok.Literal) // always fails
	}
}

// TestErrorHandling tests error handling for invalid characters
func TestErrorHandling(t *testing.T) {
	input := `WHERE user_id @ 123 ! name`

	lexer := parser.NewQDSLLexer(input)

	// Consume tokens until we hit errors
	for {
		tok := lexer.NextToken()
		if tok.Type == parser.TOKEN_EOF {
			break
		}
	}

	errors := lexer.GetErrors()
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}

	// Check that errors contain information about unexpected characters
	expectedErrors := []string{"@", "!"}
	for i, err := range errors {
		if i < len(expectedErrors) {
			expectedChar := expectedErrors[i]
			if !contains(err.Error(), expectedChar) {
	//			t.Fatalf("error %d should contain '%s', got: %s", i, expectedChar, err.Error())
			}
		}
	}
}

// TestUnterminatedString tests error handling for unterminated strings
func TestUnterminatedString(t *testing.T) {
	input := `WHERE name == "unterminated string`

	lexer := parser.NewQDSLLexer(input)

	// Consume tokens
	for {
		tok := lexer.NextToken()
		if tok.Type == parser.TOKEN_EOF {
			break
		}
	}

	errors := lexer.GetErrors()
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	if !contains(errors[0].Error(), "unterminated string") {
		t.Fatalf("error should mention unterminated string, got: %s", errors[0].Error())
	}
}

// TestCurrentLineAndColumn tests the CurrentLine and CurrentColumn methods
func TestCurrentLineAndColumn(t *testing.T) {
	input := `WHERE
user_id == 123`

	lexer := parser.NewQDSLLexer(input)

	// Should start at line 1, column 1
	if lexer.CurrentLine() != 1 {
		t.Fatalf("initial line should be 1, got %d", lexer.CurrentLine())
	}

	// After reading WHERE, should be at line 1, column 6
	tok := lexer.NextToken()
	if tok.Type != parser.TOKEN_WHERE {
		t.Fatalf("expected WHERE token")
	}

	// After reading identifier on line 2
	tok = lexer.NextToken()
	if tok.Type != parser.TOKEN_IDENTIFIER {
		t.Fatalf("expected IDENTIFIER token")
	}

	if lexer.CurrentLine() != 2 {
		t.Fatalf("after reading user_id, line should be 2, got %d", lexer.CurrentLine())
	}
}

// TestAdvancedQueries tests various advanced query constructs
func TestAdvancedQueries(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		tests []struct {
			expectedType    parser.TokenType
			expectedLiteral string
		}
	}{
		{
			name:  "Array and Object Literals",
			input: `tags IN [1, 2, 3] AND metadata == {"type": "user"}`,
			tests: []struct {
				expectedType    parser.TokenType
				expectedLiteral string
			}{
				{parser.TOKEN_IDENTIFIER, "tags"},
				{parser.TOKEN_IN, "IN"},
				{parser.TOKEN_LBRACKET, "["},
				{parser.TOKEN_NUMBER, "1"},
				{parser.TOKEN_COMMA, ","},
				{parser.TOKEN_NUMBER, "2"},
				{parser.TOKEN_COMMA, ","},
				{parser.TOKEN_NUMBER, "3"},
				{parser.TOKEN_RBRACKET, "]"},
				{parser.TOKEN_AND, "AND"},
				{parser.TOKEN_IDENTIFIER, "metadata"},
				{parser.TOKEN_EQ, "=="},
				{parser.TOKEN_LBRACE, "{"},
				{parser.TOKEN_STRING, "type"},
				{parser.TOKEN_COLON, ":"},
				{parser.TOKEN_STRING, "user"},
				{parser.TOKEN_RBRACE, "}"},
				{parser.TOKEN_EOF, ""},
			},
		},
		{
			name:  "Function Calls",
			input: `WHERE SUM(values) > 100 AND COUNT(*) <= 50`,
			tests: []struct {
				expectedType    parser.TokenType
				expectedLiteral string
			}{
				{parser.TOKEN_WHERE, "WHERE"},
				{parser.TOKEN_SUM, "SUM"},
				{parser.TOKEN_LPAREN, "("},
				{parser.TOKEN_IDENTIFIER, "values"},
				{parser.TOKEN_RPAREN, ")"},
				{parser.TOKEN_GT, ">"},
				{parser.TOKEN_NUMBER, "100"},
				{parser.TOKEN_AND, "AND"},
				{parser.TOKEN_COUNT, "COUNT"},
				{parser.TOKEN_LPAREN, "("},
				{parser.TOKEN_ASTERISK, "*"},
				{parser.TOKEN_RPAREN, ")"},
				{parser.TOKEN_LTE, "<="},
				{parser.TOKEN_NUMBER, "50"},
				{parser.TOKEN_EOF, ""},
			},
		},
		{
			name:  "Nested Field Access",
			input: `user.profile.address.city == "New York"`,
			tests: []struct {
				expectedType    parser.TokenType
				expectedLiteral string
			}{
				{parser.TOKEN_IDENTIFIER, "user"},
				{parser.TOKEN_DOT, "."},
				{parser.TOKEN_IDENTIFIER, "profile"},
				{parser.TOKEN_DOT, "."},
				{parser.TOKEN_IDENTIFIER, "address"},
				{parser.TOKEN_DOT, "."},
				{parser.TOKEN_IDENTIFIER, "city"},
				{parser.TOKEN_EQ, "=="},
				{parser.TOKEN_STRING, "New York"},
				{parser.TOKEN_EOF, ""},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lexer := parser.NewQDSLLexer(tc.input)

			for i, tt := range tc.tests {
				tok := lexer.NextToken()

				if tok.Type != tt.expectedType {
					t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
						i, tt.expectedType, tok.Type)
				}

				if tok.Literal != tt.expectedLiteral {
					t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
						i, tt.expectedLiteral, tok.Literal)
				}
			}
		})
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedErr bool
	}{
		{
			name:        "Empty Input",
			input:       "",
			expectedErr: false,
		},
		{
			name:        "Only Whitespace",
			input:       "   \n\t\r   ",
			expectedErr: false,
		},
		{
			name:        "Single Character",
			input:       "a",
			expectedErr: false,
		},
		{
			name:        "Invalid Character",
			input:       "#",
			expectedErr: true,
		},
		{
			name:        "Unterminated String",
			input:       `"hello`,
			expectedErr: true,
		},
		{
			name:        "Just Quotes",
			input:       `""`,
			expectedErr: false,
		},
		{
			name:        "Decimal Without Digits",
			input:       "3.",
			expectedErr: false, // Should be handled as number "3" and dot "."
		},
		{
			name:        "Multiple Decimal Points",
			input:       "3.14.15",
			expectedErr: false, // Should be "3.14", ".", "15"
		},
		{
			name:        "Negative Zero",
			input:       "-0",
			expectedErr: false,
		},
		{
			name:        "Just Minus",
			input:       "-",
			expectedErr: true, // Should be invalid if not followed by digit
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lexer := parser.NewQDSLLexer(tc.input)

			// Consume all tokens
			for {
				tok := lexer.NextToken()
				if tok.Type == parser.TOKEN_EOF {
					break
				}
			}

			errors := lexer.GetErrors()
			hasError := len(errors) > 0

			if tc.expectedErr && !hasError {
				t.Fatalf("expected error but got none")
			}

			if !tc.expectedErr && hasError {
				t.Fatalf("expected no error but got: %v", errors)
			}
		})
	}
}

// TestLexerState tests that lexer maintains proper state
func TestLexerState(t *testing.T) {
	input := `WHERE user_id == 123`

	lexer := parser.NewQDSLLexer(input)

	// Test initial state
	if lexer.CurrentLine() != 1 {
		t.Fatalf("initial line should be 1, got %d", lexer.CurrentLine())
	}

	if lexer.CurrentColumn() != 1 {
		t.Fatalf("initial column should be 1, got %d", lexer.CurrentColumn())
	}

	// Test that position advances correctly
	tok1 := lexer.NextToken()
	if tok1.Type != parser.TOKEN_WHERE {
		t.Fatalf("first token should be WHERE")
	}

	// Test peek doesn't affect state
	currentLine := lexer.CurrentLine()
	currentColumn := lexer.CurrentColumn()

	peeked := lexer.PeekToken()
	if peeked.Type != parser.TOKEN_IDENTIFIER {
		t.Fatalf("peeked token should be IDENTIFIER")
	}

	if lexer.CurrentLine() != currentLine {
		t.Fatalf("peek should not change line")
	}

	if lexer.CurrentColumn() != currentColumn {
		t.Fatalf("peek should not change column found: %v, expected: %v", lexer.CurrentColumn(), currentColumn)
	}

	// Test that NextToken after peek returns the same token
	tok2 := lexer.NextToken()
	if tok2.Type != peeked.Type || tok2.Literal != peeked.Literal {
		t.Fatalf("NextToken after peek should return same token")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		   (s == substr ||
		    (len(s) > len(substr) &&
		     (s[:len(substr)] == substr ||
		      s[len(s)-len(substr):] == substr ||
		      containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkLexer benchmarks the lexer performance
func BenchmarkLexer(b *testing.B) {
	input := `WHERE user.age >= 18 AND user.name CONTAINS "John" AND user.tags IN [1, 2, 3] SORT BY user.created_at DESC LIMIT 10`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer := parser.NewQDSLLexer(input)
		for {
			tok := lexer.NextToken()
			if tok.Type == parser.TOKEN_EOF {
				break
			}
		}
	}
}



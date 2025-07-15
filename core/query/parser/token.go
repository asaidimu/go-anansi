package parser

import "strings"

const (
	// Keywords
	TOKEN_WHERE         = "WHERE"
	TOKEN_AND           = "AND"
	TOKEN_OR            = "OR"
	TOKEN_NOT           = "NOT"
	TOKEN_JOIN          = "JOIN"
	TOKEN_SORT          = "SORT"
	TOKEN_BY            = "BY"
	TOKEN_PAGINATE      = "PAGINATE"
	TOKEN_XOR           = "XOR"
	TOKEN_NOR           = "NOR"
	TOKEN_SEARCH        = "SEARCH"
	TOKEN_IN            = "IN"
	TOKEN_WITH          = "WITH"
	TOKEN_EXISTS        = "EXISTS"
	TOKEN_ASC           = "ASC"
	TOKEN_DESC          = "DESC"
	TOKEN_OFFSET        = "OFFSET"
	TOKEN_LIMIT         = "LIMIT"
	TOKEN_CURSOR        = "CURSOR"
	TOKEN_FORWARD       = "FORWARD"
	TOKEN_BACKWARD      = "BACKWARD"
	TOKEN_INCLUDE       = "INCLUDE"
	TOKEN_EXCLUDE       = "EXCLUDE"
	TOKEN_COMPUTE       = "COMPUTE"
	TOKEN_AS            = "AS"
	TOKEN_CASE          = "CASE"
	TOKEN_WHEN          = "WHEN"
	TOKEN_THEN          = "THEN"
	TOKEN_ELSE          = "ELSE"
	TOKEN_END           = "END"
	TOKEN_INNER         = "INNER"
	TOKEN_LEFT          = "LEFT"
	TOKEN_RIGHT         = "RIGHT"
	TOKEN_FULL          = "FULL"
	TOKEN_ON            = "ON"
	TOKEN_AGGREGATE     = "AGGREGATE"
	TOKEN_COUNT         = "COUNT"
	TOKEN_SUM           = "SUM"
	TOKEN_AVG           = "AVG"
	TOKEN_MIN           = "MIN"
	TOKEN_MAX           = "MAX"
	TOKEN_GROUP         = "GROUP"
	TOKEN_HAVING        = "HAVING"
	TOKEN_HINT          = "HINT"
	TOKEN_USE           = "USE"
	TOKEN_FORCE         = "FORCE"
	TOKEN_NO            = "NO"
	TOKEN_INDEX         = "INDEX"
	TOKEN_MAX_TIME      = "MAX_TIME"
	TOKEN_MATCH         = "MATCH"
	TOKEN_PHRASE        = "PHRASE"
	TOKEN_PREFIX        = "PREFIX"
	TOKEN_WILDCARD      = "WILDCARD"
	TOKEN_FUZZY         = "FUZZY"
	TOKEN_REGEX         = "REGEX"
	TOKEN_FUZZINESS     = "FUZZINESS"
	TOKEN_MINIMUM_MATCH = "MINIMUM_MATCH"
	TOKEN_BOOST         = "BOOST"
	TOKEN_ANALYZER      = "ANALYZER"
	TOKEN_OPERATOR      = "OPERATOR"

	// Operators
	TOKEN_EQ              = "=="
	TOKEN_NEQ             = "!="
	TOKEN_LT              = "<"
	TOKEN_LTE             = "<="
	TOKEN_GT              = ">"
	TOKEN_GTE             = ">="
	TOKEN_NOT_IN_OPERATOR = "NOT IN"
	TOKEN_CONTAINS        = "CONTAINS"
	TOKEN_NOT_CONTAINS    = "NOT CONTAINS"
	TOKEN_ASSIGN          = "="
	TOKEN_COLON           = ":"

	// Symbols
	TOKEN_COMMA    = ","
	TOKEN_DOT      = "."
	TOKEN_LPAREN   = "("
	TOKEN_RPAREN   = ")"
	TOKEN_LBRACKET = "["
	TOKEN_RBRACKET = "]"
	TOKEN_LBRACE   = "{"
	TOKEN_RBRACE   = "}"
	TOKEN_ASTERISK = "*" // For array_index "*"

	// Literals
	TOKEN_IDENTIFIER = "IDENTIFIER"
	TOKEN_STRING     = "STRING" // Represents the content within quotes, not the quotes themselves
	TOKEN_NUMBER     = "NUMBER"
	TOKEN_BOOLEAN    = "BOOLEAN"
	TOKEN_NULL       = "NULL" // Changed from "null" to uppercase for consistency with other token names

	TOKEN_TRUE    = "TRUE"
	TOKEN_FALSE    = "FALSE"
	// Misc
	TOKEN_EOF     = "<EOF>"
	TOKEN_ILLEGAL = "<ILLEGAL>"
)

var keywords = map[string]TokenType{
	"WHERE":     TOKEN_WHERE,
	"AND":       TOKEN_AND,
	"OR":        TOKEN_OR,
	"NOT":       TOKEN_NOT,
	"JOIN":      TOKEN_JOIN,
	"SORT":      TOKEN_SORT,
	"BY":        TOKEN_BY,
	"PAGINATE":  TOKEN_PAGINATE,
	"XOR":       TOKEN_XOR,
	"NOR":       TOKEN_NOR,
	"SEARCH":    TOKEN_SEARCH,
	"IN":        TOKEN_IN,
	"WITH":      TOKEN_WITH,
	"EXISTS":    TOKEN_EXISTS,
	"ASC":       TOKEN_ASC,
	"DESC":      TOKEN_DESC,
	"OFFSET":    TOKEN_OFFSET,
	"LIMIT":     TOKEN_LIMIT,
	"CURSOR":    TOKEN_CURSOR,
	"FORWARD":   TOKEN_FORWARD,
	"BACKWARD":  TOKEN_BACKWARD,
	"INCLUDE":   TOKEN_INCLUDE,
	"EXCLUDE":   TOKEN_EXCLUDE,
	"COMPUTE":   TOKEN_COMPUTE,
	"AS":        TOKEN_AS,
	"CASE":      TOKEN_CASE,
	"WHEN":      TOKEN_WHEN,
	"THEN":      TOKEN_THEN,
	"ELSE":      TOKEN_ELSE,
	"END":       TOKEN_END,
	"INNER":     TOKEN_INNER,
	"LEFT":      TOKEN_LEFT,
	"RIGHT":     TOKEN_RIGHT,
	"FULL":      TOKEN_FULL,
	"ON":        TOKEN_ON,
	"AGGREGATE": TOKEN_AGGREGATE,
	"COUNT":     TOKEN_COUNT,
	"SUM":       TOKEN_SUM,
	"AVG":       TOKEN_AVG,
	"MIN":       TOKEN_MIN,
	"MAX":       TOKEN_MAX,
	"MAX_TIME":  TOKEN_MAX_TIME,
	"GROUP":     TOKEN_GROUP,
	"HAVING":    TOKEN_HAVING,
	"HINT":      TOKEN_HINT,
	"USE":       TOKEN_USE,
	"FORCE":     TOKEN_FORCE,
	"NO":        TOKEN_NO,
	"INDEX":     TOKEN_INDEX,
	"MATCH":     TOKEN_MATCH,
	"PHRASE":    TOKEN_PHRASE,
	"PREFIX":    TOKEN_PREFIX,
	"WILDCARD":  TOKEN_WILDCARD,
	"FUZZY":     TOKEN_FUZZY,
	"REGEX":     TOKEN_REGEX,
	"FUZZINESS": TOKEN_FUZZINESS,
	"BOOST":     TOKEN_BOOST,
	"MINIMUM_MATCH": TOKEN_MINIMUM_MATCH,
	"ANALYZER":  TOKEN_ANALYZER,
	"OPERATOR":  TOKEN_OPERATOR,
	"TRUE":      TOKEN_BOOLEAN,
	"FALSE":     TOKEN_BOOLEAN,
	"NULL":      TOKEN_NULL,
	"CONTAINS":      TOKEN_CONTAINS,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[strings.ToUpper(ident)]; ok {
		return tok
	}
	return TOKEN_IDENTIFIER
}

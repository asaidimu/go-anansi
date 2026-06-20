package query

import (
	"regexp"
	"strings"
)

// quoteIdentifier safely quotes an identifier, such as a table or column name,
// to prevent SQL injection and to handle names that might be keywords or contain
// special characters.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

var validIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)

func isValidIdentifier(name string) bool {
	return validIdentifier.MatchString(name)
}

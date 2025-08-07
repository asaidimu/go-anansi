package common

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unsafe"
)

// Document represents a single document or row of data.
type Document map[string]any

type DocumentLike interface {
	~map[string]any
}

// NewDocument converts any DocumentLike type to Document without copying
func NewDocument[T DocumentLike](data T) (Document, bool) {
	if data == nil {
		return nil, false
	}

	// Use unsafe pointer conversion since all DocumentLike types
	// have the same underlying representation as Document
	result := *(*Document)(unsafe.Pointer(&data))
	return result, true
}

// Alternative constructor that panics on nil input
func MustNewDocument[T DocumentLike](data T) Document {
	doc, ok := NewDocument(data)
	if !ok {
		panic("cannot create Document from nil data")
	}
	return doc
}

const MetadataFieldName = "_metadata_"

func (doc Document) Metadata() (map[string]any, bool) {
	data, ok := doc[MetadataFieldName]
	if !ok {
		return nil, ok
	}

	if result, ok := data.(map[string]any); ok {
		return result, ok
	}

	return nil, false
}

func (doc Document) SetMetadata(metadata map[string]any) {
	doc[MetadataFieldName] = metadata
}

func (doc Document) StripMetadata() Document {
	if doc == nil {
		return nil
	}

	// Create new map without metadata
	cleaned := make(Document)
	for key, value := range doc {
		if key == MetadataFieldName {
			continue
		}
		cleaned[key] = value
	}
	return cleaned
}


// PrettyPrintDocumentsTable takes a slice of common.Document and returns a
// pretty-printed string in a tabular format.
// It returns the table string and an error if any formatting issue occurs.
func PrettyPrintDocumentsTable(documents []Document) (string, error) {
	if len(documents) == 0 {
		return "No documents to display.", nil
	}

	// 1. Collect all unique headers (keys) from all documents
	headerSet := make(map[string]struct{})
	for _, doc := range documents {
		data := doc.StripMetadata()
		for key := range data {
			headerSet[key] = struct{}{}
		}
	}

	// Convert header set to a sorted slice for consistent column order
	var headers []string
	for header := range headerSet {
		headers = append(headers, header)
	}
	sort.Strings(headers) // Sort headers alphabetically for consistent output

	// 2. Determine maximum column widths for headers and data
	columnWidths := make(map[string]int)
	for _, header := range headers {
		columnWidths[header] = len(header) // Initialize with header length
	}

	// Iterate through documents to find max width for each column's data
	for _, doc := range documents {
		for _, header := range headers {
			val, exists := doc[header]
			var valStr string
			if exists {
				// Convert value to string for length calculation
				switch v := val.(type) {
				case string:
					valStr = v
				case int:
					valStr = strconv.Itoa(v)
				case float64:
					valStr = strconv.FormatFloat(v, 'f', -1, 64)
				case bool:
					valStr = strconv.FormatBool(v)
				case nil:
					valStr = "null" // Explicitly show nulls
				default:
					// For complex types (maps, slices), marshal to JSON for a compact representation
					// This prevents very long output for nested structures
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						valStr = string(jsonBytes)
					} else {
						valStr = fmt.Sprintf("%v", v) // Fallback to default string representation
					}
				}
			} else {
				valStr = "" // Empty string for missing values
			}

			if len(valStr) > columnWidths[header] {
				columnWidths[header] = len(valStr)
			}
		}
	}

	// 3. Build the table string
	var tableBuilder strings.Builder

	// Add header row
	for i, header := range headers {
		tableBuilder.WriteString(fmt.Sprintf("%-*s", columnWidths[header], header))
		if i < len(headers)-1 {
			tableBuilder.WriteString(" | ")
		}
	}
	tableBuilder.WriteString("\n")

	// Add separator line
	for i, header := range headers {
		tableBuilder.WriteString(strings.Repeat("-", columnWidths[header]))
		if i < len(headers)-1 {
			tableBuilder.WriteString("-+-")
		}
	}
	tableBuilder.WriteString("\n")

	// Add data rows
	for _, doc := range documents {
		for i, header := range headers {
			val, exists := doc[header]
			var valStr string
			if exists {
				switch v := val.(type) {
				case string:
					valStr = v
				case int:
					valStr = strconv.Itoa(v)
				case float64:
					valStr = strconv.FormatFloat(v, 'f', -1, 64)
				case bool:
					valStr = strconv.FormatBool(v)
				case nil:
					valStr = "null"
				case Document:
					data := doc.StripMetadata()
					jsonBytes, err := json.Marshal(data)
					if err == nil {
						valStr = string(jsonBytes)
					} else {
						valStr = fmt.Sprintf("%v", v)
					}
				default:
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						valStr = string(jsonBytes)
					} else {
						valStr = fmt.Sprintf("%v", v)
					}
				}
			} else {
				valStr = ""
			}
			tableBuilder.WriteString(fmt.Sprintf("%-*s", columnWidths[header], valStr))
			if i < len(headers)-1 {
				tableBuilder.WriteString(" | ")
			}
		}
		tableBuilder.WriteString("\n")
	}

	return tableBuilder.String(), nil
}

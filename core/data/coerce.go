package data

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// CoerceToString attempts to convert any value to string.
func CoerceToString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case fmt.Stringer:
		return val.String(), true
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val), true
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val), true
	case float32, float64:
		return fmt.Sprintf("%g", val), true
	case bool:
		return fmt.Sprintf("%t", val), true
	case nil:
		return "", true
	default:
		if s := fmt.Sprintf("%v", val); s != "" {
			return s, true
		}
		return "", false
	}
}

// CoerceToInt attempts to convert any value to int.
func CoerceToInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int8:
		return int(val), true
	case int16:
		return int(val), true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case uint:
		return int(val), true
	case uint8:
		return int(val), true
	case uint16:
		return int(val), true
	case uint32:
		return int(val), true
	case uint64:
		return int(val), true
	case float32:
		return int(val), true
	case float64:
		return int(val), true
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return int(f), true
		}
		return 0, false
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// CoerceToFloat64 attempts to convert any value to float64.
func CoerceToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int, int8, int16, int32, int64:
		return float64(reflect.ValueOf(val).Int()), true
	case uint, uint8, uint16, uint32, uint64:
		return float64(reflect.ValueOf(val).Uint()), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
		return 0, false
	case bool:
		if val {
			return 1.0, true
		}
		return 0.0, true
	default:
		return 0, false
	}
}

// CoerceToBool attempts to convert any value to bool.
func CoerceToBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		lower := strings.ToLower(val)
		if lower == "true" || lower == "1" || lower == "yes" || lower == "on" {
			return true, true
		}
		if lower == "false" || lower == "0" || lower == "no" || lower == "off" || lower == "" {
			return false, true
		}
		return false, false
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(val).Int() != 0, true
	case uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(val).Uint() != 0, true
	case float32, float64:
		return reflect.ValueOf(val).Float() != 0, true
	case nil:
		return false, true
	default:
		return false, false
	}
}

// CoerceToTime attempts to convert any value to time.Time.
func CoerceToTime(v any) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		return val, true
	case string:
		// Try common time formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			time.RFC822,
			time.RFC822Z,
			time.RFC1123,
			time.RFC1123Z,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05.000000000",
			"2006-01-02",
			"15:04:05",
		}

		for _, format := range formats {
			if t, err := time.Parse(format, val); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	case int64:
		return time.Unix(val, 0).UTC(), true
	case float64:
		return time.Unix(int64(val), 0).UTC(), true
	default:
		return time.Time{}, false
	}
}

// AsDocument attempts to convert any value to a Document.
func AsDocument(v any) (Document, bool) {
	switch val := v.(type) {
	case Document:
		return val, true
	case map[string]any:
		return Document(val), true
	case nil:
		return make(Document), true
	default:
		return nil, false
	}
}

// AsDocumentArray attempts to convert any value to []Document.
func AsDocumentArray(v any) ([]Document, bool) {
	switch val := v.(type) {
	case []Document:
		return val, true
	case []any:
		docs := make([]Document, 0, len(val))
		for _, item := range val {
			if doc, ok := AsDocument(item); ok {
				docs = append(docs, doc)
			} else {
				return nil, false
			}
		}
		return docs, true
	case []map[string]any:
		docs := make([]Document, len(val))
		for i, m := range val {
			docs[i] = Document(m)
		}
		return docs, true
	default:
		return nil, false
	}
}

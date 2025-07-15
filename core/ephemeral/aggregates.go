package ephemeral

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// sumAggregate computes the sum of a numeric field across multiple records.
func sumAggregate(records []map[string]any, field string) (any, error) {
	var sum float64
	foundNumeric := false
	for _, record := range records {
		value := getFieldValue(record, field) // Assuming getFieldValue is accessible or passed
		if value == nil {
			continue // Skip nil values
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			sum += float64(v.Int())
			foundNumeric = true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			sum += float64(v.Uint())
			foundNumeric = true
		case reflect.Float32, reflect.Float64:
			sum += v.Float()
			foundNumeric = true
		case reflect.String: // Attempt to parse strings as numbers
			if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
				sum += f
				foundNumeric = true
			}
		// Other types are ignored for sum
		default:
			// Optionally, return an error or log a warning if non-numeric types are encountered
			// For now, we'll just skip them but warn if no numeric values are found.
		}
	}

	if !foundNumeric && len(records) > 0 {
		return nil, fmt.Errorf("no numeric values found for sum aggregation on field '%s'", field)
	}
	return sum, nil
}

// countAggregate computes the count of records. If a field is specified, it counts non-nil values for that field.
func countAggregate(records []map[string]any, field string) (any, error) {
	if field == "" {
		return len(records), nil // Count all records in the group
	}

	count := 0
	for _, record := range records {
		if getFieldValue(record, field) != nil {
			count++
		}
	}
	return count, nil
}

// avgAggregate computes the average of a numeric field across multiple records.
func avgAggregate(records []map[string]any, field string) (any, error) {
	var sum float64
	var count int
	for _, record := range records {
		value := getFieldValue(record, field)
		if value == nil {
			continue
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			sum += float64(v.Int())
			count++
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			sum += float64(v.Uint())
			count++
		case reflect.Float32, reflect.Float64:
			sum += v.Float()
			count++
		case reflect.String: // Attempt to parse strings as numbers
			if f, err := strconv.ParseFloat(v.String(), 64); err == nil {
				sum += f
				count++
			}
		}
	}

	if count == 0 {
		return nil, nil // Or return an error, depending on desired behavior for empty sets
	}
	return sum / float64(count), nil
}

// minAggregate finds the minimum value of a comparable field across multiple records.
func minAggregate(records []map[string]any, field string) (any, error) {
	if len(records) == 0 {
		return nil, nil
	}

	var minValue any = nil
	firstFound := false

	for _, record := range records {
		value := getFieldValue(record, field)
		if value == nil {
			continue
		}

		if !firstFound {
			minValue = value
			firstFound = true
			continue
		}

		if utils.CompareValues(value, minValue) < 0 {
			minValue = value
		}
	}

	if !firstFound {
		return nil, nil // No comparable values found
	}
	return minValue, nil
}

// maxAggregate finds the maximum value of a comparable field across multiple records.
func maxAggregate(records []map[string]any, field string) (any, error) {
	if len(records) == 0 {
		return nil, nil
	}

	var maxValue any = nil
	firstFound := false

	for _, record := range records {
		value := getFieldValue(record, field)
		if value == nil {
			continue
		}

		if !firstFound {
			maxValue = value
			firstFound = true
			continue
		}

		if utils.CompareValues(value, maxValue) > 0 {
			maxValue = value
		}
	}

	if !firstFound {
		return nil, nil // No comparable values found
	}
	return maxValue, nil
}

// getFieldValue is a standalone helper to be used by aggregation functions.
// This is a duplicate of the method in QueryHelper, ideally it would be shared.
// For robust solution, consider passing QueryHelper's getFieldValue or a record accessor.
func getFieldValue(record map[string]any, fieldPath string) any {
	parts := strings.Split(fieldPath, ".")
	var current any = record

	for i, part := range parts {
		if current == nil {
			return nil
		}

		currentMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}

		value, exists := currentMap[part]
		if !exists {
			return nil
		}

		if i == len(parts)-1 {
			return value
		}
		current = value
	}
	return nil
}

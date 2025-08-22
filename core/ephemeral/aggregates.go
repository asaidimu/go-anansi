package ephemeral

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/asaidimu/go-anansi/v6/core/data"
	
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// sumAggregate computes the sum of a numeric field across multiple records.
func sumAggregate(records []data.Document, field string) (any, error) {
	var sum float64
	foundNumeric := false
	for _, record := range records {
		value, _ := utils.GetValueByPath(record,field) // Assuming getFieldValue is accessible or passed
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
		return nil, fmt.Errorf("%w on field '%s'", data.ErrNoNumericValuesForAggregation, field)
	}
	return sum, nil
}

// countAggregate computes the count of records. If a field is specified, it counts non-nil values for that field.
func countAggregate(records []data.Document, field string) (any, error) {
	if field == "" {
		return len(records), nil // Count all records in the group
	}

	count := 0
	for _, record := range records {
		if result, _ := utils.GetValueByPath(record,field); result != nil {
			count++
		}
	}
	return count, nil
}

// avgAggregate computes the average of a numeric field across multiple records.
func avgAggregate(records []data.Document, field string) (any, error) {
	var sum float64
	var count int
	for _, record := range records {
		value, _ := utils.GetValueByPath(record,field)
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
func minAggregate(records []data.Document, field string) (any, error) {
	if len(records) == 0 {
		return nil, nil
	}

	var minValue any = nil
	firstFound := false

	for _, record := range records {
		value, _ := utils.GetValueByPath(record,field)
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
func maxAggregate(records []data.Document, field string) (any, error) {
	if len(records) == 0 {
		return nil, nil
	}

	var maxValue any = nil
	firstFound := false

	for _, record := range records {
		value,_ := utils.GetValueByPath(record,field)
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



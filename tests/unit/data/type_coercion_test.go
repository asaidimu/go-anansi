package data_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)

func TestDocument_TypeSafeAccessors(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"str":    "hello",
		"int":    123,
		"float":  123.45,
		"bool":   true,
		"time":   "2023-01-01T10:00:00Z",
		"nested": map[string]any{"key": "value"},
		"arr":    []any{"a", "b"},
		"doc_arr": []data.Document{
			data.MustNewDocument(map[string]any{"id": "1"}),
			data.MustNewDocument(map[string]any{"id": "2"}),
		},
		"str_int": "123",
		"uncoercible_int": "hello",
		"str_float": "123.45",
		"uncoercible_float": "hello",
		"str_bool_true": "true",
		"uncoercible_bool": "abc",
		"int_time": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		"float_time": float64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
		"uncoercible_time": "abc",
	})
	require.NoError(t, err)

	// GetString - successful string retrieval
	str, err := doc.GetString("str")
	require.NoError(t, err)
	require.Equal(t, "hello", str)

	// GetString - successful coercion from int
	str, err = doc.GetString("int")
	require.NoError(t, err)
	require.Equal(t, "123", str) // int 123 should coerce to string "123"

	// GetString - successful coercion from float
	str, err = doc.GetString("float")
	require.NoError(t, err)
	require.Equal(t, "123.45", str) // float 123.45 should coerce to string "123.45"

	// GetString - successful coercion from bool
	str, err = doc.GetString("bool")
	require.NoError(t, err)
	require.Equal(t, "true", str) // bool true should coerce to string "true"

	// GetString - key not found
	_, err = doc.GetString("non_existent")
	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetInt - successful int retrieval
	intVal, err := doc.GetInt("int")
	require.NoError(t, err)
	require.Equal(t, 123, intVal)

	// GetInt - successful coercion from string ("123")
	intVal, err = doc.GetInt("str_int")
	require.NoError(t, err)
	require.Equal(t, 123, intVal)

	// GetInt - successful coercion from float
	intVal, err = doc.GetInt("float")
	require.NoError(t, err)
	require.Equal(t, 123, intVal) // float 123.45 should coerce to int 123

	// GetInt - successful coercion from bool
	intVal, err = doc.GetInt("bool")
	require.NoError(t, err)
	require.Equal(t, 1, intVal) // bool true should coerce to int 1

	// GetInt - key not found
	_, err = doc.GetInt("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetInt - cannot coerce (e.g., "hello")
	_, err = doc.GetInt("uncoercible_int")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)

	// GetFloat64 - successful float retrieval
	floatVal, err := doc.GetFloat64("float")
	require.NoError(t, err)
	require.Equal(t, 123.45, floatVal)

	// GetFloat64 - successful coercion from int
	floatVal, err = doc.GetFloat64("int")
	require.NoError(t, err)
	require.Equal(t, 123.0, floatVal)

	// GetFloat64 - successful coercion from string ("123.45")
	floatVal, err = doc.GetFloat64("str_float")
	require.NoError(t, err)
	require.Equal(t, 123.45, floatVal)

	// GetFloat64 - successful coercion from bool
	floatVal, err = doc.GetFloat64("bool")
	require.NoError(t, err)
	require.Equal(t, 1.0, floatVal) // bool true should coerce to float 1.0

	// GetFloat64 - key not found
	_, err = doc.GetFloat64("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetFloat64 - cannot coerce (e.g., "hello")
	_, err = doc.GetFloat64("uncoercible_float")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)

	// GetBool - successful bool retrieval
	boolVal, err := doc.GetBool("bool")
	require.NoError(t, err)
	require.True(t, boolVal)

	// GetBool - successful coercion from string ("true")
	boolVal, err = doc.GetBool("str_bool_true")
	require.NoError(t, err)
	require.True(t, boolVal)

	// GetBool - successful coercion from int (1)
	boolVal, err = doc.GetBool("int")
	require.NoError(t, err)
	require.True(t, boolVal) // int 123 should coerce to true

	// GetBool - key not found
	_, err = doc.GetBool("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetBool - cannot coerce (e.g., "abc")
	_, err = doc.GetBool("uncoercible_bool")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)

	// GetTime - successful time retrieval
	timeVal, err := doc.GetTime("time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC), timeVal)

	// GetTime - successful coercion from int (unix timestamp)
	timeVal, err = doc.GetTime("int_time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), timeVal)

	// GetTime - successful coercion from float (unix timestamp)
	timeVal, err = doc.GetTime("float_time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), timeVal)

	// GetTime - key not found
	_, err = doc.GetTime("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetTime - cannot coerce (e.g., "abc")
	_, err = doc.GetTime("uncoercible_time")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)

	// GetDocument - successful document retrieval
	nestedDoc, err := doc.GetDocument("nested")
	require.NoError(t, err)
	require.Equal(t, data.Document{"key": "value"}, nestedDoc)

	// GetDocument - key not found
	_, err = doc.GetDocument("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetDocument - cannot coerce (e.g., a string)
	_, err = doc.GetDocument("str")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)

	// GetDocumentArray - successful document array retrieval
	docArr, err := doc.GetDocumentArray("doc_arr")
	require.NoError(t, err)
	require.Len(t, docArr, 2)

	// GetDocumentArray - key not found
	_, err = doc.GetDocumentArray("non_existent")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// GetDocumentArray - cannot coerce (e.g., a string)
	_, err = doc.GetDocumentArray("str")
	require.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	require.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)
}

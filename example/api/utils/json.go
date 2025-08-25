package utils

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    
    "github.com/asaidimu/go-anansi/v6/example/api/types"
)

// ParseJSONWithLimits safely parses JSON with size limits
func ParseJSONWithLimits(r *http.Request, v interface{}, maxSize int64) *types.APIError {
    // Limit reader size
    limitedReader := http.MaxBytesReader(nil, r.Body, maxSize)
    defer limitedReader.Close()
    
    decoder := json.NewDecoder(limitedReader)
    decoder.DisallowUnknownFields()
    
    if err := decoder.Decode(v); err != nil {
        if err == io.EOF {
            return types.NewClientError(types.ErrCodeInvalidJSON, "Empty request body")
        }
        return types.NewClientError(types.ErrCodeInvalidJSON, "Invalid JSON format", err.Error())
    }
    
    return nil
}

// WriteJSONResponse writes a JSON response safely
func WriteJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) error {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    return json.NewEncoder(w).Encode(data)
}

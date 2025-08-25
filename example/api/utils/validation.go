package utils

import (
    "regexp"
    "strings"
    
    "github.com/asaidimu/go-anansi/v6/example/api/types"
)

var (
    collectionNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
)

// ValidateCollectionName validates collection name format
func ValidateCollectionName(name string) *types.APIError {
    if len(name) == 0 {
        return types.NewClientError(types.ErrCodeValidationFailed, "Collection name cannot be empty")
    }
    
    if len(name) > 64 {
        return types.NewClientError(types.ErrCodeValidationFailed, "Collection name too long (max 64 characters)")
    }
    
    if !collectionNameRegex.MatchString(name) {
        return types.NewClientError(types.ErrCodeValidationFailed, "Invalid collection name format")
    }
    
    return nil
}

// ValidateOperationType validates operation type
func ValidateOperationType(opType string) *types.APIError {
    switch types.OperationType(strings.ToLower(opType)) {
    case types.OpCreate, types.OpRead, types.OpUpdate, types.OpDelete:
        return nil
    default:
        return types.NewClientError(types.ErrCodeValidationFailed, "Invalid operation type")
    }
}

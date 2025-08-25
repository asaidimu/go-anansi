package types

// APIResponse represents the consistent envelope pattern for all API responses
type APIResponse struct {
	Success bool        `json:"success"`
	Data    any `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// CollectionListResponse represents the response for listing collections
type CollectionListResponse struct {
	Collections []string `json:"collections"`
}

// TransactionResult represents the result of a transaction operation
type TransactionResult struct {
	OperationIndex int         `json:"operationIndex"`
	Type           string      `json:"type"`
	Success        bool        `json:"success"`
	Data           any `json:"data,omitempty"`
	Error          string      `json:"error,omitempty"`
}

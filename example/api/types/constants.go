package types

// OperationType represents transaction operation types
type OperationType string

const (
    OpCreate OperationType = "create"
    OpRead   OperationType = "read"
    OpUpdate OperationType = "update"
    OpDelete OperationType = "delete"
)

// HTTPMethod represents supported HTTP methods
type HTTPMethod string

const (
    MethodGET    HTTPMethod = "GET"
    MethodPOST   HTTPMethod = "POST"
    MethodPUT    HTTPMethod = "PUT"
    MethodDELETE HTTPMethod = "DELETE"
)

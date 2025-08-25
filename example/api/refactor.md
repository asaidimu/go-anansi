# API Refactoring Specification and Implementation Plan

## Overview
Refactor the monolithic `api.go` file into a well-structured, modular codebase that addresses critical issues while maintaining backward compatibility.

## Current Problems Summary
1. **Security**: No authentication, authorization, or input validation
2. **Error Handling**: Verbose error exposure, inconsistent status codes
3. **Resource Management**: Memory leaks, no timeouts, no request limits
4. **Architecture**: Monolithic design, poor separation of concerns
5. **API Design**: Inconsistent HTTP methods, magic strings
6. **Reliability**: Nested transactions, webhook issues, no monitoring

## Target Architecture

```
./api/
├── server/
│   ├── server.go              # Main APIServer struct and lifecycle
│   ├── routes.go              # Route registration and URL parsing
│   └── config.go              # Server configuration
├── middleware/
│   ├── middleware.go          # Middleware chain setup
│   ├── cors.go                # CORS handling
│   ├── timeout.go             # Request timeout middleware
│   ├── logging.go             # Request/response logging
│   ├── recovery.go            # Panic recovery
│   └── validation.go          # Request validation middleware
├── handlers/
│   ├── base.go                # Base handler with common functionality
│   ├── collections.go         # Collection CRUD operations
│   ├── management.go          # Collection management (create/delete)
│   ├── transactions.go        # Transaction handling
│   ├── subscriptions.go       # Subscription management
│   ├── schema.go              # Schema operations (migrate/rollback)
│   └── metadata.go            # Metadata operations
├── types/
│   ├── requests.go            # All request structures
│   ├── responses.go           # All response structures
│   ├── errors.go              # Error handling system
│   └── constants.go           # API constants and enums
├── webhooks/
│   ├── manager.go             # Webhook lifecycle management
│   ├── sender.go              # HTTP webhook delivery
│   └── registry.go            # Webhook registration/cleanup
├── utils/
│   ├── json.go                # JSON parsing utilities
│   ├── validation.go          # Input validation helpers
│   └── http.go                # HTTP utility functions
└── internal/
    ├── context.go             # Context utilities
    └── testing/               # Test utilities and mocks
```

## Detailed Refactoring Plan

### Phase 1: Foundation Setup (Priority: Critical)
**Goal**: Establish error handling system and extract types

#### Step 1.1: Create Error Handling System
**File**: `types/errors.go`
```go
package types

import (
    "fmt"
    "net/http"
)

// ErrorCode represents standardized error codes
type ErrorCode string

const (
    // Client errors (4xx)
    ErrCodeInvalidJSON       ErrorCode = "INVALID_JSON"
    ErrCodeInvalidPath       ErrorCode = "INVALID_PATH"
    ErrCodeCollectionNotFound ErrorCode = "COLLECTION_NOT_FOUND"
    ErrCodeValidationFailed   ErrorCode = "VALIDATION_FAILED"
    ErrCodeMethodNotAllowed   ErrorCode = "METHOD_NOT_ALLOWED"
    
    // Server errors (5xx)
    ErrCodeInternalError     ErrorCode = "INTERNAL_ERROR"
    ErrCodeDatabaseError     ErrorCode = "DATABASE_ERROR"
    ErrCodeTransactionFailed ErrorCode = "TRANSACTION_FAILED"
)

// APIError represents structured error information
type APIError struct {
    Code       ErrorCode `json:"code"`
    Message    string    `json:"message"`
    Details    string    `json:"details,omitempty"`
    StatusCode int       `json:"-"`
}

func (e APIError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error constructors
func NewClientError(code ErrorCode, message string, details ...string) APIError {
    // Implementation
}

func NewServerError(code ErrorCode, message string, internalErr error) APIError {
    // Implementation with sanitized details
}

// HTTP status mapping
func (e APIError) HTTPStatus() int {
    // Map error codes to appropriate HTTP status codes
}
```

#### Step 1.2: Extract All Types
**File**: `types/requests.go`
- Move all `*Request` structs
- Add validation tags
- Group by functionality

**File**: `types/responses.go`
- Move `APIResponse` and related structs
- Standardize response patterns

**File**: `types/constants.go`
```go
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
```

### Phase 2: Core Infrastructure (Priority: High)

#### Step 2.1: Create Base Handler
**File**: `handlers/base.go`
```go
package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "time"
    
    "your-module/api/types"
    "your-module/core/persistence/base"
    "go.uber.org/zap"
)

// BaseHandler provides common functionality for all handlers
type BaseHandler struct {
    persistence base.Persistence
    logger      *zap.Logger
    timeout     time.Duration
}

func NewBaseHandler(persistence base.Persistence, logger *zap.Logger) *BaseHandler {
    return &BaseHandler{
        persistence: persistence,
        logger:      logger,
        timeout:     30 * time.Second,
    }
}

// WithTimeout adds timeout to context
func (h *BaseHandler) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
    return context.WithTimeout(ctx, h.timeout)
}

// ParseJSON safely parses JSON with size limits
func (h *BaseHandler) ParseJSON(r *http.Request, v interface{}, maxSize int64) *types.APIError {
    // Implementation with size limits and validation
}

// WriteResponse writes standardized responses
func (h *BaseHandler) WriteResponse(w http.ResponseWriter, data interface{}) {
    // Implementation
}

// WriteError writes standardized error responses
func (h *BaseHandler) WriteError(w http.ResponseWriter, err *types.APIError) {
    // Implementation with logging
}

// ValidateMethod ensures correct HTTP method
func (h *BaseHandler) ValidateMethod(r *http.Request, allowed ...types.HTTPMethod) *types.APIError {
    // Implementation
}
```

#### Step 2.2: Create Middleware System
**File**: `middleware/middleware.go`
```go
package middleware

import (
    "net/http"
    "time"
    
    "go.uber.org/zap"
)

// Chain represents a middleware chain
type Chain struct {
    middlewares []func(http.Handler) http.Handler
}

func New(middlewares ...func(http.Handler) http.Handler) *Chain {
    return &Chain{middlewares}
}

func (c *Chain) Then(h http.Handler) http.Handler {
    // Apply middlewares in reverse order
}

// Common middleware factories
func WithTimeout(timeout time.Duration) func(http.Handler) http.Handler {
    // Implementation
}

func WithRecovery(logger *zap.Logger) func(http.Handler) http.Handler {
    // Panic recovery with logging
}

func WithRequestLogging(logger *zap.Logger) func(http.Handler) http.Handler {
    // Request/response logging
}

func WithValidation() func(http.Handler) http.Handler {
    // Request validation (size limits, etc.)
}
```

### Phase 3: Handler Extraction (Priority: High)

#### Step 3.1: Collections Handler
**File**: `handlers/collections.go`
```go
package handlers

import (
    "net/http"
    "strings"
    
    "your-module/api/types"
)

// CollectionsHandler handles collection CRUD operations
type CollectionsHandler struct {
    *BaseHandler
}

func NewCollectionsHandler(base *BaseHandler) *CollectionsHandler {
    return &CollectionsHandler{BaseHandler: base}
}

// Handle routes collection operations based on URL pattern
// /api/collections/{collection}/{operation}
func (h *CollectionsHandler) Handle(w http.ResponseWriter, r *http.Request) {
    // Parse URL path
    pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(pathParts) != 4 {
        h.WriteError(w, types.NewClientError(types.ErrCodeInvalidPath, "Invalid API path"))
        return
    }
    
    collection := pathParts[2]
    operation := pathParts[3]
    
    // Route to appropriate handler
    switch operation {
    case string(types.OpCreate):
        h.handleCreate(w, r, collection)
    case string(types.OpRead):
        h.handleRead(w, r, collection)
    case string(types.OpUpdate):
        h.handleUpdate(w, r, collection)
    case string(types.OpDelete):
        h.handleDelete(w, r, collection)
    case "validate":
        h.handleValidate(w, r, collection)
    case "capabilities":
        h.handleCapabilities(w, r, collection)
    case "metadata":
        h.handleMetadata(w, r, collection)
    default:
        h.WriteError(w, types.NewClientError(types.ErrCodeInvalidPath, "Invalid operation"))
    }
}

// Individual operation handlers with proper error handling and timeouts
func (h *CollectionsHandler) handleCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
    if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
        h.WriteError(w, err)
        return
    }
    
    ctx, cancel := h.WithTimeout(r.Context())
    defer cancel()
    
    // Implementation with proper error handling
}

// ... other handlers
```

#### Step 3.2: Management Handler
**File**: `handlers/management.go`
- Collection creation/deletion
- Schema operations
- System-level operations

#### Step 3.3: Transactions Handler
**File**: `handlers/transactions.go`
```go
package handlers

// TransactionsHandler handles transaction operations
type TransactionsHandler struct {
    *BaseHandler
}

func (h *TransactionsHandler) Execute(w http.ResponseWriter, r *http.Request) {
    // Fix nested transaction issues
    // Add proper validation
    // Implement timeout handling
}

// Helper to execute operations safely
func (h *TransactionsHandler) executeOperation(ctx context.Context, tx base.BasePersistence, op types.TransactionOperation) (types.TransactionResult, error) {
    // Safe operation execution without nested transactions
}
```

### Phase 4: Resource Management (Priority: High)

#### Step 4.1: Webhook Management System
**File**: `webhooks/manager.go`
```go
package webhooks

import (
    "context"
    "sync"
    "time"
    
    "your-module/core/persistence/base"
    "go.uber.org/zap"
)

// Manager handles webhook lifecycle and cleanup
type Manager struct {
    sender      *Sender
    callbacks   map[string]*CallbackInfo
    mutex       sync.RWMutex
    logger      *zap.Logger
    cleanupTick time.Duration
}

type CallbackInfo struct {
    Callback     EventCallback
    CreatedAt    time.Time
    LastUsed     time.Time
    FailureCount int
    MaxFailures  int
}

func NewManager(logger *zap.Logger) *Manager {
    // Implementation with cleanup goroutine
}

// StartCleanup starts the cleanup process for old/failed webhooks
func (m *Manager) StartCleanup(ctx context.Context) {
    // Periodic cleanup implementation
}

// Register adds a new webhook with automatic cleanup
func (m *Manager) Register(eventType base.PersistenceEventType, url string, headers map[string]string) string {
    // Implementation with cleanup tracking
}

// Unregister removes a webhook
func (m *Manager) Unregister(id string) bool {
    // Safe removal with cleanup
}

// Close gracefully shuts down the manager
func (m *Manager) Close() error {
    // Cleanup all resources
}
```

#### Step 4.2: Enhanced Sender
**File**: `webhooks/sender.go`
```go
package webhooks

import (
    "context"
    "net/http"
    "time"
    
    "your-module/core/persistence/base"
)

// Sender handles HTTP webhook delivery with retries and timeouts
type Sender struct {
    client      *http.Client
    maxRetries  int
    retryDelay  time.Duration
}

func NewSender(timeout time.Duration, maxRetries int) *Sender {
    return &Sender{
        client: &http.Client{Timeout: timeout},
        maxRetries: maxRetries,
        retryDelay: time.Second,
    }
}

// Send delivers a webhook with retry logic
func (s *Sender) Send(ctx context.Context, callback EventCallback, event base.PersistenceEvent) error {
    // Implementation with retries and circuit breaker
}

// SendWithRetry implements exponential backoff
func (s *Sender) SendWithRetry(ctx context.Context, url string, headers map[string]string, payload []byte) error {
    // Retry implementation
}
```

### Phase 5: Server Restructure (Priority: Medium)

#### Step 5.1: Main Server
**File**: `server/server.go`
```go
package server

import (
    "context"
    "net/http"
    "time"
    
    "your-module/api/handlers"
    "your-module/api/middleware"
    "your-module/api/webhooks"
    "your-module/core/persistence/base"
    "go.uber.org/zap"
)

// APIServer represents the main API server
type APIServer struct {
    persistence     base.Persistence
    logger         *zap.Logger
    mux            *http.ServeMux
    webhookManager *webhooks.Manager
    
    // Handlers
    collections  *handlers.CollectionsHandler
    management   *handlers.ManagementHandler
    transactions *handlers.TransactionsHandler
    subscriptions *handlers.SubscriptionsHandler
    schema       *handlers.SchemaHandler
    metadata     *handlers.MetadataHandler
    
    // Configuration
    config *Config
}

// Config holds server configuration
type Config struct {
    RequestTimeout time.Duration
    MaxRequestSize int64
    CORSEnabled    bool
    CORSOrigins    []string
}

func New(persistence base.Persistence, logger *zap.Logger, config *Config) *APIServer {
    // Initialize server with all components
}

// setupMiddleware configures the middleware chain
func (s *APIServer) setupMiddleware() *middleware.Chain {
    return middleware.New(
        middleware.WithRecovery(s.logger),
        middleware.WithRequestLogging(s.logger),
        middleware.WithTimeout(s.config.RequestTimeout),
        middleware.WithValidation(),
        s.corsMiddleware(),
    )
}

// Start starts the server
func (s *APIServer) Start(addr string) error {
    handler := s.setupMiddleware().Then(s)
    return http.ListenAndServe(addr, handler)
}

// Close gracefully shuts down the server
func (s *APIServer) Close(ctx context.Context) error {
    // Proper shutdown sequence
}
```

#### Step 5.2: Routes Configuration
**File**: `server/routes.go`
```go
package server

// setupRoutes configures all API routes
func (s *APIServer) setupRoutes() {
    // Collection data operations - now using proper HTTP methods
    s.mux.HandleFunc("/api/collections/", s.collections.Handle)
    
    // Collection management operations
    s.mux.HandleFunc("/api/management/collections/list", s.management.ListCollections)
    s.mux.HandleFunc("/api/management/collections/create", s.management.CreateCollection)
    s.mux.HandleFunc("/api/management/collections/delete", s.management.DeleteCollection)
    
    // Transaction operations
    s.mux.HandleFunc("/api/transactions/execute", s.transactions.Execute)
    
    // Subscription operations
    s.mux.HandleFunc("/api/subscriptions/", s.subscriptions.Handle)
    
    // Schema operations
    s.mux.HandleFunc("/api/schema/", s.schema.Handle)
    
    // Metadata operations
    s.mux.HandleFunc("/api/metadata", s.metadata.Handle)
}
```

### Phase 6: Utilities and Polish (Priority: Low)

#### Step 6.1: JSON Utilities
**File**: `utils/json.go`
```go
package utils

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    
    "your-module/api/types"
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
```

#### Step 6.2: Validation Utilities
**File**: `utils/validation.go`
```go
package utils

import (
    "regexp"
    "strings"
    
    "your-module/api/types"
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
```

## Implementation Strategy

### Phase Execution Order
1. **Phase 1** (Foundation) 
   - Critical for all subsequent work
   - Establishes error handling patterns
   - Low risk, high impact

2. **Phase 2** (Infrastructure) 
   - Builds on Phase 1
   - Establishes handler patterns
   - Medium risk, high impact

3. **Phase 3** (Handler Extraction) 
   - Most time-consuming
   - Can be done incrementally
   - Low risk once patterns are established

4. **Phase 4** (Resource Management) 
   - Addresses critical memory leaks
   - High impact on stability
   - Medium risk

5. **Phase 5** (Server Restructure) 
   - Final assembly
   - Low risk once components are ready

6. **Phase 6** (Polish) 
   - Quality improvements
   - Can be done in parallel

### Risk Mitigation
- Maintain backward compatibility during refactor
- Extensive testing after each phase
- Gradual rollout with feature flags
- Rollback plan for each phase

### Testing Strategy
- Unit tests for each new module
- Integration tests for handler flows
- Performance tests for resource management
- Backward compatibility tests

### Success Metrics
- **Code Quality**: Reduced cyclomatic complexity, improved test coverage
- **Performance**: Reduced memory usage, eliminated leaks
- **Reliability**: Proper error handling, no information leaks
- **Maintainability**: Clear module boundaries, single responsibility

This plan provides a systematic approach to refactoring while maintaining system stability and improving code quality progressively.

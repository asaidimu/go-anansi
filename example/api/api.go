package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"

	"go.uber.org/zap"
)

// APIResponse represents the consistent envelope pattern for all API responses
type APIResponse struct {
	Success bool        `json:"success"`
	Data    any `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents error details in API responses
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// CollectionCreateRequest represents the request body for creating documents
type CollectionCreateRequest struct {
	Documents []data.Document `json:"documents"`
}

// CollectionReadRequest represents the request body for reading documents
type CollectionReadRequest struct {
	Query *query.Query `json:"query,omitempty"`
}

// CollectionUpdateRequest represents the request body for updating documents
type CollectionUpdateRequest struct {
	Data    data.Document      `json:"data"`
	Filter  *query.QueryFilter `json:"filter"`
	Version *int               `json:"version,omitempty"`
	Recover bool               `json:"recover,omitempty"`
}

// CollectionDeleteRequest represents the request body for deleting documents
type CollectionDeleteRequest struct {
	Filter *query.QueryFilter `json:"filter"`
	Unsafe bool               `json:"unsafe,omitempty"`
}

// CollectionListResponse represents the response for listing collections
type CollectionListResponse struct {
	Collections []string `json:"collections"`
}

// CollectionCreateCollectionRequest represents the request for creating a new collection
type CollectionCreateCollectionRequest struct {
	Schema schema.SchemaDefinition `json:"schema"`
}

// CollectionSchemaRequest represents the request for getting a collection schema
type CollectionSchemaRequest struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// CollectionDeleteCollectionRequest represents the request for deleting a collection
type CollectionDeleteCollectionRequest struct {
	Name string `json:"name"`
}

// CollectionValidateRequest represents the request for validating a document
type CollectionValidateRequest struct {
	Data  data.Document `json:"data"`
	Loose bool          `json:"loose,omitempty"`
}

// CollectionMetadataRequest represents the request for getting collection metadata
type CollectionMetadataRequest struct {
	Filter       *base.MetadataFilter `json:"filter,omitempty"`
	ForceRefresh bool                 `json:"forceRefresh,omitempty"`
}

// TransactionExecuteRequest represents the request for executing transactions
type TransactionExecuteRequest struct {
	Operations []TransactionOperation `json:"operations"`
}

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	Type       string                 `json:"type"` // "create", "read", "update", "delete"
	Collection string                 `json:"collection"`
	Data       map[string]any `json:"data,omitempty"`
	Query      *query.Query           `json:"query,omitempty"`
	Filter     *query.QueryFilter     `json:"filter,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

// TransactionResult represents the result of a transaction operation
type TransactionResult struct {
	OperationIndex int         `json:"operationIndex"`
	Type           string      `json:"type"`
	Success        bool        `json:"success"`
	Data           any `json:"data,omitempty"`
	Error          string      `json:"error,omitempty"`
}

// SubscriptionRegisterRequest represents the request for registering subscriptions
type SubscriptionRegisterRequest struct {
	Event       base.PersistenceEventType `json:"event"`
	Label       *string                   `json:"label,omitempty"`
	Description *string                   `json:"description,omitempty"`
	Collection  *string                   `json:"collection,omitempty"` // For collection-specific subscriptions
}

// SubscriptionUnregisterRequest represents the request for unregistering subscriptions
type SubscriptionUnregisterRequest struct {
	ID         string  `json:"id"`
	Collection *string `json:"collection,omitempty"` // For collection-specific subscriptions
}

// MigrateRequest represents the request for schema migrations
type MigrateRequest struct {
	Name      string           `json:"name"`
	Migration schema.Migration `json:"migration"`
	DryRun    *bool            `json:"dryRun,omitempty"`
}

// RollbackRequest represents the request for schema rollbacks
type RollbackRequest struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
	DryRun  *bool   `json:"dryRun,omitempty"`
}

// APIServer wraps the persistence layer and provides HTTP handlers
type APIServer struct {
	persistence    base.Persistence
	logger         *zap.Logger
	mux            *http.ServeMux
	eventCallbacks map[string]EventCallback
}

// EventCallback represents a callback for handling events via HTTP (e.g., webhooks)
type EventCallback struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// NewAPIServer creates a new API server instance
func NewAPIServer(persistence base.Persistence, logger *zap.Logger) *APIServer {
	if logger == nil {
		logger = zap.NewNop()
	}

	server := &APIServer{
		persistence:    persistence,
		logger:         logger,
		mux:            http.NewServeMux(),
		eventCallbacks: make(map[string]EventCallback),
	}

	server.setupRoutes()
	return server
}

// ServeHTTP implements http.Handler interface
func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// setupRoutes configures all API routes
func (s *APIServer) setupRoutes() {
	// Collection data operations
	s.mux.HandleFunc("/api/collections/", s.handleCollectionOperations)

	// Collection management operations
	s.mux.HandleFunc("/api/collections/list", s.handleCollectionsList)
	s.mux.HandleFunc("/api/collections/create", s.handleCollectionsCreate)
	s.mux.HandleFunc("/api/collections/schema", s.handleCollectionsSchema)
	s.mux.HandleFunc("/api/collections/delete", s.handleCollectionsDelete)
	s.mux.HandleFunc("/api/collections/metadata", s.handleCollectionsMetadata)

	// Transaction operations
	s.mux.HandleFunc("/api/transactions/execute", s.handleTransactionsExecute)

	// Subscription operations
	s.mux.HandleFunc("/api/subscriptions/register", s.handleSubscriptionsRegister)
	s.mux.HandleFunc("/api/subscriptions/unregister", s.handleSubscriptionsUnregister)
	s.mux.HandleFunc("/api/subscriptions/list", s.handleSubscriptionsList)

	// Schema operations
	s.mux.HandleFunc("/api/schema/migrate", s.handleSchemaMigrate)
	s.mux.HandleFunc("/api/schema/rollback", s.handleSchemaRollback)

	// Global metadata
	s.mux.HandleFunc("/api/metadata", s.handleMetadata)
}

// handleCollectionOperations routes collection-specific operations
func (s *APIServer) handleCollectionOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	// Parse URL: /api/collections/{collection}/{operation}
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) != 4 || pathParts[0] != "api" || pathParts[1] != "collections" {
		s.writeErrorResponse(w, http.StatusNotFound, "INVALID_PATH", "Invalid API path", "")
		return
	}

	collectionName := pathParts[2]
	operation := pathParts[3]

	switch operation {
	case "create":
		s.handleCollectionCreate(w, r, collectionName)
	case "read":
		s.handleCollectionRead(w, r, collectionName)
	case "update":
		s.handleCollectionUpdate(w, r, collectionName)
	case "delete":
		s.handleCollectionDelete(w, r, collectionName)
	case "validate":
		s.handleCollectionValidate(w, r, collectionName)
	case "capabilities":
		s.handleCollectionCapabilities(w, r, collectionName)
	case "metadata":
		s.handleCollectionMetadata(w, r, collectionName)
	case "subscriptions":
		s.handleCollectionSubscriptions(w, r, collectionName)
	default:
		s.writeErrorResponse(w, http.StatusNotFound, "INVALID_OPERATION", fmt.Sprintf("Operation '%s' not supported", operation), "")
	}
}

// handleCollectionCreate handles document creation
func (s *APIServer) handleCollectionCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionCreateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	if len(req.Documents) == 1 {
		result, err := collection.CreateOne(ctx, req.Documents[0])
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "CREATE_FAILED", "Failed to create document", err.Error())
			return
		}
		s.writeSuccessResponse(w, http.StatusCreated, result)
	} else {
		results, err := collection.CreateMany(ctx, req.Documents)
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "CREATE_FAILED", "Failed to create documents", err.Error())
			return
		}
		s.writeSuccessResponse(w, http.StatusCreated, results)
	}
}

// handleCollectionRead handles document querying
func (s *APIServer) handleCollectionRead(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionReadRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	// If no query provided, create an empty one
	queryDSL := req.Query
	if queryDSL == nil {
		queryDSL = &query.Query{}
	}

	result, err := collection.Read(ctx, queryDSL)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "READ_FAILED", "Failed to read documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleCollectionUpdate handles document updates
func (s *APIServer) handleCollectionUpdate(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionUpdateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	updateParams := &base.CollectionUpdate{
		Data:    req.Data,
		Filter:  req.Filter,
		Version: req.Version,
		Recover: req.Recover,
	}

	count, err := collection.Update(ctx, updateParams)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"updatedCount": count,
	})
}

// handleCollectionDelete handles document deletion
func (s *APIServer) handleCollectionDelete(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionDeleteRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	count, err := collection.Delete(ctx, req.Filter, req.Unsafe)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"deletedCount": count,
	})
}

// handleCollectionValidate handles document validation
func (s *APIServer) handleCollectionValidate(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionValidateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	result, err := collection.Validate(ctx, req.Data, req.Loose)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "VALIDATION_FAILED", "Failed to validate document", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleCollectionCapabilities handles getting collection capabilities
func (s *APIServer) handleCollectionCapabilities(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	capabilities := collection.Capabilities(ctx)
	s.writeSuccessResponse(w, http.StatusOK, capabilities)
}

// handleCollectionMetadata handles getting collection metadata
func (s *APIServer) handleCollectionMetadata(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()
	var req CollectionMetadataRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	metadata, err := collection.Metadata(ctx, req.Filter, req.ForceRefresh)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "METADATA_FAILED", "Failed to get collection metadata", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, metadata)
}

// handleCollectionSubscriptions handles getting collection subscriptions
func (s *APIServer) handleCollectionSubscriptions(w http.ResponseWriter, r *http.Request, collectionName string) {
	ctx := r.Context()

	collection, err := s.persistence.Collection(ctx, collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	subscriptions, err := collection.Subscriptions(ctx)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "SUBSCRIPTIONS_FAILED", "Failed to get collection subscriptions", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, subscriptions)
}

// handleCollectionsList handles listing all collections
func (s *APIServer) handleCollectionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	collections, err := s.persistence.ListCollections(ctx)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list collections", err.Error())
		return
	}

	response := CollectionListResponse{
		Collections: collections,
	}

	s.writeSuccessResponse(w, http.StatusOK, response)
}

// handleCollectionsCreate handles creating a new collection
func (s *APIServer) handleCollectionsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req CollectionCreateCollectionRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	_, err := s.persistence.CreateCollection(ctx, req.Schema)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "CREATE_COLLECTION_FAILED", "Failed to create collection", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusCreated, map[string]any{
		"collection": req.Schema.Name,
		"created":    true,
	})
}

// handleCollectionsSchema handles getting a collection schema
func (s *APIServer) handleCollectionsSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req CollectionSchemaRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	var schemaDef *schema.SchemaDefinition
	var err error

	if req.Version != nil {
		schemaDef, err = s.persistence.Schema(ctx, req.Name, *req.Version)
	} else {
		schemaDef, err = s.persistence.Schema(ctx, req.Name)
	}

	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "SCHEMA_NOT_FOUND", fmt.Sprintf("Schema for collection '%s' not found", req.Name), err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, schemaDef)
}

// handleCollectionsDelete handles deleting a collection
func (s *APIServer) handleCollectionsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req CollectionDeleteCollectionRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	deleted, err := s.persistence.Delete(ctx, req.Name)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "DELETE_COLLECTION_FAILED", "Failed to delete collection", err.Error())
		return
	}

	if !deleted {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", req.Name), "")
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"collection": req.Name,
		"deleted":    true,
	})
}

// handleCollectionsMetadata handles getting global collections metadata
func (s *APIServer) handleCollectionsMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req CollectionMetadataRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	metadata, err := s.persistence.Metadata(ctx, req.Filter)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "METADATA_FAILED", "Failed to get metadata", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, metadata)
}

// handleTransactionsExecute handles transaction execution
func (s *APIServer) handleTransactionsExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req TransactionExecuteRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	// Validate operations before executing transaction
	if err := s.validateTransactionOperations(req.Operations); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_OPERATIONS", "Transaction operations validation failed", err.Error())
		return
	}

	// Execute transaction using the persistence layer
	result, err := s.persistence.Transact(ctx, func(tx base.BasePersistence) (any, error) {
		results := make([]TransactionResult, 0, len(req.Operations))

		for i, op := range req.Operations {
			opResult := TransactionResult{
				OperationIndex: i,
				Type:           op.Type,
				Success:        false,
			}

			// Get collection for this operation
			collection, err := tx.Collection(ctx, op.Collection)
			if err != nil {
				opResult.Error = fmt.Sprintf("Collection '%s' not found: %v", op.Collection, err)
				results = append(results, opResult)
				return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
			}

			// Execute operation based on type
			switch strings.ToLower(op.Type) {
			case "create":
				if op.Data == nil {
					opResult.Error = "Data is required for create operation"
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				doc := data.MustNewDocument(op.Data)
				createResult, err := collection.CreateOne(ctx, doc)
				if err != nil {
					opResult.Error = err.Error()
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				opResult.Success = true
				opResult.Data = createResult

			case "read":
				queryDSL := op.Query
				if queryDSL == nil {
					queryDSL = &query.Query{}
				}

				readResult, err := collection.Read(ctx, queryDSL)
				if err != nil {
					opResult.Error = err.Error()
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				opResult.Success = true
				opResult.Data = readResult

			case "update":
				if op.Data == nil || op.Filter == nil {
					opResult.Error = "Data and filter are required for update operation"
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				doc := data.MustNewDocument(op.Data)
				updateParams := &base.CollectionUpdate{
					Data:   doc,
					Filter: op.Filter,
				}

				// Parse options if provided
				if op.Options != nil {
					if version, ok := op.Options["version"].(float64); ok {
						v := int(version)
						updateParams.Version = &v
					}
					if recover, ok := op.Options["recover"].(bool); ok {
						updateParams.Recover = recover
					}
				}

				count, err := collection.Update(ctx, updateParams)
				if err != nil {
					opResult.Error = err.Error()
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				opResult.Success = true
				opResult.Data = map[string]any{
					"updatedCount": count,
				}

			case "delete":
				if op.Filter == nil {
					opResult.Error = "Filter is required for delete operation"
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				unsafe := false
				if op.Options != nil {
					if u, ok := op.Options["unsafe"].(bool); ok {
						unsafe = u
					}
				}

				count, err := collection.Delete(ctx, op.Filter, unsafe)
				if err != nil {
					opResult.Error = err.Error()
					results = append(results, opResult)
					return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
				}

				opResult.Success = true
				opResult.Data = map[string]any{
					"deletedCount": count,
				}

			default:
				opResult.Error = fmt.Sprintf("Unsupported operation type: %s", op.Type)
				results = append(results, opResult)
				return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
			}

			results = append(results, opResult)
		}

		return map[string]any{
			"executed":         true,
			"operations_count": len(req.Operations),
			"results":          results,
		}, nil
	})

	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "TRANSACTION_FAILED", "Transaction execution failed", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleSubscriptionsRegister handles registering subscriptions
func (s *APIServer) handleSubscriptionsRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req SubscriptionRegisterRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	options := base.RegisterSubscriptionOptions{
		Event:       req.Event,
		Label:       req.Label,
		Description: req.Description,
		Callback:    s.createEventCallback(),
	}

	var subscriptionID string
	if req.Collection != nil {
		// Collection-specific subscription
		collection, err := s.persistence.Collection(ctx, *req.Collection)
		if err != nil {
			s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", *req.Collection), err.Error())
			return
		}
		subscriptionID = collection.RegisterSubscription(ctx, options)
	} else {
		// Global subscription
		subscriptionID = s.persistence.RegisterSubscription(ctx, options)
	}

	s.writeSuccessResponse(w, http.StatusCreated, map[string]any{
		"subscriptionId": subscriptionID,
	})
}

// handleSubscriptionsUnregister handles unregistering subscriptions
func (s *APIServer) handleSubscriptionsUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req SubscriptionUnregisterRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	if req.Collection != nil {
		// Collection-specific unsubscription
		collection, err := s.persistence.Collection(ctx, *req.Collection)
		if err != nil {
			s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", *req.Collection), err.Error())
			return
		}
		collection.UnregisterSubscription(ctx, req.ID)
	} else {
		// Global unsubscription
		s.persistence.UnregisterSubscription(ctx, req.ID)
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"unregistered": true,
	})
}

// handleSubscriptionsList handles listing subscriptions
func (s *APIServer) handleSubscriptionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	subscriptions, err := s.persistence.Subscriptions(ctx)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "SUBSCRIPTIONS_FAILED", "Failed to list subscriptions", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, subscriptions)
}

// handleSchemaMigrate handles schema migrations
func (s *APIServer) handleSchemaMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req MigrateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	_, err := s.persistence.Migrate(ctx, req.Name, req.Migration, req.DryRun)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "MIGRATION_FAILED", "Schema migration failed", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"migrated":   true,
		"collection": req.Name,
	})
}

// handleSchemaRollback handles schema rollbacks
func (s *APIServer) handleSchemaRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req RollbackRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	_, err := s.persistence.Rollback(ctx, req.Name, req.Version, req.DryRun)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "ROLLBACK_FAILED", "Schema rollback failed", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, map[string]any{
		"rolledBack": true,
		"collection": req.Name,
	})
}

// handleMetadata handles getting global metadata
func (s *APIServer) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	ctx := r.Context()
	var req CollectionMetadataRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	metadata, err := s.persistence.Metadata(ctx, req.Filter)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "METADATA_FAILED", "Failed to get metadata", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, metadata)
}

// createEventCallback creates a callback function for handling persistence events
func (s *APIServer) createEventCallback() base.EventCallbackFunction {
	return func(ctx context.Context, event base.PersistenceEvent) error {
		// Log the event
		s.logger.Info("Persistence event received",
			zap.String("type", string(event.Type)),
			zap.String("operation", event.Operation),
			zap.Any("collection", event.Collection),
			zap.Any("transactionId", event.TransactionID),
		)

		// Handle specific event types
		switch event.Type {
		case base.DocumentCreateSuccess, base.DocumentUpdateSuccess, base.DocumentDeleteSuccess:
			s.logger.Debug("Document operation completed successfully",
				zap.String("operation", event.Operation),
				zap.Any("collection", event.Collection),
			)
		case base.DocumentCreateFailed, base.DocumentUpdateFailed, base.DocumentDeleteFailed:
			s.logger.Error("Document operation failed",
				zap.String("operation", event.Operation),
				zap.Any("collection", event.Collection),
				zap.Any("error", event.Error),
			)
		case base.TransactionStart:
			s.logger.Info("Transaction started", zap.Any("transactionId", event.TransactionID))
		case base.TransactionSuccess:
			s.logger.Info("Transaction completed successfully", zap.Any("transactionId", event.TransactionID))
		case base.TransactionFailed:
			s.logger.Error("Transaction failed",
				zap.Any("transactionId", event.TransactionID),
				zap.Any("error", event.Error),
			)
		case base.MigrateStart:
			s.logger.Info("Migration started", zap.Any("collection", event.Collection))
		case base.MigrateSuccess:
			s.logger.Info("Migration completed successfully", zap.Any("collection", event.Collection))
		case base.MigrateFailed:
			s.logger.Error("Migration failed",
				zap.Any("collection", event.Collection),
				zap.Any("error", event.Error),
			)
		case base.Telemetry:
			// Handle telemetry events - could send to monitoring system
			s.logger.Debug("Telemetry event", zap.Any("context", event.Context))
		}

		// TODO: Implement webhook notifications, websocket broadcasting, etc.
		// For now, we just log the events

		return nil
	}
}

// Helper method to validate transaction operations
func (s *APIServer) validateTransactionOperations(operations []TransactionOperation) error {
	for i, op := range operations {
		if op.Type == "" {
			return fmt.Errorf("operation %d: type is required", i)
		}
		if op.Collection == "" {
			return fmt.Errorf("operation %d: collection is required", i)
		}

		switch strings.ToLower(op.Type) {
		case "create":
			if op.Data == nil {
				return fmt.Errorf("operation %d: data is required for create operation", i)
			}
		case "read":
			// Query is optional for read operations
		case "update":
			if op.Data == nil {
				return fmt.Errorf("operation %d: data is required for update operation", i)
			}
			if op.Filter == nil {
				return fmt.Errorf("operation %d: filter is required for update operation", i)
			}
		case "delete":
			if op.Filter == nil {
				return fmt.Errorf("operation %d: filter is required for delete operation", i)
			}
		default:
			return fmt.Errorf("operation %d: unsupported operation type '%s'", i, op.Type)
		}
	}
	return nil
}

// AddEventWebhook adds a webhook URL for event notifications
func (s *APIServer) AddEventWebhook(eventType base.PersistenceEventType, url string, headers map[string]string) string {
	callback := EventCallback{
		URL:     url,
		Headers: headers,
	}

	// Generate a unique ID for this callback
	callbackID := fmt.Sprintf("webhook_%s_%d", eventType, len(s.eventCallbacks))
	s.eventCallbacks[callbackID] = callback

	// Register the subscription
	options := base.RegisterSubscriptionOptions{
		Event:       eventType,
		Label:       &callbackID,
		Description: &url,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			return s.sendWebhookNotification(callback, event)
		},
	}

	subscriptionID := s.persistence.RegisterSubscription(context.Background(), options)
	s.logger.Info("Webhook registered",
		zap.String("eventType", string(eventType)),
		zap.String("url", url),
		zap.String("subscriptionId", subscriptionID),
	)

	return subscriptionID
}

// sendWebhookNotification sends an HTTP POST request to a webhook URL
func (s *APIServer) sendWebhookNotification(callback EventCallback, event base.PersistenceEvent) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("Failed to marshal event for webhook", zap.Error(err))
		return err
	}

	req, err := http.NewRequest("POST", callback.URL, strings.NewReader(string(eventJSON)))
	if err != nil {
		s.logger.Error("Failed to create webhook request", zap.Error(err))
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range callback.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Error("Failed to send webhook notification", zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Warn("Webhook returned non-success status",
			zap.String("url", callback.URL),
			zap.Int("statusCode", resp.StatusCode),
		)
	}

	return nil
}

// Close gracefully shuts down the API server
func (s *APIServer) Close() {
	s.logger.Info("Shutting down API server")

	// Close the persistence layer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.persistence.Close(ctx)
	s.logger.Info("API server shutdown complete")
}

// parseJSONBody parses JSON request body into the provided struct
func (s *APIServer) parseJSONBody(r *http.Request, v any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// writeSuccessResponse writes a successful API response
func (s *APIServer) writeSuccessResponse(w http.ResponseWriter, statusCode int, data any) {
	response := APIResponse{
		Success: true,
		Data:    data,
	}
	s.writeJSONResponse(w, statusCode, response)
}

// writeErrorResponse writes an error API response
func (s *APIServer) writeErrorResponse(w http.ResponseWriter, statusCode int, code, message, details string) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
	s.writeJSONResponse(w, statusCode, response)
}

// writeJSONResponse writes a JSON response
func (s *APIServer) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

// Middleware for adding CORS headers (optional)
func (s *APIServer) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Start starts the API server on the specified address
func (s *APIServer) Start(addr string) error {
	s.logger.Info("Starting API server", zap.String("address", addr))

	// Wrap with CORS middleware
	handler := s.CORSMiddleware(s)

	return http.ListenAndServe(addr, handler)
}

// StartTLS starts the API server with TLS on the specified address
func (s *APIServer) StartTLS(addr, certFile, keyFile string) error {
	s.logger.Info("Starting API server with TLS", zap.String("address", addr))

	// Wrap with CORS middleware
	handler := s.CORSMiddleware(s)

	return http.ListenAndServeTLS(addr, certFile, keyFile, handler)
}

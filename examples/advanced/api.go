package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/asaidimu/go-anansi/v2/core/persistence"
	"github.com/asaidimu/go-anansi/v2/core/query"
	"github.com/asaidimu/go-anansi/v2/core/schema"
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
	Documents []map[string]any `json:"documents"`
}

// CollectionReadRequest represents the request body for reading documents
type CollectionReadRequest struct {
	Query *query.QueryDSL `json:"query,omitempty"`
}

// CollectionUpdateRequest represents the request body for updating documents
type CollectionUpdateRequest struct {
	Filters query.QueryFilter       `json:"filters"`
	Data    map[string]any  `json:"data"`
	Upsert  bool                    `json:"upsert,omitempty"`
}

// CollectionDeleteRequest represents the request body for deleting documents
type CollectionDeleteRequest struct {
	Filters query.QueryFilter `json:"filters"`
	Hard    bool              `json:"hard,omitempty"`
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
	Name string `json:"name"`
}

// CollectionDeleteCollectionRequest represents the request for deleting a collection
type CollectionDeleteCollectionRequest struct {
	Name string `json:"name"`
}

// TransactionExecuteRequest represents the request for executing transactions (stubbed)
type TransactionExecuteRequest struct {
	Operations []map[string]any `json:"operations"`
}

// APIServer wraps the persistence layer and provides HTTP handlers
type APIServer struct {
	persistence persistence.PersistenceInterface
	logger      *zap.Logger
	mux         *http.ServeMux
}

// NewAPIServer creates a new API server instance
func NewAPIServer(persistence persistence.PersistenceInterface, logger *zap.Logger) *APIServer {
	if logger == nil {
		logger = zap.NewNop()
	}

	server := &APIServer{
		persistence: persistence,
		logger:      logger,
		mux:         http.NewServeMux(),
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

	// Transaction operations (stubbed)
	s.mux.HandleFunc("/api/transactions/execute", s.handleTransactionsExecute)
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
	default:
		s.writeErrorResponse(w, http.StatusNotFound, "INVALID_OPERATION", fmt.Sprintf("Operation '%s' not supported", operation), "")
	}
}

// handleCollectionCreate handles document creation
func (s *APIServer) handleCollectionCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	var req CollectionCreateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	results := make([]any, 0, len(req.Documents))
	for _, doc := range req.Documents {
		result, err := collection.Create(doc)
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "CREATE_FAILED", "Failed to create document", err.Error())
			return
		}
		results = append(results, result)
	}

	s.writeSuccessResponse(w, http.StatusCreated, results)
}

// handleCollectionRead handles document querying
func (s *APIServer) handleCollectionRead(w http.ResponseWriter, r *http.Request, collectionName string) {
	var req CollectionReadRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	// If no query provided, create an empty one
	queryDSL := req.Query
	if queryDSL == nil {
		queryDSL = &query.QueryDSL{}
	}

	result, err := collection.Read(queryDSL)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "READ_FAILED", "Failed to read documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleCollectionUpdate handles document updates
func (s *APIServer) handleCollectionUpdate(w http.ResponseWriter, r *http.Request, collectionName string) {
	var req CollectionUpdateRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	result, err := collection.Update(&persistence.CollectionUpdate{
		Data: req.Data,
		Filter: &req.Filters,
	})
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleCollectionDelete handles document deletion
func (s *APIServer) handleCollectionDelete(w http.ResponseWriter, r *http.Request, collectionName string) {
	var req CollectionDeleteRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	collection, err := s.persistence.Collection(collectionName)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "COLLECTION_NOT_FOUND", fmt.Sprintf("Collection '%s' not found", collectionName), err.Error())
		return
	}

	result, err := collection.Delete(&req.Filters, req.Hard)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete documents", err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, result)
}

// handleCollectionsList handles listing all collections
func (s *APIServer) handleCollectionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	collections, err := s.persistence.Collections()
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

	var req CollectionCreateCollectionRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	_, err := s.persistence.Create(req.Schema)
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

	var req CollectionSchemaRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	schema, err := s.persistence.Schema(req.Name)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "SCHEMA_NOT_FOUND", fmt.Sprintf("Schema for collection '%s' not found", req.Name), err.Error())
		return
	}

	s.writeSuccessResponse(w, http.StatusOK, schema)
}

// handleCollectionsDelete handles deleting a collection
func (s *APIServer) handleCollectionsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	var req CollectionDeleteCollectionRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	deleted, err := s.persistence.Delete(req.Name)
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

// handleTransactionsExecute handles transaction execution (stubbed)
func (s *APIServer) handleTransactionsExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeErrorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", "")
		return
	}

	var req TransactionExecuteRequest
	if err := s.parseJSONBody(r, &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON in request body", err.Error())
		return
	}

	// Stubbed transaction execution
	s.logger.Info("Transaction execution requested", zap.Int("operation_count", len(req.Operations)))

	// TODO: Implement actual transaction logic using s.persistence.Transact()
	response := map[string]any{
		"executed":         true,
		"operations_count": len(req.Operations),
		"message":          "Transaction execution is stubbed - not yet implemented",
	}

	s.writeSuccessResponse(w, http.StatusOK, response)
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

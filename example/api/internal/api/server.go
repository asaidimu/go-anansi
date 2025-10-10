package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v6/example/api/internal/app"
	"github.com/asaidimu/go-anansi/v6/example/api/internal/response"
)

// APIServer encapsulates the HTTP server and its dependencies.
type APIServer struct {
	Config      *app.Config
	Logger      *zap.Logger
	Persistence *app.PersistenceManager
	Response    *response.Handler
	Handler     http.Handler // Use http.Handler interface
	server      *http.Server // The actual HTTP server
}

// NewAPIServer creates a new APIServer instance.
func NewAPIServer(cfg *app.Config, logger *zap.Logger, pm *app.PersistenceManager, rh *response.Handler) *APIServer {
	router := http.NewServeMux()
	server := &http.Server{
		Addr:    cfg.Port,
		Handler: router,
	}

	return &APIServer{
		Config:      cfg,
		Logger:      logger,
		Persistence: pm,
		Response:    rh,
		Handler:     router,
		server:      server,
	}
}

// SetupRoutes configures the API endpoints.
func (s *APIServer) SetupRoutes() {
	// Collection Data Operations
	s.Handler.(*http.ServeMux).HandleFunc("/api/v1/collections", s.handleCollections)
	s.Handler.(*http.ServeMux).HandleFunc("/api/v1/collections/{collection}/documents", s.handleCollectionDocuments)
	s.Handler.(*http.ServeMux).HandleFunc("/api/v1/collections/{collection}/documents/{id}", s.handleSingleDocument)

	// Add other routes as per spec.md
}

// Start starts the HTTP server.
func (s *APIServer) Start() error {
	s.Logger.Info(fmt.Sprintf("Server starting on port %s", s.Config.Port))
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *APIServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleCollections handles operations on /api/v1/collections
func (s *APIServer) handleCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createCollection(w, r)
	case http.MethodGet:
		s.listCollections(w, r)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, response.APIError{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "Method not allowed",
		}, r)
	}
}

// createCollection handles POST /api/v1/collections
func (s *APIServer) createCollection(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Schema schema.SchemaDefinition `json:"schema"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		}, r)
		return
	}

	_, err := s.Persistence.Anansi.CreateCollection(r.Context(), reqBody.Schema)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "CREATE_COLLECTION_ERROR",
			Message: "Failed to create collection",
			Details: err.Error(),
		}, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]string{"name": reqBody.Schema.Name}, r)
}


// listCollections handles GET /api/v1/collections
func (s *APIServer) listCollections(w http.ResponseWriter, r *http.Request) {
	includeSchema := r.URL.Query().Get("include_schema") == "true"

	collectionNames, err := s.Persistence.Anansi.ListCollections(r.Context())
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "LIST_COLLECTIONS_ERROR",
			Message: "Failed to list collections",
			Details: err.Error(),
		}, r)
		return
	}

	var collections []map[string]any
	for _, name := range collectionNames {
		collection, err := s.Persistence.Anansi.Collection(r.Context(), name)
		if err != nil {
			s.Logger.Error("Failed to get collection handle for metadata", zap.String("collection", name), zap.Error(err))
			continue // Skip this collection if we can't get a handle
		}

		metadata, err := collection.Metadata(r.Context(), nil, false) // nil filter, no force refresh
		if err != nil {
			s.Logger.Error("Failed to get collection metadata", zap.String("collection", name), zap.Error(err))
			continue // Skip this collection if we can't get metadata
		}

		colData := map[string]any{
			"name":          metadata.Name,
			"document_count": metadata.RecordCount,
			"created_at":    metadata.CreatedAt,
			"updated_at":    metadata.LastModified,
		}

		if includeSchema {
			colData["schema"] = metadata.Schema // Include full schema definition
		}
		collections = append(collections, colData)
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"collections": collections,
		"total_count": len(collections),
	}, r)
}

// handleCollectionDocuments handles operations on /api/v1/collections/{collection}/documents
func (s *APIServer) handleCollectionDocuments(w http.ResponseWriter, r *http.Request) {
	collectionName := r.PathValue("collection")
	if collectionName == "" {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Collection name is required",
		}, r)
		return
	}

	collection, err := s.Persistence.Collection(r.Context(), collectionName)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "COLLECTION_ERROR",
			Message: fmt.Sprintf("Failed to get collection %s: %v", collectionName, err),
		}, r)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.createDocuments(w, r, collection)
	case http.MethodGet:
		s.readDocuments(w, r, collection)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, response.APIError{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "Method not allowed",
		}, r)
	}
}

// createDocuments handles POST /api/v1/collections/{collection}/documents
func (s *APIServer) createDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	var reqBody struct {
		Documents []data.Document `json:"documents"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		}, r)
		return
	}

	if len(reqBody.Documents) == 0 {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "No documents provided for creation",
		}, r)
		return
	}

	results, err := collection.CreateMany(r.Context(), reqBody.Documents)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "CREATE_ERROR",
			Message: "Failed to create documents",
			Details: err.Error(),
		}, r)
		return
	}

	createdDocs := make([]data.Document, len(results))
	for i, res := range results {
		createdDocs[i] = res.Data
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]any{
		"documents": createdDocs,
	}, r)
}

// readDocuments handles GET /api/v1/collections/{collection}/documents
func (s *APIServer) readDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	// For simplicity, not parsing query parameters for filter, sort, limit, offset, fields yet.
	// This will be added in a later step.

	q := query.NewQueryBuilder().Build()
	result, err := collection.Read(r.Context(), &q)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "READ_ERROR",
			Message: "Failed to read documents",
			Details: err.Error(),
		}, r)
		return
	}
	var documents []data.Document

	switch result.Count {
	case 0:
		documents = []data.Document{}
	case 1:
		doc := result.Data.(data.Document)
		documents = []data.Document{doc}
	default:
		docs := result.Data.([]data.Document)
		documents = docs
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"documents": documents,
	}, r)
}

// handleSingleDocument handles operations on /api/v1/collections/{collection}/documents/{id}
func (s *APIServer) handleSingleDocument(w http.ResponseWriter, r *http.Request) {
	collectionName := r.PathValue("collection")
	documentID := r.PathValue("id")

	if collectionName == "" || documentID == "" {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Collection name and document ID are required",
		}, r)
		return
	}

	collection, err := s.Persistence.Collection(r.Context(), collectionName)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "COLLECTION_ERROR",
			Message: fmt.Sprintf("Failed to get collection %s: %v", collectionName, err),
		}, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.readSingleDocument(w, r, collection, documentID)
	case http.MethodPatch:
		s.updateSingleDocument(w, r, collection, documentID)
	case http.MethodPut:
		s.replaceSingleDocument(w, r, collection, documentID)
	case http.MethodDelete:
		s.deleteSingleDocument(w, r, collection, documentID)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, response.APIError{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "Method not allowed",
		}, r)
	}
}

// readSingleDocument handles GET /api/v1/collections/{collection}/documents/{id}
func (s *APIServer) readSingleDocument(w http.ResponseWriter, r *http.Request, collection base.Collection, documentID string) {
	q := query.NewQueryBuilder().Where("id").Eq(documentID).Build()
	result, err := collection.Read(r.Context(), &q)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "READ_ERROR",
			Message: "Failed to read document",
			Details: err.Error(),
		}, r)
		return
	}

	if result.Count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, response.APIError{
			Code:    "NOT_FOUND",
			Message: "Document not found",
		}, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, result.Data, r)
}

// updateSingleDocument handles PATCH /api/v1/collections/{collection}/documents/{id}
func (s *APIServer) updateSingleDocument(w http.ResponseWriter, r *http.Request, collection base.Collection, documentID string) {
	var updateData data.Document
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		}, r)
		return
	}

	// First, retrieve the existing document to preserve its metadata
	readQuery := query.NewQueryBuilder().Where("id").Eq(documentID).Build()
	readResult, err := collection.Read(r.Context(), &readQuery)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "READ_ERROR",
			Message: "Failed to retrieve document for update",
			Details: err.Error(),
		}, r)
		return
	}

	if readResult.Count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, response.APIError{
			Code:    "NOT_FOUND",
			Message: "Document not found for update",
		}, r)
		return
	}

	existingDoc, ok := readResult.Data.(data.Document)
	if !ok {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "INTERNAL_ERROR",
			Message: "Unexpected data format from persistence layer",
		}, r)
		return
	}

	// Merge updateData into existingDoc
	for key, value := range updateData {
		existingDoc[key] = value
	}

	update := &base.CollectionUpdate{
		Filter: query.NewQueryBuilder().Where("id").Eq(documentID).Build().Filters,
		Set:   existingDoc, // Pass the merged document with metadata
	}

	count, err := collection.Update(r.Context(), update)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "UPDATE_ERROR",
			Message: "Failed to update document",
			Details: err.Error(),
		}, r)
		return
	}

	if count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, response.APIError{
			Code:    "NOT_FOUND",
			Message: "Document not found or no changes applied",
		}, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{"updated_count": count}, r)
}

// replaceSingleDocument handles PUT /api/v1/collections/{collection}/documents/{id}
func (s *APIServer) replaceSingleDocument(w http.ResponseWriter, r *http.Request, collection base.Collection, documentID string) {
	var replaceData data.Document
	if err := json.NewDecoder(r.Body).Decode(&replaceData); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		}, r)
		return
	}

	// Ensure the ID in the path matches the ID in the body, if provided in body
	if id, ok := replaceData["id"]; ok && id != documentID {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Document ID in path and body do not match",
		}, r)
		return
	}
	replaceData["id"] = documentID // Ensure the document has the correct ID

	// Anansi's CreateOne can act as an upsert if the ID exists
	result, err := collection.CreateOne(r.Context(), replaceData)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "REPLACE_ERROR",
			Message: "Failed to replace document",
			Details: err.Error(),
		}, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, result.Data, r)
}

// deleteSingleDocument handles DELETE /api/v1/collections/{collection}/documents/{id}
func (s *APIServer) deleteSingleDocument(w http.ResponseWriter, r *http.Request, collection base.Collection, documentID string) {
	qf := query.NewQueryBuilder().Where("id").Eq(documentID).Build().Filters

	count, err := collection.Delete(r.Context(), qf, false) // 'false' for unsafe, consider adding a query param for this
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "DELETE_ERROR",
			Message: "Failed to delete document",
			Details: err.Error(),
		}, r)
		return
	}

	if count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, response.APIError{
			Code:    "NOT_FOUND",
			Message: "Document not found",
		}, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusNoContent, nil, r) // 204 No Content for successful delete
}

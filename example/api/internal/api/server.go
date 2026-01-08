package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaidimu/go-anansi/v6/core/common"
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
	Handler     http.Handler
	server      *http.Server
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
	mux := s.Handler.(*http.ServeMux)

	mux.HandleFunc("/api/v1/collections", s.handleCollections)
	mux.HandleFunc("/api/v1/collections/{collection}/documents", s.handleCollectionDocuments)
	mux.HandleFunc("/api/v1/collections/{collection}/documents/{id}", s.handleSingleDocument)
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

// --- Collection Handlers ---

func (s *APIServer) handleCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createCollection(w, r)
	case http.MethodGet:
		s.listCollections(w, r)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, common.NewSystemError("METHOD_NOT_ALLOWED", "Method not allowed"), r)
	}
}

func (s *APIServer) createCollection(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Schema json.RawMessage `json:"schema"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("INVALID_JSON", "Malformed request body").WithCause(err), r)
		return
	}

	sc, err := schema.From(reqBody.Schema);
	if err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.SystemErrorFrom(err).WithCode("INVALID_SCHEMA"), r)
		return
	}

	_, err = s.Persistence.Anansi.CreateCollection(r.Context(), sc)
	if err != nil {
		sysErr := common.SystemErrorFrom(err).WithOperation("CreateCollection").WithCode("CREATE_FAILED")
		s.Response.WriteError(w, http.StatusInternalServerError, sysErr, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]string{"name": sc.Name}, r)
}

func (s *APIServer) listCollections(w http.ResponseWriter, r *http.Request) {
	includeSchema := r.URL.Query().Get("include_schema") == "true"

	names, err := s.Persistence.Anansi.ListCollections(r.Context())
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, common.SystemErrorFrom(err), r)
		return
	}

	var collections []map[string]any
	for _, name := range names {
		col, _ := s.Persistence.Anansi.Collection(r.Context(), name)
		meta, err := col.Metadata(r.Context(), nil, false)
		if err != nil {
			continue
		}

		colData := map[string]any{"name": meta.Name}
		if includeSchema {
			colData["schema"] = meta.Schema
		}
		collections = append(collections, colData)
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{"collections": collections}, r, len(collections))
}

// --- Document Handlers ---

func (s *APIServer) handleCollectionDocuments(w http.ResponseWriter, r *http.Request) {
	collectionName := r.PathValue("collection")
	col, err := s.Persistence.Collection(r.Context(), collectionName)
	if err != nil {
		s.Response.WriteError(w, http.StatusNotFound, common.NewSystemError("COLLECTION_NOT_FOUND", "Collection does not exist").WithPath(collectionName), r)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.createDocuments(w, r, col)
	case http.MethodGet:
		s.readDocuments(w, r, col)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, common.NewSystemError("METHOD_NOT_ALLOWED", "Method not allowed"), r)
	}
}

func (s *APIServer) createDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	var reqBody struct {
		Documents []map[string]any `json:"documents"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("INVALID_JSON", "Invalid JSON format"), r)
		return
	}

	set, ok := data.NewDocumentSet(reqBody.Documents, r.Context())
	if !ok {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("PARSE_ERROR", "Failed to parse documents"), r)
		return
	}

	results, err := collection.CreateMany(r.Context(), set)
	rs := base.CreateResultSet(results)

	if err != nil {
		sysErr := common.SystemErrorFrom(err).WithIssues(rs.Issues()).WithCode("BATCH_CREATE_FAILED")
		s.Response.WriteError(w, http.StatusUnprocessableEntity, sysErr, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]any{"documents": rs.Documents().Sanitize(r.Context()).ToMaps()}, r)
}

func (s *APIServer) readDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	q := query.NewQueryBuilder().Build()
	result, err := collection.Read(r.Context(), &q)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, common.SystemErrorFrom(err), r)
		return
	}
	docs := result.Data.Sanitize(r.Context()).ToMaps()
	s.Response.WriteJSON(w, http.StatusOK, map[string]any{"documents": docs}, r, result.Count)
}

// --- Single Document Handlers (GET, PATCH/PUT, DELETE) ---

func (s *APIServer) handleSingleDocument(w http.ResponseWriter, r *http.Request) {
	collectionName := r.PathValue("collection")
	documentID := r.PathValue("id")

	col, err := s.Persistence.Collection(r.Context(), collectionName)
	if err != nil {
		s.Response.WriteError(w, http.StatusNotFound, common.NewSystemError("COLLECTION_NOT_FOUND", "Collection not found"), r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.readSingleDocument(w, r, col, documentID)
	case http.MethodPatch, http.MethodPut:
		s.updateSingleDocument(w, r, col, documentID)
	case http.MethodDelete:
		s.deleteSingleDocument(w, r, col, documentID)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, common.NewSystemError("METHOD_NOT_ALLOWED", "Method not allowed"), r)
	}
}

func (s *APIServer) readSingleDocument(w http.ResponseWriter, r *http.Request, col base.Collection, id string) {
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	result, err := col.Read(r.Context(), &q)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, common.SystemErrorFrom(err), r)
		return
	}

	if result.Count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, common.NewSystemError("NOT_FOUND", "Document not found").WithPath(id), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, result.Data[0].Sanitize(r.Context()).ToMap(), r)
}

func (s *APIServer) updateSingleDocument(w http.ResponseWriter, r *http.Request, col base.Collection, id string) {
	var patchData *data.Patch
	if err := json.NewDecoder(r.Body).Decode(&patchData); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("INVALID_JSON", "Invalid patch format"), r)
		return
	}

	updateParams := &base.CollectionUpdate{
		Filter:         query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters,
		Set:            patchData.Document(),
		ReturnDocument: true,
	}

	result, err := col.Update(r.Context(), updateParams)
	if err != nil {
		s.Response.WriteError(w, http.StatusUnprocessableEntity, common.SystemErrorFrom(err).WithOperation("UpdateDocument"), r)
		return
	}

	if result.Count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, common.NewSystemError("NOT_FOUND", "Document not found or no changes").WithPath(id), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, result.Data[0].Sanitize(r.Context()).ToMap(), r)
}

func (s *APIServer) deleteSingleDocument(w http.ResponseWriter, r *http.Request, col base.Collection, id string) {
	qf := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters

	count, err := col.Delete(r.Context(), qf, false)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, common.SystemErrorFrom(err), r)
		return
	}

	if count == 0 {
		s.Response.WriteError(w, http.StatusNotFound, common.NewSystemError("NOT_FOUND", "Document not found"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusNoContent, nil, r)
}

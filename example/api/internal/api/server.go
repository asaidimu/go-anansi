package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v7/example/api/internal/app"
	"github.com/asaidimu/go-anansi/v7/example/api/internal/response"
)

// APIServer encapsulates the HTTP server and its dependencies.
type APIServer struct {
	Config      *app.Config
	Logger      *zap.Logger
	Persistence *app.PersistenceManager
	Response    *response.Handler
	Handler     http.Handler
	server      *http.Server
	mux         *http.ServeMux
}

// NewAPIServer creates a new APIServer instance.
func NewAPIServer(cfg *app.Config, logger *zap.Logger, pm *app.PersistenceManager, rh *response.Handler) *APIServer {
	mux := http.NewServeMux()

	server := &http.Server{
		Addr: cfg.Port,
	}

	return &APIServer{
		Config:      cfg,
		Logger:      logger,
		Persistence: pm,
		Response:    rh,
		server:      server,
		mux:         mux,
	}
}

// SetupRoutes configures the API endpoints.
func (s *APIServer) SetupRoutes() {
	// 1. Create the middleware-wrapped versions of the handlers
	docsHandler := collectionContextMiddleware(http.HandlerFunc(s.handleCollectionDocuments))
	singleDocHandler := collectionContextMiddleware(http.HandlerFunc(s.handleSingleDocument))

	// Existing routes...
	s.mux.HandleFunc("/api/v1/collections", s.handleCollections)
	s.mux.Handle("/api/v1/collections/{collection}/documents", docsHandler)
	s.mux.Handle("/api/v1/collections/{collection}/documents/{id}", singleDocHandler)

	// NEW: Sanitization policy management
	s.mux.HandleFunc("GET /api/v1/sanitization/global", s.getGlobalSanitizationPolicy)
	s.mux.HandleFunc("PUT /api/v1/sanitization/global", s.setGlobalSanitizationPolicy)
	s.mux.HandleFunc("DELETE /api/v1/sanitization/global", s.deleteGlobalSanitizationPolicy)

	s.mux.HandleFunc("GET /api/v1/sanitization/scopes", s.listSanitizationScopes)
	s.mux.HandleFunc("GET /api/v1/sanitization/scopes/{scope}", s.getSanitizationPolicy)
	s.mux.HandleFunc("PUT /api/v1/sanitization/scopes/{scope}", s.setSanitizationPolicy)
	s.mux.HandleFunc("DELETE /api/v1/sanitization/scopes/{scope}", s.deleteSanitizationPolicy)

	s.mux.HandleFunc("POST /api/v1/sanitization/import", s.importSanitizationPolicies)
	s.mux.HandleFunc("GET /api/v1/sanitization/export", s.exportSanitizationPolicies)

	finalHandler := corsMiddleware(collectionContextMiddleware(s.mux))

	s.server.Handler = finalHandler
	s.Handler = finalHandler
}

// --- Sanitization Policy Handlers ---

// Global policy handlers
func (s *APIServer) getGlobalSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	registry := data.GetSanitizationRegistry()

	if !registry.HasGlobal() {
		s.Response.WriteError(w, http.StatusNotFound,
			common.NewSystemError("NOT_FOUND", "No global sanitization policy configured"), r)
		return
	}

	configs, err := registry.Export()
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError,
			common.SystemErrorFrom(err).WithCode("EXPORT_FAILED"), r)
		return
	}

	// Find global config (scope == "")
	for _, config := range configs {
		if config.Scope == "" {
			s.Response.WriteJSON(w, http.StatusOK, config, r)
			return
		}
	}

	s.Response.WriteError(w, http.StatusNotFound,
		common.NewSystemError("NOT_FOUND", "Global policy not found in export"), r)
}

func (s *APIServer) setGlobalSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	var config data.FieldMaskConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_JSON", "Invalid JSON format").WithCause(err), r)
		return
	}

	registry := data.GetSanitizationRegistry()
	if err := registry.SetGlobal(&config); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.SystemErrorFrom(err).WithCode("VALIDATION_FAILED"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Global sanitization policy updated",
	}, r)
}

func (s *APIServer) deleteGlobalSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	registry := data.GetSanitizationRegistry()

	// SetGlobal with nil would be ideal, but current API doesn't allow it
	// So we'll need to add a ClearGlobal method or use Clear selectively
	// For now, return the configs minus global and re-import
	configs, _ := registry.Export()
	scopedOnly := make([]*data.FieldMaskConfig, 0)
	for _, cfg := range configs {
		if cfg.Scope != "" {
			scopedOnly = append(scopedOnly, cfg)
		}
	}

	if err := registry.Import(scopedOnly); err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError,
			common.SystemErrorFrom(err).WithCode("DELETE_FAILED"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusNoContent, nil, r)
}

// Scoped policy handlers
func (s *APIServer) listSanitizationScopes(w http.ResponseWriter, r *http.Request) {
	registry := data.GetSanitizationRegistry()
	scopes := registry.List()

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"scopes":      scopes,
		"count":       registry.Count(),
		"has_global":  registry.HasGlobal(),
	}, r, len(scopes))
}

func (s *APIServer) getSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	scopeID := r.PathValue("scope")
	if scopeID == "" {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_SCOPE", "Scope identifier is required"), r)
		return
	}

	registry := data.GetSanitizationRegistry()
	if !registry.Has(scopeID) {
		s.Response.WriteError(w, http.StatusNotFound,
			common.NewSystemError("NOT_FOUND", "Sanitization policy not found").
				WithPath(scopeID), r)
		return
	}

	// Export and find the specific scope
	configs, err := registry.Export()
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError,
			common.SystemErrorFrom(err).WithCode("EXPORT_FAILED"), r)
		return
	}

	for _, config := range configs {
		if config.Scope == scopeID {
			s.Response.WriteJSON(w, http.StatusOK, config, r)
			return
		}
	}

	s.Response.WriteError(w, http.StatusNotFound,
		common.NewSystemError("NOT_FOUND", "Policy not found in export").
			WithPath(scopeID), r)
}

func (s *APIServer) setSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	scopeID := r.PathValue("scope")
	if scopeID == "" {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_SCOPE", "Scope identifier is required"), r)
		return
	}

	var config data.FieldMaskConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_JSON", "Invalid JSON format").WithCause(err), r)
		return
	}

	registry := data.GetSanitizationRegistry()
	if err := registry.Register(scopeID, &config); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.SystemErrorFrom(err).WithCode("VALIDATION_FAILED"), r)
		return
	}

	statusCode := http.StatusOK
	if !registry.Has(scopeID) {
		statusCode = http.StatusCreated
	}

	s.Response.WriteJSON(w, statusCode, map[string]string{
		"message": fmt.Sprintf("Sanitization policy for scope '%s' updated", scopeID),
		"scope":   scopeID,
	}, r)
}

func (s *APIServer) deleteSanitizationPolicy(w http.ResponseWriter, r *http.Request) {
	scopeID := r.PathValue("scope")
	if scopeID == "" {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_SCOPE", "Scope identifier is required"), r)
		return
	}

	registry := data.GetSanitizationRegistry()
	if err := registry.Unregister(scopeID); err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError,
			common.SystemErrorFrom(err).WithCode("DELETE_FAILED"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusNoContent, nil, r)
}

// Bulk operations
func (s *APIServer) exportSanitizationPolicies(w http.ResponseWriter, r *http.Request) {
	registry := data.GetSanitizationRegistry()
	configs, err := registry.Export()
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError,
			common.SystemErrorFrom(err).WithCode("EXPORT_FAILED"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"policies": configs,
		"count":    len(configs),
	}, r)
}

func (s *APIServer) importSanitizationPolicies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policies []*data.FieldMaskConfig `json:"policies"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.NewSystemError("INVALID_JSON", "Invalid JSON format").WithCause(err), r)
		return
	}

	registry := data.GetSanitizationRegistry()
	if err := registry.Import(req.Policies); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest,
			common.SystemErrorFrom(err).WithCode("IMPORT_FAILED"), r)
		return
	}

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "Sanitization policies imported successfully",
		"count":   len(req.Policies),
	}, r)
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

	sc, err := definition.FromJSON(reqBody.Schema)
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
		meta := col.Metadata(r.Context(), nil, false)
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
	collectionName, ok := common.CollectionNameFromContext(r.Context())
	if !ok || collectionName == "" {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("COLLECTION_NAME_MISSING", "Collection name not found in context"), r)
		return
	}
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

	// 1. Read the body into memory so we can use it twice (logging & decoding)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("READ_ERROR", "Could not read request body"), r)
		return
	}
	// Crucial: Close the body after reading
	defer r.Body.Close()

	// 2. Structured Logging with Zap
	// We use zap.ByteString to avoid expensive fmt.Sprintf calls and handle encoding properly
	s.Logger.Info("Received Document",
		zap.ByteString("payload", bodyBytes),
	)

	// 3. Unmarshal the captured bytes into our struct
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("INVALID_JSON", "Invalid JSON format"), r)
		return
	}

	// 4. Logic Processing
	set, ok := data.NewDocumentSet(reqBody.Documents, r.Context())
	if !ok {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("PARSE_ERROR", "Failed to parse documents"), r)
		return
	}

	results, err := collection.CreateMany(r.Context(), set)
	rs := base.CreateResultSet(results)

	if err != nil {
		if e, ok := errors.AsType[*common.SystemError](err); ok {
			s.Response.WriteError(w, http.StatusUnprocessableEntity, e.Cause, r)
			return
		}
		sysErr := common.SystemErrorFrom(err).WithIssues(rs.Issues()).WithCode("BATCH_CREATE_FAILED")
		s.Response.WriteError(w, http.StatusUnprocessableEntity, sysErr, r)
		return
	}

	// 5. Sanitization and Response
	docs, err := rs.Documents().Sanitize(r.Context())
	if err != nil {
		// Log the internal error before responding
		s.Logger.Error("Sanitization failed", zap.Error(err))
		sysErr := common.SystemErrorFrom(err).WithCode("SANITIZATION_ERROR")
		s.Response.WriteError(w, http.StatusInternalServerError, sysErr, r)
		return
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]any{"documents": docs.ToMaps()}, r)
}
func (s *APIServer) readDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	q := query.NewQueryBuilder().Build()
	result, err := collection.Read(r.Context(), &q)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, common.SystemErrorFrom(err), r)
		return
	}

	docs, err := result.Data.Sanitize(r.Context())
	if err != nil {
		sysErr := common.SystemErrorFrom(err).WithCode("SANITIZATION_ERROR")
		s.Response.WriteError(w, http.StatusInternalServerError, sysErr, r)
		return
	}
	s.Response.WriteJSON(w, http.StatusOK, map[string]any{"documents": docs.ToMaps()}, r, result.Count)
}

// --- Single Document Handlers (GET, PATCH/PUT, DELETE) ---

func (s *APIServer) handleSingleDocument(w http.ResponseWriter, r *http.Request) {
	collectionName, ok := common.CollectionNameFromContext(r.Context())
	if !ok || collectionName == "" {
		s.Response.WriteError(w, http.StatusBadRequest, common.NewSystemError("COLLECTION_NAME_MISSING", "Collection name not found in context"), r)
		return
	}
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

	doc, err := result.Data[0].Sanitize(r.Context())
	if err != nil {
		sysErr := common.SystemErrorFrom(err).WithCode("SANITIZATION_ERROR")
		s.Response.WriteError(w, http.StatusInternalServerError, sysErr, r)
		return
	}
	s.Response.WriteJSON(w, http.StatusOK, doc.ToMap(), r)
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

	doc, err := result.Data[0].Sanitize(r.Context())
	if err != nil {
		sysErr := common.SystemErrorFrom(err).WithCode("SANITIZATION_ERROR")
		s.Response.WriteError(w, http.StatusInternalServerError, sysErr, r)
		return
	}
	s.Response.WriteJSON(w, http.StatusOK, doc.ToMap(), r)
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

package handlers

import (
	"fmt"
	"net/http"

	"github.com/asaidimu/go-anansi/v6/example/api/types"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// ManagementHandler handles collection management operations
type ManagementHandler struct {
	*BaseHandler
}

func NewManagementHandler(base *BaseHandler) *ManagementHandler {
	return &ManagementHandler{BaseHandler: base}
}

// ListCollections handles listing all collections
func (h *ManagementHandler) ListCollections(w http.ResponseWriter, r *http.Request) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	collections, err := h.persistence.ListCollections(ctx)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to list collections", err))
		return
	}

	response := types.CollectionListResponse{
		Collections: collections,
	}

	h.WriteResponse(w, response)
}

// CreateCollection handles creating a new collection
func (h *ManagementHandler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionCreateCollectionRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	_, err := h.persistence.CreateCollection(ctx, req.Schema)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to create collection", err))
		return
	}

	h.WriteResponse(w, map[string]any{
		"collection": req.Schema.Name,
		"created":    true,
	})
}

// GetSchema handles getting a collection schema
func (h *ManagementHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionSchemaRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	var schemaDef *schema.SchemaDefinition
	var err error

	if req.Version != nil {
		schemaDef, err = h.persistence.Schema(ctx, req.Name, *req.Version)
	} else {
		schemaDef, err = h.persistence.Schema(ctx, req.Name)
	}

	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Schema for collection '%s' not found", req.Name), err.Error()))
		return
	}

	h.WriteResponse(w, schemaDef)
}

// DeleteCollection handles deleting a collection
func (h *ManagementHandler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionDeleteCollectionRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	deleted, err := h.persistence.Delete(ctx, req.Name)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to delete collection", err))
		return
	}

	if !deleted {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", req.Name), ""))
		return
	}

	h.WriteResponse(w, map[string]any{
		"collection": req.Name,
		"deleted":    true,
	})
}
